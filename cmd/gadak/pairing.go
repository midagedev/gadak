package main

// gadak pairing — device tokens for the home serve's gated surfaces. An
// origin-scope token rides the origin passthrough (GDK-433): the remote
// side consumes the mint output with `gadak init --pairing-code`. A
// serve-scope token (GDK-797) opens the mirror REST a phone
// companion reads — bootstrap, detail, search, feed, and the
// comment/transition writes — and nothing else. A terminal-scope token
// (GDK-863) opens a shell on this machine — the terminal pane — and
// nothing else; it is never a default, and revoking it closes the shells
// it opened. While any active token exists, all three surfaces require
// their Bearer.

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
	"github.com/midagedev/gadak/internal/serveaddr"
	"github.com/midagedev/gadak/internal/store"
)

const pairingUsage = "usage: gadak pairing mint --label NAME [--scope origin|serve|terminal] [--ttl 90d] [--endpoint URL] [--json] | pairing list [--json] | pairing revoke <label|hash-prefix>"

// pairingRevokeUsage is the revoke selector: exact label, or a hash prefix
// of at least minPrefix (8) hex characters — pairing list prints 8, longer
// prefixes still match (internal/pairing.Revoke).
const pairingRevokeUsage = "usage: gadak pairing revoke <label|hash-prefix>"

// defaultPairingTTL is 90 days: a device token is revocable, so it does
// not need a short life, but an unattended one should not outlive a
// quarter by default either.
const defaultPairingTTL = 90 * 24 * time.Hour

// homeRoutingTTL is long on purpose: an expiring routing token would
// re-open the 401 cliff ensureHomeRoutingToken exists to close. Rotation
// is explicit (`pairing mint --label _home`), not expiry.
const homeRoutingTTL = 10 * 365 * 24 * time.Hour

func cmdPairing(args []string) error {
	if wantsHelp(args) {
		printHelp("pairing")
		return nil
	}
	// No args (and bare flags like --json) default to list, matching
	// views.go / dashboards.go / recipes.go / config.go.
	sub, rest := "list", args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "mint", "list", "revoke":
			sub, rest = args[0], args[1:]
		default:
			return usageError("pairing", pairingUsage)
		}
	}
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

// pairingDir is the profile directory the token store lives in. Pairing
// protects both gated surfaces of a home serve — the origin passthrough
// (standalone, GDK-433) and the mirror REST a phone companion
// reads (GDK-797) — so both workspace kinds may mint; what stays closed
// for a connected workspace is the passthrough itself (origin_rest.go
// 404s it). A workspace that is itself paired away cannot mint: its home
// is another machine.
func pairingDir() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if rem, err := origin.PairedStatus(cfg); err != nil {
		return "", err
	} else if rem != nil {
		return "", fmt.Errorf("%s — mint and revoke run on the home machine", pairedStatusLine(cfg, rem))
	}
	if !cfg.HasCredential() {
		return "", config.ErrNotConfigured
	}
	dir := cfg.Directory()
	if dir == "" {
		return "", errors.New("pairing: profile directory not found")
	}
	return dir, nil
}

func pairedStatusLine(cfg *config.Config, rem *pairing.Remote) string {
	line := fmt.Sprintf("this workspace is paired with %q (%s)", rem.Label, rem.Endpoint)
	if rem.Label == "" {
		line = fmt.Sprintf("this workspace is paired with %s", rem.Endpoint)
	}
	if cfg != nil && strings.TrimSpace(cfg.TokenOwner) != "" {
		line += " as " + cfg.TokenOwner
	}
	return line
}

