package api

import (
	"net/http"

	"github.com/danbrown95/archivarr/internal/db"
)

type sourceStatDTO struct {
	DriveID      int64  `json:"driveId"`
	Label        string `json:"label"`
	Online       bool   `json:"online"`
	Files        int    `json:"files"`
	Bytes        int64  `json:"bytes"`
	BackedFiles  int    `json:"backedFiles"`
	PendingFiles int    `json:"pendingFiles"`
	PendingBytes int64  `json:"pendingBytes"`
}

type destStatDTO struct {
	DriveID       int64  `json:"driveId"`
	Label         string `json:"label"`
	Online        bool   `json:"online"`
	Files         int    `json:"files"`
	Bytes         int64  `json:"bytes"`
	CapacityBytes *int64 `json:"capacityBytes,omitempty"`
	FreeBytes     *int64 `json:"freeBytes,omitempty"`
}

type statsDTO struct {
	Totals       db.Totals       `json:"totals"`
	Sources      []sourceStatDTO `json:"sources"`
	Destinations []destStatDTO   `json:"destinations"`
	LastScanAt   *string         `json:"lastScanAt,omitempty"`
}

// stats returns library-wide coverage plus per-source and per-destination rollups.
func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	totals, err := s.db.MediaTotals(ctx)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	sourceStats, err := s.db.PerSourceStats(ctx)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	destStats, err := s.db.PerDestinationStats(ctx)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	drives, err := s.db.ListDrives(ctx)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	lastScanAt, err := s.db.LastScanAt(ctx)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}

	sourceByID := make(map[int64]db.SourceStat, len(sourceStats))
	for _, ss := range sourceStats {
		sourceByID[ss.DriveID] = ss
	}

	out := statsDTO{
		Totals:       totals,
		Sources:      []sourceStatDTO{},
		Destinations: []destStatDTO{},
		LastScanAt:   unixPtrToRFC3339(lastScanAt),
	}
	for _, d := range drives {
		if d.Role == db.RoleSource {
			ss := sourceByID[d.ID]
			out.Sources = append(out.Sources, sourceStatDTO{
				DriveID:      d.ID,
				Label:        d.Label,
				Online:       d.Online,
				Files:        ss.Files,
				Bytes:        ss.Bytes,
				BackedFiles:  ss.BackedFiles,
				PendingFiles: ss.PendingFiles,
				PendingBytes: ss.PendingBytes,
			})
		}
		if d.Role == db.RoleDestination {
			ds := destStats[d.ID]
			out.Destinations = append(out.Destinations, destStatDTO{
				DriveID:       d.ID,
				Label:         d.Label,
				Online:        d.Online,
				Files:         ds.Files,
				Bytes:         ds.Bytes,
				CapacityBytes: d.CapacityBytes,
				FreeBytes:     d.FreeBytes,
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
