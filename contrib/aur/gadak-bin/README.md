# AUR `gadak-bin`

In-repo source of the AUR package that installs the Linux CLI tarball from
GitHub Releases. This is not the macOS desktop app (Homebrew cask `gadak`)
and not a from-source AUR package. Publishing to the AUR is a separate step
(account and SSH key); this directory is what you copy into that git
repository.

The package name is `gadak-bin` because the [AUR submission
guidelines](https://wiki.archlinux.org/title/AUR_submission_guidelines)
require the `-bin` suffix for prebuilt binaries when the sources are
available. The PKGBUILD `provides` and `conflicts` `gadak`. Homebrew's
names are different: `gadak` is the macOS app cask, `gadak-cli` is the
CLI formula. A desktop-app AUR package is out of scope here.

What `package()` installs (from the v0.15.2 linux tarballs, `tar -tzf`):

| Archive member | Installed path |
| --- | --- |
| `gadak` | `/usr/bin/gadak` |
| `LICENSE` | `/usr/share/licenses/gadak-bin/LICENSE` |
| `NOTICE` | `/usr/share/licenses/gadak-bin/NOTICE` |

`NOTICE` is installed because the archive ships it and Apache-2.0 §4(d)
requires redistributing it. The archive also contains `README.md`. It
does not contain shell completions or a man page, so the PKGBUILD does
not install those.

The web UI is compiled into the binary (`go:embed`); the tarball is the
whole install.

## Prerequisites (first publish)

1. An [AUR](https://aur.archlinux.org/) account.
2. An SSH key whose public half is pasted into the AUR profile (*My
   Account*). The guidelines recommend a key used only for the AUR.
3. SSH config for `aur.archlinux.org` as user `aur`, pointing at that
   key. See the [Authentication](https://wiki.archlinux.org/title/AUR_submission_guidelines#Authentication)
   section of the guidelines.

Do not commit a private key, token, or email address into the gadak
repository. The `Maintainer:` comment at the top of `PKGBUILD` is filled
in on the AUR clone at publish time, not here.

## First publish

On a machine that can reach `aur.archlinux.org` over SSH:

```bash
git -c init.defaultBranch=master clone ssh://aur@aur.archlinux.org/gadak-bin.git
# empty-repo warning is expected when the package does not exist yet
```

Copy `PKGBUILD` from this directory into the clone. Add the
`Maintainer:` line. Generate `.SRCINFO` **on Arch** (`makepkg` is not
on macOS):

```bash
makepkg --printsrcinfo > .SRCINFO
```

Verify the package builds and the binary runs before you push:

```bash
makepkg -si
gadak version
```

Then, in the AUR clone only:

```bash
git add PKGBUILD .SRCINFO
git commit -m "gadak-bin 0.15.2"
git push
```

The AUR only accepts pushes to `master`. `.SRCINFO` must be in the
commit or the AUR will reject the push.

The AUR git repository (not this directory) is encouraged to carry a
license for the packaging files themselves; the guidelines recommend
[0BSD](https://spdx.org/licenses/0BSD.html). Add that file at publish
time.

## Each new release

From this directory, on any machine with `curl`:

```bash
./update.sh v0.15.2
```

`update.sh` rewrites `pkgver`, resets `pkgrel` to 1, and replaces the
two linux sha256 values from that tag's `checksums.txt`. It does not
compute hashes itself. If `makepkg` is on PATH it regenerates
`.SRCINFO`; otherwise it prints:

```bash
makepkg --printsrcinfo > .SRCINFO
```

Copy the updated `PKGBUILD` (and `.SRCINFO` if regenerated) into the
AUR clone, commit, and push.

A `pkgrel`-only rebuild (same upstream version, packaging fix) is a
hand edit of `pkgrel` — do not run `update.sh` for that, because it
resets `pkgrel` to 1.

Automation is allowed. The guidelines say it cannot replace reading the
upstream release (license, dependencies, and other notable changes still
need a person).

## Verification on Arch

On an Arch machine or container:

```bash
makepkg -si
gadak version
```

Optional, from the [PKGBUILD](https://wiki.archlinux.org/title/PKGBUILD)
page: `namcap PKGBUILD` for common packaging mistakes.

`makepkg` and `namcap` are Arch tools. The checks that run from this
repository are `bash -n PKGBUILD`, `bash -n update.sh`, and comparing
the `sha256sums_*` values to the release `checksums.txt`.
