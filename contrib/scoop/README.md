# Scoop `gadak`

In-repo source of the Scoop manifest that installs the Windows CLI zip from
GitHub Releases. This is not the macOS desktop app (Homebrew cask `gadak`),
not the Homebrew CLI formula (`gadak-cli`), and not the unsigned Windows
desktop zip (`Gadak-<version>-windows-x64.zip`). The Scoop app name is
`gadak` because Scoop is Windows-only and that is the command the shim
puts on `PATH`.

**The bucket is not published.** `scoop install` has not been run on a
Windows machine. What this directory checks is offline: JSON against
Scoop's schema, `version` against the latest git tag, hashes against that
tag's `checksums.txt`, and Scoop's `checkver` regex against GitHub
`/releases/latest`. Do not tell anyone to `scoop install gadak` until the
bucket exists and a Windows host has actually installed from it.

Do **not** add this gadak repository as a Scoop bucket.
`scoop bucket add <name> <git-url>` clones the whole repo, then
`Find-BucketDirectory` uses a top-level `bucket/` subdirectory if it
exists, otherwise the repo root
([`lib/buckets.ps1`](https://github.com/ScoopInstaller/Scoop/blob/master/lib/buckets.ps1)).
This repo has no top-level `bucket/`, and `apps_in_bucket` does
`Get-ChildItem *.json -Recurse`, so the root would treat `package.json`
and every other JSON file as a manifest. A `bucket/` directory here
would work mechanically and would still clone the whole tree (demo
snapshot, web, e2e) for one file.

## Checking the manifest (any host with curl and python3)

```bash
./verify.sh
```

Asserts the list in the script header. It downloads Scoop's
`schema.json` and the release `checksums.txt`; it does not download the
zips (those were listed once with `unzip -l` when the manifest was
written: `LICENSE`, `NOTICE`, `README.md`, `gadak.exe` at the archive
root). Exit 69 means curl, python3, or git is missing.

Registered in CI ([`.github/workflows/scoop.yml`](../../.github/workflows/scoop.yml)) the same way
[`.github/workflows/aur.yml`](../../.github/workflows/aur.yml) registers
`contrib/aur/gadak-bin/verify.sh`: path-scoped to `contrib/scoop/**`.
Do not fold it into `tools/doc-checks.sh`.

## First publish (lead)

1. Create `https://github.com/midagedev/scoop-gadak` (this directory
   cannot). Same org as `midagedev/homebrew-tap`. Suggested default
   branch: `master` (Scoop extras uses `master`).
2. Layout of that repo only:

   ```
   bucket/gadak.json    # copy of this directory's gadak.json
   README.md            # the user commands below
   ```

3. User commands, once the empty repo has that commit:

   ```powershell
   scoop bucket add gadak https://github.com/midagedev/scoop-gadak
   scoop install gadak
   gadak version
   ```

4. On a Windows host that already has Scoop: run those three, then
   `Get-Command gadak` and `gadak version` against the tag in
   `gadak.json`. That is the live check this directory cannot do.
5. After that live check passes, one line in
   [`docs/INSTALL.md`](../../docs/INSTALL.md) (the `Status:` line under
   Scoop) becomes the two `scoop` commands. Then add the windows row in
   `web/src/lib/upgrade-cta.ts` — not before; a command the bucket
   cannot serve is a lie (that file's own comment).

## Each new release

From this directory, on any machine with `curl`:

```bash
./update.sh v0.15.2
./verify.sh
```

`update.sh` rewrites `version` and the two windows sha256 values from
that tag's `checksums.txt`. It does not compute hashes itself. Copy the
updated `gadak.json` into `scoop-gadak/bucket/gadak.json`, commit, and
push. Do not automate that push from the gadak release workflow (deploy
key / PAT is a lead secret, same as the AUR clone).

`checkver` + `autoupdate` in the manifest let a Scoop maintainer run
`bin/checkver.ps1 gadak -Update` against the published bucket. That is
Scoop-side automation; it does not update this file and it does not
push `scoop-gadak`.

## Why not extras / why not a release-workflow rewrite

A PR to `ScoopInstaller/Extras` is a later option — `gadak.json` is
404 in extras, main, and versions as of 2026-08-18. extras still needs
the same live Windows install this directory has not done.

`.github/workflows/release.yml` is left alone. Updating this JSON on
tag is `update.sh`; publishing it is a push to `scoop-gadak`. Neither
belongs inside GoReleaser, and changing release asset names would break
the v0.14+ macOS updater namespace.
