package main

import (
	"encoding/json"
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
