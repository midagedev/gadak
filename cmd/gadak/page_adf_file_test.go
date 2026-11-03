package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// GDK-2066: `gadak page --help` teaches `--adf-file` for create, edit and
// comment, and page edit plus the issue verbs take that name — but page
// create and page comment registered only `--doc-file`, so the spelling the
// binary's own help printed was refused with "unknown flag --adf-file".
// Both names are accepted now; two different files under the two names are
// refused instead of one being picked.
//
// FAIL-first (2026-11-03, before foldDocFileAlias): page create --adf-file
// → "unknown flag --adf-file — accepted: --doc-file, --json, -m, --parent,
// --space, --title".
func TestPageCreateAndCommentAcceptAdfFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body := filepath.Join(dir, "body.json")
	if err := os.WriteFile(body, []byte(cliWikiComplexADF), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.json")
	if err := os.WriteFile(other, []byte(cliWikiSimpleADF), 0o644); err != nil {
		t.Fatal(err)
	}

	create := func(flag string) string {
		t.Helper()
		out, err := capture(t, func() error {
			return cmdPage([]string{"create", "--space", origin.DefaultSpaceKey, "--title", "Page via " + flag, flag, body})
		})
		if err != nil {
			t.Fatalf("page create %s: %v\n%s", flag, err, out)
		}
		id := strings.SplitN(strings.TrimSpace(out), "\t", 2)[0]
		if id == "" {
			t.Fatalf("page create %s printed no id: %q", flag, out)
		}
		stored, err := capture(t, func() error { return cmdPage([]string{"get", id, "--storage"}) })
		if err != nil {
			t.Fatalf("page get --storage: %v", err)
		}
		if got := strings.TrimSpace(stored); got != cliWikiComplexADF {
			t.Fatalf("page created with %s stored %s, want the file's document", flag, got)
		}
		return id
	}
	id := create("--adf-file")
	create("--doc-file")

	for _, flag := range []string{"--adf-file", "--doc-file"} {
		if out, err := capture(t, func() error { return cmdPage([]string{"comment", id, flag, body}) }); err != nil {
			t.Fatalf("page comment %s: %v\n%s", flag, err, out)
		}
	}

	_, err := capture(t, func() error {
		return cmdPage([]string{"create", "--space", origin.DefaultSpaceKey, "--title", "Both", "--adf-file", body, "--doc-file", other})
	})
	if err == nil || !strings.Contains(err.Error(), "pass one") {
		t.Fatalf("two different files under --adf-file and --doc-file must be refused, got %v", err)
	}
}
