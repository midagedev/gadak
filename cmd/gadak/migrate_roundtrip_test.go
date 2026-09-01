package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestMigrateLocalOriginRoundtrip is the product round-trip (GDK-1264): a
// local-origin source workspace with issues, a comment, a link pair, a
// transition, attachments (text and binary), and a wiki page migrates into
// a brand-new local-origin workspace — counts match, the link exists on both
// ends, attachment bytes survive verbatim, and the next create continues
// the key sequence instead of colliding.
func TestMigrateLocalOriginRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	allowProfileCreate = true
	t.Cleanup(func() {
		allowProfileCreate = false
		_ = origin.Close()
		config.SetProfile("")
	})

	// ── source workspace ───────────────────────────────────────────────
	config.SetProfile("src")
	if out, err := capture(t, func() error { return cmdInit([]string{"--local"}) }); err != nil {
		t.Fatalf("init src: %v\n%s", err, out)
	}
	keyA := createIssue(t, "migrate roundtrip alpha")
	keyB := createIssue(t, "migrate roundtrip beta")

	// The seed-file killer measured on the first full GDK export: a body
	// whose lines start with spaces or tabs around blank lines makes
	// yaml.v3 emit a block scalar its own parser rejects ("did not find
	// expected key"). The seed is emitted as JSON exactly for this body;
	// the migrate below fails if that regresses.
	nasty := "  leading spaces\n\n\tstarts with a tab\ntrailing spaces  \n\n"
	if out, err := capture(t, func() error {
		return cmdCreate([]string{"migrate roundtrip nasty body", "-m", nasty})
	}); err != nil {
		t.Fatalf("create nasty: %v\n%s", err, out)
	}

	if out, err := capture(t, func() error { return cmdComment([]string{keyA, "-m", "first comment"}) }); err != nil {
		t.Fatalf("comment: %v\n%s", err, out)
	}
	if out, err := capture(t, func() error { return cmdLink([]string{keyA, keyB, "--type", "blocks"}) }); err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}
	if out, err := capture(t, func() error { return cmdTransition([]string{keyA, "In Progress"}) }); err != nil {
		t.Fatalf("transition: %v\n%s", err, out)
	}

	txt := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(txt, []byte("plain text attachment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	binBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0xFF, 0x10, 0x0B}
	bin := filepath.Join(filepath.Dir(txt), "blob.bin")
	if err := os.WriteFile(bin, binBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := capture(t, func() error { return cmdAttach([]string{keyA, txt, bin}) }); err != nil {
		t.Fatalf("attach: %v\n%s", err, out)
	}

	if out, err := capture(t, func() error {
		return cmdPage([]string{"create", "--space", origin.DefaultSpaceKey, "--title", "migrate note", "-m", "hello page"})
	}); err != nil {
		t.Fatalf("page create: %v\n%s", err, out)
	}

	if out, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync src: %v\n%s", err, out)
	}

	// ── migrate into a fresh workspace ─────────────────────────────────
	config.SetProfile("dst")
	report, err := capture(t, func() error { return cmdMigrate([]string{"--from", "src"}) })
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, report)
	}
	if strings.Contains(report, "MISMATCH") {
		t.Fatalf("verification mismatch:\n%s", report)
	}
	for _, want := range []string{"issues", "comments", "attachments", "links", "history", "pages"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}

	// ── target assertions ──────────────────────────────────────────────
	sqlOut := dstSQL(t, "select key, status_category from issues_full order by key")
	if !strings.Contains(sqlOut, keyA+"\tinprogress") || !strings.Contains(sqlOut, keyB) {
		t.Fatalf("issues in target:\n%s", sqlOut)
	}
	if got := dstSQL(t, "select count(*) from links"); strings.TrimSpace(got) != "2" {
		t.Fatalf("link rows in target (want both projections): %q", got)
	}
	if got := dstSQL(t, "select count(*) from pages"); strings.TrimSpace(got) != "1" {
		t.Fatalf("pages in target: %q", got)
	}
	if got := dstSQL(t, "select count(*) from changelog c join issues_full i on i.item_id = c.item_id where i.key = '"+keyA+"' and c.field = 'status'"); strings.TrimSpace(got) == "0" {
		t.Fatalf("migrated history missing for %s", keyA)
	}

	// Attachment bytes survive verbatim through the fixture.
	id := strings.TrimSpace(dstSQL(t,
		"select coalesce(nullif(external_id,''), id) from attachments a join issues_full i on i.item_id = a.item_id where a.filename = 'blob.bin'"))
	if id == "" {
		t.Fatal("binary attachment missing from target mirror")
	}
	dstCfg, err := config.LoadFor("dst")
	if err != nil {
		t.Fatal(err)
	}
	client, err := origin.Client(dstCfg)
	if err != nil {
		t.Fatalf("target origin: %v", err)
	}
	status, body, err := client.Raw(context.Background(), "GET",
		"/rest/api/3/attachment/content/"+url.PathEscape(id), nil, false)
	if err != nil || status != 200 {
		t.Fatalf("attachment content: status=%d err=%v", status, err)
	}
	if string(body) != string(binBytes) {
		t.Fatalf("attachment bytes differ: got %x want %x", body, binBytes)
	}

	// The text/plain file rides the fixture's `text:` field, a different
	// path from dataBase64 — prove it serves byte-identical too (a string
	// re-encode that normalized the newline would pass every count).
	txtID := strings.TrimSpace(dstSQL(t,
		"select coalesce(nullif(external_id,''), id) from attachments a join issues_full i on i.item_id = a.item_id where a.filename = 'notes.txt'"))
	if txtID == "" {
		t.Fatal("text attachment missing from target mirror")
	}
	status, body, err = client.Raw(context.Background(), "GET",
		"/rest/api/3/attachment/content/"+url.PathEscape(txtID), nil, false)
	if err != nil || status != 200 {
		t.Fatalf("text attachment content: status=%d err=%v", status, err)
	}
	if string(body) != "plain text attachment\n" {
		t.Fatalf("text attachment bytes differ: %q", body)
	}

	// The key sequence continues where the source left off.
	next := createIssue(t, "post-migrate issue")
	if next != "STD-4" {
		t.Fatalf("next key after migrate = %q, want STD-4", next)
	}
}

