package jobs

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danbrown95/archivarr/internal/backup"
	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/scan"
)

func newTestManager(t *testing.T) (*Manager, *db.DB, context.Context) {
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
	m := NewManager(d, &scan.Engine{DB: d}, &backup.Runner{DB: d}, 1)
	return m, d, ctx
}

// Cancelling a job that hasn't started yet marks it cancelled (so a worker skips
// it when dequeued) and reports it was not running. Workers are not started, so
// the job stays queued until we cancel it.
func TestCancelQueuedJob(t *testing.T) {
	m, d, ctx := newTestManager(t)
	ps := `{"driveId":1}`
	id, err := d.CreateJob(ctx, TypeScan, &ps, db.JobOriginManual)
	if err != nil {
		t.Fatal(err)
	}

	if running := m.Cancel(id); running {
		t.Fatal("Cancel reported a queued job as running")
	}
	j, err := d.GetJob(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != "cancelled" {
		t.Fatalf("job status = %q, want cancelled", j.Status)
	}
}
