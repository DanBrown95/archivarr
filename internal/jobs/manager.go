// Package jobs runs background work (scan/backup/...) from a persistent queue
// via a worker pool, with per-destination-drive serialization, cancellation,
// progress/log persistence, and crash recovery.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/danbrown95/archivarr/internal/backup"
	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/scan"
)

// Job type identifiers.
const (
	TypeScan   = "scan"
	TypeBackup = "backup"
)

// ScanParams is the params_json payload for a scan job.
type ScanParams struct {
	DriveID    int64 `json:"driveId"`
	HashOnScan bool  `json:"hashOnScan"`
}

// BackupParams is the params_json payload for a backup job. When MediaItemIDs is
// empty, all pending files for the source are backed up; otherwise only those.
type BackupParams struct {
	SourceDriveID int64   `json:"sourceDriveId"`
	DestDriveID   int64   `json:"destDriveId"`
	MediaItemIDs  []int64 `json:"mediaItemIds,omitempty"`
}

// Manager owns the queue and worker pool.
type Manager struct {
	db      *db.DB
	scan    *scan.Engine
	backup  *backup.Runner
	queue   chan int64
	workers int

	mu        sync.Mutex
	cancels   map[int64]context.CancelFunc
	destLocks *keyedMutex
	baseCtx   context.Context
}

// NewManager constructs a Manager. Call Start to launch workers.
func NewManager(database *db.DB, scanEngine *scan.Engine, backupRunner *backup.Runner, workers int) *Manager {
	if workers < 1 {
		workers = 1
	}
	return &Manager{
		db:        database,
		scan:      scanEngine,
		backup:    backupRunner,
		queue:     make(chan int64, 256),
		workers:   workers,
		cancels:   make(map[int64]context.CancelFunc),
		destLocks: newKeyedMutex(),
	}
}

// Start recovers interrupted jobs and launches the worker pool. It returns
// immediately; workers run until ctx is cancelled.
func (m *Manager) Start(ctx context.Context) {
	m.baseCtx = ctx
	requeue, err := m.db.RecoverJobs(ctx)
	if err != nil {
		log.Printf("jobs: recovery: %v", err)
	}
	for i := 0; i < m.workers; i++ {
		go m.worker(ctx)
	}
	for _, id := range requeue {
		m.Enqueue(id)
	}
}

// Enqueue schedules a (already-created) job for execution.
func (m *Manager) Enqueue(jobID int64) {
	select {
	case m.queue <- jobID:
	default:
		// Queue momentarily full: don't block the caller.
		go func() { m.queue <- jobID }()
	}
}

// Cancel stops a running job, or marks a queued job cancelled. Returns true if
// a running job was signalled.
func (m *Manager) Cancel(jobID int64) bool {
	m.mu.Lock()
	cancel, running := m.cancels[jobID]
	m.mu.Unlock()
	if running {
		cancel()
		return true
	}
	_ = m.db.MarkJobDone(context.Background(), jobID, "cancelled")
	return false
}

// ClearQueued cancels every job still waiting to run (not yet started),
// returning how many were cleared. Already-running jobs are left alone.
func (m *Manager) ClearQueued(ctx context.Context) (int, error) {
	ids, err := m.db.ListQueuedJobIDs(ctx)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		m.Cancel(id) // marks queued jobs cancelled; signals any held in waitIfPaused
	}
	return len(ids), nil
}

// waitIfPaused blocks while automated work is paused, until resumed or ctx ends.
func (m *Manager) waitIfPaused(ctx context.Context) {
	for {
		paused, _, err := m.db.AutomationPaused(ctx)
		if err != nil || !paused {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *Manager) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-m.queue:
			m.execute(ctx, id)
		}
	}
}

