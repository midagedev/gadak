package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"hash/fnv"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Feed window and default page size match the client contract
// (web/src/lib/api.ts getFeed / FeedFocus).
const (
	FeedWindowDays   = 30
	FeedDefaultLimit = 80
	FeedMaxLimit     = 200
	feedExcerptRunes = 120
)

// FeedFocus is the focus tab the client sends: all | assignee | reporter | mention.
// "watched" is a reason, not a focus filter.
type FeedFocus string

const (
	FeedFocusAll      FeedFocus = "all"
	FeedFocusAssignee FeedFocus = "assignee"
	FeedFocusReporter FeedFocus = "reporter"
	FeedFocusMention  FeedFocus = "mention"
)

// FeedIdentity is the local user, used for relevance and self-action exclusion.
// AccountID comes from config (Jira /myself); Email/DisplayName fall back when
// the mirror still only has display names on some rows.
type FeedIdentity struct {
	AccountID   string
	Email       string
	DisplayName string // TokenOwner / display name for author matching
}

// FeedOpts configures a personal-feed query.
type FeedOpts struct {
	Focus FeedFocus
	Limit int
	Me    FeedIdentity
	// Now, when set, freezes the 30-day window for tests.
	Now time.Time
}

// FeedItem is one activity row the client renders (web/src/lib/types.ts FeedItem).
type FeedItem struct {
	ID            int            `json:"id"`
	EventID       string         `json:"event_id"`
	IssueKey      string         `json:"issue_key"`
	Summary       string         `json:"summary"`
	CurrentStatus string         `json:"current_status"`
	EventType     string         `json:"event_type"`
	OccurredAt    *string        `json:"occurred_at"`
	ActorName     string         `json:"actor_name"`
	Payload       map[string]any `json:"payload"`
	Reasons       []string       `json:"reasons"`
	ReadAt        *string        `json:"read_at"`
}

// FeedUnreadCounts is the badge counts per focus tab.
type FeedUnreadCounts struct {
	All      int `json:"all"`
	Assignee int `json:"assignee"`
	Reporter int `json:"reporter"`
	Mention  int `json:"mention"`
}

// FeedResult is GET feed/ response body (without the outer wrapper keys).
type FeedResult struct {
	Items        []FeedItem       `json:"items"`
	UnreadCounts FeedUnreadCounts `json:"unread_counts"`
}

// MarkFeedReadOpts is POST feed/read/ body semantics.
type MarkFeedReadOpts struct {
	EventIDs  []string
	IssueKeys []string
	All       bool
	Me        FeedIdentity
	Now       time.Time
}

// MarkFeedReadResult is POST feed/read/ response.
type MarkFeedReadResult struct {
	Updated      int              `json:"updated"`
	UnreadCounts FeedUnreadCounts `json:"unread_counts"`
}

