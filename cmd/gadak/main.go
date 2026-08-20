// Command gadak serves a local mirror of your issue tracker.
//
// The command list is the `usage` string and the `commands` map in this file;
// do not maintain a second inventory here. See specs/000-product/tasks.md for
// the current state of each command.
//
// The agent-facing commands live in agent.go; AGENTS.md is their reference.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
)

var version = "0.0.0-dev"

func openStore() (*store.DB, error) {
	if err := rejectUnknownProfile(); err != nil {
		return nil, err
	}
	path, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	return store.Open(path)
}

// profileCreateOK is the command whitelist that may mint a named profile
// directory (init writes config; serve is the onboarding path). Every other
// command errors if --profile / GADAK_PROFILE names a missing directory (D3).
var profileCreateOK = map[string]bool{
	"init":  true,
	"serve": true,
}

// profileIndependent commands never read or create a profile directory, so a
// typo in GADAK_PROFILE / --profile must not fail them (D3 regression: version).
var profileIndependent = map[string]bool{
	"version": true,
	"raycast": true,
}

// allowProfileCreate is set by main for profileCreateOK commands so
// openStore / openReadOnly (which serve still calls) can create.
var allowProfileCreate bool

func rejectUnknownProfile() error {
	if allowProfileCreate {
		return nil
	}
	return config.RequireExistingProfile()
}

// browseAddr turns a listen address into the URL a person should visit:
// a blank or wildcard host becomes localhost; loopback binds prefer
// gadak.localhost when the system resolver maps it only to loopback IPs.
func browseAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	return prettyOpenURL(host, port, nil)
}

// hostLookup is the injectable DNS path for prettyOpenURL (tests pass a stub).
type hostLookup func(ctx context.Context, host string) ([]string, error)

// prettyOpenURL chooses the host shown in the terminal and opened in a browser.
// Bind address is unchanged — only the display/open URL may become gadak.localhost.
//
// Rules:
//   - non-loopback bind (LAN / --allow-remote) → keep that host
//   - loopback bind (127.0.0.1, ::1, localhost, empty) and lookup("gadak.localhost")
//     returns only loopback IPs → http://gadak.localhost:<port>
//   - lookup timeout, failure, empty, or any non-loopback result → fallback host
//
// lookup nil uses the default resolver with a 500ms budget so a slow DNS path
// never stalls serve startup.
func prettyOpenURL(bindHost, port string, lookup hostLookup) string {
	fallback := bindHost
	if fallback == "" || fallback == "0.0.0.0" || fallback == "::" {
		fallback = "localhost"
	}
	fallbackURL := "http://" + net.JoinHostPort(fallback, port)

	// Only rewrite when the process is listening on loopback (or empty host,
	// which net treats as all-interfaces but is still the local machine).
	// 0.0.0.0 / :: require --allow-remote and stay on the fallback form.
	if bindHost != "" && !isLoopback(bindHost) {
		return fallbackURL
	}

	if lookup == nil {
		lookup = lookupHostDefault
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	addrs, err := lookup(ctx, "gadak.localhost")
	if err != nil || len(addrs) == 0 {
		return fallbackURL
	}
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil || !ip.IsLoopback() {
			return fallbackURL
		}
	}
	return "http://" + net.JoinHostPort("gadak.localhost", port)
}

