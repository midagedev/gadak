package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/scry/internal/attachcache"
	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

/* ── response shapes ──
 * Field names follow web/src/lib/types.ts, which is what the client actually
 * parses and stores verbatim in IndexedDB. Where contracts/api.md and the
 * client disagree the client wins; the divergences are listed in
 * contracts/api.md.
 */

// issueLite is the stored row plus the fields only configuration can supply.
type issueLite struct {
	store.IssueLite
	// TeamGroup is omitted entirely when no group taxonomy is configured, which
	// leaves the client's group surfaces empty rather than wrong.
	TeamGroup *string `json:"team_group,omitempty"`
	// extra carries the plugin enrichment fields, already keyed by their client
	// names (see derivedView.enrichRow).
	extra map[string]any
}

// MarshalJSON flattens the configured field aliases and the plugin enrichments
// into the row. The client reads `severity`, `deploy_status` and friends as
// top-level keys and encoding/json cannot inline a map, so the two objects are
// spliced. The extras come first on purpose: a stored field of the same name is
// later in the object and therefore wins in the client's JSON.parse.
func (l issueLite) MarshalJSON() ([]byte, error) {
	extra := make(map[string]any, len(l.Custom)+len(l.extra)+1)
	for k, v := range l.Custom {
		extra[k] = v
	}
	for k, v := range l.extra {
		extra[k] = v
	}
	if l.TeamGroup != nil {
		extra["team_group"] = *l.TeamGroup
	}
	l.IssueLite.Custom = nil // spread, never nested
	stored, err := json.Marshal(l.IssueLite)
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 || len(stored) < 2 || stored[0] != '{' {
		return stored, nil
	}
	head, err := json.Marshal(extra)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(head)+len(stored))
	out = append(out, head[:len(head)-1]...) // drop the closing brace
	out = append(out, ',')
	return append(out, stored[1:]...), nil
}

type member struct {
	Email       string  `json:"email"`
	Name        string  `json:"name"`
	DisplayName *string `json:"display_name"`
	// ProfileImage is the field the client's Avatar reads; AvatarURL is the name
	// contracts/api.md uses. Same value, both spellings.
	ProfileImage  *string `json:"profile_image"`
	AvatarURL     *string `json:"avatar_url"`
	Department    *string `json:"department"`
	JobRole       *string `json:"job_role"`
	Group         *string `json:"group"`
	Status        *string `json:"status"`
	JiraAccountID *string `json:"jira_account_id"`
}

type syncSource struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Status   string  `json:"status"`
	SyncedAt *string `json:"synced_at"`
	Message  string  `json:"message"`
}

type syncHealth struct {
	Overall   string       `json:"overall"`
	CheckedAt string       `json:"checked_at"`
	Sources   []syncSource `json:"sources"`
}

// fieldSpecOut is the client's view of one configured custom field. IDs stay
// server-side; the client keys everything by alias.
type fieldSpecOut struct {
	Alias string `json:"alias"`
	Label string `json:"label"`
	Role  string `json:"role"`
	Kind  string `json:"kind,omitempty"`
}

type bootstrapResponse struct {
	ServerTime     string      `json:"server_time"`
	SyncVersion    int64       `json:"sync_version"`
	Members        []member    `json:"members"`
	MembersVersion string      `json:"members_version"`
	Issues         []issueLite `json:"issues"`
	SyncHealth     syncHealth  `json:"sync_health"`
	// FieldSpecs and FieldUsage drive the dynamic field surfaces: which rows a
	// detail panel shows, which filter axes exist, per current project scope.
	FieldSpecs []fieldSpecOut            `json:"field_specs"`
	FieldUsage map[string]map[string]int `json:"field_usage"`
	// LatestVersion / ReleaseURL are set only when a cached GitHub release is
	// newer than the running build (server.Version). Absent otherwise.
	LatestVersion string `json:"latest_version,omitempty"`
	ReleaseURL    string `json:"release_url,omitempty"`
}

type deltaResponse struct {
	ServerTime     string      `json:"server_time"`
	Upserted       []issueLite `json:"upserted"`
	DeletedKeys    []string    `json:"deleted_keys"`
	Members        []member    `json:"members"`
	MembersVersion string      `json:"members_version"`
	SyncHealth     syncHealth  `json:"sync_health"`
	// Specs ride the delta too: a long-lived tab that only ever polls delta
	// must still learn about a discovery that ran after its bootstrap.
	FieldSpecs []fieldSpecOut            `json:"field_specs"`
	FieldUsage map[string]map[string]int `json:"field_usage"`
}

