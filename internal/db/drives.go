package db

import (
	"context"
	"database/sql"
	"errors"
)

// Role describes how a drive is used.
type Role string

// A drive is either a source (media to protect) or a destination (receives
// backups) — never both. A drive serving as its own backup target provides no
// protection, so the two roles are kept distinct. (The DB CHECK still permits
// the legacy 'both' value; it's simply never written.)
const (
	RoleSource      Role = "source"
	RoleDestination Role = "destination"
)

// ValidRole reports whether r is a recognized role.
func ValidRole(r Role) bool {
	switch r {
	case RoleSource, RoleDestination:
		return true
	}
	return false
}

// Drive is a physical volume tracked by Archivarr. It exists in the DB whether
// or not it is currently mounted.
type Drive struct {
	ID            int64
	Label         string
	Role          Role
	MarkerID      *string // destination identity
	RootPath      *string // source identity
	FSUUID        *string // metadata only
	LastMountPath *string
	CapacityBytes *int64
	FreeBytes     *int64
	Online        bool
	LastSeenAt    *int64 // unix seconds
	Notes         *string
	CreatedAt     int64 // unix seconds
}

// CreateDriveInput holds the fields needed to register a drive.
type CreateDriveInput struct {
	Label    string
	Role     Role
	RootPath *string
	MarkerID *string
	Notes    *string
}

// ErrDriveNotFound is returned when a drive lookup matches no row.
var ErrDriveNotFound = errors.New("drive not found")

const driveColumns = `id, label, role, marker_id, root_path, fs_uuid,
	last_mount_path, capacity_bytes, free_bytes, online, last_seen_at, notes, created_at`

// scannable is satisfied by *sql.Row and *sql.Rows.
type scannable interface {
	Scan(dest ...any) error
}

func scanDrive(s scannable) (Drive, error) {
	var (
		d         Drive
		role      string
		online    int64
		marker    sql.NullString
		rootPath  sql.NullString
		fsUUID    sql.NullString
		lastMount sql.NullString
		notes     sql.NullString
		capacity  sql.NullInt64
		free      sql.NullInt64
		lastSeen  sql.NullInt64
	)
	if err := s.Scan(&d.ID, &d.Label, &role, &marker, &rootPath, &fsUUID,
		&lastMount, &capacity, &free, &online, &lastSeen, &notes, &d.CreatedAt); err != nil {
		return Drive{}, err
	}
	d.Role = Role(role)
	d.Online = online != 0
	d.MarkerID = nullStr(marker)
	d.RootPath = nullStr(rootPath)
	d.FSUUID = nullStr(fsUUID)
	d.LastMountPath = nullStr(lastMount)
	d.Notes = nullStr(notes)
	d.CapacityBytes = nullInt(capacity)
	d.FreeBytes = nullInt(free)
	d.LastSeenAt = nullInt(lastSeen)
	return d, nil
}

// CreateDrive inserts a drive and returns the stored row.
func (d *DB) CreateDrive(ctx context.Context, in CreateDriveInput) (*Drive, error) {
	res, err := d.ExecContext(ctx,
		`INSERT INTO drives (label, role, root_path, marker_id, notes) VALUES (?, ?, ?, ?, ?)`,
		in.Label, string(in.Role), in.RootPath, in.MarkerID, in.Notes)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return d.GetDrive(ctx, id)
}

// GetDrive returns a drive by id, or ErrDriveNotFound.
func (d *DB) GetDrive(ctx context.Context, id int64) (*Drive, error) {
	row := d.QueryRowContext(ctx, `SELECT `+driveColumns+` FROM drives WHERE id = ?`, id)
	dr, err := scanDrive(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDriveNotFound
	}
	if err != nil {
		return nil, err
	}
	return &dr, nil
}

// GetDriveByMarker returns the drive carrying the given marker id, or ErrDriveNotFound.
func (d *DB) GetDriveByMarker(ctx context.Context, marker string) (*Drive, error) {
	row := d.QueryRowContext(ctx, `SELECT `+driveColumns+` FROM drives WHERE marker_id = ?`, marker)
	dr, err := scanDrive(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDriveNotFound
	}
	if err != nil {
		return nil, err
	}
	return &dr, nil
}

// GetDriveByLabel returns the oldest drive carrying the given label, or
// ErrDriveNotFound. Labels are not unique; this is used by the legacy importer
// to find-or-create destination drives by their script label.
func (d *DB) GetDriveByLabel(ctx context.Context, label string) (*Drive, error) {
	row := d.QueryRowContext(ctx,
		`SELECT `+driveColumns+` FROM drives WHERE label = ? ORDER BY id LIMIT 1`, label)
	dr, err := scanDrive(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDriveNotFound
	}
	if err != nil {
		return nil, err
	}
	return &dr, nil
}

// ListDrives returns all drives, newest first.
func (d *DB) ListDrives(ctx context.Context) ([]Drive, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+driveColumns+` FROM drives ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Drive
	for rows.Next() {
		dr, err := scanDrive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dr)
	}
	return out, rows.Err()
}

// UpdateDrivePresence records the current online state for a drive. When online,
// it refreshes the mount path, capacity/free, and last_seen_at; when offline it
// just clears the online flag and keeps the last-known values.
func (d *DB) UpdateDrivePresence(ctx context.Context, id int64, online bool, mountPath string, capacity, free int64) error {
	if online {
		_, err := d.ExecContext(ctx,
			`UPDATE drives
			 SET online = 1, last_mount_path = ?, capacity_bytes = ?, free_bytes = ?,
			     last_seen_at = strftime('%s','now')
			 WHERE id = ?`,
			mountPath, capacity, free, id)
		return err
	}
	_, err := d.ExecContext(ctx, `UPDATE drives SET online = 0 WHERE id = ?`, id)
	return err
}

func nullStr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

func nullInt(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
