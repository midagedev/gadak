package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// cmdHelp is the prose around a command. Flag names and descriptions for
// flag.FlagSet commands are taken from the FlagSet via VisitAll at render
// time so they cannot drift from the registration site. options is only
// filled for commands that parse flags by hand.
type cmdHelp struct {
	summary  string
	usage    string
	options  []helpOption // manual Options lines; ignored when a FlagSet is given
	examples []string
	seeAlso  []string
}

type helpOption struct {
	name string // without leading dashes, e.g. "full" or "json"
	desc string
}

// spacesFlagUsage is the --spaces description for both the FlagSet and
// `gadak help init`. It must match internal/sync/confluence.go: empty
// Spaces lists type=="global" only; personal spaces are included only
// when named. Do not say "every space" / "everything".
const spacesFlagUsage = `Confluence spaces: KEY,KEY… | all (every global space; name a personal space to include it) | none (off); "all"/"none" are reserved`

// initSummary is the one-line init description for top-level usage and
// `gadak init --help`. It has to name every workspace path: connected
// init writes site/email/token (cmdInit); --local writes
// KindLocalOrigin with no site or credential (initLocalOrigin, flag usage
// in cmdInit); --pairing-code binds a fresh workspace to a remote gadak
// serve (initPaired, GDK-433).
const initSummary = "configure a Jira site and credential (projects optional), the built-in tracker (--local), or pair to a remote gadak serve (--pairing-code)"

// serveSyncDefault is the serve sync-on-start condition, matching
// startServeLoops: cfg.HasCredential() is true for a local-origin workspace,
// a connected workspace with site+email+token, or a Linear apiKey
// (internal/config.HasCredential).
const serveSyncDefault = "syncs by default on the built-in tracker, or when a Jira or Linear workspace has a credential"

// writeThroughOriginPhrase is the single owner of "where CLI writes go"
// (GDK-469). Verified: mutate in agent.go calls origin.Writer; origin.Client
// refuses a connected workspace without site/email/token (errNeedCredential)
// and admits a local-origin workspace with no token. Verb --help first lines
// name the verb; this sentence lives once in top-level usage.
const writeThroughOriginPhrase = "Jira on a Jira workspace (needs a site credential), Linear when a linear apiKey is configured, the built-in tracker when that is the origin; the mirror refreshes after the origin accepts"

// displayNameSQLTrap is the locale trap docs/MIRROR.md and sqlhint already own.
// Restated in sql / search / recipes help so the command itself says it
// (GDK-503). Verified: issues.status / issue_type / priority are display
// names; status_category is new|inprogress|done; priority_rank and
// issue_type_id are the stable keys (specs/000-product/data-model.md).
const displayNameSQLTrap = "status, priority, and issue type names localize per account — key on status_category (new|inprogress|done), priority_rank, or issue_type_id; status = 'In Progress' can return 0 rows"

