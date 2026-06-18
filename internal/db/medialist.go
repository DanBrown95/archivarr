package db

import (
	"context"
	"strings"
)

// MediaFilter narrows a media listing.
type MediaFilter struct {
	SourceDriveID *int64
	Status        string // "", "all", "backed", "pending"
	Query         string // substring match on rel_path
	Limit         int
	Offset        int
}

// BackupInfo describes one backup copy of a media item (with the dest label).
type BackupInfo struct {
	MediaItemID int64
	DestDriveID int64
	DestLabel   string
	DestRelPath string
	CopiedAt    int64
	Status      string
}

// mediaWhere builds the shared WHERE clause + args for listing/counting media.
// Only present (source-resident) items are considered.
func mediaWhere(f MediaFilter) (string, []any) {
	clauses := []string{"m.present = 1"}
	var args []any
	if f.SourceDriveID != nil {
		clauses = append(clauses, "m.source_drive_id = ?")
		args = append(args, *f.SourceDriveID)
	}
	if f.Query != "" {
		clauses = append(clauses, "m.rel_path LIKE ?")
		args = append(args, "%"+f.Query+"%")
	}
	switch f.Status {
	case "backed":
		clauses = append(clauses, "EXISTS (SELECT 1 FROM backups b WHERE b.media_item_id = m.id)")
	case "pending":
		clauses = append(clauses, "NOT EXISTS (SELECT 1 FROM backups b WHERE b.media_item_id = m.id)")
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// CountMedia returns the number of media items matching the filter.
func (d *DB) CountMedia(ctx context.Context, f MediaFilter) (int, error) {
	where, args := mediaWhere(f)
	var n int
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_items m `+where, args...).Scan(&n)
	return n, err
}

// ListMediaPage returns one page of media items matching the filter, by path.
func (d *DB) ListMediaPage(ctx context.Context, f MediaFilter) ([]MediaItem, error) {
	where, args := mediaWhere(f)
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit, f.Offset)

	rows, err := d.QueryContext(ctx,
		`SELECT `+mediaColumns+` FROM media_items m `+where+` ORDER BY rel_path LIMIT ? OFFSET ?`, args...)
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

// MediaItemsByIDs returns the media items with the given ids.
func (d *DB) MediaItemsByIDs(ctx context.Context, ids []int64) ([]MediaItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := d.QueryContext(ctx,
		`SELECT `+mediaColumns+` FROM media_items m WHERE id IN (`+strings.Join(ph, ",")+`)`, args...)
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

// BackupsForItems returns the backups for the given media item ids, keyed by
// media item id, each with the destination drive's label.
func (d *DB) BackupsForItems(ctx context.Context, ids []int64) (map[int64][]BackupInfo, error) {
	res := make(map[int64][]BackupInfo)
	if len(ids) == 0 {
		return res, nil
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := d.QueryContext(ctx,
		`SELECT b.media_item_id, b.dest_drive_id, d.label, b.dest_rel_path, b.copied_at, b.status
		 FROM backups b JOIN drives d ON d.id = b.dest_drive_id
		 WHERE b.media_item_id IN (`+strings.Join(ph, ",")+`)
		 ORDER BY b.copied_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bi BackupInfo
		if err := rows.Scan(&bi.MediaItemID, &bi.DestDriveID, &bi.DestLabel, &bi.DestRelPath, &bi.CopiedAt, &bi.Status); err != nil {
			return nil, err
		}
		res[bi.MediaItemID] = append(res[bi.MediaItemID], bi)
	}
	return res, rows.Err()
}
