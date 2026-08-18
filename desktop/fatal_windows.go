//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// windowsMessageBox is user32 MessageBoxW. wails has no dialog helper for
// a missing-runtime path (the webview is what failed).
// MB_OK | MB_ICONERROR | MB_SETFOREGROUND.
func windowsMessageBox(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	tptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	sptr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	const mbOK, mbIconError, mbSetForeground = 0x00000000, 0x00000010, 0x00010000
	_, _, _ = proc.Call(0, uintptr(unsafe.Pointer(sptr)), uintptr(unsafe.Pointer(tptr)), mbOK|mbIconError|mbSetForeground)
}
