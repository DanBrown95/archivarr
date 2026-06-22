// Package api wires the HTTP layer: the REST API under /api and the embedded
// single-page frontend served from everything else.
package api

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/drive"
	"github.com/danbrown95/archivarr/internal/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Deps are the dependencies the router needs.
type Deps struct {
	Assets    fs.FS
	Version   string
	DB        *db.DB
	Scanner   drive.Scanner
	Jobs      *jobs.Manager
	ConfigDir string
}

// server holds shared handler state.
type server struct {
	db           *db.DB
	scanner      drive.Scanner
	jobs         *jobs.Manager
	version      string
	configDir    string
	loginLimiter *loginLimiter
}

// NewRouter builds the top-level HTTP handler.
func NewRouter(d Deps) http.Handler {
	s := &server{
		db:           d.DB,
		scanner:      d.Scanner,
		jobs:         d.Jobs,
		version:      d.Version,
		configDir:    d.ConfigDir,
		loginLimiter: newLoginLimiter(),
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	// r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		// Public endpoints: health plus the auth bootstrap (status/setup/login).
		r.Get("/health", s.health)
		r.Route("/auth", func(r chi.Router) {
			r.Get("/status", s.authStatus)
			r.Post("/setup", s.setup)
			r.Post("/login", s.login)
			// Logout, account changes, and API-key management require an existing
			// browser session — the API key intentionally can't perform these.
			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Post("/logout", s.logout)
				r.Put("/account", s.updateAccount)
				r.Get("/apikey", s.getAPIKey)
				r.Post("/apikey/regenerate", s.regenerateAPIKey)
			})
		})

		// The data API accepts either a session or an API key.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuthOrKey)

			r.Get("/stats", s.stats)
			r.Get("/media", s.listMedia)

			r.Route("/drives", func(r chi.Router) {
				r.Get("/", s.listDrives)
				r.Post("/", s.createDrive)
				r.Get("/discovered", s.discoverDrives)
				r.Post("/register", s.registerDrive)
				r.Get("/{id}", s.getDrive)
				r.Delete("/{id}", s.deleteDrive)
				r.Post("/{id}/scan", s.scanDrive)
			})

			r.Route("/recovery", func(r chi.Router) {
				r.Get("/source/{id}", s.sourceRecovery)
				r.Post("/destination/{id}", s.destinationRequeue)
			})

			r.Route("/jobs", func(r chi.Router) {
				r.Get("/", s.listJobs)
				r.Post("/", s.createJob)
				r.Post("/clear-queued", s.clearQueuedJobs)
				r.Get("/{id}", s.getJob)
				r.Delete("/{id}", s.cancelJob)
			})

			r.Route("/automation", func(r chi.Router) {
				r.Get("/", s.automationState)
				r.Post("/pause", s.pauseAutomation)
				r.Post("/resume", s.resumeAutomation)
			})

			r.Route("/settings", func(r chi.Router) {
				r.Get("/", s.getSettings)
				r.Put("/", s.putSettings)
			})

			r.Post("/import", s.importLegacy)
		})
	})

	// Everything else falls through to the SPA.
	r.Handle("/*", spaHandler(d.Assets))

	return r
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"app":     "archivarr",
		"version": s.version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// spaHandler serves embedded static assets, falling back to index.html for any
// path that doesn't resolve to a file (so client-side routing works).
func spaHandler(assets fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(assets))
	return func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := assets.Open(p); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, assets)
	}
}

func serveIndex(w http.ResponseWriter, _ *http.Request, assets fs.FS) {
	f, err := assets.Open("index.html")
	if err != nil {
		http.Error(w, "frontend not built", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
