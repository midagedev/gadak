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
	"os"
	"time"

	"github.com/midagedev/gadak/internal/config"
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
	noun, exist := "issues", "they exist only here"
	if n == 1 {
		noun, exist = "issue", "it exists only here"
	}
	return fmt.Sprintf("this workspace is standalone and holds %d locally originated %s; %s — no Jira site has a copy\norigin persist file: %s\nconnect the site in a separate workspace: gadak --profile <name> init\n(list workspaces with gadak profiles)\nto replace this workspace anyway (converting deletes these issues from the mirror): --replace-standalone",
		n, noun, exist, persist)
}

// RefuseReplace stops a connected init / onboarding connect from silently
// changing which origin owns a standalone workspace that holds locally
// originated issues. An empty standalone workspace (tried it, nothing
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

// LocalData reports how many locally originated issues this standalone
// workspace holds, and the origin persist path (via origin.PersistPath —
// never rebuilt from string pieces).
//
// "Holds data" is max(mirror issues, origin issues):
//   - mirror: SELECT COUNT(*) FROM issues, only if gadak.db already exists
//     (store.Open would create it)
//   - origin: Search on the in-process origin, only if the persist file
//     already exists (origin.Client would create it)
//
// init --standalone creates a persist file with a project fixture and no
// issues, so that empty-origin case is n==0 and the common "I tried it,
// now I want to connect" path is not blocked.
func LocalData(cfg *config.Config) (n int, persist string, err error) {
	dir := ""
	if cfg != nil {
		dir = cfg.Directory()
	}
	if dir == "" {
		dir, err = config.Dir()
		if err != nil {
			return 0, "", err
		}
	}
	persist = origin.PersistPath(dir)

	mirrorN, err := mirrorIssueCount()
	if err != nil {
		return 0, persist, err
	}
	originN, err := originIssueCount(cfg, persist)
	if err != nil {
		return 0, persist, err
	}
	n = mirrorN
	if originN > n {
		n = originN
	}
	return n, persist, nil
}

func mirrorIssueCount() (int, error) {
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
	return db.TableCount(context.Background(), "issues")
}

func originIssueCount(cfg *config.Config, persist string) (int, error) {
	if persist == "" {
		return 0, nil
	}
	if _, err := os.Stat(persist); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
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