// helps is the per-command help table. Summaries recycle the top-level usage
// constant; positionals and examples match the real cmdXxx implementations.
var helps = map[string]cmdHelp{
	"init": {
		summary: initSummary,
		usage: "gadak [--workspace <name>] init [--local] [--site URL] [--email ADDR]\n" +
			"[--projects A,B] [--spaces KEYS|all|none]\n" +
			"[--token-file PATH | --token-stdin] [--token-expires DATE]\n" +
			"[--pairing-code OFFER | --pairing-code-stdin] [--json]",
		// FlagSet VisitAll supplies Options when `gadak init --help` runs; this
		// list covers formatHelp(nil) and documents the env-only token path.
		options: []helpOption{
			{name: "local", desc: "create a workspace on the built-in tracker, running here (no Jira site or credential)"},
			{name: "replace-local", desc: replaceLocalOriginUsage},
			{name: "site", desc: "Jira site URL (https://your-site.atlassian.net); env GADAK_SITE"},
			{name: "email", desc: "account email; env GADAK_EMAIL"},
			{name: "projects", desc: "project keys, comma-separated (optional — blank syncs every project you can see); env GADAK_PROJECTS"},
			{name: "spaces", desc: spacesFlagUsage + `; omit to leave unchanged`},
			{name: "token-file", desc: "read API token from this file"},
			{name: "token-stdin", desc: "read API token from stdin"},
			{name: "token-expires", desc: "token expiry date from Atlassian's create dialog (YYYY-MM-DD or RFC3339); omit to assume 365 days from verification"},
			{name: "token", desc: "not accepted; use GADAK_TOKEN, --token-file, or --token-stdin"},
			{name: "pairing-code", desc: "pairing offer from the home machine's `gadak pairing mint`; binds this fresh workspace to that serve as its origin (verified against the serve before anything is saved)"},
			{name: "pairing-code-stdin", desc: "read the pairing offer from stdin (keeps the secret out of ps and shell history)"},
			{name: "json", desc: "emit one JSON object on success"},
		},
		examples: []string{
			"gadak init",
			"gadak init --local",
			"gadak --workspace demo init",
			"GADAK_TOKEN=$(cat token) gadak init --site https://x.atlassian.net --email you@example.com --json",
			"gadak init --site https://x.atlassian.net --email you@example.com --projects ABC --token-file ./token",
			"gadak init --spaces ENG,PROD",
			"gadak init --spaces all",
			"gadak init --spaces none",
			"gadak --workspace home init --pairing-code-stdin <<< \"$OFFER\"",
		},
		seeAlso: []string{"gadak sync", "gadak profiles", "gadak pairing", "gadak config"},
	},
	"pairing": {
		summary: "manage the device tokens that gate a home serve's origin passthrough (origin scope), mirror REST (serve scope), and terminal (terminal scope); a paired remote machine binds with `gadak init --pairing-code`",
		usage: "gadak [--workspace <name>] pairing mint --label NAME [--scope origin|serve|terminal] [--ttl 90d] [--endpoint URL] [--no-qr] [--json]\n" +
			"| pairing list [--json] | pairing revoke <label|hash-prefix>",
		options: []helpOption{
			{name: "label", desc: "device name shown in `gadak pairing list` (required, unique among active tokens)"},
			{name: "scope", desc: "what the token opens: origin (default) rides the origin passthrough for a paired gadak; serve opens the whole mirror REST for a paired client such as a phone companion; terminal opens a shell on this machine (the terminal pane) and nothing else — never a default, you have to type it. Each scope is refused on the other two surfaces. A leaked serve token leaks this workspace's data; a leaked terminal token leaks the machine, so give it a short --ttl and revoke it when the device is done — revoking closes the shells it opened"},
			{name: "ttl", desc: "token lifetime: <N><d|h|m|s>, e.g. 90d (default) or 12h"},
			{name: "endpoint", desc: "URL remote devices reach this serve at; default is this machine's live serve address, refused when that is loopback (it is, unless serve runs --allow-remote) — pass your tailnet URL"},
			{name: "no-qr", desc: "skip the scannable QR mint draws below the offer line — the phone app scans it instead of copy-paste. Drawn only when stderr is a terminal and the code fits its width; --json, NO_COLOR, and TERM=dumb never draw one"},
			{name: "json", desc: "emit JSON (mint and list)"},
		},
		examples: []string{
			"gadak pairing mint --label laptop",
			"gadak pairing mint --label agent --ttl 12h --endpoint https://<machine>.<tailnet>.ts.net",
			"gadak pairing mint --label phone --scope serve --endpoint https://<machine>.<tailnet>.ts.net",
			"gadak pairing mint --label phone-shell --scope terminal --ttl 12h --endpoint https://<machine>.<tailnet>.ts.net",
			"gadak pairing mint --label laptop --json",
			"gadak pairing list",
			"gadak pairing list --json",
			"gadak pairing revoke laptop",
			"gadak pairing revoke a1b2c3d4  # hash prefix, 8 or more hex chars from pairing list",
			"# on the remote machine:",
			"gadak --workspace home init --pairing-code-stdin <<< \"$OFFER\"",
		},
		seeAlso: []string{"gadak serve", "gadak init"},
	},
	"config": {
		summary: "get or set workspace settings (everything the Settings dialog can edit)",
		usage:   "gadak [--workspace <name>] config [list|get <path>|set <path> <value>] [--json]",
		options: []helpOption{
			{name: "json", desc: "emit JSON (list and get; set prints the stored value)"},
		},
		examples: []string{
			"gadak config list",
			"gadak config list --json",
			"gadak config get appearance.theme",
			"gadak config set appearance.theme dark",
			"gadak config set syncIntervalSec 30",
			"gadak config set features.feed true",
			"gadak config set projects '[\"NMB\",\"NMA\"]'",
			"gadak config set defaultProject NMB",
			"gadak config set defaultIssueTypeId 10001",
		},
		seeAlso: []string{"gadak init", "gadak status"},
	},
	"sync": {
		summary: "mirror the workspace origin into the local SQLite database",
		usage:   "gadak [--workspace <name>] sync [--full] [--watch] [--source jira|linear|confluence|all] [--if-stale DUR]",
		examples: []string{
			"gadak sync                 # incremental, what a serve loop does",
			"gadak sync --full          # after changing projects or a mapping",
			"gadak sync --if-stale 15m  # no-op when every source is fresh",
			"gadak --workspace demo sync  # against another site",
		},
		seeAlso: []string{"gadak serve", "gadak status"},
	},
	"serve": {
		summary: "web UI and API on loopback (" + serveSyncDefault + ")",
		usage: "gadak [--workspace <name>] serve\n" +
			"[--addr HOST:PORT] [--static DIR] [--no-sync] [--no-open] [--allow-remote]",
		examples: []string{
			"gadak serve",
			"gadak serve --addr 127.0.0.1:7778 --no-open",
			"gadak --workspace demo serve --no-sync",
		},
		seeAlso: []string{"gadak sync", "gadak demo", "gadak install-service"},
	},
	"install-service": {
		summary: "keep serve running across reboots (launchd / systemd user)",
		usage:   "gadak [--workspace <name>] install-service [--uninstall]",
		examples: []string{
			"gadak install-service",
			"gadak install-service --uninstall",
			"gadak --workspace work install-service",
		},
		seeAlso: []string{"gadak serve"},
	},
	"install-cli": {
		summary: "put this binary on PATH (prefers a PATH entry, else ~/.local/bin; %LOCALAPPDATA%\\Programs\\gadak on Windows)",
		usage:   "gadak install-cli [--dir <path>] [--force] [--print]",
		examples: []string{
			"gadak install-cli",
			"/Applications/Gadak.app/Contents/Resources/bin/gadak install-cli",
			"gadak install-cli --dir /usr/local/bin",
			"gadak install-cli --print",
			"gadak install-cli --force",
		},
		seeAlso: []string{"gadak mcp install claude", "gadak version"},
	},
	"status": {
		summary: "print sync state and row counts",
		usage:   "gadak [--workspace <name>] status [--json]",
		examples: []string{
			"gadak status",
			"gadak status --json",
			"gadak --workspace demo status",
		},
		seeAlso: []string{"gadak sync", "gadak sql", "gadak doctor"},
	},
	"doctor": {
		summary: "print redacted diagnostics safe to paste into a bug report",
		usage:   "gadak [--workspace <name>] doctor [--json]",
		examples: []string{
			"gadak doctor",
			"gadak doctor --json",
			"gadak --workspace demo doctor",
		},
		seeAlso: []string{"gadak status", "gadak version"},
	},
	"demo": {
		summary: "serve the bundled snapshot; no Jira account needed",
		usage:   "gadak demo [options]",
		examples: []string{
			"gadak demo",
			"gadak demo --addr 127.0.0.1:7879 --no-open",
			"gadak demo --db examples/demo.db",
		},
		seeAlso: []string{"gadak serve", "gadak snapshot"},
	},
	"backup": {
		summary: "copy the built-in tracker's persist file (origin/issuetap.db — the record; the mirror is a cache) to one self-contained SQLite file; safe while serve runs",
		usage:   "gadak [--workspace <name>] backup [--to <dir|file>] [--json]",
		examples: []string{
			"gadak backup",
			"gadak backup --to /srv/backups",
			"gadak --workspace plan backup --to plan-$(date +%F).db --json",
		},
		seeAlso: []string{"gadak sync", "gadak doctor"},
	},
	"export": {
		summary: "dump saved views, watches, favorites, and recents as JSON (no credentials)",
		usage:   "gadak [--workspace <name>] export [--out FILE]",
		options: []helpOption{
			{name: "out", desc: "write to this file instead of stdout"},
		},
		examples: []string{
			"gadak export",
			"gadak export --out gadak-personal.json",
		},
		seeAlso: []string{"gadak import", "gadak team export", "gadak export-static"},
	},
	"import": {
		summary: "restore saved views, watches, favorites, and recents from a gadak export file (file wins on name/key conflict)",
		usage:   "gadak [--workspace <name>] import <FILE>",
		examples: []string{
			"gadak import gadak-personal.json",
		},
		seeAlso: []string{"gadak export", "gadak team import"},
	},
	"dev": {
		summary: "development-panel links: record PRs, deployments, and builds on issues (built-in tracker, here or paired)",
		usage: "gadak dev link <KEY> --pr <url>\n" +
			"[--status open|merged|declined] [--name N] [--author LOGIN] [--branch REF] [--json]\n" +
			"gadak dev scan [--dry-run] [--install-hook]\n" +
			"gadak dev deploy <KEY> --env <name> --state <state> [--url <run url>] [--json]\n" +
			"gadak dev build <KEY> --state successful|failed|unknown (--number N | --url <build url>) [--json]",
		options: []helpOption{
			{name: "pr", desc: "pull request URL (required for link)"},
			{name: "status", desc: "open (default), merged, or declined"},
			{name: "name", desc: "display title shown in the panel"},
			{name: "author", desc: "link: PR author login (omitted = the origin keeps what it holds)"},
			{name: "branch", desc: "link: head ref (omitted = the current git branch)"},
			{name: "dry-run", desc: "scan: list matched PRs without writing"},
			{name: "install-hook", desc: "scan: add a pre-push hook that runs `gadak dev scan`"},
			{name: "env", desc: "deploy: target environment, e.g. production (required)"},
			{name: "state", desc: "deploy: deployment state, e.g. successful (required); build: successful | failed | unknown (required)"},
			{name: "url", desc: "deploy: run URL (omitted = the origin keys the row by its environment); build: build URL (required when --number is omitted)"},
			{name: "number", desc: "build: build number (required when --url is omitted)"},
			{name: "json", desc: "emit JSON (link, deploy, build)"},
		},
		examples: []string{
			"gadak dev link STD-3 --pr https://github.com/org/app/pull/7 --status merged",
			"gadak dev scan            # match issue keys in `gh pr list` titles/branches, link them all",
			"gadak dev deploy STD-3 --env production --state successful",
			"gadak dev build STD-3 --state successful --number 12",
		},
		seeAlso: []string{"gadak issue", "gadak sync"},
	},
	"export-static": {
		summary: "freeze a snapshot database into static JSON for a hosted demo",
		usage:   "gadak export-static [options] <outdir>",
		examples: []string{
			"gadak export-static dist/static-demo",
			"gadak export-static --db examples/demo.db out/",
			"gadak export-static --api-base /gadak/api/v1/issues/ dist/demo",
			"gadak export-static --projects GDK --scrub --db mirror.db out/backlog",
		},
		options: []helpOption{
			{name: "projects", desc: "comma-separated project keys baked into the snapshot config"},
			{name: "scrub", desc: "whitelist-rebuild for public backlog publishing — drops descriptions, comments, attachments, history, people and custom fields"},
		},
		seeAlso: []string{"gadak demo", "gadak snapshot"},
	},
	"profiles": {
		summary: "list workspaces: which mirrors exist, which one this command used (same as workspaces); rm removes one",
		usage: "gadak profiles [--json]\n" +
			"| profiles rm <name> [--yes] [--destroy-origin] [--json]",
		options: []helpOption{
			{name: "json", desc: "emit JSON"},
			{name: "yes", desc: "with rm: remove — without it, rm explains and refuses"},
			{name: "destroy-origin", desc: "on the built-in tracker: also delete its persist, the only copy of that tracker"},
		},
		examples: []string{
			"gadak profiles",
			"gadak profiles --json",
			"gadak profiles rm demo --yes  # Jira: mirror+credential only, the origin is untouched",
			"gadak profiles rm local --yes --destroy-origin  # persist is the only copy of that tracker",
		},
		seeAlso: []string{"gadak init", "gadak workspace", "gadak workspaces"},
	},
	"workspace": {
		summary: "show the active workspace and what selected it; use NAME stores a default for later commands with no flag or env",
		usage: "gadak workspace [--json]\n" +
			"| workspace use <name>\n" +
			"| workspace use --clear",
		examples: []string{
			"gadak workspace",
			"gadak --workspace oss workspace",
			"GADAK_WORKSPACE=oss gadak workspace",
			"gadak workspace --json",
			"gadak workspace use oss",
			"gadak workspace use --clear",
		},
		seeAlso: []string{"gadak workspaces", "gadak profiles", "gadak status"},
	},
	"workspaces": {
		summary: "list workspaces: which mirrors exist, which one this command used (same as profiles); rm removes one",
		usage: "gadak workspaces [--json]\n" +
			"| workspaces rm <name> [--yes] [--destroy-origin] [--json]",
		options: []helpOption{
			{name: "json", desc: "emit JSON"},
			{name: "yes", desc: "with rm: remove — without it, rm explains and refuses"},
			{name: "destroy-origin", desc: "with rm on a local-origin workspace: also delete its persist, the only copy of that tracker"},
		},
		examples: []string{
			"gadak workspaces",
			"gadak workspaces --json",
			"gadak workspaces rm demo --yes  # Jira: mirror+credential only, the origin is untouched",
			"gadak workspaces rm local --yes --destroy-origin  # persist is the only copy of that tracker",
		},
		seeAlso: []string{"gadak workspace", "gadak profiles", "gadak init"},
	},
	"sql": {
		summary: "run a read-only SQL query against the local mirror; " + displayNameSQLTrap,
		usage: "gadak [--workspace <name>] sql [--json|--csv] [--no-header]\n" +
			"\"select ...\"",
		options: []helpOption{
			{name: "json", desc: "emit one JSON object per row"},
			{name: "csv", desc: "emit CSV with a header row"},
			{name: "no-header", desc: "omit the TSV/CSV header row (no-op with --json)"},
		},
		examples: []string{
			"gadak sql \"select count(*) from issues\"",
			"gadak sql --json \"select key, status from issues_full limit 5\"",
			"gadak sql --csv \"select key from issues where status_category = 'done'\"",
			"gadak sql --no-header \"select key from issues limit 5\"",
		},
		seeAlso: []string{"gadak issue", "gadak search", "gadak recipes", "gadak status"},
	},
	"api": {
		summary: "call Atlassian REST with the stored credential (escape hatch for endpoints the mirror does not cover)",
		usage: "gadak [--workspace <name>] api [METHOD] <PATH>\n" +
			"[--query k=v]... [--data <val|@file|->] [--write] [--status]",
		examples: []string{
			"gadak api /rest/api/3/myself",
			"gadak api GET /rest/api/3/issue/ABC-1/watchers",
			"gadak api GET /wiki/api/v2/spaces --query limit=5",
			"gadak api POST /rest/api/3/issue/ABC-1/worklog --data @wl.json --write",
		},
		seeAlso: []string{"gadak issue", "gadak comment", "gadak fields"},
	},
	"issue": {
		summary: "print full detail for one or more issues from the local mirror; --editmeta asks the origin which configured fields this issue can edit",
		usage: "gadak [--workspace <name>] issue <KEY> [KEY...] [--keys …]\n" +
			"[--json] [--derive] [--link] [--editmeta]",
		examples: []string{
			"gadak issue NMB-140",
			"gadak issue NMB-140 NMB-141",
			"gadak issue --keys -",
			"gadak issue NMB-140 --json",
			"gadak issue NMB-140 --link",
			"gadak issue NMB-140 --editmeta",
		},
		seeAlso: []string{"gadak search", "gadak open", "gadak sql", "gadak views open"},
	},
	// Five blind sessions asked to read one issue all reached for
	// `show` first. The verb they typed now works.
	"show": {
		summary: "alias of issue — full detail for one or more issues from the local mirror; --editmeta asks the origin which configured fields this issue can edit",
		usage: "gadak [--workspace <name>] show <KEY> [KEY...] [--keys …]\n" +
			"[--json] [--derive] [--link] [--editmeta]",
		examples: []string{
			"gadak show NMB-140",
			"gadak show NMB-140 NMB-141",
			"gadak show --keys -",
			"gadak show NMB-140 --json",
			"gadak show NMB-140 --link",
			"gadak show NMB-140 --editmeta",
		},
		seeAlso: []string{"gadak issue", "gadak search", "gadak open", "gadak views open"},
	},
	"open": {
		summary: "open the issue on your Jira site in the browser",
		usage:   "gadak [--workspace <name>] open <KEY>",
		examples: []string{
			"gadak open NMB-140",
		},
		seeAlso: []string{"gadak issue"},
	},
	"view": {
		summary: "list, show, or focus a Jira filter / saved view (alias of views)",
		usage:   "gadak [--workspace <name>] view [list|show|open|save] [--no-open] …",
		examples: []string{
			"gadak view",
			"gadak view open \"NMA in progress\"",
		},
		seeAlso: []string{"gadak views", "gadak search"},
	},
	"views": {
		summary: "list Jira filters and saved views; open one in the running UI",
		usage: "gadak [--workspace <name>] views [list|show <name>|open <name|KEY>|\n" +
			"save <name> --jql '…'] [--keys …] [--layout board|list] [--json] [--no-open]",
		examples: []string{
			"gadak views",
			"gadak views show \"NMA in progress\"",
			"gadak views open \"NMA in progress\"",
			"gadak views open NMB-140",
			"gadak views open --jql 'project = NMA AND statusCategory = \"In Progress\"'",
			"gadak views open --keys 'NMA-1,NMA-2'",
			"gadak views open --keys -",
			"gadak views open --jql 'project = NMA' --no-open",
			"gadak views open --jql 'project = NMA' --layout board --no-open",
			"gadak views save \"Night triage\" --jql 'assignee = currentUser() AND resolution is EMPTY'",
			"gadak views save \"NMA board\" --jql 'project = NMA' --layout board",
		},
		seeAlso: []string{"gadak search", "gadak sync"},
	},
	"search": {
		summary: "full-text search, or a JQL / Jira-URL filter against the mirror; " + displayNameSQLTrap,
		usage: "gadak [--workspace <name>] search [--jql] [--emit] [--limit N]\n" +
			"[--json] [--explain] \"text|JQL|URL\"",
		examples: []string{
			"gadak search \"flaky upload\" --limit 5",
			"gadak search \"idempotency\" --json",
			"gadak search NMB-140 --explain",
			"gadak search --jql 'project = NMA AND statusCategory = \"In Progress\"'",
			"gadak search 'https://your-site.atlassian.net/issues/?jql=project%20%3D%20NMA'",
			"gadak search --jql --emit 'assignee = currentUser()'",
		},
		seeAlso: []string{"gadak issue", "gadak sql"},
	},
	"list": {
		summary: "open issues from the local mirror, priority rank first — the default read before any query; done is hidden, updated_at breaks ties",
		usage: "gadak [--workspace <name>] list [--limit N] [--all] [--ready]\n" +
			"[--json|--csv|--no-header]",
		options: []helpOption{
			{name: "limit", desc: "maximum rows to list (default 30)"},
			{name: "all", desc: "include done issues (default hides them)"},
			{name: "ready", desc: "only issues no open blocker holds back: an inward Blocks link whose target is not done disqualifies. The blocking link type resolves against the origin's link-type catalog (one read; in-process when the origin is gadak's own); when no catalog can answer, a stderr notice says so and the filter is skipped"},
			{name: "json", desc: "emit one JSON object per row"},
			{name: "csv", desc: "emit CSV with a header row"},
			{name: "no-header", desc: "omit the TSV/CSV header row (no-op with --json)"},
		},
		examples: []string{
			"gadak list",
			"gadak list --limit 5 --json",
			"gadak list --all",
			"gadak list --ready             # same rows as gadak ready",
		},
		seeAlso: []string{"gadak ready", "gadak next", "gadak sql", "gadak search"},
	},
	"ready": {
		summary: "alias of list --ready — open issues no open blocker holds back (an inward Blocks link to an unfinished issue disqualifies)",
		usage:   "gadak [--workspace <name>] ready [--limit N] [--json|--csv|--no-header]",
		options: []helpOption{
			{name: "limit", desc: "maximum rows to list (default 30)"},
			{name: "json", desc: "emit one JSON object per row"},
			{name: "csv", desc: "emit CSV with a header row"},
			{name: "no-header", desc: "omit the TSV/CSV header row (no-op with --json)"},
		},
		examples: []string{
			"gadak ready",
			"gadak ready --json",
		},
		seeAlso: []string{"gadak list", "gadak next", "gadak link"},
	},
	"recents": {
		summary: "list the keys this workspace read recently, newest first — the first command after a context compaction or session restart (issue reads record themselves; searches go to search history, not this list)",
		usage:   "gadak [--workspace <name>] recents [--limit N] [--json]",
		examples: []string{
			"gadak recents",
			"gadak recents --limit 5",
			"gadak recents --json",
		},
		seeAlso: []string{"gadak issue", "gadak search"},
	},
	// Singular spelling of recents, registered with `done` in the same
	// same verb-synonym round.
	"recent": {
		summary: "alias of recents — the keys this workspace read recently, newest first (issue reads record themselves; searches go to search history, not this list)",
		usage:   "gadak [--workspace <name>] recent [--limit N] [--json]",
		examples: []string{
			"gadak recent",
			"gadak recent --limit 5",
			"gadak recent --json",
		},
		seeAlso: []string{"gadak recents", "gadak issue", "gadak search"},
	},
	"recipes": {
		summary: "named read-only SQL stored in local.db; " + displayNameSQLTrap,
		usage: "gadak [--workspace <name>] recipes [list]\n" +
			"| recipes save NAME [\"sql\" | -m <text|->]\n" +
			"| recipes run NAME [--json|--csv|--no-header]\n" +
			"| recipes show NAME | recipes rm NAME",
		options: []helpOption{
			{name: "json", desc: "emit one JSON object per row"},
			{name: "csv", desc: "emit CSV with a header row"},
			{name: "no-header", desc: "omit the TSV/CSV header row (no-op with --json)"},
			{name: "m", desc: "recipe SQL; - reads stdin (save)"},
		},
		examples: []string{
			"gadak recipes",
			`gadak recipes save next "` + nextRecipeSQL + `"`,
			"gadak recipes run next",
			"gadak recipes run next --json",
			"gadak recipes show next",
			"gadak recipes show next | gadak recipes save next -m -",
			"gadak recipes rm next",
		},
		seeAlso: []string{"gadak next", "gadak sql", "gadak search"},
	},
	"next": {
		summary: "run the recipe named next, or the built-in list default when none is saved (a stderr line says which) — a report, not occupancy (claiming is an origin write)",
		usage:   "gadak [--workspace <name>] next [--json|--csv|--no-header]",
		options: []helpOption{
			{name: "json", desc: "emit one JSON object per row"},
			{name: "csv", desc: "emit CSV with a header row"},
			{name: "no-header", desc: "omit the TSV/CSV header row (no-op with --json)"},
		},
		examples: []string{
			`gadak recipes save next "` + nextRecipeSQL + `"`,
			"gadak next",
			"gadak next --json",
			"gadak next                      # no recipe saved: built-in default + the save command on stderr",
		},
		seeAlso: []string{"gadak recipes", "gadak list", "gadak sql", "gadak claim"},
	},
	// GDK-992: the v0.17 changelog advertises `gadak pick` ("chooses work"),
	// and the changelog renders on the site where agents read it as API doc.
	// History is not edited here — the verb is made real instead.
	"pick": {
		summary: "alias of next — the recipe named next, or the built-in default when none is saved (the changelog's name for choosing work)",
		usage:   "gadak [--workspace <name>] pick [--json|--csv|--no-header]",
		options: []helpOption{
			{name: "json", desc: "emit one JSON object per row"},
			{name: "csv", desc: "emit CSV with a header row"},
			{name: "no-header", desc: "omit the TSV/CSV header row (no-op with --json)"},
		},
		examples: []string{
			"gadak pick",
			"gadak pick --json",
		},
		seeAlso: []string{"gadak next", "gadak recipes", "gadak list", "gadak claim"},
	},
	"dashboards": {
		summary: "agent dashboards — an HTML wall plus named sql/jql datasources, saved in local.db like a view; " + displayNameSQLTrap,
		usage: "gadak [--workspace <name>] dashboards [list|show <name>|open <name>|rm <name>]\n" +
			"dashboards save <name> --html <file|-> [--datasource name=sql:…]… [--datasource name=jql:…]… [--lib <id>]… [--json]\n" +
			"dashboards lib add <url> [--replace] | lib list [--json] | lib rm <id>",
		options: []helpOption{
			{name: "html", desc: "HTML document file; - reads stdin (save)"},
			{name: "datasource", desc: "named datasource name=sql:QUERY or name=jql:QUERY (repeatable)"},
			{name: "lib", desc: "cached library id to declare (repeatable; ids come from `dashboards lib add`)"},
			{name: "json", desc: "emit JSON"},
			{name: "no-open", desc: "write the focus hash only; do not open a window (open)"},
			{name: "replace", desc: "accept an upstream change when the same url now serves different bytes (lib add)"},
		},
		examples: []string{
			"gadak dashboards",
			"gadak dashboards save triage --html examples/dashboards/triage.html \\",
			"  --datasource open_by_status='sql:select status_category, count(*) from issues_full where status_category != ''done'' group by 1'",
			"gadak dashboards save mine --html wall.html --datasource mine=\"jql:assignee = currentUser() AND resolution is EMPTY\"",
			"gadak dashboards lib add https://cdn.jsdelivr.net/npm/three@0.149.0/build/three.min.js",
			"gadak dashboards save model --html model.html --lib <id-from-lib-add>",
			"gadak dashboards show triage",
			"gadak dashboards open triage",
			"gadak dashboards rm triage",
		},
		seeAlso: []string{"gadak recipes", "gadak sql", "gadak views"},
	},
	"comment": {
		summary: "add a comment (@Name resolves to a site user; ambiguous names are refused)",
		usage: "gadak [--workspace <name>] comment <KEY> [<text> | -m <text|->]\n" +
			"[--visibility role=NAME|group=NAME] [--internal] [--json] | --batch -",
		examples: []string{
			"gadak comment NMB-140 Reproduced on staging.",
			"gadak comment NMB-140 -m \"Reproduced on staging.\"",
			"gadak comment NMB-140 -m \"thanks @Dana\"",
			"gadak comment NMB-140 -m -          # body from stdin",
			"gadak comment NMB-140 -m \"done\" --json",
			`printf '%s\n' '{"key":"NMB-140","body":"reproduced"}' | gadak comment --batch -`,
		},
		seeAlso: []string{"gadak transition", "gadak assign", "gadak issue"},
	},
	"create": {
		summary: "create an issue",
		usage: "gadak [--workspace <name>] create [--] <SUMMARY> | --batch -\n" +
			"[--project KEY] [--type NAME-or-id] [--priority NAME-or-id]\n" +
			"[--due YYYY-MM-DD] [--parent KEY]\n" +
			"[--label L]... [--attach FILE]... [-m <text|->]\n" +
			"[--field alias=value]... [--json]",
		examples: []string{
			"gadak create Fix the flaky gate --project NMB --type Task \\\n    -m \"repro on staging\" --label batch",
			"gadak create 로그인 실패 --project NMB --type 작업",
			"gadak create Night triage item --project NMB --type Task --priority High --due 2026-09-01",
			"gadak create Severity required --project NMB --type Task --field severity=High",
			"gadak create --project NMB --type Task -- --rollback-on-failure",
			`printf '%s\n' '{"summary":"one"}' '{"summary":"two"}' | gadak create --batch - --project NMB --type Task`,
		},
		seeAlso: []string{"gadak attach", "gadak edit", "gadak comment", "gadak transition", "gadak assign", "gadak issue"},
	},
	"attach": {
		summary: "attach files",
		usage:   "gadak [--workspace <name>] attach <KEY> <file>... [--json]",
		examples: []string{
			"gadak attach NMB-140 ./screenshot.png",
			"gadak attach NMB-140 ./trace.log ./out.mp4 --json",
		},
		seeAlso: []string{"gadak create", "gadak edit", "gadak issue"},
	},
	"edit": {
		summary: "edit summary, description, labels, components, fix versions, issue type, priority, parent, due date, or a configured custom field",
		usage: "gadak [--workspace <name>] edit <KEY> [--summary S] [-m <text|->] [--force-plain]\n" +
			"[--label +x|-x]... [--component +x|-x]...\n" +
			"[--fix-version +id-or-name|-id-or-name]...\n" +
			"[--type NAME-or-id] [--priority NAME-or-id] [--due YYYY-MM-DD|none] [--parent KEY|none]\n" +
			"[--field alias=value]... [--json] | --batch -",
		examples: []string{
			"gadak edit NMB-140 --summary \"Rename without opening Jira\"",
			"gadak edit NMB-140 --label +batch --label -legacy --priority High",
			"gadak edit NMB-140 --type Bug",
			"gadak edit NMB-140 --component +SDK --component -Docs",
			"gadak edit NMB-140 --fix-version +v2.5 --fix-version -10012",
			"gadak edit NMB-140 -m \"plain rewrite\" --force-plain",
			"gadak edit NMB-140 --due 2026-09-01",
			"gadak edit NMB-140 --due none",
			"gadak edit NMB-140 --field severity=High",
			`printf '%s\n' '{"key":"NMB-140","labels":["+regression"]}' | gadak edit --batch -`,
		},
		seeAlso: []string{"gadak create", "gadak attach", "gadak comment", "gadak issue"},
	},
	"page": {
		summary: "wiki pages — read from the mirror (get, list; no network), write through the origin (create, edit, comment; Confluence, or gadak's own wiki)",
		usage: "gadak [--workspace <name>] page get <ID> [--json]\n" +
			"| page list [--space K] [--limit N] [--json|--csv|--no-header]\n" +
			"| page create|edit|comment [<ID>]\n" +
			"[--space K] [--title T] [-m <text|->] [--adf-file F]\n" +
			"[--parent ID] [--version N] [--force] [--json]",
		options: []helpOption{
			{name: "space", desc: "create in this space key (create); list only this space (list)"},
			{name: "limit", desc: "maximum rows to list (default 30) (list)"},
			{name: "json", desc: "one JSON object for get; one per row for list"},
			{name: "csv", desc: "emit CSV with a header row (list)"},
			{name: "no-header", desc: "omit the TSV/CSV header row (list)"},
		},
		examples: []string{
			"gadak page get 12345",
			"gadak page get 12345 --json",
			"gadak page list",
			"gadak page list --space ENG --limit 5",
			"gadak page create --space ENG --title \"Retention notes\" -m \"first draft\"",
			"gadak page edit 12345 --title \"Renamed page\"",
			"gadak page edit 12345 -m \"whole new body\" --force",
			"gadak page edit 12345 --append -m \"follow-up paragraph, keeps the rest\"",
			"gadak page edit 12345 --adf-file body.adf.json",
			"gadak page comment 12345 -m \"question on the retention section\"",
		},
		seeAlso: []string{"gadak search", "gadak open"},
	},
	// The wiki's own noun — the verb a session reading or writing
	// pages reaches for when it doesn't remember `page`.
	"wiki": {
		summary: "alias of page — wiki pages read from the mirror (get, list; no network), written through the origin (create, edit, comment)",
		usage: "gadak [--workspace <name>] wiki get <ID> [--json]\n" +
			"| wiki list [--space K] [--limit N] [--json|--csv|--no-header]\n" +
			"| wiki create|edit|comment [<ID>]\n" +
			"[--space K] [--title T] [-m <text|->] [--adf-file F]\n" +
			"[--parent ID] [--version N] [--force] [--json]",
		options: []helpOption{
			{name: "space", desc: "create in this space key (create); list only this space (list)"},
			{name: "limit", desc: "maximum rows to list (default 30) (list)"},
			{name: "json", desc: "one JSON object for get; one per row for list"},
			{name: "csv", desc: "emit CSV with a header row (list)"},
			{name: "no-header", desc: "omit the TSV/CSV header row (list)"},
		},
		examples: []string{
			"gadak wiki get 12345",
			"gadak wiki list --space ENG",
			"gadak wiki create --space ENG --title \"Retention notes\" -m \"first draft\"",
			"gadak wiki edit 12345 --title \"Renamed page\"",
			"gadak wiki comment 12345 -m \"question on the retention section\"",
		},
		seeAlso: []string{"gadak page", "gadak search", "gadak open"},
	},
	"ref": {
		summary: "point this issue at an issue in another workspace (built-in tracker, here or paired) — the list hydrates the target's live state from that workspace's own mirror, no network",
		usage: "gadak [--workspace <name>] ref <KEY> <workspace>/<TARGET-KEY>|<url> [--as <relationship>] [--json]\n" +
			"| ref <KEY> --list [--json] | ref <KEY> --rm <id>",
		examples: []string{
			"gadak --workspace plan ref STD-1 work/NMA-9",
			"gadak --workspace plan ref STD-1 work/NMA-9 --as \"blocked by\"",
			"gadak --workspace plan ref STD-1 --list",
		},
		seeAlso: []string{"gadak link", "gadak issue", "gadak workspaces"},
	},
	"migrate": {
		summary: "export a workspace's mirror into a new workspace on the built-in tracker — issues, comments, history, links, attachments, and wiki pages leave with you; ends with a source-vs-migrated count report",
		usage:   "gadak --workspace <new name> migrate --from <workspace> [--projects A,B] [--spaces X,Y] [--skip-attachments] [--json]",
		examples: []string{
			"gadak --workspace local migrate --from work --projects GDK",
			"gadak --workspace local migrate --from work --skip-attachments",
		},
		seeAlso: []string{"gadak init", "gadak workspaces", "gadak sync"},
	},
	"memory": {
		summary: "agent memory — leave a note the next session can find (a page in the memory space), and search just those notes; the right verb for \"leave it so the next session finds it\"",
		usage: "gadak [--workspace <name>] memory add <text> | -m <text|-> [--title T] [--json]\n" +
			"| memory search <query> [--limit N] [--json|--no-header]",
		options: []helpOption{
			{name: "title", desc: "page title (omitted = derived from the note's first line) (add)"},
			{name: "m", desc: "note as plain text; `-` reads stdin — or pass the note as an argument (add)"},
			{name: "limit", desc: "maximum matches (default 20) (search)"},
			{name: "json", desc: "what was written for add; one object per match for search"},
			{name: "no-header", desc: "omit the TSV header row (search)"},
		},
		examples: []string{
			"gadak memory add \"release audit lives in docs/runbooks/release-audit.md — check CI after\"",
			"gadak memory add -m -  # note from stdin",
			"gadak memory search \"release audit\"",
			"gadak config set memory.space ENG  # Jira workspaces refuse until this is set",
		},
		seeAlso: []string{"gadak page", "gadak search", "gadak config"},
	},
	"project": {
		summary: "grow a built-in-tracker workspace by one project key (Jira workspaces create projects in Jira)",
		usage:   "gadak [--workspace <name>] project create <KEY> [--name N] [--json]",
		examples: []string{
			"gadak project create IDEA --name Ideas",
			"gadak create --project IDEA \"first idea\"",
		},
		seeAlso: []string{"gadak create", "gadak init"},
	},
	"transition": {
		summary: "change issue status; accepts transition id, target status id, name, target status name, or status category new|inprogress|done; already in that category is a no-op",
		usage: "gadak [--workspace <name>] transition <KEY>\n" +
			"<transition-id|status-id|name|new|inprogress|done>\n" +
			"[--resolution name|id] [--field key=JSON]... [-m text] [--json] | --batch - [--dry-run]",
		examples: []string{
			"gadak transition NMB-140 \"In Review\"",
			"gadak transition NMB-140 31",
			"gadak transition NMB-140 done",
			"gadak transition NMB-140 done --resolution \"Won't Do\"",
			"gadak transition NMB-140 done -m \"fixed in 1.2\"",
			`printf '%s\n' '{"key":"NMB-140","target":"done"}' | gadak transition --batch -`,
			`printf '%s\n' '{"key":"NMB-140","target":"done"}' | gadak transition --batch - --dry-run`,
		},
		seeAlso: []string{"gadak close", "gadak comment", "gadak assign", "gadak issue"},
	},
	"close": {
		summary: "close an issue (transition to status category done); already done is a no-op",
		usage: "gadak [--workspace <name>] close <KEY>\n" +
			"[--resolution name|id] [--field key=JSON]... [-m text] [--json]",
		examples: []string{
			"gadak close NMB-140",
			"gadak close NMB-140 -m \"fixed in 1.2\"",
			"gadak close NMB-140 --json",
			"gadak transition NMB-140 inprogress  # reopen; there is no gadak reopen",
			"gadak transition NMB-140 new",
		},
		seeAlso: []string{"gadak transition", "gadak comment", "gadak issue"},
	},
	// Close's own word — the status category the command lands on.
	"done": {
		summary: "alias of close — transition an issue to status category done; already done is a no-op",
		usage: "gadak [--workspace <name>] done <KEY>\n" +
			"[--resolution name|id] [--field key=JSON]... [-m text] [--json]",
		examples: []string{
			"gadak done NMB-140",
			"gadak done NMB-140 -m \"fixed in 1.2\"",
			"gadak done NMB-140 --json",
			"gadak transition NMB-140 inprogress  # reopen; there is no gadak reopen",
		},
		seeAlso: []string{"gadak close", "gadak transition", "gadak issue"},
	},
	"assign": {
		summary: "set the assignee; pass - to unassign",
		usage:   "gadak [--workspace <name>] assign <KEY> <email|name|accountId|-> [--json] | --batch -",
		examples: []string{
			"gadak assign NMB-140 dana@example.com",
			"gadak assign NMB-140 -                 # unassign",
			"gadak assign NMB-140 dana@example.com --json",
			`printf '%s\n' '{"key":"NMB-140","assignee":"-"}' | gadak assign --batch -`,
		},
		seeAlso: []string{"gadak claim", "gadak comment", "gadak transition", "gadak issue"},
	},
	"claim": {
		summary: "take an issue as yours — assignee plus the in-progress transition in one step (an issue not in progress is moved there); refuses (exit 75) while another actor holds it",
		usage:   "gadak [--workspace <name>] claim <KEY> [--take-over] [--json]",
		examples: []string{
			"gadak claim NMB-140",
			"gadak claim NMB-140 --json             # adds the claim answer to the JSON",
			"gadak claim NMB-140 --take-over        # replace the current holder",
		},
		seeAlso: []string{"gadak assign", "gadak transition", "gadak issue"},
	},
	"link": {
		summary: "create an issue link (A <type> B); not `gadak issue --link`, which prints a gadak:// URL",
		usage:   "gadak [--workspace <name>] link <A> <B> --type <name|inward|outward|id> [--json]",
		examples: []string{
			"gadak link NMB-140 NMB-141 --type blocks",
			"gadak link NMB-140 NMB-141 --type \"is blocked by\"",
		},
		seeAlso: []string{"gadak issue", "gadak edit", "gadak comment"},
	},
	"unlink": {
		summary: "remove an issue link — the one `gadak link A B --type t` created (looked up live for its id; the mirror carries none)",
		usage:   "gadak [--workspace <name>] unlink <A> <B> --type <name|inward|outward|id> [--json]",
		examples: []string{
			"gadak unlink NMB-140 NMB-141 --type blocks",
			"gadak unlink NMB-140 NMB-141 --type \"is blocked by\"",
		},
		seeAlso: []string{"gadak link", "gadak issue"},
	},
	"fields": {
		summary: "report which custom fields are populated (samples the mirror; queries Jira)",
		usage: "gadak [--workspace <name>] fields [--sample N] [--json] [--all]\n" +
			"[--project KEY] [--apply]",
		options: []helpOption{
			{name: "sample", desc: "number of mirrored issues to sample (default 200)"},
			{name: "json", desc: "emit JSON"},
			{name: "all", desc: "include system fields (default: custom only)"},
			{name: "project", desc: "limit the sample to one project key"},
			{name: "apply", desc: "discover in-use custom fields from the mirror, save specs, and backfill (no re-download)"},
		},
		examples: []string{
			"gadak fields",
			"gadak fields --sample 100 --project NMB",
			"gadak fields --all --json",
			"gadak fields --apply",
		},
		seeAlso: []string{"gadak status", "gadak sync", "gadak sql"},
	},
	"mcp": {
		summary: "MCP server on stdio for clients without a shell; install pins the workspace",
		usage:   "gadak [--workspace <name>] mcp [--no-sync] [install <client>]",
		options: []helpOption{
			{name: "no-sync", desc: "do not run the incremental sync loop"},
		},
		examples: []string{
			"gadak mcp",
			"gadak mcp --no-sync",
			"gadak --workspace demo mcp",
			"gadak mcp install claude",
			"gadak --workspace demo mcp install claude --dry-run",
			"gadak mcp install json",
		},
		seeAlso: []string{"gadak skill install", "gadak sql", "gadak issue", "gadak status", "gadak profiles"},
	},
	"skill": {
		summary: "install the Claude Code skill (schema + query patterns; no MCP process)",
		usage: "gadak skill install [client] [--project] [--dir <path>]\n" +
			"[--print] [--force]",
		options: []helpOption{
			{name: "project", desc: "install into ./.claude/skills/gadak/ under the current directory"},
			{name: "dir", desc: "install into PATH/gadak/SKILL.md (overrides default and --project)"},
			{name: "print", desc: "print the install plan without writing"},
			{name: "force", desc: "overwrite when the existing file differs from the embedded skill"},
		},
		examples: []string{
			"gadak skill install",
			"gadak skill install claude",
			"gadak skill install --print",
			"gadak skill install --project",
			"gadak skill install --dir /tmp/skills-preview --print",
			"gadak skill install --force",
		},
		seeAlso: []string{"gadak mcp install", "gadak sql", "gadak issue"},
	},
	"snapshot": {
		summary: "write a shareable copy of the mirror (no personal tables, no credentials)",
		usage:   "gadak [--workspace <name>] snapshot <out.db> [options]",
		options: []helpOption{
			{name: "from", desc: "source database path (default: this workspace's mirror)"},
			{name: "spread", desc: "restate timestamps across this window, keeping every issue's own order (e.g. 90d)"},
			{name: "scale", desc: "clone issues onto new keys until the snapshot holds this many"},
			{name: "seed", desc: "reserved for --scale determinism (default 1)"},
			{name: "now", desc: "pin the clock to an RFC3339 timestamp for reproducible builds"},
			{name: "force", desc: "overwrite out.db if it already exists"},
		},
		examples: []string{
			"gadak snapshot out.db",
			"gadak snapshot out.db --spread 90d --force        # spread a seeded set over 3 months",
			"gadak snapshot bench.db --scale 10000             # benchmark fixture, no 10k-issue site",
		},
		seeAlso: []string{"gadak demo", "gadak export-static", "gadak sync"},
	},
	"team": {
		summary: "export or import shareable team settings and saved views (no credentials)",
		usage:   "gadak [--workspace <name>] team export|import …",
		options: []helpOption{
			{name: "out", desc: "export: write to this file instead of stdout"},
			{name: "with-members", desc: "export: include members (emails)"},
			{name: "dry-run", desc: "import: print the merge plan without writing"},
			{name: "overwrite", desc: "import: replace conflicting settings and same-named views"},
		},
		examples: []string{
			"gadak team export --out gadak-team.json",
			"gadak team import gadak-team.json --dry-run",
			"gadak team import gadak-team.json",
			"gadak team import gadak-team.json --overwrite",
		},
		seeAlso: []string{"gadak init", "gadak status"},
	},
	"version": {
		summary: "print the gadak version",
		usage:   "gadak version",
		examples: []string{
			"gadak version",
		},
	},
}

