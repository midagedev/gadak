package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

func TestStatusWarnsWhenTokenExpiring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	cfg := &config.Config{
		TokenExpiresAt:    time.Now().UTC().Add(5*24*time.Hour + time.Hour).Format(config.TokenTimeFormat),
		TokenExpirySource: config.TokenExpirySourceAssumed,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "API token expires in 5 days") {
		t.Fatalf("missing warning line:\n%s", out)
	}
	if !strings.Contains(out, "assumed from the default lifetime") {
		t.Fatalf("missing assumed hedge:\n%s", out)
	}
	if !strings.Contains(out, "gadak init") {
		t.Fatalf("missing remedy:\n%s", out)
	}
}

func TestStatusJSONIncludesTokenExpiry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	cfg := &config.Config{
		TokenExpiresAt:    time.Now().UTC().Add(5*24*time.Hour + time.Hour).Format(config.TokenTimeFormat),
		TokenExpirySource: config.TokenExpirySourceUser,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var doc struct {
		TokenExpiry config.TokenExpiry `json:"token_expiry"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.TokenExpiry.State != config.TokenExpiryExpiring {
		t.Fatalf("token_expiry %+v", doc.TokenExpiry)
	}
	if doc.TokenExpiry.Source != config.TokenExpirySourceUser {
		t.Fatalf("source %q", doc.TokenExpiry.Source)
	}
	if doc.TokenExpiry.Message == "" {
		t.Fatal("json message empty")
	}
}

func TestStatusSurfacesConfigLoadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := captureBoth(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status must still succeed when config is unreadable: %v", err)
	}
	if !strings.Contains(stderr, "gadak: config:") {
		t.Fatalf("stderr must name the config problem, got %q", stderr)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout json %q: %v", stdout, err)
	}
	if _, ok := doc["issues"]; !ok {
		t.Fatalf("mirror stats missing: %s", stdout)
	}
	ce, _ := doc["config_error"].(string)
	if ce == "" {
		t.Fatalf("json missing config_error: %s", stdout)
	}
	if !strings.Contains(ce, "config.json") {
		t.Fatalf("config_error must name the path, got %q", ce)
	}
}

func TestStatusJSONWikiPathStandaloneOn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{
		Kind:       config.KindStandalone,
		Confluence: &config.ConfluenceConfig{Spaces: []string{"LOC"}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var doc struct {
		Pages int `json:"pages"`
		Wiki  struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
		} `json:"wiki"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.Wiki.Path != "on" {
		t.Fatalf("wiki.path = %q, want on; body %s", doc.Wiki.Path, out)
	}
	if doc.Wiki.Reason != "" {
		t.Fatalf("wiki.reason = %q, want empty when on", doc.Wiki.Reason)
	}
	if strings.Contains(out, "token") && strings.Contains(strings.ToLower(out), "secret") {
		t.Fatalf("status leaked a credential-shaped field: %s", out)
	}
}

func TestStatusJSONWikiPathSkippedWhenNotConfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{
		Kind: config.KindStandalone,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var doc struct {
		Wiki struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
		} `json:"wiki"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.Wiki.Path != "skipped" {
		t.Fatalf("wiki.path = %q, want skipped; body %s", doc.Wiki.Path, out)
	}
	if doc.Wiki.Reason != "sync: confluence is not configured" {
		t.Fatalf("wiki.reason = %q", doc.Wiki.Reason)
	}
}

func TestStatusJSONWikiPathConnectedOn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{
		Site:       "https://example.invalid",
		Email:      "user@example.invalid",
		Token:      "status-test-token",
		Confluence: &config.ConfluenceConfig{Spaces: []string{"ENG"}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	if strings.Contains(out, "status-test-token") {
		t.Fatalf("token leaked in status --json: %s", out)
	}
	var doc struct {
		Wiki struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
		} `json:"wiki"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.Wiki.Path != "on" {
		t.Fatalf("wiki.path = %q, want on; body %s", doc.Wiki.Path, out)
	}
}
