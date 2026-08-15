# gadak desktop

A native macOS window around the same embedded web UI — with **no TCP
listener at all**. The Wails asset server calls straight into the
`server.Handler` that `gadak serve` mounts, so ports, addresses, and port
conflicts stop existing as UX. A second launch focuses the running window
(single-instance lock) instead of hunting for a free port.

Status: **macOS app bundle + signed/notarized dmg on tag releases** (arm64), on
`wails/v3` (beta). The module stays a nested Go module so the wails dependency
tree and its CGO requirement never touch the CLI build or the non-macOS CI
matrix. No `wails3` CLI: plain `go build`, no bindings, HTTP only.

## Build

```bash
npm run build                 # at the repo root — the app embeds dist/app
desktop/build-app.sh          # → desktop/build/Gadak.app
desktop/build-app.sh --dmg    # → desktop/build/Gadak-<version>-arm64.dmg as well
```

The bundle includes:

- `Contents/MacOS/gadak-desktop` — Wails host (App + Edit menus so ⌘C/V/X/A work)
- `Contents/Resources/bin/gadak` — same CLI binary as the brew formula, for agent
  wiring without a separate install

Put the CLI on your PATH:

```bash
sudo ln -sf "/Applications/Gadak.app/Contents/Resources/bin/gadak" /usr/local/bin/gadak
```

Then `gadak mcp install claude` (and friends) work from a desktop-only install.

### Signing and notarization

Unsigned by default (local development). Set env vars to opt in:

| Variable | Meaning |
| --- | --- |
| `GADAK_SIGN_IDENTITY` | codesign identity (`Developer ID Application: …`) |
| `GADAK_NOTARY_KEY` | path to App Store Connect API key `.p8` |
| `GADAK_NOTARY_KEY_ID` | key id |
| `GADAK_NOTARY_ISSUER_ID` | issuer id |

With `GADAK_SIGN_IDENTITY`, `build-app.sh` signs both nested binaries under a
hardened runtime, then the `.app`, and the `.dmg` when `--dmg` is passed.
Notarization runs only when `--dmg` and all three `GADAK_NOTARY_*` vars are set
(`notarytool submit --wait`, then `stapler staple` on the dmg and app).

CI: `.github/workflows/desktop-release.yml` on `v*` tags (and
`workflow_dispatch` for dry runs). It reuses the same secrets as the
goreleaser job (`MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD`, `MACOS_NOTARY_KEY`,
`MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_ISSUER_ID`). Missing secrets skip
signing; the job still produces an unsigned dmg artifact. Tag pushes upload
the dmg onto the GitHub Release.

Current ship shape is **arm64 only** (`Gadak-<ver>-arm64.dmg`). A universal
(arm64+amd64 lipo) build is a later stretch.

## Updates

The dmg (or `brew install --cask gadak`) is how you install, and how you
upgrade. The app does not download a replacement bundle or swap itself.

It does the same once-a-day anonymous GitHub Releases lookup as `gadak serve`
(`internal/selfupdate`, cached on disk as `update-check.json`). When the
cached tag is newer than the running build, the sidebar shows a banner that
links to the release notes. `updateCheck: false` turns the lookup off — that
is the only GitHub traffic, so the opt-out restores outbound traffic to Jira
only. Dev builds (`dev` / `0.0.0-dev`) never check.

Install a newer build with:

```bash
brew upgrade --cask gadak
```

or download `Gadak-<version>-arm64.dmg` from the
[latest release](https://github.com/midagedev/gadak/releases/latest).

`build-app.sh --dmg` produces only the dmg. It must not emit
`gadak-desktop-darwin-<arch>.zip`: v0.14.0 apps match that name exactly and
would try to self-swap.

## How it hangs together

- `main.go` opens the profile's mirror (`GADAK_PROFILE` respected), builds the
  API handler, starts the sync loop and update check — the same wiring as
  `cmd/gadak serve`, minus the listener and workspace mounts.
- v3's asset server takes one handler, not a file system plus a fallback, so
  `assetHandler` in `main.go` does that split: a GET that names a file in the
  embedded bundle is served from it, everything else goes to `fallbackHandler`.
- The webview's requests carry `wails://` Origins the browser guard would
  reject; the fallback handler strips them and presents as loopback. That is
  not a guard bypass in the threat-model sense: these requests never crossed
  a network boundary, and the webview is the only client that can reach the
  handler.
- `handler_test.go` pins the three seams (config.json, guard passage, SPA
  fallback). Run with `GADAK_PROFILE=demo go test -tags desktop,production ./...`
  — it refuses to open the default profile's mirror.

## Known limits

- No workspace switcher (`/w/<profile>/` mounts) — single profile per window.
- Onboarding for a machine with no credential is a separate track; until that
  lands, `gadak init` (or the bundled CLI) remains the setup path.
- macOS only; building still needs the Xcode command line tools. (The
  `UniformTypeIdentifiers` link flag `build-app.sh` used to pass by hand is
  gone — wails v3 declares that framework itself.)