func (m *Manager) execute(parent context.Context, jobID int64) {
	job, err := m.db.GetJob(parent, jobID)
	if err != nil {
		log.Printf("jobs: load %d: %v", jobID, err)
		return
	}
	if job.Status == "cancelled" {
		return // cancelled while still queued
	}

	jobCtx, cancel := context.WithCancel(parent)
	m.mu.Lock()
	m.cancels[jobID] = cancel
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		delete(m.cancels, jobID)
		m.mu.Unlock()
	}()

	// Automated jobs respect the pause (held at 'queued' until resumed);
	// manually-triggered jobs run regardless — the user asked for them.
	if job.Origin == db.JobOriginAuto {
		m.waitIfPaused(jobCtx)
		if jobCtx.Err() != nil {
			_ = m.db.MarkJobDone(parent, jobID, "cancelled")
			return
		}
	}

	_ = m.db.MarkJobRunning(jobCtx, jobID)
	prog := m.reporter(jobID)

	var runErr error
	switch job.Type {
	case TypeScan:
		runErr = m.runScan(jobCtx, job, prog)
	case TypeBackup:
		runErr = m.runBackup(jobCtx, job, prog)
	default:
		runErr = fmt.Errorf("unknown job type %q", job.Type)
	}

	switch {
	case runErr == nil:
		_ = m.db.SetJobProgress(parent, jobID, 1)
		_ = m.db.MarkJobDone(parent, jobID, "done")
	case errors.Is(runErr, context.Canceled):
		_ = m.db.AppendJobLog(parent, jobID, "cancelled")
		_ = m.db.MarkJobDone(parent, jobID, "cancelled")
	default:
		_ = m.db.AppendJobLog(parent, jobID, "error: "+runErr.Error())
		_ = m.db.MarkJobDone(parent, jobID, "failed")
	}
}

// reporter returns a Progress that persists progress (throttled) and log lines.
func (m *Manager) reporter(jobID int64) backup.Progress {
	var mu sync.Mutex
	var last time.Time
	return backup.Progress{
		OnProgress: func(done, total int) {
			if total <= 0 {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			now := time.Now()
			if done < total && now.Sub(last) < 500*time.Millisecond {
				return // throttle mid-run writes
			}
			last = now
			_ = m.db.SetJobProgress(m.baseCtx, jobID, float64(done)/float64(total))
		},
		OnLog: func(msg string) {
			_ = m.db.AppendJobLog(m.baseCtx, jobID, msg)
		},
	}
}

func (m *Manager) runScan(ctx context.Context, job *db.Job, prog backup.Progress) error {
	var p ScanParams
	if err := unmarshalParams(job, &p); err != nil {
		return err
	}
	drive, err := m.db.GetDrive(ctx, p.DriveID)
	if err != nil {
		return err
	}
	settings, _ := m.db.GetAppSettings(ctx)
	prog.OnLog(fmt.Sprintf("scanning %q (hashOnScan=%v)", drive.Label, p.HashOnScan))
	res, err := m.scan.ScanSource(ctx, drive, scan.Options{
		HashOnScan: p.HashOnScan,
		Exclude:    settings.ScanExclude,
		IncludeExt: settings.ScanIncludeExt,
	})
	if err != nil {
		return err
	}
	if b, err := json.Marshal(res); err == nil {
		_ = m.db.SetJobStats(ctx, job.ID, string(b))
	}
	prog.OnLog(fmt.Sprintf("scan done: %d new, %d changed, %d unchanged, %d missing, %d hashed",
		res.New, res.Changed, res.Unchanged, res.Missing, res.Hashed))
	return nil
}

func (m *Manager) runBackup(ctx context.Context, job *db.Job, prog backup.Progress) error {
	var p BackupParams
	if err := unmarshalParams(job, &p); err != nil {
		return err
	}
	source, err := m.db.GetDrive(ctx, p.SourceDriveID)
	if err != nil {
		return fmt.Errorf("source drive: %w", err)
	}
	dest, err := m.db.GetDrive(ctx, p.DestDriveID)
	if err != nil {
		return fmt.Errorf("destination drive: %w", err)
	}

	// One writer per physical destination drive at a time.
	unlock := m.destLocks.Lock(dest.ID)
	defer unlock()

	stats, runErr := m.backup.RunBackup(ctx, source, dest, p.MediaItemIDs, prog)
	if stats != nil {
		if b, err := json.Marshal(stats); err == nil {
			_ = m.db.SetJobStats(ctx, job.ID, string(b))
		}
	}
	return runErr
}

func unmarshalParams(job *db.Job, v any) error {
	if job.Params == nil || *job.Params == "" {
		return fmt.Errorf("job %d has no params", job.ID)
	}
	return json.Unmarshal([]byte(*job.Params), v)
}
