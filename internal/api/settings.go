package api

import (
	"encoding/json"
	"net/http"

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
	if err := s.db.SaveAppSettings(r.Context(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}
