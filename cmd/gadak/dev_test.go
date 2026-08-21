package main

import (
	"reflect"
	"testing"
)

// GDK-531: dev scan's pure parts — key extraction from PR titles/branches
// and gh's state vocabulary mapped onto dev-status's.
func TestIssueKeys(t *testing.T) {
	got := issueKeys("GDK-531 fix the drop (gdk-531-scan-branch) and STD-2, again GDK-531")
	want := []string{"GDK-531", "STD-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("issueKeys = %v, want %v", got, want)
	}
	if got := issueKeys("no keys here, just v1.2-3 and 12-34"); got != nil {
		t.Fatalf("issueKeys on keyless text = %v", got)
	}
	// Key-shaped garbage (CVE-2024-1234) still matches — the mirror filter
	// owns rejecting it, so the extractor stays dumb on purpose.
	if got := issueKeys("fixes CVE-2024-1234"); len(got) == 0 {
		t.Fatal("extractor should stay broad; the mirror filters")
	}
}

func TestDevScanStatus(t *testing.T) {
	for in, want := range map[string]string{
		"OPEN": "OPEN", "open": "OPEN", "MERGED": "MERGED",
		"CLOSED": "DECLINED", "closed": "DECLINED", "weird": "OPEN",
	} {
		if got := devScanStatus(in); got != want {
			t.Fatalf("devScanStatus(%q) = %q, want %q", in, got, want)
		}
	}
}
