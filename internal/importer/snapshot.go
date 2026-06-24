package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/danbrown95/archivarr/internal/db"
)

// SnapshotPath returns the path to a destination's Archivarr DB snapshot.
func SnapshotPath(destRoot string) string {
	return filepath.Join(destRoot, metaDir, "archivarr.db")
}

// HasSnapshot reports whether a destination carries an Archivarr DB snapshot
// (written by Archivarr's own backups), enabling a higher-fidelity import.
func HasSnapshot(destRoot string) bool {
	fi, err := os.Stat(SnapshotPath(destRoot))
	return err == nil && !fi.IsDir()
}

// SnapshotOptions controls a snapshot destination import. It reads what the
// drive recorded as backed up (paths, stored hashes, original metadata) and
// matches those against the chosen *current* source — it never creates sources
// or media items.
type SnapshotOptions struct {
	SnapshotPath  string
	DestRoot      string // the destination's mount path, for verifying files are present
	DestDriveID   int64  // the live destination drive being imported
	DestMarkerID  string // its marker id, used to locate it within the snapshot
	SourceDriveID int64  // the current source to match recorded files against
	OnProgress    func(done, total int)
	OnLog         func(msg string)
}

// SnapshotStats summarizes a snapshot destination import.
type SnapshotStats struct {
	BackupsInSnapshot int `json:"backupsInSnapshot"`
	Imported          int `json:"imported"`
	MatchedByHash     int `json:"matchedByHash"` // of imported, matched via hash (path missed)
	AlreadyKnown      int `json:"alreadyKnown"`
	Unmatched         int `json:"unmatched"` // no current source media item matched
	Missing           int `json:"missing"`   // recorded in the snapshot but not present on the drive
	Errors            int `json:"errors"`
}

func (o SnapshotOptions) logf(format string, a ...any) {
	if o.OnLog != nil {
		o.OnLog(fmt.Sprintf(format, a...))
	}
}

func (o SnapshotOptions) progress(done, total int) {
	if o.OnProgress != nil {
		o.OnProgress(done, total)
	}
}

// snapRecord is one backup (plus its media item's path/hash) read from a snapshot.
type snapRecord struct {
	destRel     string
	backupSize  int64
	copiedAt    int64
	verifiedAt  sql.NullInt64
	verifyHash  sql.NullString
	status      string
	relPath     string
	contentHash sql.NullString
}

// contentKey returns the file's content hash for fallback matching: the backup's
// verify hash (the bytes actually copied) preferred, else the media item's hash.
func (r snapRecord) contentKey() string {
	if r.verifyHash.Valid && r.verifyHash.String != "" {
		return r.verifyHash.String
	}
	if r.contentHash.Valid {
		return r.contentHash.String
	}
	return ""
}

