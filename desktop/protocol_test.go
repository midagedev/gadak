package main

import "testing"

func TestProtocolCommand(t *testing.T) {
	exe := `C:\Program Files\Gadak\gadak-desktop.exe`
	got := protocolCommand(exe)
	want := `"C:\Program Files\Gadak\gadak-desktop.exe" "%1"`
	if got != want {
		t.Fatalf("protocolCommand = %q, want %q", got, want)
	}
}

func TestProtocolNeedsRewrite(t *testing.T) {
	want := protocolCommand(`C:\Gadak\gadak-desktop.exe`)
	if protocolNeedsRewrite(want, want) {
		t.Fatal("identical command needs rewrite")
	}
	if !protocolNeedsRewrite("", want) {
		t.Fatal("empty current skipped rewrite")
	}
	moved := protocolCommand(`D:\Gadak\gadak-desktop.exe`)
	if !protocolNeedsRewrite(moved, want) {
		t.Fatal("path change skipped rewrite")
	}
}

func TestProtocolRegistersFor(t *testing.T) {
	table := []struct {
		goos string
		want bool
	}{
		{"windows", true},
		{"darwin", false},
		{"linux", false},
		{"freebsd", false},
		{"js", false},
		{"", false},
	}
	for _, tc := range table {
		if got := protocolRegistersFor(tc.goos); got != tc.want {
			t.Errorf("protocolRegistersFor(%q) = %v, want %v", tc.goos, got, tc.want)
		}
	}
}

func TestUnregisterGadakProtocolFlag(t *testing.T) {
	if hasUnregisterGadakProtocolFlag(nil) {
		t.Fatal("empty args should not unregister")
	}
	if hasUnregisterGadakProtocolFlag([]string{"gadak://view"}) {
		t.Fatal("deeplink is not the unregister flag")
	}
	if hasUnregisterGadakProtocolFlag([]string{"--print-window-chrome"}) {
		t.Fatal("chrome flag is not the unregister flag")
	}
	if !hasUnregisterGadakProtocolFlag([]string{"--unregister-gadak-protocol"}) {
		t.Fatal("flag not recognized")
	}
}
