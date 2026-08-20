package main

// gadak pairing — device tokens for the serve's origin passthrough gate
// (GDK-433). The home machine mints one token per device; while any active
// token exists, everything under /api/v1/origin requires it as a Bearer.
// The remote side consumes the mint output with `gadak init
// --pairing-code`.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/store"
)

const pairingUsage = "usage: gadak pairing mint --label NAME [--ttl 90d] [--endpoint URL] | pairing list | pairing revoke <label|hash-prefix>"

// defaultPairingTTL is 90 days: a device token is revocable, so it does
// not need a short life, but an unattended one should not outlive a
// quarter by default either.
const defaultPairingTTL = 90 * 24 * time.Hour

func cmdPairing(args []string) error {
	if wantsHelp(args) {
		printHelp("pairing")
		return nil
	}
	if len(args) == 0 {
		return usageError("pairing", pairingUsage)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "mint":
		return pairingMint(rest)
	case "list":
		return pairingList(rest)
	case "revoke":
		return pairingRevoke(rest)
	default:
		return usageError("pairing", pairingUsage)
	}
}

// parseTTL accepts one integer with a single unit — "90d", "24h", "30m",
// "45s". time.ParseDuration rejects "d", and inventing compound syntax
// ("1d12h") is surface nobody asked for; a clear error beats guessing.
func parseTTL(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty ttl")
	}
	unit := s[len(s)-1]
	digits := s[:len(s)-1]
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("bad ttl %q: want <N><unit> with unit d, h, m, or s (e.g. 90d, 24h)", s)
	}
	switch unit {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 's':
		return time.Duration(n) * time.Second, nil
	default:
		return 0, fmt.Errorf("bad ttl %q: unit must be d, h, m, or s", s)
	}
}

// pairingDir is the profile directory the token store lives in, with the
// workspace-kind gate: pairing protects a standalone serve's passthrough;
// on a connected workspace the passthrough is a 404 and a minted token
// would be a lie the user cannot see through.
func pairingDir() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if !cfg.IsStandalone() {
		return "", errors.New("pairing is for standalone workspaces — the gate sits on the serve's origin passthrough, which a connected workspace does not serve")
	}
	dir := cfg.Directory()
	if dir == "" {
		return "", errors.New("pairing: profile directory not found")
	}
	return dir, nil
}

func pairingMint(args []string) error {
	fs := newFlagSet("pairing mint")
	label := fs.String("label", "", "device name shown in `gadak pairing list` (required)")
	ttlFlag := fs.String("ttl", "90d", "token lifetime: <N><d|h|m|s>, e.g. 90d or 12h")
	endpoint := fs.String("endpoint", "", "URL remote devices reach this serve at (default: this machine's live serve address)")
	if _, err := parseAround(fs, args); err != nil {
		return err
	}
	ttl, err := parseTTL(*ttlFlag)
	if err != nil {
		return err
	}
	dir, err := pairingDir()
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Resolve the endpoint before minting: a mint without a reachable
	// endpoint is an orphan token nobody can use, and it would already
	// have flipped the gate on.
	ep := strings.TrimRight(strings.TrimSpace(*endpoint), "/")
	if ep == "" {
		ep = advertisedEndpoint(cfg)
		if ep == "" {
			return errors.New("no live serve found for this profile; start `gadak serve` first or pass --endpoint <url>")
		}
	}
	u, err := url.Parse(ep)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("bad endpoint %q: want an http(s) URL", ep)
	}
	if host := u.Hostname(); isLoopbackHost(host) {
		fmt.Fprintf(os.Stderr, "warning: endpoint %s is loopback — remote devices cannot reach it; pass --endpoint with your tailnet URL (e.g. https://<machine>.<tailnet>.ts.net)\n", ep)
	}
	token, meta, err := pairing.Mint(dir, *label, ttl, time.Now())
	if err != nil {
		return err
	}
	offer, err := pairing.EncodeOffer(pairing.Offer{
		V:         pairing.OfferV1,
		Endpoint:  ep,
		Token:     token,
		ExpiresAt: pairing.FormatExpiry(meta.ExpiresAt),
		Label:     *label,
	})
	if err != nil {
		return err
	}
	// stdout is exactly the offer: the remote side pipes it whole
	// (`gadak init --pairing-code "$(gadak pairing mint …)"`).
	fmt.Println(offer)
	fmt.Fprintf(os.Stderr, "paired device %q — token shown once, expires %s; revoke with: gadak pairing revoke %s\n",
		*label, pairing.FormatExpiry(meta.ExpiresAt), meta.Hash[:8])
	if err := ensureHomeRoutingToken(dir, cfg, ep); err != nil {
		return err
	}
	return nil
}

// ensureHomeRoutingToken keeps the home machine's own CLI working after the
// first mint. Once one token exists the gate takes Bearer only — no loopback
// bypass — and the home machine's writes route through the same passthrough
// (GDK-333 single persist owner), so without a token of its own the first
// mint would 401 this very machine. Standalone excludes the file from the
// paired-remote branch (origin.pairedRemote), so here it only ever carries
// the routing token localRoutingToken reads.
func ensureHomeRoutingToken(dir string, cfg *config.Config, mintEndpoint string) error {
	rem, err := pairing.LoadRemote(dir)
	if err != nil || rem != nil {
		return err
	}
	// Ten years: an expiring routing token would re-open the cliff this
	// exists to close. `gadak pairing revoke _home` stays available.
	token, _, err := pairing.Mint(dir, "_home", 10*365*24*time.Hour, time.Now())
	if err != nil {
		return err
	}
	ep := advertisedEndpoint(cfg)
	if ep == "" {
		ep = mintEndpoint
	}
	if err := pairing.SaveRemote(dir, pairing.Remote{Endpoint: ep, Token: token, Label: "_home"}); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "minted a _home routing token so this machine's own writes keep passing the gate")
	return nil
}

