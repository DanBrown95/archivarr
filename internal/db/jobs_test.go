package db_test

import (
	"context"
	"testing"

	"github.com/danbrown95/archivarr/internal/db"
)

func TestJobOriginAndQueue(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	ps := `{"driveId":1}`

	auto, err := d.CreateJob(ctx, "scan", &ps, db.JobOriginAuto)
	if err != nil {
		t.Fatalf("create auto job: %v", err)
	}
	manual, err := d.CreateJob(ctx, "scan", &ps, db.JobOriginManual)
	if err != nil {
		t.Fatalf("create manual job: %v", err)
	}
	// An unrecognized origin is normalized to manual (never violates the CHECK).
	other, err := d.CreateJob(ctx, "scan", &ps, "bogus")
	if err != nil {
		t.Fatalf("create job with bad origin: %v", err)
	}

	j, err := d.GetJob(ctx, auto)
	if err != nil || j.Origin != db.JobOriginAuto {
		t.Fatalf("auto job origin = %q (err=%v)", j.Origin, err)
	}
	if j, _ := d.GetJob(ctx, other); j.Origin != db.JobOriginManual {
		t.Fatalf("bad origin should normalize to manual, got %q", j.Origin)
	}

	// All three start queued.
	ids, err := d.ListQueuedJobIDs(ctx)
	if err != nil {
		t.Fatalf("list queued: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 queued, got %d", len(ids))
	}

	// Running and terminal jobs drop out of the queue list.
	if err := d.MarkJobRunning(ctx, auto); err != nil {
		t.Fatal(err)
	}
	if err := d.MarkJobDone(ctx, manual, "cancelled"); err != nil {
		t.Fatal(err)
	}
	ids, _ = d.ListQueuedJobIDs(ctx)
	if len(ids) != 1 || ids[0] != other {
		t.Fatalf("expected only job %d queued, got %v", other, ids)
	}
}
