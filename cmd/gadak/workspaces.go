package main

import (
	"encoding/json"
	"net/http"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/workspace"
)

// buildServeMux wires the serve HTTP tree: primary API + SPA, workspace mounts,
// and the workspace list. Extracted so tests can exercise routing without a
// real listener or flag parse.
func buildServeMux(primaryAPI http.Handler, spa http.Handler, reg *workspace.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": version})
	})
	// PUT settings/ rewrites the config on disk, so re-read it per request.
	mux.HandleFunc("/config.json", func(w http.ResponseWriter, r *http.Request) {
		cur, err := config.Load()
		if err != nil {
			http.Error(w, `{"error":"config_unreadable"}`, http.StatusInternalServerError)
			return
		}
		doc, err := server.WebConfig(cur)
		if err != nil {
			http.Error(w, `{"error":"config_unreadable"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(doc)
	})
	// More specific than /api/ so the list is not swallowed by the primary handler.
	mux.HandleFunc("GET /api/v1/workspaces", workspace.ListHandler())
	mux.HandleFunc("GET /api/v1/workspaces/{$}", workspace.ListHandler())
	mux.Handle("/api/", primaryAPI)
	if reg != nil {
		mux.HandleFunc("/w/", reg.Handler(spa, version))
	}
	mux.Handle("/", spa)
	// The outer guard needs the same Host exemptions as the Handler's own
	// (GDK-443, GDK-797): both run on a paired request, and either one
	// rejecting a tailnet MagicDNS Host kills the path — passthrough or
	// mirror allowlist alike.
	dirFn := func() string {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			return ""
		}
		return cfg.Directory()
	}
	return server.GuardBrowser(mux, server.PairedOriginHostExempt(dirFn), server.PairedMirrorHostExempt(dirFn))
}
