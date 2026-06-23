package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
)

type backupRefDTO struct {
	DriveID  int64  `json:"driveId"`
	Label    string `json:"label"`
	CopiedAt string `json:"copiedAt"`
	Status   string `json:"status"`
}

type mediaItemDTO struct {
	ID           int64          `json:"id"`
	RelPath      string         `json:"relPath"`
	Size         int64          `json:"size"`
	Mtime        string         `json:"mtime"`
	SourceDrive  *int64         `json:"sourceDriveId,omitempty"`
	SourceLabel  string         `json:"sourceLabel,omitempty"`
	ContentHash  *string        `json:"contentHash,omitempty"`
	BackedUp     bool           `json:"backedUp"`
	Backups      []backupRefDTO `json:"backups"`
	LastCopiedAt *string        `json:"lastCopiedAt,omitempty"`
}

type mediaListDTO struct {
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
	Items  []mediaItemDTO `json:"items"`
}

// listMedia returns a filtered, paginated page of source media with the
// destinations each file is backed up to.
func (s *server) listMedia(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := db.MediaFilter{
		Status: q.Get("status"),
		Query:  q.Get("q"),
		Limit:  atoiDefault(q.Get("limit"), 100),
		Offset: atoiDefault(q.Get("offset"), 0),
	}
	if v := q.Get("sourceDriveId"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.SourceDriveID = &id
		}
	}

	ctx := r.Context()
	total, err := s.db.CountMedia(ctx, f)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	items, err := s.db.ListMediaPage(ctx, f)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}

	ids := make([]int64, len(items))
	for i, m := range items {
		ids[i] = m.ID
	}
	backups, err := s.db.BackupsForItems(ctx, ids)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}

	labels, err := s.driveLabels(ctx)
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}

	out := mediaListDTO{Total: total, Limit: f.Limit, Offset: f.Offset, Items: make([]mediaItemDTO, 0, len(items))}
	if out.Limit <= 0 {
		out.Limit = 100
	}
	for _, m := range items {
		dto := mediaItemDTO{
			ID:          m.ID,
			RelPath:     m.RelPath,
			Size:        m.Size,
			Mtime:       time.Unix(m.Mtime, 0).UTC().Format(time.RFC3339),
			SourceDrive: m.SourceDriveID,
			ContentHash: m.ContentHash,
			Backups:     []backupRefDTO{},
		}
		if m.SourceDriveID != nil {
			dto.SourceLabel = labels[*m.SourceDriveID]
		}
		var latest int64
		for _, b := range backups[m.ID] {
			dto.Backups = append(dto.Backups, backupRefDTO{
				DriveID:  b.DestDriveID,
				Label:    b.DestLabel,
				CopiedAt: time.Unix(b.CopiedAt, 0).UTC().Format(time.RFC3339),
				Status:   b.Status,
			})
			if b.CopiedAt > latest {
				latest = b.CopiedAt
			}
		}
		dto.BackedUp = len(dto.Backups) > 0
		if latest > 0 {
			s := time.Unix(latest, 0).UTC().Format(time.RFC3339)
			dto.LastCopiedAt = &s
		}
		out.Items = append(out.Items, dto)
	}
	writeJSON(w, http.StatusOK, out)
}

// driveLabels returns a map of drive id -> label.
func (s *server) driveLabels(ctx context.Context) (map[int64]string, error) {
	drives, err := s.db.ListDrives(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[int64]string, len(drives))
	for _, d := range drives {
		m[d.ID] = d.Label
	}
	return m, nil
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
