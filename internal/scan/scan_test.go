package scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/scan"
)

// setup creates a temp DB + source drive rooted at a temp tree with two files.
func setup(t *testing.T) (*scan.Engine, *db.DB, *db.Drive, string) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "alpha")
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "bravo")

	d, err := database.CreateDrive(context.Background(), db.CreateDriveInput{
		Label:    "NAS",
		Role:     db.RoleSource,
		RootPath: &root,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &scan.Engine{DB: database}, database, d, root
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanLifecycle(t *testing.T) {
	ctx := context.Background()
	eng, database, d, root := setup(t)

	// 1. First scan: both files are new, no hashing.
	res, err := eng.ScanSource(ctx, d, scan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.New != 2 || res.FilesSeen != 2 || res.Hashed != 0 {
		t.Fatalf("first scan: %+v", res)
	}
	items, _ := database.ListSourceItems(ctx, d.ID)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	for _, m := range items {
		if m.ContentHash != nil {
			t.Fatalf("expected nil hash on lazy scan: %+v", m)
		}
	}

	// 2. Rescan unchanged: all unchanged.
	res, _ = eng.ScanSource(ctx, d, scan.Options{})
	if res.Unchanged != 2 || res.New != 0 || res.Changed != 0 || res.Missing != 0 {
		t.Fatalf("rescan unchanged: %+v", res)
	}

	// 3. Modify a.txt -> changed.
	time.Sleep(1100 * time.Millisecond) // ensure mtime ticks (1s resolution)
	mustWrite(t, filepath.Join(root, "a.txt"), "alpha-modified")
	res, _ = eng.ScanSource(ctx, d, scan.Options{})
	if res.Changed != 1 || res.Unchanged != 1 {
		t.Fatalf("after modify: %+v", res)
	}

	// 4. Delete sub/b.txt -> missing.
	if err := os.Remove(filepath.Join(root, "sub", "b.txt")); err != nil {
		t.Fatal(err)
	}
	res, _ = eng.ScanSource(ctx, d, scan.Options{})
	if res.Missing != 1 {
		t.Fatalf("after delete: %+v", res)
	}
	items, _ = database.ListSourceItems(ctx, d.ID)
	for _, m := range items {
		if m.RelPath == "sub/b.txt" && m.Present {
			t.Fatalf("deleted file should be marked not present")
		}
	}

	// 5. Restore b.txt with same content -> reappeared.
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "bravo")
	res, _ = eng.ScanSource(ctx, d, scan.Options{})
	if res.Reappeared != 1 {
		t.Fatalf("after restore: %+v", res)
	}

	// 6. Hash-on-scan backfills hashes for present items.
	res, _ = eng.ScanSource(ctx, d, scan.Options{HashOnScan: true})
	if res.Hashed == 0 {
		t.Fatalf("expected hashing to occur: %+v", res)
	}
	items, _ = database.ListSourceItems(ctx, d.ID)
	for _, m := range items {
		if m.Present && m.ContentHash == nil {
			t.Fatalf("present item %q should be hashed after hash-on-scan", m.RelPath)
		}
		if m.Present && (m.HashAlgo == nil || *m.HashAlgo != "xxh3-128") {
			t.Fatalf("expected hash algo xxh3-128, got %+v", m.HashAlgo)
		}
	}
}

func TestScanNoRootPath(t *testing.T) {
	database, _ := db.Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { database.Close() })
	_ = database.Migrate(context.Background())

	d, _ := database.CreateDrive(context.Background(), db.CreateDriveInput{
		Label: "dest", Role: db.RoleDestination, MarkerID: strPtr("m1"),
	})
	eng := &scan.Engine{DB: database}
	if _, err := eng.ScanSource(context.Background(), d, scan.Options{}); err == nil {
		t.Fatal("expected error scanning a drive with no root path")
	}
}

func strPtr(s string) *string { return &s }

func TestScanExcludeAndInclude(t *testing.T) {
	ctx := context.Background()
	database, _ := db.Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { database.Close() })
	_ = database.Migrate(ctx)

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "movie.mkv"), "v")
	mustWrite(t, filepath.Join(root, "movie.nfo"), "n")
	mustWrite(t, filepath.Join(root, "poster.jpg"), "p")
	mustWrite(t, filepath.Join(root, "@eaDir", "thumb.jpg"), "t")

	d, _ := database.CreateDrive(ctx, db.CreateDriveInput{Label: "S", Role: db.RoleSource, RootPath: &root})
	eng := &scan.Engine{DB: database}

	// Exclude .nfo files and the @eaDir directory.
	res, err := eng.ScanSource(ctx, d, scan.Options{Exclude: []string{"*.nfo", "@eaDir"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.New != 2 { // movie.mkv + poster.jpg
		t.Fatalf("exclude: expected 2 tracked, got %+v", res)
	}

	// Case-insensitive extension + filename matching.
	root3 := t.TempDir()
	mustWrite(t, filepath.Join(root3, "movie.mkv"), "v")
	mustWrite(t, filepath.Join(root3, "movie.TMP"), "t") // uppercase ext
	mustWrite(t, filepath.Join(root3, "archive.7z"), "z")
	mustWrite(t, filepath.Join(root3, ".DS_Store"), "d")
	mustWrite(t, filepath.Join(root3, "incomplete.!qB"), "q") // qBittorrent mixed case
	d3, _ := database.CreateDrive(ctx, db.CreateDriveInput{Label: "S3", Role: db.RoleSource, RootPath: &root3})
	res3, err := eng.ScanSource(ctx, d3, scan.Options{Exclude: []string{"*.tmp", "*.7z", ".ds_store", "*.!qb"}})
	if err != nil {
		t.Fatal(err)
	}
	if res3.New != 1 { // only movie.mkv survives
		t.Fatalf("case-insensitive exclude: expected 1 tracked, got %+v", res3)
	}

	// Fresh source, include only mkv.
	root2 := t.TempDir()
	mustWrite(t, filepath.Join(root2, "movie.mkv"), "v")
	mustWrite(t, filepath.Join(root2, "poster.jpg"), "p")
	d2, _ := database.CreateDrive(ctx, db.CreateDriveInput{Label: "S2", Role: db.RoleSource, RootPath: &root2})
	res2, err := eng.ScanSource(ctx, d2, scan.Options{IncludeExt: []string{"mkv"}})
	if err != nil {
		t.Fatal(err)
	}
	if res2.New != 1 {
		t.Fatalf("include: expected 1 tracked (mkv only), got %+v", res2)
	}
}