func pairingMint(args []string) error {
	fs := newFlagSet("pairing mint")
	// ttlDefault renders defaultPairingTTL in the flag's <N><unit> syntax so
	// the 90-day default has one owner.
	ttlDefault := fmt.Sprintf("%dd", int(defaultPairingTTL/(24*time.Hour)))
	label := fs.String("label", "", "device name shown in `gadak pairing list` (required)")
	scope := fs.String("scope", "origin", "what the token opens: origin = origin passthrough for a paired gadak (default), serve = the mirror REST for a paired client (a phone companion), terminal = a shell on this machine (never a default — see `gadak help pairing`)")
	ttlFlag := fs.String("ttl", ttlDefault, "token lifetime: <N><d|h|m|s>, e.g. 90d or 12h")
	endpoint := fs.String("endpoint", "", "URL remote devices reach this serve at (default: this machine's live serve address)")
	asJSON := fs.Bool("json", false, "emit JSON")
	if _, err := parseAround(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*label) == "" {
		return errors.New("pairing mint requires --label NAME")
	}
	switch strings.TrimSpace(*scope) {
	case pairing.ScopeOrigin, pairing.ScopeServe, pairing.ScopeTerminal:
	default:
		return fmt.Errorf("unknown --scope %q: want origin (a paired gadak riding the passthrough), serve (a paired client reading the mirror REST), or terminal (a shell on this machine)", strings.TrimSpace(*scope))
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
	if strings.TrimSpace(*label) == pairing.HomeLabel {
		// GDK-450: rotation, not a device offer. --json is ignored so
		// stdout stays empty (cannot be piped into init --pairing-code).
		return pairingMintHome(dir, cfg, strings.TrimRight(strings.TrimSpace(*endpoint), "/"))
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
	token, meta, err := pairing.MintScoped(dir, *label, strings.TrimSpace(*scope), ttl, time.Now())
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
	if err := writePairingMintOutput(*asJSON, offer, *label, ep, strings.TrimSpace(*scope), meta); err != nil {
		return err
	}
	// The _home routing token is a standalone home's concern: its local
	// writes ride the passthrough this serve fronts. A connected
	// workspace's writes go straight to its site, and the routing file
	// (remote-origin.json) would make origin.pairedRemote read this
	// workspace as paired with its own serve — never written here.
	if cfg.IsStandalone() {
		if err := ensureHomeRoutingToken(dir, cfg, ep); err != nil {
			return err
		}
	}
	return nil
}

// writePairingMintOutput is the device-mint output contract (GDK-456).
// Default: stdout is exactly the offer line (pipe target). --json: one
// JSON object on stdout with the same offer plus label/scope/endpoint/
// expires_at. Human stderr says what the token is for — which machine
// consumes it depends on the scope; the offer is reusable until expiry
// (not "shown once").
func writePairingMintOutput(asJSON bool, offer, label, endpoint, scope string, meta pairing.Meta) error {
	expiry := pairing.FormatExpiry(meta.ExpiresAt)
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(struct {
			Offer     string `json:"offer"`
			Label     string `json:"label"`
			Scope     string `json:"scope"`
			Endpoint  string `json:"endpoint"`
			ExpiresAt string `json:"expires_at"`
		}{offer, label, scope, endpoint, expiry})
	}
	fmt.Println(offer)
	switch scope {
	case pairing.ScopeServe:
		fmt.Fprintln(os.Stderr, pairingMintServeHint())
	case pairing.ScopeTerminal:
		fmt.Fprintln(os.Stderr, pairingMintTerminalHint())
	default:
		fmt.Fprintln(os.Stderr, pairingMintRemoteHint(label))
	}
	fmt.Fprintf(os.Stderr, "paired device %q — offer reusable until %s; revoke with: gadak pairing revoke %s\n",
		label, expiry, meta.Hash[:8])
	return nil
}

// pairingMintRemoteHint is the one thing an origin-scope mint must say:
// how to consume the offer on the other machine. The suggested profile
// name is the device label — the remote picks its own profile, and the
// home's profile name means nothing over there; the label is the one name
// both sides already share.
func pairingMintRemoteHint(label string) string {
	if label == "" {
		label = "<name>"
	}
	return fmt.Sprintf("on the other machine: gadak --workspace %s init --pairing-code-stdin  (paste the line above)", label)
}

// pairingMintServeHint names the surface a serve token opens, so the offer
// line is not pasted where an origin-scope offer belongs: a phone
// companion is a REST client, not a second gadak workspace.
func pairingMintServeHint() string {
	return "this is a serve token: in the phone app it opens this serve's mirror API (reads + comment/transition writes) and nothing else"
}

// pairingMintTerminalHint names what a terminal token opens, and says the
// thing that makes it different from the other two: the other scopes leak
// data if they leak, this one leaks the machine. It is never a default —
// `--scope terminal` has to be typed — and it is the one scope worth
// giving a short --ttl.
func pairingMintTerminalHint() string {
	return "this is a terminal token: it opens a shell on this machine (the gadak terminal pane) and nothing else — not the mirror, not the origin passthrough.\n" +
		"treat it like an SSH key: a leaked serve token leaks this workspace's data, a leaked terminal token leaks this machine.\n" +
		"revoking it closes the shells it opened, within a couple of seconds, without stopping the serve."
}

// ensureHomeRoutingToken is the single owner of "_home is valid". Once one
// token exists the gate takes Bearer only — no loopback bypass — and the
// home machine's writes route through the same passthrough (GDK-333 single
// persist owner). Standalone excludes the file from the paired-remote
// branch (origin.pairedRemote), so here it only ever carries the routing
// token localRoutingToken reads.
//
// Called after every device mint. If remote-origin.json's token is missing
// or no longer active in the store while the gate is on, it reissues
// `_home` and rewrites the file — the recovery path for a routing token
// revoked by an older build (GDK-450).
func ensureHomeRoutingToken(dir string, cfg *config.Config, mintEndpoint string) error {
	rem, err := pairing.LoadRemote(dir)
	if err != nil {
		return err
	}
	now := time.Now()
	if rem != nil {
		v, aerr := pairing.Authorize(dir, rem.Token, now)
		if aerr != nil {
			return aerr
		}
		if v == pairing.VerdictAccept {
			return nil
		}
		if v == pairing.VerdictOff {
			// Gate off: no active tokens, local writes need no bearer.
			return nil
		}
	} else {
		v, aerr := pairing.Authorize(dir, "", now)
		if aerr != nil {
			return aerr
		}
		if v == pairing.VerdictOff {
			return nil
		}
	}
	first := rem == nil
	if _, err := issueHomeRoutingToken(dir, cfg, mintEndpoint); err != nil {
		return err
	}
	if first {
		fmt.Fprintln(os.Stderr, "minted a _home routing token so this machine's own writes keep passing the gate")
	} else {
		fmt.Fprintln(os.Stderr, "reissued _home routing token so this machine's own writes keep passing the gate")
	}
	return nil
}

// pairingMintHome is `pairing mint --label _home`: routing-token rotation,
// not a device offer. stdout stays empty so the line cannot be piped into
// `init --pairing-code`. Standalone only — a connected workspace's writes
// go straight to its site, and the routing file here would make
// origin.pairedRemote read this workspace as paired with its own serve.
func pairingMintHome(dir string, cfg *config.Config, endpoint string) error {
	if !cfg.IsStandalone() {
		return errors.New("_home is a standalone workspace's routing token — this workspace's writes go to its site directly, and a routing file here would misread this workspace as paired")
	}
	ep, err := resolveHomeEndpoint(cfg, dir, endpoint)
	if err != nil {
		return err
	}
	meta, err := issueHomeRoutingToken(dir, cfg, ep)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "rotated _home routing token (%s) — local writes keep passing the gate\n", meta.Hash[:8])
	return nil
}

