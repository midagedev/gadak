// Package reflink owns the cross-workspace reference grammar (GDK-1032):
// the gadak://<workspace>/<KEY> pointer a built-in or paired workspace
// stores as a Jira remote issue link, the read-only hydration of that
// pointer from the target workspace's own mirror, and the field-for-field
// copy between the origin's remote link and its mirror row.
//
// One grammar, one owner (GDK-1316): this shape used to live three times —
// the CLI's parser, the server's parser, and two scheme constants kept in
// step by hand — and the copies were one quiet edit away from disagreeing
// about what a stored pointer is. Everything that parses, composes, or
// hydrates a gadak:// pointer goes through this package; nothing is ever
// written to the target workspace. A pointer's live half is read-only and
// best-effort: a workspace that does not exist here, has no mirror, or
// does not carry the key is a miss, never an error — the pointer stays
// valid either way.
package reflink

import (
	"context"
	"database/sql"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// Scheme is the URL form of a cross-workspace pointer: gadak://<workspace>/<KEY>.
// It is the stored identity, so a reader can recognize these rows among
// ordinary remote links (a plain https:// URL is a valid pointer too — it
// just has nothing local to hydrate from). Distinct from the deeplink
// grammar (gadak://<action>?<params>, internal/deeplink), which shares the
// scheme but owns a different shape.
const Scheme = "gadak://"

// Compose builds the stored pointer URL for workspace/key.
func Compose(workspace, key string) string {
	return Scheme + workspace + "/" + key
}

// Parse splits a stored pointer into its workspace and key. Any other URL —
// or a malformed pointer — is reported as not ours; callers treat that as
// a plain external pointer, never an error.
func Parse(url string) (workspace, key string, ok bool) {
	if !strings.HasPrefix(url, Scheme) {
		return "", "", false
	}
	ws, k, found := strings.Cut(strings.TrimPrefix(url, Scheme), "/")
	if !found || ws == "" || k == "" {
		return "", "", false
	}
	return ws, k, true
}

// StoreLink copies one origin remote link into its mirror row shape. The
// six fields are one mapping, not six coincidences: the CLI write-through
// and the sync rewrite must produce identical rows from identical origins.
// origin.RemoteLink is jira.RemoteLink (an alias), so both surfaces feed
// the same type through here.
func StoreLink(rl jira.RemoteLink) store.RemoteLink {
	return store.RemoteLink{
		ID: rl.ID, GlobalID: rl.GlobalID, Relationship: rl.Relationship,
		URL: rl.URL, Title: rl.Title, Summary: rl.Summary,
	}
}

// Lite is what hydration reads out of another workspace's mirror.
type Lite struct {
	Summary  string
	Status   string
	Category string
	Assignee string
}

// Mirror is one other workspace's mirror, opened read-only. A nil Mirror
// means "not available here" — a missing workspace, a mirror that was
// never synced, or a file this process may not read. None of that is an
// error: the pointer stands, it just has no live half.
type Mirror struct {
	db *sql.DB
}

// OpenMirror opens workspace's own mirror read-only, or returns nil when
// there is nothing here to hydrate from. The caller owns closing.
func OpenMirror(workspace string) *Mirror {
	path, err := config.DBPathFor(workspace)
	if err != nil {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	db, err := store.OpenReadOnly(path)
	if err != nil {
		return nil
	}
	return &Mirror{db: db}
}

// Lookup reads one issue out of the mirror. ctx bounds the wait — a
// foreign mirror is another process's file, and a caller that must not
// block (the detail request) passes its own deadline. A miss, for any
// reason, is reported as not found rather than an error.
func (m *Mirror) Lookup(ctx context.Context, key string) (Lite, bool) {
	if m == nil || m.db == nil {
		return Lite{}, false
	}
	var lite Lite
	err := m.db.QueryRowContext(ctx, `
		SELECT COALESCE(summary,''), COALESCE(status,''), COALESCE(status_category,''),
		       COALESCE(assignee,'')
		FROM issues_full WHERE key = ? LIMIT 1`, key).
		Scan(&lite.Summary, &lite.Status, &lite.Category, &lite.Assignee)
	if err != nil {
		return Lite{}, false
	}
	return lite, true
}

// Close releases the mirror. Safe on a nil Mirror.
func (m *Mirror) Close() {
	if m == nil || m.db == nil {
		return
	}
	_ = m.db.Close()
}
