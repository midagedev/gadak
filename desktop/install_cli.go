//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/midagedev/scry/internal/clitool"
)

// appendInstallCLIMenu adds Tools → "Install Command Line Tool…" (macOS only).
// wailsCtx is filled in OnStartup; the click handler no-ops until then.
func appendInstallCLIMenu(appMenu *menu.Menu, wailsCtx *context.Context) {
	tools := appMenu.AddSubmenu("Tools")
	tools.AddText("Install Command Line Tool…", nil, func(*menu.CallbackData) {
		ctx := *wailsCtx
		if ctx == nil {
			return
		}
		handleInstallCLI(ctx)
	})
}

// bundleCLIPath returns Contents/Resources/bin/scry next to the desktop binary
// (…/Contents/MacOS/scry-desktop). Empty path + error when not in a bundle.
func bundleCLIPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve app executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	// …/Scry.app/Contents/MacOS/scry-desktop → …/Contents/Resources/bin/scry
	macosDir := filepath.Dir(exe)
	contentsDir := filepath.Dir(macosDir)
	cli := filepath.Join(contentsDir, "Resources", "bin", "scry")
	fi, err := os.Stat(cli)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("CLI binary not found at %s — open the installed Scry.app (not a dev build)", clitool.TildeHome(cli))
		}
		return "", fmt.Errorf("stat CLI binary: %w", err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("CLI path is a directory: %s", clitool.TildeHome(cli))
	}
	return cli, nil
}

func handleInstallCLI(ctx context.Context) {
	source, err := bundleCLIPath()
	if err != nil {
		showDialog(ctx, runtime.ErrorDialog, "Command Line Tool", err.Error())
		return
	}

	plan, err := clitool.Resolve(source, "", os.Getenv("PATH"))
	if err != nil {
		showDialog(ctx, runtime.ErrorDialog, "Command Line Tool", humanInstallErr(err))
		return
	}

	switch plan.Status {
	case clitool.StatusLinked:
		msg := fmt.Sprintf("Already installed:\n%s → %s",
			clitool.TildeHome(plan.Dest), clitool.TildeHome(plan.Source))
		if !plan.OnPath {
			msg += pathOffMsg(ctx, plan.Dir)
		}
		msg += "\n\nNext: run  scry mcp install claude  in a terminal to connect your agent."
		showDialog(ctx, runtime.InfoDialog, "Command Line Tool", msg)
		return

	case clitool.StatusConflict:
		detail := fmt.Sprintf("%s already exists", clitool.TildeHome(plan.Dest))
		if plan.ExistingKind == "symlink" && plan.ExistingTarget != "" {
			detail = fmt.Sprintf("%s already points to %s",
				clitool.TildeHome(plan.Dest), clitool.TildeHome(plan.ExistingTarget))
		} else if plan.ExistingKind != "" {
			detail = fmt.Sprintf("%s already exists (%s)",
				clitool.TildeHome(plan.Dest), plan.ExistingKind)
		}
		choice, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:          runtime.QuestionDialog,
			Title:         "Replace existing file?",
			Message:       detail + "\n\nReplace it with a link to the Scry app’s CLI?",
			Buttons:       []string{"Replace", "Cancel"},
			DefaultButton: "Cancel",
			CancelButton:  "Cancel",
		})
		if err != nil || choice != "Replace" {
			return
		}
		if err := clitool.Install(plan, true); err != nil {
			showDialog(ctx, runtime.ErrorDialog, "Command Line Tool", humanInstallErr(err))
			return
		}

	default: // missing
		if err := clitool.Install(plan, false); err != nil {
			showDialog(ctx, runtime.ErrorDialog, "Command Line Tool", humanInstallErr(err))
			return
		}
	}

	msg := fmt.Sprintf("Installed:\n%s → %s",
		clitool.TildeHome(plan.Dest), clitool.TildeHome(plan.Source))
	if !plan.OnPath {
		msg += pathOffMsg(ctx, plan.Dir)
	}
	msg += "\n\nNext: run  scry mcp install claude  in a terminal to connect your agent."
	showDialog(ctx, runtime.InfoDialog, "Command Line Tool", msg)
}

// pathOffMsg notes that dir is off PATH and copies PathExportLine to the clipboard.
func pathOffMsg(ctx context.Context, dir string) string {
	line := clitool.PathExportLine(dir, os.Getenv("SHELL"))
	copied := ""
	if err := runtime.ClipboardSetText(ctx, line); err != nil {
		copied = "\n(Could not copy to the clipboard — paste this yourself.)"
	} else {
		copied = "\nThat line was copied to the clipboard."
	}
	return fmt.Sprintf(
		"\n\nNote: %s is not on your PATH. Add it with:\n  %s%s",
		clitool.TildeHome(dir), line, copied,
	)
}

func showDialog(ctx context.Context, kind runtime.DialogType, title, message string) {
	_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    kind,
		Title:   title,
		Message: message,
	})
}

// humanInstallErr keeps dialogs free of secrets; install errors are path/permission only.
func humanInstallErr(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}
