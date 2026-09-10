package transition

import (
	"context"
	"strings"

	"github.com/midagedev/gadak/internal/store"
)

// MirrorStatusUse answers "how many issues does this project actually hold in
// that status", from the mirror, which is local and free. It is the tiebreak
// between two destination statuses that display the same name in the same
// category (GDK-1356): the gdk workspace carries an In Progress the 2026-09-01
// cutover left behind with zero issues in it, beside the In Progress the board
// shows. nil when there is no mirror or no project key to scope by — the pick
// then falls back to payload order.
//
// It lives in this package, not next to the CLI that first passed it
// (GDK-1521), because all three write surfaces need the identical answer:
// `gadak transition`, the REST transition write, and `gadak claim`'s Cloud
// fallback. internal/store does not import internal/transition, so the
// dependency closes without a cycle.
func MirrorStatusUse(ctx context.Context, db *store.DB, key string) func(string) int {
	project, _, ok := strings.Cut(key, "-")
	if db == nil || !ok || project == "" {
		return nil
	}
	seen := map[string]int{}
	return func(statusID string) int {
		if n, ok := seen[statusID]; ok {
			return n
		}
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM issues WHERE project_key = ? AND status_id = ?`,
			project, statusID).Scan(&n); err != nil {
			// A mirror this read cannot answer is not a reason to refuse the
			// write: zero leaves payload order deciding, which is where the
			// tiebreak started.
			n = 0
		}
		seen[statusID] = n
		return n
	}
}
