package api

import (
	"encoding/json"
	"net/http"
	"time"
)

type automationDTO struct {
	Paused      bool    `json:"paused"`
	PausedUntil *string `json:"pausedUntil,omitempty"`
}

func (s *server) automationState(w http.ResponseWriter, r *http.Request) {
	paused, until, err := s.db.AutomationPaused(r.Context())
	if err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	writeJSON(w, http.StatusOK, automationDTO{Paused: paused, PausedUntil: unixPtrToRFC3339(until)})
}

type pauseReq struct {
	// Seconds > 0 auto-resumes after that long; 0/omitted pauses indefinitely.
	Seconds int64 `json:"seconds"`
}

func (s *server) pauseAutomation(w http.ResponseWriter, r *http.Request) {
	var req pauseReq
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	var until *int64
	if req.Seconds > 0 {
		u := time.Now().Unix() + req.Seconds
		until = &u
	}
	if err := s.db.PauseAutomation(r.Context(), until); err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	s.automationState(w, r)
}

func (s *server) resumeAutomation(w http.ResponseWriter, r *http.Request) {
	if err := s.db.ResumeAutomation(r.Context()); err != nil {
		s.serverError(w, r, "internal error", err)
		return
	}
	s.automationState(w, r)
}
