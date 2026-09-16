package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// HostPolicy tests (GDK-1966): the DNS-name allowlist the rebinding guard
// consults. A name on the list is a Host the serve answers by — membership
// is the credential — so the gates here pin exactly which names land on it
// and that nothing else moves.

func TestNewHostPolicyParsesOriginsToNames(t *testing.T) {
	p := NewHostPolicy(
		[]string{"https://Gadak.Example.com", "http://alt.example.com:8443"},
		[]string{"https://gadak.example.com", "https://third.example.com/"},
		"vps.example.ts.net",
	)
	// Flag entries first, then config, then the Tailscale name; duplicates
	// by host collapse; ports and trailing slashes drop.
	want := []string{"gadak.example.com", "alt.example.com", "third.example.com", "vps.example.ts.net"}
	if got := p.Names(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for _, host := range want {
		if !p.Allows(host) {
			t.Errorf("Allows(%q) = false, want true", host)
		}
	}
}

func TestHostPolicyAllowsIsCaseInsensitiveAndUnlistedStaysFalse(t *testing.T) {
	p := NewHostPolicy([]string{"https://gadak.example.com"}, nil, "VPS.Example.TS.Net")
	for _, host := range []string{"GADAK.EXAMPLE.COM", "gadak.example.com", "vps.example.ts.net", "VPS.example.ts.net"} {
		if !p.Allows(host) {
			t.Errorf("Allows(%q) = false, want true", host)
		}
	}
	for _, host := range []string{"evil.example.com", "gadak.example.com.evil.example", "example.ts.net", ""} {
		if p.Allows(host) {
			t.Errorf("Allows(%q) = true, want false", host)
		}
	}
}

func TestNilAndEmptyHostPolicyAdmitNothing(t *testing.T) {
	if (*HostPolicy)(nil).Allows("gadak.example.com") {
		t.Fatal("nil policy admitted a DNS name")
	}
	if p := NewHostPolicy(nil, nil, ""); !p.Empty() || p.Allows("gadak.example.com") {
		t.Fatal("empty policy admitted a DNS name")
	}
}

func TestNewHostPolicySkipsUnparseableEntries(t *testing.T) {
	// The cmd layer validates its inputs; the constructor still must not
	// panic or admit nonsense for an entry that slipped through.
	p := NewHostPolicy([]string{"::not a url::", "gadak.example.com", ""}, nil, "")
	if got := p.Names(); len(got) != 1 || got[0] != "gadak.example.com" {
		t.Fatalf("Names() = %v, want [gadak.example.com]", got)
	}
}

func TestHostPolicyDescribeNamesSources(t *testing.T) {
	p := NewHostPolicy([]string{"https://gadak.example.com"}, []string{"https://alt.example.com"}, "vps.example.ts.net")
	want := "gadak.example.com (--public-url), alt.example.com (serve.publicUrls), vps.example.ts.net (tailscale)"
	if got := p.Describe(); got != want {
		t.Fatalf("Describe() = %q, want %q", got, want)
	}
	if p.Empty() {
		t.Fatal("policy with entries reported Empty")
	}
	empty := NewHostPolicy(nil, nil, "")
	if got := empty.Describe(); got != "" {
		t.Fatalf("empty policy Describe() = %q, want \"\"", got)
	}
}

// The end-to-end gates the spec names: a policy Host passes the guard
// (with port, case-insensitively), a different DNS name still gets 403
// forbidden_host, and a nil policy behaves exactly as today.
func TestGuardBrowserHostPolicyAdmitsListedName(t *testing.T) {
	p := NewHostPolicy([]string{"https://gadak.example.com"}, nil, "vps.example.ts.net")
	for _, host := range []string{
		"gadak.example.com",
		"gadak.example.com:7777",
		"GADAK.EXAMPLE.COM",
		"vps.example.ts.net:8443",
	} {
		t.Run(host, func(t *testing.T) {
			var hit bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hit = true
				w.WriteHeader(http.StatusOK)
			})
			h := GuardBrowser(next, GuardExempts{}, func() *HostPolicy { return p })
			req := httptest.NewRequest(http.MethodGet, "/config.json", nil)
			req.Host = host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !hit {
				t.Fatalf("Host %q status %d hit=%v, want 200/true; body %s", host, rec.Code, hit, rec.Body.String())
			}
		})
	}
}

