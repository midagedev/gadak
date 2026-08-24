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

// notifier delivers one OS desktop notification. Implementations must never
// panic; callers treat every error as non-fatal and continue the sync loop.
//
// Supported is the delivery capability: false means Notify will not actually
// alert anyone (Windows and other no-op platforms). notifyAfterSync must not
// advance last_notified_at in that case — skipped events stay pending for a
// later implementation (Windows toast).
type notifier interface {
	Notify(title, body string) error
	Supported() bool
}

// OSNotifier uses the platform's desktop-notification command.
// darwin: osascript display notification; linux: notify-send; others: unsupported.
type OSNotifier struct{}

// osNotifyCommand is the single GOOS switch for both Supported and Notify.
// A nil command means this OS cannot deliver; the two methods cannot disagree.
func osNotifyCommand(goos, title, body string) *exec.Cmd {
	switch goos {
	case "darwin":
		// AppleScript string literals use double quotes; escape \ and " only.
		esc := func(s string) string {
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, `"`, `\"`)
			return s
		}
		script := fmt.Sprintf(`display notification "%s" with title "%s"`, esc(body), esc(title))
		return exec.Command("osascript", "-e", script)
	case "linux":
		return exec.Command("notify-send", title, body)
	default:
		return nil
	}
}

// Supported reports whether this process can fire a real OS notification.
func (OSNotifier) Supported() bool {
	return osNotifyCommand(runtime.GOOS, "", "") != nil
}

// Notify implements notifier.
func (OSNotifier) Notify(title, body string) error {
	cmd := osNotifyCommand(runtime.GOOS, title, body)
	if cmd == nil {
		// Windows and others: no reliable stdlib path without extra deps.
		return nil
	}
	return cmd.Run()
}

// notifyAfterSync fires one bundled OS notification for feed events newer than
// the last_notified_at watermark. Never marks feed_reads. Failures are returned
// for logging only — the caller must not abort the watch loop.
//
// First successful pass with an empty watermark seeds last_notified_at to now
// without notifying, so a long mirror history does not dump on login.
func notifyAfterSync(ctx context.Context, db *store.DB, cfg *config.Config, n notifier) error {
	if cfg == nil || !cfg.NotifyEnabled() {
		return nil
	}
	if n == nil {
		n = OSNotifier{}
	}
	st, err := db.SyncState(ctx, SourceID)
	if err != nil {
		return err
	}
	watermark := ""
	if st.LastNotifiedAt != nil {
		watermark = *st.LastNotifiedAt
	}
	if watermark == "" {
		// Bootstrap: advance the watermark without a flood of historical events.
		// Runs even when the notifier is unsupported so a later toast
		// implementation does not dump pre-install history; events after this
		// seed stay pending until delivery works.
		return db.SetLastNotifiedAt(ctx, SourceID, store.Now())
	}
	if !n.Supported() {
		// Do not Notify and do not consume the watermark. A no-op that
		// returned nil used to look like success and skip these events forever.
		return nil
	}

	res, err := db.Feed(ctx, store.FeedOpts{
		Focus: store.FeedFocusAll,
		Limit: store.FeedMaxLimit,
		Me:    store.FeedIdentityOf(cfg),
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
	return db.SetLastNotifiedAt(ctx, SourceID, maxAt)
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