func lookupHostDefault(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

func cmdVersion(args []string) error {
	if wantsHelp(args) {
		printHelp("version")
		return nil
	}
	fmt.Println(version)
	return nil
}

const usage = `gadak — a local-first mirror of your issue tracker

Usage:
  gadak [--profile <name>] <command>

Commands:
  init             ` + initSummary + `
                   [--standalone] [--site] [--email] [--projects] [--spaces] [--token-file|--token-stdin] [--json]
  config           get or set profile settings     [list|get <path>|set <path> <value>] [--json]
  sync             mirror the workspace origin into SQLite   [--full] [--watch] [--source jira|linear|confluence|all]
  serve            web UI + API on loopback  [--addr] [--static] [--no-sync] [--no-open] [--allow-remote]
                   (` + serveSyncDefault + `; --no-sync opts out)
  install-service  keep serve running across reboots (launchd / systemd user)
                   [--uninstall]
  install-cli      put this binary on PATH  [--dir] [--force] [--print]
  status           sync state and row counts [--json]
  doctor           redacted diagnostics safe to paste into a bug report [--json]
  demo             serve the bundled snapshot, no Jira account needed
  export-static    freeze demo.db into static JSON for hosted demo  <outdir>
  profiles         list mirrors and which one this command used  [--json]
  workspace        show the active workspace and what selected it  [--json]
  workspaces       list workspaces (alias of profiles)  [--json]
  version          print version

Reading the mirror (no network; see AGENTS.md):
  issue      full detail for one issue    <KEY> [--json]
  open       open the issue on your Jira site in the browser  <KEY>
  search     full-text or JQL            [--jql] [--emit] [--limit N] [--json] "text|JQL|URL"
  views      list/open Jira filters      [list|show|open|save]  (alias: view)
  sql        read-only SQL               [--json|--csv] [--no-header] "select ..."
  snapshot   shareable copy of the mirror <out.db> [--from db] [--spread 90d] [--scale N]
  export     dump saved views, watches, favorites as JSON  [--out FILE]
  import     restore them from a gadak export file         <FILE>
  mcp        MCP server on stdio; mcp install <client> pins profile (docs/MCP.md)
  skill      install Claude Code skill (schema + queries; no MCP process)
  raycast    install the Raycast search extension

Atlassian REST escape hatch (needs a credential; not on MCP):
  api        raw REST call    [METHOD] <PATH> [--query k=v] [--data …] [--write] [--status]

Writing through to the workspace origin — ` + writeThroughOriginPhrase + `:
  create     create an issue  [--] <SUMMARY> [--project KEY] [--type NAME-or-id] [--priority NAME-or-id] [--label L]... [--attach FILE]... [-m <text|->] [--json]
  attach     attach files     <KEY> <file>... [--json]
  edit       edit an issue    <KEY> [--summary S] [-m <text|->] [--label +x|-x]... [--priority NAME-or-id] [--json]
  comment    add a comment    <KEY> -m <text|-> [--json]
  transition change status    <KEY> <transition-id|status-id|name> [--json]
  assign     set assignee     <KEY> <email|-> [--json]
  page       wiki page create/edit/comment  create|edit|comment [<ID>] [--space K] [--title T] [-m <text|->] [--adf-file F] [--json]
  project    grow a standalone workspace by a project  create <KEY> [--name N] [--json]
  fields     custom-field usage report  [--sample N] [--json] [--all] [--project KEY]
  team       share team settings/views  export [--out] [--with-members]
                                        import <FILE|-> [--dry-run] [--overwrite]

Pairing other machines onto this serve (standalone):
  pairing    device tokens gating the origin passthrough  mint --label NAME [--ttl 90d] [--endpoint URL] [--json] | list | revoke <label|hash-prefix>

Profiles keep separate credentials and mirrors (e.g. work and demo):
  gadak --profile demo init && gadak --profile demo serve --addr 127.0.0.1:7778
`

func main() {
	log.SetFlags(0)
	// origin.Close flushes a standalone issuetap PersistPath. os.Exit
	// below skips defers, so Close is also called on the error path. A
	// flush failure must not exit 0 alongside a success line (GDK-342).
	defer func() {
		if err := origin.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "gadak: standalone persist flush on exit: %v\n", err)
			os.Exit(1)
		}
	}()
	server.Version = version
	if base := filepath.Base(os.Args[0]); base == config.LegacyName || base == config.LegacyName+".exe" {
		fmt.Fprintf(os.Stderr, "gadak: the `%s` command was renamed to `gadak`.\n", config.LegacyName)
	}
	args := os.Args[1:]
	// Global --workspace/--profile are only accepted before the subcommand.
	rest, err := parseGlobalWorkspace(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gadak: %v\n", err)
		os.Exit(exitStatus(err))
	}
	args = rest
	if len(args) < 1 {
		fmt.Print(usage)
		os.Exit(2)
	}
	// gadak help [<cmd>] — bare help is top-level usage; with a name, same as
	// `<cmd> --help`. Aliases --help/-h alone also print top-level usage.
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		if len(args) == 1 {
			fmt.Print(usage)
			return
		}
		args = []string{args[1], "--help"}
	}
	if args[0] == "--version" || args[0] == "-v" {
		args = []string{"version"}
	}
	run, ok := commands[args[0]]
	if !ok {
		err := unknownCommandError(args[0])
		fmt.Fprintf(os.Stderr, "gadak: %v\n", err)
		os.Exit(exitStatus(err))
	}
	if err := checkProfileForCommand(args[0], args[1:]); err != nil {
		log.Fatalf("gadak: %v", err)
	}
	if err := run(args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gadak: %v\n", err)
		if cerr := origin.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "gadak: standalone persist flush on exit: %v\n", cerr)
		}
		os.Exit(exitStatus(err))
	}
}

