# scry desktop

A native macOS window around the same embedded web UI — with **no TCP
listener at all**. The Wails asset server calls straight into the
`server.Handler` that `scry serve` mounts, so ports, addresses, and port
conflicts stop existing as UX. A second launch focuses the running window
(single-instance lock) instead of hunting for a free port.

Status: **macOS app bundle + signed/notarized dmg on tag releases** (arm64).
The module stays a nested Go module so the wails dependency tree and its CGO
requirement never touch the CLI build or the non-macOS CI matrix.

## Build

```bash
npm run build                 # at the repo root — the app embeds dist/app
desktop/build-app.sh          # → desktop/build/Scry.app
desktop/build-app.sh --dmg    # → desktop/build/Scry-<version>-arm64.dmg as well
```

The bundle includes:

- `Contents/MacOS/scry-desktop` — Wails host (App + Edit menus so ⌘C/V/X/A work)
- `Contents/Resources/bin/scry` — same CLI binary as the brew formula, for agent
  wiring without a separate install

Put the CLI on your PATH:

```bash
sudo ln -sf "/Applications/Scry.app/Contents/Resources/bin/scry" /usr/local/bin/scry
```

Then `scry mcp install claude` (and friends) work from a desktop-only install.

### Signing and notarization

Unsigned by default (local development). Set env vars to opt in:

| Variable | Meaning |
| --- | --- |
| `SCRY_SIGN_IDENTITY` | codesign identity (`Developer ID Application: …`) |
| `SCRY_NOTARY_KEY` | path to App Store Connect API key `.p8` |
| `SCRY_NOTARY_KEY_ID` | key id |
| `SCRY_NOTARY_ISSUER_ID` | issuer id |

With `SCRY_SIGN_IDENTITY`, `build-app.sh` signs both nested binaries under a
hardened runtime, then the `.app`, and the `.dmg` when `--dmg` is passed.
Notarization runs only when `--dmg` and all three `SCRY_NOTARY_*` vars are set
(`notarytool submit --wait`, then `stapler staple` on the dmg and app).

CI: `.github/workflows/desktop-release.yml` on `v*` tags (and
`workflow_dispatch` for dry runs). It reuses the same secrets as the
goreleaser job (`MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD`, `MACOS_NOTARY_KEY`,
`MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_ISSUER_ID`). Missing secrets skip
signing; the job still produces an unsigned dmg artifact. Tag pushes upload
the dmg onto the GitHub Release.

Current ship shape is **arm64 only** (`Scry-<ver>-arm64.dmg`). A universal
(arm64+amd64 lipo) build is a later stretch.

## How it hangs together

- `main.go` opens the profile's mirror (`SCRY_PROFILE` respected), builds the
  API handler, starts the sync loop and update check — the same wiring as
  `cmd/scry serve`, minus the listener and workspace mounts.
- The webview's requests carry `wails://` Origins the browser guard would
  reject; the fallback handler strips them and presents as loopback. That is
  not a guard bypass in the threat-model sense: these requests never crossed
  a network boundary, and the webview is the only client that can reach the
  handler.
- `handler_test.go` pins the three seams (config.json, guard passage, SPA
  fallback). Run with `SCRY_PROFILE=demo go test -tags desktop,production ./...`
  — it refuses to open the default profile's mirror.

## Known limits

- No workspace switcher (`/w/<profile>/` mounts) — single profile per window.
- Onboarding for a machine with no credential is a separate track; until that
  lands, `scry init` (or the bundled CLI) remains the setup path.
- macOS only; building still needs Xcode CLT and the UniformTypeIdentifiers
  link flag (handled by `build-app.sh`).
