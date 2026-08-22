package main

import "testing"

// The launcher that broke (cmd/gadak/views.go startWindowsDesktopImpl) starts
// the exe with exactly one argument — the gadak:// URL. wails v3.0.0-beta.12
// treats that shape as ApplicationLaunchedWithUrl on Windows and on GTK4
// Linux (wailsapp/wails#6000, landed in beta.10).
const launcherURL = "gadak://view?issue=NMB-1"

func TestColdStartDecisionFor(t *testing.T) {
	for _, tc := range []struct {
		name             string
		goos             string
		args             []string
		wantApplyArgv    bool
		wantDeferToEvent bool
	}{
		{
			name:             "windows launcher one-arg (GDK-293)",
			goos:             "windows",
			args:             []string{"gadak-desktop.exe", launcherURL},
			wantApplyArgv:    false,
			wantDeferToEvent: true,
		},
		{
			name:             "windows no extra args — argv fallback is a no-op",
			goos:             "windows",
			args:             []string{"gadak-desktop.exe"},
			wantApplyArgv:    true,
			wantDeferToEvent: false,
		},
		{
			name:             "windows extra args — wails ignores, argv owns the URL",
			goos:             "windows",
			args:             []string{"gadak-desktop.exe", "--flag", launcherURL},
			wantApplyArgv:    true,
			wantDeferToEvent: false,
		},
		{
			name:             "windows two extra args including the URL",
			goos:             "windows",
			args:             []string{"gadak-desktop.exe", launcherURL, "other"},
			wantApplyArgv:    true,
			wantDeferToEvent: false,
		},
		{
			name:             "windows one arg without :// — wails does not emit",
			goos:             "windows",
			args:             []string{"gadak-desktop.exe", "not-a-url"},
			wantApplyArgv:    true,
			wantDeferToEvent: false,
		},
		{
			name:             "darwin one-arg is still event-only",
			goos:             "darwin",
			args:             []string{"gadak-desktop", launcherURL},
			wantApplyArgv:    false,
			wantDeferToEvent: true,
		},
		{
			name:             "darwin no args is event-only",
			goos:             "darwin",
			args:             []string{"gadak-desktop"},
			wantApplyArgv:    false,
			wantDeferToEvent: true,
		},
		{
			// Was argv under v3.0.0-beta.9 (GTK4 run() did not emit).
			// wailsapp/wails#6000 landed in beta.10; this pin is beta.12,
			// so Linux matches Windows: one-arg :// URL is event-only.
			name:             "linux one-arg is event (GTK4 emits since #6000 / beta.10)",
			goos:             "linux",
			args:             []string{"gadak-desktop", launcherURL},
			wantApplyArgv:    false,
			wantDeferToEvent: true,
		},
		{
			name:             "linux no args is argv",
			goos:             "linux",
			args:             []string{"gadak-desktop"},
			wantApplyArgv:    true,
			wantDeferToEvent: false,
		},
		{
			name:             "linux extra args — wails ignores, argv owns the URL",
			goos:             "linux",
			args:             []string{"gadak-desktop", "--flag", launcherURL},
			wantApplyArgv:    true,
			wantDeferToEvent: false,
		},
		{
			name:             "linux one arg without :// — wails does not emit",
			goos:             "linux",
			args:             []string{"gadak-desktop", "not-a-url"},
			wantApplyArgv:    true,
			wantDeferToEvent: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := coldStartDecisionFor(tc.goos, tc.args)
			if got.ApplyArgv != tc.wantApplyArgv || got.DeferToEvent != tc.wantDeferToEvent {
				t.Fatalf("coldStartDecisionFor(%q, %q) = {ApplyArgv:%v DeferToEvent:%v}, want {ApplyArgv:%v DeferToEvent:%v}",
					tc.goos, tc.args, got.ApplyArgv, got.DeferToEvent, tc.wantApplyArgv, tc.wantDeferToEvent)
			}
		})
	}
}

// TestColdStartGate is a state machine, not a runtime event-order test: it
// pins "nothing is applied before markReady" without opening a window.
func TestColdStartGate(t *testing.T) {
	type applied struct{ raw, src string }
	var got []applied
	g := coldStartGate{apply: func(raw, source string) {
		got = append(got, applied{raw, source})
	}}

	g.offer("gadak://view?issue=NMB-1", "event")
	if len(got) != 0 {
		t.Fatalf("offer before ready applied %v", got)
	}

	g.offer("gadak://view?issue=NMB-2", "event")
	g.markReady()
	if len(got) != 1 || got[0] != (applied{"gadak://view?issue=NMB-1", "event"}) {
		t.Fatalf("markReady flush = %v, want first-offer only", got)
	}

	g.offer("gadak://view?issue=NMB-3", "argv")
	if len(got) != 2 || got[1] != (applied{"gadak://view?issue=NMB-3", "argv"}) {
		t.Fatalf("offer after ready = %v, want immediate apply", got)
	}

	g.offer("", "event")
	if len(got) != 2 {
		t.Fatalf("empty offer applied %v", got)
	}
}
