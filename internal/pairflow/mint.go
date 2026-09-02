// Package pairflow owns the device-pairing mint flow shared by the CLI
// (`gadak pairing`, GDK-433/450/797) and the desktop app's Devices tab
// (GDK-1047): endpoint resolution and validation, the scoped mint plus
// offer encode, the _home routing-token guard, list row shaping, and the
// two QR encodings (the terminal module matrix and the PNG a phone scans).
//
// One owner so the surfaces cannot drift. The guard that motivates the
// extraction is ensureHomeRoutingToken: once one device token exists the
// serve gate takes Bearer only — no loopback bypass — so a mint that skips
// the home routing token locks the local-origin home out of its own
// passthrough. The CLI carried that guard; before this package the desktop
// would have had to copy it, and a copy is where a guard goes missing.
//
// Presentation stays with the callers: this package returns values (the
// offer, the loopback flag, which _home action ran) and prints nothing.
package pairflow

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// DefaultTTL is 90 days: a device token is revocable, so it does not need
// a short life, but an unattended one should not outlive a quarter by
// default either.
const DefaultTTL = 90 * 24 * time.Hour

// homeRoutingTTL is long on purpose: an expiring routing token would
// re-open the 401 cliff ensureHomeRoutingToken exists to close. Rotation
// is explicit (`pairing mint --label _home`), not expiry.
const homeRoutingTTL = 10 * 365 * 24 * time.Hour

// DefaultTTLFlag renders DefaultTTL in the <N><unit> syntax --ttl speaks,
// so the 90-day default has one owner.
func DefaultTTLFlag() string {
	return fmt.Sprintf("%dd", int(DefaultTTL/(24*time.Hour)))
}

