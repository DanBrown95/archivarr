package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/danbrown95/archivarr/internal/api"
	"github.com/danbrown95/archivarr/internal/backup"
	"github.com/danbrown95/archivarr/internal/config"
	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/drive"
	"github.com/danbrown95/archivarr/internal/jobs"
	"github.com/danbrown95/archivarr/internal/scan"
	"github.com/danbrown95/archivarr/web"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

// setupLogging configures the global slog logger from config (level + format)
// and writes to stdout, the standard stream for container logs.
func setupLogging(cfg config.Config) {
	var level slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if strings.EqualFold(cfg.LogFormat, "json") {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(h))
}

func printBanner(version string) {
	fmt.Print(ASCII_BANNER)
	fmt.Printf("  offline-first NAS media backup  -  %s\n\n", version)
}

// fatal logs an error and exits non-zero (used for unrecoverable startup errors).
func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}

func main() {
	cfg := config.Load()
	setupLogging(cfg)

	if cfg.LogFormat == "text" {
		printBanner(version)
	}

	assets, err := web.DistFS()
	if err != nil {
		fatal("loading embedded frontend assets", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fatal("opening database", err)
	}
	defer database.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := database.Migrate(ctx); err != nil {
		fatal("running migrations", err)
	}

	if cfg.StartPaused {
		if err := database.PauseAutomation(ctx, nil); err != nil {
			slog.Warn("could not start paused", "err", err)
		} else {
			slog.Info("automation paused at startup (ARCHIVARR_AUTOMATION_PAUSED)")
		}
	}

	scanner := drive.Scanner{Roots: cfg.ScanRoots}
	monitor := &drive.Monitor{DB: database, Scanner: scanner, Interval: cfg.MonitorInterval}
	go monitor.Run(ctx)

	scanEngine := &scan.Engine{DB: database}
	backupRunner := &backup.Runner{
		DB: database,
		DiskFree: func(path string) (uint64, error) {
			u, err := drive.DiskUsage(path)
			return u.FreeBytes, err
		},
	}
	jobManager := jobs.NewManager(database, scanEngine, backupRunner, cfg.Workers)
	jobManager.Start(ctx)
	go jobManager.RunScheduler(ctx)

	handler := api.NewRouter(api.Deps{
		Assets:    assets,
		Version:   version,
		DB:        database,
		Scanner:   scanner,
		Jobs:      jobManager,
		ConfigDir: cfg.ConfigDir,
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("listening",
			"version", version, "port", cfg.Port, "configDir", cfg.ConfigDir, "scanRoots", cfg.ScanRoots)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("http server error", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	cancel() // stop the drive monitor
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}
	slog.Info("stopped")
}
