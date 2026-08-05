package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/midagedev/scry/internal/config"
)

const serviceLabel = "dev.midagedev.scry"

// cmdInstallService writes a user-level unit so the mirror survives reboot:
// launchd on darwin, systemd --user on linux. Windows is unsupported.
func cmdInstallService(args []string) error {
	fs := newFlagSet("install-service")
	uninstall := fs.Bool("uninstall", false, "remove the installed service unit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		if *uninstall {
			return uninstallLaunchd()
		}
		return installLaunchd()
	case "linux":
		if *uninstall {
			return uninstallSystemd()
		}
		return installSystemd()
	case "windows":
		return fmt.Errorf("install-service is not supported on Windows — run `scry serve --no-open` from Task Scheduler or a login script instead")
	default:
		return fmt.Errorf("install-service: unsupported OS %q", runtime.GOOS)
	}
}

// serveArgs is the ProgramArguments / ExecStart tail: optional --profile, then
// serve --no-open. Absolute binary path is separate.
func serveArgs() []string {
	var args []string
	if p := config.Profile(); p != "" {
		args = append(args, "--profile", p)
	}
	args = append(args, "serve", "--no-open")
	return args
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func installLaunchd() error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, serviceLabel+".plist")

	args := serveArgs()
	var progArgs strings.Builder
	progArgs.WriteString(fmt.Sprintf("    <string>%s</string>\n", xmlEscape(exe)))
	for _, a := range args {
		progArgs.WriteString(fmt.Sprintf("    <string>%s</string>\n", xmlEscape(a)))
	}
	logPath := filepath.Join(home, "Library", "Logs", "scry.log")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
%s  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, serviceLabel, progArgs.String(), xmlEscape(logPath), xmlEscape(logPath))

	// Prefer unload-before-write so a rewrite reloads cleanly on older launchctl.
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		// launchctl bootstrap is the modern path on newer macOS; try both.
		uid := os.Getuid()
		if err2 := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", uid), path).Run(); err2 != nil {
			return fmt.Errorf("wrote %s but could not load it (launchctl load: %v; bootstrap: %v)", path, err, err2)
		}
	}
	fmt.Printf("installed launchd agent\n  plist: %s\n  exec:  %s %s\n  logs:  %s\n",
		path, exe, strings.Join(args, " "), logPath)
	return nil
}

func uninstallLaunchd() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
	_ = exec.Command("launchctl", "unload", path).Run()
	uid := os.Getuid()
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", uid, serviceLabel)).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("removed launchd agent %s\n", path)
	return nil
}

func installSystemd() error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "scry.service")
	args := serveArgs()
	// Quote each arg for ExecStart.
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, shellQuote(exe))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	unit := fmt.Sprintf(`[Unit]
Description=scry local Jira mirror
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, strings.Join(parts, " "))

	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return err
	}
	// Reload, enable, start — best-effort so a missing user session bus still
	// leaves the unit file in place.
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	if err := exec.Command("systemctl", "--user", "enable", "--now", "scry.service").Run(); err != nil {
		fmt.Printf("wrote %s but systemctl --user enable --now failed: %v\n", path, err)
		fmt.Printf("  (enable lingering or start a user session, then: systemctl --user enable --now scry)\n")
		fmt.Printf("  exec: %s %s\n", exe, strings.Join(args, " "))
		return nil
	}
	fmt.Printf("installed systemd user unit\n  unit:  %s\n  exec:  %s %s\n",
		path, exe, strings.Join(args, " "))
	return nil
}

func uninstallSystemd() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".config", "systemd", "user", "scry.service")
	_ = exec.Command("systemctl", "--user", "disable", "--now", "scry.service").Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	fmt.Printf("removed systemd user unit %s\n", path)
	return nil
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// shellQuote is enough for ExecStart paths/args with spaces.
func shellQuote(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"'\\$`") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
