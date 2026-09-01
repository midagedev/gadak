package origin

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	issuetap "github.com/midagedev/issuetap"
)

// legacyYAMLBody is a pre-SQLite persist snapshot: one project, one space,
// one issue. Bytes must survive the one-shot seed (yaml is a rollback asset).
var legacyYAMLBody = []byte(`projects:
  - id: "10000"
    key: STD
    name: Local-origin
    type: software
    style: classic
spaces:
  - id: "40000"
    key: LOC
    name: Local
    type: global
issues:
  - key: STD-1
    summary: from legacy yaml
`)

func localOriginCfg(t *testing.T, home string) *config.Config {
	t.Helper()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = Close()
		config.SetProfile("")
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestIssuetapPinRefusesYAMLPersistPath is FAIL-first for the pin: an
// existing YAML file as PersistPath is refused with a FixturePath hint.
// gadak must not pass the legacy yaml as PersistPath.
func TestIssuetapPinRefusesYAMLPersistPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issuetap.yaml")
	if err := os.WriteFile(path, legacyYAMLBody, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := issuetap.NewEmbedded(issuetap.EmbeddedConfig{PersistPath: path})
	if err == nil {
		t.Fatal("expected YAML PersistPath to be refused on this issuetap pin")
	}
	if !strings.Contains(err.Error(), "FixturePath") {
		t.Fatalf("YAML persist error must name FixturePath; got %v", err)
	}
}

// TestLegacyYAMLSeedsSQLitePersist: yaml-only home → seed once into
// issuetap.db, issue survives Close+reopen, yaml bytes are unchanged.
func TestLegacyYAMLSeedsSQLitePersist(t *testing.T) {
	home := t.TempDir()
	cfg := localOriginCfg(t, home)

	yamlPath := LegacyYAMLPath(home)
	if err := os.MkdirAll(filepath.Dir(yamlPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(yamlPath, legacyYAMLBody, 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := Client(cfg)
	if err != nil {
		t.Fatalf("legacy yaml home: %v", err)
	}
	if !searchKey(t, c, "STD-1") {
		t.Fatal("STD-1 from yaml missing after seed")
	}

	ctx := context.Background()
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": DefaultProjectKey},
		"summary":   "written after yaml seed",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}

	dbPath := PersistPath(home)
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("sqlite persist missing at %s: %v", dbPath, err)
	}
	got, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, legacyYAMLBody) {
		t.Fatalf("legacy yaml mutated; want unchanged rollback asset")
	}

	c2, err := Client(cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !searchKey(t, c2, "STD-1") {
		t.Fatal("STD-1 missing after Close+Client")
	}
	if !searchKey(t, c2, key) {
		t.Fatalf("%s missing after Close+Client", key)
	}
}

// TestNewLocalOriginHomeCreatesDBNotYAML: a fresh local-origin origin writes
// issuetap.db and does not create issuetap.yaml.
func TestNewLocalOriginHomeCreatesDBNotYAML(t *testing.T) {
	home := t.TempDir()
	cfg := localOriginCfg(t, home)
	c, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": DefaultProjectKey},
		"summary":   "new home",
		"issuetype": map[string]any{"name": "Task"},
	}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(PersistPath(home))
	if err != nil {
		t.Fatalf("sqlite persist missing: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte("SQLite format 3")) {
		t.Fatalf("persist is not SQLite, first bytes %q", raw[:min(16, len(raw))])
	}
	yamlPath := LegacyYAMLPath(home)
	if _, err := os.Stat(yamlPath); !os.IsNotExist(err) {
		t.Fatalf("new home must not write %s (err=%v)", yamlPath, err)
	}
}

// TestExistingSQLiteIgnoresLegacyYAML: when issuetap.db exists, the
// sibling yaml is not applied.
func TestExistingSQLiteIgnoresLegacyYAML(t *testing.T) {
	home := t.TempDir()
	cfg := localOriginCfg(t, home)
	c, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	key, err := c.CreateIssue(context.Background(), map[string]any{
		"project":   map[string]any{"key": DefaultProjectKey},
		"summary":   "already in db",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}

	yamlPath := LegacyYAMLPath(home)
	overlay := []byte(`projects:
  - id: "20000"
    key: YAML
    name: Overlay
    type: software
    style: classic
issues:
  - key: YAML-1
    summary: from yaml overlay
`)
	if err := os.WriteFile(yamlPath, overlay, 0o600); err != nil {
		t.Fatal(err)
	}
	c2, err := Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !searchKey(t, c2, key) {
		t.Fatalf("%s missing from existing db", key)
	}
	if searchKey(t, c2, "YAML-1") {
		t.Fatal("yaml issue applied over existing sqlite persist")
	}
}
