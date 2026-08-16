package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// profileInventory is the JSON document for `gadak profiles --json`.
type profileInventory struct {
	Active   string         `json:"active"`
	Profiles []profileEntry `json:"profiles"`
}

// profileEntry is one row of the inventory. Secrets never appear here: site is
// host-only, and email/token are omitted entirely.
type profileEntry struct {
	Name       string  `json:"name"`
	Active     bool    `json:"active"`
	Configured bool    `json:"configured"`
	SiteHost   string  `json:"site_host"`
	Issues     int     `json:"issues"`
	Documents  int     `json:"documents"`
	LastSyncAt *string `json:"last_sync_at"`
	DBPath     string  `json:"db_path"`
	Error      string  `json:"error,omitempty"`
	// hasMirror is text-output only: false means ISSUES/DOCS print as —.
	hasMirror bool
}

func cmdProfiles(args []string) error {
	fs := newFlagSet("profiles")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	inv, err := collectProfiles()
	if err != nil {
		return err
	}
	if *asJSON {
		// Drop internal-only fields from the public document by re-encoding
		// through the same types (hasMirror is unexported and omitted).
		return json.NewEncoder(os.Stdout).Encode(inv)
	}
	printProfilesText(inv)
	return nil
}

func collectProfiles() (profileInventory, error) {
	activeName := displayProfileName(config.Profile())
	inv := profileInventory{Active: activeName}

	// Always include the default profile first.
	inv.Profiles = append(inv.Profiles, inspectProfile("", activeName))

	names, err := config.Profiles()
	if err != nil {
		return inv, err
	}
	for _, n := range names {
		inv.Profiles = append(inv.Profiles, inspectProfile(n, activeName))
	}
	return inv, nil
}

// inspectProfile builds one inventory row. name is the config profile key
// ("" for default). A broken config is reported as error:"unreadable" without
// failing the whole command.
func inspectProfile(name, activeDisplay string) profileEntry {
	display := displayProfileName(name)
	e := profileEntry{
		Name:   display,
		Active: display == activeDisplay,
	}

	dbPath, err := config.DBPathFor(name)
	if err == nil {
		e.DBPath = dbPath
	}

	cfg, err := config.LoadFor(name)
	if err != nil {
		e.Error = "unreadable"
		return e
	}
	if cfg.HasCredential() {
		e.Configured = true
		e.SiteHost = siteHostOnly(cfg.Site)
	}

	if dbPath == "" {
		return e
	}
	info, err := os.Stat(dbPath)
	if err != nil || info.IsDir() {
		// No mirror yet — do not create one (store.Open would).
		return e
	}
	e.hasMirror = true

	db, err := store.Open(dbPath)
	if err != nil {
		// File exists but will not open; leave counts at zero for JSON and
		// treat as no usable mirror for text (hasMirror false).
		e.hasMirror = false
		return e
	}
	defer db.Close()

	if n, err := db.TableCount(context.Background(), "issues"); err == nil {
		e.Issues = n
	}
	if n, err := db.TableCount(context.Background(), "pages"); err == nil {
		e.Documents = n
	}
	// Same source status uses: jira sync_state (SyncedAt = last good run).
	if ss, err := db.SyncState(context.Background(), "jira"); err == nil {
		if ss.SyncedAt != nil && *ss.SyncedAt != "" {
			at := *ss.SyncedAt
			e.LastSyncAt = &at
		}
	}
	return e
}

func displayProfileName(name string) string {
	if name == "" || name == "default" {
		return "default"
	}
	return name
}

// siteHostOnly returns the hostname of a site URL, never the full URL, path,
// userinfo, or port. Empty or unparseable input yields "".
func siteHostOnly(site string) string {
	site = strings.TrimSpace(site)
	if site == "" {
		return ""
	}
	u, err := url.Parse(site)
	if err != nil {
		return ""
	}
	if u.Host == "" {
		// Bare host without scheme: treat the whole string as host/path.
		u, err = url.Parse("https://" + site)
		if err != nil {
			return ""
		}
	}
	return u.Hostname()
}

