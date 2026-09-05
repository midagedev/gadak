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
//
// GDK-1047 moved the flow itself (validation, endpoint resolution, the
// mint, the offer encode, the _home routing-token guard, list row
// shaping, the QR encodings) into internal/pairflow so the desktop app's
// Devices tab runs the identical flow. This file keeps what is
// CLI-shaped only: flag parsing, the stdout/stderr contracts, the hints,
// and the terminal QR renderer (qr.go). parseTTL and
// endpointFromAdvertise stay as one-line wrappers because the package's
// own tests pin them by name.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairflow"
	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/store"
)

const pairingUsage = "usage: gadak pairing mint --label NAME [--scope origin|serve|terminal] [--ttl 90d] [--endpoint URL] [--no-qr] [--json] | pairing list [--json] | pairing revoke <label|hash-prefix>"

// pairingRevokeUsage is the revoke selector: exact label, or a hash prefix
// of at least minPrefix (8) hex characters — pairing list prints 8, longer
// prefixes still match (internal/pairing.Revoke).
const pairingRevokeUsage = "usage: gadak pairing revoke <label|hash-prefix>"

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

// parseTTL moved to internal/pairflow.ParseTTL (one owner); this wrapper
// keeps the package-main name the tests pin.
func parseTTL(s string) (time.Duration, error) { return pairflow.ParseTTL(s) }

// endpointFromAdvertise moved with the rest of endpoint resolution to
// pairflow.EndpointFromAdvertise; same deal — the tests pin this name.
func endpointFromAdvertise(addr string) string { return pairflow.EndpointFromAdvertise(addr) }

// pairingDir resolves where the token store lives and refuses the two
// states that cannot mint (paired away, no credential); the rules moved
// to pairflow.Dir, which both the CLI and the desktop now call.
func pairingDir() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return pairflow.Dir(cfg)
}

func pairedStatusLine(cfg *config.Config, rem *pairing.Remote) string {
	return pairflow.PairedLine(cfg, rem)
}

