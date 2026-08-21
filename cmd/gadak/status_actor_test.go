package main

// `gadak status` shows the resolved actor (GDK-586): the one place an
// agent checks that its identity was recognized before writing to a
// standalone or paired origin.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func statusActorHome(t *testing.T) {
	t.Helper()
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Setenv("GADAK_ACTOR", "")
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	config.SetProfile("")
}

func TestStatusShowsEnvActor(t *testing.T) {
	statusActorHome(t)
	t.Setenv("GADAK_ACTOR", "claude:354bff2b|Claude (build 1)")

	out, err := capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "actor") || !strings.Contains(out, "claude:354bff2b") {
		t.Fatalf("text status must carry the actor slug:\n%s", out)
	}
	if !strings.Contains(out, "(env)") {
		t.Fatalf("text status must name the source rung:\n%s", out)
	}

	out, err = capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	var doc struct {
		Actor config.ResolvedActor `json:"actor"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Actor.Slug != "claude:354bff2b" || doc.Actor.Name != "Claude (build 1)" || doc.Actor.Source != config.ActorSourceEnv {
		t.Fatalf("actor %+v", doc.Actor)
	}
}

func TestStatusShowsAutoDetectedActor(t *testing.T) {
	statusActorHome(t)
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "e3e6a49a-1382-4502-9381-2c89d3234d74")

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	var doc struct {
		Actor *config.ResolvedActor `json:"actor"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Actor == nil || doc.Actor.Slug != "claude:e3e6a49a" || doc.Actor.Source != config.ActorSourceAuto {
		t.Fatalf("actor %+v, want auto-detected claude:e3e6a49a", doc.Actor)
	}
}

func TestStatusOmitsActorWhenUnresolved(t *testing.T) {
	statusActorHome(t)
	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	if strings.Contains(out, "actor") {
		t.Fatalf("no actor resolved; status must not claim one:\n%s", out)
	}
}