func createIssue(t *testing.T, summary string) string {
	t.Helper()
	out, err := capture(t, func() error { return cmdCreate([]string{summary}) })
	if err != nil {
		t.Fatalf("create %q: %v\n%s", summary, err, out)
	}
	first := strings.TrimSpace(strings.Split(out, "\n")[0])
	key := strings.Split(first, "\t")[0]
	if key == "" {
		t.Fatalf("no key in create output:\n%s", out)
	}
	return key
}

func dstSQL(t *testing.T, q string) string {
	t.Helper()
	out, err := capture(t, func() error { return cmdSQL([]string{"--no-header", q}) })
	if err != nil {
		t.Fatalf("sql %q: %v\n%s", q, err, out)
	}
	return out
}

// TestMigrateRefusals pins the guard rails: a target that already exists, a
// missing --from, and migrating a workspace onto itself are refusals, not
// silent workspace writes.
func TestMigrateRefusals(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	allowProfileCreate = true
	t.Cleanup(func() {
		allowProfileCreate = false
		_ = origin.Close()
		config.SetProfile("")
	})

	config.SetProfile("src")
	if out, err := capture(t, func() error { return cmdInit([]string{"--local"}) }); err != nil {
		t.Fatalf("init src: %v\n%s", err, out)
	}

	// Root target (no --workspace) is refused: migrate creates a workspace.
	config.SetProfile("")
	if _, err := capture(t, func() error { return cmdMigrate([]string{"--from", "src"}) }); err == nil ||
		!strings.Contains(err.Error(), "name it") {
		t.Fatalf("root target must be refused: %v", err)
	}

	// Self-migration is refused.
	config.SetProfile("src")
	if _, err := capture(t, func() error { return cmdMigrate([]string{"--from", "src"}) }); err == nil ||
		!strings.Contains(err.Error(), "different") {
		t.Fatalf("self target must be refused: %v", err)
	}

	// An existing workspace is refused (src has a config.json).
	config.SetProfile("src")
	if _, err := capture(t, func() error { return cmdMigrate([]string{"--from", "other"}) }); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing target must be refused: %v", err)
	}

	// --from is required.
	config.SetProfile("dst")
	if _, err := capture(t, func() error { return cmdMigrate(nil) }); err == nil {
		t.Fatal("missing --from must be a usage error")
	}
}

// GDK-1275: a frozen source cannot serve attachment bytes, and the run used
// to warn and carry on — producing a workspace whose 26 attachments were
// empty shells while the verify table read 26/26. The cutover procedure
// (freeze writes, then migrate) walks straight into it. Refuse instead,
// unless the caller says metadata-only is what they want.
func TestMigrateRefusesFrozenSourceWithAttachments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	allowProfileCreate = true
	t.Cleanup(func() {
		allowProfileCreate = false
		_ = origin.Close()
		config.SetProfile("")
	})

	config.SetProfile("src")
	if out, err := capture(t, func() error { return cmdInit([]string{"--local"}) }); err != nil {
		t.Fatalf("init src: %v\n%s", err, out)
	}
	key := createIssue(t, "an issue with a file")
	att := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(att, []byte("bytes that must not vanish\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := capture(t, func() error { return cmdAttach([]string{key, att}) }); err != nil {
		t.Fatalf("attach: %v\n%s", err, out)
	}
	if out, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync src: %v\n%s", err, out)
	}
	_ = origin.Close()
	if out, err := capture(t, func() error { return cmdConfig([]string{"set", "frozen", "true"}) }); err != nil {
		t.Fatalf("freeze src: %v\n%s", err, out)
	}

	config.SetProfile("dst")
	_, err := capture(t, func() error { return cmdMigrate([]string{"--from", "src"}) })
	if err == nil {
		t.Fatal("a frozen source with attachments must be refused, not warned about")
	}
	// The message has to name both ways forward, or the refusal is a wall.
	for _, want := range []string{"frozen", "--skip-attachments"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not name %q: %v", want, err)
		}
	}

	// Saying metadata-only out loud still works.
	config.SetProfile("dst")
	if out, merr := capture(t, func() error {
		return cmdMigrate([]string{"--from", "src", "--skip-attachments"})
	}); merr != nil {
		t.Fatalf("--skip-attachments must proceed on a frozen source: %v\n%s", merr, out)
	}
}
