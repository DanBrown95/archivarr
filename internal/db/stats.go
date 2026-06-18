package db

import "context"

// Totals are library-wide backup coverage figures (present source items only).
type Totals struct {
	Files        int   `json:"files"`
	Bytes        int64 `json:"bytes"`
	BackedFiles  int   `json:"backedFiles"`
	BackedBytes  int64 `json:"backedBytes"`
	PendingFiles int   `json:"pendingFiles"`
	PendingBytes int64 `json:"pendingBytes"`
}

// SourceStat is per-source-drive coverage.
type SourceStat struct {
	DriveID      int64
	Files        int
	Bytes        int64
	BackedFiles  int
	BackedBytes  int64
	PendingFiles int
	PendingBytes int64
}

// DestStat is how much content lives on a destination drive.
type DestStat struct {
	DriveID int64
	Files   int
	Bytes   int64
}

// MediaTotals returns library-wide coverage.
func (d *DB) MediaTotals(ctx context.Context) (Totals, error) {
	var t Totals
	err := d.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(size), 0),
			COALESCE(SUM(CASE WHEN EXISTS (SELECT 1 FROM backups b WHERE b.media_item_id = m.id) THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN EXISTS (SELECT 1 FROM backups b WHERE b.media_item_id = m.id) THEN size ELSE 0 END), 0)
		FROM media_items m
		WHERE m.present = 1`).Scan(&t.Files, &t.Bytes, &t.BackedFiles, &t.BackedBytes)
	if err != nil {
		return Totals{}, err
	}
	t.PendingFiles = t.Files - t.BackedFiles
	t.PendingBytes = t.Bytes - t.BackedBytes
	return t, nil
}

// PerSourceStats returns coverage grouped by source drive.
func (d *DB) PerSourceStats(ctx context.Context) ([]SourceStat, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT
			m.source_drive_id,
			COUNT(*),
			COALESCE(SUM(m.size), 0),
			COALESCE(SUM(CASE WHEN EXISTS (SELECT 1 FROM backups b WHERE b.media_item_id = m.id) THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN EXISTS (SELECT 1 FROM backups b WHERE b.media_item_id = m.id) THEN m.size ELSE 0 END), 0)
		FROM media_items m
		WHERE m.present = 1 AND m.source_drive_id IS NOT NULL
		GROUP BY m.source_drive_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SourceStat
	for rows.Next() {
		var s SourceStat
		if err := rows.Scan(&s.DriveID, &s.Files, &s.Bytes, &s.BackedFiles, &s.BackedBytes); err != nil {
			return nil, err
		}
		s.PendingFiles = s.Files - s.BackedFiles
		s.PendingBytes = s.Bytes - s.BackedBytes
		out = append(out, s)
	}
	return out, rows.Err()
}

// PerDestinationStats returns file count and bytes stored per destination drive.
func (d *DB) PerDestinationStats(ctx context.Context) (map[int64]DestStat, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT dest_drive_id, COUNT(*), COALESCE(SUM(size), 0) FROM backups GROUP BY dest_drive_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[int64]DestStat)
	for rows.Next() {
		var s DestStat
		if err := rows.Scan(&s.DriveID, &s.Files, &s.Bytes); err != nil {
			return nil, err
		}
		res[s.DriveID] = s
	}
	return res, rows.Err()
}
