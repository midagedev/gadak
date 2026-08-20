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

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
)

// serverAPIBase is the path prefix the live server hardcodes into attachment
// content_url fields. Hosted demos rewrite it to the configured --api-base.
const serverAPIBase = "/api/v1/issues/"

func cmdExportStatic(args []string) error {
	fs := newFlagSet("export-static")
	dbPath := fs.String("db", "examples/demo.db", "snapshot database to freeze")
	attachments := fs.String("attachments", "examples/attachments",
		"directory holding manifest.json + attachment files")
	apiBase := fs.String("api-base", "/gadak/api/v1/issues/",
		"apiBase written into config.json and rewritten into attachment content_url fields")
	authBase := fs.String("auth-base", "/gadak/api/v1/auth/",
		"authBase written into config.json")
	projects := fs.String("projects", "NMB,NMA,NMS",
		"comma-separated project keys baked into the snapshot config")
	scrub := fs.Bool("scrub", false,
		"whitelist-rebuild the snapshot for public backlog publishing: "+
			"issues keep only list fields; descriptions, comments, attachments, "+
			"history, people and custom fields are dropped")
	// Flags must come before the positional outdir (Go flag stops at the first
	// non-flag). build.mjs and the usage line keep that order.
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageError("export-static", "usage: gadak export-static [flags] <outdir>\n  --db --attachments --api-base --auth-base")
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
	tmp, err := os.MkdirTemp("", "gadak-export-static-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	workDB := filepath.Join(tmp, "gadak.db")
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
	// Site is empty: NewWithCache below builds the same empty-site config, so
	// Key collapses to the legacy id-only form (same as gadak demo).
	stats, err := importAttachmentsInto(*attachments, cacheDir, "", config.Profile(), workDB)
	if err != nil {
		return fmt.Errorf("import attachments: %w", err)
	}
	logAttachmentImport("export-static: attachment import", stats)
	cache, err := attachcache.New(cacheDir, 0)
	if err != nil {
		return err
	}

	db, err := store.Open(workDB)
	if err != nil {
		return err
	}
	defer db.Close()

	projectList := strings.Split(*projects, ",")
	for i := range projectList {
		projectList[i] = strings.TrimSpace(projectList[i])
	}

	// Snapshot projects only — no credential, every optional surface off.
	cfg := &config.Config{
		Projects: projectList,
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
	if *scrub {
		bootBody, err = scrubBootstrap(bootBody)
		if err != nil {
			return fmt.Errorf("scrub bootstrap: %w", err)
		}
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
		if *scrub {
			body, err = scrubDetail(body)
			if err != nil {
				return fmt.Errorf("scrub detail %s: %w", key, err)
			}
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
		"projects":            projectList,
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
	if *scrub {
		// A named profile gives the backlog page its own IndexedDB cache scope
		// (composeCacheScope) so it never mixes rows with the demo on the same
		// origin.
		cfgDoc["profile"] = "backlog"
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

/* ── backlog scrub (GDK-389) ──
 * Whitelist-REBUILD, never blacklist-strip: a field added to the live
 * handlers later must stay private by default. Only the keys listed here
 * survive into a published snapshot.
 */

// issueWhitelist is every issue-list key a public backlog snapshot may carry.
// People (assignee/reporter/emails), custom fields and clone provenance are
// deliberately absent.
var issueWhitelist = []string{
	"issue_key", "summary", "project_key",
	"issue_type", "issue_type_id",
	"status", "status_id", "status_category",
	"priority", "priority_id", "priority_rank",
	"epic_key", "parent_key",
	"labels", "components", "fix_versions",
	"duedate", "resolution",
	"created_at", "updated_at", "status_changed_at", "resolved_at",
	"comment_count", "source",
}

func scrubBootstrap(body []byte) ([]byte, error) {
	var boot map[string]json.RawMessage
	if err := json.Unmarshal(body, &boot); err != nil {
		return nil, err
	}
	var issues []map[string]json.RawMessage
	if err := json.Unmarshal(boot["issues"], &issues); err != nil {
		return nil, err
	}
	outIssues := make([]map[string]json.RawMessage, 0, len(issues))
	for _, is := range issues {
		kept := map[string]json.RawMessage{}
		for _, k := range issueWhitelist {
			if v, ok := is[k]; ok {
				kept[k] = v
			}
		}
		outIssues = append(outIssues, kept)
	}
	issuesRaw, err := json.Marshal(outIssues)
	if err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{
		"server_time":     boot["server_time"],
		"sync_version":    boot["sync_version"],
		"members":         json.RawMessage("[]"),
		"members_version": boot["members_version"],
		"issues":          issuesRaw,
		"sync_health":     boot["sync_health"],
		"field_specs":     json.RawMessage("[]"),
		"field_usage":     json.RawMessage("{}"),
	}
	return json.Marshal(out)
}

// scrubDetail keeps the key and the linked-issue graph (public structure) and
// forces every content surface — description, comments, attachments, history,
// bodies, enrichments — to its empty shape.
func scrubDetail(body []byte) ([]byte, error) {
	var det map[string]json.RawMessage
	if err := json.Unmarshal(body, &det); err != nil {
		return nil, err
	}
	linked := det["linked_issues"]
	if len(linked) == 0 {
		linked = json.RawMessage("[]")
	}
	out := map[string]json.RawMessage{
		"issue_key":           det["issue_key"],
		"description_adf":     json.RawMessage("null"),
		"attachments":         json.RawMessage("[]"),
		"comments":            json.RawMessage("[]"),
		"history":             json.RawMessage("[]"),
		"linked_issues":       linked,
		"development_opinion": json.RawMessage("null"),
		"qa_context":          json.RawMessage("null"),
		"deploy":              json.RawMessage("null"),
		"linked_prs":          json.RawMessage("[]"),
		"bodies":              json.RawMessage("{}"),
	}
	return json.Marshal(out)
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
