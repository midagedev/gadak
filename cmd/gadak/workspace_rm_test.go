package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// rmTestHome gives the test a throwaway GADAK_HOME and resets workspace
// resolution so the stored-default file (or its absence) is what Profile()
// sees — earlier tests in this package may have left SourceFlag behind.
func rmTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.ReloadWorkspaceFromEnv()
	t.Cleanup(func() { config.SetProfile("") })
	return home
}

// seedRMWorkspace creates profiles/<name> with a config of the given kind
// ("connected" writes an empty config) and returns its absolute directory.
func seedRMWorkspace(t *testing.T, name, kind string) string {
	t.Helper()
	cfg := &config.Config{}
	seedNamedProfile(t, name, cfg, 0, 0, false)
	dir, err := config.DirFor(name)
	if err != nil {
		t.Fatal(err)
	}
	if kind == config.KindLocalOrigin {
		loaded, err := config.LoadFor(name)
		if err != nil {
			t.Fatal(err)
		}
		loaded.Kind = config.KindLocalOrigin
		if err := loaded.Save(); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// seedPersist plants a local-origin persist (origin/issuetap.db) inside dir.
func seedPersist(t *testing.T, dir string) string {
	t.Helper()
	p := origin.PersistPath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("SQLite format 3\x00fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWorkspaceRMRejectsRoot(t *testing.T) {
	home := rmTestHome(t)
	seedRMWorkspace(t, "other", config.KindConnected)

	out, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "default"}) })
	if err == nil {
		t.Fatalf("rm default: expected refusal, got success with output %q", out)
	}
	msg := err.Error()
	for _, want := range []string{"root", home, "rm -rf"} {
		if !strings.Contains(msg, want) {
			t.Errorf("root refusal should mention %q, got:\n%s", want, msg)
		}
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("home itself must survive the refusal: %v", err)
	}
}

func TestWorkspaceRMRequiresExactlyOneName(t *testing.T) {
	rmTestHome(t)
	for _, args := range [][]string{{"rm"}, {"rm", "a", "b"}, {"rm", "  "}} {
		_, err := capture(t, func() error { return cmdWorkspaces(args) })
		if err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Errorf("args %v: expected usage error, got %v", args, err)
		}
	}
}

func TestWorkspaceRMUnknownNameRefused(t *testing.T) {
	rmTestHome(t)
	_, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "ghost", "--yes", "--destroy-origin"}) })
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected refusal naming the missing workspace, got %v", err)
	}
}

func TestWorkspaceRMRejectsPathTraversal(t *testing.T) {
	home := rmTestHome(t)
	for _, name := range []string{"../sneaky", "..", "a/b", `a\b`} {
		_, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", name, "--yes", "--destroy-origin"}) })
		if err == nil {
			t.Errorf("name %q: expected refusal, got success", name)
		}
	}
	// The home itself must be untouched by every attempt above.
	if _, err := os.Stat(filepath.Join(home, "profiles")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("profiles dir state unexpected: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(home)); err != nil {
		t.Fatalf("parent of home damaged: %v", err)
	}
}

func TestWorkspaceRMConnectedRequiresYes(t *testing.T) {
	rmTestHome(t)
	dir := seedRMWorkspace(t, "w1", config.KindConnected)

	_, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "w1"}) })
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), "origin") {
		t.Errorf("--yes refusal should state the origin is untouched, got:\n%s", err.Error())
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("workspace removed without --yes: %v", err)
	}
}

