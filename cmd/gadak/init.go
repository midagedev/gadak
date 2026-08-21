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
	syncer "github.com/midagedev/gadak/internal/sync"
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
// interactive init path always has: trim, upper-case, drop empties.
func parseProjectKeys(s string) []string {
	return parseCSVKeys(s, true)
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

// replaceStandaloneUsage is the --replace-standalone help text. It names
// what is lost: locally originated issues have no Jira copy, and the
// conversion drops them from the mirror (GDK-241).
const replaceStandaloneUsage = "replace this standalone workspace with a Jira site; locally originated issues exist only here and converting deletes them from the mirror"

// renderReplaceRefusedJSON writes the --json document for a refused
// standalone replace. Shape and field values match the previous
// refuseStandaloneReplace encoder (CLI --json contract).
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
	jsonOut := fs.Bool("json", false, "emit one JSON object on success")
	// Quiet beta: independent workspace, no Jira site or credential.
	standalone := fs.Bool("standalone", false, "create an independent workspace (no Jira site or credential)")
	// Pairing (GDK-433): bind this workspace to a remote gadak serve with
	// an offer from the home machine's `gadak pairing mint`. The stdin form
	// exists for the same reason --token-stdin does: the offer carries a
	// secret and argv is ps/shell history.
	pairingCode := fs.String("pairing-code", "", "pairing offer from the home machine's `gadak pairing mint`; binds this workspace to that serve")
	pairingStdin := fs.Bool("pairing-code-stdin", false, "read the pairing offer from stdin (keeps it out of ps and shell history)")
	// Long name on purpose: a typo or a stray -f must not flip the origin.
	replaceStandalone := fs.Bool("replace-standalone", false, replaceStandaloneUsage)
	if err := fs.Parse(args); err != nil {
		return err
	}
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
		if *standalone || *replaceStandalone {
			return fmt.Errorf("--pairing-code cannot be combined with --standalone or --replace-standalone")
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

	if *standalone {
		if *replaceStandalone {
			return fmt.Errorf("--standalone cannot be combined with --replace-standalone")
		}
		if *siteFlag != "" || *emailFlag != "" || *tokenFile != "" || *tokenStdin || *tokenExpires != "" ||
			envSite != "" || envEmail != "" || envToken != "" {
			return fmt.Errorf("--standalone cannot be combined with a site, email, or token")
		}
		if *spacesFlag != "" {
			return fmt.Errorf("--standalone cannot be combined with --spaces")
		}
		return initStandalone(cfg, *jsonOut, *projectsFlag)
	}

	// Whether this run is converting a standalone workspace, captured before
	// cfg.Kind is cleared below. The seeded wiki space must be dropped on
	// every such conversion, not only the --replace-standalone one: an empty
	// standalone workspace is allowed through without that flag.
	wasStandalone := cfg.IsStandalone()

	// CLI conversion while serve/desktop has this workspace open would
	// race the persist owner (GDK-415). HTTP onboarding is the owner
	// process and does not take this gate.
	if wasStandalone {
		if err := originbind.RefuseIfOpen(cfg); err != nil {
			return err
		}
	}

	// Close the class "a command silently changes which origin owns this
	// workspace". An empty standalone workspace is not a hazard; one that
	// holds locally originated issues is (GDK-238).
	if err := originbind.RefuseReplace(cfg, *replaceStandalone); err != nil {
		if *jsonOut {
			return renderReplaceRefusedJSON(err)
		}
		return err
	}

	// Any supply flag or env forces non-interactive; half-prompted states are unpredictable for agents.
	suppliedFlag := *siteFlag != "" || *emailFlag != "" || *projectsFlag != "" || *spacesFlag != "" || *tokenFile != "" || *tokenStdin || *tokenExpires != ""
	suppliedEnv := envSite != "" || envEmail != "" || envToken != "" || envProjects != ""
	classic := initIsTerminal() && !*jsonOut && !suppliedFlag && !suppliedEnv

	var site, email, token string
	var projects []string
	prevToken := cfg.Token
	expiresFromPrompt := ""

	if classic {
		// Start from saved values; empty answers keep them (token never echoed).
		site = strings.TrimRight(cfg.Site, "/")
		email = cfg.Email
		token = cfg.Token
		projects = append([]string(nil), cfg.Projects...)

		in := bufio.NewReader(initStdin)
		prompt := func(label, current string) string {
			if current != "" {
				fmt.Printf("%s [%s]: ", label, current)
			} else {
				fmt.Printf("%s: ", label)
			}
			line, _ := in.ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" {
				return current
			}
			return line
		}
		site = strings.TrimRight(prompt("Jira site URL (https://your-site.atlassian.net)", site), "/")
		email = prompt("Account email", email)
		// Two of the three things Atlassian's token page offers 401 against a
		// site URL, and it recommends one of those two first. Say so before the
		// paste, not after the rejection — after the 401 there is nothing left
		// to do but explain it (GDK-98). The web onboarding form carries the
		// same three facts; tools/doc-checks.sh pins the two together.
		fmt.Println(tokenTrapHint)
		// Token: keep-hint in the label only — never print the secret as [current].
		tokenLabel := "API token (id.atlassian.com/manage-profile/security/api-tokens)"
		if token != "" {
			tokenLabel += " [configured; enter to keep]"
		}
		if v := prompt(tokenLabel, ""); v != "" {
			token = v
		}
		// Ask when the token is new or this profile has no stored date yet
		// (upgrade from a pre-expiry config). Keep an existing date when the
		// token is unchanged.
		if token != prevToken || cfg.TokenExpiresAt == "" {
			expiresFromPrompt = prompt("Token expiry date (YYYY-MM-DD, from Atlassian's create dialog; blank assumes 1 year)", "")
		}
		projects = parseProjectKeys(prompt("Project keys, comma-separated (optional — blank syncs every project you can see)", strings.Join(projects, ",")))
	} else {
		// flag > env > saved; never prompt.
		site = strings.TrimRight(cfg.Site, "/")
		if envSite != "" {
			site = strings.TrimRight(envSite, "/")
		}
		if *siteFlag != "" {
			site = strings.TrimRight(*siteFlag, "/")
		}

		email = cfg.Email
		if envEmail != "" {
			email = envEmail
		}
		if *emailFlag != "" {
			email = *emailFlag
		}

		token = cfg.Token
		if envToken != "" {
			token = envToken
		}
		switch {
		case *tokenStdin:
			b, err := io.ReadAll(initStdin)
			if err != nil {
				return fmt.Errorf("reading token from stdin: %w", err)
			}
			token = strings.TrimSpace(string(b))
		case *tokenFile != "":
			b, err := os.ReadFile(*tokenFile)
			if err != nil {
				return fmt.Errorf("reading --token-file: %w", err)
			}
			token = strings.TrimSpace(string(b))
		}

		projects = append([]string(nil), cfg.Projects...)
		if envProjects != "" {
			projects = parseProjectKeys(envProjects)
		}
		if *projectsFlag != "" {
			projects = parseProjectKeys(*projectsFlag)
		}

		var missing []string
		if site == "" {
			missing = append(missing, "site")
		}
		if email == "" {
			missing = append(missing, "email")
		}
		if token == "" {
			missing = append(missing, "token")
		}
		// projects is optional: empty means every project the account can see.
		if len(missing) > 0 {
			reason := "stdin is not a terminal, so init cannot prompt"
			switch {
			case *jsonOut:
				reason = "--json forbids interactive prompts"
			case *tokenStdin:
				reason = "--token-stdin consumes stdin, so init cannot prompt"
			case suppliedFlag || suppliedEnv:
				// TTY but non-classic: flags/env opted into non-interactive fill.
				if initIsTerminal() {
					reason = "non-interactive supply was used, so init cannot prompt"
				}
			}
			return initMissingError(missing, reason)
		}
	}

	if err := originbind.RefuseSiteRebind(cfg, site); err != nil {
		return err
	}

	cfg.Site = site
	cfg.Email = email
	cfg.Token = token
	cfg.Projects = projects
	// Reached only after RefuseReplace (or --replace-standalone).
	originbind.ClearStandalone(cfg)
	// A workspace is bound to one origin. Conversion drops the seeded LOC
	// space and the old origin's mirror, plus every personal row that named
	// it — a kept row does not go stale, it rebinds to whatever the new site
	// has at the same key (internal/store/origin_scope.go). Shared with HTTP
	// onboarding so the two paths cannot diverge. --spaces still owns the
	// connected wiki scope below.
	if wasStandalone {
		db, err := openStore()
		if err != nil {
			return err
		}
		reset, err := originbind.DropStandaloneProjection(cfg, db)
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

	if !cfg.HasCredential() {
		return fmt.Errorf("site, email, and token are all required\n%s", config.ErrNotConfigured.Error())
	}
	// Same verification as the server credential / onboarding endpoints (jira /myself).
	// Auth rejection is fatal. A transport / site error is a warning: save the
	// credential without identity fields so offline init still works (I6).
	name := ""
	me, err := origin.Connected(cfg.Site, cfg.Email, cfg.Token).Myself(context.Background())
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
		userExpires = expiresFromPrompt
	}
	if err := cfg.ApplyTokenExpiryIfNeeded(userExpires, cfg.TokenVerifiedAt, token != prevToken); err != nil {
		return fmt.Errorf("token expiry: %w", err)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	p, _ := config.Path()
	if *jsonOut {
		return writeInitJSON(cfg, name, p)
	}
	if name != "" {
		fmt.Printf("verified as %s — saved %s\n", name, p)
	} else {
		fmt.Printf("saved %s\n", p)
	}
	if len(cfg.Projects) == 0 {
		fmt.Println("no project filter — syncing everything this account can see; narrow it later in Settings → Sources")
	}
	printInitNextSteps(cfg.WorkspaceKind())
	return nil
}

// initStandalone writes a workspace that is not bound to a Jira site.
// There is no site credential. GET /myself is the in-process origin, used
// only to print the default author (GDK-482). The origin snapshot is created
// on first origin.Client (PersistPath under the profile directory).
// DefaultConfluenceConfig turns the wiki sync pass on and scopes it to the
// space the standalone origin seeds.
func initStandalone(cfg *config.Config, jsonOut bool, projectsFlag string) error {
	already := cfg.IsStandalone()
	cfg.Kind = config.KindStandalone
	cfg.Site = ""
	cfg.Email = ""
	cfg.Token = ""
	cfg.TokenVerifiedAt = ""
	cfg.TokenOwner = ""
	cfg.TokenExpiresAt = ""
	cfg.TokenExpirySource = ""
	cfg.AccountID = ""
	cfg.Confluence = origin.DefaultConfluenceConfig()
	if projectsFlag != "" {
		cfg.Projects = parseProjectKeys(projectsFlag)
	}
	if strings.TrimSpace(cfg.DefaultProject) == "" {
		if len(cfg.Projects) > 0 {
			cfg.DefaultProject = cfg.Projects[0]
		} else {
			cfg.DefaultProject = origin.DefaultProjectKey
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	// The seeded project offers five issue types, so "the project's only
	// type" cannot resolve and `gadak create <summary>` would demand --type
	// on every call — the one thing this workspace exists to make cheap. Ask
	// the origin we just created (in-process, no network) and record the
	// answer, so the pick lives in config.json where it can be read and
	// changed rather than being guessed per create.
	typeID, typeName, err := standaloneDefaultType(cfg)
	if err != nil {
		return err
	}
	if typeID != "" {
		cfg.DefaultIssueTypeID = typeID
		cfg.DefaultIssueType = typeName
		if err := cfg.Save(); err != nil {
			return err
		}
	}
	// Fill the mirror now so warnIfStale's empty synced_at is false.
	// Standalone origin is local; this is the same one-shot Run `gadak sync`
	// would do, without printing (stdout may be --json).
	//
	// A fill that fails does not fail init: the workspace exists, its persist
	// file is written, and writes already work. Returning an error here would
	// break `init --standalone --json && gadak create …` over something the
	// next `gadak sync` fixes — and the stale warning, which is what this call
	// exists to silence, comes back to say so.
	if err := fillStandaloneMirror(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fill the mirror yet (%v) — run `gadak sync`\n", err)
	}
	author := standaloneAuthorName(cfg)
	if err := origin.Close(); err != nil {
		return fmt.Errorf("flush origin persist: %w", err)
	}
	p, _ := config.Path()
	if jsonOut {
		return writeInitJSON(cfg, "", p)
	}
	_, persist := origin.Describe(cfg)
	if already {
		// GDK-465: re-init of an already-standalone home is one line, not
		// the first-run next list.
		if persist == "" {
			persist = p
		}
		fmt.Printf("already standalone at %s\n", persist)
		return nil
	}
	fmt.Printf("standalone workspace — saved %s\n", p)
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
		fmt.Printf("issues are authored as %s (the workspace default)\n", author)
	}
	printInitNextSteps(cfg.WorkspaceKind())
	return nil
}

// standaloneDefaultType picks the issue type new issues get when create is
// given only a summary. "Task" is preferred by name; otherwise the first type
// the origin offers. An origin.Client failure is an init failure — the
// origin we just declared is unusable (GDK-345). Returning "", "", nil
// leaves the config untouched: create then asks for --type, which is the
// pre-existing behaviour when the origin is up but offers no types.
//
// Unlike a headless per-create fallback (deliberately absent, see
// internal/create), this pick is written to config.json and printed, so the
// person can see what they got and change it.
func standaloneDefaultType(cfg *config.Config) (id, name string, err error) {
	c, err := origin.Client(cfg)
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	projects, err := c.CreateMeta(ctx, []string{cfg.DefaultProject})
	if err != nil || len(projects) == 0 {
		return "", "", nil
	}
	types := projects[0].IssueTypes
	if len(types) == 0 {
		return "", "", nil
	}
	for _, t := range types {
		if strings.EqualFold(t.Name, "Task") {
			return t.ID, t.Name, nil
		}
	}
	return types[0].ID, types[0].Name, nil
}

// initConfluenceJSON is the --json shape for Confluence after init:
// "off" | "all" (the --spaces all token: every global space) | ["ENG","PROD"].
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
// standalone-only (origin.Describe's path); connected origin is not a file.
func writeInitJSON(cfg *config.Config, account, path string) error {
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
	})
}

