package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/midagedev/gadak/internal/config"
)

const serviceLabel = "dev.midagedev.gadak"

// runServiceCmd is the launchctl/systemctl runner. Tests replace it so we
// never touch a real session bus. Default is exec.Command(name, arg...).Run.
var runServiceCmd = func(name string, arg ...string) error {
	return exec.Command(name, arg...).Run()
}

// serviceNames is the single owner of unit identity. Default/empty keeps the
// historical names so an existing install-service keeps working. A named
// profile gets its own label/file so a second install cannot overwrite the
// first (D4).
func serviceNames(profile string) (label, plistFile, systemdFile string) {
	if profile == "" || profile == "default" {
		return serviceLabel, serviceLabel + ".plist", "gadak.service"
	}
	label = serviceLabel + "." + profile
	return label, label + ".plist", "gadak-" + profile + ".service"
}

func profileLabel(profile string) string {
	if profile == "" || profile == "default" {
		return "default"
	}
	return profile
}

// serveArgsFor is serve --no-open, with --profile when the profile is named.
func serveArgsFor(profile string) []string {
	var args []string
	if profile != "" && profile != "default" {
		args = append(args, "--profile", profile)
	}
	args = append(args, "serve", "--no-open")
	return args
}

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
		return fmt.Errorf("install-service is not supported on Windows — run `gadak serve --no-open` from Task Scheduler or a login script instead")
	default:
		return fmt.Errorf("install-service: unsupported OS %q", runtime.GOOS)
	}
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
	profile := config.Profile()
	label, plistName, _ := serviceNames(profile)
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, plistName)

	args := serveArgsFor(profile)
	var progArgs strings.Builder
	progArgs.WriteString(fmt.Sprintf("    <string>%s</string>\n", xmlEscape(exe)))
	for _, a := range args {
		progArgs.WriteString(fmt.Sprintf("    <string>%s</string>\n", xmlEscape(a)))
	}
	logPath := filepath.Join(home, "Library", "Logs", "gadak.log")
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
`, label, progArgs.String(), xmlEscape(logPath), xmlEscape(logPath))

	// Prefer unload-before-write so a rewrite reloads cleanly on older launchctl.
	_ = runServiceCmd("launchctl", "unload", path)
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	if err := runServiceCmd("launchctl", "load", path); err != nil {
		// launchctl bootstrap is the modern path on newer macOS; try both.
		uid := os.Getuid()
		if err2 := runServiceCmd("launchctl", "bootstrap", fmt.Sprintf("gui/%d", uid), path); err2 != nil {
			return fmt.Errorf("wrote %s but could not load it (launchctl load: %v; bootstrap: %v)", path, err, err2)
		}
	}
	migrateLegacyGlobalUnit(home, profile, "darwin")
	fmt.Printf("installed launchd agent\n  profile: %s\n  plist: %s\n  exec:  %s %s\n  logs:  %s\n",
		profileLabel(profile), path, exe, strings.Join(args, " "), logPath)
	return nil
}

func uninstallLaunchd() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	profile := config.Profile()
	label, plistName, _ := serviceNames(profile)
	path := filepath.Join(home, "Library", "LaunchAgents", plistName)
	_ = runServiceCmd("launchctl", "unload", path)
	uid := os.Getuid()
	_ = runServiceCmd("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", uid, label))
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
	profile := config.Profile()
	_, _, unitName := serviceNames(profile)
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, unitName)
	args := serveArgsFor(profile)
	// Quote each arg for ExecStart.
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, shellQuote(exe))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	desc := "gadak local Jira mirror"
	if p := profileLabel(profile); p != "default" {
		desc = desc + " (profile " + p + ")"
	}
	unit := fmt.Sprintf(`[Unit]
Description=%s
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, desc, strings.Join(parts, " "))

	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return err
	}
	if err := enableSystemdNow(unitName); err != nil {
		return fmt.Errorf("wrote %s but %w\n  (enable lingering or start a user session, then: systemctl --user enable --now %s)\n  exec: %s %s",
			path, err, strings.TrimSuffix(unitName, ".service"), exe, strings.Join(args, " "))
	}
	migrateLegacyGlobalUnit(home, profile, "linux")
	fmt.Printf("installed systemd user unit\n  profile: %s\n  unit:  %s\n  exec:  %s %s\n",
		profileLabel(profile), path, exe, strings.Join(args, " "))
	return nil
}

func enableSystemdNow(unitName string) error {
	_ = runServiceCmd("systemctl", "--user", "daemon-reload")
	if err := runServiceCmd("systemctl", "--user", "enable", "--now", unitName); err != nil {
		return fmt.Errorf("systemctl --user enable --now %s failed: %w", unitName, err)
	}
	return nil
}

func uninstallSystemd() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	profile := config.Profile()
	_, _, unitName := serviceNames(profile)
	path := filepath.Join(home, ".config", "systemd", "user", unitName)
	_ = runServiceCmd("systemctl", "--user", "disable", "--now", unitName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = runServiceCmd("systemctl", "--user", "daemon-reload")
	fmt.Printf("removed systemd user unit %s\n", path)
	return nil
}

// legacyUnitOwnsProfile reports whether a default-named unit file was
// written for this named profile (the D4 overwrite that pre-dated
// per-profile unit names). Used on upgrade to retire that leftover.
func legacyUnitOwnsProfile(body, profile string) bool {
	if profile == "" || profile == "default" || body == "" {
		return false
	}
	// launchd writes each ProgramArgument as its own <string>.
	if strings.Contains(body, "<string>--profile</string>\n    <string>"+profile+"</string>") {
		return true
	}
	return containsProfileArg(body, profile)
}

func containsProfileArg(body, profile string) bool {
	token := "--profile " + profile
	for {
		i := strings.Index(body, token)
		if i < 0 {
			return false
		}
		rest := body[i+len(token):]
		if rest == "" || isArgEnd(rest[0]) {
			return true
		}
		body = rest
	}
}

func isArgEnd(b byte) bool {
	return b == ' ' || b == '\n' || b == '\t' || b == '<' || b == '"'
}

// migrateLegacyGlobalUnit removes a leftover default-named unit that was
// actually this named profile (the pre-D4 global overwrite). Default
// installs are left untouched so an existing gadak.service keeps working.
func migrateLegacyGlobalUnit(home, profile, goos string) {
	if profile == "" || profile == "default" || home == "" {
		return
	}
	var path string
	switch goos {
	case "darwin":
		path = filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
	default:
		path = filepath.Join(home, ".config", "systemd", "user", "gadak.service")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if !legacyUnitOwnsProfile(string(body), profile) {
		return
	}
	if goos == "darwin" {
		_ = runServiceCmd("launchctl", "unload", path)
		uid := os.Getuid()
		_ = runServiceCmd("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", uid, serviceLabel))
	} else {
		_ = runServiceCmd("systemctl", "--user", "disable", "--now", "gadak.service")
	}
	_ = os.Remove(path)
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
