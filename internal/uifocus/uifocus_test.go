package uifocus

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteTake(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := Write("pj=NMA&sc=inprogress"); err != nil {
		t.Fatal(err)
	}
	hash, ok, err := Take()
	if err != nil || !ok || hash != "pj=NMA&sc=inprogress" {
		t.Fatalf("take %q ok=%v err=%v", hash, ok, err)
	}
	if _, ok, err := Take(); err != nil || ok {
		t.Fatal("second take should be empty")
	}
}

func TestTakeIgnoresStale(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	old := `{"hash":"pj=NMA","at":"` + time.Now().UTC().Add(-time.Hour).Format(time.RFC3339) + `"}`
	if err := os.WriteFile(p, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := Take(); err != nil || ok {
		t.Fatalf("stale should be ignored ok=%v err=%v", ok, err)
	}
}
