package sync

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// Notifier delivers one OS desktop notification. Implementations must never
// panic; callers treat every error as non-fatal and continue the sync loop.
type Notifier interface {
	Notify(title, body string) error
}

// OSNotifier uses the platform's desktop-notification command.
// darwin: osascript display notification; linux: notify-send; windows: no-op.
type OSNotifier struct{}

// Notify implements Notifier.
func (OSNotifier) Notify(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		// AppleScript string literals use double quotes; escape \ and " only.
		esc := func(s string) string {
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, `"`, `\"`)
			return s
		}
		script := fmt.Sprintf(`display notification "%s" with title "%s"`, esc(body), esc(title))
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		return exec.Command("notify-send", title, body).Run()
	default:
		// Windows and others: quietly skip (no reliable stdlib path without extra deps).
		return nil
	}
}

// FeedIdentity builds the personal-feed identity the same way the HTTP server does.
func FeedIdentity(cfg *config.Config) store.FeedIdentity {
	if cfg == nil {
		return store.FeedIdentity{}
	}
	return store.FeedIdentity{
		AccountID:   cfg.AccountID,
		Email:       cfg.Email,
		DisplayName: cfg.TokenOwner,
	}
}

// notifyAfterSync fires one bundled OS notification for feed events newer than
// the last_notified_at watermark. Never marks feed_reads. Failures are returned
// for logging only — the caller must not abort the watch loop.
//
// First successful pass with an empty watermark seeds last_notified_at to now
// without notifying, so a long mirror history does not dump on login.
func notifyAfterSync(db *store.DB, cfg *config.Config, n Notifier) error {
	if cfg == nil || !cfg.NotifyEnabled() {
		return nil
	}
	if n == nil {
		n = OSNotifier{}
	}
	st, err := db.SyncState(context.Background(), SourceID)
	if err != nil {
		return err
	}
	watermark := ""
	if st.LastNotifiedAt != nil {
		watermark = *st.LastNotifiedAt
	}
	if watermark == "" {
		// Bootstrap: advance the watermark without a flood of historical events.
		return db.SetLastNotifiedAt(context.Background(), SourceID, store.Now())
	}

	res, err := db.Feed(context.Background(), store.FeedOpts{
		Focus: store.FeedFocusAll,
		Limit: store.FeedMaxLimit,
		Me:    FeedIdentity(cfg),
	})
	if err != nil {
		return err
	}
	var fresh []store.FeedItem
	maxAt := watermark
	for _, it := range res.Items {
		at := ""
		if it.OccurredAt != nil {
			at = *it.OccurredAt
		}
		if at == "" || at <= watermark {
			continue
		}
		fresh = append(fresh, it)
		if at > maxAt {
			maxAt = at
		}
	}
	if len(fresh) == 0 {
		return nil
	}

	title, body := summarizeFeedNotify(fresh)
	if err := n.Notify(title, body); err != nil {
		// Still advance the watermark? No — a failed delivery should retry next cycle.
		return err
	}
	return db.SetLastNotifiedAt(context.Background(), SourceID, maxAt)
}

// summarizeFeedNotify builds a single bundled line:
// "NMB-12 comment by Marco +2 more" with the issue title as body (no comment text).
func summarizeFeedNotify(items []store.FeedItem) (title, body string) {
	if len(items) == 0 {
		return "gadak", ""
	}
	// Feed is newest-first; the head is the headline event.
	head := items[0]
	kind := eventKindLabel(head.EventType)
	var b strings.Builder
	b.WriteString(head.IssueKey)
	if kind != "" {
		b.WriteByte(' ')
		b.WriteString(kind)
	}
	if head.ActorName != "" {
		b.WriteString(" by ")
		b.WriteString(head.ActorName)
	}
	if more := len(items) - 1; more > 0 {
		fmt.Fprintf(&b, " +%d more", more)
	}
	title = b.String()
	// Body: issue title only — never comment excerpts (sensitive).
	body = head.Summary
	if body == "" {
		body = head.IssueKey
	}
	return title, body
}

func eventKindLabel(eventType string) string {
	switch eventType {
	case "comment_added":
		return "comment"
	case "status_changed":
		return "status"
	case "reopened":
		return "reopened"
	case "assigned":
		return "assigned"
	case "attachment_added":
		return "attachment"
	case "fields_changed":
		return "fields"
	case "created":
		return "created"
	default:
		if eventType == "" {
			return "update"
		}
		return eventType
	}
}
