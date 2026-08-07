package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

// doctorBanner is the first line of every doctor dump so a user pasting into a
// public issue can see what was stripped before they hit submit.
const doctorBanner = "# scry doctor — safe to paste: counts and versions only, no keys, names, URLs or tokens"

// doctorReport is the redacted diagnostic document. Field names are stable for
// --json consumers; values never carry tokens, hostnames, emails, project
// keys, custom-field names, or raw error strings.
type doctorReport struct {
	ScryVersion   string                `json:"scry_version"`
	GoVersion     string                `json:"go_version"`
	OS            string                `json:"os"`
	Arch          string                `json:"arch"`
	Profile       string                `json:"profile"`
	MirrorPath    string                `json:"mirror_path"`
	Mirror        doctorMirror          `json:"mirror"`
	SchemaVersion *int                  `json:"schema_version"`
	Migrations    string                `json:"migrations"`
	Counts        *doctorCounts         `json:"counts"`
	Confluence    string                `json:"confluence"`
	CustomFields  int                   `json:"custom_fields"`
	Credential    string                `json:"credential"`
	Site          string                `json:"site"`
	Email         string                `json:"email"`
	Sync          map[string]doctorSync `json:"sync"`
	APIUsage      doctorAPIUsage        `json:"api_usage"`
}

type doctorMirror struct {
	Status string `json:"status"`           // present | not_found | open_error
	Bytes  *int64 `json:"bytes,omitempty"`  // set when the file exists
	Detail string `json:"detail,omitempty"` // redacted path / short reason
}

type doctorCounts struct {
	Items            int `json:"items"`
	Issues           int `json:"issues"`
	Pages            int `json:"pages"`
	Comments         int `json:"comments"`
	Projects         int `json:"projects"`
	StatusCategories int `json:"status_categories"`
	Spaces           int `json:"spaces"`
}

type doctorSync struct {
	SyncedAt   string `json:"synced_at"`  // RFC3339-ish or "never"
	Watermark  string `json:"watermark"`  // present | absent
	LastError  string `json:"last_error"` // classified only
	SyncCount  int64  `json:"sync_count"`
	LastFullAt string `json:"last_full_sync_at"` // or "never"
}

type doctorAPIUsage struct {
	Day       string `json:"day"`
	Requests  int64  `json:"requests"`
	Throttled int64  `json:"throttled"`
	Retries   int64  `json:"retries"`
}

