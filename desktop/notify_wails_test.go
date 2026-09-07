package main

// GDK-1580 adapter tests. The fake sender stands in for wails'
// *notifications.NotificationService, so every path here — mapping,
// startup refusal, permission denial, send failure — runs on any GOOS
// without native notification machinery. What these tests cannot cover is
// the real service's behaviour (an unbundled darwin build refuses startup;
// a bundled one prompts); that is lead-side, on a packaged .app.

import (
	"context"
	"errors"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type fakeNotificationSender struct {
	// startupErr, when set, makes ServiceStartup fail once and forever —
	// the unbundled-darwin stand-in.
	startupErr error
	// granted is what RequestNotificationAuthorization answers.
	granted bool
	authErr error
	sendErr error

	startups   int
	auths      int
	sent       []notifications.NotificationOptions
	sentErrIdx int // index whose SendNotification returns sendErr; -1 = never
}

func (f *fakeNotificationSender) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	f.startups++
	return f.startupErr
}

func (f *fakeNotificationSender) RequestNotificationAuthorization() (bool, error) {
	f.auths++
	return f.granted, f.authErr
}

func (f *fakeNotificationSender) SendNotification(options notifications.NotificationOptions) error {
	f.sent = append(f.sent, options)
	if f.sentErrIdx >= 0 && len(f.sent)-1 == f.sentErrIdx {
		return f.sendErr
	}
	return f.sendErr
}

func newTestWailsNotifier(sender notificationSender) *wailsNotifier {
	return &wailsNotifier{sender: sender}
}

func TestWailsNotifierMapsSyncAlertOntoService(t *testing.T) {
	sender := &fakeNotificationSender{granted: true}
	n := newTestWailsNotifier(sender)
	if !n.Supported() {
		t.Fatal("Supported() false with a startable service")
	}
	if err := n.Notify("NMB-12 comment by Marco +2 more", "Fix login"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("want 1 notification, got %d", len(sender.sent))
	}
	got := sender.sent[0]
	// The whole mapping: title and body ride straight through, and the id
	// is the stable one that makes consecutive alerts replace, not pile.
	if got.Title != "NMB-12 comment by Marco +2 more" {
		t.Errorf("Title %q, want the sync title verbatim", got.Title)
	}
	if got.Body != "Fix login" {
		t.Errorf("Body %q, want the sync body verbatim", got.Body)
	}
	if got.ID != syncNotificationID {
		t.Errorf("ID %q, want %q", got.ID, syncNotificationID)
	}
	if got.ID == "" {
		t.Error("empty id: wails validateNotificationOptions rejects it")
	}
}

func TestWailsNotifierSupportedStartsOnceAndCachesRefusal(t *testing.T) {
	sender := &fakeNotificationSender{startupErr: errors.New("notifications require a valid bundle identifier")}
	n := newTestWailsNotifier(sender)
	for i := 0; i < 3; i++ {
		if n.Supported() {
			t.Fatal("Supported() true despite startup refusal")
		}
	}
	if sender.startups != 1 {
		t.Fatalf("startup attempted %d times, want 1 — a refusal is cached, not re-probed each sync cycle", sender.startups)
	}
	if err := n.Notify("t", "b"); err == nil {
		t.Fatal("Notify must fail when the service never started")
	}
	if sender.auths != 0 {
		t.Errorf("authorization asked %d times on a dead service, want 0", sender.auths)
	}
}

func TestWailsNotifierSupportedNeverAsksAuthorization(t *testing.T) {
	// The settings handler asks Supported() per request. If that ever
	// reached RequestNotificationAuthorization, a darwin permission sheet
	// would hang the settings page for up to three minutes.
	sender := &fakeNotificationSender{granted: false}
	n := newTestWailsNotifier(sender)
	if !n.Supported() {
		t.Fatal("Supported() false with a startable service")
	}
	if sender.auths != 0 {
		t.Fatalf("Supported() asked authorization %d times, want 0", sender.auths)
	}
}

func TestWailsNotifierDeniedLeavesErrorForCaller(t *testing.T) {
	sender := &fakeNotificationSender{granted: false}
	n := newTestWailsNotifier(sender)
	err := n.Notify("t", "b")
	if !errors.Is(err, errNotificationDenied) {
		t.Fatalf("Notify error %v, want errNotificationDenied — a denial must surface so the watermark stays pending", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("denied authorization still sent %d notifications", len(sender.sent))
	}
}

func TestWailsNotifierSendErrorSurfaces(t *testing.T) {
	boom := errors.New("send failed")
	sender := &fakeNotificationSender{granted: true, sendErr: boom}
	n := newTestWailsNotifier(sender)
	if err := n.Notify("t", "b"); !errors.Is(err, boom) {
		t.Fatalf("Notify error %v, want the service error verbatim", err)
	}
}

func TestSyncNotificationOptionsShape(t *testing.T) {
	// The pure mapping, pinned independently of any sender: non-empty id
	// (wails' own validateNotificationOptions rejects ""), title and body
	// in their slots, and nothing else invented.
	got := syncNotificationOptions("T", "B")
	if got.ID == "" || got.Title != "T" || got.Body != "B" {
		t.Fatalf("got %+v", got)
	}
	if got.Subtitle != "" || got.CategoryID != "" || got.Data != nil || got.Sound != nil {
		t.Fatalf("mapping invented fields: %+v", got)
	}
}
