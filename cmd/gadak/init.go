package main

// gadak init — writes site/email/token/projects to config after verifying
// against /myself.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/store"
)

// tokenTrapHint is what the interactive prompt says before asking for a token.
// The web onboarding form says the same three things in
// web/src/lib/i18n/{en,ko}.ts — no string table spans TS and Go, so
// tools/doc-checks.sh asserts that both surfaces still name every trap.
const tokenTrapHint = `  Use "Create API token" with no scopes — a user token (ATATT…).
  A scoped token, or an org key from admin.atlassian.com (ATCTT…), cannot sign in to a site URL.`

// stdinIsTerminal reports whether stdin is a character device. Used so init can
// refuse to block on a prompt when an agent or pipe is driving the CLI.
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// Injection points so tests can exercise the interactive branch, which cannot
// be reached through a pipe.
var initStdin io.Reader = os.Stdin
var initIsTerminal = stdinIsTerminal

// parseProjectKeys splits a comma-separated project list the same way the
// interactive init path always has: trim, upper-case, drop empties. The
// originbind copy is the owner — POST onboarding/standalone parses its body
// with the same function, not a lookalike.
func parseProjectKeys(s string) []string {
	return originbind.ParseProjectKeys(s)
}

// parseSpaceKeys splits a Confluence space-key list. Keys are case-sensitive
// (personal spaces are ~accountId); unlike project keys they are not upper-cased.
func parseSpaceKeys(s string) []string {
	return parseCSVKeys(s, false)
}

func parseCSVKeys(s string, upper bool) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if upper {
			p = strings.ToUpper(p)
		}
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// replaceBuiltInUsage is the --replace-local help text. It names
// what is lost: locally originated issues have no Jira copy, and the
// conversion drops them from the mirror (GDK-241).
const replaceBuiltInUsage = "replace this workspace's built-in tracker with a Jira site; issues that originated here exist only here and converting deletes them from the mirror — `migrate --to jira` carries them out first"

