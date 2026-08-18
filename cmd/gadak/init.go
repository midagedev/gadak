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
// what is lost: locally originated issues have no Jira copy, and a later
// sync treats them as upstream deletions.
const replaceStandaloneUsage = "replace this standalone workspace with a Jira site; locally originated issues exist only here and a later sync will delete them from the mirror"

// standaloneReplaceErrCode is the --json "error" value when a connected
// init refuses to take over a standalone workspace that holds data.
const standaloneReplaceErrCode = "standalone_data_present"

// refuseStandaloneReplace stops a connected init from silently changing
// which origin owns a standalone workspace that holds locally originated
// issues. An empty standalone workspace (tried it, nothing filed) is not
// a hazard and is allowed through.
func refuseStandaloneReplace(cfg *config.Config, replace, jsonOut bool) error {
	if cfg == nil || !cfg.IsStandalone() || replace {
		return nil
	}
	n, persist, err := standaloneLocalData(cfg)
	if err != nil {
		return fmt.Errorf("cannot replace standalone workspace: %w", err)
	}
	if n == 0 {
		return nil
	}
	msg := standaloneReplaceMessage(n, persist)
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		if encErr := enc.Encode(struct {
			Error   string `json:"error"`
			Issues  int    `json:"issues"`
			Persist string `json:"persist"`
			Hint    string `json:"hint"`
		}{
			Error:   standaloneReplaceErrCode,
			Issues:  n,
			Persist: persist,
			Hint:    "gadak --profile <name> init",
		}); encErr != nil {
			return fmt.Errorf("%s\n(also failed to encode JSON: %v)", msg, encErr)
		}
	}
	return errors.New(msg)
}

func standaloneReplaceMessage(n int, persist string) string {
	noun, exist := "issues", "they exist only here"
	if n == 1 {
		noun, exist = "issue", "it exists only here"
	}
	return fmt.Sprintf("this workspace is standalone and holds %d locally originated %s; %s — no Jira site has a copy\norigin persist file: %s\nconnect the site in a separate workspace: gadak --profile <name> init\n(list workspaces with gadak profiles)\nto replace this workspace anyway (a later sync will delete these issues from the mirror): --replace-standalone",
		n, noun, exist, persist)
}

// standaloneLocalData reports how many locally originated issues this
// standalone workspace holds, and the origin persist path (via
// origin.PersistPath — never rebuilt from string pieces).
//
// "Holds data" is max(mirror issues, origin issues):
//   - mirror: SELECT COUNT(*) FROM issues, only if gadak.db already exists
//     (store.Open would create it)
//   - origin: Search on the in-process origin, only if the persist file
//     already exists (origin.Client would create it)
//
// init --standalone creates a persist file with a project fixture and no
// issues, so that empty-origin case is n==0 and the common "I tried it,
// now I want to connect" path is not blocked.
func standaloneLocalData(cfg *config.Config) (n int, persist string, err error) {
	dir := ""
	if cfg != nil {
		dir = cfg.Directory()
	}
	if dir == "" {
		dir, err = config.Dir()
		if err != nil {
			return 0, "", err
		}
	}
	persist = origin.PersistPath(dir)

	mirrorN, err := standaloneMirrorIssueCount()
	if err != nil {
		return 0, persist, err
	}
	originN, err := standaloneOriginIssueCount(cfg, persist)
	if err != nil {
		return 0, persist, err
	}
	n = mirrorN
	if originN > n {
		n = originN
	}
	return n, persist, nil
}