// newFlagSet builds a FlagSet whose -h/--help prints formatHelp to stdout and
// exits 0 (flag.ExitOnError). Unknown flags still exit 2 after the same help.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() {
		fmt.Print(formatHelp(name, fs))
	}
	return fs
}

// parseAround pulls known flags out of args in any position and Parse()s them.
// The leftover is the positional query or name. Agents type
// `gadak search --jql '…' --json`; Go's FlagSet would otherwise swallow
// --json into the JQL because it stops at the first non-flag.
//
// A token that starts with `-` and is not a registered flag is rejected
// (GDK-41). Bare `-` is a value. `--` ends flag parsing so a positional that
// starts with `-` can be passed — but a registered flag beyond the first
// argument after `--` is refused (GDK-851): it would fold into the positional
// text instead of being parsed. New commands inherit this by using
// newFlagSet + parseAround.
func parseAround(fs *flag.FlagSet, args []string) (rest []string, err error) {
	needVal := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		need := true
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			need = false
		}
		needVal["-"+f.Name] = need
		needVal["--"+f.Name] = need
	})
	var flagArgs, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			tail := args[i+1:]
			// GDK-851: a registered flag token after `--` is a misplacement —
			// everything there is positional, so `create -- "summary" -m
			// "body"` folded -m and its value into the summary. The first
			// token is exempt (a leading-dash positional is why `--` exists)
			// and only exact registered names are refused.
			for j := 1; j < len(tail); j++ {
				if _, known := needVal[tail[j]]; known {
					return nil, newFlagAfterDashDash(fs, tail[j])
				}
			}
			pos = append(pos, tail...)
			break
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		name, _, inline := strings.Cut(a, "=")
		need, known := needVal[name]
		if !known {
			return nil, newUnknownFlag(fs, a)
		}
		flagArgs = append(flagArgs, a)
		if !inline && need {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag needs an argument: %s", name)
			}
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}
	return pos, nil
}

