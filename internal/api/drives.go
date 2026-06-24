package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/drive"
	"github.com/danbrown95/archivarr/internal/util"
	"github.com/go-chi/chi/v5"
)

// driveDTO is the JSON shape returned for a drive.
type driveDTO struct {
	ID            int64   `json:"id"`
	Label         string  `json:"label"`
	Role          string  `json:"role"`
	MarkerID      *string `json:"markerId,omitempty"`
	RootPath      *string `json:"rootPath,omitempty"`
	FSUUID        *string `json:"fsUuid,omitempty"`
	LastMountPath *string `json:"lastMountPath,omitempty"`
	CapacityBytes *int64  `json:"capacityBytes,omitempty"`
	FreeBytes     *int64  `json:"freeBytes,omitempty"`
	Online        bool    `json:"online"`
	LastSeenAt    *string `json:"lastSeenAt,omitempty"`
	Notes         *string `json:"notes,omitempty"`
	CreatedAt     string  `json:"createdAt"`
}

func toDriveDTO(d db.Drive) driveDTO {
	return driveDTO{
		ID:            d.ID,
		Label:         d.Label,
		Role:          string(d.Role),
		MarkerID:      d.MarkerID,
		RootPath:      d.RootPath,
		FSUUID:        d.FSUUID,
		LastMountPath: d.LastMountPath,
		CapacityBytes: d.CapacityBytes,
		FreeBytes:     d.FreeBytes,
		Online:        d.Online,
		LastSeenAt:    unixPtrToRFC3339(d.LastSeenAt),
		Notes:         d.Notes,
		CreatedAt:     time.Unix(d.CreatedAt, 0).UTC().Format(time.RFC3339),
	}
}

