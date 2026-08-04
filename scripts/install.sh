#!/bin/sh
# Install scry from the latest GitHub Release into ~/.local/bin (or $SCRY_INSTALL_DIR).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/midagedev/scry/main/scripts/install.sh | sh
#
# Env:
#   SCRY_INSTALL_DIR  install directory (default: $HOME/.local/bin)
#   SCRY_VERSION      release tag to install (default: latest), e.g. v0.1.0
#   SCRY_REPO         owner/name (default: midagedev/scry)
#
# Verifies the archive against checksums.txt (sha256) from the same release.
# POSIX sh; needs curl, tar, and either sha256sum or shasum.

set -eu

REPO="${SCRY_REPO:-midagedev/scry}"
INSTALL_DIR="${SCRY_INSTALL_DIR:-${HOME}/.local/bin}"
BIN_NAME="scry"
GITHUB_API="https://api.github.com"
GITHUB_DL="https://github.com"

err() {
  printf 'install.sh: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"
}

need curl
need tar
need mktemp
need uname

# --- platform ---------------------------------------------------------------

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  linux | darwin) ;;
  *) err "unsupported OS: $(uname -s) (supported: linux, darwin)" ;;
esac

case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) err "unsupported architecture: $(uname -m) (supported: amd64, arm64)" ;;
esac

# --- version / assets -------------------------------------------------------

if [ -n "${SCRY_VERSION:-}" ]; then
  tag="$SCRY_VERSION"
else
  # Prefer the Releases "latest" redirect; fall back to the API.
  latest_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "${GITHUB_DL}/${REPO}/releases/latest" 2>/dev/null || true)"
  tag="$(printf '%s' "$latest_url" | sed -n 's|.*/tag/\([^/]*\)$|\1|p')"
  if [ -z "$tag" ]; then
    need sed
    api_json="$(curl -fsSL "${GITHUB_API}/repos/${REPO}/releases/latest" 2>/dev/null)" \
      || err "could not resolve latest release for ${REPO} (no public release yet?)"
    tag="$(printf '%s' "$api_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  fi
  [ -n "$tag" ] || err "could not determine latest release tag for ${REPO}"
fi

# GoReleaser .Version strips a leading v from the tag.
version="$tag"
case "$version" in
  v*) version="${version#v}" ;;
esac

archive="${BIN_NAME}_${version}_${os}_${arch}.tar.gz"
base_url="${GITHUB_DL}/${REPO}/releases/download/${tag}"
archive_url="${base_url}/${archive}"
checksums_url="${base_url}/checksums.txt"

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/scry-install.XXXXXX")"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT INT HUP TERM

printf 'install.sh: fetching %s (%s/%s)\n' "$tag" "$os" "$arch" >&2

if ! curl -fsSL -o "${tmpdir}/${archive}" "$archive_url"; then
  err "download failed: ${archive_url}
  Is there a published release for ${tag}? Archives look like scry_<version>_<os>_<arch>.tar.gz"
fi

if ! curl -fsSL -o "${tmpdir}/checksums.txt" "$checksums_url"; then
  err "download failed: ${checksums_url}
  Releases must include checksums.txt (GoReleaser default)."
fi

# --- verify sha256 ----------------------------------------------------------

# Line format from GoReleaser: "<hash>  <filename>" (two spaces).
sum_line="$(grep -E "[ /]${archive}\$" "${tmpdir}/checksums.txt" || true)"
[ -n "$sum_line" ] || err "no checksum entry for ${archive} in checksums.txt"

expected="$(printf '%s' "$sum_line" | awk '{print $1}')"
[ -n "$expected" ] || err "could not parse checksum for ${archive}"

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${tmpdir}/${archive}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "${tmpdir}/${archive}" | awk '{print $1}')"
else
  err "need sha256sum or shasum to verify the download"
fi

if [ "$actual" != "$expected" ]; then
  err "checksum mismatch for ${archive}
  expected: ${expected}
  actual:   ${actual}"
fi
printf 'install.sh: checksum ok\n' >&2

# --- install ----------------------------------------------------------------

tar -xzf "${tmpdir}/${archive}" -C "$tmpdir"
[ -f "${tmpdir}/${BIN_NAME}" ] || err "archive did not contain ./${BIN_NAME}"

mkdir -p "$INSTALL_DIR"
# Atomic-ish replace: write beside the target, then mv.
dest="${INSTALL_DIR}/${BIN_NAME}"
tmp_dest="${dest}.new.$$"
cp "${tmpdir}/${BIN_NAME}" "$tmp_dest"
chmod 755 "$tmp_dest"
mv -f "$tmp_dest" "$dest"

if [ -x "$dest" ]; then
  ver_out="$("$dest" version 2>/dev/null || true)"
  printf 'install.sh: installed %s -> %s\n' "${ver_out:-$tag}" "$dest" >&2
else
  err "installed binary is not executable: $dest"
fi

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    printf 'install.sh: note: %s is not on PATH; add it, e.g.:\n' "$INSTALL_DIR" >&2
    # Intentional: print a copy-pasteable snippet with a literal $PATH.
    # shellcheck disable=SC2016
    printf '  export PATH="%s:$PATH"\n' "$INSTALL_DIR" >&2
    ;;
esac

printf 'install.sh: done. Try: scry version && scry demo\n' >&2
