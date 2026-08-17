package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func integrationsMux() http.Handler {
	return fallbackHandler(http.NotFoundHandler(), nil, nil, nil, newBrowseTabs(), nil)
}

func TestIntegrationsGETOrderAndDetect(t *testing.T) {
	home := t.TempDir()
	gadakHome := filepath.Join(home, ".gadak")
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", gadakHome)

	h := integrationsMux()
	get := func() []map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/desktop/integrations", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("content-type %q", ct)
		}
		var doc struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("json: %v\n%s", err, rec.Body.String())
		}
		if len(doc.Items) != 3 {
			t.Fatalf("len(items)=%d want 3: %s", len(doc.Items), rec.Body.String())
		}
		return doc.Items
	}

	items := get()
	wantIDs := []string{"raycast", "skill", "mcp-claude"}
	wantCmd := []string{"gadak raycast install", "gadak skill install claude", "gadak mcp install claude"}
	for i, id := range wantIDs {
		if items[i]["id"] != id {
			t.Fatalf("items[%d].id=%v want %s", i, items[i]["id"], id)
		}
		if items[i]["command"] != wantCmd[i] {
			t.Fatalf("items[%d].command=%v want %s", i, items[i]["command"], wantCmd[i])
		}
	}
	if items[0]["installed"] != false {
		t.Fatalf("raycast installed=%v want false", items[0]["installed"])
	}
	if items[1]["installed"] != false {
		t.Fatalf("skill installed=%v want false", items[1]["installed"])
	}
	if items[1]["prerequisite"] != nil {
		t.Fatalf("skill prerequisite=%v want null", items[1]["prerequisite"])
	}
	if items[0]["detail"] != "~/.gadak/raycast-extension" {
		t.Fatalf("raycast detail=%v", items[0]["detail"])
	}

	if err := os.MkdirAll(filepath.Join(gadakHome, "raycast-extension"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gadakHome, "raycast-extension", "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("# gadak\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	items = get()
	if items[0]["installed"] != true {
		t.Fatalf("raycast after touch: installed=%v want true", items[0]["installed"])
	}
	if items[1]["installed"] != true {
		t.Fatalf("skill after touch: installed=%v want true", items[1]["installed"])
	}
}

func TestIntegrationsPOSTStreamsOutput(t *testing.T) {
	script := filepath.Join(t.TempDir(), "gadak")
	body := "#!/bin/sh\necho \"args: $*\"\necho stderr-line >&2\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GADAK_DESKTOP_CLI", script)

	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill/install", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("content-type %q", ct)
	}
	got := rec.Body.String()
	if !strings.Contains(got, "args: skill install claude") {
		t.Fatalf("missing argv line:\n%s", got)
	}
	if !strings.Contains(got, "stderr-line") {
		t.Fatalf("stderr not streamed:\n%s", got)
	}
	if lastLine(got) != "exit=0" {
		t.Fatalf("last line %q want exit=0\n%s", lastLine(got), got)
	}
}

func TestIntegrationsPOSTUnknownID(t *testing.T) {
	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/nope/install", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d want 404 body %s", rec.Code, rec.Body.String())
	}
}

func TestIntegrationsPOSTConflict(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	release := filepath.Join(dir, "release")
	script := filepath.Join(dir, "gadak")
	body := fmt.Sprintf("#!/bin/sh\ntouch %q\nwhile [ ! -f %q ]; do sleep 0.05; done\necho done\nexit 0\n", started, release)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GADAK_DESKTOP_CLI", script)

	h := integrationsMux()
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill/install", nil))
		first <- rec
	}()

	select {
	case rec := <-first:
		t.Fatalf("first install returned too soon: %d %s", rec.Code, rec.Body.String())
	case <-time.After(50 * time.Millisecond):
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first install never created the started file")
		}
		time.Sleep(20 * time.Millisecond)
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/desktop/integrations/skill/install", nil))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("concurrent: got %d want 409 body %s", rec2.Code, rec2.Body.String())
	}

	if err := os.WriteFile(release, []byte("go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec1 := <-first
	if rec1.Code != http.StatusOK {
		t.Fatalf("first: %d %s", rec1.Code, rec1.Body.String())
	}
	if lastLine(rec1.Body.String()) != "exit=0" {
		t.Fatalf("first last line %q\n%s", lastLine(rec1.Body.String()), rec1.Body.String())
	}
}

func TestIntegrationsPOSTMissingCLI(t *testing.T) {
	t.Setenv("GADAK_DESKTOP_CLI", filepath.Join(t.TempDir(), "no-such-gadak"))
	rec := httptest.NewRecorder()
	integrationsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/desktop/integrations/raycast/install", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body.String())
	}
	if lastLine(rec.Body.String()) != "exit=127" {
		t.Fatalf("last line %q want exit=127\n%s", lastLine(rec.Body.String()), rec.Body.String())
	}
}

func lastLine(s string) string {
	s = strings.TrimRight(s, "\n")
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}
	return s
}