func (s *server) listDrives(w http.ResponseWriter, r *http.Request) {
	drives, err := s.db.ListDrives(r.Context())
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	out := make([]driveDTO, 0, len(drives))
	for _, d := range drives {
		out = append(out, toDriveDTO(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getDrive(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid drive id")
		return
	}
	d, err := s.db.GetDrive(r.Context(), id)
	if errors.Is(err, db.ErrDriveNotFound) {
		writeError(w, http.StatusNotFound, "drive not found")
		return
	}
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	writeJSON(w, http.StatusOK, toDriveDTO(*d))
}

// overlappingDrive returns an existing drive of role `other` whose tracked path
// overlaps p, or nil if none. It stops a source and a backup destination from
// sharing a location (a "backup" on the source gives no protection).
func (s *server) overlappingDrive(ctx context.Context, p string, other db.Role) (*db.Drive, error) {
	drives, err := s.db.ListDrives(ctx)
	if err != nil {
		return nil, err
	}
	for i := range drives {
		d := drives[i]
		if d.Role != other {
			continue
		}
		var dp string
		if other == db.RoleSource {
			if d.RootPath != nil {
				dp = *d.RootPath
			}
		} else if d.LastMountPath != nil {
			dp = *d.LastMountPath
		}
		if util.PathsOverlap(util.ResolveSymlinks(p), util.ResolveSymlinks(dp)) {
			return &d, nil
		}
	}
	return nil, nil
}

type createDriveReq struct {
	Label    string  `json:"label"`
	Role     string  `json:"role"`
	RootPath *string `json:"rootPath"`
	Notes    *string `json:"notes"`
}

// createDrive registers a drive manually (primarily sources, identified by path).
func (s *server) createDrive(w http.ResponseWriter, r *http.Request) {
	var req createDriveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}
	role := db.Role(req.Role)
	if !db.ValidRole(role) {
		writeError(w, http.StatusBadRequest, "role must be source or destination")
		return
	}
	if role != db.RoleDestination && (req.RootPath == nil || *req.RootPath == "") {
		writeError(w, http.StatusBadRequest, "rootPath is required for source drives")
		return
	}

	// A source must not share a location with a backup destination.
	if role == db.RoleSource && req.RootPath != nil {
		if c, err := s.overlappingDrive(r.Context(), *req.RootPath, db.RoleDestination); err != nil {
			s.serverError(w, r, "internal error", err)
			return
		} else if c != nil {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("path overlaps destination %q — a source and a backup destination can't share a location", c.Label))
			return
		}
	}

	d, err := s.db.CreateDrive(r.Context(), db.CreateDriveInput{
		Label:    req.Label,
		Role:     role,
		RootPath: req.RootPath,
		Notes:    req.Notes,
	})
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	writeJSON(w, http.StatusCreated, toDriveDTO(*d))
}

// discoveredDTO describes a mount point found by scanning the configured roots.
type discoveredDTO struct {
	Path          string  `json:"path"`
	HasMarker     bool    `json:"hasMarker"`
	MarkerID      *string `json:"markerId,omitempty"`
	Known         bool    `json:"known"`
	DriveID       *int64  `json:"driveId,omitempty"`
	CapacityBytes uint64  `json:"capacityBytes"`
	FreeBytes     uint64  `json:"freeBytes"`
}

// discoverDrives scans the configured roots and reports what is currently
// mounted, flagging which mounts are already registered.
func (s *server) discoverDrives(w http.ResponseWriter, r *http.Request) {
	found, err := s.scanner.Scan()
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	out := make([]discoveredDTO, 0, len(found))
	for _, f := range found {
		dto := discoveredDTO{
			Path:          f.Path,
			HasMarker:     f.HasMarker,
			CapacityBytes: f.Usage.CapacityBytes,
			FreeBytes:     f.Usage.FreeBytes,
		}
		if f.HasMarker {
			marker := f.MarkerID
			dto.MarkerID = &marker
			if d, err := s.db.GetDriveByMarker(r.Context(), f.MarkerID); err == nil {
				dto.Known = true
				dto.DriveID = &d.ID
			}
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
}

type registerReq struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Role  string `json:"role"`
}

// registerDrive writes a marker file to a mounted destination and records it as
// a drive, so it is recognized on every future plug-in regardless of mount path.
func (s *server) registerDrive(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Path == "" || req.Label == "" {
		writeError(w, http.StatusBadRequest, "path and label are required")
		return
	}
	role := db.RoleDestination
	if req.Role != "" {
		role = db.Role(req.Role)
		if !db.ValidRole(role) {
			writeError(w, http.StatusBadRequest, "role must be source or destination")
			return
		}
	}

	fi, err := os.Stat(req.Path)
	if err != nil || !fi.IsDir() {
		writeError(w, http.StatusBadRequest, "path is not an accessible directory")
		return
	}

	// A backup destination must not share a location with a source.
	if c, err := s.overlappingDrive(r.Context(), req.Path, db.RoleSource); err != nil {
		s.serverError(w, r, "internal error", err)
		return
	} else if c != nil {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("path overlaps source %q — a backup destination can't share a location with a source", c.Label))
		return
	}

	markerID, err := drive.EnsureMarker(req.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "writing marker file: "+err.Error())
		return
	}
	if existing, err := s.db.GetDriveByMarker(r.Context(), markerID); err == nil {
		writeError(w, http.StatusConflict, "drive already registered as #"+strconv.FormatInt(existing.ID, 10))
		return
	}

	d, err := s.db.CreateDrive(r.Context(), db.CreateDriveInput{
		Label:    req.Label,
		Role:     role,
		MarkerID: &markerID,
	})
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}

	// Record presence now so the UI reflects it without waiting for the monitor.
	usage, _ := drive.DiskUsage(req.Path)
	_ = s.db.UpdateDrivePresence(r.Context(), d.ID, true, req.Path, int64(usage.CapacityBytes), int64(usage.FreeBytes))

	fresh, err := s.db.GetDrive(r.Context(), d.ID)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	writeJSON(w, http.StatusCreated, toDriveDTO(*fresh))
}

// scanDrive enqueues a background scan job for a source drive. Pass ?hash=true
// to compute content hashes inline during the scan.
func (s *server) scanDrive(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid drive id")
		return
	}
	hashOnScan := r.URL.Query().Get("hash") == "true" || r.URL.Query().Get("hash") == "1"
	s.enqueueScan(w, r, id, hashOnScan)
}

// resolveImportSource picks the source drive an import attaches to: the given id
// if it is a valid source, otherwise the sole source drive.
func (s *server) resolveImportSource(ctx context.Context, want int64) (int64, error) {
	if want != 0 {
		d, err := s.db.GetDrive(ctx, want)
		if err != nil {
			return 0, fmt.Errorf("source drive #%d not found", want)
		}
		if d.Role != db.RoleSource {
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
		if d.Role == db.RoleSource {
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

func unixPtrToRFC3339(p *int64) *string {
	if p == nil {
		return nil
	}
	s := time.Unix(*p, 0).UTC().Format(time.RFC3339)
	return &s
}
