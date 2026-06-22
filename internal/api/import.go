package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/importer"
)

type importReq struct {
	File          string `json:"file"`          // filename relative to the config directory
	SourceDriveID int64  `json:"sourceDriveId"` // optional; defaults to the only source
	DryRun        bool   `json:"dryRun"`
}

// importLegacy ingests the old backup script's pipe-delimited tracking file
// (located in the config directory) into the database. Runs synchronously and
// returns a summary; it's a one-off, low-volume operation.
func (s *server) importLegacy(w http.ResponseWriter, r *http.Request) {
	var req importReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.File = strings.TrimSpace(req.File)
	if req.File == "" {
		writeError(w, http.StatusBadRequest, "file is required (a filename in your config directory)")
		return
	}

	// Resolve the path strictly inside the config directory — no traversal out.
	full := filepath.Join(s.configDir, req.File)
	rel, err := filepath.Rel(s.configDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "file must be inside the config directory")
		return
	}
	if fi, err := os.Stat(full); err != nil || fi.IsDir() {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("file not found in config directory: %q", req.File))
		return
	}

	src, err := s.resolveImportSource(r.Context(), req.SourceDriveID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	st, err := importer.Import(r.Context(), s.db, importer.Options{
		FilePath:      full,
		SourceDriveID: src,
		DryRun:        req.DryRun,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sourceDriveId": src,
		"dryRun":        req.DryRun,
		"stats":         st,
	})
}

// resolveImportSource picks the source drive imported rows attach to: the given
// id if it's a valid source, otherwise the sole source drive.
func (s *server) resolveImportSource(ctx context.Context, want int64) (int64, error) {
	if want != 0 {
		d, err := s.db.GetDrive(ctx, want)
		if err != nil {
			return 0, fmt.Errorf("source drive #%d not found", want)
		}
		if d.Role != db.RoleSource && d.Role != db.RoleBoth {
			return 0, fmt.Errorf("drive %q is not a source", d.Label)
		}
		return d.ID, nil
	}

	drives, err := s.db.ListDrives(ctx)
	if err != nil {
		return 0, err
	}
	var sources []db.Drive
	for _, d := range drives {
		if d.Role == db.RoleSource || d.Role == db.RoleBoth {
			sources = append(sources, d)
		}
	}
	switch len(sources) {
	case 0:
		return 0, errors.New("no source drive exists — add and scan one first")
	case 1:
		return sources[0].ID, nil
	default:
		return 0, errors.New("multiple source drives exist — choose which one to import into")
	}
}
