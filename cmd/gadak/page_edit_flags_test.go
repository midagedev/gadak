package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/adf"
)

// TestCmdPageEditVersionFlagParses is FAIL-first for GDK-408: --version must
// be a registered flag so the error is "nothing to change", not unknown flag.
// Stops before origin (no title / -m / --adf-file).
func TestCmdPageEditVersionFlagParses(t *testing.T) {
	err := cmdPageEdit([]string{"12345", "--version", "3"})
	if err == nil {
		t.Fatal("expected a usage error (nothing to change), got success")
	}
	if strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--version must be registered on page edit, got %v", err)
	}
	if !strings.Contains(err.Error(), "nothing to change") {
		t.Fatalf("got %v, want nothing to change", err)
	}
}

// TestCmdPageEditRequiresPageID is an offline input-validation path: --version
// and --title are not enough without the page id positional.
func TestCmdPageEditRequiresPageID(t *testing.T) {
	err := cmdPageEdit([]string{"--title", "Renamed", "--version", "2"})
	if err == nil {
		t.Fatal("expected exactly-one-id error")
	}
	if !strings.Contains(err.Error(), "exactly one page id") {
		t.Fatalf("got %v, want exactly one page id", err)
	}
}

// Same fixtures as internal/server/write_test.go wikiSimpleADF / wikiComplexADF
// so CLI and REST judge the same documents.
const (
	cliWikiSimpleADF  = `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"plain"}]}]}`
	cliWikiComplexADF = `{"type":"doc","version":1,"content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Steps"}]}]}`
)

// cliWikiOrigin is a Confluence stand-in for page-edit CLI tests. GET returns
// a seeded ADF body; PUT increments puts so a format-loss refusal can prove
// the origin write never happened.
type cliWikiOrigin struct {
	*httptest.Server
	pages map[string]cliWikiPage
	puts  int
}

type cliWikiPage struct {
	Title string
	ADF   string
	Ver   int
}

func newCLIWikiOrigin(t *testing.T) *cliWikiOrigin {
	t.Helper()
	w := &cliWikiOrigin{
		pages: map[string]cliWikiPage{
			"100": {Title: "Rich page", ADF: cliWikiComplexADF, Ver: 5},
			"200": {Title: "Plain page", ADF: cliWikiSimpleADF, Ver: 1},
		},
	}
	w.Server = httptest.NewServer(http.HandlerFunc(w.route))
	t.Cleanup(w.Close)
	return w
}

