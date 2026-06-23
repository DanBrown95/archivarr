package importer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/hash"
	"github.com/danbrown95/archivarr/internal/pathfilter"
)

// metaDir is the destination subfolder holding Archivarr's DB snapshot; it's
// never media, so it's skipped when importing. Mirrors backup.MetaDirName (kept
// local to avoid importing the backup package here).
const metaDir = "_backup_meta"

// FSOptions controls a filesystem destination import: walk an existing backup
// drive and register files that match a source's tracked media items as existing
// backups, so they aren't re-copied.
type FSOptions struct {
	SourceDriveID int64
	DestDriveID   int64
	DestRoot      string // the destination's current mount path
	Verify        bool   // recompute hashes and compare against known source hashes
	Exclude       []string
	IncludeExt    []string
	OnProgress    func(done, total int)
	OnLog         func(msg string)
}

// FSStats summarizes a filesystem destination import.
type FSStats struct {
	FilesSeen    int `json:"filesSeen"`
	Imported     int `json:"imported"`     // backups newly registered
	Verified     int `json:"verified"`     // of those, registered with a confirmed hash
	AlreadyKnown int `json:"alreadyKnown"` // a backup was already recorded for this dest
	Unmatched    int `json:"unmatched"`    // no matching source media item (reported, not created)
	SizeMismatch int `json:"sizeMismatch"` // path matched but size differs
	HashMismatch int `json:"hashMismatch"` // verify on, hash differs from the source's
	Errors       int `json:"errors"`
}

func (o FSOptions) logf(format string, a ...any) {
	if o.OnLog != nil {
		o.OnLog(fmt.Sprintf(format, a...))
	}
}

func (o FSOptions) progress(done, total int) {
	if o.OnProgress != nil {
		o.OnProgress(done, total)
	}
}

type fsEntry struct {
	rel   string
	full  string
	size  int64
	mtime int64
}

// ImportDestinationFS walks a destination drive and registers files that match a
// source's tracked media items as existing backups. It never creates source
// media items — files with no match are counted and reported, not invented.
func ImportDestinationFS(ctx context.Context, d *db.DB, opts FSOptions) (FSStats, error) {
	var st FSStats
	if opts.DestRoot == "" {
		return st, fmt.Errorf("destination has no mount path")
	}
	if fi, err := os.Stat(opts.DestRoot); err != nil || !fi.IsDir() {
		return st, fmt.Errorf("destination not accessible: %s", opts.DestRoot)
	}

	// Load the source's tracked items once for in-memory matching by rel path.
	items, err := d.ListSourceItems(ctx, opts.SourceDriveID)
	if err != nil {
		return st, fmt.Errorf("loading source items: %w", err)
	}
	bySource := make(map[string]db.MediaItem, len(items))
	for _, m := range items {
		bySource[m.RelPath] = m
	}

	rules := pathfilter.Rules{Exclude: opts.Exclude, IncludeExt: opts.IncludeExt}

	// Pass 1: collect candidate files so we have a total for progress reporting.
	var entries []fsEntry
	walkErr := filepath.WalkDir(opts.DestRoot, func(p string, dirent fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			st.Errors++
			return nil
		}
		rel, rerr := filepath.Rel(opts.DestRoot, p)
		if rerr != nil {
			st.Errors++
			return nil
		}
		rel = filepath.ToSlash(rel)
		if dirent.IsDir() {
			if rel == metaDir {
				return filepath.SkipDir // skip Archivarr's snapshot/meta folder
			}
			return nil
		}
		if rules.Skip(rel) {
			return nil
		}
		info, ierr := dirent.Info()
		if ierr != nil {
			st.Errors++
			return nil
		}
		entries = append(entries, fsEntry{rel: rel, full: p, size: info.Size(), mtime: info.ModTime().Unix()})
		return nil
	})
	if walkErr != nil {
		return st, walkErr
	}

	total := len(entries)
	st.FilesSeen = total
	opts.logf("found %d candidate file(s) on %q", total, opts.DestRoot)

	// Pass 2: match and register.
	for i, e := range entries {
		if ctx.Err() != nil {
			return st, ctx.Err()
		}
		opts.progress(i+1, total)

		m, ok := bySource[e.rel]
		if !ok {
			st.Unmatched++
			continue
		}
		if m.Size != e.size {
			st.SizeMismatch++
			opts.logf("size mismatch %s: source %d, dest %d — skipped", e.rel, m.Size, e.size)
			continue
		}
		exists, eerr := d.BackupExists(ctx, m.ID, opts.DestDriveID)
		if eerr != nil {
			st.Errors++
			continue
		}
		if exists {
			st.AlreadyKnown++
			continue
		}

		status := "unverified"
		var verifyHash *string
		var verifiedAt *int64
		if opts.Verify {
			h, herr := hash.File(e.full)
			if herr != nil {
				st.Errors++
				opts.logf("hash failed %s: %v", e.rel, herr)
				continue
			}
			if m.ContentHash != nil {
				if *m.ContentHash != h {
					st.HashMismatch++
					opts.logf("hash mismatch %s — skipped", e.rel)
					continue
				}
				now := time.Now().Unix()
				status, verifyHash, verifiedAt = "ok", &h, &now
				st.Verified++
			}
			// If the source has no known hash there's nothing to compare against,
			// so it stays 'unverified' (size-matched only).
		}

		if _, err := d.InsertBackup(ctx, db.InsertBackupInput{
			MediaItemID: m.ID,
			DestDriveID: opts.DestDriveID,
			DestRelPath: e.rel,
			Size:        e.size,
			Status:      status,
			VerifyHash:  verifyHash,
			VerifiedAt:  verifiedAt,
			CopiedAt:    e.mtime, // true copy time is unknown; mtime is the best signal
		}); err != nil {
			st.Errors++
			opts.logf("recording backup failed %s: %v", e.rel, err)
			continue
		}
		st.Imported++
	}

	opts.logf("import done: %d imported (%d hash-verified), %d already known, %d unmatched, %d size mismatch, %d hash mismatch",
		st.Imported, st.Verified, st.AlreadyKnown, st.Unmatched, st.SizeMismatch, st.HashMismatch)
	return st, nil
}
