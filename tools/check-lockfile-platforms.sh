#!/usr/bin/env bash
# The lockfiles have to carry the platforms CI builds on, not only this laptop's.
#
# Class this closes (2026-08-26): adding one dependency on macOS re-resolved
# package-lock.json and npm 10 wrote back *only* darwin-arm64's optional
# binaries — @rollup/rollup-* went 75 entries to 26, @esbuild/* 78 to 27, and
# the same for the other native families. Every local gate stayed green,
# because the missing entries were the ones this machine does not need. CI is
# linux-x64, so `npm ci` there installed a rollup with no native binary and
# `vite build` died on "Cannot find module @rollup/rollup-linux-x64-gnu" —
# in two jobs, on the release commit.
#
# The check is deliberately a *presence* test on named platform packages
# rather than a count: a count drifts every time a dependency adds or drops
# a target, and a gate that has to be re-baselined is a gate people re-baseline
# past the real regression. These four are what the CI matrix actually runs
# (ubuntu-latest x64 for the frontend and hosted-demo jobs, windows-latest and
# macos for the desktop packs), so each one is here because something builds
# on it.
#
# The way out, if this fires: do not delete the lockfile and reinstall — that
# is what produced the pruned file. Restore the last lockfile that had the
# platforms (`git show <ref>:package-lock.json > package-lock.json`) and then
# `npm install --package-lock-only`, which updates the changed dependency and
# leaves every other resolution alone.
set -uo pipefail

cd "$(dirname "$0")/.."

fails=0
fail() {
  printf 'FAIL: %s\n' "$1" >&2
  fails=$((fails + 1))
}

# One representative package per native family, per platform CI builds on.
# rollup and esbuild are the two that actually stopped a build; the others
# ship the same optional-binary shape and would fail the same way.
want=(
  "@rollup/rollup-linux-x64-gnu"
  "@rollup/rollup-darwin-arm64"
  "@rollup/rollup-win32-x64-msvc"
  "@esbuild/linux-x64"
  "@esbuild/darwin-arm64"
  "@esbuild/win32-x64"
)

for lock in package-lock.json site/package-lock.json; do
  [[ -f "$lock" ]] || continue
  for pkg in "${want[@]}"; do
    # site/ does not depend on every family; only require a platform when the
    # lockfile knows that family at all, so this cannot become a demand that
    # a project install something it does not use.
    family="${pkg%%/*}"
    grep -q "\"node_modules/${family}/" "$lock" || continue
    if ! grep -q "\"node_modules/${pkg}\"" "$lock"; then
      fail "$lock has no $pkg — the lockfile was resolved on one platform and pruned the others (see the header of $0)"
    fi
  done
done

if (( fails > 0 )); then
  exit 1
fi
echo "ok: lockfiles carry the platforms CI builds on"