// ParseTTL accepts one integer with a single unit — "90d", "24h", "30m",
// "45s". time.ParseDuration rejects "d", and inventing compound syntax
// ("1d12h") is surface nobody asked for; a clear error beats guessing.
func ParseTTL(s string) (time.Duration, error) {
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

// MintResult is everything a mint produced. The offer is a credential: it
// exists here and in the caller output, never in a log or an error.
type MintResult struct {
	Offer, Label, Scope, Endpoint, ExpiresAt string
	Meta                                     pairing.Meta
	// LoopbackWarning says the explicit endpoint is a loopback address —
	// only a device on this machine can reach it. The caller renders its
	// own copy. (A discovered loopback endpoint is refused instead:
	// LoopbackEndpointError, GDK-1266.)
	LoopbackWarning bool
	// HomeRouting names what ensureHomeRoutingToken did, so the CLI can
	// print its stderr note and the desktop can stay quiet.
	HomeRouting HomeRoutingAction
}

// HomeRoutingAction is what a mint did about the _home routing token.
type HomeRoutingAction string

const (
	// HomeRoutingNone: the existing credential is valid, or the gate is
	// off, or the workspace is connected (no routing token by design).
	HomeRoutingNone HomeRoutingAction = ""
	// HomeRoutingMinted: this mint created the first _home credential.
	HomeRoutingMinted HomeRoutingAction = "minted"
	// HomeRoutingReissued: the stored credential was stale or revoked
	// while the gate was on (GDK-450 recovery).
	HomeRoutingReissued HomeRoutingAction = "reissued"
)

// Dir is the profile directory the token store lives in. Pairing protects
// both gated surfaces of a home serve — the origin passthrough
// (localOrigin, GDK-433) and the mirror REST a phone companion reads
// (GDK-797) — so both workspace kinds may mint; what stays closed for a
// connected workspace is the passthrough itself (origin_rest.go 404s it).
// A workspace that is itself paired away cannot mint: its home is another
// machine.
func Dir(cfg *config.Config) (string, error) {
	if rem, err := origin.PairedStatus(cfg); err != nil {
		return "", err
	} else if rem != nil {
		return "", fmt.Errorf("%s — mint and revoke run on the home machine", PairedLine(cfg, rem))
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

// PairedLine is the self-status sentence a paired workspace gets where it
// tried to act as a home.
func PairedLine(cfg *config.Config, rem *pairing.Remote) string {
	line := fmt.Sprintf("this workspace is paired with %q (%s)", rem.Label, rem.Endpoint)
	if rem.Label == "" {
		line = fmt.Sprintf("this workspace is paired with %s", rem.Endpoint)
	}
	if cfg != nil && strings.TrimSpace(cfg.TokenOwner) != "" {
		line += " as " + cfg.TokenOwner
	}
	return line
}

// MintDevice is the device-mint flow: validate, resolve the endpoint,
// mint the scoped token, encode the offer, and — for a local-origin home —
// keep the _home routing token valid. Callers pre-validate their own input
// surface (flags, form fields); this function re-validates because it is
// the structural owner, not a trusted callee.
//
// endpoint may be empty: a live serve for cfg's profile is then discovered
// via origin.LiveServeFor. ttl may be empty: DefaultTTL applies. On a failure after
// the token was minted (the routing-token step is the only one) the
// result still carries the offer: the plaintext exists exactly once, in
// the caller output, or a minted token would be stranded unrecoverable.
func MintDevice(dir string, cfg *config.Config, label, scope, ttl, endpoint string, now time.Time) (MintResult, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return MintResult{}, errors.New("pairing mint requires --label NAME")
	}
	if label == pairing.HomeLabel {
		return MintResult{}, fmt.Errorf("%s is the machine routing key, not a device label — mint devices under any other name", pairing.HomeLabel)
	}
	scope = strings.TrimSpace(scope)
	switch scope {
	case pairing.ScopeOrigin, pairing.ScopeServe, pairing.ScopeTerminal:
	default:
		return MintResult{}, fmt.Errorf("unknown scope %q: want origin (a paired gadak riding the passthrough), serve (a paired client reading the mirror REST), or terminal (a shell on this machine)", scope)
	}
	ttlDur := DefaultTTL
	if strings.TrimSpace(ttl) != "" {
		var err error
		ttlDur, err = ParseTTL(ttl)
		if err != nil {
			return MintResult{}, err
		}
	}
	// Resolve the endpoint before minting: a mint without a reachable
	// endpoint is an orphan token nobody can use, and it would already
	// have flipped the gate on.
	ep := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	discovered := ep == ""
	if discovered {
		ep = AdvertisedEndpoint(cfg)
		if ep == "" {
			return MintResult{}, errors.New("no live serve found for this profile; start `gadak serve` first or pass --endpoint <url>")
		}
	}
	u, err := url.Parse(ep)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return MintResult{}, fmt.Errorf("bad endpoint %q: want an http(s) URL", ep)
	}
	loopback := isLoopbackHost(u.Hostname())
	// GDK-1266: the serve binds loopback by default, so the discovered
	// endpoint is one only this machine can dial — an offer carrying it
	// can never work on the device it is labelled for. Refuse before the
	// token exists. An explicit loopback --endpoint is the caller saying
	// "this machine" and is allowed (flagged, below).
	if discovered && loopback {
		return MintResult{}, &LoopbackEndpointError{Endpoint: ep}
	}
	token, meta, err := pairing.MintScoped(dir, label, scope, ttlDur, now)
	if err != nil {
		return MintResult{}, err
	}
	offer, err := pairing.EncodeOffer(pairing.Offer{
		V:         pairing.OfferV1,
		Endpoint:  ep,
		Token:     token,
		ExpiresAt: pairing.FormatExpiry(meta.ExpiresAt),
		Label:     label,
	})
	if err != nil {
		return MintResult{}, err
	}
	res := MintResult{
		Offer:           offer,
		Label:           label,
		Scope:           scope,
		Endpoint:        ep,
		ExpiresAt:       pairing.FormatExpiry(meta.ExpiresAt),
		Meta:            meta,
		LoopbackWarning: loopback,
	}
	// The _home routing token is a local-origin home concern: its local
	// writes ride the passthrough this serve fronts. A connected
	// workspace writes go straight to its site, and the routing file
	// (remote-origin.json) would make origin.pairedRemote read this
	// workspace as paired with its own serve — never written here.
	if cfg.HasLocalOrigin() {
		action, err := ensureHomeRoutingToken(dir, cfg, ep, now)
		res.HomeRouting = action
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

// ensureHomeRoutingToken is the single owner of "_home is valid". Once one
// token exists the gate takes Bearer only — no loopback bypass — and the
// home machine writes route through the same passthrough (GDK-333 single
// persist owner). Local-origin excludes the file from the paired-remote
// branch (origin.pairedRemote), so here it only ever carries the routing
// token localRoutingToken reads.
//
// Called after every device mint. If remote-origin.json token is missing
// or no longer active in the store while the gate is on, it reissues
// `_home` and rewrites the file — the recovery path for a routing token
// revoked by an older build (GDK-450).
func ensureHomeRoutingToken(dir string, cfg *config.Config, mintEndpoint string, now time.Time) (HomeRoutingAction, error) {
	rem, err := pairing.LoadRemote(dir)
	if err != nil {
		return HomeRoutingNone, err
	}
	if rem != nil {
		v, aerr := pairing.Authorize(dir, rem.Token, now)
		if aerr != nil {
			return HomeRoutingNone, aerr
		}
		if v == pairing.VerdictAccept {
			return HomeRoutingNone, nil
		}
		if v == pairing.VerdictOff {
			// Gate off: no active tokens, local writes need no bearer.
			return HomeRoutingNone, nil
		}
	} else {
		v, aerr := pairing.Authorize(dir, "", now)
		if aerr != nil {
			return HomeRoutingNone, aerr
		}
		if v == pairing.VerdictOff {
			return HomeRoutingNone, nil
		}
	}
	first := rem == nil
	if _, err := issueHomeRoutingToken(dir, cfg, mintEndpoint, now); err != nil {
		return HomeRoutingNone, err
	}
	if first {
		return HomeRoutingMinted, nil
	}
	return HomeRoutingReissued, nil
}

// MintHome is `pairing mint --label _home`: routing-token rotation, not a
// device offer. No offer is produced. Local-origin only — a connected
// workspace writes go straight to its site, and the routing file here
// would make origin.pairedRemote read this workspace as paired with its
// own serve.
func MintHome(dir string, cfg *config.Config, endpoint string, now time.Time) (pairing.Meta, error) {
	if !cfg.HasLocalOrigin() {
		return pairing.Meta{}, errors.New("_home is a local-origin workspace's routing token — this workspace's writes go to its site directly, and a routing file here would misread this workspace as paired")
	}
	ep, err := resolveHomeEndpoint(cfg, dir, endpoint)
	if err != nil {
		return pairing.Meta{}, err
	}
	return issueHomeRoutingToken(dir, cfg, ep, now)
}

func resolveHomeEndpoint(cfg *config.Config, dir, endpoint string) (string, error) {
	ep := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if ep == "" {
		ep = AdvertisedEndpoint(cfg)
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

func issueHomeRoutingToken(dir string, cfg *config.Config, mintEndpoint string, now time.Time) (pairing.Meta, error) {
	token, meta, err := pairing.Rotate(dir, pairing.HomeLabel, homeRoutingTTL, now)
	if err != nil {
		return pairing.Meta{}, err
	}
	ep := strings.TrimRight(strings.TrimSpace(mintEndpoint), "/")
	if ep == "" {
		ep = AdvertisedEndpoint(cfg)
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
		// Rotate revoked every previous _home token in the same store write
		// (its crash-atomicity — never reorder these), so a failed save
		// leaves the gate up with no valid routing credential on disk. The
		// recovery is a fresh mint, and the error must say so (GDK-1236).
		return pairing.Meta{}, fmt.Errorf("home routing token rotated (previous ones already revoked) but saving it failed — re-run to mint a fresh one: %w", err)
	}
	return meta, nil
}

// LoopbackEndpointError is the GDK-1266 refusal: no --endpoint was given
// and the live serve's own address is loopback, so no remote device could
// use the offer. Callers add their own prescription (the CLI: a completed
// command); the endpoint is here so they can name it.
type LoopbackEndpointError struct{ Endpoint string }

func (e *LoopbackEndpointError) Error() string {
	return fmt.Sprintf("the live serve listens on loopback (%s) — a device that is not this machine cannot reach it; pass --endpoint https://<home>.<tailnet>.ts.net (the URL other devices reach this serve at)", e.Endpoint)
}

// isLoopbackHost names the hosts only this machine can dial. Used for the
// mint refusal (discovered endpoint) and the mint warning (explicit one),
// never for a trust decision (GDK-433 prior art: a tunnel can look like
// loopback).
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// AdvertisedEndpoint turns a live UI-serve listen address into the URL
// form endpoint validation wants. Discovery is origin.LiveServeFor, the
// single owner of the serveaddr walk. An explicit endpoint is not
// normalized: making the user name the scheme is the point.
func AdvertisedEndpoint(cfg *config.Config) string {
	want := ""
	if cfg != nil {
		want = cfg.ProfileName()
	}
	rec, ok := origin.LiveServeFor(want)
	if !ok {
		return ""
	}
	return EndpointFromAdvertise(rec.Addr)
}

// EndpointFromAdvertise upgrades the raw bind address the advertise file
// stores (host:port) to the URL form, leaving an address that already
// carries a scheme alone.
func EndpointFromAdvertise(addr string) string {
	if addr == "" {
		return ""
	}
	if !strings.Contains(addr, "://") {
		return "http://" + addr
	}
	return addr
}

// rowTime is the table/JSON timestamp for pairing list columns.
const rowTime = "2006-01-02T15:04Z"

// Row is one pairing-list row. JSON tags are the table columns — never
// the plaintext token, never the full hash (only the 8-char prefix the
// list already showed).
type Row struct {
	Hash     string `json:"hash"`
	Label    string `json:"label"`
	Scope    string `json:"scope"`
	Created  string `json:"created"`
	Expires  string `json:"expires"`
	LastUsed string `json:"last_used"`
	State    string `json:"state"`
}

// Rows shapes every stored token for listing: the _home row reads as
// local-routing (not a device scope), and state distinguishes revoked and
// expired from active.
func Rows(dir string, now time.Time) ([]Row, error) {
	toks, err := pairing.List(dir)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(toks))
	for _, m := range toks {
		rows = append(rows, RowFrom(m, now))
	}
	return rows, nil
}

// RowFrom shapes one stored token. The 8-char hash prefix is the same
// floor revoke accepts.
func RowFrom(m pairing.Meta, now time.Time) Row {
	last := "-"
	if m.LastUsedAt != nil {
		last = m.LastUsedAt.UTC().Format(rowTime)
	}
	state := "active"
	if m.RevokedAt != nil {
		state = "revoked " + m.RevokedAt.UTC().Format(rowTime)
	} else if now.After(m.ExpiresAt) {
		state = "expired"
	}
	scope := m.Scope
	if m.Label == pairing.HomeLabel {
		scope = pairing.ScopeLocalRouting
	}
	return Row{
		Hash:     m.Hash[:8],
		Label:    m.Label,
		Scope:    scope,
		Created:  m.CreatedAt.UTC().Format(rowTime),
		Expires:  m.ExpiresAt.UTC().Format(rowTime),
		LastUsed: last,
		State:    state,
	}
}

// GateOpen reports whether the serve gate has fallen back open: no active
// token exists (including _home). The GDK-481 sentence callers print is
// theirs; this is the one boolean behind it.
func GateOpen(dir string, now time.Time) bool {
	v, err := pairing.Authorize(dir, "", now)
	return err == nil && v == pairing.VerdictOff
}

// Revoke is the store call the CLI and desktop share: selector is an exact
// label or a hash prefix of at least 8 hex characters (what Rows prints).
// The _home refusal and ambiguity wording live in internal/pairing —
// callers classify by that error, they do not rewrite it.
func Revoke(dir, selector string, now time.Time) (pairing.Meta, error) {
	return pairing.Revoke(dir, selector, now)
}

// QRModules encodes the offer at EC Medium and returns the module matrix,
// quiet zone included (the library default 4-module border). Medium rather
// than Lower: pairing happens once per device, and a code that scans from
// a phone held at an angle beats a dense one; higher than Medium buys
// almost nothing at this payload size.
func QRModules(offer string) ([][]bool, error) {
	q, err := qrcode.New(offer, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	return q.Bitmap(), nil
}

// qrPNGScale is px per module in QRPNG — the same 8 px/module the standing
// scanner-repro tool in cmd/gadak/qr_test.go writes.
const qrPNGScale = 8

// QRPNG renders the offer QR as a PNG: white background, black modules,
// quiet zone included via the module matrix, square. The desktop serves
// it to the webview as a data URI; the geometry matches what the terminal
// half-block renderer draws, so both encodings are the same code.
func QRPNG(offer string) ([]byte, error) {
	mods, err := QRModules(offer)
	if err != nil {
		return nil, err
	}
	n := len(mods) * qrPNGScale
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	white, black := color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{A: 255}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if mods[y/qrPNGScale][x/qrPNGScale] {
				img.SetRGBA(x, y, black)
			} else {
				img.SetRGBA(x, y, white)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
