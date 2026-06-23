package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/danbrown95/archivarr/internal/backup"
	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/hash"
	"github.com/danbrown95/archivarr/internal/pathfilter"
	"github.com/danbrown95/archivarr/internal/scan"
)

func TestCopyFileIntegrity(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	if err := os.WriteFile(src, []byte("hello archivarr"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out", "src.bin")

	h, n, err := backup.CopyFile(src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if n != 15 {
		t.Fatalf("size = %d, want 15", n)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "hello archivarr" {
		t.Fatalf("dest content = %q", got)
	}
	want, _ := hash.File(src)
	if h != want {
		t.Fatalf("copy hash %q != file hash %q", h, want)
	}
	// No temp file should remain.
	if _, err := os.Stat(dest + backup.TempSuffix); !os.IsNotExist(err) {
		t.Fatalf("temp file lingered")
	}
}

// harness builds a DB with a scanned source and a mounted destination.
func harness(t *testing.T) (*backup.Runner, *db.DB, *db.Drive, *db.Drive, string, string) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	srcRoot := t.TempDir()
	destRoot := t.TempDir()
	write(t, filepath.Join(srcRoot, "Movies", "a.mkv"), "movie a data")
	write(t, filepath.Join(srcRoot, "Movies", "b.mkv"), "movie b is bigger data")

	ctx := context.Background()
	source, _ := database.CreateDrive(ctx, db.CreateDriveInput{Label: "NAS", Role: db.RoleSource, RootPath: &srcRoot})
	marker := "dest-marker"
	dest, _ := database.CreateDrive(ctx, db.CreateDriveInput{Label: "Backup_01", Role: db.RoleDestination, MarkerID: &marker})
	// Mark the destination mounted at destRoot.
	if err := database.UpdateDrivePresence(ctx, dest.ID, true, destRoot, 1<<40, 1<<40); err != nil {
		t.Fatal(err)
	}

	// Populate media_items.
	eng := &scan.Engine{DB: database}
	if _, err := eng.ScanSource(ctx, source, scan.Options{}); err != nil {
		t.Fatal(err)
	}

	source, _ = database.GetDrive(ctx, source.ID)
	dest, _ = database.GetDrive(ctx, dest.ID)
	runner := &backup.Runner{DB: database, DiskFree: func(string) (uint64, error) { return 1 << 40, nil }}
	return runner, database, source, dest, srcRoot, destRoot
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunBackupCopiesVerifiesRecords(t *testing.T) {
	ctx := context.Background()
	runner, database, source, dest, _, destRoot := harness(t)

	stats, err := runner.RunBackup(ctx, source, dest, nil, nil, backup.Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Copied != 2 || stats.Failed != 0 || stats.Total != 2 {
		t.Fatalf("stats = %+v", stats)
	}

	// Files exist on the destination.
	for _, rel := range []string{"Movies/a.mkv", "Movies/b.mkv"} {
		if _, err := os.Stat(filepath.Join(destRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s on destination: %v", rel, err)
		}
	}

	// Nothing pending anymore.
	pending, _ := database.ListPendingForBackup(ctx, source.ID)
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending after backup, got %d", len(pending))
	}

	// media_items hashes were backfilled during copy.
	items, _ := database.ListSourceItems(ctx, source.ID)
	for _, m := range items {
		if m.ContentHash == nil {
			t.Fatalf("expected content hash backfilled for %s", m.RelPath)
		}
	}

	// DB snapshot written to the destination.
	if _, err := os.Stat(filepath.Join(destRoot, backup.MetaDirName, "archivarr.db")); err != nil {
		t.Fatalf("expected DB snapshot on destination: %v", err)
	}

	// Re-running copies nothing.
	stats2, err := runner.RunBackup(ctx, source, dest, nil, nil, backup.Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if stats2.Copied != 0 || stats2.Total != 0 {
		t.Fatalf("second run should be a no-op: %+v", stats2)
	}
}

func TestRunBackupSpecificItem(t *testing.T) {
	ctx := context.Background()
	runner, database, source, dest, _, destRoot := harness(t)

	items, _ := database.ListSourceItems(ctx, source.ID)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// Back up only the first item.
	target := items[0]
	stats, err := runner.RunBackup(ctx, source, dest, []int64{target.ID}, nil, backup.Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 1 || stats.Copied != 1 {
		t.Fatalf("expected exactly 1 copied, got %+v", stats)
	}
	if _, err := os.Stat(filepath.Join(destRoot, filepath.FromSlash(target.RelPath))); err != nil {
		t.Fatalf("target file should be on destination: %v", err)
	}
	// The other item is still pending.
	pending, _ := database.ListPendingForBackup(ctx, source.ID)
	if len(pending) != 1 {
		t.Fatalf("expected 1 still pending, got %d", len(pending))
	}
	// Re-backing-up the same item is a no-op (already on this destination).
	stats2, _ := runner.RunBackup(ctx, source, dest, []int64{target.ID}, nil, backup.Progress{})
	if stats2.Total != 0 {
		t.Fatalf("expected no-op re-backup, got %+v", stats2)
	}
}

func TestRunBackupRespectsFilter(t *testing.T) {
	ctx := context.Background()
	runner, database, source, dest, _, destRoot := harness(t)

	// b.mkv is excluded by the current rules even though it's still pending
	// (e.g. the exclude was added after the last scan tracked it).
	skip := pathfilter.Rules{Exclude: []string{"b.mkv"}}.Skip
	stats, err := runner.RunBackup(ctx, source, dest, nil, skip, backup.Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 1 || stats.Copied != 1 {
		t.Fatalf("expected exactly 1 copied (a.mkv), got %+v", stats)
	}
	if _, err := os.Stat(filepath.Join(destRoot, "Movies", "a.mkv")); err != nil {
		t.Fatalf("a.mkv should be on destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destRoot, "Movies", "b.mkv")); !os.IsNotExist(err) {
		t.Fatalf("excluded b.mkv should not be copied")
	}
	// b.mkv stays pending (it was skipped, not backed up).
	pending, _ := database.ListPendingForBackup(ctx, source.ID)
	if len(pending) != 1 || pending[0].RelPath != "Movies/b.mkv" {
		t.Fatalf("expected only Movies/b.mkv pending, got %+v", pending)
	}
}

func TestRunBackupAdoptsMatchAndSkipsConflict(t *testing.T) {
	ctx := context.Background()
	runner, database, source, dest, _, destRoot := harness(t)

	// Pre-place files on the destination BEFORE backup runs:
	//   Movies/a.mkv — identical to the source → should be ADOPTED (recorded, not rewritten)
	//   Movies/b.mkv — different from the source → CONFLICT (left untouched, not backed up)
	write(t, filepath.Join(destRoot, "Movies", "a.mkv"), "movie a data")        // matches source
	write(t, filepath.Join(destRoot, "Movies", "b.mkv"), "DO NOT OVERWRITE ME") // differs from source

	stats, err := runner.RunBackup(ctx, source, dest, nil, nil, backup.Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Adopted != 1 || stats.Conflicts != 1 || stats.Copied != 0 {
		t.Fatalf("stats = %+v; want Adopted=1 Conflicts=1 Copied=0", stats)
	}

	// The conflicting file must be byte-for-byte untouched.
	got, _ := os.ReadFile(filepath.Join(destRoot, "Movies", "b.mkv"))
	if string(got) != "DO NOT OVERWRITE ME" {
		t.Fatalf("conflict file was overwritten: %q", got)
	}

	// a.mkv adopted → backup recorded; b.mkv conflict → no record (still pending).
	items, _ := database.ListSourceItems(ctx, source.ID)
	for _, m := range items {
		ok, _ := database.BackupExists(ctx, m.ID, dest.ID)
		if m.RelPath == "Movies/a.mkv" && !ok {
			t.Fatal("a.mkv should have been adopted (backup recorded)")
		}
		if m.RelPath == "Movies/b.mkv" && ok {
			t.Fatal("b.mkv conflict must NOT have a backup record")
		}
	}
}

func TestRunBackupStopsWhenFull(t *testing.T) {
	ctx := context.Background()
	runner, database, source, dest, _, _ := harness(t)
	// Pretend the destination has only 1 byte free.
	runner.DiskFree = func(string) (uint64, error) { return 1, nil }

	stats, err := runner.RunBackup(ctx, source, dest, nil, nil, backup.Progress{})
	if err != nil {
		t.Fatal(err)
	}
	if !stats.StoppedFull {
		t.Fatalf("expected StoppedFull, got %+v", stats)
	}
	if stats.Copied != 0 || stats.Remaining != stats.Total {
		t.Fatalf("expected nothing copied and all remaining: %+v", stats)
	}
	// Everything still pending.
	pending, _ := database.ListPendingForBackup(ctx, source.ID)
	if len(pending) != 2 {
		t.Fatalf("expected 2 still pending, got %d", len(pending))
	}
}
