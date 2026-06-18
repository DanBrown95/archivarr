package db

import (
	"context"
	"database/sql"
)

// DeleteBackupsForDest removes all backup records pointing at a destination
// drive (re-queue after a destination dies). The media items resurface as
// pending. Returns the number of backup rows removed.
func (d *DB) DeleteBackupsForDest(ctx context.Context, destID int64) (int64, error) {
	res, err := d.ExecContext(ctx, `DELETE FROM backups WHERE dest_drive_id = ?`, destID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DestSourceBreakdown is how much of a destination's content came from a source.
type DestSourceBreakdown struct {
	SourceDriveID *int64
	Files         int
	Bytes         int64
}

// DestContentsBySource summarizes what a destination holds, grouped by the
// source drive each backed-up file came from.
func (d *DB) DestContentsBySource(ctx context.Context, destID int64) ([]DestSourceBreakdown, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT m.source_drive_id, COUNT(*), COALESCE(SUM(b.size), 0)
		FROM backups b JOIN media_items m ON m.id = b.media_item_id
		WHERE b.dest_drive_id = ?
		GROUP BY m.source_drive_id`, destID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DestSourceBreakdown
	for rows.Next() {
		var b DestSourceBreakdown
		var srcID sql.NullInt64
		if err := rows.Scan(&srcID, &b.Files, &b.Bytes); err != nil {
			return nil, err
		}
		b.SourceDriveID = nullInt(srcID)
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteDrive removes a drive and all data associated with it, in one
// transaction: backups stored on it (if a destination), and the media items it
// sourced together with their backups (if a source). Leaves no orphans.
func (d *DB) DeleteDrive(ctx context.Context, id int64) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []struct {
		q    string
		args []any
	}{
		{`DELETE FROM backups WHERE dest_drive_id = ?`, []any{id}},
		{`DELETE FROM backups WHERE media_item_id IN (SELECT id FROM media_items WHERE source_drive_id = ?)`, []any{id}},
		{`DELETE FROM media_items WHERE source_drive_id = ?`, []any{id}},
		{`DELETE FROM drives WHERE id = ?`, []any{id}},
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s.q, s.args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}
