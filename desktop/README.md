# gadak desktop

A native window around the same embedded web UI — with **no TCP
listener at all**. The Wails asset server calls straight into the
`server.Handler` that `gadak serve` mounts, so ports, addresses, and port
conflicts stop existing as UX. A second launch focuses the running window
(single-instance lock) instead of hunting for a free port.

The pack scripts on disk are the ship-shape owner; `coldStartDecisionFor` in
`main.go` is the deep-link delivery owner ([GDK-293]). This table summarises
both so the rest of the tree can point here instead of restating either.

| GOOS | Pack | Shipped on tag | `ApplicationLaunchedWithUrl` |
| --- | --- | --- | --- |
| darwin | `desktop/build-app.sh` → `Gadak-<ver>-arm64.dmg` (signed/notarized) | yes | yes (Apple Event; argv is not applied) |
| windows | `desktop/build-windows.ps1` → `Gadak-<ver>-windows-<x64\|arm64>.zip` (unsigned; [GDK-211]) | yes (from 0.16) | yes when argv is exactly one `://` argument (wails `pkg/application/application_windows.go`); otherwise argv |
| linux | `desktop/build-linux.sh` → AppDir / AppImage | no — from-source only | yes when argv is exactly one `://` argument (wails `pkg/application/application_linux.go`; wailsapp/wails#6000 landed in beta.10); otherwise argv |

The in-app Jira/Confluence browse pane is still darwin-only (`embed_darwin.go`; other GOOS use the stub in `embed_other.go`).

The module stays a nested Go module so the wails dependency tree and its CGO requirement
never touch the CLI build or the non-macOS CI matrix. No `wails3` CLI: plain
`go build`, no bindings, HTTP only. On `wails/v3` (beta).

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

Windows (cross-compile with `CGO_ENABLED=0`; Authenticode and a missing-WebView2 check still need a Windows host — see WebView2 below):

```powershell
desktop/build-windows.ps1   # → desktop/build/Gadak-<version>-x64/
```

The Windows directory name uses the Windows pack labels (`x64`, `arm64`), not
the macOS dmg labels (`amd64`, `arm64`) or the AppImage / `uname -m` labels
(`x86_64`, `aarch64`). All three scripts stamp the version from the same
`version=` line in `desktop/build-app.sh` (`git describe --tags --always`).

`build-linux.sh` and `build-windows.ps1` exit 64 on usage / unknown arguments
and 69 when a required tool is missing. For Windows that is `go` (and `git`
when Git Bash is not on `PATH` to eval the `version=` line). `build-linux.sh`
also needs `pkg-config`, `cc`, `magick`/`convert`/`ffmpeg`, the `gtk4` and
`webkitgtk-6.0` pkg-config modules, and `appimagetool` when `--appimage` is
passed.

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

The Windows portable directory includes the same two binaries at the root
(`gadak-desktop.exe` and `gadak.exe`). `gadak.exe install-cli` copies
`gadak.exe` onto PATH (Windows has no default symlink right). Protocol-handler
registration (`HKCU\SOFTWARE\Classes\gadak`) is not part of this pack:
`gadak-desktop.exe` writes that key on first launch and rewrites it when the
exe path changes. To remove it: `gadak-desktop.exe --unregister-gadak-protocol`.

### Linux build prerequisites

wails v3 (`v3.0.0-beta.12`, `desktop/go.mod`) compiles the Linux host with
`#cgo pkg-config: gtk4 webkitgtk-6.0`. `CGO_ENABLED=0` does not compile (see
the comment on the desktop job in `.github/workflows/ci.yml`). Do not pass
`-tags gtk3`: that is the webkit2gtk 4.1 legacy stack, and wails plans to
remove it in v3.1. GTK4 `gtk_application_new` uses `G_APPLICATION_NON_UNIQUE`
(`pkg/application/linux_cgo.h`); second-launch focusing is wails
`SingleInstanceOptions` (session-bus name on Linux), not GTK uniqueness.

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
self-swap (same rule as `build-app.sh --dmg`). `build-windows.ps1` writes
only the directory tree for the same reason.

### Windows build prerequisites

wails v3 (`v3.0.0-beta.12`, `desktop/go.mod`) talks to WebView2 over COM. The
pack script sets `CGO_ENABLED=0`. This script has not been executed on a
Windows machine in this repository (the authoring runner is darwin).

Required: Go, and `dist/app` from `npm run build` at the repo root. Git is
required only when `bash` is missing, so the script can run the same
`git describe --tags --always` formula as `desktop/build-app.sh`.

### WebView2

**Decision: Evergreen runtime, not a Fixed Version bundle.** A Fixed Version
tree is hundreds of MB, and this pack is a directory — there is no installer
to keep a private runtime current. Microsoft documents that Windows 11
includes the Evergreen runtime and that many Windows 10 machines already
have it via Edge; **this repository has not checked either claim on a
Windows machine.**

What happens if the runtime is missing is taken from the wails v3.0.0-beta.12
source this module links, **not from launching `gadak-desktop.exe` on a
machine without WebView2** (that has not been done):

- `webviewloader` reports `no webview2 found`
  (`internal/webview2/webviewloader/find_dll.go`).
- `Chromium.Embed` waits at most 30s for the controller (`embedTimeout` /
  `pumpUntilInited` in `internal/webview2/pkg/edge/chromium.go`). A
  controller that never becomes ready calls `errorCallback` with a timeout
  instead of blocking forever on `GetMessageW`.
- `Chromium.errorCallback` still calls `os.Exit(1)` after the handler
  (`internal/webview2/pkg/edge/chromium.go`).
- `gadak-desktop` sets an `ErrorHandler` that runs `desktopFatal`
  (`main.go`): it first runs the same `shutdown` a clean exit runs — wails
  `os.Exit(1)`s past every defer, so this is the only place the terminals
  get reaped and standalone persist gets flushed on a fatal (GDK-917/348) —
  then `handleDesktopFatal` shows a `MessageBoxW` on Windows
  (`fatal_windows.go`) and writes stderr. wails still `os.Exit(1)` after the
  handler returns, so there is still no download dialog — the process exits.
  The silent half is handled and cleanup now runs; the exit is not stopped.

Install the Evergreen runtime from
<https://developer.microsoft.com/en-us/microsoft-edge/webview2/>.

### SmartScreen

The portable exe is **unsigned**. Signing is a separate decision ([GDK-211]).
Windows SmartScreen is expected to warn on a first download of an unsigned
exe (`Windows protected your PC` / More info / Run anyway). Reputation is
built after distribution, not before. This dialog has **not** been captured
on a Windows machine in this repository — treat the wording as the usual
SmartScreen unsigned-download prompt, not as a measured screenshot.

`build-windows.ps1` produces only the directory. It must not emit a zip:
v0.14.0 apps match `gadak-desktop-*-<arch>.zip` and would try to
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
- A boot failure before any window exists (a mirror schema refusal from a
  newer gadak, an unreadable config, a locked DB) goes through `bootFatal` in
  `main.go`: the same stderr + log line the old `log.Fatal`
  wrote, then a native dialog carrying the error verbatim — `MessageBoxW` on
  Windows (the `fatal_windows.go` primitive), `osascript display dialog` on
  macOS. Not a wails dialog: before `application.New`, `application.Get()`
  is nil, and between New and `app.Run` nothing drains the main-queue
  dispatch a wails dialog needs. Linux keeps stderr (no shipped artifact).
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

- Workspace switcher: `/w/<profile>/` is served from `fallbackHandler` in
  `main.go`, and `/config.json` on that mount is stamped `desktop=true`.
  The sidebar lists profiles when more than one exists (see `docs/DESKTOP.md`).
- Onboarding for a machine with no credential is a separate track; until that
  lands, `gadak init` (or the bundled CLI) remains the setup path.
- Ship shape and deep-link delivery are the platform table above. The
  in-app Jira/Confluence pane (`embed_darwin.go`) is still macOS-only —
  other platforms get the stub in `embed_other.go`.
- macOS builds still need the Xcode command line tools. (The
  `UniformTypeIdentifiers` link flag `build-app.sh` used to pass by hand is
  gone — wails v3 declares that framework itself.)

[GDK-211]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-211
[GDK-293]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-293