// checkProfileForCommand is the D3 gate used by main after alias rewrite
// (help X → X --help, --version → version). Version is profile-independent.
// A trailing --help/-h is static help and must not require a profile.
// Create-ok commands may mint a directory; everything else requires the
// named profile to already exist.
func checkProfileForCommand(cmd string, rest []string) error {
	if profileIndependent[cmd] {
		return nil
	}
	if helpTail(rest) {
		return nil
	}
	if profileCreateOK[cmd] {
		allowProfileCreate = true
		return nil
	}
	return config.RequireExistingProfile()
}

// parseGlobalWorkspace consumes leading --workspace/-w/--profile/-p pairs.
// The two names are aliases for the same selection; passing more than one
// is an error (exit 64) rather than a silent winner. The leftover args
// start at the subcommand.
func parseGlobalWorkspace(args []string) ([]string, error) {
	i := 0
	var selectedFlag, selectedName string
	selected := false
	for i < len(args) {
		a := args[i]
		if a != "--workspace" && a != "-w" && a != "--profile" && a != "-p" {
			break
		}
		if i+1 >= len(args) {
			return nil, &exitCodeError{code: 64, msg: a + " requires a workspace name"}
		}
		if selected {
			return nil, &exitCodeError{
				code: 64,
				msg:  fmt.Sprintf("command names two workspaces (%s and %s); pass only one", selectedFlag, a),
			}
		}
		selected = true
		selectedFlag = a
		selectedName = args[i+1]
		i += 2
	}
	if selected {
		config.SetProfile(selectedName)
	}
	return args[i:], nil
}

// helpTail reports a static help invocation: the last arg is --help or -h
// (the shape main rewrites `gadak help status` into).
func helpTail(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[len(args)-1] {
	case "--help", "-h":
		return true
	}
	return false
}

// commands is the single registry of subcommands. Dispatch, the command-name
// list, and help coverage all derive from this map (helps stays separate prose).
// Aliases handled before lookup (help/--help/-h, --version/-v) are not entries.
var commands = map[string]func([]string) error{
	"api":             cmdAPI,
	"assign":          cmdAssign,
	"attach":          cmdAttach,
	"comment":         cmdComment,
	"config":          cmdConfig,
	"create":          cmdCreate,
	"demo":            cmdDemo,
	"doctor":          cmdDoctor,
	"edit":            cmdEdit,
	"export":          cmdExport,
	"export-static":   cmdExportStatic,
	"fields":          cmdFields,
	"import":          cmdImport,
	"init":            cmdInit,
	"install-cli":     cmdInstallCLI,
	"install-service": cmdInstallService,
	"issue":           cmdIssue,
	"mcp":             cmdMCP,
	"open":            cmdOpen,
	"page":            cmdPage,
	"pairing":         cmdPairing,
	"profiles":        cmdProfiles,
	"project":         cmdProject,
	"raycast":         cmdRaycast,
	"search":          cmdSearch,
	"serve":           cmdServe,
	"skill":           cmdSkill,
	"snapshot":        cmdSnapshot,
	"sql":             cmdSQL,
	"status":          cmdStatus,
	"sync":            cmdSync,
	"team":            cmdTeam,
	"transition":      cmdTransition,
	"version":         cmdVersion,
	"view":            cmdViews,
	"views":           cmdViews,
	"workspace":       cmdWorkspace,
	"workspaces":      cmdProfiles,
}

// commandNames returns sorted keys of commands. helps has one entry per name;
// tests assert the two sets stay equal so a new command without help (or a
// leftover help for a removed command) fails CI, and that every name dispatches.
func commandNames() []string {
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
