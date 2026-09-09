package atlhttp

import (
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// Request kinds for the per-pass request breakdown. One
// classifier maps every origin dialect — Jira Cloud v3, Jira Server v2,
// Confluence Cloud v1, and the built-in tracker, which speaks the same
// shapes in-process — onto these, so the sync pass line is comparable
// across origins. kindOrder is the canonical print order; every tally,
// snapshot, and line iterates it, never a map.
const (
	KindSearchPages    = "search-pages"
	KindIssueComment   = "issue-comment"
	KindIssueChangelog = "issue-changelog"
	KindPageList       = "page-list"
	KindPageBody       = "page-body"
	KindPageComments   = "page-comments"
	KindPageVersions   = "page-versions"
	KindAgile          = "agile"
	KindOther          = "other"
)

var kindOrder = [...]string{
	KindSearchPages, KindIssueComment, KindIssueChangelog, KindPageList,
	KindPageBody, KindPageComments, KindPageVersions, KindAgile, KindOther,
}

// kindRule is one row of the classifier table: a request whose method and
// path match classifies as the rule's kind. First match wins; the table is
// the single owner of "what kind of request was that", which is why the
// rows name the concrete routes each dialect really serves rather than a
// prefix guess. Reads (GET) are separated from writes: a POST to a comment
// route is a write, not part of the mirror's comment-read cost.
type kindRule struct {
	kind    string
	methods string // "" = any method; else "|"-separated uppercase names
	path    *regexp.Regexp
}

var kindRules = []kindRule{
	// The Agile API sits outside /rest/api on every dialect that has it.
	{KindAgile, "", regexp.MustCompile(`^/rest/agile/`)},
	// Confluence content tree (Cloud v1 and the built-in tracker's wiki
	// shape). Longer paths first: page-body's [^/]+$ would otherwise eat
	// the search route.
	{KindPageComments, http.MethodGet, regexp.MustCompile(`^/rest/api/content/[^/]+/child/comment`)},
	{KindPageVersions, http.MethodGet, regexp.MustCompile(`^/rest/api/content/[^/]+/version`)},
	{KindPageList, http.MethodGet, regexp.MustCompile(`^(/rest/api/content/search|/api/v2/pages)`)},
	{KindPageBody, http.MethodGet, regexp.MustCompile(`^/rest/api/content/[^/]+$`)},
	// Jira per-issue reads. Cloud serves /rest/api/3, Server /rest/api/2.
	{KindIssueComment, http.MethodGet, regexp.MustCompile(`^/rest/api/[23]/issue/[^/]+/comment`)},
	{KindIssueChangelog, http.MethodGet, regexp.MustCompile(`^/rest/api/[23]/issue/[^/]+/changelog`)},
	// The JQL search family: Cloud's tokened POST /search/jql, Server's
	// classic POST /search (and its GET form), and the approximate-count
	// Cloud serves for progress denominators — same endpoint family, same
	// rate cost.
	{KindSearchPages, http.MethodGet + "|" + http.MethodPost,
		regexp.MustCompile(`^/rest/api/[23]/search(/jql|/approximate-count)?$`)},
}

// kindIndexOf maps a kind to its slot in kindOrder; unknown kinds land in
// "other" so a renamed constant can never panic the request path.
func kindIndexOf(kind string) int {
	for i, k := range kindOrder {
		if k == kind {
			return i
		}
	}
	return len(kindOrder) - 1
}

// ClassifyRequest maps one outbound request onto a request kind. path is
// the site-relative path exactly as DoRaw receives it; query is ignored,
// and a leading /wiki mount prefix is tolerated for callers that pass the
// routed path (the Confluence client carries /wiki in its base, not its
// paths).
func ClassifyRequest(method, path string) string {
	p := path
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimPrefix(p, "/wiki")
	for _, r := range kindRules {
		if r.methods == "" || strings.Contains("|"+r.methods+"|", "|"+method+"|") {
			if r.path.MatchString(p) {
				return r.kind
			}
		}
	}
	return KindOther
}

// KindTally is one row of a breakdown snapshot.
type KindTally struct {
	Kind   string `json:"kind"`
	Count  int64  `json:"count"`
	WallMS int64  `json:"wall_ms"`
}

// BreakdownSnapshot is the take-once view of a Breakdown: per-kind rows in
// canonical order (zero-count kinds omitted), their total, and the
// politeness-sleep total.
type BreakdownSnapshot struct {
	Kinds   []KindTally `json:"kinds"`
	Total   int64       `json:"total"`
	WallMS  int64       `json:"wall_ms"`
	SleepMS int64       `json:"sleep_ms,omitempty"`
}

// Breakdown tallies per-kind request count and wall time for one client —
// the "sync: requests …" pass line. It also carries the
// Confluence client's PauseBetween sleep, which no request meter sees and
// which was 45.7 s of one first-sync benchmark. Same concurrency shape as
// Meter: atomics, never blocks a request, nil-safe.
type Breakdown struct {
	counts  [len(kindOrder)]atomic.Int64
	wallNS  [len(kindOrder)]atomic.Int64
	sleepMS atomic.Int64
}

// Note records one completed attempt of method+path with the wall time the
// caller spent on it. The count is noted even when d is zero — the request
// still left the process.
func (b *Breakdown) Note(method, path string, d time.Duration) {
	if b == nil {
		return
	}
	i := kindIndexOf(ClassifyRequest(method, path))
	b.counts[i].Add(1)
	if d > 0 {
		b.wallNS[i].Add(int64(d))
	}
}

// NoteSleep records time spent in the Confluence client's PauseBetween
// politeness sleep. Non-positive durations are ignored, like Meter.NoteWait.
func (b *Breakdown) NoteSleep(d time.Duration) {
	if b == nil || d <= 0 {
		return
	}
	b.sleepMS.Add(d.Milliseconds())
}

// Take returns the snapshot and zeroes the accumulators — the same
// accumulate-once contract as Meter.Take, so a per-pass printer cannot
// double-count the next pass.
func (b *Breakdown) Take() BreakdownSnapshot {
	if b == nil {
		return BreakdownSnapshot{}
	}
	var out BreakdownSnapshot
	for i, kind := range kindOrder {
		n := b.counts[i].Swap(0)
		ns := b.wallNS[i].Swap(0)
		if n == 0 {
			continue
		}
		row := KindTally{Kind: kind, Count: n, WallMS: ns / int64(time.Millisecond)}
		out.Kinds = append(out.Kinds, row)
		out.Total += n
		out.WallMS += row.WallMS
	}
	out.SleepMS = b.sleepMS.Swap(0)
	return out
}
