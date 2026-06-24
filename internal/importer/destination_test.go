package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/hash"
	"github.com/danbrown95/archivarr/internal/util"
)

func setupImportDB(t *testing.T) (*db.DB, context.Context) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return d, ctx
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func addMediaItem(t *testing.T, d *db.DB, ctx context.Context, srcID int64, rel string, size int64, contentHash *string) {
	t.Helper()
	in := db.InsertMediaItemInput{SourceDriveID: &srcID, RelPath: rel, Size: size, Mtime: 1}
	if contentHash != nil {
		algo := hash.Algo
		in.ContentHash = contentHash
		in.HashAlgo = &algo
	}
	if _, err := d.InsertMediaItem(ctx, in); err != nil {
		t.Fatal(err)
	}
}

func TestImportDestinationFS(t *testing.T) {
	d, ctx := setupImportDB(t)
	src, _ := d.CreateDrive(ctx, db.CreateDriveInput{Label: "NAS", Role: db.RoleSource})
	marker := "m1"
	dest, _ := d.CreateDrive(ctx, db.CreateDriveInput{Label: "Backup1", Role: db.RoleDestination, MarkerID: &marker})

	addMediaItem(t, d, ctx, src.ID, "a.txt", 5, nil)
	addMediaItem(t, d, ctx, src.ID, "Movies/b.txt", 5, nil)
	addMediaItem(t, d, ctx, src.ID, "c.txt", 100, nil) // size won't match the file

	destRoot := t.TempDir()
	writeFile(t, filepath.Join(destRoot, "a.txt"), "hello")                         // match (5 bytes)
	writeFile(t, filepath.Join(destRoot, "Movies", "b.txt"), "world")               // match, nested (5 bytes)
	writeFile(t, filepath.Join(destRoot, "c.txt"), "abc")                           // size mismatch (3 != 100)
	writeFile(t, filepath.Join(destRoot, "extra.txt"), "xx")                        // unmatched
	writeFile(t, filepath.Join(destRoot, util.MetaDirName, util.SnapshotName), "x") // skipped (meta dir)

	st, err := ImportDestinationFS(ctx, d, FSOptions{
		SourceDriveID: src.ID, DestDriveID: dest.ID, DestRoot: destRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.FilesSeen != 4 { // a, b, c, extra — .archivarr skipped
		t.Fatalf("FilesSeen = %d, want 4", st.FilesSeen)
	}
	if st.Imported != 2 || st.SizeMismatch != 1 || st.Unmatched != 1 {
		t.Fatalf("stats = %+v; want Imported=2 SizeMismatch=1 Unmatched=1", st)
	}

	// The matched files now have backup rows (status defaults to unverified).
	item, _ := d.GetMediaItem(ctx, src.ID, "a.txt")
	if ok, _ := d.BackupExists(ctx, item.ID, dest.ID); !ok {
		t.Fatal("expected a backup row for a.txt")
	}

	// Re-running is idempotent.
	st2, err := ImportDestinationFS(ctx, d, FSOptions{SourceDriveID: src.ID, DestDriveID: dest.ID, DestRoot: destRoot})
	if err != nil {
		t.Fatal(err)
	}
	if st2.Imported != 0 || st2.AlreadyKnown != 2 {
		t.Fatalf("re-run stats = %+v; want Imported=0 AlreadyKnown=2", st2)
	}
}

func TestImportDestinationFSVerify(t *testing.T) {
	d, ctx := setupImportDB(t)
	src, _ := d.CreateDrive(ctx, db.CreateDriveInput{Label: "NAS", Role: db.RoleSource})
	marker := "m2"
	dest, _ := d.CreateDrive(ctx, db.CreateDriveInput{Label: "Backup1", Role: db.RoleDestination, MarkerID: &marker})

	destRoot := t.TempDir()
	writeFile(t, filepath.Join(destRoot, "good.txt"), "hello")
	writeFile(t, filepath.Join(destRoot, "bad.txt"), "hello")

	// good.txt: source hash matches the file's real hash → verifies to 'ok'.
	goodHash, err := hash.File(filepath.Join(destRoot, "good.txt"))
	if err != nil {
		t.Fatal(err)
	}
	addMediaItem(t, d, ctx, src.ID, "good.txt", 5, &goodHash)
	// bad.txt: source hash is wrong → hash mismatch, not imported.
	wrong := "00000000000000000000000000000000"
	addMediaItem(t, d, ctx, src.ID, "bad.txt", 5, &wrong)

	st, err := ImportDestinationFS(ctx, d, FSOptions{
		SourceDriveID: src.ID, DestDriveID: dest.ID, DestRoot: destRoot, Verify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Imported != 1 || st.Verified != 1 || st.HashMismatch != 1 {
		t.Fatalf("stats = %+v; want Imported=1 Verified=1 HashMismatch=1", st)
	}
}
