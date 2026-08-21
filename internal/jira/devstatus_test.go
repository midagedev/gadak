package jira

import "testing"

func TestParseDevPRStatus(t *testing.T) {
	for in, want := range map[string]DevPRStatus{
		"OPEN": DevPROpen, "open": DevPROpen, " Open ": DevPROpen,
		"MERGED": DevPRMerged, "merged": DevPRMerged,
		"DECLINED": DevPRDeclined, "declined": DevPRDeclined,
	} {
		got, ok := ParseDevPRStatus(in)
		if !ok || got != want {
			t.Errorf("ParseDevPRStatus(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	if _, ok := ParseDevPRStatus("CLOSED"); ok {
		t.Error("CLOSED is a GitHub token, not an origin status")
	}
	if _, ok := ParseDevPRStatus(""); ok {
		t.Error("empty parsed as a status")
	}
}

func TestDevPRStatusFromGitHub(t *testing.T) {
	for in, want := range map[string]DevPRStatus{
		"OPEN": DevPROpen, "open": DevPROpen, "weird": DevPROpen,
		"MERGED": DevPRMerged, "merged": DevPRMerged,
		"CLOSED": DevPRDeclined, "closed": DevPRDeclined,
	} {
		if got := DevPRStatusFromGitHub(in); got != want {
			t.Errorf("DevPRStatusFromGitHub(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDevPRStatusStored(t *testing.T) {
	if got := DevPROpen.Stored(); got != "open" {
		t.Errorf("OPEN.Stored() = %q, want open", got)
	}
	if got := DevPRMerged.Stored(); got != "merged" {
		t.Errorf("MERGED.Stored() = %q, want merged", got)
	}
	if got := DevPRDeclined.Stored(); got != "declined" {
		t.Errorf("DECLINED.Stored() = %q, want declined", got)
	}
}
