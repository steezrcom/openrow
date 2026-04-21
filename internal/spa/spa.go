package spa

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Handler serves the built React SPA from a directory on disk. Actual files are
// returned as-is; unknown non-API paths fall through to index.html so the client
// router can handle them. If dir is empty or missing, the handler returns a
// helpful 503 so developers know to run the build.
func Handler(dir string) http.Handler {
	if dir == "" {
		dir = "web/dist"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return errorHandler("resolve spa dir: " + err.Error())
	}

	indexPath := filepath.Join(abs, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errorHandler("spa build missing at " + abs + "; run `cd web && npm run build`")
		}
		return errorHandler("stat spa index: " + err.Error())
	}

	fileServer := http.FileServer(http.Dir(abs))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "." || clean == "" {
			serveIndex(w, indexPath)
			return
		}
		// Reject traversal.
		if strings.HasPrefix(clean, "..") {
			http.NotFound(w, r)
			return
		}
		target := filepath.Join(abs, clean)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, indexPath)
	})
}

func errorHandler(msg string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, msg, http.StatusServiceUnavailable)
	})
}

func serveIndex(w http.ResponseWriter, path string) {
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "read index: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, f)
}
