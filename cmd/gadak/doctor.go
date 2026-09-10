package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/applog"
	"github.com/midagedev/gadak/internal/attachaudit"
	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/skillinstall"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// doctorBanner is the first line of every doctor dump so a user pasting into a
// public issue can see what was stripped before they hit submit. The site
// hostname is the one identifier the document does carry: it names
// the server, not the user, and it is the only surface that can reveal a
// wrong or placeholder site — masking it turned a config error into a
// half-day "Atlassian outage".
const doctorBanner = "# gadak doctor — safe to paste: no keys, names, emails or tokens; the site hostname is shown (it names the server, not you)"

// doctorReport is the redacted diagnostic document. Field names are stable for
// --json consumers; values never carry tokens, emails, project keys,
// custom-field names, or raw error strings. The one exception
// is Site, which shows the configured hostname — a wrong site is a config
// error this document must be able to name.
type doctorReport struct {
	GadakVersion    string `json:"gadak_version"`
	GoVersion       string `json:"go_version"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	Profile         string `json:"profile"`
	WorkspaceSource string `json:"workspace_source"`
	WorkspaceKind   string `json:"workspace_kind"`
	Origin          string `json:"origin"`
	OriginOwner     string `json:"origin_owner,omitempty"`
	// Home is the directory the default profile lives in, and HomeReason says
	// why it is that one when it is not ~/.gadak: "dev build" (GDK-1697) or
	// "GADAK_HOME".
	Home       string `json:"home"`
	HomeReason string `json:"home_reason,omitempty"`
	MirrorPath string `json:"mirror_path"`
	// HomeLeftover is the abandoned legacy home (~/.scry) when it still exists
	// beside ~/.gadak. The stderr warning fires once per machine (GDK-1072);
	// this is the standing report.
	HomeLeftover string `json:"home_leftover,omitempty"`
	// ProjectsMismatch is the config↔mirror project-scope rename signature
	// (GDK-973), as counts only — the keys themselves stay out of this
	// paste-safe document (the rule the banner states and TestDoctorRedaction
	// enforces). The post-sync warning and `gadak status` name the keys. Nil
	// when the signature is absent.
	ProjectsMismatch *doctorProjectsMismatch `json:"projects_mismatch,omitempty"`
	Mirror           doctorMirror            `json:"mirror"`
	MirrorHolders    *doctorMirrorHolders    `json:"mirror_holders,omitempty"`
	SchemaVersion    *int                    `json:"schema_version"`
	SchemaSinceSync  string                  `json:"schema_since_sync,omitempty"`
	SchemaAudit      *doctorSchemaAudit      `json:"schema_audit,omitempty"`
	Migrations       string                  `json:"migrations"`
	Counts           *doctorCounts           `json:"counts"`
	Confluence       string                  `json:"confluence"`
	CustomFields     doctorCustomFields      `json:"custom_fields"`
	Credential       string                  `json:"credential"`
	Site             string                  `json:"site"`
	Email            string                  `json:"email"`
	Skill            doctorSkill             `json:"skill"`
	MCP              doctorMCP               `json:"mcp"`
	Sync             map[string]doctorSync   `json:"sync"`
	APIUsage         doctorAPIUsage          `json:"api_usage"`
	Workspace        doctorWorkspace         `json:"workspace"`
	Logs             doctorLogs              `json:"logs"`
	// ConfluenceSpaces is the config↔mirror wiki-scope signature (GDK-1484),
	// as counts only — the keys stay out of this paste-safe document, the
	// same rule ProjectsMismatch follows. Nil when the signature is absent.
	ConfluenceSpaces *doctorConfluenceSpaces `json:"confluence_spaces,omitempty"`
	// MirrorShort is the mirror-vs-origin shortfall (GDK-1400): the origin
	// held more issues in scope at the last two-way reconcile than the mirror
	// holds now. Counts only, no keys — the same paste-safe rule
	// ProjectsMismatch follows. Nil when the mirror is level, is ahead, or no
	// reconcile has run.
	MirrorShort *doctorMirrorShort `json:"mirror_short,omitempty"`
	// AttachmentsMaybeTruncated counts mirrored attachments of exactly 8 MiB
	// on a built-in-origin workspace (GDK-1615): the size the old upload cap
	// produced. Nil elsewhere and when the count is zero — a Jira or Linear
	// workspace never went through that cap, so the line would be a false
	// statement about intact files.
	AttachmentsMaybeTruncated *int `json:"attachments_maybe_truncated,omitempty"`
	// LinksOneSided counts stored relationships whose far end does not hold
	// the counterpart row (GDK-1507): the standing symptom of an incremental
	// window that carried one end of a link and not the other. Counts only,
	// no keys — the paste-safe rule. Nil when the mirror is consistent; a
	// non-zero number is repaired by one full sync.
	LinksOneSided *int `json:"links_one_sided,omitempty"`
	// Attachments is the state of this workspace's attachment bytes: how
	// many rows the mirror holds, and how much of it is local. It exists so
	// "what shape are the attachments in here?" is one command rather than
	// a directory walk plus a SQL query (GDK-1616/1615).
	Attachments *doctorAttachments `json:"attachments,omitempty"`
	// DevLinks answers the question an empty dev_links table cannot
	// (GDK-1496): is this workspace mirroring the development panel at
	// all? A connected Cloud workspace has the fetch off by default, so
	// zero rows there means "not synced", not "no pull requests" — and
	// SQL alone cannot tell those apart.
	DevLinks *doctorDevLinks `json:"dev_links,omitempty"`
}

// doctorDevLinks is counts plus the one flag that explains them. No URLs,
// no issue keys — the same paste-safe rule as every other section.
type doctorDevLinks struct {
	Mirrored bool   `json:"mirrored"`
	Rows     int    `json:"rows"`
	Issues   int    `json:"issues"`
	Origin   string `json:"origin,omitempty"`
}

// doctorAttachments is counts only, like every other section of this
// paste-safe document: no filenames, no issue keys.
type doctorAttachments struct {
	Mirrored     int    `json:"mirrored"`
	Cached       int    `json:"cached"`
	CachedBytes  int64  `json:"cached_bytes"`
	CacheDir     string `json:"cache_dir,omitempty"`
	PerFileCapMB int    `json:"per_file_cap_mb,omitempty"`
}

// doctorMirrorShort is the count-only view of a mirror that holds less than
// its origin does. It is the standing form of the question the GDK-1400 field
// case needed a second machine to answer: that host reported a healthy sync
// on every tick while holding 134 of its origin's 1,399 issues.
type doctorMirrorShort struct {
	Mirror   int    `json:"mirror"`
	Upstream int    `json:"upstream"`
	At       string `json:"at"`
}

// doctorLogs is the process log file Install opens under the gadak home.
// Path is tilde-abbreviated like mirror_path. Size is omitted when the
// file is absent. Recent is error-ish ring lines, cap 10.
type doctorLogs struct {
	Path    string   `json:"path"`
	Size    *int64   `json:"size,omitempty"`
	Rotated bool     `json:"rotated"`
	Recent  []string `json:"recent,omitempty"`
}

// doctorWorkspace is the one-line consistency view: kind, whether a site
// token is stored (never the token itself), the origin persist path, and
// how many locally originated issues LocalData counts. Inconsistent is
// built-in-with-a-token — a site token on a built-in workspace is
// unused and contradicts Kind (GDK-247).
//
// HasSiteToken is site-token presence, not config.HasCredential (GDK-470).
// Built-in writes work with no site token; this field stays false there.
// doctorCustomFields is the mapping-visibility object (GDK-522). mapped is
// the configured alias count; applied_at is when `gadak fields --apply` last
// succeeded. usage_rows and raw_has_custom need the mirror (0 / false when
// it is missing). The previous JSON value was that same mapped count as an
// int; mapped preserves it.
type doctorCustomFields struct {
	Mapped       int    `json:"mapped"`
	AppliedAt    string `json:"applied_at,omitempty"`
	UsageRows    int    `json:"usage_rows"`
	RawHasCustom bool   `json:"raw_has_custom"`
	// rawScanned is true only when HasCustomFieldKeysInRaw actually ran.
	rawScanned bool
}

type doctorWorkspace struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// OriginType and Transport are the two questions Kind used to answer
	// at once (GDK-1278): which tracker (jira|linear|gadak), and whether
	// it is reached in-process or across a serve API (local|remote). Kind
	// stays for readers that have not moved.
	OriginType   string `json:"origin_type"`
	Transport    string `json:"transport"`
	HasSiteToken bool   `json:"has_site_token"`
	Persist      string `json:"persist"`
	// PreUpgradeCopy is the copy the attachment migration left behind
	// (`<persist>.pre-v2.bak`). It is the only way back to a gadak older
	// than the migration, and it is a second full copy of the database —
	// both reasons to say it is there rather than let it be found by a
	// disk-usage tool (GDK-1617).
	PreUpgradeCopy      string `json:"pre_upgrade_copy,omitempty"`
	PreUpgradeCopyBytes int64  `json:"pre_upgrade_copy_bytes,omitempty"`
	LocalIssues         int    `json:"local_issues"`
	Inconsistent        bool   `json:"inconsistent"`
	Frozen              bool   `json:"frozen"`
}

// doctorSkill answers "is my agent's skill current?" without the user having
// to diff files by hand. Identity is the content hash, never mtime.
//
// Status/Scope/Path summarise the whole machine in the three fields this report
// has always had: the first host that has a copy, in table order. Hosts carries
// the per-host detail (GDK-1508) — before it, a user who had correctly
// installed the skill for Codex was told "missing", because doctor only ever
// looked at ~/.claude.
type doctorSkill struct {
	Status string `json:"status"` // missing | stale | current
	Scope  string `json:"scope"`  // user | project
	Path   string `json:"path"`   // tilde-abbreviated (user) or repo-relative (project)
	// LastAutoCheck is when the daily auto-sync last looked (GDK-996), so
	// "why did/didn't it update?" is one line. Same convention as
	// sync.*.synced_at: "never" before the first check.
	LastAutoCheck string `json:"last_auto_check"`
	// Hosts is one row per agent host gadak can see: every host with a copy
	// installed, plus every host whose configuration directory says it is on
	// this machine. A host that is neither is left out rather than listed as
	// missing — a row nobody can act on is noise.
	Hosts []doctorSkillHost `json:"hosts,omitempty"`
}

// doctorSkillHost is one host's verdict. Status here keeps the installer's own
// fourth word, conflict, which the summary line folds into stale: for the
// summary either one means "what the agent loads is not this binary's skill",
// but a JSON consumer can tell "behind" from "yours, and gadak will not touch
// it" without running `skill install --print`.
type doctorSkillHost struct {
	Client string `json:"client"` // claude | codex | agents | …
	Status string `json:"status"` // current | stale | conflict | missing
	Scope  string `json:"scope"`  // user | project
	Path   string `json:"path"`   // tilde-abbreviated (user) or literal relative (project)
	// Source, InstalledByVersion and Revision come from the receipt beside the
	// file (GDK-1531). They answer "current according to which gadak?": a copy
	// a checkout build wrote reads `dev-tree`, so a working-tree draft can no
	// longer sit in an agent home looking like a shipped release. Absent when
	// no receipt describes the bytes on disk — a hand-edited file keeps the
	// receipt of the copy it replaced, and that receipt is not about it.
	Source             string `json:"source,omitempty"`               // release | dev-tree
	InstalledByVersion string `json:"installed_by_version,omitempty"` // the receipt's gadak_version
	Revision           string `json:"revision,omitempty"`             // short git hash, "+" if the tree was dirty
}

// doctorMCP reports whether gadak is registered as an MCP server. Only the
// config file and the scope name are reported — never the project directory a
// local registration is filed under, which would put the user's working path in
// a document the banner promises is safe to paste.
type doctorMCP struct {
	Status string `json:"status"`          // absent | registered
	Scope  string `json:"scope,omitempty"` // user | local | project | other | claude-desktop
	Path   string `json:"path,omitempty"`  // tilde-abbreviated config path
}

// doctorProjectsMismatch is the count-only view of the config↔mirror
// project-scope rename signature (GDK-973): configured keys the Jira mirror
// holds no issues under, beside Jira-mirrored keys the config does not list.
// Which keys those are never enters doctor output — the banner promises a
// paste-safe document; the post-sync warning and `gadak status` name them.
type doctorProjectsMismatch struct {
	ConfiguredNotMirrored       int `json:"configured_not_mirrored"`        // configured keys with zero mirrored issues
	MirroredNotConfigured       int `json:"mirrored_not_configured"`        // Jira-mirrored keys outside the config
	MirroredNotConfiguredIssues int `json:"mirrored_not_configured_issues"` // issues held under those keys
}

type doctorMirror struct {
	Status string `json:"status"`           // present | not_found | open_error | schema_too_new
	Bytes  *int64 `json:"bytes,omitempty"`  // set when the file exists
	Detail string `json:"detail,omitempty"` // redacted path / short reason
	// Version and ChangedAt are the live-update signal, reported so "why is
	// my open board not showing what I just wrote?" is one command instead of
	// a guess (GDK-1170). The ui-focus poll hands Version to the web every
	// 500ms and the board pulls a delta when it moves; ChangedAt (RFC3339,
	// UTC) is when the mirror bytes last moved. A write that leaves ChangedAt
	// in the past is a mirror that never got refreshed — the board is right
	// and the write-through is the suspect. Both are counters and a
	// timestamp: nothing here names a file, a person, or a key.
	Version   string `json:"version,omitempty"`
	ChangedAt string `json:"changed_at,omitempty"`
}

// doctorMirrorHolders is the best-effort list of other processes that have
// this profile's mirror open (GDK-740). Nil means the scan was skipped
// (non-darwin/linux, lsof missing, or it failed). Count 0 is a successful
// scan that found nobody but us.
type doctorMirrorHolders struct {
	Count     int                   `json:"count"`
	Processes []doctorMirrorProcess `json:"processes,omitempty"`
}

type doctorMirrorProcess struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

// doctorSchemaAudit is the user_version-vs-DDL check (GDK-180). Status is
// ok | mismatch | error. Sample is a handful of missing names, never the
// full list; table/column identifiers are schema, not personal data, but
// doctor still keeps the paste short.
type doctorSchemaAudit struct {
	Status    string   `json:"status"`
	Stamp     int      `json:"stamp"`
	Supported int      `json:"supported"`
	Missing   int      `json:"missing"`
	Extra     int      `json:"extra"`
	Sample    []string `json:"sample,omitempty"`
	Detail    string   `json:"detail,omitempty"`
}

type doctorCounts struct {
	Items  int `json:"items"`
	Issues int `json:"issues"`
	Pages  int `json:"pages"`
	// The comments table is shared by issue and wiki comments (GDK-628),
	// so doctor labels each share by meaning instead of one "comments" row
	// that disagrees with the settings runtime on every mirror with wiki
	// comments. Same owners as `gadak status --json`.
	IssueComments    int `json:"issue_comments"`
	PageComments     int `json:"page_comments"`
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
		GadakVersion:    version,
		GoVersion:       runtime.Version(),
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		Profile:         profile,
		WorkspaceSource: workspaceJSONSource(),
		WorkspaceKind:   config.KindConnected,
		Origin:          "jira",
		Confluence:      "inactive",
		Credential:      "absent",
		Site:            "none",
		Email:           "none",
		Sync:            map[string]doctorSync{},
		APIUsage: doctorAPIUsage{
			Day: time.Now().UTC().Format("2006-01-02"),
		},
		Migrations: "unknown",
		Workspace: doctorWorkspace{
			Name: workspaceJSONName(),
			Kind: config.KindConnected,
		},
	}

	if home, err := config.HomeRoot(); err == nil {
		rep.Home = tildeHome(home)
		switch {
		case config.Env("HOME") != "":
			rep.HomeReason = "GADAK_HOME"
		case config.DevHome():
			rep.HomeReason = "dev build"
		}
	} else {
		rep.Home = "unknown"
	}
	if path, err := config.DBPath(); err == nil {
		rep.MirrorPath = tildeHome(path)
	} else {
		rep.MirrorPath = "unknown"
	}
	if prev := config.DualHomeLeftover(); prev != "" {
		rep.HomeLeftover = tildeHome(prev)
	}

	if cfg, err := config.Load(); err == nil && cfg != nil {
		if cfg.Token != "" {
			rep.Credential = "present"
			if sampleTokenLiterals[cfg.Token] {
				// The placeholder config.json carried the doc
				// example "secret-token" and doctor passed it as present.
				rep.Credential = "sample placeholder"
			}
		}
		rep.Site = siteReport(cfg.Site)
		if cfg.Email != "" {
			rep.Email = "configured"
			if sampleEmailLiterals[strings.ToLower(cfg.Email)] {
				rep.Email = "sample placeholder"
			}
		}
		rep.CustomFields.Mapped = len(cfg.FieldSpecs())
		rep.CustomFields.AppliedAt = cfg.FieldsAppliedAt
		if cfg.Confluence != nil {
			rep.Confluence = "active"
		}
		kind, src := origin.Describe(cfg)
		rep.WorkspaceKind = kind
		if kind == config.KindStandalone {
			// Persist path is the origin; tilde so the account username
			// does not appear (same rule as mirror_path).
			rep.Origin = tildeHome(src)
			rep.OriginOwner = origin.OwnerStatus(cfg)
		} else {
			rep.Origin = src
		}
		n, persist, _ := originbind.LocalData(cfg)
		hasTok := cfg.Token != ""
		if rem, err := origin.PairedStatus(cfg); err == nil && rem != nil {
			// No endpoint here, unlike the site line (which shows only
			// the configured site host): a pairing label is the identity
			// doctor can name without leaking the serve endpoint.
			if rem.Label != "" {
				rep.Origin = fmt.Sprintf("paired gadak serve (label %q)", rem.Label)
			} else {
				rep.Origin = "paired gadak serve"
			}
			hasTok = true
			if cfg.Token == "" {
				rep.Credential = "present"
			}
		}
		rep.Workspace = doctorWorkspace{
			Name:         workspaceJSONName(),
			Kind:         cfg.WorkspaceKind(),
			OriginType:   cfg.OriginType(),
			Transport:    cfg.Transport(),
			HasSiteToken: hasTok,
			Persist:      tildeHome(persist),
			LocalIssues:  n,
			Inconsistent: cfg.HasBuiltInOrigin() && hasTok,
			Frozen:       cfg.SyncFrozen(),
		}
		if persist != "" {
			if fi, err := os.Stat(persist + ".pre-v2.bak"); err == nil {
				rep.Workspace.PreUpgradeCopy = tildeHome(persist + ".pre-v2.bak")
				rep.Workspace.PreUpgradeCopyBytes = fi.Size()
			}
		}
	}

	// Agent wiring is independent of the mirror, and the mirror branch below
	// returns early — collect it first so a user with no mirror still gets the
	// answer to "is my skill current?".
	rep.Skill = collectSkillStatus()
	rep.Skill.LastAutoCheck = doctorSkillAutoCheckWord(lastSkillAutoCheck())
	rep.MCP = collectMCPStatus()
	rep.Logs = collectLogs()

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
	// Read before store.Open below: opening mints -wal/-shm when they are
	// absent, which is itself a move. This has to report what an open board's
	// poll would have seen a moment ago, not what doctor just caused.
	rep.Mirror.Version = store.MirrorVersion(path)
	if at := store.MirrorChangedAt(path); !at.IsZero() {
		rep.Mirror.ChangedAt = at.UTC().Format(time.RFC3339)
	}
	rep.MirrorHolders = listMirrorHolders(path)

	// The user's workspace mirror: the dev-lockout policy applies — a dev build
	// must not migrate a release-written file just because doctor looked.
	db, err := store.OpenWith(path, storeOpenOptions())
	if err != nil {
		// doctor is what someone runs when the mirror has stopped opening, so
		// "open failed" is the one answer it must not give for a cause it can
		// name (GDK-498). The version pair is the whole diagnosis here.
		var tooNew *store.SchemaTooNewError
		if errors.As(err, &tooNew) {
			rep.Mirror.Status = "schema_too_new"
			rep.Mirror.Detail = fmt.Sprintf("written by a newer gadak; this build reads up to %d — run the newer gadak, or set the file aside and re-sync", tooNew.Supported)
			rep.SchemaVersion = &tooNew.Have
			rep.Migrations = "none applied"
			return rep
		}
		var refused *store.SchemaForwardRefusedError
		if errors.As(err, &refused) {
			rep.Mirror.Status = "schema_forward_refused"
			rep.Mirror.Detail = fmt.Sprintf("dev build refusing to migrate schema %d to %d — set GADAK_DEV_MIGRATE=1 to migrate anyway, or work on a copy", refused.Have, refused.Head)
			rep.SchemaVersion = &refused.Have
			rep.Migrations = "none applied"
			return rep
		}
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
	rep.SchemaSinceSync = doctorSchemaSinceSync(db, sv)
	rep.SchemaAudit = collectSchemaAudit(db)

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
	if n, err := db.IssueCommentCount(context.Background()); err == nil {
		counts.IssueComments = n
	}
	if n, err := db.PageCommentCount(context.Background()); err == nil {
		counts.PageComments = n
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

	if pm := collectProjectsMismatch(db); pm != nil {
		rep.ProjectsMismatch = pm
	}

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

	ctx := context.Background()
	if rows, err := db.FieldUsage(ctx); err == nil {
		rep.CustomFields.UsageRows = len(rows)
	}
	// The raw probe reads whole documents (~12 KB each; 230 MB at 20k issues
	// when no custom field exists — the exact-no case GDK-749). doctor's hint
	// samples instead: a hit is actionable advice either way, and a miss says
	// "none seen", which is all the hint ever claimed. `fields --apply` keeps
	// the exact probe, where a false no would wrongly refuse a sync.
	if rep.CustomFields.Mapped == 0 {
		if has, err := db.HasCustomFieldKeysInRawSampled(ctx, doctorRawSampleLimit); err == nil {
			rep.CustomFields.RawHasCustom = has
			rep.CustomFields.rawScanned = true
		}
	}

	if cs := collectConfluenceSpaces(db); cs != nil {
		rep.ConfluenceSpaces = cs
	}

	if ms := collectMirrorShort(db); ms != nil {
		rep.MirrorShort = ms
	}

	if n, err := db.CountOneSidedLinks(ctx); err == nil && n > 0 {
		rep.LinksOneSided = &n
	}

	rep.Attachments = collectAttachments(db)
	rep.DevLinks = collectDevLinks(db)

	// Only a built-in origin ever had the 8 MiB upload cap. The gate is the
	// origin type the attachment proxy itself keys on (config.OriginGadak),
	// not the presence of the count.
	if cfg, err := config.Load(); err == nil && cfg.OriginType() == config.OriginGadak {
		if n, err := db.CountAttachmentsOfSize(ctx, attachaudit.TruncatedSize); err == nil && n > 0 {
			rep.AttachmentsMaybeTruncated = &n
		}
	}

	return rep
}

// collectSkillStatus reuses the installer's own classifier (skillDestStatus in
// skill.go), so `gadak doctor` and `gadak skill install --print` can never
// disagree about the same file.
// collectProjectsMismatch reads the config↔mirror rename signature through
// sync.ProjectScopeMismatch — the same verdict the post-Jira-pass warning
// logs — so doctor and sync can never disagree about it. Reads are
// best-effort: an unopenable config or count query reports nothing rather
// than a half verdict.
func collectProjectsMismatch(db *store.DB) *doctorProjectsMismatch {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return nil
	}
	counts, err := db.ProjectIssueCounts(context.Background(), syncer.SourceID)
	if err != nil {
		return nil
	}
	empty, extra := syncer.ProjectScopeMismatch(cfg, counts)
	if len(extra) == 0 {
		return nil
	}
	out := &doctorProjectsMismatch{
		ConfiguredNotMirrored: len(empty),
		MirroredNotConfigured: len(extra),
	}
	for _, k := range extra {
		out.MirroredNotConfiguredIssues += counts[k]
	}
	return out
}

// formatDoctorProjectsMismatch names the verdict and the way out without
// naming keys: `gadak status` prints both scope-mismatch lines (GDK-809).
func formatDoctorProjectsMismatch(m doctorProjectsMismatch) string {
	return fmt.Sprintf("configured_not_mirrored=%d mirrored_not_configured=%d (%d issues) — a renamed Jira project key? `gadak status` lists the keys; fix with: gadak config set projects '[\"…\"]'",
		m.ConfiguredNotMirrored, m.MirroredNotConfigured, m.MirroredNotConfiguredIssues)
}

func collectSkillStatus() doctorSkill {
	content := gadak.SkillMarkdown()
	env := skillinstall.OSEnv()

	out := doctorSkill{}
	for _, client := range skillinstall.Clients() {
		host, found := skillHostStatus(client, env, content)
		// Always report the default client: "no skill anywhere" still needs a
		// path to tell the user where one would go.
		if !found && client.Name != skillinstall.DefaultClient && !client.Present(env) {
			continue
		}
		out.Hosts = append(out.Hosts, host)
	}

	// The summary triple is the first host that actually has a copy, in table
	// order — so a machine with only Codex set up reports Codex rather than the
	// Claude path it does not use.
	for _, h := range out.Hosts {
		if h.Status != skillinstall.StatusMissing {
			out.Status = doctorSkillWord(h.Status)
			out.Scope = h.Scope
			out.Path = h.Path
			return out
		}
	}
	out.Status = skillinstall.StatusMissing
	out.Scope = "user"
	out.Path = "unknown"
	for _, h := range out.Hosts {
		if h.Client == skillinstall.DefaultClient {
			out.Path = h.Path
		}
	}
	return out
}

// skillHostStatus classifies one host: user scope first, then project scope for
// the hosts that have one. found says whether a copy exists anywhere for it.
//
// A project install is reported with the literal relative path. Printing the
// working directory would put the user's project name into a report whose
// banner promises it is safe to paste in public.
func skillHostStatus(client skillinstall.Client, env skillinstall.Env, content []byte) (doctorSkillHost, bool) {
	host := doctorSkillHost{Client: client.Name, Status: skillinstall.StatusMissing, Scope: "user", Path: "unknown"}

	userDest, userErr := client.HomeDest(env)
	if userErr == nil {
		host.Path = tildeHome(userDest)
		if status, existing, err := skillinstall.DestStatus(userDest, content); err == nil && status != skillinstall.StatusMissing {
			host.Status = skillinstall.StatusWord(status)
			addSkillReceiptFacts(&host, userDest, existing)
			return host, true
		}
	}
	if client.HasProjectScope() {
		if projDest, err := client.ProjectDest(env); err == nil {
			if status, existing, err := skillinstall.DestStatus(projDest, content); err == nil && status != skillinstall.StatusMissing {
				proj := doctorSkillHost{
					Client: client.Name,
					Status: skillinstall.StatusWord(status),
					Scope:  "project",
					Path:   client.ProjectRelDir(),
				}
				addSkillReceiptFacts(&proj, projDest, existing)
				return proj, true
			}
		}
	}
	return host, false
}

// addSkillReceiptFacts copies the receipt's provenance onto a host row, but
// only when the receipt describes the bytes that are actually there. Identity
// is the content hash, the same rule the installer's classifier uses: a file
// somebody edited by hand still has the previous copy's receipt lying beside
// it, and reporting that receipt as this file's provenance would be a lie of
// exactly the kind GDK-1531 is about.
func addSkillReceiptFacts(host *doctorSkillHost, dest string, existing []byte) {
	r, ok := skillinstall.ReadReceipt(filepath.Dir(dest))
	if !ok || r.SHA256 != skillinstall.Digest(existing) {
		return
	}
	host.Source = r.SourceWord()
	host.InstalledByVersion = r.GadakVersion
	host.Revision = r.Revision
}

// formatDoctorSkill is the one summary line. With a single known host it is
// what it has always been — the status and the path, which is the detail that
// helps. With several, the paths stop fitting and the host names are what the
// user needs: `current (claude, codex) · missing (agents)`.
func formatDoctorSkill(s doctorSkill) string {
	marker := devTreeMarker(s.Hosts)
	if len(s.Hosts) <= 1 {
		switch {
		case s.Path != "" && marker != "":
			return s.Status + " (" + s.Path + ", " + marker + ")"
		case s.Path != "":
			return s.Status + " (" + s.Path + ")"
		case marker != "":
			return s.Status + " (" + marker + ")"
		}
		return s.Status
	}
	byStatus := map[string][]string{}
	for _, h := range s.Hosts {
		w := doctorSkillWord(h.Status)
		byStatus[w] = append(byStatus[w], h.Client)
	}
	var parts []string
	for _, w := range []string{"current", "stale", skillinstall.StatusMissing} {
		if names := byStatus[w]; len(names) > 0 {
			parts = append(parts, w+" ("+strings.Join(names, ", ")+")")
		}
	}
	if marker != "" {
		parts = append(parts, marker)
	}
	return strings.Join(parts, " · ")
}

// devTreeMarker is the suffix that keeps a "current" verdict honest: the copy
// the agent loads is byte-identical to this binary's skill, but this binary was
// built from somebody's checkout, so "current" says nothing about whether the
// text was ever reviewed (GDK-1531). It names the revision when the receipt
// recorded one, so `skill: current (dev-tree ac8e154+)` is enough to go find
// the build — the trailing "+" means that tree had uncommitted changes.
func devTreeMarker(hosts []doctorSkillHost) string {
	marker := ""
	for _, h := range hosts {
		if h.Source != skillinstall.SourceDevTree {
			continue
		}
		if h.Revision != "" {
			return skillinstall.SourceDevTree + " " + h.Revision
		}
		marker = skillinstall.SourceDevTree
	}
	return marker
}

// doctorSkillWord maps the installer's four-way classification onto the three
// words doctor reports. "conflict" (a file gadak did not write) reports as
// stale because either way what the agent loads is not this binary's skill;
// `gadak skill install --print` is the command that tells the two apart.
func doctorSkillWord(installStatus string) string {
	switch installStatus {
	case skillinstall.StatusIdentical, "current":
		return "current"
	case skillinstall.StatusMissing:
		return skillinstall.StatusMissing
	}
	return "stale"
}

// doctorSkillAutoCheckWord maps the empty auto-sync stamp (no check has run
// yet on this machine) to "never".
func doctorSkillAutoCheckWord(stamp string) string {
	if stamp == "" {
		return "never"
	}
	return stamp
}

// claudeMCPConfig is the slice of Claude Code's config that says whether gadak
// is registered. `claude mcp add` (what `gadak mcp install claude` runs) files
// the entry under projects[<cwd>] at its default scope, and under the top-level
// mcpServers at user scope.
type claudeMCPConfig struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
	Projects   map[string]struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	} `json:"projects"`
}

// claudeConfigMaxBytes caps the read of a config file doctor does not own.
const claudeConfigMaxBytes = 32 << 20

func collectMCPStatus() doctorMCP {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cfgPath := filepath.Join(home, ".claude.json")
		if cfg, ok := readClaudeMCPConfig(cfgPath); ok {
			if _, found := cfg.MCPServers["gadak"]; found {
				return doctorMCP{Status: "registered", Scope: "user", Path: tildeHome(cfgPath)}
			}
			if cwd, err := os.Getwd(); err == nil {
				if _, found := cfg.Projects[cwd].MCPServers["gadak"]; found {
					return doctorMCP{Status: "registered", Scope: "local", Path: tildeHome(cfgPath)}
				}
			}
			for _, p := range cfg.Projects {
				if _, found := p.MCPServers["gadak"]; found {
					// Registered, but filed under some other directory — the
					// directory itself is never printed.
					return doctorMCP{Status: "registered", Scope: "other", Path: tildeHome(cfgPath)}
				}
			}
		}
	}
	// Project-scoped registrations live in a .mcp.json beside the checkout.
	if cfg, ok := readClaudeMCPConfig(".mcp.json"); ok {
		if _, found := cfg.MCPServers["gadak"]; found {
			return doctorMCP{Status: "registered", Scope: "project", Path: ".mcp.json"}
		}
	}
	// Claude Desktop is a different app with its own config file, and it has
	// no shell — `gadak mcp install claude-desktop` is the only way in, and
	// nothing above can see the result. Reporting "absent" to someone whose
	// registration is fine was a false negative (GDK-1643). The scope is its
	// own value: borrowing user/local/project would drop which host it is,
	// which is the one thing this branch exists to say. Host-bound path on
	// purpose — doctor reports on the machine it runs on.
	if path, err := clitool.ClaudeDesktopConfigPath(); err == nil {
		if cfg, ok := readClaudeMCPConfig(path); ok {
			if _, found := cfg.MCPServers["gadak"]; found {
				return doctorMCP{Status: "registered", Scope: "claude-desktop", Path: tildeHome(path)}
			}
		}
	}
	return doctorMCP{Status: "absent"}
}

// readClaudeMCPConfig is best-effort: a missing, oversized, or unparsable file
// is simply "no registration found". doctor never writes to it.
func readClaudeMCPConfig(path string) (claudeMCPConfig, bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() || fi.Size() > claudeConfigMaxBytes {
		return claudeMCPConfig{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return claudeMCPConfig{}, false
	}
	var cfg claudeMCPConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return claudeMCPConfig{}, false
	}
	return cfg, true
}

const schemaAuditSampleCap = 3

// doctorRawSampleLimit bounds doctor's raw customfield probe (GDK-749).
const doctorRawSampleLimit = 500

func collectSchemaAudit(db *store.DB) *doctorSchemaAudit {
	got, err := db.SchemaAudit(context.Background())
	if err != nil {
		return &doctorSchemaAudit{Status: "error", Detail: "audit failed"}
	}
	out := &doctorSchemaAudit{
		Status:    "ok",
		Stamp:     got.Stamp,
		Supported: got.Supported,
		Missing:   len(got.Missing),
		Extra:     len(got.Extra),
	}
	if len(got.Missing) > 0 {
		out.Status = "mismatch"
		out.Sample = schemaAuditSample(got.Missing, schemaAuditSampleCap)
	}
	return out
}

func schemaAuditSample(names []string, cap int) []string {
	if len(names) <= cap {
		return append([]string(nil), names...)
	}
	return append([]string(nil), names[:cap]...)
}

func formatDoctorSchemaAudit(a doctorSchemaAudit) string {
	switch a.Status {
	case "ok":
		return "ok"
	case "mismatch":
		sample := strings.Join(a.Sample, ", ")
		if a.Missing > len(a.Sample) && sample != "" {
			sample += ", ..."
		}
		if sample == "" {
			return fmt.Sprintf("mismatch (%d missing) stamp=%d this_build=%d — mirror is damaged; delete the mirror file and run gadak sync",
				a.Missing, a.Stamp, a.Supported)
		}
		return fmt.Sprintf("mismatch (%d missing: %s) stamp=%d this_build=%d — mirror is damaged; delete the mirror file and run gadak sync",
			a.Missing, sample, a.Stamp, a.Supported)
	default:
		if a.Detail != "" {
			return a.Status + " (" + a.Detail + ")"
		}
		return a.Status
	}
}

// doctorSchemaSinceSync names a lag between PRAGMA user_version and the
// sync_state.schema_version column. The column is only rewritten when a
// migration actually runs, so a later Open can leave it behind (GDK-526).
func doctorSchemaSinceSync(db *store.DB, live int) string {
	seen := map[int]struct{}{}
	var stale []int
	for _, src := range []string{"jira", "confluence"} {
		ss, err := db.SyncState(context.Background(), src)
		if err != nil {
			continue
		}
		row := ss.SchemaVersionRow
		if row <= 0 || row == live {
			continue
		}
		if _, ok := seen[row]; ok {
			continue
		}
		seen[row] = struct{}{}
		stale = append(stale, row)
	}
	if len(stale) == 0 {
		return ""
	}
	parts := make([]string, len(stale))
	for i, n := range stale {
		parts[i] = strconv.Itoa(n)
	}
	return "migrated since last sync (sync_state has " + strings.Join(parts, ", ") + ")"
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

	line("gadak_version", r.GadakVersion)
	line("go_version", r.GoVersion)
	line("os", r.OS+"/"+r.Arch)
	line("profile", r.Profile)
	line("workspace_kind", r.WorkspaceKind)
	line("origin_type", r.Workspace.OriginType)
	line("transport", r.Workspace.Transport)
	line("origin", r.Origin)
	if r.OriginOwner != "" {
		line("origin owner", r.OriginOwner)
	}
	line("workspace", formatDoctorWorkspace(r.Workspace))
	if r.HomeReason != "" {
		line("home", r.Home+" ("+r.HomeReason+")")
	} else {
		line("home", r.Home)
	}
	line("mirror_path", r.MirrorPath)
	if r.HomeLeftover != "" {
		line("home_leftover", r.HomeLeftover+" (ignored legacy home — delete it, or move anything you still need into the active home)")
	}
	if r.MirrorHolders != nil {
		line("mirror_holders", formatDoctorMirrorHolders(*r.MirrorHolders))
	}

	switch r.Mirror.Status {
	case "present":
		if r.Mirror.Bytes != nil {
			line("mirror", fmt.Sprintf("present (%s)", store.HumanBytes(*r.Mirror.Bytes)))
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
	if r.Mirror.Version != "" {
		line("mirror_version", formatDoctorMirrorVersion(r.Mirror))
	}

	if r.SchemaVersion != nil {
		line("schema_version", strconv.Itoa(*r.SchemaVersion))
	} else {
		line("schema_version", "unknown")
	}
	if r.SchemaSinceSync != "" {
		line("schema_since_sync", r.SchemaSinceSync)
	}
	if r.SchemaAudit != nil {
		line("schema_audit", formatDoctorSchemaAudit(*r.SchemaAudit))
	}
	line("migrations", r.Migrations)

	if r.Counts != nil {
		line("items", strconv.Itoa(r.Counts.Items))
		line("issues", strconv.Itoa(r.Counts.Issues))
		line("pages", strconv.Itoa(r.Counts.Pages))
		line("issue_comments", strconv.Itoa(r.Counts.IssueComments))
		line("page_comments", strconv.Itoa(r.Counts.PageComments))
		line("projects", strconv.Itoa(r.Counts.Projects))
		line("status_categories", strconv.Itoa(r.Counts.StatusCategories))
		line("spaces", strconv.Itoa(r.Counts.Spaces))
	} else {
		line("items", "n/a")
		line("issues", "n/a")
		line("pages", "n/a")
		line("issue_comments", "n/a")
		line("page_comments", "n/a")
		line("projects", "n/a")
		line("status_categories", "n/a")
		line("spaces", "n/a")
	}

	if r.ProjectsMismatch != nil {
		line("projects_mismatch", formatDoctorProjectsMismatch(*r.ProjectsMismatch))
	}

	line("custom_fields", formatDoctorCustomFields(r.CustomFields))
	line("confluence", r.Confluence)
	if r.MirrorShort != nil {
		line("mirror_short", formatDoctorMirrorShort(*r.MirrorShort))
	}
	if r.Attachments != nil {
		line("attachments", formatDoctorAttachments(*r.Attachments))
	}
	if r.DevLinks != nil {
		line("dev_links", formatDoctorDevLinks(*r.DevLinks))
	}
	if r.AttachmentsMaybeTruncated != nil {
		line("attachments_maybe_truncated", attachaudit.Summary(*r.AttachmentsMaybeTruncated))
	}
	if r.LinksOneSided != nil {
		line("links_one_sided", fmt.Sprintf("%d link rows whose far end has no counterpart row — an incremental window carried one end only; one `gadak sync --full` levels it (GDK-1507)", *r.LinksOneSided))
	}
	if r.ConfluenceSpaces != nil {
		line("confluence_spaces", formatDoctorConfluenceSpaces(*r.ConfluenceSpaces))
	}
	line("credential", r.Credential)
	line("site", r.Site)
	line("email", r.Email)

	line("skill", formatDoctorSkill(r.Skill))
	if r.Skill.LastAutoCheck != "" {
		line("skill.last_auto_check", r.Skill.LastAutoCheck)
	}
	if r.MCP.Path != "" {
		line("mcp", r.MCP.Status+" ("+r.MCP.Path+", "+r.MCP.Scope+")")
	} else {
		line("mcp", r.MCP.Status)
	}

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

	if r.Logs.Path != "" {
		line("logs.path", r.Logs.Path)
		if r.Logs.Size != nil {
			line("logs.size", store.HumanBytes(*r.Logs.Size))
		} else {
			line("logs.size", "not found")
		}
		rotated := "no"
		if r.Logs.Rotated {
			rotated = "yes"
		}
		line("logs.rotated", rotated)
		if len(r.Logs.Recent) > 0 {
			line("logs.recent", strings.Join(r.Logs.Recent, " | "))
		}
	}

	return b.String()
}

func collectLogs() doctorLogs {
	dir, err := config.DirFor("")
	if err != nil {
		return doctorLogs{Path: "unknown"}
	}
	p := applog.Path(dir)
	out := doctorLogs{Path: tildeHome(p)}
	if fi, err := os.Stat(p); err == nil {
		sz := fi.Size()
		out.Size = &sz
	}
	if _, err := os.Stat(p + ".1"); err == nil {
		out.Rotated = true
	}
	// The ring only holds what this process logged, and doctor is usually a
	// fresh process that has logged nothing — so the file is the real source
	// here and the ring is the fallback for a long-lived one (serve).
	if lines := errorishLogLines(applog.Tail(dir, 2000), 10); len(lines) > 0 {
		out.Recent = lines
	} else {
		out.Recent = errorishLogLines(applog.Recent(500), 10)
	}
	return out
}

func errorishLogLines(lines []string, capN int) []string {
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < capN; i-- {
		if isErrorishLog(lines[i]) {
			out = append(out, lines[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func isErrorishLog(line string) bool {
	lower := strings.ToLower(line)
	for _, k := range []string{"error", "failed", "denied", "refused", "panic"} {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func formatDoctorCustomFields(cf doctorCustomFields) string {
	if cf.Mapped > 0 {
		word := "aliases"
		if cf.Mapped == 1 {
			word = "alias"
		}
		applied := ""
		if cf.AppliedAt != "" {
			applied = " (applied " + cf.AppliedAt + ")"
		}
		return fmt.Sprintf("%d %s mapped%s, usage rows %d", cf.Mapped, word, applied, cf.UsageRows)
	}
	if cf.rawScanned && cf.RawHasCustom {
		return "none mapped — raw carries customfield keys; run gadak fields --apply"
	}
	if cf.rawScanned {
		return "none mapped (none seen in raw)"
	}
	return "none mapped"
}

// formatDoctorMirrorVersion renders the live-update signal as the one line
// that answers "is my open board going to notice a write?" — the identity the
// ui-focus poll compares, plus how long ago it last moved. A version that is
// hours old right after a `gadak claim` says the mirror never got refreshed;
// a version that moves while the board sits still says the web half is where
// to look (GDK-1170).
func formatDoctorMirrorVersion(m doctorMirror) string {
	if m.ChangedAt == "" {
		return m.Version
	}
	at, err := time.Parse(time.RFC3339, m.ChangedAt)
	if err != nil {
		return m.Version + " (moved " + m.ChangedAt + ")"
	}
	return fmt.Sprintf("%s (moved %s ago, %s)", m.Version, formatDoctorAge(time.Since(at)), m.ChangedAt)
}

// formatDoctorAge is a coarse duration for a human reading one line. Sub-second
// reads as "0s" on purpose: what matters is "just now" vs "not since I wrote".
func formatDoctorAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours())/24)
}

func formatDoctorMirrorHolders(h doctorMirrorHolders) string {
	if h.Count == 0 {
		return "none"
	}
	parts := make([]string, 0, len(h.Processes))
	for _, p := range h.Processes {
		cmd := p.Command
		if cmd == "" {
			cmd = "?"
		}
		parts = append(parts, fmt.Sprintf("pid %d %s", p.PID, cmd))
	}
	if len(parts) == 0 {
		return strconv.Itoa(h.Count)
	}
	return fmt.Sprintf("%d (%s)", h.Count, strings.Join(parts, ", "))
}

// listMirrorHolders is best-effort: darwin and linux run lsof on the mirror
// and its WAL/SHM sidecars, drop this process, and return count+pid/command.
// A missing binary, timeout, or other failure returns nil so doctor omits
// the section rather than guessing.
func listMirrorHolders(mirrorPath string) *doctorMirrorHolders {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil
	}
	if mirrorPath == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := []string{"-F", "pc", "--", mirrorPath, mirrorPath + "-wal", mirrorPath + "-shm"}
	cmd := exec.CommandContext(ctx, "lsof", args...)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return nil
		}
	}
	procs := parseLsofHolders(out, os.Getpid())
	return &doctorMirrorHolders{Count: len(procs), Processes: procs}
}

// parseLsofHolders reads `lsof -F pc` output: one process per pid, command
// name only, self excluded, no paths. Empty input is a successful empty scan.
func parseLsofHolders(out []byte, selfPID int) []doctorMirrorProcess {
	byPID := map[int]string{}
	var curPID int
	var curCmd string
	flush := func() {
		if curPID != 0 && curPID != selfPID && curCmd != "" {
			byPID[curPID] = curCmd
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			flush()
			n, convErr := strconv.Atoi(line[1:])
			if convErr != nil {
				curPID, curCmd = 0, ""
				continue
			}
			curPID, curCmd = n, ""
		case 'c':
			curCmd = line[1:]
		}
	}
	flush()
	pids := make([]int, 0, len(byPID))
	for pid := range byPID {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	procs := make([]doctorMirrorProcess, 0, len(pids))
	for _, pid := range pids {
		procs = append(procs, doctorMirrorProcess{PID: pid, Command: byPID[pid]})
	}
	return procs
}

func formatDoctorWorkspace(w doctorWorkspace) string {
	tok := "no"
	if w.HasSiteToken {
		tok = "yes"
	}
	persist := w.Persist
	if persist == "" {
		persist = "none"
	}
	frozen := "no"
	if w.Frozen {
		frozen = "yes"
	}
	s := fmt.Sprintf("kind=%s site_token=%s persist=%s issues=%d frozen=%s",
		w.Kind, tok, persist, w.LocalIssues, frozen)
	if w.PreUpgradeCopy != "" {
		s += fmt.Sprintf("\n  pre-upgrade copy %s (%d MiB) — the way back to a gadak older than the attachment migration; delete it once you are staying",
			w.PreUpgradeCopy, w.PreUpgradeCopyBytes>>20)
	}
	if w.Inconsistent {
		s += " inconsistent"
	}
	return s
}

// tildeHome shortens an absolute path under the user's home to ~/… so doctor
// output never embeds the account username. It is clitool.TildeHome — the one
// implementation, kept behind this name so doctor's many call sites stay short.
// GADAK_HOME may sit outside $HOME (tests, CI); such a path is returned cleaned
// but otherwise as-is, since it carries no home prefix to strip.
func tildeHome(path string) string {
	return clitool.TildeHome(path)
}

// sampleSiteHosts are the reserved example hosts: the one gadak's own prompts
// suggest (your-site.atlassian.net) and the one a real incident found
// configured on a real machine (example.atlassian.net — unprovisioned, so
// Atlassian answers every request with 404 "Site temporarily unavailable",
// which reads as a remote outage). A site on one of these hosts is a config
// error, and doctor says so instead of showing it as an ordinary site.
var sampleSiteHosts = map[string]bool{
	"example.atlassian.net":   true,
	"your-site.atlassian.net": true,
}

// sampleTokenLiterals and sampleEmailLiterals are the sample strings from the
// same incident's placeholder config.json: they reported "present" and
// "configured" while the workspace could not talk to anything. Match is exact
// (email case-insensitive); a real token colliding with "secret-token" is not
// a configuration anyone has.
var (
	sampleTokenLiterals = map[string]bool{"secret-token": true}
	sampleEmailLiterals = map[string]bool{"someone@example.com": true}
)

// siteReport renders the configured Jira site for the doctor document: the
// hostname, unmasked. Before the unmasking this collapsed every Atlassian Cloud
// site to "<redacted>.atlassian.net", which hid the one value whose wrongness
// explained every symptom — masking turned a placeholder site into a
// half-day "wait for Atlassian to recover". A hostname names the server, not
// the user; tokens, emails, project keys and error bodies stay out (the
// banner's rule, TestDoctorRedaction). Userinfo in the URL is dropped by
// Hostname(); a value with no parsable host is shown raw behind "malformed:"
// because that value itself is the defect.
func siteReport(site string) string {
	site = strings.TrimSpace(site)
	if site == "" {
		return "none"
	}
	host := ""
	if u, err := url.Parse(site); err == nil {
		host = u.Hostname()
	}
	if host == "" && !strings.Contains(site, "/") {
		// A bare host with no scheme is still a site someone meant; classify
		// it by host rather than calling it malformed.
		if u, err := url.Parse("https://" + site); err == nil {
			host = u.Hostname()
		}
	}
	if host == "" {
		return "malformed: " + site
	}
	host = strings.ToLower(host) // hostnames are case-insensitive; one spelling
	if sampleSiteHosts[host] {
		return host + " (sample placeholder — run gadak init)"
	}
	return host
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

// doctorConfluenceSpaces is the count-only view of the config↔mirror wiki
// scope (GDK-1484): configured space keys the spaces catalog does not hold.
// The pass writes a catalog row only for a key the origin resolved, so a
// configured key with no row is one the origin does not have — the shape a
// built-in default (LOC) takes after `gadak migrate` or a pairing builds
// a new origin under the old config. No network call: doctor stays offline
// and fast, and the mirror already carries the answer. Which keys those are
// never enters doctor output; `gadak status` names them.
type doctorConfluenceSpaces struct {
	Configured  int `json:"configured"`    // non-empty keys in confluence.spaces
	NotOnOrigin int `json:"not_on_origin"` // of those, keys with no catalog row
}

// collectConfluenceSpaces returns nil when there is nothing to say: the wiki
// source is off, the scope is the empty list (every space the origin has, so
// it cannot name a missing one), the pass has never run (the catalog is
// empty for a reason that is not a mismatch — the offline equivalent of the
// "skipped" a network check would report), or every configured key resolved.
func collectConfluenceSpaces(db *store.DB) *doctorConfluenceSpaces {
	cfg, err := config.Load()
	if err != nil || cfg == nil || cfg.Confluence == nil || len(cfg.Confluence.Spaces) == 0 {
		return nil
	}
	ctx := context.Background()
	ss, err := db.SyncState(ctx, syncer.ConfluenceSourceID)
	if err != nil || ss.SyncCount == 0 {
		return nil
	}
	rows, err := db.Query(`SELECT key FROM spaces WHERE source_id = ?`, syncer.ConfluenceSourceID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil
		}
		known[k] = true
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	out := &doctorConfluenceSpaces{}
	for _, k := range cfg.Confluence.Spaces {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out.Configured++
		if !known[k] {
			out.NotOnOrigin++
		}
	}
	if out.NotOnOrigin == 0 {
		return nil
	}
	return out
}

// formatDoctorConfluenceSpaces names the verdict and the way out without
// naming keys — `gadak status` prints the configured list (GDK-1484).
func formatDoctorConfluenceSpaces(s doctorConfluenceSpaces) string {
	return fmt.Sprintf(`configured=%d not_on_origin=%d — the origin has no space under those keys, so nothing is mirrored for them; `+
		"`gadak status`"+` names them; fix with: gadak config set confluence.spaces "[]"`,
		s.Configured, s.NotOnOrigin)
}

// collectMirrorShort compares what the mirror holds against what the origin
// reported at the last two-way reconcile (GDK-1400). A short mirror is silent
// by nature — every incremental pass over `updated >= watermark` succeeds and
// reports nothing wrong — so this is the only standing surface that names it.
// Reads are best-effort: no reconcile yet, an unreadable config, or a failed
// count reports nothing rather than a half verdict. A mirror that is level or
// ahead is not a finding: ahead means rows outside the reconcile's scope, and
// deciding those are surplus is the reconcile's job, not doctor's.
func collectMirrorShort(db *store.DB) *doctorMirrorShort {
	ctx := context.Background()
	ss, err := db.IssueSyncState(ctx)
	if err != nil || !ss.Reconcile.Ran() {
		return nil
	}
	var projects []string
	if ss.SourceID == syncer.SourceID {
		// Jira's reconcile scope is the configured project list. A config we
		// cannot read means we cannot name the scope, and an unscoped count
		// compared against a scoped one is not a verdict.
		cfg, cerr := config.Load()
		if cerr != nil || cfg == nil {
			return nil
		}
		projects = cfg.Projects
	}
	have, err := db.CountIssuesInScope(ctx, ss.SourceID, projects)
	if err != nil || have >= ss.Reconcile.UpstreamKeys {
		return nil
	}
	return &doctorMirrorShort{Mirror: have, Upstream: ss.Reconcile.UpstreamKeys, At: ss.Reconcile.At}
}

// formatDoctorMirrorShort names the verdict and the way out. The repair is
// automatic — the hourly reconcile fetches what is missing — so the line says
// how to make it happen now rather than presenting a manual fix as the only
// one.
func formatDoctorMirrorShort(m doctorMirrorShort) string {
	return fmt.Sprintf("mirror=%d upstream=%d (at %s) — the origin held more than this mirror does; the hourly reconcile fetches the difference, or force it now with: gadak sync --full",
		m.Mirror, m.Upstream, m.At)
}

// collectAttachments reports how many attachments the mirror knows about and
// how many of their bytes are already local. The cache is read where it is,
// never created: doctor must not mint a directory just by looking (the same
// rule that keeps it from minting -wal/-shm above).
func collectAttachments(db *store.DB) *doctorAttachments {
	ctx := context.Background()
	total, err := db.CountAttachments(ctx)
	if err != nil {
		return nil
	}
	out := &doctorAttachments{Mirrored: total}
	if cfg, err := config.Load(); err == nil && cfg != nil {
		out.PerFileCapMB = cfg.AttachmentMaxMB
	}
	dir, err := config.AttachmentDir()
	if err != nil {
		return out
	}
	if _, err := os.Stat(dir); err != nil {
		// No directory yet means nothing has been cached — not an error.
		return out
	}
	out.CacheDir = tildeHome(dir)
	cache, err := attachcache.New(dir, 0, 0)
	if err != nil {
		return out
	}
	files, bytes := cache.Stats()
	out.Cached, out.CachedBytes = files, bytes
	return out
}

// formatDoctorAttachments is the one-line answer to "are this workspace's
// attachment bytes local yet?".
func formatDoctorAttachments(a doctorAttachments) string {
	s := fmt.Sprintf("%d mirrored, %d cached (%s)", a.Mirrored, a.Cached, store.HumanBytes(a.CachedBytes))
	if a.PerFileCapMB > 0 {
		s += fmt.Sprintf(", per-file cap %d MB", a.PerFileCapMB)
	}
	if a.CacheDir != "" {
		s += " at " + a.CacheDir
	}
	return s
}

// collectDevLinks reports whether this workspace mirrors the development
// panel, and how much of it landed (GDK-1496). The flag comes from
// config.MirrorsDevLinks — the same owner sync consults — so doctor can
// never claim a state the sync does not act on.
func collectDevLinks(db *store.DB) *doctorDevLinks {
	rows, issues, err := db.CountDevLinks(context.Background())
	if err != nil {
		return nil
	}
	out := &doctorDevLinks{Rows: rows, Issues: issues}
	if cfg, err := config.Load(); err == nil && cfg != nil {
		out.Mirrored = cfg.MirrorsDevLinks()
		out.Origin = cfg.OriginType()
	}
	return out
}

// formatDoctorDevLinks is the one line that separates "no pull requests" from
// "this workspace never asked" — the GDK-1496 confusion.
func formatDoctorDevLinks(d doctorDevLinks) string {
	if !d.Mirrored {
		s := "not synced"
		if d.Origin == config.OriginLinear {
			s += " (Linear has no development panel)"
		} else if d.Origin != "" {
			s += " (devStatus off; `gadak config set devStatus true`)"
		}
		if d.Rows > 0 {
			s += fmt.Sprintf("; %d stale rows over %d issues", d.Rows, d.Issues)
		}
		return s
	}
	return fmt.Sprintf("synced, %d rows over %d issues", d.Rows, d.Issues)
}
