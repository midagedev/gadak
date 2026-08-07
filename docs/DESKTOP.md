# The desktop app

Scry.app is the same scry — the web UI in its own macOS window, over the same
mirror, with one structural difference from `scry serve`: **there is no local
server.** The window talks to the mirror in-process, so there is no port, no
address, no port conflict, and nothing new listening on your machine. Launch
it twice and the running window comes forward instead.

If you already use the CLI, nothing changes underneath: the app reads and
writes the same `~/.scry` profiles, and WAL means the app, `scry tui`, a
`serve` instance, and your agent can hold the file at once.

## Install

Download `Scry-<version>-arm64.dmg` from the
[latest release](https://github.com/midagedev/scry/releases/latest), drag
Scry.app to Applications. The dmg is Developer ID-signed and notarized
(macOS, Apple Silicon; Intel and other platforms use the
[CLI](../README.md#install)).

First launch walks you through setup in the window — Jira site, email, API
token (from <https://id.atlassian.com/manage-profile/security/api-tokens>),
pick projects, watch the first sync land. No terminal involved.

## Where the CLI fits

The app is for reading and triaging; the CLI is how agents and scripts read
the same mirror. You don't choose one — the app **ships the CLI inside it**.

**1. From the app (preferred).** macOS menu **Tools → Install Command Line
Tool…** creates the symlink itself (user-writable location; no sudo, no
terminal). If the install directory is not already on your PATH, the app
copies a one-line shell snippet to the clipboard so you can paste it into
your shell rc. When it succeeds, run `scry mcp install claude` once so your
agent reads the same mirror the window shows.

**2. From a terminal (same result).** If you already have a shell open:

```bash
/Applications/Scry.app/Contents/Resources/bin/scry install-cli
scry mcp install claude
```

`install-cli` (and the menu item) prefer a directory already on PATH —
`~/.local/bin` when it exists there, else `/usr/local/bin` when writable,
else `~/.local/bin` (created; PATH hint printed). Pass `--dir` to override.
No sudo is required for the default locations.

**3. Manual link (last resort):**

```bash
sudo ln -sf "/Applications/Scry.app/Contents/Resources/bin/scry" /usr/local/bin/scry
```

That is the whole relationship: the window is your view of the mirror, the
SQLite file is your agent's view, and they are the same bytes. A brew install
(`brew install midagedev/tap/scry`) works too and the two can coexist — the
binaries are identical per release; just avoid pointing a PATH at a stale copy.

## Profiles

The app opens the default profile. To pin a window to another mirror, launch
with the profile in the environment:

```bash
SCRY_PROFILE=work open -a Scry
```

One window per process, one profile per window — the in-app workspace
switcher that `serve` offers under `/w/<profile>/` is not in the app yet.

## Good to know

- **Sync loops:** the app runs its own background sync, exactly like `serve`.
  If you keep a `serve` running *on the same profile* alongside the app, both
  will poll Jira — harmless (SQLite serializes writers) but twice the API
  volume. Run one or the other per profile.
- **Security posture:** unchanged from [SECURITY.md](../SECURITY.md), minus
  the local port: with no listener there is nothing for another local process
  or a hostile web page to connect to. The webview reaches the mirror through
  an in-process handler.
- **Updates:** the app checks GitHub Releases once a day like the CLI
  (`updateCheck: false` disables it). Updating is replacing Scry.app.
- **Uninstall:** trash Scry.app; the mirror and credential live in `~/.scry`,
  so offboarding fully is still `rm -rf ~/.scry`.

## Building from source

See [`desktop/README.md`](../desktop/README.md) — `desktop/build-app.sh`
produces the same bundle locally, unsigned by default.
