# Install

No Atlassian account:

```bash
brew install midagedev/tap/gadak-cli
gadak init --standalone
gadak create "the thing I just noticed"
gadak serve
```

**Already have Jira?** One [API token](https://id.atlassian.com/manage-profile/security/api-tokens)
covers both Jira and Confluence on the same site. Then `gadak init && gadak sync`,
or the walkthrough in [First run](#first-run).

**Glossary.** A *profile* is the directory on disk (`--profile`,
`~/.gadak/profiles/<name>/`, `gadak profiles`). A *workspace* is that same
profile mounted on the serve origin (`GET /api/v1/workspaces`, `/w/<name>/`,
the sidebar switcher). There is no `gadak workspaces` command — that invocation
prints top-level usage and exits 2.

## How many things am I installing?

**One.** There is a single binary and a single app, and the app contains the
binary. Which one you download depends on how you want to use it, not on how
much you want to install:

| You want | Install | Do you also need the CLI? |
|---|---|---|
| A window to read and triage in | the desktop app | Not until you wire up an agent — and it is already inside the app |
| Your agent to query the mirror | `brew install midagedev/tap/gadak-cli` | That *is* the CLI |
| To live in the terminal | `brew install midagedev/tap/gadak-cli` | Same thing |

The catch worth knowing: macOS does not put an app bundle's contents on your
`PATH`, so after installing only the app, typing `gadak` in a terminal still
says *command not found* even though the binary is right there. One command
fixes it:

```bash
/Applications/Gadak.app/Contents/Resources/bin/gadak install-cli
```

After that `gadak` works everywhere, and it is the same build as the app — not
a second copy to keep in sync. If you already installed via Homebrew, the two
coexist; just avoid leaving a stale binary earlier in `PATH` (see
[Staying current](#staying-current)).

## The ways in

### Homebrew

```bash
brew install midagedev/tap/gadak        # the macOS app (ships the CLI too)
brew install midagedev/tap/gadak-cli    # CLI only — also the Linux path
```

From [`midagedev/homebrew-tap`][tap]. `gadak` is a cask: it installs the
signed, notarized macOS app and puts the same `gadak` binary on your PATH.
`gadak-cli` is the formula for a machine that wants only the command — every
Linux install, and any Mac where you do not want the app. macOS release
binaries are signed with a Developer ID certificate and notarized by Apple —
[`SECURITY.md`](../SECURITY.md) shows how to verify one yourself.

[tap]: https://github.com/midagedev/homebrew-tap

### Arch Linux

`gadak-bin` is **not in the AUR**. New AUR account registration has been closed
upstream since the August 2026 supply-chain incidents, and the `aur-general`
list says there is no manual path in the meantime. The package itself is
written and checked, so publishing is one push whenever registration returns.

Until then, build it from the PKGBUILD in this repository:

```bash
sudo pacman -S --needed base-devel
git clone --depth 1 https://github.com/midagedev/gadak
cd gadak/contrib/aur/gadak-bin
makepkg -si            # fetches the release tarball, verifies sha256, installs
gadak version
```

This installs the same prebuilt release binary as the tarball path above.
`package()` only places files — there is no `build()`, no `prepare()`, no
install hook, so nothing in this package executes code at install time — and
each release archive's sha256 is pinned in the `PKGBUILD`. `options=('!strip')`
keeps the installed `/usr/bin/gadak` byte-identical to the archive member, so
verifying the release checksum still means something after pacman has installed
it. [`contrib/aur/gadak-bin/verify.sh`](../contrib/aur/gadak-bin/verify.sh)
asserts all of that in a throwaway Arch container, from any host with Docker.

Upgrading is the one thing this costs you: `pacman -Syu` cannot update a
package pacman did not get from a repository, so pull and run `makepkg -si`
again. Gadak notices a new release on its own and shows the notes in Settings —
it will not print an upgrade command there on Arch, because there is not an
honest one to print yet.

### Scoop (Windows CLI)

Status: **not in a published bucket; `scoop install` has not been run on a Windows machine.**
<!-- PUBLISH: replace the Status line with:
scoop bucket add gadak https://github.com/midagedev/scoop-gadak
scoop install gadak
-->

The in-repo manifest is [`contrib/scoop/gadak.json`](../contrib/scoop/gadak.json).
It is checked offline (Scoop's schema, sha256 against that tag's
`checksums.txt`, zip members). It is the Windows CLI (`gadak.exe`) — the
same file as `gadak_<version>_windows_amd64.zip` / `windows_arm64` — not
the desktop zip `Gadak-<version>-windows-x64.zip`. Scoop's app name is
`gadak` (Windows-only, that is the command on `PATH`). Homebrew uses
`gadak` for the macOS app cask and `gadak-cli` for this same CLI.

Until the bucket exists, install from the zip under
[Desktop app (Windows)](#desktop-app-windows) or
[Release binary](#release-binary).

### Desktop app (macOS)

Download `Gadak-<version>-arm64.dmg` from the
[latest release](https://github.com/midagedev/gadak/releases/latest) and drag it
to Applications — signed and notarized, Apple Silicon. It is the same web UI in
its own window with **no local server at all**: no port, no address, nothing
new listening. First launch walks through setup in the window.
[`DESKTOP.md`](DESKTOP.md) has the details.

### Desktop app (Windows)

From 0.16, the same GitHub Release attaches a Windows portable zip:
`Gadak-<version>-windows-x64.zip` or `Gadak-<version>-windows-arm64.zip`.
Unzip and run `gadak-desktop.exe`. The pack is a directory (the two exes at
the root), not an installer — 0.16 has no Authenticode certificate, and an
unsigned installer is more friction than a zip.

It is the same web UI in a native Windows window with **no local server**:
no port, no address, nothing new listening (`desktop/main.go`). First launch
walks through setup in the window. The window needs the Evergreen WebView2
runtime (Microsoft documents that Windows 11 includes it, and that many
Windows 10 machines already have it via Edge). If the window never appears,
install the runtime from
<https://developer.microsoft.com/en-us/microsoft-edge/webview2/>.

The build is **unsigned**. Signing is planned ([GDK-211]); this page does not
name a date.

Windows may show one of two dialogs. Neither is a virus finding:

- **Microsoft Defender SmartScreen:** **Windows protected your PC**, then
  **More info** / **Run anyway**. SmartScreen has a per-file override.
- **Smart App Control:** **Smart App Control blocked an app that may be
  unsafe.** Microsoft's FAQ says there is currently **no way to bypass
  Smart App Control for one app**
  ([Smart App Control FAQ](https://support.microsoft.com/en-us/windows/smart-app-control-frequently-asked-questions-285ea03d-fa88-4d56-882e-6698afdb7003)).
  Do **not** turn Smart App Control off.

If the desktop exe is blocked, use the CLI path that has been shipping on
every release: download `gadak_<version>_windows_amd64.zip` (or
`windows_arm64`) and `checksums.txt` from the same release, unzip, put
`gadak.exe` on `PATH`, then:

```powershell
gadak init
gadak sync
gadak serve      # http://gadak.localhost:7777
```

That CLI zip is the reliable Windows route for 0.16. The measured unsigned
CLI (`gadak.exe`) has run without this block.

[`DESKTOP.md`](DESKTOP.md) has the rest of the window.

### Install script

```bash
curl -fsSL https://raw.githubusercontent.com/midagedev/gadak/main/scripts/install.sh | sh
```

Downloads the latest GitHub Release for your OS/arch, verifies `checksums.txt`
(sha256), and installs to `~/.local/bin/gadak` (override with
`GADAK_INSTALL_DIR`). Upgrades in place if a binary is already there.

### Release binary

Download the archive for your OS/arch from
[GitHub Releases](https://github.com/midagedev/gadak/releases), verify against
`checksums.txt`, unpack, and put `gadak` on your `PATH`. Windows CLI archives
are `gadak_<version>_windows_amd64.zip` and `gadak_<version>_windows_arm64.zip`
(goreleaser). The desktop pack is a different file —
`Gadak-<version>-windows-x64.zip` / `windows-arm64` — and is not in
`checksums.txt` (same as the macOS dmg).

### Build from source

Requirements: Go 1.25+, Node.js 20+.

```bash
npm ci && npm run build       # build the web UI into dist/app
go build -o gadak ./cmd/gadak   # embeds it — the binary is the whole install
# or: make build  → bin/gadak

./gadak demo                   # the hosted demo's backlog, served locally
```

A cold build (clone, npm ci, module downloads) takes a few minutes; the hosted
demo needs none of it.

## First run

### Standalone (no Atlassian account)

```bash
gadak init --standalone
gadak create "the thing I just noticed"
gadak serve                    # http://gadak.localhost:7777
```

`gadak init --standalone` writes `config.json` in the profile directory
(`~/.gadak/` by default, or `$GADAK_HOME`) and creates `origin/issuetap.yaml`
there — plain YAML, the origin, the file to back up
(`internal/origin/origin.go` `PersistRel`). It seeds project `STD` and wiki
space `LOC`, and records a default issue type so `gadak create` takes only a
summary (`cmd/gadak/init.go` `initStandalone`). The SQLite file `gadak.db` is
still a cache. The first `gadak sync` against that origin finishes in 0s
(measured; the origin is already local). While `gadak serve` is running, other
gadak processes route writes through it so the persist file has one owner.

### Already have Jira

```bash
gadak serve                    # http://gadak.localhost:7777
```

The first run walks you through it in the browser: paste your site, email, and
token, pick projects from your site's own list, and watch the first sync fill
the mirror. (The desktop app — macOS dmg or the Windows portable zip — runs
the same setup in its own window — no terminal at any point.) If you would
rather stay in the terminal,
`gadak init && gadak sync` does the same thing. `gadak serve` keeps the mirror
fresh in the background whenever a credential is configured (`--no-sync` opts
out). To survive reboot:

```bash
gadak install-service   # launchd (macOS) or systemd --user (Linux)
```

**Mirroring the wiki too** is one config key — the same site, email, and token
already cover it. Add to `~/.gadak/config.json`:

```json
"confluence": { "spaces": ["ENG", "PROD"] }
```

Empty `spaces` means every *global* space; personal spaces only if named.
`gadak sync` then pulls
pages (current version, comments, labels) alongside issues —
`--source jira|confluence|all` narrows a run. Pages land in the same FTS index,
the sidebar grows a DOCS tree, and search answers across both.
`gadak init --standalone` writes `confluence.spaces` as `["LOC"]` (the space
the in-process origin seeds) so the wiki pass is on for a new standalone
workspace.

## Two sites at once

`gadak --profile demo init` keeps a separate credential and mirror under
`~/.gadak/profiles/demo/`. One `gadak serve` then mounts every profile as a
workspace under `/w/<name>/` (full API, reads and writes, opened on first
request), and when there is more than one, the web sidebar grows a WORKSPACES
switcher. Same loopback listener, same single user — `GET /api/v1/workspaces`
never exposes credentials. HTTP mounts are lazy (opened on first request);
**every credentialed profile gets its own watch loop**, not just the one
`serve` started on — same rule as the desktop app
([DESKTOP.md](DESKTOP.md)). Notifications stay on the primary. The disk
inventory is `gadak profiles`; there is no `gadak workspaces` command.

## Staying current

gadak checks GitHub Releases for a newer version once a day (anonymous, cached,
`updateCheck: false` opts out) and says so in the web sidebar — but
four things still catch people, learned the hard way:

1. **A running `gadak serve` keeps its old code.** Upgrading the binary does not
   touch a process that is already up — restart it (or re-run
   `gadak install-service`, which restarts the unit) after an upgrade.
2. **A stale Homebrew tap pins you silently.** If `brew info midagedev/tap/gadak`
   shows an old "stable" after `brew update`, reset the tap:
   `brew untap midagedev/tap && brew tap midagedev/tap && brew upgrade gadak`.
3. **Check what `gadak` actually resolves to.** `which -a gadak` — a leftover
   `go install` build in `~/go/bin` earlier in `PATH` will shadow the brew
   binary forever, versions be damned.
4. **A `makepkg -si` install is invisible to `pacman -Syu`** ([Arch
   Linux](#arch-linux)). pacman only upgrades what a repository gave it, so
   nothing will ever tell you at the package-manager level; pull the repo and
   rebuild.

`gadak --version` against the
[releases page](https://github.com/midagedev/gadak/releases) settles any doubt,
and `gadak doctor` prints the same thing alongside everything else worth
knowing about your install.

## Run it in a container

```bash
docker build -t gadak . && docker run --rm -p 7777:7777 -v gadak-data:/data gadak
```

The process has no authentication by design, so it refuses to bind a
non-loopback address without `--allow-remote` (the image passes it). Only put
it on a network you trust. Config and `gadak.db` live under `/data`.

## Try it without installing anything

The [hosted demo](https://midagedev.github.io/gadak/) is a static build of the
web UI plus a frozen copy of the demo snapshot. No binary, no Jira account, no
trust decision.

To build or preview it yourself:

```bash
make hosted-demo          # → dist/hosted/  (UI + bootstrap/detail/attachments)
make hosted-demo-test     # Playwright smoke (boot, search, detail, image)

mkdir -p dist/pages/gadak && cp -R dist/hosted/. dist/pages/gadak/
npx serve dist/pages -l 4173     # http://127.0.0.1:4173/gadak/
```

Limits of the hosted snapshot: read-only (writes return `501 demo_read_only`);
server full-text search (`search/`) is unavailable (client-side typing search
still works over the issue pool); no live sync or identity. The wiki mirror
needs the binary demo — the static snapshot carries issues only.

## About the demo data

`gadak demo` serves `examples/demo.db` — a fictional SaaS company: 534 issues
(15 of them epics parenting 163 issues) across three projects, plus 71 wiki
pages in two spaces, some in Korean because search should survive CJK. It is
also what the test suite and the README's clips run against, so what you see is
what CI checks.

[GDK-211]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-211
