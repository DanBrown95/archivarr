package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/hash"
	"github.com/danbrown95/archivarr/internal/util"
)

// MetaDirName is the folder on a destination drive holding the DB snapshot.
const MetaDirName = "_backup_meta"

// Stats summarizes a backup run.
type Stats struct {
	Total       int   `json:"total"`
	Copied      int   `json:"copied"`
	Failed      int   `json:"failed"`
	Bytes       int64 `json:"bytes"`
	StoppedFull bool  `json:"stoppedFull"`
	Remaining   int   `json:"remaining"`
}

// Progress lets a caller observe a running backup. Either field may be nil.
type Progress struct {
	OnProgress func(done, total int)
	OnLog      func(msg string)
}

func (p Progress) progress(done, total int) {
	if p.OnProgress != nil {
		p.OnProgress(done, total)
	}
}

func (p Progress) logf(format string, a ...any) {
	if p.OnLog != nil {
		p.OnLog(fmt.Sprintf(format, a...))
	}
}

// Runner executes backup jobs.
type Runner struct {
	DB *db.DB
	// DiskFree returns the bytes available at a mount path. Injected so this
	// package needs no OS-specific drive dependency. If nil, the space check
	// is skipped.
	DiskFree func(path string) (uint64, error)
}

// RunBackup copies not-yet-backed-up files from source to dest, verifying each
// copy by hash, and snapshots the tracking DB onto the destination. When itemIDs
// is empty it backs up every pending file for the source; otherwise it backs up
// only those items (that belong to the source, are present, and aren't already
// on this destination).
//
// skip, when non-nil, drops any pending file whose relative path it matches, so
// the backup honors the current include/exclude rules even if media_items is
// stale (e.g. rules changed since the last scan).
func (r *Runner) RunBackup(ctx context.Context, source, dest *db.Drive, itemIDs []int64, skip func(relPath string) bool, prog Progress) (*Stats, error) {
	if source.RootPath == nil || *source.RootPath == "" {
		return nil, fmt.Errorf("source drive %d has no root path", source.ID)
	}
	if dest.LastMountPath == nil || *dest.LastMountPath == "" {
		return nil, fmt.Errorf("destination drive %d is not mounted", dest.ID)
	}
	srcRoot, destRoot := *source.RootPath, *dest.LastMountPath
	if fi, err := os.Stat(srcRoot); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("source root not accessible: %s", srcRoot)
	}
	if fi, err := os.Stat(destRoot); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("destination root not accessible: %s", destRoot)
	}

	var pending []db.MediaItem
	if len(itemIDs) == 0 {
		p, err := r.DB.ListPendingForBackup(ctx, source.ID)
		if err != nil {
			return nil, err
		}
		pending = p
	} else {
		items, err := r.DB.MediaItemsByIDs(ctx, itemIDs)
		if err != nil {
			return nil, err
		}
		for _, m := range items {
			if m.SourceDriveID == nil || *m.SourceDriveID != source.ID || !m.Present {
				continue
			}
			if exists, _ := r.DB.BackupExists(ctx, m.ID, dest.ID); exists {
				continue // already on this destination
			}
			pending = append(pending, m)
		}
	}

	// Honor the current include/exclude rules at copy time (media_items may be
	// stale relative to settings changed since the last scan).
	if skip != nil {
		kept := pending[:0]
		var excluded int
		for _, m := range pending {
			if skip(m.RelPath) {
				excluded++
				continue
			}
			kept = append(kept, m)
		}
		pending = kept
		if excluded > 0 {
			prog.logf("skipped %d file(s) matching current exclude/include rules", excluded)
		}
	}

	stats := &Stats{Total: len(pending)}
	prog.logf("backup start: %d pending file(s), %q -> %q", len(pending), source.Label, dest.Label)

	for i, item := range pending {
		if ctx.Err() != nil {
			stats.Remaining = len(pending) - i
			return stats, ctx.Err()
		}

		if r.DiskFree != nil {
			if free, ferr := r.DiskFree(destRoot); ferr == nil && free < uint64(item.Size) {
				stats.StoppedFull = true
				stats.Remaining = len(pending) - i
				prog.logf("destination full: next file needs %s, %s free — stopping, %d file(s) remain",
					util.Bytes(item.Size), util.Bytes(int64(free)), stats.Remaining)
				break
			}
		}

		srcPath := filepath.Join(srcRoot, filepath.FromSlash(item.RelPath))
		destPath := filepath.Join(destRoot, filepath.FromSlash(item.RelPath))

		hashHex, size, cerr := CopyFile(srcPath, destPath)
		if cerr != nil {
			stats.Failed++
			prog.logf("copy failed %s: %v", item.RelPath, cerr)
			continue
		}

		// Integrity: if we already knew this file's hash, the freshly-copied
		// bytes must match. A mismatch means the source changed since the scan;
		// drop the copy and leave it pending for the next scan to reconcile.
		if item.ContentHash != nil && *item.ContentHash != hashHex {
			os.Remove(destPath)
			stats.Failed++
			prog.logf("hash mismatch %s (source changed since scan?) — left pending", item.RelPath)
			continue
		}
		if item.ContentHash == nil {
			_ = r.DB.SetMediaItemHash(ctx, item.ID, hashHex, hash.Algo)
		}

		now := time.Now().Unix()
		if _, err := r.DB.InsertBackup(ctx, db.InsertBackupInput{
			MediaItemID: item.ID,
			DestDriveID: dest.ID,
			DestRelPath: item.RelPath,
			Size:        size,
			VerifyHash:  &hashHex,
			Status:      "ok",
			CopiedAt:    now,
			VerifiedAt:  &now,
		}); err != nil {
			stats.Failed++
			prog.logf("recording backup failed %s: %v", item.RelPath, err)
			continue
		}

		stats.Copied++
		stats.Bytes += size
		prog.progress(i+1, len(pending))
	}

	// Best-effort: snapshot the tracking DB onto the destination drive so the
	// backup is recoverable even if the source/NAS dies.
	if err := r.copyDBMeta(ctx, destRoot); err != nil {
		prog.logf("warning: could not write DB snapshot to destination: %v", err)
	} else {
		prog.logf("wrote DB snapshot to %s", filepath.Join(destRoot, MetaDirName))
	}

	prog.logf("backup done: copied %d, failed %d, %s", stats.Copied, stats.Failed, util.Bytes(stats.Bytes))
	return stats, nil
}

func (r *Runner) copyDBMeta(ctx context.Context, destRoot string) error {
	metaDir := filepath.Join(destRoot, MetaDirName)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		return err
	}
	return r.DB.BackupTo(ctx, filepath.Join(metaDir, "archivarr.db"))
}
