package db

import (
	"context"
	"database/sql"
	"errors"
)

// MediaItem is a logical file on a source drive.
type MediaItem struct {
	ID            int64
	SourceDriveID *int64
	RelPath       string
	Size          int64
	Mtime         int64 // unix seconds
	ContentHash   *string
	HashAlgo      *string
	Present       bool
	LastScannedAt *int64 // unix seconds
}

// ErrMediaItemNotFound is returned when a media item lookup matches no row.
var ErrMediaItemNotFound = errors.New("media item not found")

const mediaColumns = `id, source_drive_id, rel_path, size, mtime,
	content_hash, hash_algo, present, last_scanned_at`

func scanMediaItem(s scannable) (MediaItem, error) {
	var (
		m           MediaItem
		srcID       sql.NullInt64
		hash        sql.NullString
		algo        sql.NullString
		present     int64
		lastScanned sql.NullInt64
	)
	if err := s.Scan(&m.ID, &srcID, &m.RelPath, &m.Size, &m.Mtime,
		&hash, &algo, &present, &lastScanned); err != nil {
		return MediaItem{}, err
	}
	m.SourceDriveID = nullInt(srcID)
	m.ContentHash = nullStr(hash)
	m.HashAlgo = nullStr(algo)
	m.Present = present != 0
	m.LastScannedAt = nullInt(lastScanned)
	return m, nil
}

// InsertMediaItemInput holds the fields for a new media item.
type InsertMediaItemInput struct {
	SourceDriveID *int64
	RelPath       string
	Size          int64
	Mtime         int64
	ContentHash   *string
	HashAlgo      *string
	ScanTime      int64
}

// InsertMediaItem inserts a new (present) media item and returns its id.
func (d *DB) InsertMediaItem(ctx context.Context, in InsertMediaItemInput) (int64, error) {
	res, err := d.ExecContext(ctx,
		`INSERT INTO media_items
		 (source_drive_id, rel_path, size, mtime, content_hash, hash_algo, present, last_scanned_at)
		 VALUES (?, ?, ?, ?, ?, ?, 1, ?)`,
		in.SourceDriveID, in.RelPath, in.Size, in.Mtime, in.ContentHash, in.HashAlgo, in.ScanTime)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateMediaItemContent records a changed file: new size/mtime/hash, present=1.
// A nil hash clears any stale content hash (file changed, not yet re-hashed).
func (d *DB) UpdateMediaItemContent(ctx context.Context, id, size, mtime int64, hash, algo *string, scanTime int64) error {
	_, err := d.ExecContext(ctx,
		`UPDATE media_items
		 SET size = ?, mtime = ?, content_hash = ?, hash_algo = ?, present = 1, last_scanned_at = ?
		 WHERE id = ?`,
		size, mtime, hash, algo, scanTime, id)
	return err
}

// SetMediaItemHash backfills the content hash for an unchanged item.
func (d *DB) SetMediaItemHash(ctx context.Context, id int64, hash, algo string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE media_items SET content_hash = ?, hash_algo = ? WHERE id = ?`, hash, algo, id)
	return err
}

// SetMediaItemPresent flips the present flag (file reappeared / went missing).
func (d *DB) SetMediaItemPresent(ctx context.Context, id int64, present bool) error {
	p := 0
	if present {
		p = 1
	}
	_, err := d.ExecContext(ctx, `UPDATE media_items SET present = ? WHERE id = ?`, p, id)
	return err
}

// ListSourceItems returns every media item (present or not) for a source drive.
func (d *DB) ListSourceItems(ctx context.Context, sourceDriveID int64) ([]MediaItem, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT `+mediaColumns+` FROM media_items WHERE source_drive_id = ?`, sourceDriveID)
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

// GetMediaItem returns a single item by source drive and relative path.
func (d *DB) GetMediaItem(ctx context.Context, sourceDriveID int64, relPath string) (*MediaItem, error) {
	row := d.QueryRowContext(ctx,
		`SELECT `+mediaColumns+` FROM media_items WHERE source_drive_id = ? AND rel_path = ?`,
		sourceDriveID, relPath)
	m, err := scanMediaItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMediaItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
