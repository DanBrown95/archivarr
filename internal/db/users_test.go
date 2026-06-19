package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
)

func TestUserLifecycle(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Fresh DB: no users → setup required.
	if n, err := d.UserCount(ctx); err != nil || n != 0 {
		t.Fatalf("expected 0 users, got %d (err=%v)", n, err)
	}

	u, err := d.CreateUser(ctx, "Admin", "hash1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 || u.Username != "Admin" || u.PasswordHash != "hash1" {
		t.Fatalf("unexpected user: %+v", u)
	}

	if n, _ := d.UserCount(ctx); n != 1 {
		t.Fatalf("expected 1 user, got %d", n)
	}

	// Username lookup is case-insensitive.
	got, err := d.GetUserByUsername(ctx, "admin")
	if err != nil || got.ID != u.ID {
		t.Fatalf("case-insensitive lookup failed: %+v err=%v", got, err)
	}

	// Missing user.
	if _, err := d.GetUserByUsername(ctx, "nobody"); !errors.Is(err, db.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}

	// Update credentials.
	if err := d.UpdateUserCredentials(ctx, u.ID, "newname", "hash2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = d.GetUserByID(ctx, u.ID)
	if got.Username != "newname" || got.PasswordHash != "hash2" {
		t.Fatalf("update not applied: %+v", got)
	}
}

func TestSessionLifecycle(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	u, err := d.CreateUser(ctx, "admin", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	future := time.Now().Add(time.Hour).Unix()
	if err := d.CreateSession(ctx, "tok-live", u.ID, future); err != nil {
		t.Fatalf("create session: %v", err)
	}

	sess, err := d.GetSession(ctx, "tok-live")
	if err != nil || sess.UserID != u.ID {
		t.Fatalf("get session: %+v err=%v", sess, err)
	}

	// Expired sessions are not returned.
	past := time.Now().Add(-time.Hour).Unix()
	if err := d.CreateSession(ctx, "tok-dead", u.ID, past); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	if _, err := d.GetSession(ctx, "tok-dead"); !errors.Is(err, db.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound for expired token, got %v", err)
	}

	// Sliding the expiry forward keeps it alive.
	if err := d.TouchSession(ctx, "tok-live", time.Now().Add(2*time.Hour).Unix()); err != nil {
		t.Fatalf("touch: %v", err)
	}

	// DeleteUserSessions wipes everything for the user.
	if err := d.DeleteUserSessions(ctx, u.ID); err != nil {
		t.Fatalf("delete user sessions: %v", err)
	}
	if _, err := d.GetSession(ctx, "tok-live"); !errors.Is(err, db.ErrSessionNotFound) {
		t.Fatalf("expected session gone after DeleteUserSessions, got %v", err)
	}

	// Single-session delete + expired cleanup.
	_ = d.CreateSession(ctx, "tok-a", u.ID, future)
	_ = d.CreateSession(ctx, "tok-b", u.ID, past)
	if err := d.DeleteSession(ctx, "tok-a"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if err := d.DeleteExpiredSessions(ctx); err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if _, err := d.GetSession(ctx, "tok-b"); !errors.Is(err, db.ErrSessionNotFound) {
		t.Fatalf("expected expired session purged, got %v", err)
	}
}

func TestSessionCascadeOnUserDelete(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	u, _ := d.CreateUser(ctx, "admin", "hash")
	future := time.Now().Add(time.Hour).Unix()
	if err := d.CreateSession(ctx, "tok", u.ID, future); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	// ON DELETE CASCADE should have removed the session.
	if _, err := d.GetSession(ctx, "tok"); !errors.Is(err, db.ErrSessionNotFound) {
		t.Fatalf("expected session cascade-deleted, got %v", err)
	}
}