func TestWorkspaceRMConnectedRemovesDirAndKeepsOriginClaim(t *testing.T) {
	rmTestHome(t)
	dir := seedRMWorkspace(t, "w2", config.KindConnected)

	out, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "w2", "--yes"}) })
	if err != nil {
		t.Fatalf("rm --yes: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("profile dir should be gone, stat err = %v", err)
	}
	for _, want := range []string{"w2", dir, "connected"} {
		if !strings.Contains(out, want) {
			t.Errorf("success output should mention %q, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "untouched") {
		t.Errorf("success output should state the origin is untouched, got:\n%s", out)
	}
}

func TestWorkspaceRMLocalOriginRequiresDestroyOrigin(t *testing.T) {
	rmTestHome(t)
	dir := seedRMWorkspace(t, "s1", config.KindLocalOrigin)
	persist := seedPersist(t, dir)

	// --yes alone must not be enough: persist is the only copy.
	_, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "s1", "--yes"}) })
	if err == nil {
		t.Fatal("local-origin rm with --yes only: expected refusal")
	}
	msg := err.Error()
	for _, want := range []string{"--destroy-origin", persist, "only copy"} {
		if !strings.Contains(msg, want) {
			t.Errorf("local-origin refusal should mention %q, got:\n%s", want, msg)
		}
	}
	if _, err := os.Stat(persist); err != nil {
		t.Fatalf("persist removed without --destroy-origin: %v", err)
	}

	// Both flags: removal succeeds and takes the persist with it.
	out, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "s1", "--yes", "--destroy-origin"}) })
	if err != nil {
		t.Fatalf("rm --yes --destroy-origin: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("profile dir should be gone, stat err = %v", err)
	}
	if !strings.Contains(out, "standalone") {
		t.Errorf("success output should name the local-origin origin destruction, got:\n%s", out)
	}
}

func TestWorkspaceRMLocalOriginNoPersistNeedsOnlyYes(t *testing.T) {
	rmTestHome(t)
	dir := seedRMWorkspace(t, "s2", config.KindLocalOrigin)
	// No persist planted: there is no origin data to protect.

	_, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "s2", "--yes"}) })
	if err != nil {
		t.Fatalf("local-origin without persist should remove with --yes alone: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("profile dir should be gone, stat err = %v", err)
	}
}

func TestWorkspaceRMUnreadableConfigRefused(t *testing.T) {
	rmTestHome(t)
	dir := seedRMWorkspace(t, "broken", config.KindLocalOrigin) // kind unknowable below
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "broken", "--yes", "--destroy-origin"}) })
	if err == nil {
		t.Fatal("unreadable config: expected refusal, got success")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Errorf("refusal should say the kind cannot be determined, got:\n%s", err.Error())
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("broken workspace must not be removed: %v", err)
	}
}

func TestWorkspaceRMJSONShape(t *testing.T) {
	rmTestHome(t)
	seedRMWorkspace(t, "w3", config.KindConnected)
	dir := seedRMWorkspace(t, "s3", config.KindLocalOrigin)
	seedPersist(t, dir)

	out, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "w3", "--yes", "--json"}) })
	if err != nil {
		t.Fatalf("connected json rm: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, out)
	}
	if len(doc) != 3 || doc["removed"] != "w3" || doc["kind"] != "connected" || doc["origin_destroyed"] != false {
		t.Errorf("connected json shape wrong: %v", doc)
	}

	out, err = capture(t, func() error { return cmdWorkspaces([]string{"rm", "s3", "--yes", "--destroy-origin", "--json"}) })
	if err != nil {
		t.Fatalf("local-origin json rm: %v", err)
	}
	doc = nil
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, out)
	}
	if len(doc) != 3 || doc["removed"] != "s3" || doc["kind"] != "standalone" || doc["origin_destroyed"] != true {
		t.Errorf("local-origin json shape wrong: %v", doc)
	}
}

func TestWorkspaceRMPairedHintMentionsRevoke(t *testing.T) {
	rmTestHome(t)
	dir := seedRMWorkspace(t, "w4", config.KindConnected)
	const token = "tok-definitely-not-printed"
	rem := pairing.Remote{Endpoint: "https://192.0.2.10:7800", Token: token, Label: "phone"}
	if err := os.WriteFile(pairing.RemotePath(dir), mustJSON(t, rem), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "w4", "--yes"}) })
	if err != nil {
		t.Fatalf("paired rm: %v", err)
	}
	if !strings.Contains(out, "gadak pairing revoke phone") {
		t.Errorf("paired removal should point at `gadak pairing revoke phone`, got:\n%s", out)
	}
	if strings.Contains(out, token) {
		t.Errorf("paired removal leaked the pairing token:\n%s", out)
	}
}

