package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danbrown95/archivarr/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func TestForeignKeysEnabled(t *testing.T) {
	d := openTestDB(t)
	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	d := openTestDB(t)
	// Running again should be a no-op, not an error.
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestCreateAndGetDrive(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	root := "/mnt/nas"

	created, err := d.CreateDrive(ctx, db.CreateDriveInput{
		Label:    "NAS",
		Role:     db.RoleSource,
		RootPath: &root,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.Label != "NAS" || created.Role != db.RoleSource {
		t.Fatalf("unexpected created drive: %+v", created)
	}
	if created.RootPath == nil || *created.RootPath != root {
		t.Fatalf("root path not stored: %+v", created.RootPath)
	}
	if created.Online {
		t.Fatalf("new drive should default offline")
	}

	got, err := d.GetDrive(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Label != "NAS" {
		t.Fatalf("get label = %q", got.Label)
	}
}

func TestGetDriveNotFound(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetDrive(context.Background(), 999)
	if err != db.ErrDriveNotFound {
		t.Fatalf("err = %v, want ErrDriveNotFound", err)
	}
}

func TestMarkerLookupAndPresence(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	marker := "abc123"

	created, err := d.CreateDrive(ctx, db.CreateDriveInput{
		Label:    "Backup_01",
		Role:     db.RoleDestination,
		MarkerID: &marker,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	byMarker, err := d.GetDriveByMarker(ctx, marker)
	if err != nil {
		t.Fatalf("by marker: %v", err)
	}
	if byMarker.ID != created.ID {
		t.Fatalf("marker lookup returned wrong drive")
	}

	if err := d.UpdateDrivePresence(ctx, created.ID, true, "/mnt/usb3", 2000, 1500); err != nil {
		t.Fatalf("update presence: %v", err)
	}
	got, _ := d.GetDrive(ctx, created.ID)
	if !got.Online {
		t.Fatalf("expected online after presence update")
	}
	if got.LastMountPath == nil || *got.LastMountPath != "/mnt/usb3" {
		t.Fatalf("mount path not updated: %+v", got.LastMountPath)
	}
	if got.FreeBytes == nil || *got.FreeBytes != 1500 {
		t.Fatalf("free bytes not updated: %+v", got.FreeBytes)
	}
	if got.LastSeenAt == nil {
		t.Fatalf("last_seen_at should be set when online")
	}

	if err := d.UpdateDrivePresence(ctx, created.ID, false, "", 0, 0); err != nil {
		t.Fatalf("offline update: %v", err)
	}
	got, _ = d.GetDrive(ctx, created.ID)
	if got.Online {
		t.Fatalf("expected offline")
	}
	// Last-known mount path is retained when going offline.
	if got.LastMountPath == nil || *got.LastMountPath != "/mnt/usb3" {
		t.Fatalf("offline should retain last mount path: %+v", got.LastMountPath)
	}
}