// Feed computes personal-feed events from the mirror at query time.
func (db *DB) Feed(ctx context.Context, opts FeedOpts) (FeedResult, error) {
	events, err := db.computeFeedEvents(ctx, opts.Me, opts.Now)
	if err != nil {
		return FeedResult{}, err
	}
	unread := countUnread(events)
	focus := normalizeFocus(opts.Focus)
	filtered := filterFocus(events, focus)
	limit := opts.Limit
	if limit <= 0 {
		limit = FeedDefaultLimit
	}
	if limit > FeedMaxLimit {
		limit = FeedMaxLimit
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	// Stable numeric ids for Svelte {#each} keys — FNV of event_id.
	for i := range filtered {
		filtered[i].ID = eventNumericID(filtered[i].EventID)
		if filtered[i].Payload == nil {
			filtered[i].Payload = map[string]any{}
		}
		if filtered[i].Reasons == nil {
			filtered[i].Reasons = []string{}
		}
	}
	return FeedResult{Items: filtered, UnreadCounts: unread}, nil
}

// MarkFeedRead upserts read receipts and returns the refreshed unread counts.
func (db *DB) MarkFeedRead(ctx context.Context, opts MarkFeedReadOpts) (MarkFeedReadResult, error) {
	events, err := db.computeFeedEvents(ctx, opts.Me, opts.Now)
	if err != nil {
		return MarkFeedReadResult{}, err
	}
	want := map[string]bool{}
	switch {
	case opts.All:
		for _, e := range events {
			want[e.EventID] = true
		}
	default:
		for _, id := range opts.EventIDs {
			if id != "" {
				want[id] = true
			}
		}
		if len(opts.IssueKeys) > 0 {
			keys := map[string]bool{}
			for _, k := range opts.IssueKeys {
				if k != "" {
					keys[k] = true
				}
			}
			for _, e := range events {
				if keys[e.IssueKey] {
					want[e.EventID] = true
				}
			}
		}
	}
	if len(want) == 0 {
		return MarkFeedReadResult{Updated: 0, UnreadCounts: countUnread(events)}, nil
	}
	now := Now()
	if !opts.Now.IsZero() {
		now = opts.Now.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	updated := 0
	err = db.write(ctx, func(tx *sql.Tx) error {
		for id := range want {
			res, err := tx.Exec(`
				INSERT INTO feed_reads (event_id, read_at) VALUES (?, ?)
				ON CONFLICT(event_id) DO UPDATE SET read_at = excluded.read_at`,
				id, now)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n > 0 {
				updated++
			}
		}
		return nil
	})
	if err != nil {
		return MarkFeedReadResult{}, err
	}
	// Recompute so read_at stamps and unread counts match the write we just did.
	events, err = db.computeFeedEvents(ctx, opts.Me, opts.Now)
	if err != nil {
		return MarkFeedReadResult{}, err
	}
	return MarkFeedReadResult{Updated: updated, UnreadCounts: countUnread(events)}, nil
}

// ── internals ──────────────────────────────────────────────────────────────

type issueMeta struct {
	ItemID        string
	Key           string
	Title         string
	Status        string
	AssigneeID    string
	AssigneeEmail string
	ReporterID    string
	ReporterEmail string
	CreatedAt     string
	ReopenedAt    string
	Author        string
	AuthorID      string
}

type rawChange struct {
	ID, ItemID, At, Author, AuthorID, Field, FromValue, ToValue string
}

type rawComment struct {
	ID, ItemID, Author, AuthorID, BodyADF, BodyText, CreatedAt string
}

type rawAttach struct {
	ID, ItemID, Filename, Author, AuthorID, CreatedAt string
}

func (db *DB) computeFeedEvents(ctx context.Context, me FeedIdentity, now time.Time) ([]FeedItem, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	since := now.AddDate(0, 0, -FeedWindowDays).Format("2006-01-02T15:04:05.000Z")

	watches, err := db.watchSet(ctx)
	if err != nil {
		return nil, err
	}
	issues, err := db.loadIssueMeta(ctx)
	if err != nil {
		return nil, err
	}
	changes, err := db.loadChangelogSince(ctx, since)
	if err != nil {
		return nil, err
	}
	comments, err := db.loadCommentsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	attaches, err := db.loadAttachmentsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	reads, err := db.loadFeedReads(ctx)
	if err != nil {
		return nil, err
	}

	var out []FeedItem

	// Issue creates.
	for _, iss := range issues {
		if iss.CreatedAt == "" || iss.CreatedAt < since {
			continue
		}
		if isSelfActor(me, iss.AuthorID, iss.Author) {
			continue
		}
		reasons := relevance(me, iss, watches, false)
		if len(reasons) == 0 {
			continue
		}
		out = append(out, FeedItem{
			EventID:       "cr:" + iss.Key,
			IssueKey:      iss.Key,
			Summary:       iss.Title,
			CurrentStatus: iss.Status,
			EventType:     "created",
			OccurredAt:    nilIfEmptyStr(iss.CreatedAt),
			ActorName:     iss.Author,
			Payload:       map[string]any{},
			Reasons:       reasons,
			ReadAt:        nilIfEmptyStr(reads["cr:"+iss.Key]),
		})
	}

	// Changelog: status / assignee as discrete events; other fields grouped.
	type fieldGroup struct {
		itemID, at, author string
		fields             []string
		// first change supplies from/to for a single-field group when useful
	}
	groups := map[string]*fieldGroup{} // key: itemID|at|author
	groupOrder := []string{}

	for _, ch := range changes {
		iss, ok := issues[ch.ItemID]
		if !ok {
			continue
		}
		if isSelfActor(me, ch.AuthorID, ch.Author) {
			continue
		}
		switch ch.Field {
		case "status":
			eventType := "status_changed"
			if iss.ReopenedAt != "" && iss.ReopenedAt == ch.At {
				eventType = "reopened"
			}
			reasons := relevance(me, iss, watches, false)
			if len(reasons) == 0 {
				continue
			}
			eid := "cl:" + ch.ID
			out = append(out, FeedItem{
				EventID:       eid,
				IssueKey:      iss.Key,
				Summary:       iss.Title,
				CurrentStatus: iss.Status,
				EventType:     eventType,
				OccurredAt:    nilIfEmptyStr(ch.At),
				ActorName:     ch.Author,
				Payload:       map[string]any{"from": ch.FromValue, "to": ch.ToValue},
				Reasons:       reasons,
				ReadAt:        nilIfEmptyStr(reads[eid]),
			})
		case "assignee":
			reasons := relevance(me, iss, watches, false)
			if len(reasons) == 0 {
				continue
			}
			eid := "cl:" + ch.ID
			out = append(out, FeedItem{
				EventID:       eid,
				IssueKey:      iss.Key,
				Summary:       iss.Title,
				CurrentStatus: iss.Status,
				EventType:     "assigned",
				OccurredAt:    nilIfEmptyStr(ch.At),
				ActorName:     ch.Author,
				Payload:       map[string]any{"from": ch.FromValue, "to": ch.ToValue},
				Reasons:       reasons,
				ReadAt:        nilIfEmptyStr(reads[eid]),
			})
		default:
			if ch.Field == "" {
				continue
			}
			gk := ch.ItemID + "\x00" + ch.At + "\x00" + ch.Author
			g, ok := groups[gk]
			if !ok {
				g = &fieldGroup{itemID: ch.ItemID, at: ch.At, author: ch.Author}
				groups[gk] = g
				groupOrder = append(groupOrder, gk)
			}
			g.fields = append(g.fields, ch.Field)
		}
	}
	for _, gk := range groupOrder {
		g := groups[gk]
		iss, ok := issues[g.itemID]
		if !ok {
			continue
		}
		reasons := relevance(me, iss, watches, false)
		if len(reasons) == 0 {
			continue
		}
		// Dedupe field names while keeping first-seen order.
		seen := map[string]bool{}
		fields := make([]string, 0, len(g.fields))
		for _, f := range g.fields {
			if seen[f] {
				continue
			}
			seen[f] = true
			fields = append(fields, f)
		}
		eid := "fl:" + g.itemID + ":" + g.at
		out = append(out, FeedItem{
			EventID:       eid,
			IssueKey:      iss.Key,
			Summary:       iss.Title,
			CurrentStatus: iss.Status,
			EventType:     "fields_changed",
			OccurredAt:    nilIfEmptyStr(g.at),
			ActorName:     g.author,
			Payload:       map[string]any{"fields": fields},
			Reasons:       reasons,
			ReadAt:        nilIfEmptyStr(reads[eid]),
		})
	}

	// Comments.
	for _, c := range comments {
		iss, ok := issues[c.ItemID]
		if !ok {
			continue
		}
		if isSelfActor(me, c.AuthorID, c.Author) {
			continue
		}
		mentioned := mentionHit(c.BodyADF, me.AccountID)
		reasons := relevance(me, iss, watches, mentioned)
		if len(reasons) == 0 {
			continue
		}
		eid := "cm:" + c.ID
		excerpt := c.BodyText
		if excerpt == "" {
			excerpt = plainExcerptFromADF(c.BodyADF)
		}
		out = append(out, FeedItem{
			EventID:       eid,
			IssueKey:      iss.Key,
			Summary:       iss.Title,
			CurrentStatus: iss.Status,
			EventType:     "comment_added",
			OccurredAt:    nilIfEmptyStr(c.CreatedAt),
			ActorName:     c.Author,
			Payload:       map[string]any{"excerpt": truncateRunes(excerpt, feedExcerptRunes)},
			Reasons:       reasons,
			ReadAt:        nilIfEmptyStr(reads[eid]),
		})
	}

	// Attachments.
	for _, a := range attaches {
		iss, ok := issues[a.ItemID]
		if !ok {
			continue
		}
		if isSelfActor(me, a.AuthorID, a.Author) {
			continue
		}
		reasons := relevance(me, iss, watches, false)
		if len(reasons) == 0 {
			continue
		}
		eid := "at:" + a.ID
		out = append(out, FeedItem{
			EventID:       eid,
			IssueKey:      iss.Key,
			Summary:       iss.Title,
			CurrentStatus: iss.Status,
			EventType:     "attachment_added",
			OccurredAt:    nilIfEmptyStr(a.CreatedAt),
			ActorName:     a.Author,
			Payload:       map[string]any{"filename": a.Filename},
			Reasons:       reasons,
			ReadAt:        nilIfEmptyStr(reads[eid]),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := derefStr(out[i].OccurredAt), derefStr(out[j].OccurredAt)
		if ai != aj {
			return ai > aj
		}
		return out[i].EventID > out[j].EventID
	})
	return out, nil
}

func (db *DB) watchSet(ctx context.Context) (map[string]bool, error) {
	keys, err := db.Watches(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out, nil
}

func (db *DB) loadIssueMeta(ctx context.Context) (map[string]issueMeta, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT it.id, i.key, COALESCE(it.title,''), COALESCE(i.status,''),
		       COALESCE(i.assignee_id,''), COALESCE(i.assignee_email,''),
		       COALESCE(i.reporter_id,''), COALESCE(i.reporter_email,''),
		       COALESCE(i.created_at,''), COALESCE(i.reopened_at,''),
		       COALESCE(it.author,''), COALESCE(it.author_id,'')
		FROM issues i JOIN items it ON it.id = i.item_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]issueMeta{}
	for rows.Next() {
		var m issueMeta
		if err := rows.Scan(&m.ItemID, &m.Key, &m.Title, &m.Status,
			&m.AssigneeID, &m.AssigneeEmail, &m.ReporterID, &m.ReporterEmail,
			&m.CreatedAt, &m.ReopenedAt, &m.Author, &m.AuthorID); err != nil {
			return nil, err
		}
		out[m.ItemID] = m
	}
	return out, rows.Err()
}

func (db *DB) loadChangelogSince(ctx context.Context, since string) ([]rawChange, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, item_id, COALESCE(at,''), COALESCE(author,''), COALESCE(author_id,''), COALESCE(field,''),
		       COALESCE(from_value,''), COALESCE(to_value,'')
		FROM changelog WHERE at >= ?`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rawChange
	for rows.Next() {
		var c rawChange
		if err := rows.Scan(&c.ID, &c.ItemID, &c.At, &c.Author, &c.AuthorID, &c.Field, &c.FromValue, &c.ToValue); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (db *DB) loadCommentsSince(ctx context.Context, since string) ([]rawComment, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, item_id, COALESCE(author,''), COALESCE(author_id,''),
		       COALESCE(body_adf,''), COALESCE(body_text,''), COALESCE(created_at,'')
		FROM comments WHERE created_at >= ?`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rawComment
	for rows.Next() {
		var c rawComment
		if err := rows.Scan(&c.ID, &c.ItemID, &c.Author, &c.AuthorID, &c.BodyADF, &c.BodyText, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (db *DB) loadAttachmentsSince(ctx context.Context, since string) ([]rawAttach, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, item_id, COALESCE(filename,''), COALESCE(author,''), COALESCE(author_id,''), COALESCE(created_at,'')
		FROM attachments WHERE created_at >= ?`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rawAttach
	for rows.Next() {
		var a rawAttach
		if err := rows.Scan(&a.ID, &a.ItemID, &a.Filename, &a.Author, &a.AuthorID, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (db *DB) loadFeedReads(ctx context.Context) (map[string]string, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT event_id, read_at FROM feed_reads`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, at string
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = at
	}
	return out, rows.Err()
}

// relevance returns why this event is on the feed (OR of conditions). Order is
// stable for the client badges.
func relevance(me FeedIdentity, iss issueMeta, watches map[string]bool, mentioned bool) []string {
	var reasons []string
	if watches[iss.Key] {
		reasons = append(reasons, "watched")
	}
	if isMe(me, iss.AssigneeID, iss.AssigneeEmail) {
		reasons = append(reasons, "assignee")
	}
	if isMe(me, iss.ReporterID, iss.ReporterEmail) {
		reasons = append(reasons, "reporter")
	}
	if mentioned {
		reasons = append(reasons, "mention")
	}
	return reasons
}

func isMe(me FeedIdentity, id, email string) bool {
	if me.AccountID != "" && id != "" && id == me.AccountID {
		return true
	}
	if me.Email != "" && email != "" && strings.EqualFold(email, me.Email) {
		return true
	}
	return false
}

// isSelfActor excludes actions the local user performed. author_id wins when
// both sides have one (same display name, two accounts, must not collide).
// Name fallback is only for legacy rows whose author_id is NULL.
func isSelfActor(me FeedIdentity, authorID, author string) bool {
	if me.AccountID != "" && authorID != "" {
		return authorID == me.AccountID
	}
	if me.DisplayName != "" && author != "" && author == me.DisplayName {
		return true
	}
	return false
}

// mentionHit walks ADF JSON for mention nodes whose attrs.id matches accountID.
// Disabled when accountID is empty.
func mentionHit(bodyADF, accountID string) bool {
	if accountID == "" || bodyADF == "" {
		return false
	}
	var doc any
	if json.Unmarshal([]byte(bodyADF), &doc) != nil {
		return false
	}
	return walkMentions(doc, accountID)
}

func walkMentions(node any, accountID string) bool {
	switch v := node.(type) {
	case []any:
		for _, child := range v {
			if walkMentions(child, accountID) {
				return true
			}
		}
	case map[string]any:
		if kind, _ := v["type"].(string); kind == "mention" {
			if attrs, ok := v["attrs"].(map[string]any); ok {
				if id, _ := attrs["id"].(string); id == accountID {
					return true
				}
			}
		}
		if walkMentions(v["content"], accountID) {
			return true
		}
	}
	return false
}

// plainExcerptFromADF is a last-resort excerpt when body_text is empty.
// Full flattened text; callers apply truncateRunes. Shared ADF walker lives in
// excerpt.go (pageExcerptFromADF uses the same extraction).
func plainExcerptFromADF(raw string) string {
	return plainTextFromADF(raw)
}

func truncateRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func normalizeFocus(f FeedFocus) FeedFocus {
	switch f {
	case FeedFocusAssignee, FeedFocusReporter, FeedFocusMention:
		return f
	default:
		return FeedFocusAll
	}
}

func filterFocus(events []FeedItem, focus FeedFocus) []FeedItem {
	if focus == FeedFocusAll {
		// copy so callers can mutate without sharing the full slice header
		out := make([]FeedItem, len(events))
		copy(out, events)
		return out
	}
	out := make([]FeedItem, 0, len(events))
	for _, e := range events {
		for _, r := range e.Reasons {
			if r == string(focus) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

func countUnread(events []FeedItem) FeedUnreadCounts {
	var c FeedUnreadCounts
	for _, e := range events {
		if e.ReadAt != nil && *e.ReadAt != "" {
			continue
		}
		c.All++
		var hasA, hasR, hasM bool
		for _, r := range e.Reasons {
			switch r {
			case "assignee":
				hasA = true
			case "reporter":
				hasR = true
			case "mention":
				hasM = true
			}
		}
		if hasA {
			c.Assignee++
		}
		if hasR {
			c.Reporter++
		}
		if hasM {
			c.Mention++
		}
	}
	return c
}

func eventNumericID(eventID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(eventID))
	// Keep positive; 0 is an awkward Svelte each key.
	n := int(h.Sum32() & 0x7fffffff)
	if n == 0 {
		return 1
	}
	return n
}

func nilIfEmptyStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