func pairingMint(args []string) error {
	fs := newFlagSet("pairing mint")
	// The 90-day default has one owner: pairflow.DefaultTTL.
	ttlDefault := pairflow.DefaultTTLFlag()
	label := fs.String("label", "", "device name shown in `gadak pairing list` (required)")
	scope := fs.String("scope", "origin", "what the token opens: origin = origin passthrough for a paired gadak (default), serve = the mirror REST for a paired client (a phone companion), terminal = a shell on this machine (never a default — see `gadak help pairing`)")
	ttlFlag := fs.String("ttl", ttlDefault, "token lifetime: <N><d|h|m|s>, e.g. 90d or 12h")
	endpoint := fs.String("endpoint", "", "URL remote devices reach this serve at (default: this machine's live serve address — refused when that is loopback, which it is unless serve runs --allow-remote)")
	asJSON := fs.Bool("json", false, "emit JSON")
	noQR := fs.Bool("no-qr", false, "skip the scannable QR mint draws below the offer line (drawn only when stderr is a terminal; --json, NO_COLOR, and TERM=dumb never draw one)")
	if _, err := parseAround(fs, args); err != nil {
		return err
	}
	// Flag-surface validation keeps the CLI's exact error strings; the
	// flow re-validates inside pairflow.MintDevice (structural owner), so
	// the desktop gets the same rules without this file.
	if strings.TrimSpace(*label) == "" {
		return errors.New("pairing mint requires --label NAME")
	}
	switch strings.TrimSpace(*scope) {
	case pairing.ScopeOrigin, pairing.ScopeServe, pairing.ScopeTerminal:
	default:
		return fmt.Errorf("unknown --scope %q: want origin (a paired gadak riding the passthrough), serve (a paired client reading the mirror REST), or terminal (a shell on this machine)", strings.TrimSpace(*scope))
	}
	if _, err := parseTTL(*ttlFlag); err != nil {
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
	res, err := pairflow.MintDevice(dir, cfg, *label, strings.TrimSpace(*scope), *ttlFlag, *endpoint, time.Now())
	var lb *pairflow.LoopbackEndpointError
	if errors.As(err, &lb) {
		// GDK-1266: usage class (EX_USAGE) — nothing was minted; the fix is
		// a flag, and the hint spells it out when tailscale can name it.
		return &exitCodeError{code: 64, msg: err.Error() + tailnetMintHint(strings.TrimSpace(*label), strings.TrimSpace(*scope))}
	}
	// The mint already produced the credential: the output contract prints
	// even when a later step (the routing-token guard) failed, or the
	// plaintext would exist nowhere and the token would be stranded.
	if res.Offer != "" {
		if res.LoopbackWarning {
			fmt.Fprintf(os.Stderr, "warning: endpoint %s is loopback — only a device on this machine can reach it; a remote device needs --endpoint with your tailnet URL (e.g. https://<machine>.<tailnet>.ts.net)\n", res.Endpoint)
		}
		if outErr := writePairingMintOutput(*asJSON, *noQR, res); outErr != nil {
			return outErr
		}
	}
	if res.HomeRouting == pairflow.HomeRoutingMinted {
		fmt.Fprintln(os.Stderr, "minted a _home routing token so this machine's own writes keep passing the gate")
	} else if res.HomeRouting == pairflow.HomeRoutingReissued {
		fmt.Fprintln(os.Stderr, "reissued _home routing token so this machine's own writes keep passing the gate")
	}
	return err
}

// tailnetMintHint completes the GDK-1266 refusal when the tailscale CLI is
// on PATH: `tailscale status --json` answers Self.DNSName without root, and
// through `tailscale serve` (docs/NETWORK.md, the intended transport) that
// name is the URL other devices reach this serve at. Suggested only — the
// user re-runs with it; adopting it silently would hand out an offer for a
// URL nobody checked. No tailscale, or any failure: no hint, the refusal
// stands alone. A local exec, not an outbound request.
func tailnetMintHint(label, scope string) string {
	out, err := tailnetStatusJSON()
	if err != nil {
		return ""
	}
	var st struct {
		Self struct{ DNSName string } `json:"Self"`
	}
	if json.Unmarshal(out, &st) != nil {
		return ""
	}
	dns := strings.TrimSuffix(strings.TrimSpace(st.Self.DNSName), ".")
	if dns == "" {
		return ""
	}
	cmd := "gadak pairing mint --label " + label
	if scope != pairing.ScopeOrigin {
		cmd += " --scope " + scope
	}
	return fmt.Sprintf("\ntry: %s --endpoint https://%s", cmd, dns)
}

// tailnetStatusJSON is `tailscale status --json` as a seam: the CLI's
// answer when tailscale is on PATH, an error when it is not or the daemon
// does not answer. A test swaps it for a stub instead of planting a shell
// script on PATH — that script's fork ran past the 5s bound on a machine
// saturated by two Playwright suites and made the hint assertion flaky
// (GDK-1306); the hint's wording is what the test owns, not the fork.
var tailnetStatusJSON = func() ([]byte, error) {
	if _, err := exec.LookPath("tailscale"); err != nil {
		return nil, err
	}
	// 5s: tailscale answers in milliseconds when its daemon is up; the
	// bound is for a wedged daemon, and macOS's first exec of a fresh
	// binary can itself take a second or two.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
}

// writePairingMintOutput is the device-mint output contract (GDK-456).
// Default: stdout is exactly the offer line (pipe target). --json: one
// JSON object on stdout with the same offer plus label/scope/endpoint/
// expires_at. Human stderr says what the token is for — which machine
// consumes it depends on the scope; the offer is reusable until expiry
// (not "shown once"). On a terminal the offer is also drawn as a
// scannable QR after the hints (GDK-1047) — decoration on top of the
// contract, never part of it: it cannot fail the mint, and `_home`
// rotation (pairingMintHome) never reaches this function.
func writePairingMintOutput(asJSON, noQR bool, res pairflow.MintResult) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(struct {
			Offer     string `json:"offer"`
			Label     string `json:"label"`
			Scope     string `json:"scope"`
			Endpoint  string `json:"endpoint"`
			ExpiresAt string `json:"expires_at"`
		}{res.Offer, res.Label, res.Scope, res.Endpoint, res.ExpiresAt})
	}
	fmt.Println(res.Offer)
	switch res.Scope {
	case pairing.ScopeServe:
		fmt.Fprintln(os.Stderr, pairingMintServeHint())
	case pairing.ScopeTerminal:
		fmt.Fprintln(os.Stderr, pairingMintTerminalHint())
	default:
		fmt.Fprintln(os.Stderr, pairingMintRemoteHint(res.Label))
	}
	fmt.Fprintf(os.Stderr, "paired device %q — offer reusable until %s; revoke with: gadak pairing revoke %s\n",
		res.Label, res.ExpiresAt, res.Meta.Hash[:8])
	if shouldDrawQR(pairingStderrIsTerminal(), noQR, asJSON,
		os.Getenv("NO_COLOR") != "", os.Getenv("TERM") == "dumb") {
		// The QR's own errors (a fixed "content too long" class string
		// from the library — never the payload) cannot fail a mint that
		// already printed its offer.
		if err := drawPairingQR(os.Stderr, res.Offer, pairingTerminalWidth()); err != nil {
			fmt.Fprintf(os.Stderr, "pairing: QR not drawn (%v) — the offer line on stdout is unaffected\n", err)
		}
	}
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

