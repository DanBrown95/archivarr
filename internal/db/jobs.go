package db

import (
	"context"
	"database/sql"
	"errors"
)

// Job is a unit of background work.
type Job struct {
	ID         int64
	Type       string
	Status     string
	Params     *string // JSON
	Progress   float64
	Stats      *string // JSON
	Log        string
	CreatedAt  int64
	StartedAt  *int64
	FinishedAt *int64
}

// ErrJobNotFound is returned when a job lookup matches no row.
var ErrJobNotFound = errors.New("job not found")

const jobColumns = `id, type, status, params_json, progress, stats_json, log,
	created_at, started_at, finished_at`

func scanJob(s scannable) (Job, error) {
	var (
		j        Job
		params   sql.NullString
		stats    sql.NullString
		logStr   sql.NullString
		started  sql.NullInt64
		finished sql.NullInt64
	)
	if err := s.Scan(&j.ID, &j.Type, &j.Status, &params, &j.Progress, &stats, &logStr,
		&j.CreatedAt, &started, &finished); err != nil {
		return Job{}, err
	}
	j.Params = nullStr(params)
	j.Stats = nullStr(stats)
	j.Log = logStr.String
	j.StartedAt = nullInt(started)
	j.FinishedAt = nullInt(finished)
	return j, nil
}

// CreateJob inserts a queued job and returns its id.
func (d *DB) CreateJob(ctx context.Context, jobType string, params *string) (int64, error) {
	res, err := d.ExecContext(ctx,
		`INSERT INTO jobs (type, status, params_json, progress) VALUES (?, 'queued', ?, 0)`,
		jobType, params)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetJob returns a job by id, or ErrJobNotFound.
func (d *DB) GetJob(ctx context.Context, id int64) (*Job, error) {
	row := d.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// ListJobs returns recent jobs, newest first.
func (d *DB) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.QueryContext(ctx,
		`SELECT `+jobColumns+` FROM jobs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// MarkJobRunning sets a job running and stamps started_at.
func (d *DB) MarkJobRunning(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx,
		`UPDATE jobs SET status = 'running', started_at = strftime('%s','now') WHERE id = ?`, id)
	return err
}

// MarkJobDone sets a terminal status (done/failed/cancelled) and stamps finished_at.
func (d *DB) MarkJobDone(ctx context.Context, id int64, status string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE jobs SET status = ?, finished_at = strftime('%s','now') WHERE id = ?`, status, id)
	return err
}

// SetJobProgress updates the 0..1 progress fraction.
func (d *DB) SetJobProgress(ctx context.Context, id int64, progress float64) error {
	_, err := d.ExecContext(ctx, `UPDATE jobs SET progress = ? WHERE id = ?`, progress, id)
	return err
}

// SetJobStats stores the job's stats JSON blob.
func (d *DB) SetJobStats(ctx context.Context, id int64, statsJSON string) error {
	_, err := d.ExecContext(ctx, `UPDATE jobs SET stats_json = ? WHERE id = ?`, statsJSON, id)
	return err
}

// AppendJobLog appends a line (a newline is added) to the job log.
func (d *DB) AppendJobLog(ctx context.Context, id int64, line string) error {
	_, err := d.ExecContext(ctx,
		`UPDATE jobs SET log = COALESCE(log, '') || ? WHERE id = ?`, line+"\n", id)
	return err
}

// RecoverJobs is called at startup: any job left 'running' by a previous process
// is marked failed, and the ids of still-'queued' jobs are returned for re-enqueue.
func (d *DB) RecoverJobs(ctx context.Context) ([]int64, error) {
	if _, err := d.ExecContext(ctx,
		`UPDATE jobs SET status = 'failed', finished_at = strftime('%s','now') WHERE status = 'running'`); err != nil {
		return nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT id FROM jobs WHERE status = 'queued' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
