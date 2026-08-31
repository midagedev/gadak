package workspace

// The removal core of `gadak workspaces rm <name>` (GDK-1098), extracted so
// the HTTP surface (DELETE /api/v1/workspaces/{name}, GDK-1096) enforces the
// same contract instead of a copy of it. Every refusal names its reason and
// the next move: the root workspace is the home itself, a standalone persist
// is the only copy of that tracker anywhere (SECURITY.md, "Offboarding"),
// and --yes is what separates "explain" from "delete" in a non-interactive
// verb. The CLI's refusal wording is byte-preserved: each typed error's
// Error() is exactly the message the CLI verb printed before the extraction,
// with cmdHint ("workspaces" / "profiles") standing in for the invoked
// spelling.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// RootRefusalError is the ""/"default" refusal: the unnamed workspace is the
// home directory itself, not a profile under it — removing "the default
// workspace" must not be rm -rf on the whole home by another door.
type RootRefusalError struct {
	Name string
	Home string
}

func (e *RootRefusalError) Error() string {
	return fmt.Sprintf("cannot remove the root workspace: %q is the unnamed workspace — the home directory itself\n  %s\n  offboarding the whole home is manual: rm -rf %s (SECURITY.md, \"Offboarding\" —\n  standalone profiles there hold the only copy of their trackers)",
		e.Name, e.Home, e.Home)
}

// NotFoundError is a name with no profile directory under profiles/.
type NotFoundError struct {
	Name        string
	ProfilesDir string
	CmdHint     string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no workspace named %q — profiles live under %s; see `gadak %s`",
		e.Name, e.ProfilesDir, e.CmdHint)
}

// KindUnreadableError is a config.json that will not parse. An unreadable
// config is a refusal, not a guess: if this might be standalone, its persist
// is the only copy of that tracker and a plain --yes must not destroy it.
type KindUnreadableError struct {
	Name       string
	ConfigPath string
	Err        error
}

func (e *KindUnreadableError) Error() string {
	return fmt.Sprintf("cannot determine what kind of workspace %q is: %s\n  its directory may hold a standalone persist (the only copy of that tracker) —\n  fix or delete %s by hand, then retry",
		e.Name, e.Err, e.ConfigPath)
}

func (e *KindUnreadableError) Unwrap() error { return e.Err }

// NeedsDestroyOriginError: standalone with an existing persist, but the
// caller did not opt into destroying the only copy of that tracker.
type NeedsDestroyOriginError struct {
	Name    string
	Persist string
	CmdHint string
}

func (e *NeedsDestroyOriginError) Error() string {
	return fmt.Sprintf("refusing: %q is a standalone workspace and its persist is the only copy of that tracker anywhere\n  persist: %s — plain SQLite; copy it out and it reads without gadak\n  to remove it anyway: gadak %s rm %s --yes --destroy-origin",
		e.Name, e.Persist, e.CmdHint, e.Name)
}

// NeedsYesError: the caller asked about removal without committing to it.
// Standalone picks the wording that says there is no origin data to protect
// (no persist exists); connected picks the one that says the origin keeps
// everything.
type NeedsYesError struct {
	Name       string
	CmdHint    string
	Standalone bool
}

func (e *NeedsYesError) Error() string {
	if e.Standalone {
		return fmt.Sprintf("refusing: removing %q needs --yes\n  no standalone persist exists in it — there is no origin data to protect\n  to proceed: gadak %s rm %s --yes",
			e.Name, e.CmdHint, e.Name)
	}
	return fmt.Sprintf("refusing: removing %q needs --yes\n  it deletes only this machine's mirror and credential — the origin (your Jira site, or the\n  paired home serve) keeps everything\n  to proceed: gadak %s rm %s --yes",
		e.Name, e.CmdHint, e.Name)
}

// InvalidNameError wraps config.DirFor's refusal (separator, "..", ...).
// DirFor stays the single owner of name validation; this type only makes the
// refusal distinguishable from other failures for the HTTP mapper.
type InvalidNameError struct {
	Name string
	Err  error
}

func (e *InvalidNameError) Error() string { return e.Err.Error() }

func (e *InvalidNameError) Unwrap() error { return e.Err }

