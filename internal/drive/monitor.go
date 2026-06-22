package drive

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
)

// Monitor periodically reconciles each drive's online state into the database:
// destinations are matched by marker id against the scanner's findings; sources
// are matched by whether their configured root path is a present directory.
type Monitor struct {
	DB       *db.DB
	Scanner  Scanner
	Interval time.Duration
}

// Run refreshes immediately, then on each interval tick until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	m.Refresh(ctx)
	t := time.NewTicker(m.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Refresh(ctx)
		}
	}
}

// Refresh performs a single reconciliation pass. It runs even while automation
// is paused: drive online/free-space status is read-only and keeping it current
// means the UI stays accurate and manual backups (which bypass pause) work.
func (m *Monitor) Refresh(ctx context.Context) {
	found, _ := m.Scanner.Scan()
	byMarker := make(map[string]Found, len(found))
	for _, f := range found {
		if f.HasMarker {
			byMarker[f.MarkerID] = f
		}
	}

	drives, err := m.DB.ListDrives(ctx)
	if err != nil {
		slog.Error("drive monitor: list drives failed", "err", err)
		return
	}

	for _, d := range drives {
		online := false
		var mount string
		var usage Usage

		switch {
		case d.MarkerID != nil:
			if f, ok := byMarker[*d.MarkerID]; ok {
				online, mount, usage = true, f.Path, f.Usage
			}
		case d.RootPath != nil:
			if fi, err := os.Stat(*d.RootPath); err == nil && fi.IsDir() {
				online, mount = true, *d.RootPath
				usage, _ = DiskUsage(*d.RootPath)
			}
		}

		if err := m.DB.UpdateDrivePresence(ctx, d.ID, online, mount,
			int64(usage.CapacityBytes), int64(usage.FreeBytes)); err != nil {
			slog.Error("drive monitor: update drive failed", "drive", d.ID, "err", err)
		}
	}
}
