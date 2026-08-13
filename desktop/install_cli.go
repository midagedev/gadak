//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/midagedev/gadak/internal/clitool"
)

// appendInstallCLIMenu adds Tools → "Install Command Line Tool…" (macOS only).
func appendInstallCLIMenu(appMenu *application.Menu) {
	tools := appMenu.AddSubmenu("Tools")
	tools.Add("Install Command Line Tool…").OnClick(func(*application.Context) {
		handleInstallCLI()
	})
}

// bundleCLIPath returns Contents/Resources/bin/gadak next to the desktop binary
// (…/Contents/MacOS/gadak-desktop). Empty path + error when not in a bundle.
func bundleCLIPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve app executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	// …/Gadak.app/Contents/MacOS/gadak-desktop → …/Contents/Resources/bin/gadak
	macosDir := filepath.Dir(exe)
	contentsDir := filepath.Dir(macosDir)
	cli := filepath.Join(contentsDir, "Resources", "bin", "gadak")
	fi, err := os.Stat(cli)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("CLI binary not found at %s — open the installed Gadak.app (not a dev build)", clitool.TildeHome(cli))
		}
		return "", fmt.Errorf("stat CLI binary: %w", err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("CLI path is a directory: %s", clitool.TildeHome(cli))
	}
	return cli, nil
}

func handleInstallCLI() {
	source, err := bundleCLIPath()
	if err != nil {
		showError("Command Line Tool", err.Error())
		return
	}

	plan, err := clitool.Resolve(source, "", os.Getenv("PATH"))
	if err != nil {
		showError("Command Line Tool", humanInstallErr(err))
		return
	}

	switch plan.Status {
	case clitool.StatusLinked:
		showInfo("Command Line Tool", installedMsg(plan, "Already installed:"))
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
		// v3 dialogs answer through a button callback rather than a return
		// value, so the rest of the install runs from inside Replace.
		dialog := application.Get().Dialog.Question()
		dialog.SetTitle("Replace existing file?")
		dialog.SetMessage(detail + "\n\nReplace it with a link to the Gadak app’s CLI?")
		replace := dialog.AddButton("Replace")
		cancel := dialog.AddButton("Cancel")
		dialog.SetDefaultButton(cancel)
		dialog.SetCancelButton(cancel)
		replace.OnClick(func() {
			if err := clitool.Install(plan, true); err != nil {
				showError("Command Line Tool", humanInstallErr(err))
				return
			}
			showInfo("Command Line Tool", installedMsg(plan, "Installed:"))
		})
		dialog.Show()
		return

	default: // missing
		if err := clitool.Install(plan, false); err != nil {
			showError("Command Line Tool", humanInstallErr(err))
			return
		}
	}

	showInfo("Command Line Tool", installedMsg(plan, "Installed:"))
}

// installedMsg reports where the link landed, plus the PATH note and the next
// step. headline is what changed ("Installed:" / "Already installed:").
func installedMsg(plan clitool.Plan, headline string) string {
	msg := fmt.Sprintf("%s\n%s → %s",
		headline, clitool.TildeHome(plan.Dest), clitool.TildeHome(plan.Source))
	if !plan.OnPath {
		msg += pathOffMsg(plan.Dir)
	}
	msg += "\n\nNext: run  gadak mcp install claude  in a terminal to connect your agent."
	return msg
}

// pathOffMsg notes that dir is off PATH and copies PathExportLine to the clipboard.
func pathOffMsg(dir string) string {
	line := clitool.PathExportLine(dir, os.Getenv("SHELL"))
	copied := ""
	if application.Get().Clipboard.SetText(line) {
		copied = "\nThat line was copied to the clipboard."
	} else {
		copied = "\n(Could not copy to the clipboard — paste this yourself.)"
	}
	return fmt.Sprintf(
		"\n\nNote: %s is not on your PATH. Add it with:\n  %s%s",
		clitool.TildeHome(dir), line, copied,
	)
}

func showInfo(title, message string) {
	dialog := application.Get().Dialog.Info()
	dialog.SetTitle(title)
	dialog.SetMessage(message)
	dialog.Show()
}

func showError(title, message string) {
	dialog := application.Get().Dialog.Error()
	dialog.SetTitle(title)
	dialog.SetMessage(message)
	dialog.Show()
}

// humanInstallErr keeps dialogs free of secrets; install errors are path/permission only.
func humanInstallErr(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}
