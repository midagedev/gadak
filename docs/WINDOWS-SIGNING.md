# Windows signing

**Status (checked 2026-08-24 against [v0.17.1](https://github.com/midagedev/gadak/releases/tag/v0.17.1), the current tag):**
Windows release binaries are **not Authenticode-signed** — that is the
settled decision below, not a pending task, and the v0.17.1 release notes
claim no signature. macOS is Developer ID-signed and notarized
([SECURITY.md](../SECURITY.md#release-artifacts)).

**The decision (2026-08-23, [GDK-211]): Windows stays unsigned, and this page
is the compensation.** Not a deferral — a choice, with a reason. A certificate
would not buy what it looks like it buys: SmartScreen reputation accrues to a
publisher over downloads, so a fresh certificate still shows the warning for a
while, and Smart App Control cannot be bypassed per app at all — so the one
dialog with no user-side override is the one signing would not immediately
silence either. Against that, the CLI zip already gives every blocked user a
working route, and it is the route the docs point at.

What would reopen this: a free OSS certificate we actually qualify for, or
enough download volume that publisher reputation would carry. Until then, the
honest thing is a page that says which file you have and what the warning is,
rather than a roadmap line promising a date.

**The first condition has since been met from a direction this page did not
list (2026-09-03, [GDK-1380]).** Microsoft Store developer registration is
free now, and the Store re-signs every MSIX it certifies with a Microsoft
certificate — no publisher certificate, and a Store-installed app is neither
a SmartScreen download nor an unknown signer to Smart App Control. So the
Windows app is being submitted there: `desktop/build-windows.ps1 --msix`
packs the same `gadak-desktop.exe` as the zip into an unsigned `.msix` for
Partner Center. Until a listing has passed certification, nothing below
changes — the GitHub Release zip stays unsigned and this page still describes
it. When it passes, this section becomes the install route for anyone Smart
App Control blocks, and the zip stays for everyone else.

This page is the place to check when Windows shows a warning. It is not a
claim that the unsigned zip is “safe”. It is how to see **which file you
have**, and what that warning actually is.

Install steps (unzip, WebView2, CLI fallback) stay in
[INSTALL.md](INSTALL.md#desktop-app-windows).

## Why Windows warns

An Authenticode signature is Windows’ publisher identity on a PE file
(`.exe`). Without one, two separate mechanisms may intervene. Neither is a
virus scan result:

- **Microsoft Defender SmartScreen** may show **Windows protected your PC**,
  then **More info** / **Run anyway**. That override is per file.
- **Smart App Control** (Windows 11) may show **Smart App Control blocked an
  app that may be unsafe.** Microsoft’s FAQ currently says there is **no way
  to bypass Smart App Control for one app**
  ([Smart App Control FAQ](https://support.microsoft.com/en-us/windows/smart-app-control-frequently-asked-questions-285ea03d-fa88-4d56-882e-6698afdb7003)).
  Do **not** turn Smart App Control off to run gadak.

### What the Smart App Control block actually looks like

Measured on Windows 11 Home with Smart App Control enforcing, against
v0.16.1's `gadak-desktop.exe` carrying its download mark (2026-08-23):

| How it was launched | What the user sees |
| --- | --- |
| Explorer double-click | Two windows: the **Smart App Control blocked an app that may be unsafe** notice, plus a message box titled with the exe's full path reading *An Application Control policy has blocked this file*. The only button is **OK** — no *Run anyway*. |
| `Start-Process` / `Invoke-Item` from PowerShell or `cmd` | **No dialog.** The caller receives `An Application Control policy has blocked this file` as an error and the process never starts. |

Both paths log Code Integrity 3033 and 3077 with *did not meet the Enterprise
signing level requirements*. The difference is only which process asked: a
dialog exists because Explorer draws it, so a scripted launch is silently
refused. **Measuring only the scripted path is how this page briefly came to
claim there was no dialog at all** — the FAQ's "no bypass" is what the dialog
confirms, not something it contradicts.

The wording of the **SmartScreen** dialog has still **not** been captured on a
Windows machine in this repository (`desktop/README.md`): a host with Smart
App Control enforcing never gets that far, because Smart App Control blocks
first. Treat it as the usual unsigned-download prompt, not as a measured
screenshot.

A signature would give every release one stable publisher identity.
Microsoft’s reputation for that identity still builds over subsequent
downloads; the first signed release is not an instant SmartScreen silence.

## What a GitHub Release actually attaches

Two Windows products, two files, two integrity stories. Do not mix them up.

| Asset | Built by | Signed? | In `checksums.txt`? |
| --- | --- | --- | --- |
| `gadak_<version>_windows_amd64.zip` / `windows_arm64.zip` — CLI `gadak.exe` | GoReleaser on `ubuntu-latest` ([`.github/workflows/release.yml`](../.github/workflows/release.yml), [`.goreleaser.yaml`](../.goreleaser.yaml)) | No | **Yes** (sha256). Measured on v0.17.1: the file lists the six CLI archives only (same as v0.16.1). |
| `Gadak-<version>-windows-x64.zip` / `windows-arm64.zip` — portable directory with `gadak-desktop.exe` and `gadak.exe` at the root | `desktop/build-windows.ps1 --archive` on `windows-latest` ([`.github/workflows/desktop-release.yml`](../.github/workflows/desktop-release.yml) job `desktop-windows`) | No. The job comment says it never looks for a certificate. | **No** (same as the macOS `.dmg`). [INSTALL.md](INSTALL.md#release-binary) already says this. |

There is no MSI or setup.exe. An unsigned installer is more Windows friction
than a zip (`desktop/build-windows.ps1`).

`checksums.txt` is produced by GoReleaser (`checksum.name_template`,
algorithm sha256 in `.goreleaser.yaml`). It is **not** a signature: it travels
on the same GitHub Release as the zip, so it catches a truncated or
substituted download, not a compromised GitHub account.

## Verify the file you downloaded

Download from
[the latest GitHub Release](https://github.com/midagedev/gadak/releases/latest)
only. Do not run a zip that arrived as an email attachment or a third-party
mirror.

### CLI zip (`gadak_<version>_windows_*.zip`)

This is the path with a published digest in `checksums.txt`. PowerShell
(built-in `Get-FileHash`). Replace `<version>` with the tag you fetched,
without the leading `v`, and `amd64` with the arch of the zip you actually
downloaded. The two lines assume the zip and `checksums.txt` from that
release are in the current folder:

```powershell
Get-FileHash -Algorithm SHA256 .\gadak_<version>_windows_amd64.zip
Select-String -Path .\checksums.txt -Pattern 'gadak_<version>_windows_amd64.zip$'
```

The 64-hex `Hash` from `Get-FileHash` must equal the first field on the
matching `checksums.txt` line (GoReleaser format: `<hash>  <filename>`, two
spaces). A mismatch means the bytes are not the release archive — delete the
zip.

`scripts/install.sh` does the same comparison on macOS/Linux before it
installs; it does not run on Windows (`scripts/install.sh` refuses a
non-linux/darwin `uname`).

### Desktop zip (`Gadak-<version>-windows-*.zip`)

**Gap:** this zip is not in `checksums.txt`. There is no project-published
digest to paste into a README that would stay true across tags.

GitHub’s Releases API does attach a `digest` field (`sha256:…`) to every
asset, including this zip. Measured on v0.17.1 (and previously on v0.16.1),
that field matched `checksums.txt` for every CLI archive that appears in
both. Use it as a cross-check against accidental corruption, not as a
second independent publisher. Replace `<version>` with the tag you fetched,
without the leading `v`, and `x64` with the arch of the zip you actually
downloaded:

```powershell
$rel = Invoke-RestMethod https://api.github.com/repos/midagedev/gadak/releases/latest
$rel.assets |
  Where-Object { $_.name -like 'Gadak-*-windows-*.zip' } |
  Select-Object name, digest, size
Get-FileHash -Algorithm SHA256 .\Gadak-<version>-windows-x64.zip
```

`digest` is `sha256:` plus the hex. `Get-FileHash` prints the hex only. They
must match. If you cannot reach the API, you do not have a digest to compare
— do not invent one from a blog or a chat log.

## If Windows blocks the desktop exe

Use the CLI zip from the same release (the row that *is* in `checksums.txt`),
put `gadak.exe` on `PATH`, then `gadak init && gadak sync && gadak serve`.
That is the documented CLI fallback on every release since 0.16
([INSTALL.md](INSTALL.md#desktop-app-windows)). Do not disable Smart App
Control.

## Uninstall

The desktop pack is a directory, not an installer. Delete the unzipped
folder. `gadak install-service` is refused on Windows
(`cmd/gadak/service.go`). Workspace files live under `%USERPROFILE%\.gadak`
(or `%GADAK_HOME%`) and are not removed by deleting the zip; see
[SECURITY.md](../SECURITY.md) for offboarding (`rm -rf ~/.gadak` on Unix;
the same directory on Windows).

## Code signing policy

This heading is the term SignPath Foundation requires on the project home
page and download pages
([signpath.org/terms.html](https://signpath.org/terms.html), “Conditions for
the website / repository”). **No Windows file is SignPath-signed today.**
Do not read the planned notice below as a current publisher identity.

**Authors / reviewers / approvers.** gadak is maintained by one person
([README.md](../README.md#who-makes-this)). GitHub user
[midagedev](https://github.com/midagedev) is the author (direct commits),
the reviewer of outside pull requests, and — after a SignPath grant — the
approver of each signing request. Accounts with repository or SignPath
access must use multi-factor authentication (SignPath condition; confirm
before applying).

**What would be signed, if the application is accepted.** Production
Authenticode signatures would be limited to PE files built from this
repository by the tag-triggered GitHub Actions workflows on GitHub-hosted
runners:

- `gadak.exe` inside `gadak_<version>_windows_amd64.zip` /
  `windows_arm64.zip` (GoReleaser, `ubuntu-latest`)
- `gadak-desktop.exe` and `gadak.exe` inside
  `Gadak-<version>-windows-x64.zip` / `windows-arm64.zip`
  (`windows-latest`)

The zip container itself is not an Authenticode target. There is no MSI.

**Private key.** SignPath stores the certificate’s private key on their HSM.
This repository and GitHub Actions would never hold it. The workflow would
upload an unsigned GitHub Actions artifact and get signed files back
([docs.signpath.io — GitHub trusted build system](https://docs.signpath.io/trusted-build-systems/github)).

**Every release signing request needs manual approval** (SignPath OSS
condition). The approver is the same maintainer.

**Planned attribution, only after a grant and a signed tag:**

> Free code signing provided by [SignPath.io](https://about.signpath.io),
> certificate by [SignPath Foundation](https://signpath.org).

Until that grant exists, the publisher Windows would show is unsigned /
unknown publisher.

### Privacy (runtime network)

gadak does not phone home on install. Outbound destinations are enumerated
in [SECURITY.md](../SECURITY.md) (five: your configured tracker, optional
anonymous GitHub Releases version check, Linear when configured, a paired
home serve when configured, and user-invoked `gh` from `gadak dev scan`).
There is no telemetry.

SignPath’s canned one-liner (“will not transfer any information to other
networked systems unless specifically requested…”) is **not** an exact
description of the default build: the version check is on unless
`updateCheck: false` (or a dev build). The destination list in SECURITY.md
is the accurate policy; [NETWORK.md](NETWORK.md) is the operating manual.

## Roadmap

1. **Now:** unsigned Windows zips; this page; CLI `checksums.txt`; desktop
   zip digest only via the GitHub Releases API. SmartScreen / Smart App
   Control behave as above.
2. **Apply** to [SignPath Foundation’s OSS program](https://signpath.org/apply.html)
   (draft answers and a pre-submit checklist:
   [runbooks/signpath-application.md](runbooks/signpath-application.md)).
   The application is not submitted from this document.
3. **If granted:** PE `VERSIONINFO` (product name / version) so SignPath can
   enforce metadata restrictions; GitHub App; `signpath/github-action-submit-signing-request`
   on the two release jobs; this page’s planned attribution becomes current;
   README / INSTALL stop saying “unsigned”.
4. **If refused:** keep this page; consider Azure Artifact Signing (formerly
   Trusted Signing) only if identity-validation geography can be met — see
   the runbook. A commercial CA certificate is a paid alternative, not the
   next step.

Publishing the desktop zip’s sha256 into `checksums.txt` (or a sibling file)
would close the desktop digest gap without a certificate. That is a CI
change and is not in this round.

[GDK-211]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-211
[GDK-1380]: https://gadak.dev/backlog/#/?ks=GDK-1380
