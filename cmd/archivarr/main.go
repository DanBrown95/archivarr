package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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

func main() {
	cfg := config.Load()

	assets, err := web.DistFS()
	if err != nil {
		log.Fatalf("loading embedded frontend assets: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := database.Migrate(ctx); err != nil {
		log.Fatalf("running migrations: %v", err)
	}

	if cfg.StartPaused {
		if err := database.PauseAutomation(ctx, nil); err != nil {
			log.Printf("could not start paused: %v", err)
		} else {
			log.Println("automation PAUSED at startup (ARCHIVARR_AUTOMATION_PAUSED)")
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
	}

	go func() {
		log.Printf("Archivarr %s listening on :%d (config dir: %s, scan roots: %v)",
			version, cfg.Port, cfg.ConfigDir, cfg.ScanRoots)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	cancel() // stop the drive monitor
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	log.Println("stopped")
}
