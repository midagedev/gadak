package server

import (
	"testing"

	"github.com/midagedev/gadak/internal/uifocus"
)

func TestWorkspaceFocusTakesProfileFile(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := uifocus.WriteFor("work", "ks=BBB-1"); err != nil {
		t.Fatal(err)
	}
	if err := uifocus.WriteFor("", "ks=AAA-1"); err != nil {
		t.Fatal(err)
	}
	h := NewWorkspace(nil, nil, nil, "work")
	got := decode[struct {
		Hash string `json:"hash"`
	}](t, get(t, h, apiBase+"ui-focus/", nil))
	if got.Hash != "ks=BBB-1" {
		t.Fatalf("workspace took %q, want ks=BBB-1", got.Hash)
	}
	hash, ok, err := uifocus.TakeFor("")
	if err != nil || !ok || hash != "ks=AAA-1" {
		t.Fatalf("default file should be untouched: %q ok=%v err=%v", hash, ok, err)
	}
}

func TestPrimaryFocusTakesProcessProfileFile(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := uifocus.Write("pj=NMA"); err != nil {
		t.Fatal(err)
	}
	h := New(nil, nil)
	got := decode[struct {
		Hash string `json:"hash"`
	}](t, get(t, h, apiBase+"ui-focus/", nil))
	if got.Hash != "pj=NMA" {
		t.Fatalf("primary took %q", got.Hash)
	}
}
