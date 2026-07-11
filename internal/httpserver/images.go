package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Image kinds served by the API. Each maps to a subdirectory under the
// downloads path. Kept as a fixed allow-list so a client can never select an
// arbitrary directory.
const (
	imageKindPredictions = "predictions"
	imageKindPlots       = "plots"
)

// handleImage serves a single image file (annotated prediction PNG or per-user
// plot PNG) from downloads/{kind}/{name}.
//
// Filenames are single path segments by construction (predictions are keyed by
// the source attachment basename; plots are "{authorID}_roundhistory.png"), so
// we reject anything that isn't its own basename. This blocks "../" traversal
// and absolute paths outright without needing to resolve symlinks.
func (s *Server) handleImage(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !safeImageName(name) {
			writeError(w, http.StatusBadRequest, "invalid image name")
			return
		}
		path := filepath.Join(s.downloadsPath, kind, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			writeError(w, http.StatusNotFound, "image not found")
			return
		}
		http.ServeFile(w, r, path)
	}
}

// safeImageName reports whether name is a plain single-segment filename with no
// traversal, separators, or leading dot. It must equal its own filepath.Base.
func safeImageName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return false
	}
	return filepath.Base(name) == name
}
