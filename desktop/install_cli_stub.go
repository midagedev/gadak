//go:build !darwin

package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/menu"
)

// appendInstallCLIMenu is a no-op off macOS (CLI install is a desktop-app concern there).
func appendInstallCLIMenu(appMenu *menu.Menu, wailsCtx *context.Context) {}
