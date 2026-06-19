package db

import (
	"context"
	"database/sql"
	"errors"
)

// Session is a server-side login session keyed by an opaque random token.
type Session struct {
	Token     string
	UserID    int64
	CreatedAt int64 // unix seconds
	ExpiresAt int64 // unix seconds
}

// ErrSessionNotFound is returned when a token matches no (live) session.
var ErrSessionNotFound = errors.New("session not found")

// CreateSession stores a new session token for a user, expiring at expiresAt.
func (d *DB) CreateSession(ctx context.Context, token string, userID, expiresAt int64) error {
	_, err := d.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, expiresAt)
	return err
}

// GetSession returns a non-expired session by token, or ErrSessionNotFound if
// it is missing or has expired.
func (d *DB) GetSession(ctx context.Context, token string) (Session, error) {
	var s Session
	row := d.QueryRowContext(ctx,
		`SELECT token, user_id, created_at, expires_at FROM sessions
		 WHERE token = ? AND expires_at > strftime('%s','now')`, token)
	err := row.Scan(&s.Token, &s.UserID, &s.CreatedAt, &s.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	return s, err
}

// TouchSession extends a session's expiry (sliding window).
func (d *DB) TouchSession(ctx context.Context, token string, expiresAt int64) error {
	_, err := d.ExecContext(ctx,
		`UPDATE sessions SET expires_at = ? WHERE token = ?`, expiresAt, token)
	return err
}

// DeleteSession removes a single session (logout).
func (d *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// DeleteUserSessions removes every session for a user (used after a credential
// change so old cookies stop working everywhere).
func (d *DB) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// DeleteExpiredSessions purges sessions past their expiry.
func (d *DB) DeleteExpiredSessions(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= strftime('%s','now')`)
	return err
}