// isLoopbackHost names the hosts only this machine can dial. Used for the
// mint warning, never for a trust decision (GDK-433 prior art: a tunnel
// can look like loopback).
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// advertisedEndpoint turns the advertise file's machine-written listen
// address (host:port, no scheme — WriteAdvertise stores the bind addr) into
// the URL form --endpoint validation wants. An explicit --endpoint is not
// normalized: making the user name the scheme is the point.
func advertisedEndpoint(cfg *config.Config) string {
	return endpointFromAdvertise(origin.AdvertisedAddr(cfg))
}

func endpointFromAdvertise(addr string) string {
	if addr == "" {
		return ""
	}
	if !strings.Contains(addr, "://") {
		return "http://" + addr
	}
	return addr
}

func pairingList(args []string) error {
	fs := newFlagSet("pairing list")
	if _, err := parseAround(fs, args); err != nil {
		return err
	}
	dir, err := pairingDir()
	if err != nil {
		return err
	}
	toks, err := pairing.List(dir)
	if err != nil {
		return err
	}
	if len(toks) == 0 {
		fmt.Println("no pairing tokens — the origin passthrough is open to loopback (implicit trust)")
		return nil
	}
	fmt.Printf("%-8s  %-20s  %-9s  %-20s  %-20s  %-20s  %s\n",
		"HASH", "LABEL", "SCOPE", "CREATED", "EXPIRES", "LAST-USED", "STATE")
	for _, m := range toks {
		last := "-"
		if m.LastUsedAt != nil {
			last = m.LastUsedAt.UTC().Format("2006-01-02T15:04Z")
		}
		state := "active"
		if m.RevokedAt != nil {
			state = "revoked " + m.RevokedAt.UTC().Format("2006-01-02T15:04Z")
		} else if time.Now().After(m.ExpiresAt) {
			state = "expired"
		}
		fmt.Printf("%-8s  %-20s  %-9s  %-20s  %-20s  %-20s  %s\n",
			m.Hash[:8], truncate([]byte(m.Label), 20), m.Scope,
			m.CreatedAt.UTC().Format("2006-01-02T15:04Z"),
			m.ExpiresAt.UTC().Format("2006-01-02T15:04Z"),
			last, state)
	}
	return nil
}

func pairingRevoke(args []string) error {
	fs := newFlagSet("pairing revoke")
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageError("pairing revoke", "usage: gadak pairing revoke <label|hash-prefix>")
	}
	dir, err := pairingDir()
	if err != nil {
		return err
	}
	meta, err := pairing.Revoke(dir, rest[0], time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("revoked %q (%s)\n", meta.Label, meta.Hash[:8])
	return nil
}

// initPaired is cmdInit's --pairing-code path: decode the offer, prove it
// against the serve with one /rest/api/3/myself round trip over the exact
// transport the workspace would use, and only then write anything
// (verify-before-save, GDK-433). The offer string never reaches a log or
// an error message; on failure nothing is written and the cause — refused
// token vs unreachable serve vs bad offer — is named distinctly.
func initPaired(cfg *config.Config, code string, fromStdin bool, jsonOut bool) error {
	if fromStdin {
		if code != "" {
			return errors.New("use either --pairing-code or --pairing-code-stdin, not both")
		}
		b, err := io.ReadAll(initStdin)
		if err != nil {
			return fmt.Errorf("reading pairing offer from stdin: %w", err)
		}
		code = strings.TrimSpace(string(b))
	}
	offer, err := pairing.DecodeOffer(code)
	if err != nil {
		return err
	}
	dir := cfg.Directory()
	if dir == "" {
		return errors.New("pairing: profile directory not found")
	}
	// A workspace is bound to one origin: pairing creates a NEW remote
	// workspace. Re-pairing an existing one (fresh token + revoke of the
	// old, from the home machine) is deliberately not this slice.
	if cfg.IsStandalone() || cfg.Site != "" || cfg.Email != "" || cfg.Token != "" {
		return errors.New("this profile already owns an origin; pair into a fresh profile: gadak --profile <name> init --pairing-code …")
	}
	if rem, err := pairing.LoadRemote(dir); err != nil {
		return fmt.Errorf("pairing: %w", err)
	} else if rem != nil {
		return fmt.Errorf("this profile is already paired with %q; re-pairing lands with a fresh offer in a later slice — use a new profile", rem.Label)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	me, err := origin.VerifyPaired(ctx, offer.Endpoint, offer.Token)
	if err != nil {
		if errors.Is(err, jira.ErrAuth) {
			return errors.New("the serve answered but refused this pairing token (401) — ask the home machine to mint a fresh offer")
		}
		return fmt.Errorf("could not verify the pairing against %s: %w", offer.Endpoint, err)
	}
	if err := pairing.SaveRemote(dir, pairing.Remote{
		Endpoint: offer.Endpoint,
		Token:    offer.Token,
		Label:    offer.Label,
	}); err != nil {
		return err
	}
	cfg.ApplyVerifiedIdentity(me.AccountID, me.DisplayName, store.Now())
	if err := cfg.Save(); err != nil {
		return err
	}
	p, _ := config.Path()
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(struct {
			Profile  string `json:"profile"`
			Endpoint string `json:"endpoint"`
			Label    string `json:"label"`
			Account  string `json:"account"`
			Path     string `json:"path"`
		}{config.Profile(), offer.Endpoint, offer.Label, me.DisplayName, p})
	}
	fmt.Printf("paired with %s as %s — saved %s\n", offer.Endpoint, me.DisplayName, p)
	printInitNextSteps(cfg.WorkspaceKind())
	return nil
}