// RemoveResult is what a successful removal did. The CLI shell reassembles
// its output lines from these fields; the HTTP handler serialises them. The
// core itself prints nothing except the stored-default warning, which was
// stderr-bound before the extraction and stays that way (both surfaces want
// it on stderr, and a silent failure to clear a now-dangling default is
// worse than a duplicated line).
type RemoveResult struct {
	Removed         string
	Kind            string
	OriginDestroyed bool

	// The success lines below need more than the three fields above:
	// Dir is the removed directory, Persist the destroyed persist path
	// (empty when none), Standalone the pre-removal kind reading.
	Dir        string
	Persist    string
	Standalone bool

	// Advisory materials.
	PairedSelector string
	ClearedStored  bool
	WasActive      bool
}

// Advisories returns the lines that are true for every successful removal,
// whatever the kind: the pairing hint for a workspace that paired with a
// home serve, the stored-default cleanup note, the was-active retarget hint,
// and the serve-staleness warning (there is no reliable detector for a
// running serve since the persist lock was removed, GDK-936). One owner of
// the wording — the CLI prints each line, the HTTP handler returns them as
// the JSON "advisories" array.
func (res RemoveResult) Advisories() []string {
	var lines []string
	if res.PairedSelector != "" {
		lines = append(lines, fmt.Sprintf("the home serve still holds this workspace's pairing token — revoke it there: gadak pairing revoke %s", res.PairedSelector))
	}
	if res.ClearedStored {
		lines = append(lines, "cleared the stored default workspace (it pointed at the removed one)")
	}
	if res.WasActive {
		lines = append(lines, "this was the active workspace — target another with --workspace, or clear the selection with: gadak workspace use --clear")
	}
	lines = append(lines, "note: a serve that was running against this workspace keeps its last view until it is restarted")
	return lines
}

// Remove deletes the named workspace profile. Refusals are typed so each
// surface (CLI text, HTTP JSON) can answer with its own shape without
// re-deriving the decision; every refusal's Error() is the CLI wording.
// cmdHint is the command spelling the "to proceed" lines name ("workspaces"
// or "profiles" for the CLI aliases, "workspaces" for HTTP).
func Remove(name string, yes, destroyOrigin bool, cmdHint string) (RemoveResult, error) {
	// The unnamed workspace is the home directory itself — see
	// RootRefusalError. Offboarding the whole home stays a manual step.
	if name == "" || name == "default" {
		home, err := config.DirFor("")
		if err != nil {
			return RemoveResult{}, err
		}
		return RemoveResult{}, &RootRefusalError{Name: name, Home: home}
	}

	// DirFor is the single owner of name validation: it rejects separators,
	// "."/"..", and anything else that could escape profiles/.
	dir, err := config.DirFor(name)
	if err != nil {
		return RemoveResult{}, &InvalidNameError{Name: name, Err: err}
	}
	info, err := os.Stat(dir)
	if os.IsNotExist(err) || (err == nil && !info.IsDir()) {
		return RemoveResult{}, &NotFoundError{Name: name, ProfilesDir: filepath.Dir(dir), CmdHint: cmdHint}
	}
	if err != nil {
		return RemoveResult{}, err
	}

	// Kind decides the safety contract (see KindUnreadableError).
	cfg, err := config.LoadFor(name)
	if err != nil {
		return RemoveResult{}, &KindUnreadableError{Name: name, ConfigPath: filepath.Join(dir, "config.json"), Err: err}
	}
	standalone := cfg.IsStandalone()
	persist := ""
	if standalone {
		persist = existingPersist(dir)
	}

	if standalone && persist != "" && !destroyOrigin {
		return RemoveResult{}, &NeedsDestroyOriginError{Name: name, Persist: persist, CmdHint: cmdHint}
	}
	if !yes {
		return RemoveResult{}, &NeedsYesError{Name: name, CmdHint: cmdHint, Standalone: standalone}
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
	// os.RemoveAll failing mid-removal (permissions, open handles on
	// Windows) can leave the directory half gone — name the directory.
	// Nothing distinguishes this from other failures (the GDK-1239 census
	// found no errors.As or field reader), so it stays a wrapped error.
	if err := os.RemoveAll(dir); err != nil {
		return RemoveResult{}, fmt.Errorf("removing %s: %w", dir, err)
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
	return RemoveResult{
		Removed:         name,
		Kind:            kind,
		OriginDestroyed: persist != "",
		Dir:             dir,
		Persist:         persist,
		Standalone:      standalone,
		PairedSelector:  pairedSelector,
		ClearedStored:   clearedStored,
		WasActive:       wasActive,
	}, nil
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
