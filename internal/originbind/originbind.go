// Package originbind owns one invariant: a workspace is bound to one origin.
//
// Both surfaces that can change which origin owns a workspace live behind it —
// `gadak init` and PUT onboarding/connect/ — so the decision cannot exist on
// one path and not the other. That split is exactly how GDK-247 happened: the
// CLI path was closed and the unauthenticated HTTP path was not.
//
// It is not part of internal/workspace, which mounts profiles under /w/ and
// imports internal/server: the server calls this, so that would cycle.
package originbind

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// ErrCodeReplaceRefused is the --json / HTTP "error" value when a connected
// init or onboarding connect refuses to take over a standalone workspace
// that holds data. The string is a CLI --json contract; do not change it.
const ErrCodeReplaceRefused = "standalone_data_present"

// ReplaceRefusedError is returned when RefuseReplace blocks a connect.
// Error() is the human sentence previously printed by gadak init.
type ReplaceRefusedError struct {
	Issues  int
	Persist string
}

func (e *ReplaceRefusedError) Error() string {
	if e == nil {
		return ""
	}
	return replaceMessage(e.Issues, e.Persist)
}

// replaceMessage is the former standaloneReplaceMessage. Wording is a
// user-facing contract; do not invent a new sentence.
func replaceMessage(n int, persist string) string {
	noun, exist := "issues or pages", "they exist only here"
	if n == 1 {
		noun, exist = "issue or page", "it exists only here"
	}
	return fmt.Sprintf("this workspace holds %d %s that originated here, in gadak's own tracker; %s — no Jira site has a copy\norigin persist file: %s\nconnect the site in a separate workspace: gadak --workspace <name> init\n(list workspaces with gadak workspaces)\nto replace this workspace anyway (converting deletes these issues or pages from the mirror): --replace-standalone",
		n, noun, exist, persist)
}

// WorkspaceOpenError is returned when CLI conversion would run while another
// process (serve or the desktop app) has this workspace open.
type WorkspaceOpenError struct {
	PID  int
	Addr string
}

func (e *WorkspaceOpenError) Error() string {
	if e == nil {
		return ""
	}
	const tail = " has this workspace open — close the app or serve, then retry"
	if e.PID == 0 {
		return "another process" + tail
	}
	// The desktop app holds a workspace without listening on anything, so a
	// holder with no address is normal here — printing "port )" for it was
	// the tell that this branch assumed a serve (GDK-971).
	port := e.Addr
	if _, p, err := net.SplitHostPort(e.Addr); err == nil && p != "" {
		port = p
	}
	if port == "" {
		return fmt.Sprintf("another process (pid %d)%s", e.PID, tail)
	}
	return fmt.Sprintf("another process (pid %d / port %s)%s", e.PID, port, tail)
}

// RefuseIfOpen stops a CLI standalone→connected conversion while another
// process has this workspace open. HTTP conversion runs inside the owner
// process and must not call this.
//
// Two questions, because one answer does not cover both holders:
//
//	A live `gadak serve` is found through the home-root run directory
//	(serveaddr) and an identity probe — not leftover serve-origin.json,
//	which GDK-936 made meaningless.
//
//	Gadak.app opens no port at all (its assets go through a custom scheme
//	handler), so serveaddr cannot see it. The persist's open marker can
//	(GDK-971). That marker is advisory and never arbitrates writes — WAL
//	does that — it only says someone is holding this workspace.
func RefuseIfOpen(cfg *config.Config) error {
	if cfg == nil || !cfg.IsStandalone() {
		return nil
	}
	if rec, ok := origin.LiveServeFor(cfg.ProfileName()); ok {
		return &WorkspaceOpenError{PID: rec.PID, Addr: rec.Addr}
	}
	if pid := origin.OpenHolder(origin.PersistPath(profileDir(cfg))); pid > 0 {
		return &WorkspaceOpenError{PID: pid}
	}
	return nil
}

func profileDir(cfg *config.Config) string {
	if cfg != nil {
		if d := cfg.Directory(); d != "" {
			return d
		}
	}
	d, _ := config.Dir()
	return d
}

// DropStandaloneProjection is the conversion cleanup both CLI init and HTTP
// onboarding must run: drop the seeded LOC wiki scope, then hand both sources
// to the store's origin-replacement owner.
//
// What that owner removes is decided by the table classification in
// internal/store/origin_scope.go, not here. It used to be a literal list of
// DELETEs, and four tables added by later migrations were never added to it —
// so a converted workspace kept plugin enrichments, feed read marks, field
// usage and sync runs that named the retired origin's keys, which do not go
// stale but rebind to whatever the new site put at the same key (GDK-418).
//
// The returned OriginReset is what to tell the user; an empty String() means
// nothing personal was bound to the old origin.
func DropStandaloneProjection(cfg *config.Config, db *store.DB) (store.OriginReset, error) {
	if cfg != nil {
		cfg.Confluence = nil
	}
	if db == nil {
		return store.OriginReset{}, fmt.Errorf("drop standalone mirror: nil store")
	}
	reset, err := db.ResetForNewOrigin(context.Background(), []string{"jira", "confluence"})
	if err != nil {
		return store.OriginReset{}, fmt.Errorf("drop standalone mirror: %w", err)
	}
	return reset, nil
}

