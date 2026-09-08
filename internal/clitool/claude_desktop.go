// Claude Desktop (the desktop app, not the claude CLI / Claude Code) keeps its
// MCP config at a fixed per-OS path. The path has one owner — this file —
// because two surfaces need it: `gadak mcp install claude-desktop` writes the
// file and the desktop Integrations row reads it, and the two must never
// disagree about where Claude Desktop looks.
package clitool

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// claudeDesktopConfigFile is the file name on every OS.
const claudeDesktopConfigFile = "claude_desktop_config.json"

// ClaudeDesktopConfigPathFor returns the Claude Desktop config path for a
// GOOS value, with every lookup injected so the Windows and Linux branches
// are testable on any host (same shape as ResolveFor). These are the paths
// Anthropic documents for Claude Desktop; docs/MCP.md names the macOS one.
//
//	darwin:         <home>/Library/Application Support/Claude/claude_desktop_config.json
//	windows:        <appdata>/Claude/claude_desktop_config.json (empty appdata is an error)
//	linux, others:  <xdg>/Claude/claude_desktop_config.json when xdg is set,
//	                else <home>/.config/Claude/claude_desktop_config.json
func ClaudeDesktopConfigPathFor(goos, home, appdata, xdg string) (string, error) {
	switch goos {
	case "darwin":
		if home == "" {
			return "", errors.New("cannot locate Claude Desktop config: home directory is not set")
		}
		return filepath.Join(home, "Library", "Application Support", "Claude", claudeDesktopConfigFile), nil
	case "windows":
		if appdata == "" {
			return "", errors.New("cannot locate Claude Desktop config: APPDATA is not set")
		}
		return filepath.Join(appdata, "Claude", claudeDesktopConfigFile), nil
	default:
		if xdg != "" {
			return filepath.Join(xdg, "Claude", claudeDesktopConfigFile), nil
		}
		if home == "" {
			return "", errors.New("cannot locate Claude Desktop config: neither XDG_CONFIG_HOME nor a home directory is set")
		}
		return filepath.Join(home, ".config", "Claude", claudeDesktopConfigFile), nil
	}
}

// ClaudeDesktopConfigPath is ClaudeDesktopConfigPathFor for this process:
// home from os.UserHomeDir, appdata from APPDATA, xdg from XDG_CONFIG_HOME.
// An unresolvable home matters only on the branches that need it — the For
// function decides which error, so a Windows process without a home still
// resolves through APPDATA.
func ClaudeDesktopConfigPath() (string, error) {
	return ClaudeDesktopConfigPathOn(runtime.GOOS)
}

// ClaudeDesktopConfigPathOn is ClaudeDesktopConfigPath for an explicit GOOS:
// the environment lookups stay this process's, the branch is the caller's.
// A catalogue that describes one OS (integrations.listFor) needs exactly
// this — without it, the row calls ClaudeDesktopConfigPath and answers for
// the host no matter which GOOS was asked for.
func ClaudeDesktopConfigPathOn(goos string) (string, error) {
	home, _ := os.UserHomeDir()
	return ClaudeDesktopConfigPathFor(goos, home, os.Getenv("APPDATA"), os.Getenv("XDG_CONFIG_HOME"))
}