func TestGuardBrowserHostPolicyRejectsUnlistedDNS(t *testing.T) {
	p := NewHostPolicy([]string{"https://gadak.example.com"}, nil, "vps.example.ts.net")
	h := GuardBrowser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next ran for an unlisted DNS host")
	}), GuardExempts{}, func() *HostPolicy { return p })
	req := httptest.NewRequest(http.MethodGet, "/config.json", nil)
	req.Host = "evil.example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "forbidden_host" {
		t.Fatalf("error %q, want forbidden_host", got)
	}
}

func TestGuardBrowserNilPolicyBehavesAsToday(t *testing.T) {
	for name, hosts := range map[string]func() *HostPolicy{
		"nil getter":   nil,
		"nil policy":   func() *HostPolicy { return nil },
		"empty policy": func() *HostPolicy { return NewHostPolicy(nil, nil, "") },
	} {
		t.Run(name, func(t *testing.T) {
			h := GuardBrowser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}), GuardExempts{}, hosts)
			req := httptest.NewRequest(http.MethodGet, "/config.json", nil)
			req.Host = "gadak.example.com" // would be admitted by a policy; must not be here
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status %d, want 403; body %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// The mirror REST gate must honor the policy too (GDK-1966): a serve-scope
// path from a policy Host reaches the handler with no Bearer — membership is
// the credential — while the same path from an unlisted DNS Host keeps
// demanding one (401 pairing_rejected here: this fixture has no pairing
// store, so the exempt probe never fires and the guard itself refuses
// earlier; through the full stack an unlisted name is 403 forbidden_host).
func TestMirrorGateHonorsHostPolicy(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	h.SetHostPolicy(NewHostPolicy(nil, nil, "vps.example.ts.net"))

	req := httptest.NewRequest(http.MethodGet, apiBase+"bootstrap/", nil)
	req.Host = "vps.example.ts.net:7777"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy Host bootstrap status %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, apiBase+"bootstrap/", nil)
	req.Host = "evil.example.com"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unlisted Host status %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	if got := decode[map[string]string](t, rec)["error"]; got != "forbidden_host" {
		t.Fatalf("error %q, want forbidden_host", got)
	}
}

// ── the tailscale probe ─────────────────────────────────────────────────────

// fakeTailscale writes a `tailscale` executable into a temp dir and points
// PATH at it. body is what the binary prints; sleep, when set, stalls past
// any timeout the test passes.
func fakeTailscale(t *testing.T, body string, sleep time.Duration) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script probe fixture does not exec on windows")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n"
	if sleep > 0 {
		script += "sleep " + sleep.String() + "\n"
	}
	script += "printf '%s' " + shellQuote(body) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "tailscale"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return dir
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func TestTailscaleDNSNameParsesSelfDNSName(t *testing.T) {
	fakeTailscale(t, `{"Self":{"DNSName":"Vps.Example.TS.Net."}}`, 0)
	if got := TailscaleDNSName(); got != "Vps.Example.TS.Net" {
		t.Fatalf("TailscaleDNSName() = %q, want Vps.Example.TS.Net (dot stripped)", got)
	}
}

func TestTailscaleDNSNameEmptyWhenNoSelfName(t *testing.T) {
	fakeTailscale(t, `{"Self":{}}`, 0)
	if got := TailscaleDNSName(); got != "" {
		t.Fatalf("TailscaleDNSName() = %q, want \"\"", got)
	}
}

func TestTailscaleDNSNameGarbageOutputIsEmpty(t *testing.T) {
	fakeTailscale(t, "not json at all", 0)
	if got := TailscaleDNSName(); got != "" {
		t.Fatalf("TailscaleDNSName() = %q, want \"\"", got)
	}
}

func TestTailscaleDNSNameNotOnPathIsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if got := TailscaleDNSName(); got != "" {
		t.Fatalf("TailscaleDNSName() = %q, want \"\"", got)
	}
}

func TestTailscaleDNSNameTimeoutIsEmpty(t *testing.T) {
	fakeTailscale(t, `{"Self":{"DNSName":"never.example.ts.net."}}`, 30*time.Second)
	start := time.Now()
	if got := tailscaleDNSName(50 * time.Millisecond); got != "" {
		t.Fatalf("tailscaleDNSName(timed out) = %q, want \"\"", got)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("probe took %s — the timeout did not bound it", elapsed)
	}
}