// pairingMintHome is the CLI half of `_home` rotation: the flow lives in
// pairflow.MintHome; this prints the one stderr line. stdout stays empty
// so the line cannot be piped into `init --pairing-code`.
func pairingMintHome(dir string, cfg *config.Config, endpoint string) error {
	meta, err := pairflow.MintHome(dir, cfg, endpoint, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "rotated _home routing token (%s) — local writes keep passing the gate\n", meta.Hash[:8])
	return nil
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
				"tokens": jsonList([]pairflow.Row{}),
			})
		}
		fmt.Println(pairedStatusLine(cfg, rem))
		return nil
	}
	dir, err := pairingDir()
	if err != nil {
		return err
	}
	rows, err := pairflow.Rows(dir, time.Now())
	if err != nil {
		return err
	}
	if *asJSON {
		// Empty --json is {"tokens":[]} (jsonList), never the human
		// sentence and never null: a JSON consumer that gets English on
		// stdout is a bug. pairingGateOpenLine stays on stderr.
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"tokens": jsonList(rows)})
	}
	if len(rows) == 0 {
		fmt.Println("no pairing tokens — the origin passthrough is open to loopback (implicit trust)")
	} else {
		fmt.Printf("%-8s  %-20s  %-14s  %-20s  %-20s  %-20s  %s\n",
			"HASH", "LABEL", "SCOPE", "CREATED", "EXPIRES", "LAST-USED", "STATE")
		for _, row := range rows {
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

// pairingGateOpenLine is the GDK-481 copy when the gate has fallen back
// open (no active token, including _home). Empty when the gate is still
// closed. _home is counted: Authorize includes ScopeLocalRouting, so
// revoking the last device token while _home remains must not claim the
// gate is open. The sentence is presentation and stays here; the boolean
// moved to pairflow.GateOpen.
func pairingGateOpenLine(dir string) string {
	if !pairflow.GateOpen(dir, time.Now()) {
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
	meta, err := pairflow.Revoke(dir, rest[0], time.Now())
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
	if cfg.HasLocalOrigin() || cfg.Site != "" || cfg.Email != "" || cfg.Token != "" {
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
	cfg.Confluence = origin.PairedConfluenceConfig()
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
// Local-origin is excluded: the home machine stores its routing token in the
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
