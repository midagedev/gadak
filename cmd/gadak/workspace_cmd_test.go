package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

func TestWorkspaceCommandsRegistered(t *testing.T) {
	if commands["workspace"] == nil {
		t.Fatal("commands[workspace] is nil")
	}
	if commands["workspaces"] == nil {
		t.Fatal("commands[workspaces] is nil")
	}
	if commands["profiles"] == nil {
		t.Fatal("commands[profiles] is nil")
	}
}

func TestWorkspacesAliasMatchesProfiles(t *testing.T) {
	if commands["workspaces"] == nil {
		t.Fatal("commands[workspaces] is nil")
	}
	profilesFixture(t)
	config.SetProfile("demo")
	a, err := capture(t, func() error { return cmdProfiles(nil) })
	if err != nil {
		t.Fatalf("profiles: %v\n%s", err, a)
	}
	b, err := capture(t, func() error { return commands["workspaces"](nil) })
	if err != nil {
		t.Fatalf("workspaces: %v\n%s", err, b)
	}
	if a != b {
		t.Fatalf("workspaces output differed from profiles\nprofiles:\n%s\nworkspaces:\n%s", a, b)
	}
}

func TestStatusJSONWorkspaceAndSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{Kind: config.KindStandalone}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if _, ok := doc["profile"]; !ok {
		t.Fatalf("status --json dropped profile: %s", out)
	}
	if doc["workspace"] != "default" {
		t.Fatalf("workspace = %v, want default; body %s", doc["workspace"], out)
	}
	if doc["workspace_source"] != config.SourceFlag {
		t.Fatalf("workspace_source = %v, want flag (SetProfile); body %s", doc["workspace_source"], out)
	}
}

func TestDoctorJSONWorkspaceSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{Kind: config.KindStandalone}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	var doc struct {
		Profile         string `json:"profile"`
		WorkspaceSource string `json:"workspace_source"`
		Workspace       struct {
			Name         string `json:"name"`
			Kind         string `json:"kind"`
			HasSiteToken bool   `json:"has_site_token"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, raw)
	}
	if doc.Profile != "default" {
		t.Fatalf("profile = %q, want default", doc.Profile)
	}
	if doc.WorkspaceSource != config.SourceFlag {
		t.Fatalf("workspace_source = %q, want flag", doc.WorkspaceSource)
	}
	if doc.Workspace.Name != "default" {
		t.Fatalf("workspace.name = %q, want default", doc.Workspace.Name)
	}
	if doc.Workspace.Kind != config.KindStandalone {
		t.Fatalf("workspace.kind = %q", doc.Workspace.Kind)
	}
	if doc.Workspace.HasSiteToken {
		t.Fatal("standalone with no site token must keep has_site_token=false")
	}
}

func TestInitJSONWorkspaceAndSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	out, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) })
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if doc["profile"] != "default" {
		t.Fatalf("profile = %v, want default", doc["profile"])
	}
	if doc["workspace"] != "default" {
		t.Fatalf("workspace = %v, want default", doc["workspace"])
	}
	if doc["workspace_source"] != config.SourceFlag {
		t.Fatalf("workspace_source = %v, want flag", doc["workspace_source"])
	}
}

func TestProfilesJSONWorkspaceAndSource(t *testing.T) {
	profilesFixture(t)
	config.SetProfile("work")
	out, err := capture(t, func() error { return cmdProfiles([]string{"--json"}) })
	if err != nil {
		t.Fatalf("profiles --json: %v\n%s", err, out)
	}
	var inv struct {
		Active          string `json:"active"`
		Workspace       string `json:"workspace"`
		WorkspaceSource string `json:"workspace_source"`
	}
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if inv.Active != "work" {
		t.Fatalf("active = %q", inv.Active)
	}
	if inv.Workspace != "work" {
		t.Fatalf("workspace = %q, want work", inv.Workspace)
	}
	if inv.WorkspaceSource != config.SourceFlag {
		t.Fatalf("workspace_source = %q, want flag", inv.WorkspaceSource)
	}
}

func TestParseGlobalWorkspaceFlagParity(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Cleanup(func() { config.SetProfile("") })
	clearWorkspaceEnv(t)
	config.ReloadWorkspaceFromEnv()

	for _, args := range [][]string{
		{"--workspace", "oss", "status"},
		{"-w", "oss", "status"},
		{"--profile", "oss", "status"},
		{"-p", "oss", "status"},
	} {
		config.ReloadWorkspaceFromEnv()
		rest, err := parseGlobalWorkspace(args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if got := strings.Join(rest, " "); got != "status" {
			t.Fatalf("%v rest = %q", args, got)
		}
		if config.Profile() != "oss" {
			t.Fatalf("%v Profile() = %q", args, config.Profile())
		}
		kind, envName := config.WorkspaceSource()
		if kind != config.SourceFlag {
			t.Fatalf("%v source = %q %q, want flag", args, kind, envName)
		}
	}
}

func TestParseGlobalWorkspaceConflict(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Cleanup(func() { config.SetProfile("") })
	clearWorkspaceEnv(t)
	config.ReloadWorkspaceFromEnv()

	_, err := parseGlobalWorkspace([]string{"--workspace", "a", "--profile", "b", "status"})
	if err == nil {
		t.Fatal("expected error when --workspace and --profile both given")
	}
	if exitStatus(err) != 64 {
		t.Fatalf("exitStatus=%d want 64; err=%v", exitStatus(err), err)
	}
	if !strings.Contains(err.Error(), "two workspaces") {
		t.Fatalf("error should name the conflict, got %v", err)
	}
}

func TestWorkspaceEnvPrecedence(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Cleanup(func() { config.SetProfile("") })
	t.Setenv("GADAK_WORKSPACE", "ws")
	t.Setenv("GADAK_PROFILE", "pf")
	t.Setenv("SCRY_PROFILE", "scry")
	config.ReloadWorkspaceFromEnv()
	if config.Profile() != "ws" {
		t.Fatalf("Profile() = %q, want ws (GADAK_WORKSPACE wins)", config.Profile())
	}
	kind, envName := config.WorkspaceSource()
	if kind != config.SourceEnv || envName != "GADAK_WORKSPACE" {
		t.Fatalf("source = %q %q, want env GADAK_WORKSPACE", kind, envName)
	}

	t.Setenv("GADAK_WORKSPACE", "")
	config.ReloadWorkspaceFromEnv()
	if config.Profile() != "pf" {
		t.Fatalf("Profile() = %q, want pf (GADAK_PROFILE fallback)", config.Profile())
	}
	kind, envName = config.WorkspaceSource()
	if kind != config.SourceEnv || envName != "GADAK_PROFILE" {
		t.Fatalf("source = %q %q, want env GADAK_PROFILE", kind, envName)
	}

	t.Setenv("GADAK_PROFILE", "")
	config.ReloadWorkspaceFromEnv()
	if config.Profile() != "scry" {
		t.Fatalf("Profile() = %q, want scry (SCRY_PROFILE fallback)", config.Profile())
	}
	kind, envName = config.WorkspaceSource()
	if kind != config.SourceEnv || envName != "SCRY_PROFILE" {
		t.Fatalf("source = %q %q, want env SCRY_PROFILE", kind, envName)
	}

	t.Setenv("SCRY_PROFILE", "")
	config.ReloadWorkspaceFromEnv()
	if config.Profile() != "" {
		t.Fatalf("Profile() = %q, want empty default", config.Profile())
	}
	kind, envName = config.WorkspaceSource()
	if kind != config.SourceDefault || envName != "" {
		t.Fatalf("source = %q %q, want default", kind, envName)
	}
}

func TestWarnWorkspaceIfEnvOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	t.Cleanup(func() { config.SetProfile("") })

	// env + named → one stderr line, stdout clean
	t.Setenv("GADAK_PROFILE", "oss")
	t.Setenv("GADAK_WORKSPACE", "")
	t.Setenv("SCRY_PROFILE", "")
	config.ReloadWorkspaceFromEnv()
	stdout, stderr, err := captureBoth(t, func() error {
		return cmdCreate([]string{"hello"})
	})
	_ = err
	if !strings.Contains(stderr, "warning: workspace: oss (from GADAK_PROFILE)") {
		t.Fatalf("env write must disclose workspace on stderr, got stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
	if strings.Contains(stdout, "warning:") || strings.Contains(stdout, "workspace:") {
		t.Fatalf("disclosure leaked onto stdout: %q", stdout)
	}

	// flag → silent
	config.SetProfile("oss")
	stdout, stderr, err = captureBoth(t, func() error {
		return cmdCreate([]string{"hello"})
	})
	_ = err
	if strings.Contains(stderr, "warning: workspace:") {
		t.Fatalf("flag source must not disclose, stderr=%q", stderr)
	}

	// root (default) even from env GADAK_PROFILE=default → Profile() empty → silent
	t.Setenv("GADAK_PROFILE", "default")
	config.ReloadWorkspaceFromEnv()
	stdout, stderr, err = captureBoth(t, func() error {
		return cmdCreate([]string{"hello"})
	})
	_ = err
	if strings.Contains(stderr, "warning: workspace:") {
		t.Fatalf("root must not disclose, stderr=%q", stderr)
	}

	// SetProfile("") is flag+root → silent
	config.SetProfile("")
	stdout, stderr, err = captureBoth(t, func() error {
		return cmdCreate([]string{"hello"})
	})
	_ = err
	if strings.Contains(stderr, "warning: workspace:") {
		t.Fatalf("SetProfile empty must not disclose, stderr=%q", stderr)
	}
}

func TestCmdWorkspaceShowsSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	clearWorkspaceEnv(t)
	config.SetProfile("")

	seedNamedProfile(t, "", &config.Config{Kind: config.KindStandalone}, 0, 0, false)
	seedNamedProfile(t, "oss", &config.Config{Kind: config.KindStandalone}, 0, 0, false)

	out, err := capture(t, func() error { return cmdWorkspace(nil) })
	if err != nil {
		t.Fatalf("workspace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "default") {
		t.Fatalf("missing name:\n%s", out)
	}
	if !strings.Contains(out, config.SourceFlag) && !strings.Contains(out, "flag") {
		t.Fatalf("missing source:\n%s", out)
	}
	if !strings.Contains(out, "export GADAK_WORKSPACE=") {
		t.Fatalf("missing export hint:\n%s", out)
	}
	if !strings.Contains(out, "oss") {
		t.Fatalf("missing other workspace oss:\n%s", out)
	}

	config.SetProfile("oss")
	jout, err := capture(t, func() error { return cmdWorkspace([]string{"--json"}) })
	if err != nil {
		t.Fatalf("workspace --json: %v\n%s", err, jout)
	}
	var doc struct {
		Workspace       string   `json:"workspace"`
		WorkspaceSource string   `json:"workspace_source"`
		Kind            string   `json:"kind"`
		Persist         string   `json:"persist"`
		Others          []string `json:"others"`
	}
	if err := json.Unmarshal([]byte(jout), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, jout)
	}
	if doc.Workspace != "oss" {
		t.Fatalf("workspace = %q", doc.Workspace)
	}
	if doc.WorkspaceSource != config.SourceFlag {
		t.Fatalf("workspace_source = %q", doc.WorkspaceSource)
	}
	if doc.Kind == "" {
		t.Fatal("kind empty")
	}
	if doc.Persist == "" {
		t.Fatal("persist empty")
	}
	foundDefault := false
	for _, n := range doc.Others {
		if n == "default" {
			foundDefault = true
		}
		if n == "oss" {
			t.Fatalf("others still lists the active workspace: %v", doc.Others)
		}
	}
	if !foundDefault {
		t.Fatalf("others missing default: %v", doc.Others)
	}
}

func TestCmdWorkspaceUseStoresDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	clearWorkspaceEnv(t)
	config.SetProfile("")
	config.ReloadWorkspaceFromEnv()

	seedNamedProfile(t, "", &config.Config{Kind: config.KindStandalone}, 0, 0, false)
	seedNamedProfile(t, "oss", &config.Config{Kind: config.KindStandalone}, 0, 0, false)

	out, err := capture(t, func() error { return cmdWorkspace([]string{"use", "oss"}) })
	if err != nil {
		t.Fatalf("workspace use oss: %v\n%s", err, out)
	}
	config.ReloadWorkspaceFromEnv()
	if config.Profile() != "oss" {
		t.Fatalf("after use, Profile() = %q, want oss", config.Profile())
	}
	kind, envName := config.WorkspaceSource()
	if kind != config.SourceStored || envName != "" {
		t.Fatalf("after use, source = %q %q, want stored", kind, envName)
	}

	out, err = capture(t, func() error { return cmdWorkspace(nil) })
	if err != nil {
		t.Fatalf("workspace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "oss") {
		t.Fatalf("no-arg workspace missing name oss:\n%s", out)
	}
	if !strings.Contains(out, "stored") {
		t.Fatalf("no-arg workspace missing stored source:\n%s", out)
	}

	out, err = capture(t, func() error { return cmdWorkspace([]string{"use", "--clear"}) })
	if err != nil {
		t.Fatalf("workspace use --clear: %v\n%s", err, out)
	}
	config.ReloadWorkspaceFromEnv()
	if config.Profile() != "" {
		t.Fatalf("after --clear, Profile() = %q, want root", config.Profile())
	}
	kind, envName = config.WorkspaceSource()
	if kind != config.SourceDefault {
		t.Fatalf("after --clear, source = %q, want default", kind)
	}
}

func TestCmdWorkspaceUseMissingErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	clearWorkspaceEnv(t)
	config.SetProfile("")
	config.ReloadWorkspaceFromEnv()
	seedNamedProfile(t, "demo", &config.Config{Kind: config.KindStandalone}, 0, 0, false)

	err := cmdWorkspace([]string{"use", "nosuch"})
	if err == nil {
		t.Fatal("workspace use nosuch must error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `workspace "nosuch" not found`) {
		t.Fatalf("error %q, want existing not-found format", msg)
	}
	if !strings.Contains(msg, "available: demo") {
		t.Fatalf("error %q, want available: demo", msg)
	}
	config.ReloadWorkspaceFromEnv()
	if config.Profile() != "" {
		t.Fatalf("failed use must not store, Profile() = %q", config.Profile())
	}
	if _, statErr := os.Stat(filepath.Join(home, "profiles", "nosuch")); !os.IsNotExist(statErr) {
		t.Fatalf("use must not create the dir; stat=%v", statErr)
	}
}

func TestWorkspaceUseClearWhenStoredMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	clearWorkspaceEnv(t)
	if err := os.WriteFile(filepath.Join(home, "default-workspace"), []byte("gone\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.ReloadWorkspaceFromEnv()
	if err := checkProfileForCommand("workspace", nil); err == nil {
		t.Fatal("no-arg workspace must error when stored default is missing")
	} else if !strings.Contains(err.Error(), `workspace "gone" not found`) {
		t.Fatalf("no-arg error %q, want not-found", err)
	}
	if err := checkProfileForCommand("workspace", []string{"use", "--clear"}); err != nil {
		t.Fatalf("use --clear must be reachable with a missing stored default: %v", err)
	}
}

// A home with one workspace has nothing to select, so the export hint is
// work that changes nothing — it must not be printed (lead review, GDK-490).
func TestCmdWorkspaceHidesExportHintWhenAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	clearWorkspaceEnv(t)
	config.SetProfile("")

	seedNamedProfile(t, "", &config.Config{Kind: config.KindStandalone}, 0, 0, false)

	out, err := capture(t, func() error { return cmdWorkspace(nil) })
	if err != nil {
		t.Fatalf("workspace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "(none)") {
		t.Fatalf("expected no other workspaces:\n%s", out)
	}
	if strings.Contains(out, "export GADAK_WORKSPACE") {
		t.Fatalf("export hint offered with nothing to select:\n%s", out)
	}
}

func clearWorkspaceEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GADAK_WORKSPACE", "")
	t.Setenv("GADAK_PROFILE", "")
	t.Setenv("SCRY_PROFILE", "")
	t.Setenv("SCRY_WORKSPACE", "")
}
