package config

// GDK-1278/1279: one field carried two questions — which tracker the origin
// is, and whether it is reached in-process. The proof that matters is that
// every config.json shape already on disk still answers both correctly,
// because a workspace file is not a throwaway cache.

import (
	"testing"

	"github.com/midagedev/gadak/internal/pairing"
)

func TestOriginTypeAndTransportFromStoredKinds(t *testing.T) {
	cases := []struct {
		name      string
		cfg       Config
		origin    string
		transport string
	}{
		// The pre-split vocabulary. These values are on real disks.
		{"legacy empty kind, jira site", Config{Site: "https://x.atlassian.net", Email: "a@b.c", Token: "t"}, OriginJira, TransportRemote},
		{"legacy connected", Config{Kind: KindConnected, Site: "https://x.atlassian.net", Email: "a@b.c", Token: "t"}, OriginJira, TransportRemote},
		{"legacy local-origin", Config{Kind: KindLocalOrigin}, OriginGadak, TransportLocal},
		// Linear only when no Atlassian site is configured — the same
		// precedence origin.Client applies.
		{"linear key, no site", Config{Linear: &LinearConfig{APIKey: "lin_x"}}, OriginLinear, TransportRemote},
		{"linear key beside a site", Config{Site: "https://x.atlassian.net", Email: "a@b.c", Token: "t", Linear: &LinearConfig{APIKey: "lin_x"}}, OriginJira, TransportRemote},
		// The new stored values.
		{"stored gadak", Config{Kind: OriginGadak}, OriginGadak, TransportLocal},
		{"stored jira", Config{Kind: OriginJira, Site: "https://x.atlassian.net"}, OriginJira, TransportRemote},
		{"stored linear", Config{Kind: OriginLinear, Linear: &LinearConfig{APIKey: "lin_x"}}, OriginLinear, TransportRemote},
		// An unconfigured home is not a third tracker.
		{"empty", Config{}, OriginJira, TransportRemote},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("GADAK_HOME", home)
			cfg := c.cfg
			if got := cfg.OriginType(); got != c.origin {
				t.Errorf("OriginType() = %q, want %q", got, c.origin)
			}
			if got := cfg.Transport(); got != c.transport {
				t.Errorf("Transport() = %q, want %q", got, c.transport)
			}
		})
	}
}

// The case that named the defect (GDK-1262 cutover): the origin is gadak's
// own tracker, one machine away. The old vocabulary called this "connected",
// which reads as an external tracker, and the truth was only visible in a
// separate pairing object.
func TestPairedWorkspaceIsGadakOverRemote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { SetProfile("") })
	SetProfile("")

	c, err := LoadFor("")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.OriginType(); got != OriginJira {
		t.Fatalf("unpaired empty workspace: OriginType() = %q, want %q", got, OriginJira)
	}
	if err := pairing.SaveRemote(c.Directory(), pairing.Remote{
		Endpoint: "https://home.example.com:8443",
		Token:    "device-token",
		Label:    "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	if got := c.OriginType(); got != OriginGadak {
		t.Errorf("paired: OriginType() = %q, want %q", got, OriginGadak)
	}
	if got := c.Transport(); got != TransportRemote {
		t.Errorf("paired: Transport() = %q, want %q", got, TransportRemote)
	}
	// Same origin type as an in-process issuetap — the transport is the
	// only thing that differs, which is exactly the point of the split.
	local := Config{Kind: KindLocalOrigin}
	if local.OriginType() != c.OriginType() {
		t.Errorf("paired and in-process disagree on origin type: %q vs %q", c.OriginType(), local.OriginType())
	}
	if local.Transport() == c.Transport() {
		t.Errorf("paired and in-process must differ on transport, both %q", local.Transport())
	}
}

// HasLocalOrigin gates writes all over the tree; it must accept the new
// stored value or a migrated config silently loses its origin.
func TestIsLocalOriginAcceptsBothStoredValues(t *testing.T) {
	for _, kind := range []string{KindLocalOrigin, OriginGadak} {
		c := Config{Kind: kind}
		if !c.HasLocalOrigin() {
			t.Errorf("Kind %q: HasLocalOrigin() = false", kind)
		}
	}
	for _, kind := range []string{"", KindConnected, OriginJira, OriginLinear} {
		c := Config{Kind: kind}
		if c.HasLocalOrigin() {
			t.Errorf("Kind %q: HasLocalOrigin() = true", kind)
		}
	}
}
