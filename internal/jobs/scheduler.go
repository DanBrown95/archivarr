package jobs

import (
	"context"
	"encoding/json"
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

	drives, err := m.db.ListDrives(ctx)
	if err != nil {
		return
	}
	for _, d := range drives {
		if d.Role != db.RoleSource && d.Role != db.RoleBoth {
			continue
		}
		params, _ := json.Marshal(ScanParams{DriveID: d.ID, HashOnScan: settings.ScanHashOnScan})
		ps := string(params)
		id, err := m.db.CreateJob(ctx, TypeScan, &ps)
		if err != nil {
			continue
		}
		m.Enqueue(id)
		log.Printf("scheduler: enqueued scan job %d for source drive %d (%s)", id, d.ID, d.Label)
	}
}