func standaloneMirrorIssueCount() (int, error) {
	path, err := config.DBPath()
	if err != nil {
		return 0, err
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if st.IsDir() {
		return 0, nil
	}
	db, err := store.Open(path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	return db.TableCount(context.Background(), "issues")
}

func standaloneOriginIssueCount(cfg *config.Config, persist string) (int, error) {
	if persist == "" {
		return 0, nil
	}
	if _, err := os.Stat(persist); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	c, err := origin.Client(cfg)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n := 0
	err = c.Search(ctx, "ORDER BY created ASC", []string{"summary"}, false, func(issues []jira.Issue) error {
		n += len(issues)
		return nil
	})
	return n, err
}

// initMissingError lists every value still empty and how to supply it without a
// prompt. reason is why prompting was skipped (not a TTY, --json, --token-stdin).
// projects is optional (empty = every project the account can see).
func initMissingError(missing []string, reason string) error {
	return fmt.Errorf("missing: %s (%s)\nsupply them with flags (--site, --email) or environment\n(GADAK_SITE, GADAK_EMAIL, GADAK_TOKEN); for the token also\n--token-file <path> or --token-stdin; optional --projects / GADAK_PROJECTS",
		strings.Join(missing, ", "), reason)
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

	// Close the class "a command silently changes which origin owns this
	// workspace". An empty standalone workspace is not a hazard; one that
	// holds locally originated issues is (GDK-238).
	if err := refuseStandaloneReplace(cfg, *replaceStandalone, *jsonOut); err != nil {
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

	cfg.Site = site
	cfg.Email = email
	cfg.Token = token
	cfg.Projects = projects
	// Reached only after refuseStandaloneReplace (or --replace-standalone).
	cfg.Kind = ""

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
		return fmt.Errorf("site, email, and token are all required")
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
		enc := json.NewEncoder(os.Stdout)
		// One line, no HTML escaping — machine consumers parse this.
		enc.SetEscapeHTML(false)
		return enc.Encode(struct {
			Profile    string   `json:"profile"`
			Account    string   `json:"account"`
			Site       string   `json:"site"`
			Projects   []string `json:"projects"`
			Path       string   `json:"path"`
			Confluence any      `json:"confluence"`
			Kind       string   `json:"kind"`
		}{
			Profile:    config.Profile(),
			Account:    name,
			Site:       cfg.Site,
			Projects:   cfg.Projects,
			Path:       p,
			Confluence: initConfluenceJSON(cfg),
			Kind:       cfg.WorkspaceKind(),
		})
	}
	if name != "" {
		fmt.Printf("verified as %s — saved %s\n", name, p)
	} else {
		fmt.Printf("saved %s\n", p)
	}
	if len(cfg.Projects) == 0 {
		fmt.Println("no project filter — syncing everything this account can see; narrow it later in Settings → Sources")
	}
	printInitNextSteps()
	return nil
}

// initStandalone writes a workspace that is not bound to a Jira site.
// No /myself call: there is no credential. The origin snapshot is created
// on first origin.Client (PersistPath under the profile directory).
func initStandalone(cfg *config.Config, jsonOut bool, projectsFlag string) error {
	cfg.Kind = config.KindStandalone
	cfg.Site = ""
	cfg.Email = ""
	cfg.Token = ""
	cfg.TokenVerifiedAt = ""
	cfg.TokenOwner = ""
	cfg.TokenExpiresAt = ""
	cfg.TokenExpirySource = ""
	cfg.AccountID = ""
	cfg.Confluence = nil
	if projectsFlag != "" {
		cfg.Projects = parseProjectKeys(projectsFlag)
	}
	if strings.TrimSpace(cfg.DefaultProject) == "" {
		cfg.DefaultProject = origin.DefaultProjectKey
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
	if typeID, typeName := standaloneDefaultType(cfg); typeID != "" {
		cfg.DefaultIssueTypeID = typeID
		cfg.DefaultIssueType = typeName
		if err := cfg.Save(); err != nil {
			return err
		}
	}
	p, _ := config.Path()
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(struct {
			Profile    string   `json:"profile"`
			Account    string   `json:"account"`
			Site       string   `json:"site"`
			Projects   []string `json:"projects"`
			Path       string   `json:"path"`
			Confluence any      `json:"confluence"`
			Kind       string   `json:"kind"`
		}{
			Profile:    config.Profile(),
			Account:    "",
			Site:       "",
			Projects:   cfg.Projects,
			Path:       p,
			Confluence: initConfluenceJSON(cfg),
			Kind:       cfg.WorkspaceKind(),
		})
	}
	fmt.Printf("standalone workspace — saved %s\n", p)
	if cfg.DefaultProject != "" && cfg.DefaultIssueType != "" {
		// Say what create will fill in when given only a summary, so the
		// defaults are something the person saw rather than found out.
		fmt.Printf("new issues default to %s / %s — change them in %s\n",
			cfg.DefaultProject, cfg.DefaultIssueType, p)
	}
	printInitNextSteps()
	return nil
}

// standaloneDefaultType picks the issue type new issues get when create is
// given only a summary. "Task" is preferred by name; otherwise the first type
// the origin offers. Returning "" leaves the config untouched — create then
// asks for --type, which is the pre-existing behaviour, not a new failure.
//
// Unlike a headless per-create fallback (deliberately absent, see
// internal/create), this pick is written to config.json and printed, so the
// person can see what they got and change it.
func standaloneDefaultType(cfg *config.Config) (id, name string) {
	c, err := origin.Client(cfg)
	if err != nil {
		return "", ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	projects, err := c.CreateMeta(ctx, []string{cfg.DefaultProject})
	if err != nil || len(projects) == 0 {
		return "", ""
	}
	types := projects[0].IssueTypes
	if len(types) == 0 {
		return "", ""
	}
	for _, t := range types {
		if strings.EqualFold(t.Name, "Task") {
			return t.ID, t.Name
		}
	}
	return types[0].ID, types[0].Name
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

// printInitNextSteps ends `init` with the whole path to value, not just the
// next command. Filling the mirror is step one of three, and the other two —
// something to read it in, an agent wired to it — used to live only in the
// docs, so the product's own headline use case began with a search of the
// README.
func printInitNextSteps() {
	fmt.Print(`
next:
  gadak sync                    fill the mirror (a few minutes on a first run)
  gadak serve                   read it in the browser
  gadak mcp install claude      let your coding agent query it (also: cursor, codex)

docs/AGENT_SETUP.md has one paste per agent; docs/RECIPES.md has the questions
JQL cannot ask.
`)
}