func (w *cliWikiOrigin) route(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	const prefix = "/wiki/rest/api/content/"
	if !strings.HasPrefix(path, prefix) {
		http.NotFound(rw, r)
		return
	}
	id := strings.TrimPrefix(path, prefix)
	if slash := strings.IndexByte(id, '/'); slash >= 0 {
		id = id[:slash]
	}
	pg, ok := w.pages[id]
	if !ok {
		http.NotFound(rw, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(rw).Encode(cliWikiFullPage(id, pg))
	case http.MethodPut:
		w.puts++
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		if title, _ := payload["title"].(string); title != "" {
			pg.Title = title
		}
		if bodyMap, _ := payload["body"].(map[string]any); bodyMap != nil {
			if adf, _ := bodyMap["atlas_doc_format"].(map[string]any); adf != nil {
				if v, _ := adf["value"].(string); v != "" {
					pg.ADF = v
				}
			}
		}
		if ver, _ := payload["version"].(map[string]any); ver != nil {
			if n, ok := ver["number"].(float64); ok {
				pg.Ver = int(n)
			}
		}
		w.pages[id] = pg
		_ = json.NewEncoder(rw).Encode(cliWikiFullPage(id, pg))
	default:
		http.NotFound(rw, r)
	}
}

func cliWikiFullPage(id string, pg cliWikiPage) map[string]any {
	return map[string]any{
		"id": id, "type": "page", "status": "current", "title": pg.Title,
		"space": map[string]any{"key": "PROD", "name": "Product"},
		"version": map[string]any{
			"number": pg.Ver, "when": "2026-08-05T15:00:00.000Z",
			"by": map[string]any{"accountId": "acc-1", "displayName": "Ada"},
		},
		"body": map[string]any{
			"atlas_doc_format": map[string]any{
				"value":          pg.ADF,
				"representation": "atlas_doc_format",
			},
		},
		"ancestors": []map[string]any{},
	}
}

func TestCmdPageEditHelpListsForce(t *testing.T) {
	out, err := capture(t, func() error { return cmdPageEdit([]string{"--help"}) })
	if err != nil {
		t.Fatalf("page edit --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--force") {
		t.Fatalf("FlagSet help missing --force:\n%s", out)
	}
	if !strings.Contains(out, "--adf-file") {
		t.Fatalf("FlagSet help missing --adf-file:\n%s", out)
	}
	t.Log(out)
}

// TestCmdPageEditPlainTextRefusesRichADF is FAIL-first for GDK-682: -m on a
// heading/list/marks page must refuse and must not PUT the origin (REST
// 409 format_loss). The current CLI overwrites.
func TestCmdPageEditPlainTextRefusesRichADF(t *testing.T) {
	wo := newCLIWikiOrigin(t)
	mirror(t, wo.URL)

	_, err := capture(t, func() error {
		return cmdPageEdit([]string{"100", "-m", "hello"})
	})
	if err == nil {
		t.Fatal("expected a rich-page refusal, got success")
	}
	msg := err.Error()
	for _, want := range []string{"--adf-file", "--force"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q: %v", want, err)
		}
	}
	if wo.puts != 0 {
		t.Fatalf("origin UpdatePage was called %d times (format-loss must not write)", wo.puts)
	}
	t.Log(msg)
}

// TestCmdPageEditForceReplacesRichADF: --force is the documented escape hatch,
// matching REST force:true. PUT must happen.
func TestCmdPageEditForceReplacesRichADF(t *testing.T) {
	wo := newCLIWikiOrigin(t)
	mirror(t, wo.URL)

	_, _ = capture(t, func() error {
		return cmdPageEdit([]string{"100", "-m", "hello", "--force"})
	})
	if wo.puts == 0 {
		t.Fatal("--force must reach origin PUT")
	}
}

// TestCmdPageEditPlainTextAllowsSimpleADF: a paragraph-only page is not
// format loss; -m without --force still writes.
func TestCmdPageEditPlainTextAllowsSimpleADF(t *testing.T) {
	wo := newCLIWikiOrigin(t)
	mirror(t, wo.URL)

	_, _ = capture(t, func() error {
		return cmdPageEdit([]string{"200", "-m", "hello"})
	})
	if wo.puts == 0 {
		t.Fatal("simple ADF -m must reach origin PUT")
	}
}

// TestCmdPageEditAppendKeepsRichAndAdds: --append grafts paragraphs onto
// the current body instead of replacing it, so a rich page keeps its
// formatting — that is the point of the flag. PUT must happen and the
// stored ADF must hold both the old heading and the new paragraph.
func TestCmdPageEditAppendKeepsRichAndAdds(t *testing.T) {
	wo := newCLIWikiOrigin(t)
	mirror(t, wo.URL)

	out, err := capture(t, func() error {
		return cmdPageEdit([]string{"100", "--append", "-m", "follow-up note"})
	})
	if err != nil {
		t.Fatalf("page edit --append on a rich page: %v\n%s", err, out)
	}
	if wo.puts != 1 {
		t.Fatalf("--append must PUT exactly once, got %d", wo.puts)
	}
	stored := wo.pages["100"].ADF
	for _, want := range []string{"Steps", "follow-up note", `"heading"`, `"paragraph"`} {
		if !strings.Contains(stored, want) {
			t.Errorf("stored ADF missing %q after append:\n%s", want, stored)
		}
	}
	if adf.PlainText(json.RawMessage(stored)) == "" {
		t.Errorf("stored ADF lost its text:\n%s", stored)
	}
}

// TestCmdPageEditAppendRefusesAdfFile: --append's merge is defined for
// plain-text paragraphs; an arbitrary ADF document has no append semantics,
// so the combination is refused before any PUT.
func TestCmdPageEditAppendRefusesAdfFile(t *testing.T) {
	wo := newCLIWikiOrigin(t)
	mirror(t, wo.URL)

	f := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(f, []byte(cliWikiSimpleADF), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error {
		return cmdPageEdit([]string{"100", "--append", "-m", "note", "--adf-file", f})
	})
	if err == nil {
		t.Fatal("--append + --adf-file must refuse")
	}
	if !strings.Contains(err.Error(), "--adf-file") {
		t.Errorf("got %v", err)
	}
	if wo.puts != 0 {
		t.Fatalf("refusal must not PUT, got %d", wo.puts)
	}
}

// TestCmdPageEditAppendNeedsText: --append names a mode, not a payload —
// without -m there is nothing to append.
func TestCmdPageEditAppendNeedsText(t *testing.T) {
	wo := newCLIWikiOrigin(t)
	mirror(t, wo.URL)

	_, err := capture(t, func() error {
		return cmdPageEdit([]string{"100", "--append"})
	})
	if err == nil {
		t.Fatal("--append without -m must refuse")
	}
	if !strings.Contains(err.Error(), "-m") {
		t.Errorf("got %v", err)
	}
	if wo.puts != 0 {
		t.Fatalf("refusal must not PUT, got %d", wo.puts)
	}
}