// unknownFlagErr is a rejected dash-token. main maps it to process exit 2
// so a mistyped flag is the same class of failure as an unknown subcommand,
// not a generic log.Fatalf (exit 1).
type unknownFlagErr struct {
	token    string
	accepted []string
	cmd      string
}

func (e *unknownFlagErr) Error() string {
	var b strings.Builder
	if len(e.accepted) == 0 {
		fmt.Fprintf(&b, "unknown flag %s", e.token)
	} else {
		fmt.Fprintf(&b, "unknown flag %s — accepted: %s", e.token, strings.Join(e.accepted, ", "))
	}
	fmt.Fprintf(&b, "\nrun \"gadak %s --help\" for examples", e.cmd)
	return b.String()
}

// newUnknownFlag lists the FlagSet's accepted names the way resolveCreateType
// lists available types: one line, enough to recover without a second invocation.
func newUnknownFlag(fs *flag.FlagSet, token string) error {
	var accepted []string
	fs.VisitAll(func(f *flag.Flag) {
		accepted = append(accepted, dashed(f.Name))
	})
	return &unknownFlagErr{token: token, accepted: accepted, cmd: fs.Name()}
}

// flagAfterDashDashErr is a registered flag token refused after `--`
// (GDK-851). Everything after `--` is positional, so `create -- "summary"
// -m "body"` folded -m and the body into the summary — a short one silently
// created the polluted issue, a long one died on the 255-char limit. The
// first token after `--` stays exempt (a leading-dash positional is why
// `--` exists) and unknown dash tokens stay positional; only exact
// registered names are refused.
type flagAfterDashDashErr struct {
	token string
	cmd   string
}

