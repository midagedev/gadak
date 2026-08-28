package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// workspaceRMUsage is the shape refusal for `workspaces rm`.
const workspaceRMUsage = "usage: gadak workspaces rm <name> [--yes] [--destroy-origin] [--json]"

// workspaceRMDoc is the `--json` success document: exactly what was removed
// and whether the standalone origin died with it.
type workspaceRMDoc struct {
	Removed         string `json:"removed"`
	Kind            string `json:"kind"`
	OriginDestroyed bool   `json:"origin_destroyed"`
}

// removeWorkspace implements `gadak workspaces rm <name>` (same verb through
// the `profiles` alias). Every refusal names its reason and the next move:
// the root workspace is the home itself, a standalone persist is the only
// copy of that tracker anywhere (SECURITY.md, "Offboarding"), and --yes is
// what separates "explain" from "delete" in a non-interactive verb.
func removeWorkspace(invoked string, args []string) error {
	if wantsHelp(args) {
		printHelp(invoked)
		return nil
	}
	fs := newFlagSet(invoked + " rm")
	yes := fs.Bool("yes", false, "remove the workspace — without this, rm explains and refuses")
	destroyOrigin := fs.Bool("destroy-origin", false, "standalone only: also destroy this profile's persist, the only copy of that tracker")
	asJSON := fs.Bool("json", false, "emit JSON")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || strings.TrimSpace(pos[0]) == "" {
		return usageError(invoked+" rm", workspaceRMUsage)
	}
	name := strings.TrimSpace(pos[0])

	// The unnamed workspace is the home directory itself, not a profile
	// under it — removing "the default workspace" must not be rm -rf on the
	// whole home by another door. Offboarding is a documented manual step.
	if name == "" || name == "default" {
		home, err := config.DirFor("")
		if err != nil {
			return err
		}
		return fmt.Errorf("cannot remove the root workspace: %q is the unnamed workspace — the home directory itself\n  %s\n  offboarding the whole home is manual: rm -rf %s (SECURITY.md, \"Offboarding\" —\n  standalone profiles there hold the only copy of their trackers)",
			name, home, home)
	}

	// DirFor is the single owner of name validation: it rejects separators,
	// "."/"..", and anything else that could escape profiles/.
	dir, err := config.DirFor(name)
	if err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if os.IsNotExist(err) || (err == nil && !info.IsDir()) {
		return fmt.Errorf("no workspace named %q — profiles live under %s; see `gadak %s`",
			name, filepath.Dir(dir), invoked)
	}
	if err != nil {
		return err
	}

	// Kind decides the safety contract. An unreadable config is a refusal,
	// not a guess: if this might be standalone, its persist is the only
	// copy of that tracker and a plain --yes must not destroy it.
	cfg, err := config.LoadFor(name)
	if err != nil {
		return fmt.Errorf("cannot determine what kind of workspace %q is: %w\n  its directory may hold a standalone persist (the only copy of that tracker) —\n  fix or delete %s by hand, then retry", name, err, filepath.Join(dir, "config.json"))
	}
	standalone := cfg.IsStandalone()
	persist := ""
	if standalone {
		persist = existingPersist(dir)
	}

	if standalone && persist != "" && !*destroyOrigin {
		return fmt.Errorf("refusing: %q is a standalone workspace and its persist is the only copy of that tracker anywhere\n  persist: %s — plain SQLite; copy it out and it reads without gadak\n  to remove it anyway: gadak %s rm %s --yes --destroy-origin",
			name, persist, invoked, name)
	}
	if !*yes {
		if standalone {
			return fmt.Errorf("refusing: removing %q needs --yes\n  no standalone persist exists in it — there is no origin data to protect\n  to proceed: gadak %s rm %s --yes",
				name, invoked, name)
		}
		return fmt.Errorf("refusing: removing %q needs --yes\n  it deletes only this machine's mirror and credential — the origin (your Jira site, or the\n  paired home serve) keeps everything\n  to proceed: gadak %s rm %s --yes",
			name, invoked, name)
	}

	wasActive := config.Profile() == name
	// Read pairing state before the removal deletes the credential it
	// lives in — the hint describes a token that outlives this directory
	// on the home serve, not one we just destroyed here.
	pairedSelector := ""
	if !standalone {
		if rem, err := origin.PairedStatus(cfg); err == nil && rem != nil {
			pairedSelector = rem.Label
			if pairedSelector == "" {
				pairedSelector = "<label|hash-prefix>"
			}
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing %s: %w", dir, err)
	}

	// A stored default pointing at the removed name would dangle: every
	// later command would resolve to a workspace that no longer exists.
	clearedStored := false
	if src, _ := config.WorkspaceSource(); src == config.SourceStored && config.Profile() == name {
		if err := config.ClearStoredWorkspace(); err == nil {
			clearedStored = true
		} else {
			fmt.Fprintf(os.Stderr, "warning: could not clear the stored default workspace: %v\n", err)
		}
	}

	kind, _ := origin.Describe(cfg) // same single owner the workspaces list's Kind column uses
	doc := workspaceRMDoc{Removed: name, Kind: kind, OriginDestroyed: persist != ""}
	if *asJSON {
		// Advisories go to stderr so stdout stays exactly the document.
		if err := json.NewEncoder(os.Stdout).Encode(doc); err != nil {
			return err
		}
		rmAdvisories(os.Stderr, pairedSelector, clearedStored, wasActive)
		return nil
	}

	fmt.Printf("removed workspace %q (%s) — %s\n", name, kind, dir)
	if doc.OriginDestroyed {
		fmt.Printf("destroyed standalone origin: %s\n", persist)
	} else if standalone {
		fmt.Println("no standalone persist was present; no origin data existed to destroy")
	} else {
		fmt.Println("the origin is untouched: only this machine's mirror and credential existed here")
	}
	rmAdvisories(os.Stdout, pairedSelector, clearedStored, wasActive)
	return nil
}

// rmAdvisories prints the lines that are true for every successful removal,
// whatever the kind: the pairing hint for a workspace that paired with a
// home serve, the serve-staleness warning (there is no reliable detector
// for a running serve since the persist lock was removed, GDK-936), and the
// stored-default/active cleanup notes.
func rmAdvisories(w *os.File, pairedSelector string, clearedStored, wasActive bool) {
	if pairedSelector != "" {
		fmt.Fprintf(w, "the home serve still holds this workspace's pairing token — revoke it there: gadak pairing revoke %s\n", pairedSelector)
	}
	if clearedStored {
		fmt.Fprintln(w, "cleared the stored default workspace (it pointed at the removed one)")
	}
	if wasActive {
		fmt.Fprintln(w, "this was the active workspace — target another with --workspace, or clear the selection with: gadak workspace use --clear")
	}
	fmt.Fprintln(w, "note: a serve that was running against this workspace keeps its last view until it is restarted")
}

// existingPersist returns the absolute persist path that actually exists in
// a profile directory — the SQLite state, or the legacy YAML for profiles
// last written before it. Empty means the profile holds no origin data.
func existingPersist(dir string) string {
	for _, p := range []string{origin.PersistPath(dir), origin.LegacyYAMLPath(dir)} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
