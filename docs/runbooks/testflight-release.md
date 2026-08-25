# Runbook: ship gadak mobile to TestFlight

The phone app (`mobile/`, Tauri v2 + Svelte) goes to internal TestFlight with
one command. Internal testing needs no Beta App Review, so this path is open
while the app is still mid-refactor — the public store is a separate decision
(GDK-805), and it is gated on a problem this runbook does not solve: a
reviewer cannot reach a tailnet-only `gadak serve`, so there is no demo path
for review yet.

```bash
cd mobile && scripts/testflight-upload.sh --bump
```

That runs the fast gates, takes the next build number, builds a release
device binary with `tauri ios build --export-method app-store-connect`,
**verifies the exported `.ipa` against the submission contract**, validates
it, uploads it, and waits for Apple to finish processing. It finishes by
writing `artifacts/app-store/<date>-build<N>/upload.md`.

| Command | Use |
| --- | --- |
| `scripts/testflight-upload.sh` | ship the build number already in `tauri.conf.json` |
| `scripts/testflight-upload.sh --dry-run` | build + verify + validate, stop before upload |
| `scripts/testflight-upload.sh --status` | what does App Store Connect hold for this version? |
| `--allow-dirty` | ship an uncommitted tree (records "dirty tree" in the receipt) |
| `--no-gates` | skip svelte-check / vitest / ios-contract (only when you just ran them) |

The pipeline is a port of `~/repo/naru-remote/scripts/testflight-upload.sh` —
same Apple account, same API-key credentials, same receipt discipline. What
differs: `tauri ios build` replaces `xcodebuild archive`, so the contract is
checked against the exported `.ipa` rather than an `.xcarchive`.

## Where the identity lives

| Fact | Owner |
| --- | --- |
| bundle id `dev.gadak.mobile` | `mobile/src-tauri/tauri.conf.json` (`identifier`) |
| marketing version | same file (`version`) — drives `CFBundleShortVersionString` |
| build number | same file (`bundle.iOS.bundleVersion`) — drives `CFBundleVersion` |
| signing team | same file (`bundle.iOS.developmentTeam`) |
| export compliance, camera string | `mobile/src-tauri/Info.ios.plist` (merged into the generated plist, survives project regeneration) |

The build number stays in a tracked file on purpose: it is reviewable in git
rather than rewritten silently at export time. `--bump` edits it in place.

## One-time setup

### Credentials (already present on the lead machine)

Shared with naru-remote — one App Store Connect API key covers every app on
the account. Two files, both private, both outside any repo:

```
~/.appstoreconnect/credentials.env                     # 0600
~/.appstoreconnect/private_keys/AuthKey_<KEY_ID>.p8     # 0600
```

`credentials.env` holds `ASC_KEY_ID` and `ASC_ISSUER_ID`. Override the
locations with `ASC_CREDENTIALS_FILE` / `ASC_PRIVATE_KEY` on another machine.
The key needs the **App Manager** role to upload.

`altool` takes the key id and issuer as command-line arguments — Apple's
interface, not a choice — so they are visible in the process list while an
upload runs. Nothing in the repo, the logs or the receipt records them.

### The App Store Connect app record — a web step, and the owner's

`scripts/testflight-upload.sh --status` says
`no App Store Connect app record for dev.gadak.mobile yet` until this is done.
The App Store Connect API cannot create an app record; the web UI can.

1. appstoreconnect.apple.com → **Apps** → **+** → New App
2. Platform **iOS**, bundle ID `dev.gadak.mobile` (register the identifier
   first at developer.apple.com → Identifiers, or let the first signed build
   register it through automatic signing)
3. Name — must be unique across the whole store, so `gadak` may be taken;
   the TestFlight-facing name can be changed later
4. SKU — any private string, e.g. `gadak-mobile`
5. User access: Full Access

Then `--status` starts answering, and `--bump` can ship.

## The rust toolchain trap (worth knowing before you debug for an hour)

The Xcode run phase shells out to whatever `cargo` is first on `PATH`. On a
machine carrying **both** Homebrew's standalone `rust` formula and rustup,
`/opt/homebrew/bin/cargo` wins — and that toolchain ships only the host std.
The build then dies a few hundred lines deep in cargo with:

```
error[E0463]: can't find crate for `std`
  = note: the `aarch64-apple-ios` target may not be installed
```

…naming a target that `rustup target list --installed` swears is present. It
is — for the rustup toolchain the build never reached. Measured on 2026-08-26:
Homebrew rust 1.96.1's `rustlib/` held `aarch64-apple-darwin` and nothing else,
while `~/.rustup/toolchains/stable-*/lib/rustlib/` held all three iOS stds.

The script closes this rather than documenting around it: it resolves the
toolchain with `rustup which cargo` and puts that directory in front of `PATH`,
so the Xcode script inherits it. The gate that follows asks **the rustc that
will actually run** — `rustc --print target-libdir --target aarch64-apple-ios`,
then checks the directory exists, because that flag prints a path whether or
not anything is there. A gate phrased as `rustup target list --installed` is
the bug wearing a green light; do not reintroduce it.

If you would rather fix the machine than rely on the script, `brew uninstall
rust` leaves rustup as the only toolchain. Not required.

## What the script verifies before it uploads

Each check is here because Apple would only tell you after a full upload, or
would not tell you at all:

| Check | Why |
| --- | --- |
| build number not already upstream | App Store Connect rejects a duplicate *after* the build and upload; asking the API first costs a second |
| `CFBundleShortVersionString` / `CFBundleVersion` match `tauri.conf.json` | the `.ipa` is the thing shipped; the config is the thing reviewed |
| `CFBundleIdentifier` = `dev.gadak.mobile`, signing team matches | a wrong identity uploads to nothing |
| `ITSAppUsesNonExemptEncryption=false` | without it the build parks in "Missing Compliance" and never reaches a tester |
| `NSCameraUsageDescription` present | the QR pairing scanner is killed by iOS on first use without it |
| an app icon is compiled into the bundle | TestFlight rejects an icon-less build after the upload |
| no `demo-tour` string in the shipped bundle | `src/lib/demo-tour.ts` drives the store and the DOM with no user input. Vite is *expected* to drop it (the dynamic import sits behind `import.meta.env.DEV`); expected is not verified, so the gate greps the shipped binary for its arming token |

A failed check aborts before anything is uploaded.

## Account-owner steps the script cannot do

These are web actions, and they are the account owner's:

1. **Create the app record** (above), the first time only.
2. **Assign the build to the internal tester group** (TestFlight → Internal
   Testing). A processed build is invisible to testers until it is assigned.
3. **Export compliance**, if prompted the first time on a version — the plist
   key should pre-empt it.
4. **For a public release**: screenshots, EU DSA trader declaration, Korea
   declarations, the reviewer demo path, and submit for review. None of that
   is decided yet (GDK-805).

## When it fails

- **`no App Store Connect app record`** — the web step above has not been done.
- **`build N ... already exists upstream`** — re-run with `--bump`.
- **build failure** — the full log path is printed. Automatic signing needs
  the Apple ID in Xcode to still be authorised; `-allowProvisioningUpdates`
  cannot fix an expired session.
- **`processingState=FAILED|INVALID`** — Apple rejected it after upload; the
  reason is in App Store Connect (usually an asset or an entitlement).
- **still `PROCESSING` after 30 minutes** — normal for a first upload on a
  version; check later with `--status`.
