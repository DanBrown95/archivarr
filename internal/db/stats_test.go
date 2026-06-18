package db_test

import (
	"context"
	"testing"

	"github.com/danbrown95/archivarr/internal/db"
)

func strPtr(s string) *string { return &s }

func TestMediaStatsAndListing(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	src, _ := d.CreateDrive(ctx, db.CreateDriveInput{Label: "NAS", Role: db.RoleSource, RootPath: strPtr("/mnt/src")})
	marker := "m1"
	dest, _ := d.CreateDrive(ctx, db.CreateDriveInput{Label: "Backup_01", Role: db.RoleDestination, MarkerID: &marker})

	// Two media items; back up only the first.
	id1, _ := d.InsertMediaItem(ctx, db.InsertMediaItemInput{SourceDriveID: &src.ID, RelPath: "a.mkv", Size: 100, Mtime: 1, ScanTime: 1})
	_, _ = d.InsertMediaItem(ctx, db.InsertMediaItemInput{SourceDriveID: &src.ID, RelPath: "b.mkv", Size: 200, Mtime: 1, ScanTime: 1})
	if _, err := d.InsertBackup(ctx, db.InsertBackupInput{
		MediaItemID: id1, DestDriveID: dest.ID, DestRelPath: "a.mkv", Size: 100, Status: "ok", CopiedAt: 5,
	}); err != nil {
		t.Fatal(err)
	}

	// Totals.
	tot, err := d.MediaTotals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Files != 2 || tot.Bytes != 300 || tot.BackedFiles != 1 || tot.BackedBytes != 100 ||
		tot.PendingFiles != 1 || tot.PendingBytes != 200 {
		t.Fatalf("totals = %+v", tot)
	}

	// Per-source.
	ss, _ := d.PerSourceStats(ctx)
	if len(ss) != 1 || ss[0].Files != 2 || ss[0].BackedFiles != 1 || ss[0].PendingFiles != 1 {
		t.Fatalf("source stats = %+v", ss)
	}

	// Per-destination.
	ds, _ := d.PerDestinationStats(ctx)
	if ds[dest.ID].Files != 1 || ds[dest.ID].Bytes != 100 {
		t.Fatalf("dest stats = %+v", ds)
	}

	// Listing: pending filter returns b.mkv only.
	if n, _ := d.CountMedia(ctx, db.MediaFilter{Status: "pending"}); n != 1 {
		t.Fatalf("pending count = %d", n)
	}
	page, _ := d.ListMediaPage(ctx, db.MediaFilter{Status: "pending"})
	if len(page) != 1 || page[0].RelPath != "b.mkv" {
		t.Fatalf("pending page = %+v", page)
	}

	// Backups joined with labels.
	bmap, _ := d.BackupsForItems(ctx, []int64{id1})
	if len(bmap[id1]) != 1 || bmap[id1][0].DestLabel != "Backup_01" {
		t.Fatalf("backups for item = %+v", bmap)
	}
}