/* ── bootstrap / delta ── */

func (s *server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	st, err := s.db.SyncState(r.Context(), sourceID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(st.Version))
	if etagMatches(r.Header.Get("If-None-Match"), st.Version) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	lites, err := s.db.IssueLites(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	view, err := s.derived(r.Context(), st.Version, lites)
	if err != nil {
		serverError(w, r, err)
		return
	}
	latest, releaseURL := s.bootstrapUpdate()
	writeJSON(w, http.StatusOK, bootstrapResponse{
		ServerTime:     store.Now(),
		SyncVersion:    st.Version,
		Members:        view.members,
		MembersVersion: view.membersVersion,
		Issues:         view.issues(lites),
		SyncHealth:     s.health(r.Context(), st),
		FieldSpecs:     s.fieldSpecsOut(),
		FieldUsage:     s.fieldUsageOut(r.Context()),
		LatestVersion:  latest,
		ReleaseURL:     releaseURL,
	})
}

// fieldSpecsOut projects the effective field specs for the client. Never nil —
// the client treats the empty list as "no custom fields configured".
func (s *server) fieldSpecsOut() []fieldSpecOut {
	specs := s.config().FieldSpecs()
	out := make([]fieldSpecOut, 0, len(specs))
	for _, sp := range specs {
		if sp.Alias == "" {
			continue
		}
		label := sp.Label
		if label == "" {
			label = sp.Alias
		}
		role := sp.Role
		if role == "" {
			role = "facet"
		}
		out = append(out, fieldSpecOut{Alias: sp.Alias, Label: label, Role: role, Kind: sp.Kind})
	}
	return out
}

// fieldUsageOut is project → alias → filled from the field_usage table. Never
// nil. A missing table row means the alias was never seen in that project.
func (s *server) fieldUsageOut(ctx context.Context) map[string]map[string]int {
	out := map[string]map[string]int{}
	rows, err := s.db.FieldUsage(ctx)
	if err != nil {
		return out
	}
	for _, r := range rows {
		m := out[r.ProjectKey]
		if m == nil {
			m = map[string]int{}
			out[r.ProjectKey] = m
		}
		m[r.Alias] = r.Filled
	}
	return out
}

func (s *server) handleDelta(w http.ResponseWriter, r *http.Request) {
	st, err := s.db.SyncState(r.Context(), sourceID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	since := r.URL.Query().Get("since")
	upserted, err := s.db.IssueLitesSince(r.Context(), since)
	if err != nil {
		serverError(w, r, err)
		return
	}
	deleted, err := s.db.DeletedKeysSince(r.Context(), since)
	if err != nil {
		serverError(w, r, err)
		return
	}
	view, err := s.derived(r.Context(), st.Version, nil)
	if err != nil {
		serverError(w, r, err)
		return
	}
	res := deltaResponse{
		ServerTime:     store.Now(),
		Upserted:       view.issues(upserted),
		DeletedKeys:    deleted,
		MembersVersion: view.membersVersion,
		SyncHealth:     s.health(r.Context(), st),
		FieldSpecs:     s.fieldSpecsOut(),
		FieldUsage:     s.fieldUsageOut(r.Context()),
	}
	// members ride along only when the client's hash is stale.
	if r.URL.Query().Get("mv") != view.membersVersion {
		res.Members = view.members
	}
	writeJSON(w, http.StatusOK, res)
}

// etagMatches accepts any `"<prefix>-<version>"` tag, not just the `"sv-"` one
// this server sends: the client's cache-hydration path invents `"in-<version>"`
// when it has a stored sync_version but no stored ETag, and answering 304 there
// saves a full re-hydration.
func etagMatches(header string, version int64) bool {
	if header == "" {
		return false
	}
	suffix := fmt.Sprintf("-%d\"", version)
	for _, tag := range strings.Split(header, ",") {
		if strings.HasSuffix(strings.TrimSpace(tag), suffix) {
			return true
		}
	}
	return false
}

/* ── detail ── */

type detailComment struct {
	CommentID       string          `json:"comment_id"`
	Author          *string         `json:"author"`
	AuthorEmail     *string         `json:"author_email"`
	AuthorAccountID *string         `json:"author_account_id"`
	Body            string          `json:"body"`
	RawBody         json.RawMessage `json:"raw_body"`
	CreatedAt       *string         `json:"created_at"`
}

type detailAttachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
	// MediaID is the ADF media node id, which the mirror does not carry. Empty
	// makes the client's ADF renderer fall back to matching on filename.
	MediaID         string  `json:"media_id"`
	MediaCollection string  `json:"media_collection"`
	IsImage         bool    `json:"is_image"`
	IsVideo         bool    `json:"is_video"`
	CacheStatus     string  `json:"cache_status"`
	CreatedAt       *string `json:"created_at"`
	ContentURL      string  `json:"content_url"`
}

type historyEntry struct {
	At    *string `json:"at"`
	Field string  `json:"field"`
	From  *string `json:"from"`
	To    *string `json:"to"`
	By    *string `json:"by"`
	// Categories are resolved from status ids, never from localized names.
	FromCategory *string `json:"from_category"`
	ToCategory   *string `json:"to_category"`
}

type linkedIssue struct {
	Key            string  `json:"key"`
	Type           string  `json:"type"`
	Direction      string  `json:"direction"`
	Summary        *string `json:"summary"`
	StatusCategory *string `json:"status_category"`
}

type detailResponse struct {
	IssueKey       string             `json:"issue_key"`
	DescriptionADF json.RawMessage    `json:"description_adf"`
	Attachments    []detailAttachment `json:"attachments"`
	Comments       []detailComment    `json:"comments"`
	History        []historyEntry     `json:"history"`
	LinkedIssues   []linkedIssue      `json:"linked_issues"`
	// RefPages / BacklinkPages are text-derived wiki cross-refs (item_refs).
	// Empty lists omit (omitempty), matching optional detail enrichments style.
	RefPages      []store.PageLite `json:"ref_pages,omitempty"`
	BacklinkPages []store.PageLite `json:"backlink_pages,omitempty"`
	// The four below come from plugin enrichments (docs/PLUGINS.md). With no
	// plugin writing them they stay null / [], which is what the client's
	// optional-field guards expect.
	DevelopmentOpinion json.RawMessage `json:"development_opinion"`
	QaContext          json.RawMessage `json:"qa_context"`
	Deploy             json.RawMessage `json:"deploy"`
	LinkedPRs          json.RawMessage `json:"linked_prs"`
	// Bodies carries the body-role custom field values (ADF documents), keyed
	// by alias. List rows strip these; the detail panel renders them as blocks.
	Bodies map[string]json.RawMessage `json:"bodies"`
}

func (s *server) handleDetail(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	d, err := s.db.Detail(r.Context(), key)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	st, err := s.db.SyncState(r.Context(), sourceID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	view, err := s.derived(r.Context(), st.Version, nil)
	if err != nil {
		serverError(w, r, err)
		return
	}

	res := detailResponse{
		IssueKey:       d.IssueKey,
		DescriptionADF: d.DescriptionADF,
		Attachments:    make([]detailAttachment, 0, len(d.Attachments)),
		Comments:       make([]detailComment, 0, len(d.Comments)),
		History:        make([]historyEntry, 0, len(d.History)),
		LinkedIssues:   make([]linkedIssue, 0, len(d.LinkedIssues)),
		RefPages:       d.RefPages,
		BacklinkPages:  d.BacklinkPages,
		LinkedPRs:      json.RawMessage("[]"),
		Bodies:         map[string]json.RawMessage{},
	}
	for alias := range view.bodyAliases {
		val, ok := d.Custom[alias]
		if !ok || val == nil {
			continue
		}
		if raw, err := json.Marshal(val); err == nil {
			res.Bodies[alias] = raw
		}
	}
	// The detail half of the plugin boundary: each kind maps to one response field.
	en, err := s.db.EnrichmentsFor(r.Context(), key)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if p := payload(en["deploy"]); p != nil {
		res.Deploy = pick(p, "detail")
	}
	if p := payload(en["qa"]); p != nil {
		res.QaContext = pick(p, "context")
	}
	if p := payload(en["prs"]); p != nil {
		res.LinkedPRs = p
	}
	if p := payload(en["opinion"]); p != nil {
		res.DevelopmentOpinion = p
	}
	for _, a := range d.Attachments {
		id := a.ExternalID
		if id == "" {
			id = a.ID
		}
		res.Attachments = append(res.Attachments, detailAttachment{
			ID:          id,
			Filename:    a.Filename,
			MimeType:    a.MimeType,
			Size:        a.Size,
			IsImage:     strings.HasPrefix(a.MimeType, "image/"),
			IsVideo:     strings.HasPrefix(a.MimeType, "video/"),
			CacheStatus: s.cacheStatus(id),
			CreatedAt:   nilIfEmpty(a.CreatedAt),
			ContentURL:  attachmentURL(d.IssueKey, id),
		})
	}
	for _, c := range d.Comments {
		id := c.ExternalID
		if id == "" {
			id = c.ID
		}
		res.Comments = append(res.Comments, detailComment{
			CommentID:       id,
			Author:          nilIfEmpty(c.Author),
			AuthorEmail:     nilIfEmpty(view.emailByAccount[c.AuthorID]),
			AuthorAccountID: nilIfEmpty(c.AuthorID),
			Body:            c.Body, // the client's fallback when the ADF will not render
			RawBody:         c.BodyADF,
			CreatedAt:       nilIfEmpty(c.CreatedAt),
		})
	}
	for _, h := range d.History {
		e := historyEntry{
			At:    nilIfEmpty(h.At),
			Field: h.Field,
			From:  nilIfEmpty(h.FromValue),
			To:    nilIfEmpty(h.ToValue),
			By:    nilIfEmpty(h.Author),
		}
		if h.Field == "status" {
			e.FromCategory = nilIfEmpty(view.categories[h.FromID])
			e.ToCategory = nilIfEmpty(view.categories[h.ToID])
		}
		res.History = append(res.History, e)
	}
	for _, l := range d.LinkedIssues {
		res.LinkedIssues = append(res.LinkedIssues, linkedIssue{
			Key:            l.Key,
			Type:           l.Type,
			Direction:      l.Direction,
			Summary:        nilIfEmpty(l.Summary),
			StatusCategory: nilIfEmpty(l.StatusCategory),
		})
	}
	// Start pulling the images down while the client renders the rest of the
	// detail, so opening an issue with screenshots does not wait on Jira twice.
	s.warmAttachments(s.config(), res.Attachments)
	writeJSON(w, http.StatusOK, res)
}

// attachmentURL builds the one URL shape the client's ADF renderer accepts as an
// image source: `<apiBase><key>/attachments/<id>/content/`, unescaped. Changing
// it silently blocks every inline image (web/src/lib/adf.ts, safeMediaUrl).
func attachmentURL(key, id string) string {
	return apiBase + key + "/attachments/" + id + "/content/"
}

/* ── search ── */

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	res, err := s.db.Search(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		serverError(w, r, err)
		return
	}
	// keys/total keep pre-R2 meaning for issue clients; pages/matches are additive.
	// Always emit pages and matches (empty) so clients can rely on the keys.
	if res.Pages == nil {
		res.Pages = []store.PageLite{}
	}
	if res.Keys == nil {
		res.Keys = []string{}
	}
	if res.Matches == nil {
		res.Matches = map[string]store.SearchMatch{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"keys": res.Keys, "pages": res.Pages, "total": res.Total, "matches": res.Matches,
	})
}

/* ── Confluence pages (R2) ── */

func (s *server) handlePages(w http.ResponseWriter, r *http.Request) {
	st, err := s.db.SyncState(r.Context(), "confluence")
	if err != nil {
		serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(st.Version))
	if etagMatches(r.Header.Get("If-None-Match"), st.Version) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	pages, err := s.db.PageLites(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	if pages == nil {
		pages = []store.PageLite{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": pages, "total": len(pages)})
}

func (s *server) handlePageDetail(w http.ResponseWriter, r *http.Request) {
	s.handlePageDetailKey(w, r, r.PathValue("key"))
}

func (s *server) handlePageDetailKey(w http.ResponseWriter, r *http.Request, key string) {
	d, err := s.db.PageDetail(r.Context(), key)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if d == nil {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

/* ── attachment bytes: local cache first, Jira only on a miss ── */

// ponytail: a 2-minute ceiling on one attachment download, no resumption.
var proxyClient = &http.Client{Timeout: 2 * time.Minute}

// handleAttachment serves attachment bytes from the on-disk cache, falling back
// to Jira on a miss and caching what it fetches. Bytes for an attachment id are
// immutable in Jira, so a hit is served with a long-lived validator and a cached
// attachment keeps working with no credential at all — which is how the bundled
// demo snapshot shows real images offline.
func (s *server) handleAttachment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.cache != nil {
		if served := s.serveCached(w, r, id); served {
			return
		}
	}

	cfg := s.config()
	if !cfg.HasCredential() {
		fail(w, http.StatusConflict, "credential_required")
		return
	}

	// Fill the cache, then serve from it, so the bytes are written once and every
	// later view is local. A cache failure is not a request failure: fall through
	// to a straight stream.
	if s.cache != nil {
		err := s.cache.Fill(id, func() (io.ReadCloser, attachcache.Meta, error) {
			res, err := s.fetchAttachment(r.Context(), cfg, id)
			if err != nil {
				return nil, attachcache.Meta{}, err
			}
			return res.Body, attachcache.Meta{
				ContentType: contentTypeOf(res),
				Size:        res.ContentLength,
			}, nil
		})
		switch {
		case err == nil:
			if s.serveCached(w, r, id) {
				return
			}
		case errors.Is(err, errAttachmentAuth):
			fail(w, http.StatusConflict, "credential_rejected")
			return
		case errors.Is(err, errAttachmentMissing):
			fail(w, http.StatusNotFound, "not_found")
			return
		case attachcache.TooLarge(err):
			// Too big to keep; stream it through below.
		default:
			log.Printf("server: attachment cache fill: %v", err)
		}
	}

	res, err := s.fetchAttachment(r.Context(), cfg, id)
	switch {
	case errors.Is(err, errAttachmentAuth):
		fail(w, http.StatusConflict, "credential_rejected")
		return
	case errors.Is(err, errAttachmentMissing):
		fail(w, http.StatusNotFound, "not_found")
		return
	case err != nil:
		log.Printf("server: attachment proxy: %v", err)
		fail(w, http.StatusBadGateway, "attachment_unavailable")
		return
	}
	defer res.Body.Close()

	ct := contentTypeOf(res)
	w.Header().Set("Content-Type", ct)
	if cl := res.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	setAttachmentGuards(w, ct)
	if _, err := io.Copy(w, res.Body); err != nil {
		log.Printf("server: attachment stream: %v", err)
	}
}

// serveCached answers from disk. Reports whether it wrote a response.
func (s *server) serveCached(w http.ResponseWriter, r *http.Request, id string) bool {
	f, meta, err := s.cache.Get(id)
	if err != nil {
		return false
	}
	defer f.Close()
	w.Header().Set("Content-Type", meta.ContentType)
	// The bytes behind an attachment id never change, so the browser may keep
	// them for as long as it likes. This is what makes a second view instant.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", fmt.Sprintf("%q", "att-"+id))
	setAttachmentGuards(w, meta.ContentType)
	http.ServeContent(w, r, "", time.Time{}, f)
	return true
}

// cacheStatus is what the client shows next to an attachment: "ready" once the
// bytes are local, "pending" while they still have to come from Jira.
func (s *server) cacheStatus(id string) string {
	if s.cache == nil || !s.cache.Has(id) {
		return "pending"
	}
	return "ready"
}

// warmAttachments pre-downloads the inline-renderable attachments of an issue the
// user just opened, so the images are local before the browser asks for them.
// Bounded and fire-and-forget: a failure only means the proxy path handles it.
func (s *server) warmAttachments(cfg *config.Config, atts []detailAttachment) {
	if s.cache == nil || !cfg.HasCredential() {
		return
	}
	var pending []detailAttachment
	for _, a := range atts {
		if (a.IsImage || a.IsVideo) && !s.cache.Has(a.ID) {
			pending = append(pending, a)
		}
	}
	if len(pending) == 0 {
		return
	}
	// ponytail: at most four in flight and eight per open; raise it when someone
	// has an issue with dozens of screenshots and complains.
	const maxWarm, workers = 8, 4
	if len(pending) > maxWarm {
		pending = pending[:maxWarm]
	}
	jobs := make(chan detailAttachment)
	for i := 0; i < workers; i++ {
		go func() {
			for a := range jobs {
				// Detached from the request: the browser may have moved on already.
				id := a.ID
				if err := s.cache.Fill(id, func() (io.ReadCloser, attachcache.Meta, error) {
					res, err := s.fetchAttachment(context.Background(), cfg, id)
					if err != nil {
						return nil, attachcache.Meta{}, err
					}
					return res.Body, attachcache.Meta{
						ContentType: contentTypeOf(res),
						Size:        res.ContentLength,
						Filename:    a.Filename,
					}, nil
				}); err != nil && !attachcache.TooLarge(err) {
					log.Printf("server: attachment warm %s: %v", id, err)
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, a := range pending {
			jobs <- a
		}
	}()
}

var (
	errAttachmentAuth    = errors.New("attachment: credential rejected")
	errAttachmentMissing = errors.New("attachment: not found")
)

// fetchAttachment performs the one call that leaves this process.
func (s *server) fetchAttachment(ctx context.Context, cfg *config.Config, id string) (*http.Response, error) {
	target := strings.TrimRight(cfg.Site, "/") + "/rest/api/3/attachment/content/" + url.PathEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	// Jira answers with a redirect to a pre-signed media URL. Go drops the
	// Authorization header on a cross-host redirect, which is exactly right: the
	// token must not travel to the media host.
	req.SetBasicAuth(cfg.Email, cfg.Token)
	res, err := proxyClient.Do(req)
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
		res.Body.Close()
		return nil, errAttachmentAuth
	case res.StatusCode == http.StatusNotFound:
		res.Body.Close()
		return nil, errAttachmentMissing
	case res.StatusCode != http.StatusOK:
		res.Body.Close()
		return nil, fmt.Errorf("attachment: upstream status %d", res.StatusCode)
	}
	return res, nil
}

func contentTypeOf(res *http.Response) string {
	if ct := res.Header.Get("Content-Type"); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// setAttachmentGuards keeps attacker-controlled bytes from executing on this
// origin: nothing scriptable renders inline.
func setAttachmentGuards(w http.ResponseWriter, contentType string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !inlineSafe(contentType) {
		w.Header().Set("Content-Disposition", "attachment")
	}
}

// inlineSafe reports whether a type may render in the page. SVG is an image that
// executes script, so it is deliberately excluded.
func inlineSafe(contentType string) bool {
	mime := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	if mime == "image/svg+xml" {
		return false
	}
	return strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "video/") ||
		strings.HasPrefix(mime, "audio/") || mime == "application/pdf"
}

/* ── derived view (members, group index, status categories) ── */

// derivedView is what bootstrap, delta and detail all need but the store does
// not hold: the member directory and its hash, the account-id → email index the
// detail panel resolves comment authors through, and the status id → category
// map history entries are annotated from. Building it scans the mirror, so it is
// cached until either the sync version or the configuration moves.
type derivedView struct {
	key            string
	members        []member
	membersVersion string
	emailByAccount map[string]string
	categories     map[string]string
	groupByEmail   map[string]string
	rules          []config.GroupRule
	groupsEnabled  bool
	// Plugin enrichments the list rows carry, by issue key. They are cached with
	// everything else here, which is exactly as fresh as the ETag: a plugin that
	// forgets to bump sync_state.version gets no refetch either way (docs/PLUGINS.md).
	deploy map[string]json.RawMessage
	qa     map[string]json.RawMessage
	// bodyAliases are the field aliases whose values are documents (role=body).
	// List rows strip them — a 60-issue page must not carry sixty ADF bodies —
	// and the detail response is where they surface.
	bodyAliases map[string]bool
}

// derived returns the cached view for this sync version, rebuilding it when
// stale. lites may be nil, in which case it scans; bootstrap passes the rows it
// already read so the mirror is scanned once per request, not twice.
func (s *server) derived(ctx context.Context, version int64, lites []store.IssueLite) (*derivedView, error) {
	key := fmt.Sprintf("%d:%d", version, s.gen.Load())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && s.cached.key == key {
		return s.cached, nil
	}
	if lites == nil {
		var err error
		if lites, err = s.db.IssueLites(ctx); err != nil {
			return nil, err
		}
	}
	v, err := s.buildView(ctx, key, lites)
	if err != nil {
		return nil, err
	}
	s.cached = v
	return v, nil
}

func (s *server) buildView(ctx context.Context, key string, lites []store.IssueLite) (*derivedView, error) {
	cfg := s.config()
	v := &derivedView{
		key:            key,
		emailByAccount: map[string]string{},
		categories:     map[string]string{},
		groupByEmail:   map[string]string{},
		rules:          cfg.GroupRules,
		bodyAliases:    map[string]bool{},
	}
	for _, sp := range cfg.FieldSpecs() {
		if sp.Role == "body" && sp.Alias != "" {
			v.bodyAliases[sp.Alias] = true
		}
	}
	var err error
	if v.deploy, err = s.db.EnrichmentsByKind(ctx, "deploy"); err != nil {
		return nil, err
	}
	if v.qa, err = s.db.EnrichmentsByKind(ctx, "qa"); err != nil {
		return nil, err
	}
	byEmail := map[string]*member{}
	for i := range lites {
		l := &lites[i]
		if l.StatusID != "" && l.StatusCategory != "" {
			v.categories[l.StatusID] = l.StatusCategory
		}
		// Only assignees carry an email in the mirror, so only they can seed the
		// directory: members are keyed by email on the client.
		email := deref(l.AssigneeEmail)
		if email == "" {
			continue
		}
		m := byEmail[email]
		if m == nil {
			m = &member{Email: email}
			byEmail[email] = m
		}
		if m.Name == "" {
			m.Name = deref(l.Assignee)
		}
		if m.JiraAccountID == nil {
			m.JiraAccountID = nilIfEmpty(deref(l.AssigneeID))
		}
	}
	// Configuration wins: group, department, job role and avatar exist nowhere
	// else.
	for _, cm := range cfg.Members {
		if cm.Email == "" {
			continue
		}
		m := byEmail[cm.Email]
		if m == nil {
			m = &member{Email: cm.Email}
			byEmail[cm.Email] = m
		}
		if cm.Name != "" {
			m.Name = cm.Name
		} else if m.Name == "" {
			m.Name = cm.DisplayName
		}
		m.DisplayName = nilIfEmpty(cm.DisplayName)
		m.Department = nilIfEmpty(cm.Department)
		m.JobRole = nilIfEmpty(cm.JobRole)
		m.Group = nilIfEmpty(cm.Group)
		m.ProfileImage = nilIfEmpty(cm.AvatarURL)
		m.AvatarURL = m.ProfileImage
		if cm.JiraAccountID != "" {
			m.JiraAccountID = nilIfEmpty(cm.JiraAccountID)
		}
		if cm.Group != "" {
			v.groupByEmail[cm.Email] = cm.Group
			v.groupsEnabled = true
		}
	}
	v.groupsEnabled = v.groupsEnabled || len(cfg.GroupRules) > 0

	v.members = make([]member, 0, len(byEmail))
	for _, m := range byEmail {
		if m.JiraAccountID != nil {
			v.emailByAccount[*m.JiraAccountID] = m.Email
		}
		v.members = append(v.members, *m)
	}
	sort.Slice(v.members, func(i, j int) bool { return v.members[i].Email < v.members[j].Email })
	v.membersVersion = hashMembers(v.members)
	return v, nil
}

// hashMembers is the token the client returns as `mv`; equal hashes let delta
// skip the member payload.
func hashMembers(members []member) string {
	b, err := json.Marshal(members)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (v *derivedView) issues(lites []store.IssueLite) []issueLite {
	out := make([]issueLite, 0, len(lites))
	for _, l := range lites {
		// Body-role values are documents; the list row drops them and the
		// detail response carries them instead.
		if len(v.bodyAliases) > 0 && len(l.Custom) > 0 {
			for alias := range v.bodyAliases {
				delete(l.Custom, alias)
			}
		}
		out = append(out, issueLite{IssueLite: l, TeamGroup: v.group(l), extra: v.enrichRow(l.IssueKey)})
	}
	return out
}

// enrichRow is the list half of the plugin boundary: the `deploy` payload's
// status object becomes `deploy_status`, and the `qa` payload's impact object is
// spread as the `qa_impact_*` / `qa_runs` / `qa_suites` fields the client filters
// on. A plugin cannot shadow a mirrored field this way — enrichment keys are
// serialized before the stored ones, so the stored value is what survives
// JSON.parse.
func (v *derivedView) enrichRow(key string) map[string]any {
	if len(v.deploy) == 0 && len(v.qa) == 0 {
		return nil
	}
	extra := map[string]any{}
	if p := payload(v.deploy[key]); p != nil {
		extra["deploy_status"] = pick(p, "status")
	}
	if p := payload(v.qa[key]); p != nil {
		var impact map[string]json.RawMessage
		if json.Unmarshal(pick(p, "impact"), &impact) == nil {
			for k, val := range impact {
				extra[k] = val
			}
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

// payload guards the one place this server copies bytes written by another
// process straight into a response: invalid JSON would corrupt the whole
// document, so it is dropped instead.
func payload(p json.RawMessage) json.RawMessage {
	if len(p) == 0 || !json.Valid(p) {
		return nil
	}
	return p
}

// pick unwraps `{"status": …}` / `{"impact": …}` / `{"detail": …}` / `{"context": …}`
// and passes anything else through whole, so a plugin that writes the bare
// object documented in data-model.md still works.
func pick(p json.RawMessage, field string) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(p, &m) == nil {
		if v, ok := m[field]; ok {
			return v
		}
	}
	return p
}

// group classifies one issue: the first matching rule wins, otherwise the
// assignee's configured group. Nothing configured means the field is omitted
// rather than sent as null, which keeps the client's group surfaces empty
// instead of showing a bogus bucket.
func (v *derivedView) group(l store.IssueLite) *string {
	if !v.groupsEnabled {
		return nil
	}
	for _, r := range v.rules {
		if ruleMatches(r, l) {
			return nilIfEmpty(r.Group)
		}
	}
	return nilIfEmpty(v.groupByEmail[deref(l.AssigneeEmail)])
}

// ruleMatches: conditions are ANDed, values inside one condition are ORed, an
// empty condition is always true.
func ruleMatches(r config.GroupRule, l store.IssueLite) bool {
	if len(r.Projects) > 0 && !contains(r.Projects, l.ProjectKey) {
		return false
	}
	if len(r.Labels) > 0 && !overlaps(r.Labels, l.Labels) {
		return false
	}
	if len(r.Components) > 0 && !overlaps(r.Components, l.Components) {
		return false
	}
	return true
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func overlaps(want, have []string) bool {
	for _, v := range have {
		if contains(want, v) {
			return true
		}
	}
	return false
}

/* ── sync health ── */

// health maps the store's sync bookkeeping onto the shape the sidebar renders.
// The message is server text in one language — English, like every other string
// this process emits — and the client localizes only the status label. "ok" is
// not decoration: the client suppresses the tooltip line for exactly that value
// (web/src/components/sidebar/SidebarNav.svelte).
//
// Staleness is measured from the last run that finished without an error, never
// from the watermark: a quiet project leaves its watermark in the past forever
// and would read as permanently delayed.
//
// st is the Jira sync_state (bootstrap/delta still pass only that). Confluence
// is appended when a sources row exists — bootstrap/delta payloads stay issue-
// only and their ETags stay jira-version-only.
func (s *server) health(ctx context.Context, st store.SyncState) syncHealth {
	sources := []syncSource{s.sourceHealth("jira", "Jira", st)}
	if ok, err := s.db.HasSource(ctx, "confluence"); err == nil && ok {
		if cst, err := s.db.SyncState(ctx, "confluence"); err == nil {
			sources = append(sources, s.sourceHealth("confluence", "Confluence", cst))
		}
	}
	overall := "healthy"
	for _, src := range sources {
		switch src.Status {
		case "failed":
			overall = "failed"
		case "missing", "stale":
			if overall == "healthy" {
				overall = "warning"
			}
		}
	}
	return syncHealth{Overall: overall, CheckedAt: store.Now(), Sources: sources}
}

func (s *server) sourceHealth(key, label string, st store.SyncState) syncSource {
	src := syncSource{Key: key, Label: label, Status: "healthy", Message: "ok", SyncedAt: st.SyncedAt}
	switch {
	case st.LastError != nil && *st.LastError != "":
		src.Status, src.Message = "failed", *st.LastError
	case st.SyncedAt == nil && st.Watermark == "" && st.LastFullSyncAt == nil:
		src.Status, src.Message = "missing", "not synced yet"
	case s.stale(st.SyncedAt):
		src.Status, src.Message = "stale", "last sync is behind"
	}
	if src.SyncedAt == nil {
		src.SyncedAt = st.LastFullSyncAt
	}
	return src
}

// stale allows a wide margin — ten missed incremental runs — because this only
// has to catch a sync loop that died without recording an error.
func (s *server) stale(syncedAt *string) bool {
	if syncedAt == nil {
		return false
	}
	at, err := time.Parse(time.RFC3339, *syncedAt)
	if err != nil {
		return false
	}
	interval := time.Duration(s.config().SyncIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	return time.Since(at) > 10*interval
}
