// Package httpserver is a thin read-only HTTP API over the shared DB layer,
// running alongside the bot. It exposes stats endpoints and a liveness check
// so an admin panel can be built on top later. No write/admin actions.
package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/hawawa4/breadbotdiscord/internal/db"
)

// BotStatus reports whether the Discord session is connected (for /healthz).
type BotStatus interface {
	Ready() bool
}

// Server holds the dependencies for the read-only API.
type Server struct {
	db       *db.DB
	bot      BotStatus
	token    string // shared-secret auth; empty disables auth
	basePath string // URL mount prefix (e.g. "/breadbot"); empty = root
	http     *http.Server
}

// New builds a Server. token is the optional ADMIN_API_TOKEN (empty = auth
// off). basePath is the URL prefix the server is mounted under behind a reverse
// proxy (empty = root); it must already be normalized (leading slash, no
// trailing slash) as config.Load does.
func New(addr string, database *db.DB, bot BotStatus, token, basePath string) *Server {
	s := &Server{db: database, bot: bot, token: token, basePath: basePath}
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// handler wraps the route mux with base-path handling. When mounted under a
// prefix, the reverse proxy forwards the full "/breadbot/..." path; we strip it
// once here so every route pattern in routes() stays prefix-agnostic. healthz
// is also registered at the bare root so infra health checks don't need to know
// the subpath.
func (s *Server) handler() http.Handler {
	if s.basePath == "" {
		return s.routes()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.Handle(s.basePath+"/", http.StripPrefix(s.basePath, s.routes()))
	return mux
}

// routes builds the mux. Go 1.22+ pattern matching gives us {id} path params
// without an external router. Patterns are relative to the mount point (base
// path already stripped by handler()).
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	// healthz is always unauthenticated.
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	mux.HandleFunc("GET /api/leaderboard", s.auth(s.handleLeaderboard))
	mux.HandleFunc("GET /api/users/{id}/roundness", s.auth(s.handleUserRoundness))
	mux.HandleFunc("GET /api/users/{id}", s.auth(s.handleUser))
	mux.HandleFunc("GET /api/messages/{ogmessage_id}", s.auth(s.handleMessage))
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
