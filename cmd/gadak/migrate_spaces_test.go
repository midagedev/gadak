package main

import (
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestMigrateWithoutWikiPagesDoesNotInheritTheLocalDefaultSpace is the
// GDK-1484 seed half: `gadak migrate` builds a brand-new origin from the
// export, so the local-origin default space key (LOC) is per-origin state
// from a workspace that no longer exists. An import carrying no wiki page
// used to leave confluence.spaces = ["LOC"] behind; the sync then took the
// explicit path, 404'd that key on every pass and mirrored zero pages
// forever. The honest scope for a migrated workspace is "every space this
// origin has" — the empty list.
func TestMigrateWithoutWikiPagesDoesNotInheritTheLocalDefaultSpace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	allowProfileCreate = true
	t.Cleanup(func() {
		allowProfileCreate = false
		_ = origin.Close()
		config.SetProfile("")
	})

	config.SetProfile("src-nowiki")
	if out, err := capture(t, func() error { return cmdInit([]string{"--local"}) }); err != nil {
		t.Fatalf("init src: %v\n%s", err, out)
	}
	createIssue(t, "migrate without a wiki page")
	if out, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync src: %v\n%s", err, out)
	}

	config.SetProfile("dst-nowiki")
	if out, err := capture(t, func() error { return cmdMigrate([]string{"--from", "src-nowiki"}) }); err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}

	cfg, err := config.LoadFor("dst-nowiki")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Confluence == nil {
		t.Fatalf("confluence block missing after migrate — the wiki pass would not run at all")
	}
	if len(cfg.Confluence.Spaces) != 0 {
		t.Fatalf("confluence.spaces = %v after a migrate that carried no wiki page; want the empty list (every space this origin has), never the source workspace's %q",
			cfg.Confluence.Spaces, origin.DefaultSpaceKey)
	}
}