// fillStandaloneMirror runs the same one-shot Jira+Confluence sync `gadak
// sync` would, without printing. That stamps sync_state.synced_at so
// warnIfStale does not fire on the next command.
func fillStandaloneMirror(cfg *config.Config) error {
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()
	var opts syncer.Options
	if _, err := syncer.Run(ctx, cfg, db, opts); err != nil {
		return fmt.Errorf("fill mirror: %w", err)
	}
	if cfg.Confluence != nil {
		if _, err := syncer.RunConfluence(ctx, cfg, db, opts); err != nil {
			return fmt.Errorf("fill wiki mirror: %w", err)
		}
	}
	return nil
}

// printInitNextSteps ends `init` with the whole path to value, not just the
// next command. Kind owns the duration hedge: a standalone first sync is
// local (the fill already ran); a connected first run can take minutes.
// printPairedInitNextSteps is the paired twin (pairing.go) — do not fold it in.
func printInitNextSteps(kind string) {
	if kind == config.KindStandalone {
		// GDK-465: the mirror is already filled; skill-first, MCP secondary.
		fmt.Printf(`
next:
  gadak create "first ticket"   file an issue in this workspace
  gadak serve                   read it in the browser
  gadak skill install           let a coding agent use it (shell-less hosts: gadak mcp install claude)
`)
		return
	}
	fmt.Printf(`
next:
  gadak sync                    fill the mirror (a few minutes on a first run)
  gadak serve                   read it in the browser
  gadak mcp install claude      let your coding agent query it (also: cursor, codex)

docs/AGENT_SETUP.md has one paste per agent; docs/RECIPES.md has the questions
JQL cannot ask.
`)
}

// standaloneAuthorName is the display name GET /myself returns on the
// in-process origin. Empty if the origin cannot answer — the init success
// line is then omitted rather than inventing a name. GDK-482: no gadak
// verb changes this (measured against config list paths and issuetap seed).
func standaloneAuthorName(cfg *config.Config) string {
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
