package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func frozenHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	cfg := &config.Config{
		Frozen: true,
		Site:   "http://127.0.0.1:1",
		Email:  "a@example.invalid",
		Token:  "tok",
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestSyncFrozenFailsWithCauseAndUnfreeze(t *testing.T) {
	home := frozenHome(t)
	_, err := capture(t, func() error { return cmdSync(nil) })
	if err == nil {
		t.Fatal("gadak sync on a frozen workspace must fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "frozen") {
		t.Fatalf("missing cause: %v", err)
	}
	if !strings.Contains(msg, `config "frozen": true`) {
		t.Fatalf("missing config field: %v", err)
	}
	if !strings.Contains(msg, home) {
		t.Fatalf("missing config path %q: %v", home, err)
	}
	if !strings.Contains(msg, "gadak config set frozen false") {
		t.Fatalf("missing unfreeze hint: %v", err)
	}
}

func TestSyncUnfrozenWithoutCredentialStillNotConfigured(t *testing.T) {
	emptyHome(t)
	_, err := capture(t, func() error { return cmdSync(nil) })
	if err == nil {
		t.Fatal("unfrozen empty workspace: want ErrNotConfigured")
	}
	if !strings.Contains(err.Error(), config.ErrNotConfigured.Error()) {
		t.Fatalf("unfrozen sync error changed: %v", err)
	}
	if strings.Contains(err.Error(), "frozen") {
		t.Fatalf("unfrozen workspace used the freeze message: %v", err)
	}
}

func TestStatusReportsFrozen(t *testing.T) {
	frozenHome(t)

	out, err := capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "frozen") {
		t.Fatalf("status text missing frozen:\n%s", out)
	}

	js, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, js)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(js), &doc); err != nil {
		t.Fatalf("decode %q: %v", js, err)
	}
	if doc["frozen"] != true {
		t.Fatalf("status --json frozen=%v, want true; body %s", doc["frozen"], js)
	}
}

func TestDoctorReportsFrozen(t *testing.T) {
	frozenHome(t)

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "frozen=yes") {
		t.Fatalf("doctor text missing frozen=yes:\n%s", out)
	}

	js, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, js)
	}
	var doc struct {
		Workspace struct {
			Frozen bool `json:"frozen"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal([]byte(js), &doc); err != nil {
		t.Fatalf("decode %q: %v", js, err)
	}
	if !doc.Workspace.Frozen {
		t.Fatalf("doctor --json workspace.frozen=%v, want true; body %s", doc.Workspace.Frozen, js)
	}
}