func (e *flagAfterDashDashErr) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s cannot follow --: everything after -- is positional text, so %s would fold into it\n", e.token, e.token)
	fmt.Fprintf(&b, "move %s before --, or quote the text as one argument when it really contains %q\n", e.token, e.token)
	fmt.Fprintf(&b, "run \"gadak %s --help\" for examples", e.cmd)
	return b.String()
}

// newFlagAfterDashDash refuses a registered flag token found after `--`.
func newFlagAfterDashDash(fs *flag.FlagSet, token string) error {
	return &flagAfterDashDashErr{token: token, cmd: fs.Name()}
}

// exitCoder is a command error that names its process status (e.g. unknown
// config path → 64). unknownFlagErr stays a separate 2 so a mistyped flag
// does not change class.
type exitCoder interface {
	ExitCode() int
}

// exitStatus is the process code main uses for a command error.
// Unknown or misplaced flags are usage (2); an exitCoder keeps the code it
// named; everything else stays 1.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var u *unknownFlagErr
	if errors.As(err, &u) {
		return 2
	}
	var f *flagAfterDashDashErr
	if errors.As(err, &f) {
		return 2 // a misplaced flag is the same usage class as a mistyped one
	}
	var c exitCoder
	if errors.As(err, &c) {
		return c.ExitCode()
	}
	return 1
}

