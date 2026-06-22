package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

// Setting keys.
const (
	settingPaused      = "automation.paused"
	settingPausedUntil = "automation.paused_until"
	settingAppConfig   = "app.config"
)

// AppSettings holds user-configurable behavior, persisted as one JSON blob.
type AppSettings struct {
	// ScanExclude are glob patterns; a file is skipped if a pattern matches its
	// basename or any path segment, case-insensitively (e.g. "*.nfo", "@eaDir",
	// ".DS_Store").
	ScanExclude []string `json:"scanExclude"`
	// ScanIncludeExt, when non-empty, restricts tracking to these extensions
	// (without the dot, e.g. "mkv", "mp4"). Empty means all files.
	ScanIncludeExt []string `json:"scanIncludeExt"`
	// ScanHashOnScan controls whether scheduled scans compute content hashes.
	ScanHashOnScan bool `json:"scanHashOnScan"`
	// ScanIntervalMinutes schedules automatic scans of every source (0 = off).
	ScanIntervalMinutes int `json:"scanIntervalMinutes"`
}

// defaultScanExclude is the starting set of exclude patterns for a fresh install
// (no saved settings yet). It's a convenience baseline, fully editable per
// install via Settings. Matching is case-insensitive (see scan.Options.skip).
var defaultScanExclude = []string{
	// In-progress / temporary download files
	"*.tmp", "*.part", "*.!qb", "*.qb!", "*.crdownload", "*.nzb",
	// OS / NAS junk files
	".DS_Store", "Thumbs.db", "desktop.ini", "*.thumb",
	// Logs
	"*.log",
	// Executables / scripts (not media)
	"*.exe", "*.bat", "*.sh", "*.url",
	// Archives (not media)
	"*.zip", "*.rar", "*.7z",
}

// DefaultAppSettings returns the zero-config defaults.
func DefaultAppSettings() AppSettings {
	return AppSettings{
		ScanExclude:    append([]string(nil), defaultScanExclude...),
		ScanIncludeExt: []string{},
	}
}

// GetAppSettings loads app settings, falling back to defaults.
func (d *DB) GetAppSettings(ctx context.Context) (AppSettings, error) {
	s := DefaultAppSettings()
	v, ok, err := d.GetSetting(ctx, settingAppConfig)
	if err != nil {
		return s, err
	}
	if ok && v != "" {
		if err := json.Unmarshal([]byte(v), &s); err != nil {
			return DefaultAppSettings(), nil
		}
	}
	if s.ScanExclude == nil {
		s.ScanExclude = []string{}
	}
	if s.ScanIncludeExt == nil {
		s.ScanIncludeExt = []string{}
	}
	return s, nil
}

// SaveAppSettings persists app settings.
func (d *DB) SaveAppSettings(ctx context.Context, s AppSettings) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return d.SetSetting(ctx, settingAppConfig, string(b))
}

// GetSetting returns a stored setting value, with ok=false if unset.
func (d *DB) GetSetting(ctx context.Context, key string) (value string, ok bool, err error) {
	row := d.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key)
	err = row.Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetSetting upserts a setting value.
func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := d.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// DeleteSetting removes a setting.
func (d *DB) DeleteSetting(ctx context.Context, key string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	return err
}

// AutomationPaused reports whether automated work (jobs + drive monitor) is
// currently paused. A timed pause that has elapsed auto-resumes. When paused
// for a duration, until is the unix-second expiry.
func (d *DB) AutomationPaused(ctx context.Context) (paused bool, until *int64, err error) {
	v, ok, err := d.GetSetting(ctx, settingPaused)
	if err != nil {
		return false, nil, err
	}
	if !ok || v != "true" {
		return false, nil, nil
	}

	u, ok, err := d.GetSetting(ctx, settingPausedUntil)
	if err != nil {
		return false, nil, err
	}
	if ok && u != "" {
		ts, perr := strconv.ParseInt(u, 10, 64)
		if perr == nil {
			if time.Now().Unix() >= ts {
				_ = d.ResumeAutomation(ctx) // expired
				return false, nil, nil
			}
			return true, &ts, nil
		}
	}
	return true, nil, nil
}

// PauseAutomation pauses automated work. A nil until pauses indefinitely;
// otherwise it auto-resumes at the given unix second.
func (d *DB) PauseAutomation(ctx context.Context, until *int64) error {
	if err := d.SetSetting(ctx, settingPaused, "true"); err != nil {
		return err
	}
	if until != nil {
		return d.SetSetting(ctx, settingPausedUntil, strconv.FormatInt(*until, 10))
	}
	return d.DeleteSetting(ctx, settingPausedUntil)
}

// ResumeAutomation clears any pause.
func (d *DB) ResumeAutomation(ctx context.Context) error {
	if err := d.SetSetting(ctx, settingPaused, "false"); err != nil {
		return err
	}
	return d.DeleteSetting(ctx, settingPausedUntil)
}
