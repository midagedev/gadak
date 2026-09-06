# The desktop app

The desktop app is the same gadak — the web UI in its own window, over the same
mirror, with one structural difference from `gadak serve`: **there is no local
server.** The window talks to the mirror in-process, so there is no port, no
address, no port conflict, and nothing new listening on your machine
(`desktop/main.go`). Launch it twice and the running window comes forward
instead.

On macOS the window is `Gadak.app` (no title bar: the traffic lights sit in
the sidebar). On Windows it is `gadak-desktop.exe` from the portable zip,
with ordinary Windows chrome.

If you already use the CLI, nothing changes underneath: the app reads and
writes the same `~/.gadak` profiles, and WAL means the app, a
`serve` instance, and your agent can hold the file at once.

There is no title bar. It would have spent 32px saying "Gadak" above a sidebar
that already says `gadak`, so the window controls sit in that sidebar row
instead — which is also where you grab the window to move it.

## Install

**macOS.** Download `Gadak-<version>-arm64.dmg` from the
[latest release](https://github.com/midagedev/gadak/releases/latest), drag
Gadak.app to Applications. The dmg is Developer ID-signed and notarized
(Apple Silicon).

**Windows.** The app is on the [Microsoft Store](https://apps.microsoft.com/detail/9NZW91TXH36G) — Store-signed,
so SmartScreen and Smart App Control let it through, and since 0.20.2 the
Store package puts the `gadak` command on `PATH` too (an app-execution
alias), so a Store install is the CLI as well. Every release since 0.16
also attaches `Gadak-<version>-windows-x64.zip` or
`Gadak-<version>-windows-arm64.zip`: unzip, run `gadak-desktop.exe`. That zip
is unsigned by decision, not by omission — [GDK-211], reasoning in
[WINDOWS-SIGNING.md](WINDOWS-SIGNING.md). If Windows blocks the exe, install
from the Store, or use the CLI zip and `gadak serve`. Do not turn Smart App
Control off.
The wording and the CLI fallback live in
[INSTALL.md](INSTALL.md#desktop-app-windows).

Intel Macs have no shipped dmg (tag releases are arm64 only;
`desktop/build-app.sh` packs the host arch). Linux is a from-source
AppDir/AppImage (`desktop/build-linux.sh`); the tag workflow does not
upload it. Either build that pack or use the [CLI](../README.md#install).
The platforms that ship, and which of them receive the `gadak://` event,
are the table in [`desktop/README.md`](../desktop/README.md).

First launch walks you through setup in the window — Jira site, email, API
token (from <https://id.atlassian.com/manage-profile/security/api-tokens>),
pick projects, watch the first sync land. No terminal involved.

## Where the CLI fits

The app is for reading and triaging; the CLI is how agents and scripts read
the same mirror. You don't choose one — the app **ships the CLI inside it**.

**1. From the app (preferred).** **Settings → Integrations** runs the install
itself (user-writable location; no sudo, no terminal) — a symlink on macOS and
Linux, a copy on Windows, where symlinks need elevation. If the install
directory is not already on your PATH, the app
copies a one-line shell snippet to the clipboard so you can paste it into
your shell rc. When it succeeds, the next step is `gadak skill install`
(Claude Code). For shell-less hosts like Claude Desktop: `gadak mcp install claude`.

**2. From a terminal (same result).** If you already have a shell open:

```bash
/Applications/Gadak.app/Contents/Resources/bin/gadak install-cli
```

`install-cli` (and the menu item) prefer a directory already on PATH —
`~/.local/bin` when it exists there, else `/usr/local/bin` when writable,
else `~/.local/bin` (created; PATH hint printed). Pass `--dir` to override.
No sudo is required for the default locations.

For a coding agent that has a shell (Claude Code):

```bash
gadak skill install
```

For hosts without a shell (Claude Desktop):

```bash
gadak mcp install claude
```

**3. Manual link (last resort):**

```bash
sudo ln -sf "/Applications/Gadak.app/Contents/Resources/bin/gadak" /usr/local/bin/gadak
```

That is the whole relationship: the window is your view of the mirror, the
SQLite file is your agent's view, and they are the same bytes. A brew install
(`brew install midagedev/tap/gadak`) works too and the two can coexist — the
binaries are identical per release; just avoid pointing a PATH at a stale copy.

## The `gadak://` scheme

The app registers `gadak://`, so a piece of gadak can be handed over as a link
instead of a command:

```
gadak://view?issue=NMB-140                    # the default mirror
gadak://view/w/work?ks=NMA-1,NMA-2            # a key list on the "work" mirror
gadak://view/w/oss?pj=GDK&sc=inprogress       # a filtered list
```

Clicking one raises the app — launching it first if it was closed — and shows
that view. You do not have to build these by hand: every `gadak views open`
prints the link for what it just opened, as a `deeplink` line and as a
`"deeplink"` field under `--json`.

This is what an agent hands you when it has built a list. Its alternative was
to run `gadak views open` itself, which needs a shell and a running `serve`,
and which yanks the window forward mid-sentence rather than letting you
choose. `gadak views open` now uses the same link internally: one `open` both
raises the app and says which view to show.

### Issue links

A single issue is the `view` action with the place parameter `issue=KEY`:

```
gadak://view?issue=NMB-140              # default mirror
gadak://view/w/oss?issue=NMB-140        # named profile
```

`/w/<profile>` is the same rule as the rest of this section: omit it on the
default mirror; include it for any other profile. The http form a running
`serve` answers is that hash on the serve origin:

```
http://127.0.0.1:7777/#/?issue=NMB-140
http://127.0.0.1:7777/w/oss/#/?issue=NMB-140
```

`gadak issue NMB-140 --link` prints the `gadak://` form always, and the http
form when a serve is discoverable the same way `gadak views open` finds one.
`--json` names the fields `deeplink` and `web`, matching `views open --json`.

### The grammar

```
gadak://<action>[/w/<profile>][/<subject>][?<params>]
```

`<action>` is what the link asks for; `/w/<profile>` is the same workspace
segment the web UI uses, in the same position; `<subject>` is what the action
acts on; `<params>` is a view hash.

`view` is the only action today, and it goes further than its name suggests:
the hash carries every filter and display setting, plus which panel and which
screen you are on — `issue=KEY`, `doc=KEY`, `space=`, and so on. Anything the
web app can put in its URL, a `gadak://view` link can carry, with no change
here. Which is why the work of adding surfaces is on the UI side ([GDK-124]),
not on the scheme.

A second action is for something the hash cannot express — a place with no
URL, or a different kind of address entirely. Each is then one entry in the
table in `desktop/deeplink.go`.

The split is deliberate. A link lives in a chat log forever and is opened by
whatever version happens to be installed, so the **grammar** must not change
while the **action list** grows. A gadak that meets a link for an action it
does not have says so — *"this link needs a newer Gadak"* — rather than
calling the link malformed.

### Read-only by construction

Any web page can put a `gadak://` link on it, so the scheme carries no verb:
a link says *where to go*, never what to do. The worst a hostile link can
achieve is that you look at the wrong thing. Links are refused if they name
an action that is not a plain word, carry a profile or subject that is not a
plain name, try to traverse a path, or exceed the size limits
(`internal/deeplink`). Actions that submit or write will not be added.

macOS registers the scheme from the bundle's `Info.plist`. The Windows
portable zip does not write `HKCU\SOFTWARE\Classes\gadak` — the pack never
touches the registry. `gadak-desktop.exe` registers that key on first launch
and rewrites it when its path changes. Unregister with
`gadak-desktop.exe --unregister-gadak-protocol`. On Linux, and on Windows
when you are using `gadak serve` instead of the exe, the `web` field from
`gadak views open --json` is the link to hand over — it needs a running
`serve`.

## Profiles

The app opens the default profile. To pin a window to another mirror, launch
with the profile in the environment:

```bash
GADAK_PROFILE=work open -a Gadak
```

Or switch inside the window: the sidebar lists every profile under
**Workspaces** and each one opens in place. The list appears only when you have
more than one profile, so a single-mirror install never sees it. The profile in
the environment is still the one the window starts on, and the one every write
goes to until you switch.

## Good to know

- **Sync loops:** the app runs its own background sync, exactly like `serve` —
  and since every profile you can switch to is one you can read, **every
  profile that has a credential gets its own loop**, not just the one the
  window started on. A mirror you can see is a mirror that stays fresh; the
  cost is that Jira API volume scales with how many profiles you have
  configured, whether or not you look at them today.
  Keeping a `serve` running alongside the app now multiplies rather than
  doubles: both processes poll every credentialed profile. Run one or the
  other. `gadak serve --no-sync` turns off all of them at once.
- **Security posture:** unchanged from [SECURITY.md](../SECURITY.md), minus
  the local port: with no listener there is nothing for another local process
  or a hostile web page to connect to. The webview reaches the mirror through
  an in-process handler.
- **Updates:** the app checks GitHub Releases once a day like the CLI
  (`updateCheck: false` disables it) and shows a sidebar banner when a newer
  release exists. Installing it is `brew upgrade --cask gadak`, downloading
  the new dmg, or replacing the Windows portable-zip directory with a newer
  zip — the app does not swap itself.
- **Uninstall:** trash Gadak.app; the mirror and credential live in `~/.gadak`,
  so offboarding fully is still `rm -rf ~/.gadak` (PowerShell:
  `Remove-Item -Recurse -Force $HOME\.gadak`).

## Building from source

See [`desktop/README.md`](../desktop/README.md) — `desktop/build-app.sh`
produces the same bundle locally, unsigned by default.

[GDK-124]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-124
[GDK-211]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-211