// renderReplaceRefusedJSON writes the --json document for a refused
// built-in replace. Shape and field values match the previous
// refuseBuiltInReplace encoder (CLI --json contract).
func renderReplaceRefusedJSON(err error) error {
	var refused *originbind.ReplaceRefusedError
	if !errors.As(err, &refused) {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if encErr := enc.Encode(struct {
		Error   string `json:"error"`
		Issues  int    `json:"issues"`
		Persist string `json:"persist"`
		Hint    string `json:"hint"`
	}{
		Error:   originbind.ErrCodeReplaceRefused,
		Issues:  refused.Issues,
		Persist: refused.Persist,
		Hint:    "gadak --workspace <name> init",
	}); encErr != nil {
		return fmt.Errorf("%s\n(also failed to encode JSON: %v)", refused.Error(), encErr)
	}
	return err
}

// initMissingError lists every value still empty and how to supply it without a
// prompt. reason is why prompting was skipped (not a TTY, --json, --token-stdin).
// projects is optional (empty = every project the account can see).
func initMissingError(missing []string, reason string) error {
	return fmt.Errorf("missing: %s (%s)\nsupply them with flags (--site, --email) or environment\n(GADAK_SITE, GADAK_EMAIL, GADAK_TOKEN); for the token also\n--token-file <path> or --token-stdin; optional --projects / GADAK_PROJECTS\n%s",
		strings.Join(missing, ", "), reason, config.ErrNotConfigured.Error())
}

// renameLegacyInitFlags rewrites the pre-GDK-1281 flag names to the ones
// the FlagSet knows. A flag already sitting in someone's script is a
// contract, so the old spellings never stop working — but they are
// translated here rather than registered, because a registered alias
// would appear in `gadak init --help` and teach two names for one thing.
func renameLegacyInitFlags(args []string) []string {
	// GDK-1312: the second entry used to map the NEW name to itself, so the
	// v0.19 spelling --replace-standalone died while the alias test only
	// exercised --standalone.
	renamed := map[string]string{"standalone": "local", "replace-standalone": "replace-local"}
	out := make([]string, 0, len(args))
	for _, a := range args {
		dashes := ""
		switch {
		case strings.HasPrefix(a, "--"):
			dashes = "--"
		case strings.HasPrefix(a, "-") && len(a) > 1:
			dashes = "-"
		}
		if dashes == "" {
			out = append(out, a)
			continue
		}
		name, tail := strings.TrimPrefix(a, dashes), ""
		if i := strings.IndexByte(name, '='); i >= 0 {
			name, tail = name[:i], name[i:]
		}
		if to, ok := renamed[name]; ok {
			a = dashes + to + tail
		}
		out = append(out, a)
	}
	return out
}

// cmdInit writes site/email/token/projects to config after verifying against
// /myself. Classic interactive mode (TTY, no supply flags/env, no --json)
// re-prompts credentials (and optional projects) so a human can replace an
// expired token. Any non-interactive supply turns prompting off entirely.
// Projects are optional: blank means sync every project the account can see.
func cmdInit(args []string) error {
	fs := newFlagSet("init")
	siteFlag := fs.String("site", "", "Jira site URL (https://your-site.atlassian.net)")
	emailFlag := fs.String("email", "", "account email")
	projectsFlag := fs.String("projects", "", "project keys, comma-separated (optional — blank syncs every project you can see)")
	// Confluence: reserved words "all" / "none" (case-insensitive); any other
	// value is a comma-separated space-key list. Flag absent leaves Confluence alone.
	spacesFlag := fs.String("spaces", "", spacesFlagUsage)
	tokenFile := fs.String("token-file", "", "read API token from this file")
	tokenStdin := fs.Bool("token-stdin", false, "read API token from stdin")
	// Date from Atlassian's create dialog. Omitted → assume 365 days from
	// a successful /myself (config.ApplyTokenExpiry). No Atlassian API for this.
	tokenExpires := fs.String("token-expires", "", "token expiry date from Atlassian's create dialog (YYYY-MM-DD or RFC3339); omit to assume 365 days from verification")
	// Defined only so a mistaken `--token secret` gets a clear error instead of
	// "flag provided but not defined"; the value must never be accepted (ps/history).
	tokenFlag := fs.String("token", "", "not accepted; use GADAK_TOKEN, --token-file, or --token-stdin")
	// Jira Server / Data Center (GDK-1635/1640): a base URL that may carry a
	// context path, and a Personal Access Token instead of email + API token.
	// Explicit rather than sniffed — a workspace that silently decided which
	// Jira it was talking to is the "quietly points at another tracker" class
	// of defect. init still verifies the choice against serverInfo and refuses
	// a mismatch.
	serverFlag := fs.Bool("server", false, "the site is Jira Server / Data Center: authenticate with a Personal Access Token, no email")
	jsonOut := fs.Bool("json", false, "emit one JSON object on success")
	// The origin is the built-in tracker, running in this process — the
	// transport axis's local (GDK-1278).
	builtIn := fs.Bool("local", false, "create a workspace on the built-in tracker, running here (no Jira site or credential)")
	// Pairing (GDK-433): bind this workspace to a remote gadak serve with
	// an offer from the home machine's `gadak pairing mint`. The stdin form
	// exists for the same reason --token-stdin does: the offer carries a
	// secret and argv is ps/shell history.
	pairingCode := fs.String("pairing-code", "", "pairing offer from the home machine's `gadak pairing mint`; binds this workspace to that serve")
	pairingStdin := fs.Bool("pairing-code-stdin", false, "read the pairing offer from stdin (keeps it out of ps and shell history)")
	// Long name on purpose: a typo or a stray -f must not flip the origin.
	replaceLocalFlag := fs.Bool("replace-local", false, replaceBuiltInUsage)
	if err := fs.Parse(renameLegacyInitFlags(args)); err != nil {
		return err
	}
	wantBuiltIn := *builtIn
	replaceLocal := *replaceLocalFlag
	if *tokenFlag != "" {
		return fmt.Errorf("--token is not accepted: it would be visible in `ps` and shell history.\nuse GADAK_TOKEN=..., --token-file <path>, or --token-stdin")
	}
	if *tokenStdin && *tokenFile != "" {
		return fmt.Errorf("use only one of --token-file or --token-stdin")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// The pairing path is its own workspace creation: verify-before-save
	// over the remote serve, then a remote-origin credential. Nothing else
	// in init applies, so refuse the combinations instead of ignoring them.
	if *pairingCode != "" || *pairingStdin {
		if wantBuiltIn || replaceLocal {
			return fmt.Errorf("--pairing-code cannot be combined with --local or --replace-local")
		}
		if *siteFlag != "" || *emailFlag != "" || *tokenFile != "" || *tokenStdin || *tokenExpires != "" || *spacesFlag != "" {
			return fmt.Errorf("--pairing-code cannot be combined with site, email, token, or spaces flags")
		}
	}

	// Single owner: a workspace bound to a remote serve cannot be rebound
	// by any init path (bare, --standalone, site flags, or another offer).
	if err := refuseIfPairedOrigin(cfg); err != nil {
		return err
	}

	if *pairingCode != "" || *pairingStdin {
		return initPaired(cfg, *pairingCode, *pairingStdin, *jsonOut)
	}

	envSite := config.Env("SITE")
	envEmail := config.Env("EMAIL")
	envToken := config.Env("TOKEN")
	envProjects := config.Env("PROJECTS")

	if wantBuiltIn {
		if replaceLocal {
			return fmt.Errorf("--local cannot be combined with --replace-local")
		}
		if *siteFlag != "" || *emailFlag != "" || *tokenFile != "" || *tokenStdin || *tokenExpires != "" ||
			envSite != "" || envEmail != "" || envToken != "" {
			return fmt.Errorf("--local cannot be combined with a site, email, or token")
		}
		if *spacesFlag != "" {
			return fmt.Errorf("--local cannot be combined with --spaces")
		}
		return initBuiltIn(cfg, *jsonOut, *projectsFlag)
	}

	// Whether this run is converting a built-in workspace, captured before
	// cfg.Kind is cleared below. The seeded wiki space must be dropped on
	// every such conversion, not only the --replace-local one: an empty
	// built-in workspace is allowed through without that flag.
	wasBuiltIn := cfg.HasBuiltInOrigin()

	// CLI conversion while a live `gadak serve` is listening would race
	// the origin the UI still holds (GDK-415). HTTP onboarding is the
	// owner process and does not take this gate.
	if wasBuiltIn {
		if err := originbind.RefuseIfOpen(cfg); err != nil {
			return err
		}
	}

	// Close the class "a command silently changes which origin owns this
	// workspace". An empty built-in workspace is not a hazard; one that
	// holds locally originated issues is (GDK-238).
	if err := originbind.RefuseReplace(cfg, replaceLocal); err != nil {
		if *jsonOut {
			return renderReplaceRefusedJSON(err)
		}
		return err
	}

	// Any supply flag or env forces non-interactive; half-prompted states are unpredictable for agents.
	suppliedFlag := *siteFlag != "" || *emailFlag != "" || *projectsFlag != "" || *spacesFlag != "" || *tokenFile != "" || *tokenStdin || *tokenExpires != ""
	suppliedEnv := envSite != "" || envEmail != "" || envToken != "" || envProjects != ""
	classic := initIsTerminal() && !*jsonOut && !suppliedFlag && !suppliedEnv

	prevToken := cfg.Token
	creds, err := resolveCredentials(cfg, initSupply{
		classic:    classic,
		jsonOut:    *jsonOut,
		server:     *serverFlag,
		siteFlag:   *siteFlag,
		emailFlag:  *emailFlag,
		projFlag:   *projectsFlag,
		tokenFile:  *tokenFile,
		tokenStdin: *tokenStdin,
		envSite:    envSite,
		envEmail:   envEmail,
		envToken:   envToken,
		envProj:    envProjects,
		supplied:   suppliedFlag || suppliedEnv,
	})
	if err != nil {
		return err
	}
	site, email, token := creds.site, creds.email, creds.token
	projects := creds.projects

	if err := originbind.RefuseSiteRebind(cfg, site); err != nil {
		return err
	}

	cfg.Site = site
	cfg.Email = email
	cfg.Token = token
	cfg.Projects = projects
	// Reached only after RefuseReplace (or --replace-local).
	originbind.ClearBuiltIn(cfg)
	// A workspace is bound to one origin. Conversion drops the seeded LOC
	// space and the old origin's mirror, plus every personal row that named
	// it — a kept row does not go stale, it rebinds to whatever the new site
	// has at the same key (internal/store/origin_scope.go). Shared with HTTP
	// onboarding so the two paths cannot diverge. --spaces still owns the
	// connected wiki scope below.
	if wasBuiltIn {
		db, err := openStore()
		if err != nil {
			return err
		}
		reset, err := originbind.DropBuiltInProjection(cfg, db)
		if err != nil {
			_ = db.Close()
			return err
		}
		if err := db.Close(); err != nil {
			return err
		}
		// Say what went. A silently emptied feed or picker is the kind of
		// thing a user attributes to the new site being broken.
		if line := reset.String(); line != "" {
			fmt.Fprintf(os.Stderr, "%s\n", line)
		}
	}

	// Confluence: flag absent leaves the section untouched.
	if *spacesFlag != "" {
		switch {
		case strings.EqualFold(*spacesFlag, "none"):
			cfg.Confluence = nil
		case strings.EqualFold(*spacesFlag, "all"):
			// Empty Spaces = every *global* space (internal/sync/confluence.go).
			cfg.Confluence = &config.ConfluenceConfig{Spaces: []string{}}
		default:
			cfg.Confluence = &config.ConfluenceConfig{Spaces: parseSpaceKeys(*spacesFlag)}
		}
	}

	if *serverFlag {
		cfg.Kind = config.OriginJiraServer
		cfg.Email = ""
	}
	if cfg.OriginType() == config.OriginJiraServer {
		if cfg.Site == "" || cfg.Token == "" {
			return fmt.Errorf("--server needs a base URL and a personal access token\n%s", config.ErrNotConfigured.Error())
		}
	} else if !cfg.HasCredential() {
		return fmt.Errorf("site, email, and token are all required\n%s", config.ErrNotConfigured.Error())
	}
	// Same verification as the server credential / onboarding endpoints (jira /myself).
	// Auth rejection is fatal. A transport / site error is a warning: save the
	// credential without identity fields so offline init still works (I6).
	name := ""
	client := origin.Connected(cfg.Site, cfg.Email, cfg.Token)
	if cfg.OriginType() == config.OriginJiraServer {
		client = origin.ConnectedServer(cfg.Site, cfg.Token)
	}
	// Verify the deployment the user declared against what the origin says.
	// A mismatch is fatal: every REST path in the process branches on this
	// value, so saving a wrong one produces failures that read as a bad token.
	// An unreachable origin is not a mismatch — that is the offline case the
	// /myself check below already tolerates.
	if got, derr := probeDeployment(context.Background(), cfg); derr == nil && got != cfg.OriginType() {
		return fmt.Errorf("this site is %s, but the workspace was created as %s — %s",
			got, cfg.OriginType(), deploymentHint(got))
	}
	me, err := client.Myself(context.Background())
	if err != nil {
		if errors.Is(err, jira.ErrAuth) {
			// Restore the pre-jira.Myself hint: org API keys are a common mistake.
			return fmt.Errorf("credential check failed: %w (org API keys do not work; use a user token)", err)
		}
		fmt.Fprintf(os.Stderr, "warning: credential check failed (%v); saved without account_id — re-run init when the site is reachable\n", err)
	} else {
		cfg.ApplyVerifiedIdentity(me.AccountID, me.DisplayName, store.Now())
		name = me.DisplayName
	}
	userExpires := *tokenExpires
	if classic {
		userExpires = creds.expiresFromPrompt
	}
	if err := cfg.ApplyTokenExpiryIfNeeded(userExpires, cfg.TokenVerifiedAt, token != prevToken); err != nil {
		return fmt.Errorf("token expiry: %w", err)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	p, _ := config.Path()
	skill := autoInstallSkill(os.Stderr)
	if *jsonOut {
		return writeInitJSON(cfg, name, p, skill)
	}
	if name != "" {
		fmt.Printf("verified as %s — saved %s\n", name, p)
	} else {
		fmt.Printf("saved %s\n", p)
	}
	if len(cfg.Projects) == 0 {
		fmt.Println("no project filter — syncing everything this account can see; narrow it later in Settings → Sources")
	}
	// The wiki is opt-in and scoped per space, and until now nothing on the way
	// in said either. Inbound report (2026-08-25): a reader of "gadak mirrors
	// Jira *and* Confluence" did not install at all, because a whole Confluence
	// was more than they were willing to pull. Same shape as the projects line
	// above — say what the scope is at the moment it is set.
	if cfg.Confluence == nil {
		fmt.Println("wiki off — no Confluence page is mirrored; add named spaces with `gadak init --spaces ENG,PROD` (or Settings → Sources)")
	}
	printSkillAutoResult(skill)
	printInitNextSteps(cfg.WorkspaceKind())
	return nil
}

// initSupply is every credential source cmdInit resolved before prompting:
// the parsed flags, the environment, and whether the classic interactive
// prompt applies. resolveCredentials owns the precedence between them.
type initSupply struct {
	classic    bool
	jsonOut    bool
	server     bool
	siteFlag   string
	emailFlag  string
	projFlag   string
	tokenFile  string
	tokenStdin bool
	envSite    string
	envEmail   string
	envToken   string
	envProj    string
	// supplied reports that a flag or environment value opted a TTY run
	// into non-interactive fill — it names the reason in the missing error.
	supplied bool
}

// initCredentials is the resolved credential set both init modes produce:
// the effective site/email/token/projects, plus the expiry date the
// interactive prompt collected (empty when the token is unchanged).
type initCredentials struct {
	site, email, token string
	projects           []string
	expiresFromPrompt  string
}

// resolveCredentials is the single owner of init's two credential modes
// (GDK-1772). Classic prompts from saved values — an empty answer keeps
// each one, and the token is never echoed; every non-classic run resolves
// flag > env > saved and never prompts, failing with the no-prompt reason
// when something required is still empty. Interactive and non-interactive
// fill are the two shapes of one decision, so they live here instead of
// one of them being inlined in cmdInit.
func resolveCredentials(cfg *config.Config, in initSupply) (initCredentials, error) {
	var c initCredentials
	c.site = strings.TrimRight(cfg.Site, "/")
	c.email = cfg.Email
	c.token = cfg.Token
	c.projects = append([]string(nil), cfg.Projects...)
	prevToken := cfg.Token

	if in.classic {
		rd := bufio.NewReader(initStdin)
		prompt := func(label, current string) string {
			if current != "" {
				fmt.Printf("%s [%s]: ", label, current)
			} else {
				fmt.Printf("%s: ", label)
			}
			line, _ := rd.ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" {
				return current
			}
			return line
		}
		c.site = strings.TrimRight(prompt("Jira site URL (https://your-site.atlassian.net)", c.site), "/")
		c.email = prompt("Account email", c.email)
		// Two of the three things Atlassian's token page offers 401 against a
		// site URL, and it recommends one of those two first. Say so before the
		// paste, not after the rejection — after the 401 there is nothing left
		// to do but explain it (GDK-98). The web onboarding form carries the
		// same three facts; tools/doc-checks.sh pins the two together.
		fmt.Println(tokenTrapHint)
		// Token: keep-hint in the label only — never print the secret as [current].
		tokenLabel := "API token (id.atlassian.com/manage-profile/security/api-tokens)"
		if c.token != "" {
			tokenLabel += " [configured; enter to keep]"
		}
		if v := prompt(tokenLabel, ""); v != "" {
			c.token = v
		}
		// Ask when the token is new or this profile has no stored date yet
		// (upgrade from a pre-expiry config). Keep an existing date when the
		// token is unchanged.
		if c.token != prevToken || cfg.TokenExpiresAt == "" {
			c.expiresFromPrompt = prompt("Token expiry date (YYYY-MM-DD, from Atlassian's create dialog; blank assumes 1 year)", "")
		}
		c.projects = parseProjectKeys(prompt("Project keys, comma-separated (optional — blank syncs every project you can see)", strings.Join(c.projects, ",")))
		return c, nil
	}

	// flag > env > saved; never prompt.
	if in.envSite != "" {
		c.site = strings.TrimRight(in.envSite, "/")
	}
	if in.siteFlag != "" {
		c.site = strings.TrimRight(in.siteFlag, "/")
	}

	if in.envEmail != "" {
		c.email = in.envEmail
	}
	if in.emailFlag != "" {
		c.email = in.emailFlag
	}

	if in.envToken != "" {
		c.token = in.envToken
	}
	switch {
	case in.tokenStdin:
		b, err := io.ReadAll(initStdin)
		if err != nil {
			return c, fmt.Errorf("reading token from stdin: %w", err)
		}
		c.token = strings.TrimSpace(string(b))
	case in.tokenFile != "":
		b, err := os.ReadFile(in.tokenFile)
		if err != nil {
			return c, fmt.Errorf("reading --token-file: %w", err)
		}
		c.token = strings.TrimSpace(string(b))
	}

	if in.envProj != "" {
		c.projects = parseProjectKeys(in.envProj)
	}
	if in.projFlag != "" {
		c.projects = parseProjectKeys(in.projFlag)
	}

	var missing []string
	if c.site == "" {
		missing = append(missing, "site")
	}
	if c.email == "" && !in.server {
		// --server authenticates with a PAT alone; there is no email.
		missing = append(missing, "email")
	}
	if c.token == "" {
		missing = append(missing, "token")
	}
	// projects is optional: empty means every project the account can see.
	if len(missing) > 0 {
		reason := "stdin is not a terminal, so init cannot prompt"
		switch {
		case in.jsonOut:
			reason = "--json forbids interactive prompts"
		case in.tokenStdin:
			reason = "--token-stdin consumes stdin, so init cannot prompt"
		case in.supplied:
			// TTY but non-classic: flags/env opted into non-interactive fill.
			if initIsTerminal() {
				reason = "non-interactive supply was used, so init cannot prompt"
			}
		}
		return c, initMissingError(missing, reason)
	}
	return c, nil
}

// initBuiltIn is the CLI shell of a built-in init: the seeding core
// (config mutation, default type, mirror fill) lives in
// originbind.SeedBuiltIn, shared with POST onboarding/standalone. What
// stays here is CLI-only — the origin flush at process exit, the author line
// (GET /myself on the in-process origin, GDK-482), skill auto-install, and
// the human/JSON output.
func initBuiltIn(cfg *config.Config, jsonOut bool, projectsFlag string) error {
	already := cfg.HasBuiltInOrigin()
	// A fill that fails does not fail init (the contract moved with the core,
	// see originbind.SeedBuiltIn): the workspace exists, its persist file
	// is written, and writes already work — the next `gadak sync` fixes it.
	fillErr, err := originbind.SeedBuiltIn(cfg, projectsFlag, nil, func() (*store.DB, func() error, error) {
		db, err := openStore()
		if err != nil {
			return nil, nil, err
		}
		return db, db.Close, nil
	})
	if err != nil {
		return err
	}
	if fillErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fill the mirror yet (%v) — run `gadak sync`\n", fillErr)
	}
	author := builtInAuthorName(cfg)
	if err := origin.Close(); err != nil {
		return fmt.Errorf("flush origin persist: %w", err)
	}
	p, _ := config.Path()
	skill := autoInstallSkill(os.Stderr)
	if jsonOut {
		return writeInitJSON(cfg, "", p, skill)
	}
	_, persist := origin.Describe(cfg)
	if already {
		// GDK-465: re-init of an already-built-in home is one line, not
		// the first-run next list.
		if persist == "" {
			persist = p
		}
		fmt.Printf("origin is already the built-in tracker, at %s\n", persist)
		printSkillAutoResult(skill)
		return nil
	}
	fmt.Printf("origin is the built-in tracker — saved %s\n", p)
	if persist != "" {
		fmt.Printf("origin persist (this file is the original): %s\n", persist)
	}
	if cfg.DefaultProject != "" && cfg.DefaultIssueType != "" {
		// Say what create will fill in when given only a summary, so the
		// defaults are something the person saw rather than found out.
		fmt.Printf("new issues default to %s / %s — change them in %s\n",
			cfg.DefaultProject, cfg.DefaultIssueType, p)
	}
	if author != "" {
		// GDK-482: there is no CLI verb that changes this display name
		// (config list has no path; issuetap seeds the fixture user).
		// GDK-586: with an actor resolved, /myself above answered the
		// agent — this process writes as the agent, so the parenthetical
		// must not claim it is the workspace default.
		if _, ok := config.ResolveActor(cfg); ok {
			fmt.Printf("issues are authored as %s (this session's actor; writes without one use the workspace default)\n", author)
		} else {
			fmt.Printf("issues are authored as %s (the workspace default)\n", author)
		}
	}
	printSkillAutoResult(skill)
	printInitNextSteps(cfg.WorkspaceKind())
	return nil
}

// initConfluenceJSON is the --json shape for Confluence after init:
// "off" | "all" (the --spaces all token: every team space, personal excluded) | ["ENG","PROD"].
func initConfluenceJSON(cfg *config.Config) any {
	if cfg == nil || cfg.Confluence == nil {
		return "off"
	}
	if len(cfg.Confluence.Spaces) == 0 {
		return "all"
	}
	return cfg.Confluence.Spaces
}

// writeInitJSON is the --json document for both init kinds. Persist is
// built-in-only (origin.Describe's path); connected origin is not a file.
// skill is the auto-install result (installed|skipped|failed); initPaired
// has its own encoder and must name the same field.
func writeInitJSON(cfg *config.Config, account, path, skill string) error {
	kind, src := origin.Describe(cfg)
	persist := ""
	if kind == config.KindStandalone {
		persist = src
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(struct {
		Profile         string   `json:"profile"`
		Workspace       string   `json:"workspace"`
		WorkspaceSource string   `json:"workspace_source"`
		Account         string   `json:"account"`
		Site            string   `json:"site"`
		Projects        []string `json:"projects"`
		Path            string   `json:"path"`
		Confluence      any      `json:"confluence"`
		Kind            string   `json:"kind"`
		Persist         string   `json:"persist,omitempty"`
		Skill           string   `json:"skill"`
	}{
		Profile:         displayProfileName(config.Profile()),
		Workspace:       workspaceJSONName(),
		WorkspaceSource: workspaceJSONSource(),
		Account:         account,
		Site:            cfg.Site,
		Projects:        jsonList(cfg.Projects),
		Path:            path,
		Confluence:      initConfluenceJSON(cfg),
		Kind:            kind,
		Persist:         persist,
		Skill:           skill,
	})
}

// printInitNextSteps ends `init` with the whole path to value, not just the
// next command. Kind owns the duration hedge: a built-in first sync is
// local (the fill already ran); a connected first run can take minutes.
// printPairedInitNextSteps is the paired twin (pairing.go) — do not fold it in.
func printInitNextSteps(kind string) {
	if kind == config.KindStandalone {
		// GDK-465: the mirror is already filled; skill-first, MCP secondary.
		fmt.Printf(`
next:
  gadak create "first ticket"   file an issue in this workspace
  gadak serve                   read it in the browser
  gadak skill install           let a coding agent use it (Claude Desktop: gadak mcp install claude-desktop)
`)
		return
	}
	fmt.Printf(`
next:
  gadak serve                   open the browser — the first sync runs inside it, newest issues first
  gadak sync                    no browser? fill the mirror from the terminal instead (minutes on a first run)
  gadak mcp install claude      let your coding agent query it (also: claude-desktop, cursor, codex)

docs/AGENT_SETUP.md has one paste per agent; docs/RECIPES.md has the questions
JQL cannot ask.
`)
}

// builtInAuthorName is the display name GET /myself returns on the
// in-process origin. Empty if the origin cannot answer — the init success
// line is then omitted rather than inventing a name. GDK-482: no gadak
// verb changes this (measured against config list paths and issuetap seed).
func builtInAuthorName(cfg *config.Config) string {
	c, err := origin.Client(cfg)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	me, err := c.Myself(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(me.DisplayName)
}

// deploymentHint names the flag that would have matched what serverInfo
// reported, so the error says what to do rather than only what is wrong.
func deploymentHint(got string) string {
	if got == config.OriginJiraServer {
		return "re-run init with --server"
	}
	return "re-run init without --server"
}

// probeDeployment asks the origin which Jira it is, trying every credential
// shape the typed secret could be (GDK-1635).
//
// One shape is not enough. A mismatched deployment makes the configured
// shape fail authentication — a Cloud-shaped Basic email:token is rejected
// by Server — so asking only the configured way lets the mismatch hide
// behind the 401 it caused, which is the misdiagnosis this whole axis
// exists to prevent. The anonymous attempt is last and often refused:
// a Server instance need not grant anonymous browse.
//
// This is diagnosis, not routing. Nothing here decides which client the
// workspace uses; it only decides what the error message may claim.
func probeDeployment(ctx context.Context, cfg *config.Config) (string, error) {
	attempts := []*jira.Client{
		origin.ConnectedServer(cfg.Site, cfg.Token),
		origin.Connected(cfg.Site, cfg.Email, cfg.Token),
		jira.NewAnonymous(cfg.Site),
	}
	var lastErr error
	for _, c := range attempts {
		got, err := c.Deployment(ctx)
		if err == nil {
			return got, nil
		}
		lastErr = err
	}
	return "", lastErr
}
