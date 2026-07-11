package httpserver

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// spaFiles holds the compiled Svelte SPA. The build output is written here by
// `just frontend` (Vite outDir → internal/httpserver/frontend/dist). A
// committed placeholder index.html keeps this embed — and therefore the whole
// Go build — working on a clean checkout before the frontend is ever built.
//
//go:embed all:frontend/dist
var spaFiles embed.FS

// spaFS returns the dist directory as a filesystem rooted at its contents (so
// "index.html" resolves, not "frontend/dist/index.html").
func spaFS() fs.FS {
	sub, err := fs.Sub(spaFiles, "frontend/dist")
	if err != nil {
		// Unreachable: the path is a compile-time embed constant.
		panic(err)
	}
	return sub
}

// spaHandler serves the embedded SPA. Real asset files are served directly;
// any other (non-/api, non-/healthz) path falls back to index.html so the
// client-side router can handle it. The base path is already stripped by
// handler(), so paths here are relative to the mount point.
func (s *Server) spaHandler() http.HandlerFunc {
	files := spaFS()
	fileServer := http.FileServer(http.FS(files))
	return func(w http.ResponseWriter, r *http.Request) {
		// Never let the SPA fallback shadow the API surface.
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			serveIndex(w, r, files)
			return
		}
		if f, err := files.Open(p); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// Unknown path with no matching asset → SPA entry point.
		serveIndex(w, r, files)
	}
}

// serveIndex writes the SPA entry document.
func serveIndex(w http.ResponseWriter, r *http.Request, files fs.FS) {
	data, err := fs.ReadFile(files, "index.html")
	if err != nil {
		http.Error(w, "SPA not built", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