func printProfilesText(inv profileInventory) {
	const emDash = "\u2014" // —

	// Measure columns for alignment.
	nameW, issuesW, docsW, syncW := len("NAME"), len("ISSUES"), len("DOCS"), len("LAST SYNC")
	rows := make([][5]string, 0, len(inv.Profiles))
	for _, p := range inv.Profiles {
		if p.Error != "" {
			// The word goes in the SITE column, not the ISSUES one: a row that
			// breaks the columns reads as a rendering bug rather than as a
			// profile whose config could not be read.
			row := [5]string{p.Name, emDash, emDash, emDash, "unreadable config"}
			if len(row[0]) > nameW {
				nameW = len(row[0])
			}
			rows = append(rows, row)
			continue
		}
		var issues, docs string
		if !p.hasMirror {
			issues, docs = emDash, emDash
		} else {
			issues = formatIntComma(p.Issues)
			docs = formatIntComma(p.Documents)
		}
		sync := "never"
		if p.LastSyncAt != nil {
			sync = relativeAge(*p.LastSyncAt, time.Now())
		}
		site := emDash
		if p.SiteHost != "" {
			site = p.SiteHost
		}
		row := [5]string{p.Name, issues, docs, sync, site}
		if len(row[0]) > nameW {
			nameW = len(row[0])
		}
		if len(row[1]) > issuesW {
			issuesW = len(row[1])
		}
		if len(row[2]) > docsW {
			docsW = len(row[2])
		}
		if len(row[3]) > syncW {
			syncW = len(row[3])
		}
		rows = append(rows, row)
	}

	// Header: two spaces before NAME (marker column empty).
	fmt.Printf("  %-*s  %*s  %*s  %-*s  %s\n",
		nameW, "NAME", issuesW, "ISSUES", docsW, "DOCS", syncW, "LAST SYNC", "SITE")
	for i, p := range inv.Profiles {
		row := rows[i]
		marker := " "
		if p.Active {
			marker = "*"
		}
		fmt.Printf("%s %-*s  %*s  %*s  %-*s  %s\n",
			marker, nameW, row[0], issuesW, row[1], docsW, row[2], syncW, row[3], row[4])
	}
	fmt.Println()
	// The example names a profile that is actually in the list above — a hint
	// built around a profile you do not have reads as stale documentation, and
	// this hint exists because remembering the flag is the real friction.
	// A named mirror is preferred over "default": switching to the implicit
	// profile is the one move nobody needs an example for.
	example := "<name>"
	for _, p := range inv.Profiles {
		if p.Active || p.Name == "default" {
			continue
		}
		example = p.Name
		break
	}
	if example == "<name>" {
		for _, p := range inv.Profiles {
			if !p.Active {
				example = p.Name
				break
			}
		}
	}
	fmt.Println("* = the profile this command ran against. Target another one per command with")
	fmt.Printf("  `gadak --profile %s <cmd>`, or for this shell with `export GADAK_PROFILE=%s`.\n",
		example, example)
}

// formatIntComma renders n with thousands separators (e.g. 6832 → "6,832").
func formatIntComma(n int) string {
	if n < 0 {
		return "-" + formatIntComma(-n)
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	// First group may be shorter than 3.
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	b.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// relativeAge formats how long ago iso was: "just now", "4m ago", "2h ago",
// "3d ago", or "never" when empty/unparseable.
func relativeAge(iso string, now time.Time) string {
	if iso == "" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		for _, layout := range []string{
			config.ISOMilli,
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02T15:04:05.000-0700",
		} {
			if t, err = time.Parse(layout, iso); err == nil {
				break
			}
		}
		if err != nil {
			return "never"
		}
	}
	d := now.Sub(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	}
}
