# Install

Atlassian Cloud only. You need an API token from
<https://id.atlassian.com/manage-profile/security/api-tokens> — one token
covers both Jira and Confluence on the same site.

## How many things am I installing?

**One.** There is a single binary and a single app, and the app contains the
binary. Which one you download depends on how you want to use it, not on how
much you want to install:

| You want | Install | Do you also need the CLI? |
|---|---|---|
| A window to read and triage in | the desktop app | Not until you wire up an agent — and it is already inside the app |
| Your agent to query the mirror | `brew install midagedev/tap/gadak` | That *is* the CLI |
| To live in the terminal | `brew install midagedev/tap/gadak` | Same thing |

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
brew install midagedev/tap/gadak
```

macOS and Linux, from [`midagedev/homebrew-tap`][tap]. A formula, not a cask:
gadak is a single CLI binary, which is what formulas are for. macOS release
binaries are signed with a Developer ID certificate and notarized by Apple —
[`SECURITY.md`](../SECURITY.md) shows how to verify one yourself.

[tap]: https://github.com/midagedev/homebrew-tap

### Desktop app (macOS)

Download `Gadak-<version>-arm64.dmg` from the
[latest release](https://github.com/midagedev/gadak/releases/latest) and drag it
to Applications — signed and notarized, Apple Silicon. It is the same web UI in
its own window with **no local server at all**: no port, no address, nothing
new listening. First launch walks through setup in the window.
[`DESKTOP.md`](DESKTOP.md) has the details.

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
`checksums.txt`, unpack, and put `gadak` on your `PATH`.

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

```bash
gadak serve                    # http://gadak.localhost:7777
```

The first run walks you through it in the browser: paste your site, email, and
token, pick projects from your site's own list, and watch the first sync fill
the mirror. (The desktop app runs the same setup in its own window — no
terminal at any point.) If you would rather stay in the terminal,
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

## Two sites at once

Profiles: `gadak --profile demo init` keeps a separate credential and mirror
under `~/.gadak/profiles/demo/`. One `gadak serve` then serves every profile:
each mounts under `/w/<name>/` (full API, reads and writes, opened on first
request), and when there is more than one, the web sidebar grows a WORKSPACES
switcher. Same loopback listener, same single user — the workspace list never
exposes credentials. HTTP mounts are lazy (opened on first request);
**every profile that has a credential gets its own watch loop**, not just
the one `serve` started on — same rule as the desktop app
([DESKTOP.md](DESKTOP.md)). Notifications stay on the primary.

## Staying current

gadak checks GitHub Releases for a newer version once a day (anonymous, cached,
`updateCheck: false` opts out) and says so in the web sidebar — but
three things still catch people, learned the hard way:

1. **A running `gadak serve` keeps its old code.** Upgrading the binary does not
   touch a process that is already up — restart it (or re-run
   `gadak install-service`, which restarts the unit) after an upgrade.
2. **A stale Homebrew tap pins you silently.** If `brew info midagedev/tap/gadak`
   shows an old "stable" after `brew update`, reset the tap:
   `brew untap midagedev/tap && brew tap midagedev/tap && brew upgrade gadak`.
3. **Check what `gadak` actually resolves to.** `which -a gadak` — a leftover
   `go install` build in `~/go/bin` earlier in `PATH` will shadow the brew
   binary forever, versions be damned.

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