// RefuseReplace stops a connected init / onboarding connect from silently
// changing which origin owns a standalone workspace that holds locally
// originated data. An empty standalone workspace (tried it, nothing
// filed) is not a hazard and is allowed through.
//
// JSON rendering stays at the CLI call site — this returns an error only.
func RefuseReplace(cfg *config.Config, replace bool) error {
	if cfg == nil || !cfg.IsStandalone() || replace {
		return nil
	}
	n, persist, err := LocalData(cfg)
	if err != nil {
		return fmt.Errorf("cannot replace standalone workspace: %w", err)
	}
	if n == 0 {
		return nil
	}
	return &ReplaceRefusedError{Issues: n, Persist: persist}
}

// ClearStandalone is the single owner of "origin is now a Jira site, so
// this workspace is no longer standalone". Reached only after RefuseReplace
// (or an explicit replace opt-in). Empty standalone workspaces take this
// path too: once connected, Kind must be cleared.
func ClearStandalone(next *config.Config) {
	if next == nil {
		return
	}
	next.Kind = ""
}

// LocalData reports how much locally originated data this standalone
// workspace holds, and the origin persist path (via origin.PersistPath —
// never rebuilt from string pieces). The returned n is max(issues, pages).
//
// Each of issues and pages is itself max(mirror, origin):
//   - mirror: SELECT COUNT(*) FROM issues / pages, only if gadak.db exists
//     (store.Open would create it)
//   - origin issues: Search on the in-process origin, only if the persist
//     file or a sibling legacy YAML already exists (origin.Client would
//     create an empty persist)
//   - origin pages: wiki SearchPages, best-effort (a SearchPages miss is
//     0, not a LocalData failure — issuetap may not implement CQL)
//
// init --standalone creates a persist file with a project fixture and no
// issues or pages, so that empty-origin case is n==0 and the common "I
// tried it, now I want to connect" path is not blocked.
func LocalData(cfg *config.Config) (n int, persist string, err error) {
	dir := profileDir(cfg)
	if dir == "" {
		dir, err = config.Dir()
		if err != nil {
			return 0, "", err
		}
	}
	persist = origin.PersistPath(dir)

	mirrorIssues, err := mirrorTableCount("issues")
	if err != nil {
		return 0, persist, err
	}
	mirrorPages, err := mirrorTableCount("pages")
	if err != nil {
		return 0, persist, err
	}
	originIssues, err := originIssueCount(cfg, persist)
	if err != nil {
		return 0, persist, err
	}
	originPages, err := originPageCount(cfg, persist)
	if err != nil {
		return 0, persist, err
	}
	issues := max(mirrorIssues, originIssues)
	pages := max(mirrorPages, originPages)
	return max(issues, pages), persist, nil
}

func mirrorTableCount(table string) (int, error) {
	path, err := config.DBPath()
	if err != nil {
		return 0, err
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if st.IsDir() {
		return 0, nil
	}
	db, err := store.Open(path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	return db.TableCount(context.Background(), table)
}

// originPersistPresent is true when the SQLite persist exists or a sibling
// legacy YAML does. Either is enough for origin.Client to have a graph
// (YAML seeds the db on first open). Absent both, Client would create an
// empty persist — LocalData must not do that.
func originPersistPresent(persist string) (bool, error) {
	if persist == "" {
		return false, nil
	}
	paths := []string{persist}
	if dir := filepath.Dir(persist); dir != "." && dir != "" {
		paths = append(paths, filepath.Join(dir, filepath.Base(filepath.FromSlash(origin.LegacyYAMLRel))))
	}
	seen := map[string]bool{}
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		_, err := os.Stat(p)
		if err == nil {
			return true, nil
		}
		if !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}

func originIssueCount(cfg *config.Config, persist string) (int, error) {
	present, err := originPersistPresent(persist)
	if err != nil {
		return 0, err
	}
	if !present {
		return 0, nil
	}
	c, err := origin.Client(cfg)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n := 0
	err = c.Search(ctx, "ORDER BY created ASC", []string{"summary"}, false, func(issues []jira.Issue) error {
		n += len(issues)
		return nil
	})
	return n, err
}

func originPageCount(cfg *config.Config, persist string) (int, error) {
	present, err := originPersistPresent(persist)
	if err != nil {
		return 0, err
	}
	if !present {
		return 0, nil
	}
	w, err := origin.Wiki(cfg)
	if err != nil {
		// Best-effort: a busy/unreadable origin still has the mirror count.
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n := 0
	err = w.SearchPages(ctx, "type=page", func(pages []confluence.Page) error {
		n += len(pages)
		return nil
	})
	if err != nil {
		return 0, nil
	}
	return n, nil
}
