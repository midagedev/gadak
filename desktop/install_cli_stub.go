//go:build !darwin

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// appendInstallCLIMenu is a no-op off macOS (CLI install is a desktop-app concern there).
func appendInstallCLIMenu(appMenu *application.Menu) {}