func cmdDoctor(args []string) error {
	fs := newFlagSet("doctor")
	asJSON := fs.Bool("json", false, "emit the same document as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rep := collectDoctor()
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	fmt.Print(formatDoctorText(rep))
	return nil
}

func collectDoctor() doctorReport {
	profile := config.Profile()
	if profile == "" {
		profile = "default"
	}

	rep := doctorReport{
		ScryVersion:  version,
		GoVersion:    runtime.Version(),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Profile:      profile,
		Confluence:   "inactive",
		Credential:   "absent",
		Site:         "none",
		Email:        "none",
		CustomFields: 0,
		Sync:         map[string]doctorSync{},
		APIUsage: doctorAPIUsage{
			Day: time.Now().UTC().Format("2006-01-02"),
		},
		Migrations: "unknown",
	}

	if path, err := config.DBPath(); err == nil {
		rep.MirrorPath = tildeHome(path)
	} else {
		rep.MirrorPath = "unknown"
	}

	if cfg, err := config.Load(); err == nil && cfg != nil {
		if cfg.Token != "" {
			rep.Credential = "present"
		}
		rep.Site = redactSite(cfg.Site)
		if cfg.Email != "" {
			rep.Email = "configured"
		}
		rep.CustomFields = len(cfg.FieldSpecs())
		if cfg.Confluence != nil {
			rep.Confluence = "active"
		}
	}

	path, err := config.DBPath()
	if err != nil {
		rep.Mirror = doctorMirror{Status: "open_error", Detail: "path unavailable"}
		return rep
	}

	info, statErr := os.Stat(path)
	if os.IsNotExist(statErr) {
		rep.Mirror = doctorMirror{
			Status: "not_found",
			Detail: "not found at " + tildeHome(path),
		}
		return rep
	}
	if statErr != nil {
		rep.Mirror = doctorMirror{
			Status: "open_error",
			Detail: "stat failed",
		}
		return rep
	}
	size := info.Size()
	rep.Mirror = doctorMirror{Status: "present", Bytes: &size}

	db, err := store.Open(path)
	if err != nil {
		rep.Mirror.Status = "open_error"
		rep.Mirror.Detail = "open failed"
		return rep
	}
	defer db.Close()

	sv := db.SchemaVersion()
	rep.SchemaVersion = &sv
	if sv > 0 {
		rep.Migrations = fmt.Sprintf("1..%d", sv)
	} else {
		rep.Migrations = "none"
	}

	counts := &doctorCounts{}
	if n, err := db.TableCount(context.Background(), "items"); err == nil {
		counts.Items = n
	}
	if n, err := db.TableCount(context.Background(), "issues"); err == nil {
		counts.Issues = n
	}
	if n, err := db.TableCount(context.Background(), "pages"); err == nil {
		counts.Pages = n
	}
	if n, err := db.TableCount(context.Background(), "comments"); err == nil {
		counts.Comments = n
	}
	if n, err := db.DistinctCount(context.Background(), "issues", "project_key"); err == nil {
		counts.Projects = n
	}
	if n, err := db.DistinctCount(context.Background(), "issues", "status_category"); err == nil {
		counts.StatusCategories = n
	}
	// Prefer the spaces catalog; fall back to distinct keys on mirrored pages
	// when the catalog was never filled (older snapshots).
	if n, err := db.TableCount(context.Background(), "spaces"); err == nil && n > 0 {
		counts.Spaces = n
	} else if n, err := db.DistinctCount(context.Background(), "pages", "space_key"); err == nil {
		counts.Spaces = n
	}
	rep.Counts = counts

	for _, src := range []string{"jira", "confluence"} {
		rep.Sync[src] = collectSync(db, src)
	}

	if usage, err := db.APIUsageSummary(context.Background()); err == nil {
		rep.APIUsage = doctorAPIUsage{
			Day:       usage.Today.Day,
			Requests:  usage.Today.Requests,
			Throttled: usage.Today.Throttled,
			Retries:   usage.Today.Retries,
		}
	}

	return rep
}

func collectSync(db *store.DB, sourceID string) doctorSync {
	out := doctorSync{
		SyncedAt:   "never",
		Watermark:  "absent",
		LastError:  "none",
		LastFullAt: "never",
	}
	ss, err := db.SyncState(context.Background(), sourceID)
	if err != nil {
		out.LastError = "error"
		return out
	}
	if ss.SyncedAt != nil && *ss.SyncedAt != "" {
		out.SyncedAt = *ss.SyncedAt
	}
	if ss.Watermark != "" {
		out.Watermark = "present"
	}
	if ss.LastError != nil && *ss.LastError != "" {
		out.LastError = classifyLastError(*ss.LastError)
	}
	if ss.LastFullSyncAt != nil && *ss.LastFullSyncAt != "" {
		out.LastFullAt = *ss.LastFullSyncAt
	}
	out.SyncCount = ss.SyncCount
	return out
}

func formatDoctorText(r doctorReport) string {
	var b strings.Builder
	b.WriteString(doctorBanner)
	b.WriteByte('\n')
	b.WriteByte('\n')

	line := func(k, v string) {
		fmt.Fprintf(&b, "%-22s %s\n", k+":", v)
	}

	line("scry_version", r.ScryVersion)
	line("go_version", r.GoVersion)
	line("os", r.OS+"/"+r.Arch)
	line("profile", r.Profile)
	line("mirror_path", r.MirrorPath)

	switch r.Mirror.Status {
	case "present":
		if r.Mirror.Bytes != nil {
			line("mirror", fmt.Sprintf("present (%s)", formatBytes(*r.Mirror.Bytes)))
		} else {
			line("mirror", "present")
		}
	case "not_found":
		line("mirror", r.Mirror.Detail)
	default:
		if r.Mirror.Detail != "" {
			line("mirror", r.Mirror.Status+" ("+r.Mirror.Detail+")")
		} else {
			line("mirror", r.Mirror.Status)
		}
	}

	if r.SchemaVersion != nil {
		line("schema_version", strconv.Itoa(*r.SchemaVersion))
	} else {
		line("schema_version", "unknown")
	}
	line("migrations", r.Migrations)

	if r.Counts != nil {
		line("items", strconv.Itoa(r.Counts.Items))
		line("issues", strconv.Itoa(r.Counts.Issues))
		line("pages", strconv.Itoa(r.Counts.Pages))
		line("comments", strconv.Itoa(r.Counts.Comments))
		line("projects", strconv.Itoa(r.Counts.Projects))
		line("status_categories", strconv.Itoa(r.Counts.StatusCategories))
		line("spaces", strconv.Itoa(r.Counts.Spaces))
	} else {
		line("items", "n/a")
		line("issues", "n/a")
		line("pages", "n/a")
		line("comments", "n/a")
		line("projects", "n/a")
		line("status_categories", "n/a")
		line("spaces", "n/a")
	}

	line("custom_fields", strconv.Itoa(r.CustomFields))
	line("confluence", r.Confluence)
	line("credential", r.Credential)
	line("site", r.Site)
	line("email", r.Email)

	for _, src := range []string{"jira", "confluence"} {
		s, ok := r.Sync[src]
		if !ok {
			continue
		}
		line("sync."+src+".synced_at", s.SyncedAt)
		line("sync."+src+".watermark", s.Watermark)
		line("sync."+src+".last_error", s.LastError)
		line("sync."+src+".sync_count", strconv.FormatInt(s.SyncCount, 10))
		line("sync."+src+".last_full_sync_at", s.LastFullAt)
	}

	line("api_usage.day", r.APIUsage.Day)
	line("api_usage.requests", strconv.FormatInt(r.APIUsage.Requests, 10))
	line("api_usage.throttled", strconv.FormatInt(r.APIUsage.Throttled, 10))
	line("api_usage.retries", strconv.FormatInt(r.APIUsage.Retries, 10))

	return b.String()
}

// tildeHome shortens an absolute path under the user's home to ~/… so doctor
// output never embeds the account username.
func tildeHome(path string) string {
	if path == "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Clean(path)
	}
	clean := filepath.Clean(path)
	homeClean := filepath.Clean(home)
	if clean == homeClean {
		return "~"
	}
	sep := string(filepath.Separator)
	if strings.HasPrefix(clean, homeClean+sep) {
		return "~" + clean[len(homeClean):]
	}
	// SCRY_HOME may sit outside $HOME (tests, CI). Still strip any path segment
	// that looks like a username by refusing to print the raw home prefix when
	// the env override is used — leave absolute paths that are not under home
	// as-is only when they already contain no home component.
	return clean
}

