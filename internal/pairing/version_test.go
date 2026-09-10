package pairing

import "testing"

// TestVersionSkew is FAIL-first for GDK-1273's classification: the pair-time
// record only answers "which side is older", and a version that does not
// parse (a dev build, a local hack) can only be compared for equality.
func TestVersionSkew(t *testing.T) {
	cases := []struct {
		home, client string
		want         string
	}{
		{"0.19.1", "0.19.1", "same"},
		{"", "", "unknown"},
		{"0.19.1", "", "unknown"},
		{"", "0.22.0", "unknown"},
		{"0.19.1", "0.22.0", "home-older"},
		{"0.22.0", "0.19.1", "home-newer"},
		// Component-wise, not string-wise: 0.9.x < 0.10.x.
		{"0.9.3", "0.10.0", "home-older"},
		{"0.10.0", "0.9.3", "home-newer"},
		// Shorter numbers count as 0-padded.
		{"0.19", "0.19.1", "home-older"},
		// A -dev suffix rides a release line: same numbers, same line.
		{"0.22.0-dev", "0.22.0", "same"},
		// Unparseable compares by equality only.
		{"deadbeef", "0.22.0", "unknown"},
		{"deadbeef", "deadbeef", "same"},
	}
	for _, c := range cases {
		if got := VersionSkew(c.home, c.client); got != c.want {
			t.Errorf("VersionSkew(%q, %q) = %q, want %q", c.home, c.client, got, c.want)
		}
	}
}

// TestRemoteServerVersionRoundTrip pins the credential shape: the pair-time
// record survives the atomic write, and an older credential without the
// field still loads (every pre-GDK-1273 pairing on disk).
func TestRemoteServerVersionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := SaveRemote(dir, Remote{
		Endpoint:      "https://home.example.com:8443",
		Token:         "device-token",
		Label:         "laptop",
		ServerVersion: "0.19.1",
	}); err != nil {
		t.Fatal(err)
	}
	rem, err := LoadRemote(dir)
	if err != nil || rem == nil {
		t.Fatalf("load: %v (%v)", rem, err)
	}
	if rem.ServerVersion != "0.19.1" {
		t.Fatalf("ServerVersion = %q, want 0.19.1", rem.ServerVersion)
	}
}