func TestWorkspaceRMClearsStoredDefaultPointingAtIt(t *testing.T) {
	rmTestHome(t)
	seedRMWorkspace(t, "w5", config.KindConnected)
	if err := config.SetStoredWorkspace("w5"); err != nil {
		t.Fatal(err)
	}
	if got := config.Profile(); got != "w5" {
		t.Fatalf("setup: Profile() = %q, want w5", got)
	}

	out, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "w5", "--yes"}) })
	if err != nil {
		t.Fatalf("rm active/stored workspace: %v", err)
	}
	if !strings.Contains(out, "cleared the stored default") {
		t.Errorf("success output should report the cleared stored default, got:\n%s", out)
	}
	// Exported-API proof the stored default is really gone: re-resolving
	// with no flag/env override lands back on the root workspace.
	config.ReloadWorkspaceFromEnv()
	if got := config.Profile(); got != "" {
		t.Errorf("stored default survived removal: Profile() = %q", got)
	}
}

func TestWorkspaceRMServeWarningLine(t *testing.T) {
	rmTestHome(t)
	seedRMWorkspace(t, "w6", config.KindConnected)

	out, err := capture(t, func() error { return cmdWorkspaces([]string{"rm", "w6", "--yes"}) })
	if err != nil {
		t.Fatalf("rm: %v", err)
	}
	if !strings.Contains(out, "serve") || !strings.Contains(out, "restart") {
		t.Errorf("success output should warn about a running serve, got:\n%s", out)
	}
}

func TestWorkspaceRMProfilesAliasWorks(t *testing.T) {
	rmTestHome(t)
	dir := seedRMWorkspace(t, "p1", config.KindConnected)

	out, err := capture(t, func() error { return cmdProfiles([]string{"rm", "p1", "--yes"}) })
	if err != nil {
		t.Fatalf("profiles rm alias: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("profile dir should be gone, stat err = %v", err)
	}
	if !strings.Contains(out, "p1") {
		t.Errorf("alias success output should name the workspace, got:\n%s", out)
	}
}

func TestWorkspaceSingularRMRoutes(t *testing.T) {
	rmTestHome(t)
	dir := seedRMWorkspace(t, "s1", config.KindConnected)

	// `workspace rm` (singular) is a guessable spelling of `workspaces rm`;
	// it must route before the singular command's own flags parse, or --yes
	// dies as a mistyped flag.
	out, err := capture(t, func() error { return cmdWorkspace([]string{"rm", "s1", "--yes"}) })
	if err != nil {
		t.Fatalf("workspace rm route: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("profile dir should be gone, stat err = %v", err)
	}
	if !strings.Contains(out, "s1") {
		t.Errorf("success output should name the workspace, got:\n%s", out)
	}
}

func TestWorkspaceListUnchangedByRMVerb(t *testing.T) {
	rmTestHome(t)
	seedRMWorkspace(t, "w7", config.KindConnected)

	// Bare invocation and --json keep listing — the rm branch must not
	// change the existing surface.
	out, err := capture(t, func() error { return cmdWorkspaces(nil) })
	if err != nil {
		t.Fatalf("bare list: %v", err)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "w7") {
		t.Errorf("bare list output lost its table:\n%s", out)
	}
	out, err = capture(t, func() error { return cmdWorkspaces([]string{"--json"}) })
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var inv profileInventory
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("list --json no longer decodes as an inventory: %v", err)
	}
	found := false
	for _, p := range inv.Profiles {
		if p.Name == "w7" {
			found = true
		}
	}
	if !found {
		t.Errorf("list --json lost profile w7: %s", out)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
