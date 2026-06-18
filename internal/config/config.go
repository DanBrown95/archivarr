// Package config loads runtime configuration from environment variables.
//
// Defaults match the Docker deployment (a /config volume); override with
// ARCHIVARR_* env vars for local development.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultPort            = 7979
	DefaultConfigDir       = "/config"
	DBFileName             = "archivarr.db"
	DefaultMonitorInterval = 30 * time.Second
	DefaultWorkers         = 4
)

// DefaultScanRoots are searched for mounted destination drives.
var DefaultScanRoots = []string{"/mnt"}

// Config holds resolved runtime settings.
type Config struct {
	Port            int           // HTTP listen port
	ConfigDir       string        // directory holding the sqlite db, logs, settings
	DBPath          string        // full path to the sqlite database file
	ScanRoots       []string      // directories scanned for mounted drives
	MonitorInterval time.Duration // how often the drive monitor reconciles state
	Workers         int           // job worker pool size
	StartPaused     bool          // begin with automated work paused (handy for dev/testing)
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	configDir := getenv("ARCHIVARR_CONFIG_DIR", DefaultConfigDir)
	return Config{
		Port:            getenvInt("ARCHIVARR_PORT", DefaultPort),
		ConfigDir:       configDir,
		DBPath:          filepath.Join(configDir, DBFileName),
		ScanRoots:       getenvList("ARCHIVARR_SCAN_ROOTS", DefaultScanRoots),
		MonitorInterval: time.Duration(getenvInt("ARCHIVARR_MONITOR_INTERVAL", int(DefaultMonitorInterval.Seconds()))) * time.Second,
		Workers:         getenvInt("ARCHIVARR_WORKERS", DefaultWorkers),
		StartPaused:     getenvBool("ARCHIVARR_AUTOMATION_PAUSED", false),
	}
}

func getenvBool(key string, fallback bool) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

// getenvList splits a comma-separated env var into a trimmed, non-empty slice.
func getenvList(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
