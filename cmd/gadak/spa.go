package main

// spaHandler and spaHandlerFS serve the built UI with index.html fallback so
// client-side routes survive a reload.

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// spaHandler serves the built UI, falling back to index.html so client-side
// routes survive a reload.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

// spaHandlerFS serves the embedded web UI with the same SPA fallback:
// unknown paths get index.html so hash-routed deep links work.
func spaHandlerFS(fsys fs.FS) http.Handler {
	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" {
			if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.Error(w, "web UI not embedded — build with `npm run build` before `go build`, or pass --static", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}
