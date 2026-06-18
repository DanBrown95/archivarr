package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
)

// BackupExists reports whether a media item already has a backup on a destination.
func (d *DB) BackupExists(ctx context.Context, mediaItemID, destDriveID int64) (bool, error) {
	var one int
	err := d.QueryRowContext(ctx,
		`SELECT 1 FROM backups WHERE media_item_id = ? AND dest_drive_id = ?`,
		mediaItemID, destDriveID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// InsertBackupInput holds the fields for recording a completed backup copy.
type InsertBackupInput struct {
	MediaItemID int64
	DestDriveID int64
	DestRelPath string
	Size        int64
	VerifyHash  *string
	Status      string
	CopiedAt    int64
	VerifiedAt  *int64
}

// InsertBackup records that a media item was copied to a destination drive.
func (d *DB) InsertBackup(ctx context.Context, in InsertBackupInput) (int64, error) {
	res, err := d.ExecContext(ctx,
		`INSERT INTO backups
		 (media_item_id, dest_drive_id, dest_rel_path, size, copied_at, verified_at, verify_hash, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		in.MediaItemID, in.DestDriveID, in.DestRelPath, in.Size,
		in.CopiedAt, in.VerifiedAt, in.VerifyHash, in.Status)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListPendingForBackup returns present source items that have no backup on any
// destination yet, ordered by path (groups copies by directory).
func (d *DB) ListPendingForBackup(ctx context.Context, sourceDriveID int64) ([]MediaItem, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT m.id, m.source_drive_id, m.rel_path, m.size, m.mtime,
		        m.content_hash, m.hash_algo, m.present, m.last_scanned_at
		 FROM media_items m
		 LEFT JOIN backups b ON b.media_item_id = m.id
		 WHERE m.source_drive_id = ? AND m.present = 1 AND b.id IS NULL
		 ORDER BY m.rel_path`, sourceDriveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MediaItem
	for rows.Next() {
		m, err := scanMediaItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// BackupTo writes a consistent snapshot of the database to path using
// VACUUM INTO (safe while the database is in use). The target must not exist.
func (d *DB) BackupTo(ctx context.Context, path string) error {
	_ = os.Remove(path) // VACUUM INTO fails if the target already exists
	_, err := d.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}
