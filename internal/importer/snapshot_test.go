package importer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danbrown95/archivarr/internal/db"
)

func sp(s string) *string { return &s }

// buildSnapshot writes a standalone Archivarr DB file (as a prior backup would
// have written to <dest>/.archivarr/archivarr.db) recording five backups to a
// destination, and returns its path and the destination's marker id.
func buildSnapshot(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "archivarr.db")
	snap, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	root := "/mnt/oldnas" // the OLD source layout — irrelevant to matching now
	src, _ := snap.CreateDrive(ctx, db.CreateDriveInput{Label: "OldNAS", Role: db.RoleSource, RootPath: &root})
	marker := "marker-xyz"
	dest, _ := snap.CreateDrive(ctx, db.CreateDriveInput{Label: "Backup1", Role: db.RoleDestination, MarkerID: &marker})

	add := func(rel string, size int64, hash string) {
		id, err := snap.InsertMediaItem(ctx, db.InsertMediaItemInput{SourceDriveID: &src.ID, RelPath: rel, Size: size, ContentHash: sp(hash)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := snap.InsertBackup(ctx, db.InsertBackupInput{
			MediaItemID: id, DestDriveID: dest.ID, DestRelPath: rel, Size: size,
			Status: "ok", CopiedAt: 1000, VerifyHash: sp(hash),
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("a.mkv", 10, "ha")
	add("movies/b.mkv", 20, "hb")
	add("old/layout/c.mkv", 30, "hc") // path differs on the new NAS; matches by hash
	add("orphan.mkv", 40, "ho")       // present on the drive, but no longer in the source
	add("deleted.mkv", 50, "hd")      // recorded, but no longer on the drive (missing)
	snap.Close()
	return path, marker
}

func TestImportDestinationSnapshot(t *testing.T) {
	snapPath, marker := buildSnapshot(t)

	live, ctx := setupImportDB(t)
	// Current source (a NEW NAS) with the same files — c.mkv reorganized to a new
	// path but the same content hash; "gone.mkv" no longer exists.
	src, _ := live.CreateDrive(ctx, db.CreateDriveInput{Label: "NewNAS", Role: db.RoleSource, RootPath: sp("/mnt/newnas")})
	addMediaItem(t, live, ctx, src.ID, "a.mkv", 10, nil)
	addMediaItem(t, live, ctx, src.ID, "movies/b.mkv", 20, nil)
	addMediaItem(t, live, ctx, src.ID, "new/place/c.mkv", 30, sp("hc")) // hash fallback target
	liveDest, _ := live.CreateDrive(ctx, db.CreateDriveInput{Label: "Backup1", Role: db.RoleDestination, MarkerID: &marker})

	// Physically lay out the drive with everything the snapshot recorded EXCEPT
	// deleted.mkv (left absent to exercise the presence check).
	destRoot := t.TempDir()
	for _, rel := range []string{"a.mkv", "movies/b.mkv", "old/layout/c.mkv", "orphan.mkv"} {
		writeFile(t, filepath.Join(destRoot, rel), "x")
	}

	st, err := ImportDestinationSnapshot(ctx, live, SnapshotOptions{
		SnapshotPath: snapPath, DestRoot: destRoot, DestDriveID: liveDest.ID, DestMarkerID: marker, SourceDriveID: src.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.BackupsInSnapshot != 5 || st.Imported != 3 || st.MatchedByHash != 1 || st.Unmatched != 1 || st.Missing != 1 {
		t.Fatalf("stats = %+v; want inSnapshot=5 imported=3 byHash=1 unmatched=1 missing=1", st)
	}

	// Path match and hash-fallback match both produced backup rows; nothing created.
	for _, rel := range []string{"a.mkv", "new/place/c.mkv"} {
		item, err := live.GetMediaItem(ctx, src.ID, rel)
		if err != nil {
			t.Fatalf("media %q missing: %v", rel, err)
		}
		if ok, _ := live.BackupExists(ctx, item.ID, liveDest.ID); !ok {
			t.Fatalf("expected a backup row for %q", rel)
		}
	}
	// No phantom source was created from the snapshot's old layout.
	drives, _ := live.ListDrives(ctx)
	for _, d := range drives {
		if d.RootPath != nil && *d.RootPath == "/mnt/oldnas" {
			t.Fatal("import must not recreate the snapshot's old source")
		}
	}

	// Idempotent re-run.
	st2, err := ImportDestinationSnapshot(ctx, live, SnapshotOptions{
		SnapshotPath: snapPath, DestRoot: destRoot, DestDriveID: liveDest.ID, DestMarkerID: marker, SourceDriveID: src.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st2.Imported != 0 || st2.AlreadyKnown != 3 {
		t.Fatalf("re-run stats = %+v; want imported=0 alreadyKnown=3", st2)
	}
}

func TestImportDestinationSnapshotUnknownMarker(t *testing.T) {
	snapPath, _ := buildSnapshot(t)
	live, ctx := setupImportDB(t)
	src, _ := live.CreateDrive(ctx, db.CreateDriveInput{Label: "S", Role: db.RoleSource, RootPath: sp("/mnt/x")})
	liveDest, _ := live.CreateDrive(ctx, db.CreateDriveInput{Label: "Other", Role: db.RoleDestination})

	_, err := ImportDestinationSnapshot(ctx, live, SnapshotOptions{
		SnapshotPath: snapPath, DestDriveID: liveDest.ID, DestMarkerID: "not-in-snapshot", SourceDriveID: src.ID,
	})
	if err == nil {
		t.Fatal("expected an error when the snapshot has no record of this drive")
	}
}
