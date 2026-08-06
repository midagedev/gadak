// export-static freezes the demo mirror into static JSON + attachment bytes
// for the zero-install hosted demo (GitHub Pages). Reuses the same HTTP
// handlers the live server uses so the snapshot cannot drift from the API.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/midagedev/scry/internal/attachcache"
	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/server"
	"github.com/midagedev/scry/internal/store"
)

// serverAPIBase is the path prefix the live server hardcodes into attachment
// content_url fields. Hosted demos rewrite it to the configured --api-base.
const serverAPIBase = "/api/v1/issues/"

func cmdExportStatic(args []string) error {
	fs := newFlagSet("export-static")
	dbPath := fs.String("db", "examples/demo.db", "snapshot database to freeze")
	attachments := fs.String("attachments", "examples/attachments",
		"directory holding manifest.json + attachment files")
	apiBase := fs.String("api-base", "/scry/api/v1/issues/",
		"apiBase written into config.json and rewritten into attachment content_url fields")
	authBase := fs.String("auth-base", "/scry/api/v1/auth/",
		"authBase written into config.json")
	// Flags must come before the positional outdir (Go flag stops at the first
	// non-flag). build.mjs and the usage line keep that order.
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageError("export-static", "usage: scry export-static [flags] <outdir>\n  --db --attachments --api-base --auth-base")
	}
	outDir := fs.Arg(0)
	if !strings.HasSuffix(*apiBase, "/") {
		*apiBase += "/"
	}
	if !strings.HasSuffix(*authBase, "/") {
		*authBase += "/"
	}

	if _, err := os.Stat(*dbPath); err != nil {
		return fmt.Errorf("demo snapshot %q: %w", *dbPath, err)
	}

	// Work on a throwaway copy so freshening the sync clock never mutates the
	// committed fixture, and so attachment cache files land in a temp tree.
	tmp, err := os.MkdirTemp("", "scry-export-static-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	workDB := filepath.Join(tmp, "scry.db")
	src, err := os.ReadFile(*dbPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(workDB, src, 0o600); err != nil {
		return err
	}
	if err := freshenDemoClock(workDB); err != nil {
		return fmt.Errorf("freshen sync clock: %w", err)
	}

	cacheDir := filepath.Join(tmp, "attachments")
	if err := importAttachmentsInto(*attachments, cacheDir); err != nil {
		return fmt.Errorf("import attachments: %w", err)
	}
	cache, err := attachcache.New(cacheDir, 0)
	if err != nil {
		return err
	}

	db, err := store.Open(workDB)
	if err != nil {
		return err
	}
	defer db.Close()

	// Demo projects only — no credential, every optional surface off.
	cfg := &config.Config{
		Projects: []string{"NMB", "NMA", "NMS"},
		Features: map[string]bool{
			"presence":   false,
			"feed":       false,
			"push":       false,
			"deploy":     false,
			"qa":         false,
			"teamGroups": false,
		},
	}
	handler := server.NewWithCache(db, cfg, cache)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	detailDir := filepath.Join(outDir, "detail")
	attachOut := filepath.Join(outDir, "attachments")
	if err := os.MkdirAll(detailDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(attachOut, 0o755); err != nil {
		return err
	}

	// bootstrap.json
	bootBody, err := getJSON(handler, "GET", serverAPIBase+"bootstrap/")
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "bootstrap.json"), bootBody, 0o644); err != nil {
		return err
	}

	var boot struct {
		Issues []struct {
			Key string `json:"issue_key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(bootBody, &boot); err != nil {
		return fmt.Errorf("parse bootstrap: %w", err)
	}
	if len(boot.Issues) == 0 {
		return fmt.Errorf("bootstrap returned 0 issues — is %q a valid mirror?", *dbPath)
	}

	// detail/<KEY>.json + collect attachment ids
	seenAttach := map[string]struct{}{}
	for i, issue := range boot.Issues {
		key := issue.Key
		if key == "" {
			return fmt.Errorf("bootstrap issue %d missing issue_key", i)
		}
		body, err := getJSON(handler, "GET", serverAPIBase+key+"/detail/")
		if err != nil {
			return fmt.Errorf("detail %s: %w", key, err)
		}
		// Rewrite absolute content_url paths so the hosted client (apiBase under
		// the Pages subpath) and the service worker agree.
		if *apiBase != serverAPIBase {
			body = bytes.ReplaceAll(body, []byte(serverAPIBase), []byte(*apiBase))
		}
		if err := os.WriteFile(filepath.Join(detailDir, key+".json"), body, 0o644); err != nil {
			return err
		}
		var det struct {
			Attachments []struct {
				ID string `json:"id"`
			} `json:"attachments"`
		}
		if err := json.Unmarshal(body, &det); err != nil {
			return fmt.Errorf("parse detail %s: %w", key, err)
		}
		for _, a := range det.Attachments {
			if a.ID != "" {
				seenAttach[a.ID] = struct{}{}
			}
		}
	}

	// attachments/<id> — bytes only; SW serves image/png (demo fixture is PNG).
	// Prefer the manifest files when present so we do not require the cache path.
	if err := copyAttachmentsFromManifest(*attachments, attachOut, seenAttach); err != nil {
		return err
	}
	// Fall back to the live cache for any id the manifest did not cover.
	for id := range seenAttach {
		dest := filepath.Join(attachOut, id)
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		body, err := getBytes(handler, "GET", serverAPIBase+"NMB-1/attachments/"+id+"/content/")
		// The key in the URL is ignored by the handler (id is the lookup).
		if err != nil {
			// Try via any issue key pattern — handler only uses {id}.
			return fmt.Errorf("attachment %s: %w", id, err)
		}
		if err := os.WriteFile(dest, body, 0o644); err != nil {
			return err
		}
	}

	// config.json — features all off, api under the Pages base path.
	cfgDoc := map[string]any{
		"apiBase":             *apiBase,
		"authBase":            *authBase,
		"jiraBaseUrl":         "",
		"qaDashboardUrl":      "",
		"projects":            []string{"NMB", "NMA", "NMS"},
		"groupLabels":         map[string]string{},
		"groupColors":         map[string]string{},
		"productByGroup":      map[string]any{},
		"staleThresholdHours": 72,
		"features": map[string]bool{
			"presence":   false,
			"feed":       false,
			"push":       false,
			"deploy":     false,
			"qa":         false,
			"teamGroups": false,
		},
		// Hosted-demo marker for operators; the client ignores unknown keys.
		"hostedDemo": true,
	}
	cfgBytes, err := json.MarshalIndent(cfgDoc, "", "  ")
	if err != nil {
		return err
	}
	cfgBytes = append(cfgBytes, '\n')
	if err := os.WriteFile(filepath.Join(outDir, "config.json"), cfgBytes, 0o644); err != nil {
		return err
	}

	fmt.Printf("export-static: %d issues, %d attachments → %s\n",
		len(boot.Issues), len(seenAttach), outDir)
	return nil
}

// copyAttachmentsFromManifest writes attachments/<id> from the fixture dir for
// every id listed in both the manifest and seen. Missing files are skipped so
// the cache fallback can fill them.
func copyAttachmentsFromManifest(dir, outDir string, seen map[string]struct{}) error {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var manifest struct {
		Attachments []struct {
			ID   string `json:"id"`
			File string `json:"file"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return err
	}
	for _, a := range manifest.Attachments {
		if _, ok := seen[a.ID]; !ok {
			continue
		}
		src := filepath.Join(dir, a.File)
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", src, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, a.ID), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func getJSON(h http.Handler, method, path string) ([]byte, error) {
	body, code, err := doReq(h, method, path)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("%s %s → %d: %s", method, path, code, truncate(body, 200))
	}
	return body, nil
}

func getBytes(h http.Handler, method, path string) ([]byte, error) {
	return getJSON(h, method, path)
}

func doReq(h http.Handler, method, path string) ([]byte, int, error) {
	req := httptest.NewRequest(method, path, nil)
	// httptest defaults Host to example.com, which the server's browser guard
	// rejects; these are in-process requests, so present as loopback.
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, 0, err
	}
	return body, res.StatusCode, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
