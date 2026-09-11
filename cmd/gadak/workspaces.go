package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/skillinstall"
	"github.com/midagedev/gadak/internal/workspace"
)

// buildDigest is the source digest of the tree this binary was built from,
// stamped by the harness builds that need a server to prove its own identity
// (GDK-1555): e2e/serve.sh and mobile/e2e/gate-serve.sh pass it as
// -X main.buildDigest=<sha256>. A release build or a plain `go build` leaves
// it empty — the healthz contract is "answer what you know", never "fail".
var buildDigest = ""

// buildCommit is the revision of the tree this binary was built from, same
// stamping route as buildDigest (-X main.buildCommit=<head>, full hex). The
// runtime fallback (skillinstall.BuildRevision, via debug.ReadBuildInfo) is
// not enough on its own for the one consumer that needs it: Go's buildvcs
// stamps nothing when the build happens in a *linked* worktree — the .git at
// the root is a file, not a directory, and the toolchain declines (measured
// 2026-09-11: main tree build carries vcs.revision, a worktree build of the
// same commit carries none). Worktrees are exactly where the phone gate and
// parallel e2e rounds run, so the harnesses stamp the commit explicitly and
// the fallback still covers release builds and hand-run `go build`.
var buildCommit = ""

// processStartedAt is this process's start, in epoch ms. /healthz reports it
// so a harness can tell the server it just started from one that
// reuseExistingServer adopted (same distinction the phone gate's stamp makes
// with builtAt, but answered by the process itself).
var processStartedAt = time.Now().UnixMilli()

// healthzCommit answers the healthz commit: the ldflags stamp when the
// harness provided one, else BuildRevision()'s short form with its "+"
// dirty marker.
func healthzCommit() string {
	if buildCommit != "" {
		return buildCommit
	}
	return skillinstall.BuildRevision()
}

// healthzDoc is /healthz's body: liveness plus identity (GDK-1555). A poll
// that only checks the status code will happily adopt whichever server is
// listening on the port it was told to poll — the incident behind GDK-1789 —
// so every field a poller needs to tell servers apart lives here: which
// binary (version, commit, digest), what it serves (home, workspace), and
// since when (startedAt, pid).
func healthzDoc() map[string]any {
	home := ""
	if cfg, err := config.Load(); err == nil && cfg != nil {
		home = cfg.Directory()
	}
	return map[string]any{
		"status":    "ok",
		"version":   version,
		"commit":    healthzCommit(),
		"digest":    buildDigest,
		"home":      home,
		"workspace": config.NormalizeProfile(config.Profile()),
		"startedAt": processStartedAt,
		"pid":       os.Getpid(),
	}
}

// buildServeMux wires the serve HTTP tree: primary API + SPA, workspace mounts,
// and the workspace list. Extracted so tests can exercise routing without a
// real listener or flag parse.
func buildServeMux(primaryAPI http.Handler, spa http.Handler, reg *workspace.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(healthzDoc())
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
	// Workspace creation/removal (GDK-1096), same spot as the list: the
	// outer GuardBrowser below covers these too, and neither Host exemption
	// admits /api/v1/workspaces* — a DNS-named Host never reaches the
	// handlers. DELETE takes the registry so an open /w/<name>/ mount is
	// closed before its directory is deleted.
	mux.HandleFunc("POST /api/v1/workspaces", workspace.CreateHandler())
	mux.HandleFunc("POST /api/v1/workspaces/{$}", workspace.CreateHandler())
	mux.HandleFunc("DELETE /api/v1/workspaces/{name}", workspace.RemoveHandler(reg))
	mux.Handle("/api/", primaryAPI)
	if reg != nil {
		mux.HandleFunc("/w/", reg.Handler(spa, version))
	}
	mux.Handle("/", spa)
	// The outer guard needs the same Host exemptions as the Handler's own
	// (GDK-443, GDK-797): both run on a paired request, and either one
	// rejecting a tailnet MagicDNS Host kills the path — passthrough or
	// mirror REST alike.
	dirFn := func() string {
		cfg, err := config.Load()
		if err != nil || cfg == nil {
			return ""
		}
		return cfg.Directory()
	}
	return server.GuardBrowser(mux, server.GuardExempts{
		Host: []func(*http.Request) bool{
			server.PairedOriginHostExempt(dirFn), server.PairedMirrorHostExempt(dirFn),
		},
		Origin: []func(*http.Request) bool{server.PairedAppOriginExempt(dirFn)},
	})
}