func resolveHomeEndpoint(cfg *config.Config, dir, endpoint string) (string, error) {
	ep := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if ep == "" {
		ep = advertisedEndpoint(cfg)
	}
	if ep == "" {
		if rem, err := pairing.LoadRemote(dir); err == nil && rem != nil {
			ep = strings.TrimRight(strings.TrimSpace(rem.Endpoint), "/")
		}
	}
	if ep == "" {
		return "", errors.New("no live serve found for this profile; start `gadak serve` first or pass --endpoint <url>")
	}
	u, err := url.Parse(ep)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("bad endpoint %q: want an http(s) URL", ep)
	}
	return ep, nil
}

func issueHomeRoutingToken(dir string, cfg *config.Config, mintEndpoint string) (pairing.Meta, error) {
	token, meta, err := pairing.Rotate(dir, pairing.HomeLabel, homeRoutingTTL, time.Now())
	if err != nil {
		return pairing.Meta{}, err
	}
	ep := strings.TrimRight(strings.TrimSpace(mintEndpoint), "/")
	if ep == "" {
		ep = advertisedEndpoint(cfg)
	}
	if ep == "" {
		if rem, rerr := pairing.LoadRemote(dir); rerr == nil && rem != nil {
			ep = rem.Endpoint
		}
	}
	if ep == "" {
		return pairing.Meta{}, errors.New("no live serve found for this profile; start `gadak serve` first or pass --endpoint <url>")
	}
	if err := pairing.SaveRemote(dir, pairing.Remote{Endpoint: ep, Token: token, Label: pairing.HomeLabel}); err != nil {
		return pairing.Meta{}, err
	}
	return meta, nil
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

// advertisedEndpoint turns a live UI-serve listen address into the URL
// form --endpoint validation wants. Discovery uses the home-root run
// directory (serveaddr), not leftover serve-origin.json. An explicit
// --endpoint is not normalized: making the user name the scheme is the point.
func advertisedEndpoint(cfg *config.Config) string {
	dir, err := serveaddr.Dir()
	if err != nil {
		return ""
	}
	want := ""
	if cfg != nil {
		want = cfg.ProfileName()
	}
	for _, rec := range serveaddr.List(dir) {
		got := origin.ProbeGadakOnPort(rec.Port, 0)
		if !got.IsGadak {
			continue
		}
		if got.Profile != want {
			continue
		}
		return endpointFromAdvertise(rec.Addr)
	}
	return ""
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

// pairingListTime is the table/JSON timestamp for pairing list columns.
const pairingListTime = "2006-01-02T15:04Z"

// pairingListRow is one pairing-list table row. JSON tags are the same
// columns the human table prints — never the plaintext token, never the
// full hash (only the 8-char prefix pairing list already showed).
type pairingListRow struct {
	Hash     string `json:"hash"`
	Label    string `json:"label"`
	Scope    string `json:"scope"`
	Created  string `json:"created"`
	Expires  string `json:"expires"`
	LastUsed string `json:"last_used"`
	State    string `json:"state"`
}

func pairingListRowFrom(m pairing.Meta, now time.Time) pairingListRow {
	last := "-"
	if m.LastUsedAt != nil {
		last = m.LastUsedAt.UTC().Format(pairingListTime)
	}
	state := "active"
	if m.RevokedAt != nil {
		state = "revoked " + m.RevokedAt.UTC().Format(pairingListTime)
	} else if now.After(m.ExpiresAt) {
		state = "expired"
	}
	scope := m.Scope
	if m.Label == pairing.HomeLabel {
		scope = pairing.ScopeLocalRouting
	}
	return pairingListRow{
		Hash:     m.Hash[:8],
		Label:    m.Label,
		Scope:    scope,
		Created:  m.CreatedAt.UTC().Format(pairingListTime),
		Expires:  m.ExpiresAt.UTC().Format(pairingListTime),
		LastUsed: last,
		State:    state,
	}
}

func pairingList(args []string) error {
	fs := newFlagSet("pairing list")
	asJSON := fs.Bool("json", false, "emit JSON")
	if _, err := parseAround(fs, args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if rem, err := origin.PairedStatus(cfg); err != nil {
		return err
	} else if rem != nil {
		// This workspace holds no tokens of its own — it is a client of
		// someone else's serve. --json still has to answer in JSON here:
		// the whole point of the flag is that stdout is machine-readable,
		// and a consumer that has to parse an English sentence on one
		// branch does not have a JSON verb. `tokens` stays present and
		// empty so a reader can take the same path on both branches.
		if *asJSON {
			paired := map[string]any{"endpoint": rem.Endpoint, "label": rem.Label}
			// Same nil guard pairedStatusLine uses.
			if cfg != nil {
				if owner := strings.TrimSpace(cfg.TokenOwner); owner != "" {
					paired["owner"] = owner
				}
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"paired": paired,
				"tokens": jsonList([]pairingListRow{}),
			})
		}
		fmt.Println(pairedStatusLine(cfg, rem))
		return nil
	}
	dir, err := pairingDir()
	if err != nil {
		return err
	}
	toks, err := pairing.List(dir)
	if err != nil {
		return err
	}
	now := time.Now()
	if *asJSON {
		// Empty --json is {"tokens":[]} (jsonList), never the human
		// sentence and never null: a JSON consumer that gets English on
		// stdout is a bug. pairingGateOpenLine stays on stderr.
		rows := make([]pairingListRow, 0, len(toks))
		for _, m := range toks {
			rows = append(rows, pairingListRowFrom(m, now))
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"tokens": jsonList(rows)}); err != nil {
			return err
		}
	} else if len(toks) == 0 {
		fmt.Println("no pairing tokens — the origin passthrough is open to loopback (implicit trust)")
	} else {
		fmt.Printf("%-8s  %-20s  %-14s  %-20s  %-20s  %-20s  %s\n",
			"HASH", "LABEL", "SCOPE", "CREATED", "EXPIRES", "LAST-USED", "STATE")
		for _, m := range toks {
			row := pairingListRowFrom(m, now)
			fmt.Printf("%-8s  %-20s  %-14s  %-20s  %-20s  %-20s  %s\n",
				row.Hash, truncate([]byte(row.Label), 20), row.Scope,
				row.Created, row.Expires, row.LastUsed, row.State)
		}
	}
	if line := pairingGateOpenLine(dir); line != "" {
		fmt.Fprintln(os.Stderr, line)
	}
	return nil
}

// pairingGateOpenLine is the GDK-481 copy when Authorize would return
// VerdictOff (no active token, including _home). Empty when the gate is
// still closed. _home is counted: Authorize includes ScopeLocalRouting, so
// revoking the last device token while _home remains must not claim the
// gate is open.
func pairingGateOpenLine(dir string) string {
	v, err := pairing.Authorize(dir, "", time.Now())
	if err != nil || v != pairing.VerdictOff {
		return ""
	}
	return "no active tokens remain — the gate is open again; stop the serve to cut access"
}

func pairingRevoke(args []string) error {
	fs := newFlagSet("pairing revoke")
	rest, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageError("pairing revoke", pairingRevokeUsage)
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
	if line := pairingGateOpenLine(dir); line != "" {
		fmt.Fprintln(os.Stderr, line)
	}
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
	// workspace. A profile that is already paired is refused by
	// refuseIfPairedOrigin in cmdInit, before this function runs.
	if cfg.IsStandalone() || cfg.Site != "" || cfg.Email != "" || cfg.Token != "" {
		return errors.New("this workspace already owns an origin; pair into a fresh workspace: gadak --workspace <name> init --pairing-code …")
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
	skill := autoInstallSkill(os.Stderr)
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(struct {
			Profile  string `json:"profile"`
			Endpoint string `json:"endpoint"`
			Label    string `json:"label"`
			Account  string `json:"account"`
			Path     string `json:"path"`
			Skill    string `json:"skill"`
		}{displayProfileName(config.Profile()), offer.Endpoint, offer.Label, me.DisplayName, p, skill})
	}
	fmt.Printf("paired with %s as %s — saved %s\n", offer.Endpoint, me.DisplayName, p)
	printSkillAutoResult(skill)
	printPairedInitNextSteps()
	return nil
}

func printPairedInitNextSteps() {
	fmt.Printf(`
next:
  gadak sync                    fill the mirror from the home serve
  gadak status                  confirm the pairing
  If a command fails with "cannot reach the home serve", start gadak serve on the home machine.

docs/AGENT_SETUP.md has one paste per agent; docs/RECIPES.md has the questions
JQL cannot ask.
`)
}

// refuseIfPairedOrigin is the single owner of "this profile's origin is a
// remote gadak serve — init cannot rebind it". Every init path (bare,
// --standalone, site flags, --pairing-code) comes through here.
//
// Standalone is excluded: the home machine stores its routing token in the
// same remote-origin.json file (origin.pairedRemote documents the same
// split). LoadRemote returning a credential is therefore not sufficient on
// its own to mean "paired origin".
func refuseIfPairedOrigin(cfg *config.Config) error {
	rem, err := origin.PairedStatus(cfg)
	if err != nil {
		return err
	}
	if rem == nil {
		return nil
	}
	label := rem.Label
	if label == "" {
		label = rem.Endpoint
	}
	return fmt.Errorf("this workspace is paired with %q (%s) — to start over, use a new workspace: gadak --workspace <name> init", label, rem.Endpoint)
}
