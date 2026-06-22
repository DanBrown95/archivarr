package jobs

import (
	"context"
	"log"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
)

// RunScheduler periodically enqueues scan jobs for every source drive, based on
// the configured scan interval. It checks the interval each minute (so settings
// changes take effect promptly) and skips while automation is paused. Runs until
// ctx is cancelled.
func (m *Manager) RunScheduler(ctx context.Context) {
	var lastScan time.Time
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.maybeScheduleScans(ctx, &lastScan)
		}
	}
}

func (m *Manager) maybeScheduleScans(ctx context.Context, lastScan *time.Time) {
	settings, err := m.db.GetAppSettings(ctx)
	if err != nil || settings.ScanIntervalMinutes <= 0 {
		return
	}
	if paused, _, _ := m.db.AutomationPaused(ctx); paused {
		return
	}
	interval := time.Duration(settings.ScanIntervalMinutes) * time.Minute
	if !lastScan.IsZero() && time.Since(*lastScan) < interval {
		return
	}
	*lastScan = time.Now()

	ids, err := m.EnqueueSourceScans(ctx, settings.ScanHashOnScan, db.JobOriginAuto)
	if err != nil {
		log.Printf("scheduler: enqueue scans: %v", err)
		return
	}
	if len(ids) > 0 {
		log.Printf("scheduler: enqueued %d scheduled scan job(s)", len(ids))
	}
}
