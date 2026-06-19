package db

import (
	"context"
	"database/sql"
	"errors"
)

// User is an account that can sign in to the web UI. The app currently manages
// exactly one (the admin created at first-run), though the schema allows more.
type User struct {
	ID           int64
	Username     string
	PasswordHash string // bcrypt
	CreatedAt    int64  // unix seconds
	UpdatedAt    int64  // unix seconds
}

// ErrUserNotFound is returned when a user lookup matches no row.
var ErrUserNotFound = errors.New("user not found")

const userColumns = `id, username, password_hash, created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

// UserCount returns the number of accounts. Zero means first-run setup is required.
func (d *DB) UserCount(ctx context.Context) (int, error) {
	var n int
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// GetUserByUsername looks up a user by name (case-insensitive). Returns
// ErrUserNotFound if absent.
func (d *DB) GetUserByUsername(ctx context.Context, username string) (User, error) {
	row := d.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ? COLLATE NOCASE`, username)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return u, err
}

// GetUserByID looks up a user by id. Returns ErrUserNotFound if absent.
func (d *DB) GetUserByID(ctx context.Context, id int64) (User, error) {
	row := d.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return u, err
}

// CreateUser inserts a new account with an already-hashed password.
func (d *DB) CreateUser(ctx context.Context, username, passwordHash string) (User, error) {
	res, err := d.ExecContext(ctx,
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`, username, passwordHash)
	if err != nil {
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return d.GetUserByID(ctx, id)
}

// UpdateUserCredentials updates a user's username and password hash, bumping
// updated_at.
func (d *DB) UpdateUserCredentials(ctx context.Context, id int64, username, passwordHash string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE users SET username = ?, password_hash = ?, updated_at = strftime('%s','now') WHERE id = ?`,
		username, passwordHash, id)
	return err
}
