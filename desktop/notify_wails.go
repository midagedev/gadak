package main

// GDK-1580: the desktop's sync-feed alerts ride wails' notifications service
// — Windows toasts (the CLI's osascript/notify-send default could not),
// UNUserNotificationCenter on a bundled macOS build, D-Bus on Linux.
// internal/sync cannot import wails (the root module forbids it), so this
// adapter is injected through the package-level seam
// (gadaksync.SetDefaultNotifier) in run(); the CLI keeps its default.

import (
	"context"
	"errors"
	"sync"

	gadaksync "github.com/midagedev/gadak/internal/sync"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// syncNotificationID is the stable id every feed notification carries. On
// macOS the center replaces an undelivered notification with the same id, so
// a burst of watch cycles shows one current alert, not a pile.
const syncNotificationID = "gadak.sync"

// notificationSender is the wails service surface this adapter uses. The
// concrete *notifications.NotificationService satisfies it; tests fake it so
// the mapping and refusal paths run without native code on any GOOS.
type notificationSender interface {
	ServiceStartup(ctx context.Context, options application.ServiceOptions) error
	RequestNotificationAuthorization() (bool, error)
	SendNotification(options notifications.NotificationOptions) error
}

// wailsNotifier implements gadaksync.Notifier over the wails service.
//
// The service is NOT registered in application.Options.Services on purpose:
// a service whose ServiceStartup fails is fatal in app.Run(), and startup
// legitimately fails here — an unbundled dev binary has no bundle identifier
// on darwin. Starting lazily instead turns that into Supported()==false,
// which notifyAfterSync already treats as "leave the watermark pending".
//
// First use must happen after application.New (windows Startup reads
// application.Get().Config()); in production every caller is a watch cycle
// or a settings request, both of which only exist once the app runs.
type wailsNotifier struct {
	mu       sync.Mutex
	started  bool
	startErr error
	// sender is nil until first use, then notifications.New() once. The
	// wails constructor is a sync.Once singleton, so repeated lazy calls
	// would return the same instance anyway; the local started flag keeps
	// the startup attempt single too.
	sender notificationSender
}

// newWailsNotifier returns the adapter run() injects. It touches nothing
// native until the first Supported/Notify call.
func newWailsNotifier() *wailsNotifier {
	return &wailsNotifier{}
}

// service returns the started sender, starting it once. The bool is false
// when the platform cannot host notifications (unbundled darwin build, no
// session bus on linux); startErr says why, for the log line.
func (w *wailsNotifier) service() (notificationSender, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return w.sender, nil
	}
	if w.startErr != nil {
		return nil, w.startErr
	}
	sender := w.sender
	if sender == nil {
		sender = notifications.New()
	}
	if err := sender.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		// Cache the refusal: an unbundled binary does not become bundled,
		// and Supported() must not retry platform probing every sync cycle.
		w.startErr = err
		return nil, err
	}
	w.sender = sender
	w.started = true
	return sender, nil
}

// Supported reports whether the wails service could start here. It never
// prompts and never blocks — authorization (which on darwin means the
// permission sheet, held open for up to three minutes) belongs to Notify,
// which the sync loop calls only when there is something to deliver.
func (w *wailsNotifier) Supported() bool {
	_, err := w.service()
	return err == nil
}

// errNotificationDenied is returned when the user has not granted (or has
// revoked) notification permission. Delivered as an error so
// notifyAfterSync leaves the watermark alone: grant it later and the pending
// events still deliver on a following cycle.
var errNotificationDenied = errors.New("notification permission not granted")

// Notify maps one sync feed alert onto the wails service.
func (w *wailsNotifier) Notify(title, body string) error {
	sender, err := w.service()
	if err != nil {
		return err
	}
	// darwin: the real permission check — instant once decided, a sheet the
	// first time. windows/linux: a (true, nil) stub. Denied is not fatal and
	// not retried into a prompt; it returns as an error so the events stay
	// pending for the moment the user changes their mind.
	granted, err := sender.RequestNotificationAuthorization()
	if err != nil {
		return err
	}
	if !granted {
		return errNotificationDenied
	}
	return sender.SendNotification(syncNotificationOptions(title, body))
}

// syncNotificationOptions is the whole message mapping: title and body ride
// straight through (summarizeFeedNotify already built both — issue key and
// event kind in the title, issue title only in the body, never comment
// text), and the id is what makes consecutive alerts replace, not pile.
func syncNotificationOptions(title, body string) notifications.NotificationOptions {
	return notifications.NotificationOptions{
		ID:    syncNotificationID,
		Title: title,
		Body:  body,
	}
}

// compile-time proof that the adapter is the seam's shape.
var _ gadaksync.Notifier = (*wailsNotifier)(nil)
