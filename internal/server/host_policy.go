package server

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// The host policy (GDK-1966): the DNS names this serve answers by. The
// rebinding guard admits loopback, *.localhost, and IP literals by
// construction; a DNS name is exactly what it exists to refuse. But the
// product decision says reaching the tailnet address IS the credential — a
// name this list carries is a Host the serve answers on purpose, the way
// `tailscale serve` and --public-url forward it.
//
// The policy is built once at serve start (NewHostPolicy) and installed via
// Handler.SetHostPolicy; both guard layers (the top-level mux's and the
// Handler's own) and the mirror gate read the same live pointer, so nothing
// else can widen between them. Membership admits the Host — it does not
// mint a token, a shell, or anything else the pairing gates own.

// hostPolicyEntry is one admitted name with the surface that contributed it,
// so the startup line can say where each name came from.
type hostPolicyEntry struct {
	name   string // lower-cased, no port, no trailing dot
	source string // "--public-url" | "serve.publicUrls" | "tailscale"
}

// HostPolicy is the ordered set of admitted DNS names. Construct with
// NewHostPolicy; a nil or empty policy behaves exactly as the guard did
// before this type existed (every DNS name refused). All methods are
// nil-safe and read-only, so the guard's per-request consult needs no lock.
type HostPolicy struct {
	entries []hostPolicyEntry
}

// NewHostPolicy builds the policy from its three sources, in order: the
// repeatable --public-url flag, the stored serve.publicUrls config, and the
// Tailscale MagicDNS self name. Entries reduce to bare DNS names (scheme,
// port, and trailing dot drop); duplicates by name keep the first source;
// entries that do not name a host are skipped rather than widening
// anything — the flag and config surfaces validate their inputs and refuse
// with a sentence of their own.
func NewHostPolicy(flagURLs, configURLs []string, tsName string) *HostPolicy {
	p := &HostPolicy{}
	for _, u := range flagURLs {
		p.add(u, "--public-url")
	}
	for _, u := range configURLs {
		p.add(u, "serve.publicUrls")
	}
	if name, ok := policyHostName(tsName); ok && !p.Allows(name) {
		p.entries = append(p.entries, hostPolicyEntry{name: name, source: "tailscale"})
	}
	return p
}

func (p *HostPolicy) add(entry, source string) {
	name, ok := policyHostName(entry)
	if !ok || p.Allows(name) {
		return
	}
	p.entries = append(p.entries, hostPolicyEntry{name: name, source: source})
}

// policyHostName reduces one policy entry to the DNS name it admits: the
// host of an http(s) origin ("https://Name:8443/" → "name"), or a bare
// host[:port]. ok is false for anything else — empty entries, unparseable
// strings, paths, non-http schemes, IP literals (allowedHost admits those
// by construction; the policy names DNS names only).
func policyHostName(entry string) (string, bool) {
	v := strings.TrimSpace(entry)
	if v == "" {
		return "", false
	}
	var host string
	if strings.Contains(v, "://") {
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			(u.Path != "" && u.Path != "/") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", false
		}
		host = u.Hostname()
	} else {
		if strings.ContainsAny(v, "/?#") {
			return "", false
		}
		host = v
		// One trailing :port is a host[:port]; any other colon (or a
		// non-numeric port) means the string never named a host.
		if i := strings.LastIndex(v, ":"); i >= 0 {
			if port := v[i+1:]; port == "" || strings.TrimLeft(port, "0123456789") != "" {
				return "", false
			}
			host = v[:i]
		}
		// Bracketed IPv6 bare form: unbracket so the IP-literal refusal
		// below sees it (allowedHost admits IP literals on its own).
		if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
			host = host[1 : len(host)-1]
		}
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || net.ParseIP(host) != nil {
		return "", false
	}
	return host, true
}

// Allows reports whether name (a bare DNS name, any case) is on the policy.
// A nil policy admits nothing.
func (p *HostPolicy) Allows(name string) bool {
	if p == nil {
		return false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for _, e := range p.entries {
		if e.name == name {
			return true
		}
	}
	return false
}

// Names lists the admitted names in admission order (flag, config, probe).
func (p *HostPolicy) Names() []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.entries))
	for _, e := range p.entries {
		out = append(out, e.name)
	}
	return out
}

// Empty reports whether nothing is admitted — the pre-policy guard surface.
func (p *HostPolicy) Empty() bool {
	return p == nil || len(p.entries) == 0
}

// Describe is the one-line form the serve startup log prints, each name
// with its source: "vps.example.ts.net (tailscale), gadak.example.com
// (--public-url)". Empty policy describes as "".
func (p *HostPolicy) Describe() string {
	if p.Empty() {
		return ""
	}
	parts := make([]string, 0, len(p.entries))
	for _, e := range p.entries {
		parts = append(parts, e.name+" ("+e.source+")")
	}
	return strings.Join(parts, ", ")
}

// TailscaleDNSName is this machine's Tailscale MagicDNS name, best-effort:
// the tailscale binary on PATH, `status --json` with a 2s timeout, the
// Self.DNSName field with its trailing dot stripped. Every failure mode —
// binary missing, command failing, timeout, unparseable output — answers ""
// after exactly one stderr line; the serve must start without a tailnet.
func TailscaleDNSName() string {
	return tailscaleDNSName(2 * time.Second)
}

func tailscaleDNSName(timeout time.Duration) string {
	path, err := exec.LookPath("tailscale")
	if err != nil {
		log.Printf("serve: no tailscale binary on PATH — MagicDNS name not added to allowed hosts")
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "status", "--json")
	// The timeout kills `tailscale`, but Output() also waits for the stdout
	// pipe to close, and a child the CLI spawned can hold it open past the
	// kill. WaitDelay bounds that wait so a hung probe costs the serve one
	// timeout, never a hang at startup.
	cmd.WaitDelay = timeout
	out, err := cmd.Output()
	if err != nil {
		log.Printf("serve: tailscale status --json failed (%v) — MagicDNS name not added to allowed hosts", err)
		return ""
	}
	var st struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &st); err != nil {
		log.Printf("serve: tailscale status --json: %v — MagicDNS name not added to allowed hosts", err)
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(st.Self.DNSName), ".")
}
