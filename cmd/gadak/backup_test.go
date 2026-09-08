package main

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// builtInTrackerHome is `init --local` plus two issues, with the embedded
// origin still open (the WAL sidecars on disk are the "serve is running"
// shape backup must copy through).
func builtInTrackerHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if out, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatalf("init --local: %v\n%s", err, out)
	}
	for _, s := range []string{"backup one", "backup two"} {
		if out, err := capture(t, func() error { return cmdCreate([]string{s}) }); err != nil {
			t.Fatalf("create: %v\n%s", err, out)
		}
	}
}

func countPersistIssues(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM issues").Scan(&n); err != nil {
		t.Fatalf("count issues in %s: %v", path, err)
	}
	return n
}

func TestBackupCopiesBuiltInTrackerPersist(t *testing.T) {
	builtInTrackerHome(t)
	dir := t.TempDir()

	out, err := capture(t, func() error { return cmdBackup([]string{"--to", dir, "--json"}) })
	if err != nil {
		t.Fatalf("backup: %v\n%s", err, out)
	}
	var rep struct {
		Path   string `json:"path"`
		Issues int    `json:"issues"`
		Bytes  int64  `json:"bytes"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("backup --json: %v\n%s", err, out)
	}
	if filepath.Dir(rep.Path) != dir || !strings.HasSuffix(rep.Path, ".tar") {
		t.Fatalf("backup landed at %q, want a .tar inside %q", rep.Path, dir)
	}
	if rep.Bytes <= 0 {
		t.Fatalf("bytes %d", rep.Bytes)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	_, src := origin.Describe(cfg)
	want := countPersistIssues(t, src)
	if want != 2 {
		t.Fatalf("fixture has %d issues, want 2", want)
	}
	if rep.Issues != want {
		t.Fatalf("report says %d issues, source has %d", rep.Issues, want)
	}
	// Unpack and count: the archive is the backup, so the check has to go
	// through the archive.
	restored := t.TempDir()
	names := untar(t, rep.Path, restored)
	if !names["issuetap.db"] {
		t.Fatalf("archive has no issuetap.db: %v", keys(names))
	}
	if got := countPersistIssues(t, filepath.Join(restored, "issuetap.db")); got != want {
		t.Fatalf("restored copy has %d issues, source has %d", got, want)
	}
	// VACUUM INTO leaves no WAL sidecar to lose.
	for _, sfx := range []string{"-wal", "-shm"} {
		if names["issuetap.db"+sfx] {
			t.Fatalf("archive carries a %s sidecar", sfx)
		}
	}
	// The temp copy is cleaned up, not left beside the archive.
	if _, err := os.Stat(rep.Path + ".db.tmp"); !os.IsNotExist(err) {
		t.Fatalf("backup left its scratch copy behind (err=%v)", err)
	}

	// Text mode: one line, the path, nothing else.
	file := filepath.Join(dir, "explicit.tar")
	out, err = capture(t, func() error { return cmdBackup([]string{"--to", file}) })
	if err != nil {
		t.Fatalf("backup --to file: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != file {
		t.Fatalf("stdout %q, want %q", out, file)
	}
	// An existing target is refused, never overwritten.
	if _, err := capture(t, func() error { return cmdBackup([]string{"--to", file}) }); err == nil {
		t.Fatal("backup onto an existing file must fail")
	}
}

func TestBackupRefusesJiraWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	cfg := &config.Config{Site: "http://127.0.0.1:1", Email: "a@example.invalid", Token: "tok"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error { return cmdBackup([]string{"--to", t.TempDir()}) })
	if err == nil || !strings.Contains(err.Error(), "built-in tracker") {
		t.Fatalf("want built-in-tracker refusal, got %v", err)
	}
}

func TestBackupRefusesPairedWorkspace(t *testing.T) {
	seedPairedProfile(t)
	_, err := capture(t, func() error { return cmdBackup([]string{"--to", t.TempDir()}) })
	if err == nil || !strings.Contains(err.Error(), "home machine") || !strings.Contains(err.Error(), `"laptop"`) {
		t.Fatalf("want run-on-home-machine refusal naming the pairing label, got %v", err)
	}
	if strings.Contains(err.Error(), "home.ts.net") {
		t.Fatalf("endpoint leaked into refusal: %v", err)
	}
}

// TestBackupCarriesAttachmentBytes is GDK-1617's backup half. Attachment
// bytes moved out of the persist file and into a directory beside it, so a
// backup that copies only the file is a backup with every attachment
// missing — and it reports success, which makes it worse than none.
func TestBackupCarriesAttachmentBytes(t *testing.T) {
	builtInTrackerHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	const body = "these bytes have to be in the archive"
	if _, err := c.Upload(context.Background(), "STD-1", "note.txt", strings.NewReader(body)); err != nil {
		t.Fatalf("upload: %v", err)
	}

	dir := t.TempDir()
	out, err := capture(t, func() error { return cmdBackup([]string{"--to", dir, "--json"}) })
	if err != nil {
		t.Fatalf("backup: %v\n%s", err, out)
	}
	var rep struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	restored := t.TempDir()
	names := untar(t, rep.Path, restored)

	sum := sha256.Sum256([]byte(body))
	want := "blobs/" + hex.EncodeToString(sum[:])[:2] + "/" + hex.EncodeToString(sum[:])
	if !names[want] {
		t.Fatalf("the uploaded bytes are not in the archive as %s; members: %v", want, keys(names))
	}
	got, err := os.ReadFile(filepath.Join(restored, filepath.FromSlash(want)))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("restored attachment = %q, want %q", got, body)
	}
}

// untar extracts into dir and returns the member names.
func untar(t *testing.T, archive, dir string) map[string]bool {
	t.Helper()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	names := map[string]bool{}
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[h.Name] = true
		dst := filepath.Join(dir, filepath.FromSlash(h.Name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			t.Fatal(err)
		}
		w, err := os.Create(dst)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(w, tr); err != nil {
			t.Fatal(err)
		}
		w.Close()
	}
	return names
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestFailedBackupLeavesNoPartialArchive is GDK-1617 review finding 6. The
// completeness check runs after the tar members are written, so a refusal
// used to leave a truncated archive at the destination — and O_EXCL then
// turned "an existing target is never overwritten" into a guard over a
// known-bad file, blocking the retry that would have fixed it.
func TestFailedBackupLeavesNoPartialArchive(t *testing.T) {
	builtInTrackerHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Upload(context.Background(), "STD-1", "gone.txt", strings.NewReader("bytes about to vanish")); err != nil {
		t.Fatal(err)
	}
	// The bytes disappear from disk while the database still references
	// them — a damaged workspace, which is exactly when a backup matters.
	_, persist := origin.Describe(cfg)
	blobs := filepath.Join(filepath.Dir(persist), "blobs")
	if err := os.RemoveAll(blobs); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "b.tar")
	_, err = capture(t, func() error { return cmdBackup([]string{"--to", dst}) })
	if err == nil {
		t.Fatal("a backup missing referenced bytes must be refused")
	}
	if !strings.Contains(err.Error(), "not on disk") {
		t.Fatalf("the refusal does not say what is missing: %v", err)
	}
	if _, serr := os.Stat(dst); !os.IsNotExist(serr) {
		t.Fatalf("a partial archive was left at %s (err=%v) — the retry now hits the existing-target guard", dst, serr)
	}
}

// The existing-target guard must still protect a real backup: a refusal
// before anything is written must not delete what is already there.
func TestBackupRefusalKeepsTheExistingFile(t *testing.T) {
	builtInTrackerHome(t)
	dst := filepath.Join(t.TempDir(), "keep.tar")
	if err := os.WriteFile(dst, []byte("an earlier backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := capture(t, func() error { return cmdBackup([]string{"--to", dst}) }); err == nil {
		t.Fatal("backup onto an existing file must fail")
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "an earlier backup" {
		t.Fatalf("the existing backup was destroyed by the refusal that exists to protect it: %q %v", got, err)
	}
}
