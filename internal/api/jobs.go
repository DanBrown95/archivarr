package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/jobs"
	"github.com/go-chi/chi/v5"
)

type jobDTO struct {
	ID         int64           `json:"id"`
	Type       string          `json:"type"`
	Status     string          `json:"status"`
	Origin     string          `json:"origin"`
	Progress   float64         `json:"progress"`
	Params     json.RawMessage `json:"params,omitempty"`
	Stats      json.RawMessage `json:"stats,omitempty"`
	Log        string          `json:"log,omitempty"`
	CreatedAt  string          `json:"createdAt"`
	StartedAt  *string         `json:"startedAt,omitempty"`
	FinishedAt *string         `json:"finishedAt,omitempty"`
}

func toJobDTO(j db.Job) jobDTO {
	dto := jobDTO{
		ID:         j.ID,
		Type:       j.Type,
		Status:     j.Status,
		Origin:     j.Origin,
		Progress:   j.Progress,
		Log:        j.Log,
		CreatedAt:  time.Unix(j.CreatedAt, 0).UTC().Format(time.RFC3339),
		StartedAt:  unixPtrToRFC3339(j.StartedAt),
		FinishedAt: unixPtrToRFC3339(j.FinishedAt),
	}
	if j.Params != nil {
		dto.Params = json.RawMessage(*j.Params)
	}
	if j.Stats != nil {
		dto.Stats = json.RawMessage(*j.Stats)
	}
	return dto
}

func (s *server) listJobs(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListJobs(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jobDTO, 0, len(list))
	for _, j := range list {
		out = append(out, toJobDTO(j))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	j, err := s.db.GetJob(r.Context(), id)
	if errors.Is(err, db.ErrJobNotFound) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toJobDTO(*j))
}

type createJobReq struct {
	Type          string  `json:"type"`
	DriveID       int64   `json:"driveId"`       // scan
	SourceDriveID int64   `json:"sourceDriveId"` // backup, import
	DestDriveID   int64   `json:"destDriveId"`   // backup, import
	MediaItemIDs  []int64 `json:"mediaItemIds"`  // backup (optional: specific files)
	HashOnScan    bool    `json:"hashOnScan"`    // scan
	Verify        bool    `json:"verify"`        // import (hash-verify matches)
}

func (s *server) createJob(w http.ResponseWriter, r *http.Request) {
	var req createJobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	switch req.Type {
	case jobs.TypeScan:
		s.enqueueScan(w, r, req.DriveID, req.HashOnScan)
	case jobs.TypeBackup:
		s.enqueueBackup(w, r, req.SourceDriveID, req.DestDriveID, req.MediaItemIDs)
	case jobs.TypeImport:
		s.enqueueImport(w, r, req.DestDriveID, req.SourceDriveID, req.Verify)
	default:
		writeError(w, http.StatusBadRequest, "type must be 'scan', 'backup', or 'import'")
	}
}

// clearQueuedJobs cancels every job still waiting to run.
func (s *server) clearQueuedJobs(w http.ResponseWriter, r *http.Request) {
	n, err := s.jobs.ClearQueued(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"cancelled": n})
}

func (s *server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	if _, err := s.db.GetJob(r.Context(), id); errors.Is(err, db.ErrJobNotFound) {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	s.jobs.Cancel(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelling"})
}

// enqueueScan validates a source drive and creates+enqueues a scan job.
func (s *server) enqueueScan(w http.ResponseWriter, r *http.Request, driveID int64, hashOnScan bool) {
	d, err := s.db.GetDrive(r.Context(), driveID)
	if errors.Is(err, db.ErrDriveNotFound) {
		writeError(w, http.StatusNotFound, "drive not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d.Role != db.RoleSource {
		writeError(w, http.StatusBadRequest, "only source drives can be scanned")
		return
	}
	params, _ := json.Marshal(jobs.ScanParams{DriveID: driveID, HashOnScan: hashOnScan})
	s.createAndEnqueue(w, r, jobs.TypeScan, params)
}

// enqueueBackup validates the source/destination and creates+enqueues a backup
// job. itemIDs may be empty (back up everything pending) or specific files.
func (s *server) enqueueBackup(w http.ResponseWriter, r *http.Request, sourceID, destID int64, itemIDs []int64) {
	src, err := s.db.GetDrive(r.Context(), sourceID)
	if errors.Is(err, db.ErrDriveNotFound) {
		writeError(w, http.StatusNotFound, "source drive not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if src.Role != db.RoleSource {
		writeError(w, http.StatusBadRequest, "source drive must have role source")
		return
	}
	dst, err := s.db.GetDrive(r.Context(), destID)
	if errors.Is(err, db.ErrDriveNotFound) {
		writeError(w, http.StatusNotFound, "destination drive not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dst.Role != db.RoleDestination {
		writeError(w, http.StatusBadRequest, "destination drive must have role destination")
		return
	}
	params, _ := json.Marshal(jobs.BackupParams{SourceDriveID: sourceID, DestDriveID: destID, MediaItemIDs: itemIDs})
	s.createAndEnqueue(w, r, jobs.TypeBackup, params)
}

// enqueueImport validates the destination (must be an online destination drive)
// and resolves the source, then creates+enqueues an import job.
func (s *server) enqueueImport(w http.ResponseWriter, r *http.Request, destID, sourceID int64, verify bool) {
	dst, err := s.db.GetDrive(r.Context(), destID)
	if errors.Is(err, db.ErrDriveNotFound) {
		writeError(w, http.StatusNotFound, "destination drive not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dst.Role != db.RoleDestination {
		writeError(w, http.StatusBadRequest, "drive must have role destination")
		return
	}
	if !dst.Online || dst.LastMountPath == nil || *dst.LastMountPath == "" {
		writeError(w, http.StatusBadRequest, "destination drive must be online to import from it")
		return
	}
	src, err := s.resolveImportSource(r.Context(), sourceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	params, _ := json.Marshal(jobs.ImportParams{DestDriveID: destID, SourceDriveID: src, Verify: verify})
	s.createAndEnqueue(w, r, jobs.TypeImport, params)
}

func (s *server) createAndEnqueue(w http.ResponseWriter, r *http.Request, jobType string, params []byte) {
	ps := string(params)
	// Jobs created via the API are user-initiated, so they run even while paused.
	id, err := s.jobs.CreateAndEnqueue(r.Context(), jobType, &ps, db.JobOriginManual)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	j, err := s.db.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, toJobDTO(*j))
}
