package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"

	"github.com/danbrown95/archivarr/internal/db"
)

func (s *server) getSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.db.GetAppSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *server) putSettings(w http.ResponseWriter, r *http.Request) {
	var cfg db.AppSettings
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if cfg.ScanIntervalMinutes < 0 {
		cfg.ScanIntervalMinutes = 0
	}
	if cfg.ScanExclude == nil {
		cfg.ScanExclude = []string{}
	}
	if cfg.ScanIncludeExt == nil {
		cfg.ScanIncludeExt = []string{}
	}

	// get existing settings to match against for changes that require a scan, then save the new settings to the DB
	existing, err := s.db.GetAppSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.db.SaveAppSettings(r.Context(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !slices.Equal(existing.ScanExclude, cfg.ScanExclude) || !slices.Equal(existing.ScanIncludeExt, cfg.ScanIncludeExt) {
		// Filters changed: rescan every source so media_items reflects the new
		// include/exclude rules. Manual origin → runs even while automation is paused.
		if _, err := s.jobs.EnqueueSourceScans(r.Context(), cfg.ScanHashOnScan, db.JobOriginManual); err != nil {
			slog.Error("settings: could not enqueue rescan after filter change", "err", err)
		}
	}
	writeJSON(w, http.StatusOK, cfg)
}