// wantsHelp reports whether args contain -h or --help (for commands that do
// not use flag.FlagSet).
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// usageError keeps the existing one-line usage text and points at --help for
// examples. The first line is intentionally unchanged for callers that match it.
func usageError(cmd, line string) error {
	return fmt.Errorf("%s\nrun \"gadak %s --help\" for examples", line, cmd)
}

// unknownCommandError is the one-line unknown-subcommand refusal (GDK-466).
// 64 is EX_USAGE, the same class unknownConfigPath uses. A name close to a
// real verb carries a did-you-mean (GDK-1015): curated synonyms first — the
// word a session reaches for is often not a typo of anything — then edit
// distance against every command. Distant names stay unadorned; a bad guess
// is worse than none.
func unknownCommandError(name string) error {
	msg := fmt.Sprintf("unknown command %q — see gadak --help", name)
	if sug := suggestCommand(name); sug != "" {
		msg = fmt.Sprintf("unknown command %q — did you mean \"gadak %s\"? (see gadak --help)", name, sug)
	}
	return &exitCodeError{
		code: 64,
		msg:  msg,
	}
}

// commandSynonyms are near-verbs a session types when it wants the real one
// (GDK-1015). None of the keys are real commands — mapping them here is the
// refusal plus a pointer, not a new verb.
var commandSynonyms = map[string]string{
	"get":      "show",
	"read":     "show",
	"ls":       "list",
	"issues":   "list",
	"backlog":  "list",
	"history":  "recents",
	"finish":   "done",
	"complete": "done",
}

