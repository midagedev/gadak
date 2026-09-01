package originbind

// The reusable core of `gadak init --standalone` (config mutation, default
// issue-type resolution, mirror fill). Both origin-changing surfaces share it:
// the CLI verb and POST onboarding/standalone — the same reason RefuseReplace
// lives here (see the package doc). Every byte of CLI output stays at the call
// site; this file is silent.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// ParseProjectKeys splits a comma-separated project list the way init always
// has: trim, upper-case, drop empties. Exported because POST
// onboarding/standalone parses its body with the same rule — one function,
// not two parsers kept in step by review.
func ParseProjectKeys(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		p = strings.ToUpper(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// MirrorOpener hands SeedStandalone a mirror store and the release that must
// run after the fill. The CLI opens a fresh store and closes it; a serve
// process passes its already-open store back with a no-op release — that
// handle lives as long as the process does and must not be closed under a
// running server.
type MirrorOpener func() (db *store.DB, release func() error, err error)

// SeedStandalone turns cfg into a standalone workspace: it mutates and saves
// the config, resolves and records the default issue type, and fills the
// mirror so the next command is not "stale, run sync".
//
// Already-standalone is not detected here — callers need it before the call
// (the CLI prints a one-line idempotent path); compute cfg.IsStandalone()
// first if you need it.
//
// A fill that fails does not fail the seed: the workspace exists, its persist
// file is written, and writes already work. That contract moved here from
// initStandalone — returning a fatal error would break
// `init --standalone --json && gadak create …` over something the next
// `gadak sync` fixes. The fill failure comes back as fillErr instead; the
// caller decides how to surface it (CLI: stderr warning; serve: log line).
//
// The origin flush (origin.Close) is deliberately NOT here: it is the CLI
// process-exit path. A serve holds the origin session for its lifetime.
//
// spaces is the wiki space set the workspace syncs; nil keeps the seeded
// default (LOC). migrate passes the space keys its fixture carries, so the
// first fill mirrors them instead of a space the fixture does not contain.
func SeedStandalone(cfg *config.Config, projectsCSV string, spaces []string, openMirror MirrorOpener) (fillErr error, err error) {
	cfg.Kind = config.KindStandalone
	cfg.Site = ""
	cfg.Email = ""
	cfg.Token = ""
	cfg.TokenVerifiedAt = ""
	cfg.TokenOwner = ""
	cfg.TokenExpiresAt = ""
	cfg.TokenExpirySource = ""
	cfg.AccountID = ""
	if len(spaces) > 0 {
		cfg.Confluence = &config.ConfluenceConfig{Spaces: append([]string{}, spaces...)}
	} else {
		cfg.Confluence = origin.DefaultConfluenceConfig()
	}
	if projectsCSV != "" {
		cfg.Projects = ParseProjectKeys(projectsCSV)
	}
	if strings.TrimSpace(cfg.DefaultProject) == "" {
		if len(cfg.Projects) > 0 {
			cfg.DefaultProject = cfg.Projects[0]
		} else {
			cfg.DefaultProject = origin.DefaultProjectKey
		}
	}
	if err := cfg.Save(); err != nil {
		return nil, err
	}
	// The seeded project offers five issue types, so "the project's only
	// type" cannot resolve and `gadak create <summary>` would demand --type
	// on every call — the one thing this workspace exists to make cheap. Ask
	// the origin we just created (in-process, no network) and record the
	// answer, so the pick lives in config.json where it can be read and
	// changed rather than being guessed per create.
	typeID, typeName, err := standaloneDefaultType(cfg)
	if err != nil {
		return nil, err
	}
	if typeID != "" {
		cfg.DefaultIssueTypeID = typeID
		cfg.DefaultIssueType = typeName
		if err := cfg.Save(); err != nil {
			return nil, err
		}
	}
	fillErr = fillStandaloneMirror(cfg, openMirror)
	return fillErr, nil
}

// standaloneDefaultType picks the issue type new issues get when create is
// given only a summary. "Task" is preferred by name; otherwise the first type
// the origin offers. An origin.Client failure is an init failure — the
// origin we just declared is unusable (GDK-345). Returning "", "", nil
// leaves the config untouched: create then asks for --type, which is the
// pre-existing behaviour when the origin is up but offers no types.
//
// Unlike a headless per-create fallback (deliberately absent, see
// internal/create), this pick is written to config.json and printed, so the
// person can see what they got and change it.
func standaloneDefaultType(cfg *config.Config) (id, name string, err error) {
	c, err := origin.Client(cfg)
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	projects, err := c.CreateMeta(ctx, []string{cfg.DefaultProject})
	if err != nil || len(projects) == 0 {
		return "", "", nil
	}
	types := projects[0].IssueTypes
	if len(types) == 0 {
		return "", "", nil
	}
	for _, t := range types {
		if strings.EqualFold(t.Name, "Task") {
			return t.ID, t.Name, nil
		}
	}
	return types[0].ID, types[0].Name, nil
}

// fillStandaloneMirror runs the same one-shot Jira+Confluence sync `gadak
// sync` would, without printing. That stamps sync_state.synced_at so
// warnIfStale does not fire on the next command.
func fillStandaloneMirror(cfg *config.Config, openMirror MirrorOpener) error {
	db, release, err := openMirror()
	if err != nil {
		return err
	}
	defer release()
	ctx := context.Background()
	var opts syncer.Options
	if _, err := syncer.Run(ctx, cfg, db, opts); err != nil {
		return fmt.Errorf("fill mirror: %w", err)
	}
	if cfg.Confluence != nil {
		if _, err := syncer.RunConfluence(ctx, cfg, db, opts); err != nil {
			return fmt.Errorf("fill wiki mirror: %w", err)
		}
	}
	return nil
}
