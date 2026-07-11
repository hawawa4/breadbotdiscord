// Package httpserver is a thin read-only HTTP API over the shared DB layer,
// running alongside the bot. It exposes stats endpoints and a liveness check
// so an admin panel can be built on top later. No write/admin actions.
package httpserver

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/hawawa4/breadbotdiscord/internal/db"
)

// BotStatus reports whether the Discord session is connected (for /healthz).
type BotStatus interface {
	Ready() bool
}

// Server holds the dependencies for the read-only API.
type Server struct {
	db            *db.DB
	bot           BotStatus
	token         string // shared-secret auth; empty disables auth
	basePath      string // URL mount prefix (e.g. "/breadbot"); empty = root
	downloadsPath string // root under which predictions/ and plots/ live
	debug         bool   // when true, enable permissive CORS for local dev
	http          *http.Server
}

// New builds a Server. token is the optional ADMIN_API_TOKEN (empty = auth
// off). basePath is the URL prefix the server is mounted under behind a reverse
// proxy (empty = root); it must already be normalized (leading slash, no
// trailing slash) as config.Load does. downloadsPath is where the bot writes
// prediction/plot images, served read-only under /api/images. debug enables
// permissive CORS so the Vite dev server can call the API cross-origin; it is
// never on in production (where the SPA is same-origin embedded).
func New(addr string, database *db.DB, bot BotStatus, token, basePath, downloadsPath string, debug bool) *Server {
	s := &Server{db: database, bot: bot, token: token, basePath: basePath, downloadsPath: downloadsPath, debug: debug}
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// handler wraps the route mux with base-path handling and (in debug) CORS. When
// mounted under a prefix, the reverse proxy forwards the full "/breadbot/..."
// path; we strip it once here so every route pattern in routes() stays
// prefix-agnostic. healthz is also registered at the bare root so infra health
// checks don't need to know the subpath.
func (s *Server) handler() http.Handler {
	var h http.Handler
	if s.basePath == "" {
		h = s.routes()
	} else {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /healthz", s.handleHealthz)
		mux.Handle(s.basePath+"/", http.StripPrefix(s.basePath, s.routes()))
		h = mux
	}
	return s.withCORS(h)
}

// withCORS adds permissive CORS headers for local development only (gated on
// debug). In production the SPA is served same-origin from this process, so no
// CORS is needed or wanted. It also short-circuits preflight OPTIONS requests.
func (s *Server) withCORS(next http.Handler) http.Handler {
	if !s.debug {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// routes builds the mux. Go 1.22+ pattern matching gives us {id} path params
// without an external router. Patterns are relative to the mount point (base
// path already stripped by handler()).
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	// healthz is always unauthenticated.
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	mux.HandleFunc("GET /api/leaderboard", s.auth(s.handleLeaderboard))
	mux.HandleFunc("GET /api/stats/summary", s.auth(s.handleStatsSummary))
	mux.HandleFunc("GET /api/users", s.auth(s.handleUsers))
	mux.HandleFunc("GET /api/users/{id}/roundness", s.auth(s.handleUserRoundness))
	mux.HandleFunc("GET /api/users/{id}", s.auth(s.handleUser))
	mux.HandleFunc("GET /api/messages/{ogmessage_id}", s.auth(s.handleMessage))
	mux.HandleFunc("GET /api/images/predictions/{name}", s.auth(s.handleImage(imageKindPredictions)))
	mux.HandleFunc("GET /api/images/plots/{name}", s.auth(s.handleImage(imageKindPlots)))

	// Catch-all: serve the embedded SPA for everything else. This "/" pattern is
	// the lowest-priority match, so all specific routes above win. Unmatched
	// /api/* paths must still 404 as JSON (not fall through to index.html).
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		s.spaHandler()(w, r)
	})
	return mux
}

// ListenAndServe starts the server (blocks). Returns http.ErrServerClosed on
// graceful shutdown.
func (s *Server) ListenAndServe() error { return s.http.ListenAndServe() }

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

// auth wraps a handler with shared-token check when a token is configured. When
// no token is set, auth is a pass-through (matching the plan's "off if unset").
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	if s.token == "" {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.token {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}