// suggestCommand returns the command the name most likely meant, or "" when
// nothing is close enough to say. The fallback walks the sorted command
// names keeping a strictly smaller distance, so the lexicographically first
// of equal-distance candidates wins.
func suggestCommand(name string) string {
	if sug, ok := commandSynonyms[name]; ok {
		return sug
	}
	best, bestDist := "", 3 // beyond the ≤2 suggestion threshold
	for _, c := range commandNames() {
		if d := levenshtein(name, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// levenshtein is the plain two-row edit distance. internal/sqlhint has one
// too, but it is unexported and this package must not reach into internal
// helpers for a ten-line function.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return len(b)
	}
	if b == "" {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			n := prev[j] + 1 // deletion
			if ins := cur[j-1] + 1; ins < n {
				n = ins // insertion
			}
			if sub := prev[j-1] + cost; sub < n {
				n = sub // substitution
			}
			cur[j] = n
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// jsonList returns s, or a non-nil empty slice so encoding/json writes [].
func jsonList[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// formatHelp renders the help text for name. When fs is non-nil, Options come
// from VisitAll (double-dash names). When fs is nil, Options come from the
// manual options slice on the help entry.
func formatHelp(name string, fs *flag.FlagSet) string {
	h, ok := helps[name]
	if !ok {
		return fmt.Sprintf("gadak: no help for %q\n", name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "gadak %s — %s\n\n", name, h.summary)
	writeUsage(&b, h.usage)

	optLines := optionLines(fs, h.options)
	if len(optLines) > 0 {
		b.WriteString("\nOptions:\n")
		for _, line := range optLines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	if len(h.examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, ex := range h.examples {
			for i, line := range strings.Split(ex, "\n") {
				if i == 0 {
					fmt.Fprintf(&b, "  %s\n", line)
					continue
				}
				fmt.Fprintln(&b, line)
			}
		}
	}

	if len(h.seeAlso) > 0 {
		fmt.Fprintf(&b, "\nSee also: %s\n", strings.Join(h.seeAlso, ", "))
	}
	return b.String()
}

// writeUsage prints the Usage block. The first line is indented two spaces;
// continuation lines keep the indent stored in the table (or hang by 7 spaces
// when the author omitted leading whitespace).
func writeUsage(b *strings.Builder, usage string) {
	fmt.Fprintln(b, "Usage:")
	lines := strings.Split(usage, "\n")
	fmt.Fprintf(b, "  %s\n", strings.TrimRight(lines[0], " \t"))
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			b.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(line, " ") {
			fmt.Fprintln(b, line)
			continue
		}
		fmt.Fprintf(b, "       %s\n", line)
	}
}

// dashed renders a flag the way it is actually written. Go's flag package
// accepts one or two dashes for any name, but a single-letter flag reads as
// -m everywhere in this project's docs, and "--m" looks like a typo.
func dashed(name string) string {
	if len(name) == 1 {
		return "-" + name
	}
	return "--" + name
}

func optionLines(fs *flag.FlagSet, manual []helpOption) []string {
	type item struct {
		name, desc, def string
	}
	var items []item
	if fs != nil {
		fs.VisitAll(func(f *flag.Flag) {
			items = append(items, item{
				name: dashed(f.Name),
				desc: f.Usage,
				def:  f.DefValue,
			})
		})
	} else {
		for _, o := range manual {
			items = append(items, item{name: dashed(o.name), desc: o.desc})
		}
	}
	if len(items) == 0 {
		return nil
	}
	width := 0
	for _, it := range items {
		if n := len(it.name); n > width {
			width = n
		}
	}
	lines := make([]string, 0, len(items))
	for _, it := range items {
		desc := it.desc
		if showDefault(it.def) {
			desc = fmt.Sprintf("%s (default %s)", desc, it.def)
		}
		lines = append(lines, fmt.Sprintf("  %-*s  %s", width, it.name, desc))
	}
	return lines
}

// showDefault omits zero-ish defaults that flag.PrintDefaults also hides.
func showDefault(def string) bool {
	switch def {
	case "", "false", "0":
		return false
	default:
		return true
	}
}

// printHelp writes formatHelp to stdout. Used by non-FlagSet commands.
func printHelp(name string) {
	fmt.Fprint(os.Stdout, formatHelp(name, nil))
}
