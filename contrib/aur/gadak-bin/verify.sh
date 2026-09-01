#!/usr/bin/env bash
# Build and check the AUR package in a throwaway Arch container.
#
# Usage: contrib/aur/gadak-bin/verify.sh
#
# `makepkg` and `namcap` are Arch tools, so before this script the only way
# to find a packaging defect was to have an Arch machine — and the defect
# this exists to catch (an empty gadak-bin-debug whose .build-id symlink
# dangles) was found only when one was finally used. Docker makes the check
# runnable from any host, which is the point: an unrun check is not a check.
#
# What it asserts, all against the real published release:
#   1. PKGBUILD parses and makepkg builds it
#   2. the downloaded tarball matches sha256sums_x86_64
#   3. exactly one package is produced (no stray -debug split)
#   4. namcap reports no E: lines on PKGBUILD or the package
#   5. the installed /usr/bin/gadak is byte-identical to the tarball member
#      — a person can verify the release checksum and have it mean something
#   6. `gadak version` prints pkgver
#
# It also regenerates .SRCINFO in place. That file must be in the AUR
# commit, and makepkg is the only thing that can produce it — so it is kept
# here, next to the PKGBUILD it is derived from, where a stale one shows up
# in a diff instead of in a rejected push. Commit both together.
#
# Only the x86_64 source is exercised: makepkg builds for the container's
# architecture, and one arch proves the package() body. The aarch64 sha is
# checked against checksums.txt by update.sh.
#
# Exit 64 = usage / bad arguments
#      69 = a required tool is missing (docker, or its daemon is down)
#       1 = a verification above failed
#       0 = all of them passed
set -euo pipefail

usage() {
  echo "usage: contrib/aur/gadak-bin/verify.sh" >&2
  exit 64
}

if [[ $# -ne 0 ]]; then
  usage
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "verify.sh: missing docker" >&2
  exit 69
fi
if ! docker info >/dev/null 2>&1; then
  echo "verify.sh: docker is installed but its daemon is not reachable" >&2
  exit 69
fi

self="${BASH_SOURCE[0]}"
if [[ "$self" != /* ]]; then
  self="$(pwd)/$self"
fi
here="${self%/*}"
if [[ ! -f "${here}/PKGBUILD" ]]; then
  echo "verify.sh: PKGBUILD not found at ${here}/PKGBUILD" >&2
  exit 1
fi

# The scratch dir must live next to the PKGBUILD, not under ${TMPDIR}: on
# macOS mktemp answers /var/folders/…, which Docker Desktop does not share
# with the VM — the container then writes /out somewhere VM-local, every
# check passes, and the host is left with no .SRCINFO to commit (GDK-1256,
# measured: a probe file written through that mount never reaches the host).
out="$(mktemp -d "${here}/.verify-out.XXXXXX")"
cleanup() { rm -rf "$out"; }
trap cleanup EXIT INT HUP TERM

# pacman's alpm sandbox uses seccomp/landlock, which fails under x86_64
# emulation on a non-x86_64 host ("error restricting syscalls via seccomp").
# Relax it only there — a native runner keeps the default confinement.
sec=()
if [[ "$(uname -m)" != "x86_64" ]]; then
  sec=(--security-opt seccomp=unconfined)
fi

# Reads /pkg/PKGBUILD, writes /out/.SRCINFO. Runs as root to install build
# deps, then drops to `build` because makepkg refuses to run as root.
inner='
set -euo pipefail
# -Syu, never -Sy: Arch does not support partial upgrades. The image ships a
# package database snapshot, and installing against a synced db without
# upgrading resolves versions the mirrors have already rotated out —
# "failed retrieving file elfutils-0.195-8-x86_64.pkg.tar.zst : 404", which
# is what this script did on its first CI run while passing locally forty
# minutes earlier. The window between the two is the whole bug.
pacman -Syu --noconfirm --needed --disable-sandbox base-devel namcap sudo >/dev/null
useradd -m build
echo "build ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/build
install -d -o build /work
cp /pkg/PKGBUILD /work/
chown build /work/PKGBUILD
cd /work

bash -n PKGBUILD
sudo -u build makepkg --printsrcinfo > /out/.SRCINFO

ver="$(sudo -u build bash -c ". ./PKGBUILD >/dev/null 2>&1; echo \$pkgver")"

sudo -u build makepkg --noconfirm > /out/makepkg.log 2>&1 || {
  echo "FAIL: makepkg" >&2; tail -40 /out/makepkg.log >&2; exit 1
}
grep -q "Passed" /out/makepkg.log || { echo "FAIL: sha256 validation did not run" >&2; exit 1; }

# A prebuilt binary must produce exactly one package. A second one is the
# debug split reappearing, which means options=(!strip !debug) was lost.
mapfile -t pkgs < <(ls -1 ./*.pkg.tar.zst)
if [[ ${#pkgs[@]} -ne 1 ]]; then
  echo "FAIL: want exactly 1 package, got ${#pkgs[@]}: ${pkgs[*]}" >&2
  exit 1
fi

namcap PKGBUILD > /out/namcap.txt 2>&1 || true
namcap "${pkgs[0]}" >> /out/namcap.txt 2>&1 || true
cat /out/namcap.txt
if grep -q " E: " /out/namcap.txt; then
  echo "FAIL: namcap reported an error" >&2
  exit 1
fi

pacman -U --noconfirm --disable-sandbox "${pkgs[0]}" > /dev/null

got="$(gadak version)"
if [[ "$got" != "$ver" ]]; then
  echo "FAIL: gadak version printed ${got}, PKGBUILD pkgver is ${ver}" >&2
  exit 1
fi

# Stripping would rewrite the binary we published, so the checksum a person
# can verify against the release would stop matching what pacman installed.
want_sum="$(tar -xzOf "src/gadak_${ver}_linux_amd64.tar.gz" gadak | sha256sum | cut -d" " -f1)"
got_sum="$(sha256sum /usr/bin/gadak | cut -d" " -f1)"
if [[ "$want_sum" != "$got_sum" ]]; then
  echo "FAIL: installed binary ${got_sum} != release tarball member ${want_sum}" >&2
  exit 1
fi

echo "ok: ${pkgs[0]}"
echo "ok: gadak version ${got}"
echo "ok: installed binary is byte-identical to the release tarball (${got_sum})"
pacman -Ql gadak-bin
'

docker run --rm --platform linux/amd64 "${sec[@]}" \
  -v "${here}:/pkg:ro" -v "${out}:/out" \
  archlinux:latest bash -c "$inner"

# Belt for any other mount arrangement that swallows /out: a green
# container run without the artifact is a failure, not a pass.
if [[ ! -f "${out}/.SRCINFO" ]]; then
  echo "verify.sh: container passed but ${out}/.SRCINFO never reached the host — the /out mount is not shared with the docker daemon" >&2
  exit 1
fi
cp "${out}/.SRCINFO" "${here}/.SRCINFO"
echo "verify.sh: wrote ${here}/.SRCINFO"
