package serveaddr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func TestDirIsHomeRootNotProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("work")
	t.Cleanup(func() { config.SetProfile("") })

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, Rel)
	if dir != want {
		t.Fatalf("Dir() = %q, want home-root %q", dir, want)
	}
	if strings.Contains(filepath.ToSlash(dir), "/profiles/") {
		t.Fatalf("Dir() must not be per-profile, got %q", dir)
	}
}

func TestWriteRemoveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "127.0.0.1:7891", "oss"); err != nil {
		t.Fatal(err)
	}
	p := Path(dir, "7891")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Addr != "127.0.0.1:7891" || rec.Profile != "oss" || rec.PID != os.Getpid() || rec.StartedAt == "" {
		t.Fatalf("record = %+v", rec)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("file mode = %04o, want 0600", fi.Mode().Perm())
		}
	}

	listed := List(dir)
	if len(listed) != 1 || listed[0].Port != "7891" || listed[0].Addr != rec.Addr {
		t.Fatalf("List = %+v", listed)
	}

	if err := Remove(dir, "7891"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	if err := Remove(dir, "7891"); err != nil {
		t.Fatalf("remove missing: %v", err)
	}
	if recs := List(dir); len(recs) != 0 {
		t.Fatalf("List after remove = %+v", recs)
	}
}

func TestWriteNamesFileByPort(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "[::1]:9001", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Path(dir, "9001")); err != nil {
		t.Fatal(err)
	}
}

func TestWriteRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := Write("", "127.0.0.1:1", ""); err == nil {
		t.Fatal("empty dir must fail")
	}
	if err := Write(dir, "", ""); err == nil {
		t.Fatal("empty addr must fail")
	}
	if err := Write(dir, "not-a-hostport", ""); err == nil {
		t.Fatal("malformed addr must fail")
	}
}

func TestListEmptyMissingDir(t *testing.T) {
	if recs := List(filepath.Join(t.TempDir(), "missing")); recs != nil && len(recs) != 0 {
		t.Fatalf("missing dir List = %+v", recs)
	}
	if recs := List(""); recs != nil && len(recs) != 0 {
		t.Fatalf("empty dir List = %+v", recs)
	}
}

func TestListSkipsMalformedAndTmp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "7891.json.tmp"), []byte(`{"addr":"127.0.0.1:7891"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "7777.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "7778.json"), []byte(`{"addr":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if recs := List(dir); len(recs) != 0 {
		t.Fatalf("List = %+v", recs)
	}
}

func TestPublishRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	stop, err := Publish("127.0.0.1:8123", "work")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	recs := List(dir)
	if len(recs) != 1 || recs[0].Addr != "127.0.0.1:8123" || recs[0].Profile != "work" {
		t.Fatalf("List = %+v", recs)
	}
	stop()
	if recs := List(dir); len(recs) != 0 {
		t.Fatalf("after unpublish = %+v", recs)
	}
}

func TestPathRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	if p := Path(dir, "../etc"); p != "" {
		t.Fatalf("Path accepted non-port: %q", p)
	}
	if err := Remove(dir, "../etc"); err != nil {
		t.Fatalf("Remove non-port: %v", err)
	}
}
