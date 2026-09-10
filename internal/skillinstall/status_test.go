package skillinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontmatterName(t *testing.T) {
	cases := map[string]string{
		"---\nname: gadak\n---\nbody\n":                 "gadak",
		"---\ndescription: x\nname:  gadak  \n---\nb\n": "gadak",
		"---\nname: my-notes\n---\nbody\n":              "my-notes",
		"no frontmatter at all\n":                       "",
		"---\nname: gadak\nunterminated frontmatter\n":  "",
		"body first\n---\nname: gadak\n---\n":           "",
		// The key has to start the line: a description that happens to contain
		// "name:" is not a name.
		"---\ndescription: mentions name: gadak\n---\n": "",
	}
	for in, want := range cases {
		if got := FrontmatterName([]byte(in)); got != want {
			t.Errorf("FrontmatterName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFrontmatterDescription(t *testing.T) {
	t.Run("plain scalar", func(t *testing.T) {
		got, ok := FrontmatterDescription([]byte("---\nname: gadak\ndescription: one line\n---\nbody\n"))
		if !ok || got != "one line" {
			t.Errorf("got %q ok=%v", got, ok)
		}
	})

	t.Run("folded block joins with spaces", func(t *testing.T) {
		src := "---\nname: gadak\ndescription: >\n  first line\n  second line\n---\nbody\n"
		got, ok := FrontmatterDescription([]byte(src))
		if !ok || got != "first line second line" {
			t.Errorf("got %q ok=%v", got, ok)
		}
	})

	t.Run("folded block: a blank line is a real break", func(t *testing.T) {
		src := "---\ndescription: >\n  a\n\n  b\n---\n"
		got, _ := FrontmatterDescription([]byte(src))
		if got != "a\nb" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("literal block keeps its lines", func(t *testing.T) {
		src := "---\ndescription: |\n  a\n  b\n---\n"
		got, _ := FrontmatterDescription([]byte(src))
		if got != "a\nb" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("the block ends at the next key", func(t *testing.T) {
		src := "---\ndescription: >\n  folded text\nname: gadak\n---\n"
		got, _ := FrontmatterDescription([]byte(src))
		if got != "folded text" {
			t.Errorf("got %q — the block must not swallow the next key", got)
		}
		if n := FrontmatterName([]byte(src)); n != "gadak" {
			t.Errorf("name after a folded block = %q", n)
		}
	})

	t.Run("absent", func(t *testing.T) {
		if _, ok := FrontmatterDescription([]byte("---\nname: gadak\n---\n")); ok {
			t.Error("want ok=false for a frontmatter with no description")
		}
	})
}

// TestDestStatusToleratesOSMetadataSibling — orca's incident is the reason this
// test exists: a single Finder visit writes .DS_Store into the skill directory,
// and there it was enough to make an untouched copy read as "unrecognized", so
// the user was asked to --force over their own unmodified files.
//
// gadak cannot fail that way today: identity is one file's hash, and the
// provenance check reads the receipt by exact name rather than listing the
// directory. So this passed before the tolerance existed too — it is a guard,
// not a fix, and it is here so the directory-wide comparison the desktop
// integrations round needs cannot reintroduce the bug.
func TestDestStatusToleratesOSMetadataSibling(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "SKILL.md")
	content := []byte("---\nname: gadak\ndescription: x\n---\nbody\n")
	if err := os.WriteFile(dest, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteReceipt(dir, content, "test"); err != nil {
		t.Fatal(err)
	}

	for _, junk := range []string{".DS_Store", "Thumbs.db", "desktop.ini", "._SKILL.md"} {
		if err := os.WriteFile(filepath.Join(dir, junk), []byte("os"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	status, _, err := DestStatus(dest, content)
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusIdentical {
		t.Errorf("status beside OS metadata = %q, want %q", status, StatusIdentical)
	}

	// And the same directory, one release behind, is still gadak's own copy.
	next := append(append([]byte{}, content...), '\n')
	status, _, err = DestStatus(dest, next)
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusStale {
		t.Errorf("behind-by-one beside OS metadata = %q, want %q", status, StatusStale)
	}
}

func TestDestStatusMissingAndConflict(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "SKILL.md")
	content := []byte("gadak skill\n")

	status, _, err := DestStatus(dest, content)
	if err != nil || status != StatusMissing {
		t.Fatalf("empty dir: %q err=%v", status, err)
	}

	if err := os.WriteFile(dest, []byte("someone else's file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, existing, err := DestStatus(dest, content)
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusConflict {
		t.Errorf("foreign file = %q, want %q", status, StatusConflict)
	}
	if string(existing) != "someone else's file\n" {
		t.Errorf("existing bytes not returned: %q", existing)
	}
}

func TestDestStatusRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "SKILL.md")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := DestStatus(dest, []byte("x")); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Errorf("want a directory error, got %v", err)
	}
}

// TestStatusWord — GDK-1534. "identical" is the classifier's word; the word a
// person reads is "current". Two callers renamed it by hand (doctor, the
// desktop integrations list); this is the one owner, so a third caller cannot
// diverge. The other three words are already the words people read.
func TestStatusWord(t *testing.T) {
	for _, tc := range []struct{ status, want string }{
		{StatusIdentical, "current"},
		{StatusStale, StatusStale},
		{StatusMissing, StatusMissing},
		{StatusConflict, StatusConflict},
	} {
		if got := StatusWord(tc.status); got != tc.want {
			t.Errorf("StatusWord(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestReceiptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok := ReadReceipt(dir); ok {
		t.Error("no receipt should read as absent")
	}
	// GDK-1520: the receipt records both digests of the bytes it is handed —
	// the exact form, and the line-ending-folded form — which is why
	// WriteReceipt takes content rather than a precomputed digest.
	content := []byte("line one\nline two\n")
	if err := WriteReceipt(dir, content, "0.0.0-test"); err != nil {
		t.Fatal(err)
	}
	r, ok := ReadReceipt(dir)
	if !ok || r.SHA256 != Digest(content) || r.GadakVersion != "0.0.0-test" {
		t.Fatalf("receipt = %+v ok=%v", r, ok)
	}
	if want, _ := TextDigest(content); r.TextSHA256 != want {
		t.Fatalf("text digest = %q, want %q", r.TextSHA256, want)
	}
	// The two digests differ, or the folded form would be testing nothing.
	if text, _ := TextDigest(toCRLF(content)); text != r.TextSHA256 {
		t.Fatalf("CRLF of the same text hashed to %q, want the receipt's %q", text, r.TextSHA256)
	}
	// A corrupt receipt is "no receipt", never an error: it is a disposable
	// cache, and the worst it costs is one --force.
	if err := os.WriteFile(filepath.Join(dir, ReceiptName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadReceipt(dir); ok {
		t.Error("corrupt receipt should read as absent")
	}
}

// toCRLF rewrites a skill body the way git's autocrlf=true rewrites a file the
// moment it lands on a Windows checkout — every LF becomes CRLF (GDK-1520,
// the vector orca's verify-skill-update-roundtrip --autocrlf matrix covers).
func toCRLF(b []byte) []byte {
	return []byte(strings.ReplaceAll(string(b), "\n", "\r\n"))
}

// TestDestStatusCRLFCurrentIsIdentical — GDK-1520. The copy on disk is the
// embedded skill after Windows line-ending conversion: same text, different
// bytes. Telling the user "differs — re-run with --force" over an OS-side
// rewrite is the roundtrip break the receipt exists to prevent.
func TestDestStatusCRLFCurrentIsIdentical(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "SKILL.md")
	content := []byte("---\nname: gadak\ndescription: x\n---\n\n# body\n")
	if err := os.WriteFile(dest, toCRLF(content), 0o644); err != nil {
		t.Fatal(err)
	}
	status, _, err := DestStatus(dest, content)
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusIdentical {
		t.Errorf("CRLF of the current skill = %q, want %q", status, StatusIdentical)
	}
}

// TestDestStatusCRLFofOurLastWriteIsStale — GDK-1520. gadak wrote the previous
// release's body, the OS converted it, and this binary carries a newer skill.
// That is gadak's copy behind by one release — stale, upgradable without
// --force — not a conflict.
func TestDestStatusCRLFofOurLastWriteIsStale(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "SKILL.md")
	prev := []byte("---\nname: gadak\ndescription: previous\n---\n\n# older\n")
	if err := os.WriteFile(dest, prev, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteReceipt(dir, prev, "0.0.0-test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, toCRLF(prev), 0o644); err != nil {
		t.Fatal(err)
	}
	status, _, err := DestStatus(dest, []byte("---\nname: gadak\ndescription: newer\n---\n\n# newer\n"))
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusStale {
		t.Errorf("CRLF of gadak's own previous write = %q, want %q", status, StatusStale)
	}
}

// TestDestStatusSymlinkedDestConsultsTargetReceipt — GDK-1520, the
// --shape=symlink leg. A provider-style layout symlinks the dest at a host's
// path to a canonical copy that carries its receipt. The classifier judged by
// the directory *beside the link* only, never found the receipt, and called
// gadak's own stale copy a conflict.
func TestDestStatusSymlinkedDestConsultsTargetReceipt(t *testing.T) {
	canonicalDir := t.TempDir()
	canonical := filepath.Join(canonicalDir, "SKILL.md")
	prev := []byte("---\nname: gadak\ndescription: previous\n---\n\n# older\n")
	if err := os.WriteFile(canonical, prev, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteReceipt(canonicalDir, prev, "0.0.0-test"); err != nil {
		t.Fatal(err)
	}

	providerDir := t.TempDir()
	link := filepath.Join(providerDir, "SKILL.md")
	if err := os.Symlink(canonical, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	status, _, err := DestStatus(link, []byte("---\nname: gadak\ndescription: newer\n---\n\n# newer\n"))
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusStale {
		t.Errorf("symlink to gadak's own previous write = %q, want %q", status, StatusStale)
	}
	// And the current case through a link is identical, which is what a
	// provider layout sees on a happy day.
	if err := os.WriteFile(canonical, []byte("---\nname: gadak\ndescription: newer\n---\n\n# newer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, _, err = DestStatus(link, []byte("---\nname: gadak\ndescription: newer\n---\n\n# newer\n"))
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusIdentical {
		t.Errorf("symlink to the current skill = %q, want %q", status, StatusIdentical)
	}
}
