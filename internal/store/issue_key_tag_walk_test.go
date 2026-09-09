//go:build sourcelint

// Repo-wide AST gate (GDK-1144): lives behind the sourcelint tag so the
// default `go test ./...` never pays the whole-tree parse. Run with
// `bash tools/sourcelint.sh` — CI's Go-tests step runs exactly that.

package store

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/archlint"
)

// TestIssueKeyStructTagsUseHelper fails closed: a new production
// `json:"issue_key"` tag that does not go through MarshalWithIssueKeyAlias
// (or the export_static whitelist copy) is a drift hole.
func TestIssueKeyStructTagsUseHelper(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	allowed := map[string]string{
		"internal/store/read.go":     "MarshalWithIssueKeyAlias",
		"internal/store/feed.go":     "MarshalWithIssueKeyAlias",
		"internal/server/read.go":    "MarshalWithIssueKeyAlias",
		"cmd/gadak/export_static.go": `"key"`, // whitelist + scrubDetail copy
		// The terminal session's issue binding (GDK-1158) is not a mirror
		// row: it is runtime state on a PTY session, emitted by the binding
		// route. The GDK-255 alias would be a lie here — a session Info has
		// no issues_full.key to alias — so the tags are allowed without
		// MarshalWithIssueKeyAlias, pinned by the handler that owns them.
		"internal/server/terminal.go": "handleTerminalIssue",
	}
	var unexpected []string
	err := archlint.Walk(root, func(af *archlint.File) error {
		if strings.HasSuffix(af.Rel, "_test.go") {
			return nil
		}
		body, err := af.Src()
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), `json:"issue_key"`) {
			return nil
		}
		need, ok := allowed[af.Rel]
		if !ok {
			unexpected = append(unexpected, af.Rel)
			return nil
		}
		if !strings.Contains(string(body), need) {
			t.Errorf("%s has json:\"issue_key\" but does not contain %s", af.Rel, need)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(unexpected) > 0 {
		t.Errorf("new json:\"issue_key\" emit sites must use MarshalWithIssueKeyAlias (or be added to the allowed map with a reason): %s", strings.Join(unexpected, ", "))
	}
}
