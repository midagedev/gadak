//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// raiseGadakWindowByTitle finds the native window the smoke harness already
// locates (title "Gadak") and restores + focuses it. user32 is the inbox
// API; we do not shell out to PowerShell.
func raiseGadakWindowByTitle(title string) bool {
	user32 := syscall.NewLazyDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")
	setForeground := user32.NewProc("SetForegroundWindow")
	showWindow := user32.NewProc("ShowWindow")
	isIconic := user32.NewProc("IsIconic")
	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return false
	}
	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return false
	}
	const swRestore = 9
	iconic, _, _ := isIconic.Call(hwnd)
	if iconic != 0 {
		_, _, _ = showWindow.Call(hwnd, swRestore)
	}
	_, _, _ = setForeground.Call(hwnd)
	return true
}