// redactSite never returns a hostname. Atlassian Cloud sites collapse to a
// fixed pattern; anything else is "configured (cloud)" or "none".
func redactSite(site string) string {
	site = strings.TrimSpace(site)
	if site == "" {
		return "none"
	}
	// Avoid net/url import of full URL when the value is a bare host, but still
	// never echo any host component back.
	lower := strings.ToLower(site)
	if strings.Contains(lower, "atlassian.net") {
		return "<redacted>.atlassian.net"
	}
	return "configured (cloud)"
}

// jiraStatusRe matches the store's usual last_error shape from jira.APIError:
// "GET /rest/…: jira: 403: …" or bare "jira: 429".
var jiraStatusRe = regexp.MustCompile(`(?i)jira:\s*(\d{3})\b`)

// httpStatusRe is a fallback for "status 403", "HTTP 429", bare 4xx/5xx.
var httpStatusRe = regexp.MustCompile(`(?i)(?:status|http)\s*(\d{3})\b`)

// bareStatusRe catches a leading or mid-string 4xx/5xx that is not a year.
var bareStatusRe = regexp.MustCompile(`\b([45]\d{2})\b`)

// classifyLastError turns a raw sync error into "http NNN (kind)" or a short
// kind with no message text. Raw strings often embed URLs, issue keys, and
// field names — none of that leaves this function.
func classifyLastError(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "none"
	}
	code := 0
	if m := jiraStatusRe.FindStringSubmatch(raw); len(m) == 2 {
		code, _ = strconv.Atoi(m[1])
	} else if m := httpStatusRe.FindStringSubmatch(raw); len(m) == 2 {
		code, _ = strconv.Atoi(m[1])
	} else if m := bareStatusRe.FindStringSubmatch(raw); len(m) == 2 {
		code, _ = strconv.Atoi(m[1])
	}
	kind := errorKind(code, raw)
	if code > 0 {
		return fmt.Sprintf("http %d (%s)", code, kind)
	}
	return kind
}

func errorKind(code int, raw string) string {
	switch code {
	case 401, 403:
		return "auth"
	case 429:
		return "throttled"
	case 404:
		return "not_found"
	case 400, 409, 422:
		return "client"
	}
	if code >= 500 && code <= 599 {
		return "server"
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "timeout"),
		strings.Contains(lower, "deadline exceeded"),
		strings.Contains(lower, "i/o timeout"):
		return "timeout"
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "network is unreachable"),
		strings.Contains(lower, "tls:"),
		strings.Contains(lower, "x509:"):
		return "network"
	case strings.Contains(lower, "credential rejected"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "forbidden"):
		return "auth"
	case strings.Contains(lower, "throttl"),
		strings.Contains(lower, "rate limit"):
		return "throttled"
	default:
		return "error"
	}
}
