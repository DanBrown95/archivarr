// Package jobs runs background work (scan/backup/...) from a persistent queue
// via a worker pool, with per-destination-drive serialization, cancellation,
// progress/log persistence, and crash recovery.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/danbrown95/archivarr/internal/backup"
	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/importer"
	"github.com/danbrown95/archivarr/internal/pathfilter"
	"github.com/danbrown95/archivarr/internal/scan"
	"github.com/danbrown95/archivarr/internal/util"
)

// Job type identifiers.
const (
	TypeScan   = "scan"
	TypeBackup = "backup"
	TypeImport = "import"
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

// ImportParams is the params_json payload for an import job: scan an existing
// destination drive and register its files as backups of the given source.
type ImportParams struct {
	DestDriveID   int64 `json:"destDriveId"`
	SourceDriveID int64 `json:"sourceDriveId"`
	Verify        bool  `json:"verify"` // recompute hashes and confirm against the source
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
		slog.Error("job recovery failed", "err", err)
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

// CreateAndEnqueue creates a new job and schedules it for execution,
// returning the new job's ID.
func (m *Manager) CreateAndEnqueue(ctx context.Context, jobType string, params *string, origin string) (int64, error) {
	id, err := m.db.CreateJob(ctx, jobType, params, origin)
	if err != nil {
		return 0, err
	}
	m.Enqueue(id)
	return id, nil
}

// EnqueueSourceScans creates and enqueues a scan job for every source drive,
// returning the created job ids. origin is db.JobOriginManual or db.JobOriginAuto.
func (m *Manager) EnqueueSourceScans(ctx context.Context, hashOnScan bool, origin string) ([]int64, error) {
	drives, err := m.db.ListDrives(ctx)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, d := range drives {
		if d.Role != db.RoleSource {
			continue
		}
		params, _ := json.Marshal(ScanParams{DriveID: d.ID, HashOnScan: hashOnScan})
		ps := string(params)
		id, err := m.CreateAndEnqueue(ctx, TypeScan, &ps, origin)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
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
		slog.Error("could not load job", "job", jobID, "err", err)
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
	started := time.Now()
	slog.Info("job started", "job", jobID, "type", job.Type, "origin", job.Origin)

	var summary string
	var runErr error
	switch job.Type {
	case TypeScan:
		summary, runErr = m.runScan(jobCtx, job, prog)
	case TypeBackup:
		summary, runErr = m.runBackup(jobCtx, job, prog)
	case TypeImport:
		summary, runErr = m.runImport(jobCtx, job, prog)
	default:
		runErr = fmt.Errorf("unknown job type %q", job.Type)
	}

	dur := time.Since(started).Round(time.Millisecond)
	switch {
	case runErr == nil:
		_ = m.db.SetJobProgress(parent, jobID, 1)
		_ = m.db.MarkJobDone(parent, jobID, "done")
		slog.Info("job completed", "job", jobID, "type", job.Type, "summary", summary, "dur", dur.String())
	case errors.Is(runErr, context.Canceled):
		_ = m.db.AppendJobLog(parent, jobID, "cancelled")
		_ = m.db.MarkJobDone(parent, jobID, "cancelled")
		slog.Warn("job cancelled", "job", jobID, "type", job.Type, "dur", dur.String())
	default:
		_ = m.db.AppendJobLog(parent, jobID, "error: "+runErr.Error())
		_ = m.db.MarkJobDone(parent, jobID, "failed")
		slog.Error("job failed", "job", jobID, "type", job.Type, "err", runErr.Error(), "dur", dur.String())
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

// runScan executes a scan job and returns a short human summary for the log.
func (m *Manager) runScan(ctx context.Context, job *db.Job, prog backup.Progress) (string, error) {
	var p ScanParams
	if err := unmarshalParams(job, &p); err != nil {
		return "", err
	}
	drive, err := m.db.GetDrive(ctx, p.DriveID)
	if err != nil {
		return "", err
	}
	settings, _ := m.db.GetAppSettings(ctx)
	prog.OnLog(fmt.Sprintf("scanning %q (hashOnScan=%v)", drive.Label, p.HashOnScan))
	res, err := m.scan.ScanSource(ctx, drive, scan.Options{
		HashOnScan: p.HashOnScan,
		Exclude:    settings.ScanExclude,
		IncludeExt: settings.ScanIncludeExt,
	})
	if err != nil {
		return "", err
	}
	if b, err := json.Marshal(res); err == nil {
		_ = m.db.SetJobStats(ctx, job.ID, string(b))
	}
	summary := fmt.Sprintf("%d new, %d changed, %d unchanged, %d missing, %d hashed",
		res.New, res.Changed, res.Unchanged, res.Missing, res.Hashed)
	prog.OnLog("scan done: " + summary)
	return summary, nil
}

// runBackup executes a backup job and returns a short human summary for the log.
func (m *Manager) runBackup(ctx context.Context, job *db.Job, prog backup.Progress) (string, error) {
	var p BackupParams
	if err := unmarshalParams(job, &p); err != nil {
		return "", err
	}
	source, err := m.db.GetDrive(ctx, p.SourceDriveID)
	if err != nil {
		return "", fmt.Errorf("source drive: %w", err)
	}
	dest, err := m.db.GetDrive(ctx, p.DestDriveID)
	if err != nil {
		return "", fmt.Errorf("destination drive: %w", err)
	}

	// One writer per physical destination drive at a time.
	unlock := m.destLocks.Lock(dest.ID)
	defer unlock()

	// Apply the current include/exclude rules at copy time too, so a backup
	// honors settings changed since the last scan (media_items may be stale).
	settings, _ := m.db.GetAppSettings(ctx)
	skip := pathfilter.Rules{Exclude: settings.ScanExclude, IncludeExt: settings.ScanIncludeExt}.Skip

	stats, runErr := m.backup.RunBackup(ctx, source, dest, p.MediaItemIDs, skip, prog)
	var summary string
	if stats != nil {
		if b, err := json.Marshal(stats); err == nil {
			_ = m.db.SetJobStats(ctx, job.ID, string(b))
		}
		summary = fmt.Sprintf("copied %d, failed %d, %s", stats.Copied, stats.Failed, util.Bytes(stats.Bytes))
		if stats.StoppedFull {
			summary += fmt.Sprintf(", destination full (%d remaining)", stats.Remaining)
		}
	}
	return summary, runErr
}

// runImport scans an existing destination drive and registers files that match
// the source's tracked media items as backups (Mode A — filesystem match).
func (m *Manager) runImport(ctx context.Context, job *db.Job, prog backup.Progress) (string, error) {
	var p ImportParams
	if err := unmarshalParams(job, &p); err != nil {
		return "", err
	}
	dest, err := m.db.GetDrive(ctx, p.DestDriveID)
	if err != nil {
		return "", fmt.Errorf("destination drive: %w", err)
	}
	if dest.LastMountPath == nil || *dest.LastMountPath == "" {
		return "", fmt.Errorf("destination %q is not mounted", dest.Label)
	}
	source, err := m.db.GetDrive(ctx, p.SourceDriveID)
	if err != nil {
		return "", fmt.Errorf("source drive: %w", err)
	}
	if source.RootPath != nil && util.PathsOverlap(*source.RootPath, *dest.LastMountPath) {
		return "", fmt.Errorf("destination %q and source %q share a location — refusing to import the source onto itself", dest.Label, source.Label)
	}

	// One writer per physical destination at a time, same as backups.
	unlock := m.destLocks.Lock(dest.ID)
	defer unlock()

	settings, _ := m.db.GetAppSettings(ctx)
	prog.OnLog(fmt.Sprintf("importing existing backups on %q, matching against source %q (verify=%v)",
		dest.Label, source.Label, p.Verify))

	st, err := importer.ImportDestinationFS(ctx, m.db, importer.FSOptions{
		SourceDriveID: source.ID,
		DestDriveID:   dest.ID,
		DestRoot:      *dest.LastMountPath,
		Verify:        p.Verify,
		Exclude:       settings.ScanExclude,
		IncludeExt:    settings.ScanIncludeExt,
		OnProgress:    prog.OnProgress,
		OnLog:         prog.OnLog,
	})
	if b, jerr := json.Marshal(st); jerr == nil {
		_ = m.db.SetJobStats(ctx, job.ID, string(b))
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d imported, %d already known, %d unmatched, %d size mismatch",
		st.Imported, st.AlreadyKnown, st.Unmatched, st.SizeMismatch), nil
}

func unmarshalParams(job *db.Job, v any) error {
	if job.Params == nil || *job.Params == "" {
		return fmt.Errorf("job %d has no params", job.ID)
	}
	return json.Unmarshal([]byte(*job.Params), v)
}
