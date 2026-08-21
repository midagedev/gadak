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
// init writes site/email/token (cmdInit); --standalone writes
// KindStandalone with no site or credential (initStandalone, flag usage
// in cmdInit); --pairing-code binds a fresh workspace to a remote gadak
// serve (initPaired, GDK-433).
const initSummary = "configure a Jira site and credential (projects optional), a standalone workspace (--standalone), or pair to a remote gadak serve (--pairing-code)"

// serveSyncDefault is the serve sync-on-start condition, matching
// startServeLoops: cfg.HasCredential() is true for a standalone workspace
// or a connected workspace with site+email+token (internal/config.HasCredential).
const serveSyncDefault = "syncs by default on a standalone workspace, or on a connected workspace with a credential"

// writeThroughOriginPhrase is the single owner of "where CLI writes go"
// (GDK-469). Verified: mutate in agent.go calls origin.Writer; origin.Client
// refuses a connected workspace without site/email/token (errNeedCredential)
// and admits a standalone workspace with no token. Verb --help first lines
// name the verb; this sentence lives once in top-level usage.
const writeThroughOriginPhrase = "Jira on a connected workspace (needs a credential), the embedded origin on a standalone one; the mirror refreshes after the origin accepts"

// helps is the per-command help table. Summaries recycle the top-level usage
// constant; positionals and examples match the real cmdXxx implementations.
var helps = map[string]cmdHelp{
	"init": {
		summary: initSummary,
		usage:   "gadak [--workspace <name>] init [--standalone] [--site URL] [--email ADDR] [--projects A,B] [--spaces KEYS|all|none] [--token-file PATH | --token-stdin] [--token-expires DATE] [--pairing-code OFFER | --pairing-code-stdin] [--json]",
		// FlagSet VisitAll supplies Options when `gadak init --help` runs; this
		// list covers formatHelp(nil) and documents the env-only token path.
		options: []helpOption{
			{name: "standalone", desc: "create an independent workspace (no Jira site or credential)"},
			{name: "replace-standalone", desc: replaceStandaloneUsage},
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
			"gadak init --standalone",
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
		summary: "manage the device tokens that gate a standalone serve's origin passthrough; a paired remote machine binds with `gadak init --pairing-code`",
		usage:   "gadak [--workspace <name>] pairing mint --label NAME [--ttl 90d] [--endpoint URL] [--json] | pairing list | pairing revoke <label|hash-prefix>",
		options: []helpOption{
			{name: "label", desc: "device name shown in `gadak pairing list` (required, unique among active tokens)"},
			{name: "ttl", desc: "token lifetime: <N><d|h|m|s>, e.g. 90d (default) or 12h"},
			{name: "endpoint", desc: "URL remote devices reach this serve at; default is this machine's live serve address (loopback draws a warning — pass your tailnet URL)"},
			{name: "json", desc: "emit JSON"},
		},
		examples: []string{
			"gadak pairing mint --label laptop",
			"gadak pairing mint --label agent --ttl 12h --endpoint https://<machine>.<tailnet>.ts.net",
			"gadak pairing mint --label laptop --json",
			"gadak pairing list",
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
		usage:   "gadak [--workspace <name>] sync [--full] [--watch] [--source jira|linear|confluence|all]",
		examples: []string{
			"gadak sync                 # incremental, what a serve loop does",
			"gadak sync --full          # after changing projects or a mapping",
			"gadak --workspace demo sync  # against another site",
		},
		seeAlso: []string{"gadak serve", "gadak status"},
	},
	"serve": {
		summary: "web UI and API on loopback (" + serveSyncDefault + ")",
		usage:   "gadak [--workspace <name>] serve [options]",
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
		summary: "put this binary on PATH (prefers a PATH entry, else ~/.local/bin)",
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
		summary: "development-panel links: record PRs on issues (standalone origin)",
		usage:   "gadak dev link <KEY> --pr <url> [--status open|merged|declined] [--name N] [--json]\n       gadak dev scan [--dry-run] [--install-hook]",
		options: []helpOption{
			{name: "pr", desc: "pull request URL (required for link)"},
			{name: "status", desc: "open (default), merged, or declined"},
			{name: "name", desc: "display title shown in the panel"},
			{name: "dry-run", desc: "scan: list matched PRs without writing"},
			{name: "install-hook", desc: "scan: add a pre-push hook that runs `gadak dev scan`"},
		},
		examples: []string{
			"gadak dev link STD-3 --pr https://github.com/org/app/pull/7 --status merged",
			"gadak dev scan            # match issue keys in `gh pr list` titles/branches, link them all",
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
		summary: "list workspaces: which mirrors exist, which one this command used (same as workspaces)",
		usage:   "gadak profiles [--json]",
		examples: []string{
			"gadak profiles",
			"gadak profiles --json",
		},
		seeAlso: []string{"gadak init", "gadak workspace", "gadak workspaces"},
	},
	"workspace": {
		summary: "show the active workspace, what selected it, origin kind, persist path, and other workspaces",
		usage:   "gadak workspace [--json]",
		examples: []string{
			"gadak workspace",
			"gadak --workspace oss workspace",
			"GADAK_WORKSPACE=oss gadak workspace",
			"gadak workspace --json",
		},
		seeAlso: []string{"gadak workspaces", "gadak profiles", "gadak status"},
	},
	"workspaces": {
		summary: "list workspaces: which mirrors exist, which one this command used (same as profiles)",
		usage:   "gadak workspaces [--json]",
		examples: []string{
			"gadak workspaces",
			"gadak workspaces --json",
		},
		seeAlso: []string{"gadak workspace", "gadak profiles", "gadak init"},
	},
	"sql": {
		summary: "run a read-only SQL query against the local mirror",
		usage:   "gadak [--workspace <name>] sql [--json|--csv] [--no-header] \"select ...\"",
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
		seeAlso: []string{"gadak issue", "gadak search", "gadak status"},
	},
	"api": {
		summary: "call Atlassian REST with the stored credential (escape hatch for endpoints the mirror does not cover)",
		usage:   "gadak [--workspace <name>] api [METHOD] <PATH> [--query k=v]... [--data <val|@file|->] [--write] [--status]",
		examples: []string{
			"gadak api /rest/api/3/myself",
			"gadak api GET /rest/api/3/issue/ABC-1/watchers",
			"gadak api GET /wiki/api/v2/spaces --query limit=5",
			"gadak api POST /rest/api/3/issue/ABC-1/worklog --data @wl.json --write",
		},
		seeAlso: []string{"gadak issue", "gadak comment", "gadak fields"},
	},
	"issue": {
		summary: "print full detail for one issue from the local mirror; --editmeta asks the origin which configured fields this issue can edit",
		usage:   "gadak [--workspace <name>] issue <KEY> [--json] [--derive] [--link] [--editmeta]",
		examples: []string{
			"gadak issue NMB-140",
			"gadak issue NMB-140 --json",
			"gadak issue NMB-140 --link",
			"gadak issue NMB-140 --editmeta",
		},
		seeAlso: []string{"gadak search", "gadak open", "gadak sql", "gadak views open"},
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
		usage:   "gadak [--workspace <name>] views [list|show <name>|open <name|KEY>|save <name> --jql '…'] [--keys …] [--json] [--no-open]",
		examples: []string{
			"gadak views",
			"gadak views show \"NMA in progress\"",
			"gadak views open \"NMA in progress\"",
			"gadak views open NMB-140",
			"gadak views open --jql 'project = NMA AND statusCategory = \"In Progress\"'",
			"gadak views open --keys 'NMA-1,NMA-2'",
			"gadak views open --keys -",
			"gadak views open --jql 'project = NMA' --no-open",
			"gadak views save \"Night triage\" --jql 'assignee = currentUser() AND resolution is EMPTY'",
		},
		seeAlso: []string{"gadak search", "gadak sync"},
	},
	"search": {
		summary: "full-text search, or a JQL / Jira-URL filter against the mirror",
		usage:   "gadak [--workspace <name>] search [--jql] [--emit] [--limit N] [--json] [--explain] \"text|JQL|URL\"",
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
	"comment": {
		summary: "add a comment (@Name resolves to a site user; ambiguous names are refused)",
		usage:   "gadak [--workspace <name>] comment <KEY> [<text> | -m <text|->] [--visibility role=NAME|group=NAME] [--internal] [--json]",
		examples: []string{
			"gadak comment NMB-140 Reproduced on staging.",
			"gadak comment NMB-140 -m \"Reproduced on staging.\"",
			"gadak comment NMB-140 -m \"thanks @Dana\"",
			"gadak comment NMB-140 -m -          # body from stdin",
			"gadak comment NMB-140 -m \"done\" --json",
		},
		seeAlso: []string{"gadak transition", "gadak assign", "gadak issue"},
	},
	"create": {
		summary: "create an issue",
		usage:   "gadak [--workspace <name>] create [--] <SUMMARY> | --batch - [--project KEY] [--type NAME-or-id] [--priority NAME-or-id] [--due YYYY-MM-DD] [--parent KEY] [--label L]... [--attach FILE]... [-m <text|->] [--json]",
		examples: []string{
			"gadak create Fix the flaky gate --project NMB --type Task -m \"repro on staging\" --label batch",
			"gadak create 로그인 실패 --project NMB --type 작업",
			"gadak create Night triage item --project NMB --type Task --priority High --due 2026-09-01",
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
		summary: "edit summary, description, labels, components, priority, parent, or due date",
		usage:   "gadak [--workspace <name>] edit <KEY> [--summary S] [-m <text|->] [--label +x|-x]... [--component +x|-x]... [--priority NAME-or-id] [--due YYYY-MM-DD|none] [--parent KEY|none] [--json]",
		examples: []string{
			"gadak edit NMB-140 --summary \"Rename without opening Jira\"",
			"gadak edit NMB-140 --label +batch --label -legacy --priority High",
			"gadak edit NMB-140 --component +SDK --component -Docs",
			"gadak edit NMB-140 --due 2026-09-01",
			"gadak edit NMB-140 --due none",
		},
		seeAlso: []string{"gadak create", "gadak attach", "gadak comment", "gadak issue"},
	},
	"page": {
		summary: "wiki page writes through the origin (page create, edit, comment; connected Confluence or standalone issuetap)",
		usage:   "gadak [--workspace <name>] page create|edit|comment [<ID>] [--space K] [--title T] [-m <text|->] [--adf-file F] [--parent ID] [--version N] [--json]",
		examples: []string{
			"gadak page create --space ENG --title \"Retention notes\" -m \"first draft\"",
			"gadak page edit 12345 --title \"Renamed page\"",
			"gadak page edit 12345 -m \"whole new body (plain text; replaces formatting)\"",
			"gadak page edit 12345 --adf-file body.adf.json",
			"gadak page comment 12345 -m \"question on the retention section\"",
		},
		seeAlso: []string{"gadak search", "gadak open"},
	},
	"project": {
		summary: "grow a standalone workspace by one project key (connected workspaces create projects in Jira)",
		usage:   "gadak [--workspace <name>] project create <KEY> [--name N] [--json]",
		examples: []string{
			"gadak project create IDEA --name Ideas",
			"gadak create --project IDEA \"first idea\"",
		},
		seeAlso: []string{"gadak create", "gadak init"},
	},
	"transition": {
		summary: "change issue status; accepts transition id, target status id, name, target status name, or status category new|inprogress|done",
		usage:   "gadak [--workspace <name>] transition <KEY> <transition-id|status-id|name|new|inprogress|done> [--resolution name|id] [--field key=JSON]... [-m text] [--json]",
		examples: []string{
			"gadak transition NMB-140 \"In Review\"",
			"gadak transition NMB-140 31",
			"gadak transition NMB-140 done",
			"gadak transition NMB-140 done --resolution \"Won't Do\"",
			"gadak transition NMB-140 done -m \"fixed in 1.2\"",
		},
		seeAlso: []string{"gadak comment", "gadak assign", "gadak issue"},
	},
	"assign": {
		summary: "set the assignee; pass - to unassign",
		usage:   "gadak [--workspace <name>] assign <KEY> <email|-> [--json]",
		examples: []string{
			"gadak assign NMB-140 dana@example.com",
			"gadak assign NMB-140 -                 # unassign",
			"gadak assign NMB-140 dana@example.com --json",
		},
		seeAlso: []string{"gadak comment", "gadak transition", "gadak issue"},
	},
	"fields": {
		summary: "report which custom fields are populated (samples the mirror; queries Jira)",
		usage:   "gadak [--workspace <name>] fields [--sample N] [--json] [--all] [--project KEY] [--apply]",
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
		usage:   "gadak [--workspace <name>] mcp [install <client>]",
		examples: []string{
			"gadak mcp",
			"gadak --workspace demo mcp",
			"gadak mcp install claude",
			"gadak --workspace demo mcp install claude --dry-run",
			"gadak mcp install json",
		},
		seeAlso: []string{"gadak skill install", "gadak sql", "gadak issue", "gadak status", "gadak profiles"},
	},
	"skill": {
		summary: "install the Claude Code skill (schema + query patterns; no MCP process)",
		usage:   "gadak skill install [client] [--project] [--dir <path>] [--print] [--force]",
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
// starts with `-` can be passed. New commands inherit this by using
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
			pos = append(pos, args[i+1:]...)
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

// exitCoder is a command error that names its process status (e.g. unknown
// config path → 64). unknownFlagErr stays a separate 2 so a mistyped flag
// does not change class.
type exitCoder interface {
	ExitCode() int
}

// exitStatus is the process code main uses for a command error.
// unknown flags are usage (2); an exitCoder keeps the code it named;
// everything else stays 1.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var u *unknownFlagErr
	if errors.As(err, &u) {
		return 2
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
// 64 is EX_USAGE, the same class unknownConfigPath uses.
func unknownCommandError(name string) error {
	return &exitCodeError{
		code: 64,
		msg:  fmt.Sprintf("unknown command %q — see gadak --help", name),
	}
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
	fmt.Fprintf(&b, "Usage:\n  %s\n", h.usage)

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
			fmt.Fprintf(&b, "  %s\n", ex)
		}
	}

	if len(h.seeAlso) > 0 {
		fmt.Fprintf(&b, "\nSee also: %s\n", strings.Join(h.seeAlso, ", "))
	}
	return b.String()
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
