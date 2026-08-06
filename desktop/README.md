# scry desktop (prototype)

A native macOS window around the same embedded web UI — with **no TCP
listener at all**. The Wails asset server calls straight into the
`server.Handler` that `scry serve` mounts, so ports, addresses, and port
conflicts stop existing as UX. A second launch focuses the running window
(single-instance lock) instead of hunting for a free port.

Status: **prototype, macOS only, not part of the release pipeline yet.**
The CLI remains the primary distribution; this module is deliberately
separate from the root Go module so the wails dependency tree and its CGO
requirement never touch the CLI build or CI.

## Build

```bash
npm run build            # at the repo root — the app embeds dist/app
desktop/build-app.sh     # → desktop/build/Scry.app
desktop/build-app.sh --dmg  # → desktop/build/Scry-<version>.dmg as well
```

The bundle is unsigned; release signing/notarization would reuse the CLI's
Developer ID pipeline when this graduates.

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

## Known gaps (why this is not shipped)

- No workspace switcher (`/w/<profile>/` mounts) — single profile per window.
- No onboarding: a machine with no credential shows the empty UI; `scry init`
  is still the setup path.
- Unsigned, macOS only, and `wails doctor` deps are required to build.
