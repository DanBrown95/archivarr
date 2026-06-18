package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/go-chi/chi/v5"
)

type driveBriefDTO struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	Role  string `json:"role"`
}

type lostItemDTO struct {
	RelPath string `json:"relPath"`
	Size    int64  `json:"size"`
}

type destBreakdownDTO struct {
	DriveID int64  `json:"driveId"`
	Label   string `json:"label"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
}

type sourceRecoveryDTO struct {
	Drive            driveBriefDTO      `json:"drive"`
	TotalTracked     int                `json:"totalTracked"`
	RecoverableFiles int                `json:"recoverableFiles"`
	RecoverableBytes int64              `json:"recoverableBytes"`
	LostFiles        int                `json:"lostFiles"`
	LostBytes        int64              `json:"lostBytes"`
	PerDestination   []destBreakdownDTO `json:"perDestination"`
	Lost             []lostItemDTO      `json:"lost"`
}

// sourceRecovery reports, for a (presumed dead) source drive, every tracked file
// and where its backups live — plus which files have no backup at all.
func (s *server) sourceRecovery(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid drive id")
		return
	}
	ctx := r.Context()
	d, err := s.db.GetDrive(ctx, id)
	if errors.Is(err, db.ErrDriveNotFound) {
		writeError(w, http.StatusNotFound, "drive not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items, err := s.db.ListSourceItems(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ids := make([]int64, len(items))
	for i, m := range items {
		ids[i] = m.ID
	}
	backups, err := s.db.BackupsForItems(ctx, ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := sourceRecoveryDTO{
		Drive:          driveBriefDTO{ID: d.ID, Label: d.Label, Role: string(d.Role)},
		PerDestination: []destBreakdownDTO{},
		Lost:           []lostItemDTO{},
	}
	perDest := map[int64]*destBreakdownDTO{}
	for _, m := range items {
		out.TotalTracked++
		bks := backups[m.ID]
		if len(bks) > 0 {
			out.RecoverableFiles++
			out.RecoverableBytes += m.Size
			for _, b := range bks {
				pd := perDest[b.DestDriveID]
				if pd == nil {
					pd = &destBreakdownDTO{DriveID: b.DestDriveID, Label: b.DestLabel}
					perDest[b.DestDriveID] = pd
				}
				pd.Files++
				pd.Bytes += m.Size
			}
		} else if m.Present {
			out.LostFiles++
			out.LostBytes += m.Size
			out.Lost = append(out.Lost, lostItemDTO{RelPath: m.RelPath, Size: m.Size})
		}
	}
	for _, pd := range perDest {
		out.PerDestination = append(out.PerDestination, *pd)
	}
	sort.Slice(out.PerDestination, func(i, j int) bool {
		return out.PerDestination[i].Files > out.PerDestination[j].Files
	})

	writeJSON(w, http.StatusOK, out)
}

type destRecoveryDTO struct {
	Removed  int64  `json:"removed"`
	BySource []struct {
		SourceDriveID *int64 `json:"sourceDriveId"`
		SourceLabel   string `json:"sourceLabel"`
		Files         int    `json:"files"`
		Bytes         int64  `json:"bytes"`
	} `json:"bySource"`
}

// destinationRequeue removes a dead destination's backup records so its files
// become pending again (to be copied to a replacement drive).
func (s *server) destinationRequeue(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid drive id")
		return
	}
	ctx := r.Context()
	if _, err := s.db.GetDrive(ctx, id); errors.Is(err, db.ErrDriveNotFound) {
		writeError(w, http.StatusNotFound, "drive not found")
		return
	}

	breakdown, err := s.db.DestContentsBySource(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	labels, err := s.driveLabels(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	removed, err := s.db.DeleteBackupsForDest(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := destRecoveryDTO{Removed: removed}
	for _, b := range breakdown {
		label := "(unknown source)"
		if b.SourceDriveID != nil {
			if l, ok := labels[*b.SourceDriveID]; ok {
				label = l
			}
		}
		out.BySource = append(out.BySource, struct {
			SourceDriveID *int64 `json:"sourceDriveId"`
			SourceLabel   string `json:"sourceLabel"`
			Files         int    `json:"files"`
			Bytes         int64  `json:"bytes"`
		}{SourceDriveID: b.SourceDriveID, SourceLabel: label, Files: b.Files, Bytes: b.Bytes})
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteDrive fully removes a drive and its associated data.
func (s *server) deleteDrive(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid drive id")
		return
	}
	ctx := r.Context()
	if _, err := s.db.GetDrive(ctx, id); errors.Is(err, db.ErrDriveNotFound) {
		writeError(w, http.StatusNotFound, "drive not found")
		return
	}
	if err := s.db.DeleteDrive(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
