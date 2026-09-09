package main

import (
	"testing"

	"github.com/midagedev/gadak/internal/apprun"
	"github.com/midagedev/gadak/internal/config"
)

// TestDesktopBootFollowsStoredDefaultWorkspace — GDK-1688. The incident
// (measured 2026-09-09): the CLI followed the home's default-workspace file
// while the app opened the root profile, which still carried an old
// config.json — two surfaces, two answers, one home. The desktop main's
// boot pick is apprun.SelectWorkspace (desktop/main.go, before any config
// read), the same single owner the CLI main calls, and Profile() consults
// the stored default whenever no flag or env selected a workspace. This test
// pins the incident shape — a live root config.json must NOT win over the
// stored default — so the two surfaces cannot drift apart again without red.
//
// No FAIL-first: the current code already resolves through the single owner
// (the premise is closed — see the track report); this is the regression
// pin for the closed defect.
func TestDesktopBootFollowsStoredDefaultWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	for _, v := range []string{"GADAK_WORKSPACE", "GADAK_PROFILE", "SCRY_PROFILE"} {
		t.Setenv(v, "")
	}
	t.Cleanup(func() {
		config.SetProfile("")
		config.ReloadWorkspaceFromEnv()
	})

	// The real workspace: profiles/work with its own config.
	config.SetProfile("work")
	workCfg := &config.Config{Site: "https://work.example.com", Email: "w@example.com", Token: "token"}
	if err := workCfg.Save(); err != nil {
		t.Fatalf("save work config: %v", err)
	}
	// The root profile with the stale credential that used to win on the app.
	config.SetProfile("")
	rootCfg := &config.Config{Site: "https://old.example.com", Email: "old@example.com", Token: "token"}
	if err := rootCfg.Save(); err != nil {
		t.Fatalf("save root config: %v", err)
	}
	// The stored default the CLI was already following.
	if err := config.SetStoredWorkspace("work"); err != nil {
		t.Fatalf("store default workspace: %v", err)
	}

	// The desktop main's exact boot pick (desktop/main.go).
	apprun.SelectWorkspace()

	if got := config.Profile(); got != "work" {
		t.Fatalf("boot profile = %q, want the stored default work", got)
	}
	if kind, _ := config.WorkspaceSource(); kind != config.SourceStored {
		t.Fatalf("workspace source = %q, want %q", kind, config.SourceStored)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Site != "https://work.example.com" {
		t.Fatalf("loaded site = %q, want the work profile's — the root's stale config.json must not win", cfg.Site)
	}

	// The env override still outranks the file — this is what makes
	// `GADAK_PROFILE=work open -a Gadak` work (desktop/main.go's comment).
	t.Setenv("GADAK_PROFILE", "oss")
	apprun.SelectWorkspace()
	if got := config.Profile(); got != "oss" {
		t.Fatalf("env override: profile = %q, want oss", got)
	}
}
