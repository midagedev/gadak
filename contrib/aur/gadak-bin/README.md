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

## Checking the package (any host with Docker)

```bash
./verify.sh
```

Builds `gadak-bin` in a throwaway Arch container against the real
published tarball and asserts: sha256 validation ran and passed, exactly
one package was produced, `namcap` reports no `E:` line, `gadak version`
prints `pkgver`, and the installed `/usr/bin/gadak` is byte-identical to
the tarball member. It regenerates `.SRCINFO` in place — commit that with
the `PKGBUILD` it came from. CI runs the same script whenever anything
under `contrib/aur/` changes (`.github/workflows/aur.yml`).

`makepkg` and `namcap` are Arch tools, but no Arch machine is needed to
run them. Exit 69 means Docker is missing or its daemon is down.

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

Run `./verify.sh` here first, then copy `PKGBUILD` and `.SRCINFO` from
this directory into the clone and add the `Maintainer:` line. The
maintainer name and address are **public and permanent** — they land in
the AUR git history and on the package page — so use a public-facing
identity, not a work address.

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
compute hashes itself. Then run `./verify.sh` — it regenerates
`.SRCINFO` for the new `pkgver` and proves the new tarball actually
installs, which is the half `update.sh` cannot check.

Copy the updated `PKGBUILD` and `.SRCINFO` into the AUR clone, commit,
and push.

A `pkgrel`-only rebuild (same upstream version, packaging fix) is a
hand edit of `pkgrel` — do not run `update.sh` for that, because it
resets `pkgrel` to 1.

Automation is allowed. The guidelines say it cannot replace reading the
upstream release (license, dependencies, and other notable changes still
need a person).

## Known namcap warnings

`verify.sh` fails on `E:` and prints `W:` without failing. Two warnings
are expected and are properties of the released binary, not of this
package:

```
ELF file ('usr/bin/gadak') lacks FULL RELRO, check LDFLAGS.
ELF file ('usr/bin/gadak') lacks PIE.
```

Go does not build position-independent executables by default. Changing
that is a GoReleaser decision about every platform's binary, not
something to paper over here — and the fix must not be to strip or
rebuild the binary in `package()`, because then what pacman installs is
no longer the artifact whose checksum people can verify.

`W: Missing Maintainer tag` is expected too: the tag is added on the AUR
clone, and this in-repo copy deliberately stores no address.