// ImportDestinationSnapshot reads the backups a drive recorded in its own
// Archivarr DB snapshot and registers them against the chosen current source —
// matching by relative path, falling back to content hash — so files already on
// the drive aren't re-copied. Unmatched files are reported, never created.
// Idempotent.
func ImportDestinationSnapshot(ctx context.Context, d *db.DB, opts SnapshotOptions) (SnapshotStats, error) {
	var st SnapshotStats
	if opts.DestMarkerID == "" {
		return st, fmt.Errorf("destination has no marker id to match within the snapshot")
	}

	// Open the snapshot read-only (query_only blocks any writes to the drive).
	snap, err := sql.Open("sqlite", opts.SnapshotPath+"?_pragma=busy_timeout(5000)&_pragma=query_only(true)")
	if err != nil {
		return st, fmt.Errorf("opening snapshot: %w", err)
	}
	defer snap.Close()
	snap.SetMaxOpenConns(1)
	if err := snap.PingContext(ctx); err != nil {
		return st, fmt.Errorf("reading snapshot: %w", err)
	}

	// Locate this physical drive within the snapshot by its marker.
	var snapDestID int64
	err = snap.QueryRowContext(ctx, `SELECT id FROM drives WHERE marker_id = ?`, opts.DestMarkerID).Scan(&snapDestID)
	if errors.Is(err, sql.ErrNoRows) {
		return st, fmt.Errorf("snapshot has no record of this drive (marker %s)", opts.DestMarkerID)
	}
	if err != nil {
		return st, fmt.Errorf("reading snapshot drives: %w", err)
	}

	rows, err := snap.QueryContext(ctx, `
		SELECT b.dest_rel_path, b.size, b.copied_at, b.verified_at, b.verify_hash, b.status,
		       m.rel_path, m.content_hash
		FROM backups b
		JOIN media_items m ON m.id = b.media_item_id
		WHERE b.dest_drive_id = ?
		ORDER BY m.rel_path`, snapDestID)
	if err != nil {
		return st, fmt.Errorf("reading snapshot backups: %w", err)
	}
	defer rows.Close()

	var records []snapRecord
	for rows.Next() {
		var r snapRecord
		if err := rows.Scan(&r.destRel, &r.backupSize, &r.copiedAt, &r.verifiedAt, &r.verifyHash, &r.status,
			&r.relPath, &r.contentHash); err != nil {
			return st, fmt.Errorf("scanning snapshot row: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	st.BackupsInSnapshot = len(records)
	opts.logf("snapshot records %d backup(s) for this drive", len(records))

	// Index the current source's tracked items for path + hash matching.
	items, err := d.ListSourceItems(ctx, opts.SourceDriveID)
	if err != nil {
		return st, fmt.Errorf("loading source items: %w", err)
	}
	byRel := make(map[string]db.MediaItem, len(items))
	byHash := make(map[string]db.MediaItem, len(items))
	ambiguousHash := make(map[string]bool)
	for _, m := range items {
		byRel[m.RelPath] = m
		if m.ContentHash == nil || *m.ContentHash == "" {
			continue
		}
		h := *m.ContentHash
		if ambiguousHash[h] {
			continue
		}
		if _, dup := byHash[h]; dup {
			// Two source files share this content — a hash match would be
			// ambiguous, so exclude it (path matching still applies).
			delete(byHash, h)
			ambiguousHash[h] = true
			continue
		}
		byHash[h] = m
	}

	for i, r := range records {
		if ctx.Err() != nil {
			return st, ctx.Err()
		}
		opts.progress(i+1, len(records))

		// The snapshot is trusted for what it recorded, but the drive may have
		// changed since — only record files that are actually present at their
		// recorded path, so we never claim coverage for moved/deleted files.
		if opts.DestRoot != "" {
			full := filepath.Join(opts.DestRoot, filepath.FromSlash(r.destRel))
			if fi, statErr := os.Stat(full); statErr != nil || fi.IsDir() {
				st.Missing++
				continue
			}
		}

		m, ok := byRel[r.relPath]
		viaHash := false
		if !ok {
			if key := r.contentKey(); key != "" {
				if mm, hok := byHash[key]; hok {
					m, ok, viaHash = mm, true, true
				}
			}
		}
		if !ok {
			st.Unmatched++
			continue
		}

		exists, err := d.BackupExists(ctx, m.ID, opts.DestDriveID)
		if err != nil {
			st.Errors++
			continue
		}
		if exists {
			st.AlreadyKnown++
			continue
		}
		if _, err := d.InsertBackup(ctx, db.InsertBackupInput{
			MediaItemID: m.ID,
			DestDriveID: opts.DestDriveID,
			DestRelPath: r.destRel,
			Size:        r.backupSize,
			Status:      r.status,
			VerifyHash:  nullStrPtr(r.verifyHash),
			VerifiedAt:  nullIntPtr(r.verifiedAt),
			CopiedAt:    r.copiedAt,
		}); err != nil {
			st.Errors++
			continue
		}
		st.Imported++
		if viaHash {
			st.MatchedByHash++
		}
	}

	opts.logf("snapshot import done: %d imported (%d by hash), %d already known, %d unmatched, %d missing from drive, %d errors",
		st.Imported, st.MatchedByHash, st.AlreadyKnown, st.Unmatched, st.Missing, st.Errors)
	return st, nil
}

func nullStrPtr(n sql.NullString) *string {
	if !n.Valid || n.String == "" {
		return nil
	}
	s := n.String
	return &s
}

func nullIntPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
