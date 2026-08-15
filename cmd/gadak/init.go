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

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

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
	me, err := jira.New(cfg.Site, cfg.Email, cfg.Token).Myself(context.Background())
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
		}{
			Profile:    config.Profile(),
			Account:    name,
			Site:       cfg.Site,
			Projects:   cfg.Projects,
			Path:       p,
			Confluence: initConfluenceJSON(cfg),
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
