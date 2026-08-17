# gadak desktop

A native window around the same embedded web UI — with **no TCP
listener at all**. The Wails asset server calls straight into the
`server.Handler` that `gadak serve` mounts, so ports, addresses, and port
conflicts stop existing as UX. A second launch focuses the running window
(single-instance lock) instead of hunting for a free port.

Status: **macOS app bundle + signed/notarized dmg on tag releases** (arm64), on
`wails/v3` (beta). Linux is a from-source AppImage pack (`desktop/build-linux.sh`);
this tree's tag workflow does not upload that file. The module stays a nested
Go module so the wails dependency tree and its CGO requirement never touch the
CLI build or the non-macOS CI matrix. No `wails3` CLI: plain `go build`, no
bindings, HTTP only.

## Build

```bash
npm run build                 # at the repo root — the app embeds dist/app
desktop/build-app.sh          # → desktop/build/Gadak.app
desktop/build-app.sh --dmg    # → desktop/build/Gadak-<version>-arm64.dmg as well
```

Linux (must run on Linux; CGO plus GTK4 / WebKitGTK 6.0 headers — see below):

```bash
desktop/build-linux.sh              # → desktop/build/Gadak.AppDir
desktop/build-linux.sh --appimage   # → desktop/build/Gadak-<version>-x86_64.AppImage as well
```

The AppImage name uses the AppImage / `uname -m` labels (`x86_64`, `aarch64`),
not the macOS dmg labels (`amd64`, `arm64`). Both scripts stamp the version
from the same `version=` line in `desktop/build-app.sh` (`git describe --tags --always`).

`build-linux.sh` exits 64 on usage / unknown arguments and 69 when a required
tool is missing (`go`, `pkg-config`, `cc`, `magick`/`convert`/`ffmpeg`, the
`gtk4` and `webkitgtk-6.0` pkg-config modules, and `appimagetool` when
`--appimage` is passed).

The bundle includes:

- `Contents/MacOS/gadak-desktop` — Wails host (App + Edit menus so ⌘C/V/X/A work)
- `Contents/Resources/bin/gadak` — same CLI binary as the brew formula, for agent
  wiring without a separate install

Put the CLI on your PATH:

```bash
sudo ln -sf "/Applications/Gadak.app/Contents/Resources/bin/gadak" /usr/local/bin/gadak
```

Then `gadak mcp install claude` (and friends) work from a desktop-only install.

The Linux AppDir includes the same two binaries under `usr/bin/`
(`gadak-desktop` and `gadak`). `AppRun` prepends that directory to `PATH` for
the running app. A `.desktop` file in the AppDir declares
`MimeType=x-scheme-handler/gadak`; this script does not register the handler
with the host.

### Linux build prerequisites

wails v3 (`v3.0.0-beta.6`, `desktop/go.mod`) compiles the Linux host with
`#cgo pkg-config: gtk4 webkitgtk-6.0`. `CGO_ENABLED=0` does not compile (see
the comment on the desktop job in `.github/workflows/ci.yml`). Do not pass
`-tags gtk3`: that is the webkit2gtk 4.1 legacy stack, and wails plans to
remove it in v3.1.

Development packages as listed by that wails release's doctor (not installed
or compiled against in this repository's CI):

| Distro family | GTK4 | WebKitGTK 6.0 |
| --- | --- | --- |
| Debian / Ubuntu | `libgtk-4-dev` | `libwebkitgtk-6.0-dev` |
| Fedora | `gtk4-devel` | `webkitgtk6.0-devel` |
| Arch | `gtk4` | `webkitgtk-6.0` |
| openSUSE | `gtk4-devel` | `webkitgtk-6_0-devel` |

Also required: `pkg-config`, a C compiler, and one of ImageMagick (`magick` /
`convert`) or `ffmpeg` to resize `docs/media/logo.png`. `--appimage` needs
`appimagetool` already on `PATH`. The script does not download packers.

`build-linux.sh --appimage` produces only the AppImage. It must not emit a
zip: v0.14.0 apps match `gadak-desktop-*-<arch>.zip` and would try to
self-swap (same rule as `build-app.sh --dmg`).

### NVIDIA and Wayland

wails v3's GTK4 Linux host (the library this module links) sets
`WEBKIT_DISABLE_DMABUF_RENDERER=1` at process start when `/sys/module/nvidia`
exists. Their comment says this avoids blank windows and `Error 71 (Protocol
error)` on both X11 and Wayland, because the proprietary NVIDIA driver fails
`gbm_bo_map()` when importing DMA-BUF. You can export the same variable
yourself if a window stays blank.

This repository has not run the desktop app on a Linux NVIDIA or Wayland
machine. Other compositor-specific workarounds are unverified.

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
- macOS is the shipped desktop artifact (signed/notarized dmg). Linux is a
  from-source AppImage pack; this repository's CI does not build or run it.
  The in-app Jira/Confluence pane (`embed_darwin.go`) is still macOS-only —
  other platforms get the stub in `embed_other.go`.
- macOS builds still need the Xcode command line tools. (The
  `UniformTypeIdentifiers` link flag `build-app.sh` used to pass by hand is
  gone — wails v3 declares that framework itself.)
