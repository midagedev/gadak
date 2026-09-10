#!/usr/bin/env bash
# Tests for the tools/audit/*.sh census scripts (GDK-1707).
#
#   bash tools/audit-test.sh
#
# For every tools/audit/*.sh this asserts the census contract:
#   1. runs with no arguments and exits 0 (a census degrades, never fails)
#   2. the page starts with a markdown `#` header
#   3. the page carries a `## Sources` section with at least one command
#   4. no absolute path outside the repo leaks into the page
#      (/Users/, /tmp/, /private/ — the scan-internal rule)
#
# ci-ledger runs behind a fake unauthenticated gh for the whole test, so
# the suite is offline and deterministic AND covers the degradation
# contract: exit 0 with "Not measured" prose. fact-ledger is additionally
# run from a foreign cwd to prove each script re-roots itself via its own
# path.
#
# The bad fixtures at the end are the FAIL-first for this gate: they prove
# checks 2-4 can actually go red on a page that violates them.
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/audit-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# fake gh on PATH for the whole run: present but unauthenticated.
mkdir -p "$WORK/fakebin"
cat > "$WORK/fakebin/gh" <<'SHIM'
#!/usr/bin/env bash
# fake gh for tools/audit-test.sh: present but unauthenticated.
if [[ "${1:-}" == "auth" ]]; then
  echo "fake-gh: not logged in" >&2
  exit 1
fi
echo "fake-gh: unexpected invocation: $*" >&2
exit 1
SHIM
chmod +x "$WORK/fakebin/gh"

FAILURES=0
fail() { echo "  FAIL: $*"; FAILURES=$((FAILURES + 1)); }

# page_violations <file> — prints one message per violated check, returns
# non-zero when any tripped. This is the detector; check_page is the gate
# wrapper around it, and the FAIL-first fixtures call the detector
# directly so their rejections never count as real failures.
page_violations() {
  local file="$1" bad=0 first leaks
  first="$(head -1 "$file")"
  if [[ "$first" != \#* ]]; then
    echo "first line is not a markdown header — $first"
    bad=1
  fi
  if ! grep -q '^## Sources' "$file"; then
    echo "no '## Sources' section"
    bad=1
  elif ! awk '/^## Sources/{f=1;next} f && /^- `/{ok=1} END{exit !ok}' "$file"; then
    echo "'## Sources' has no command bullet"
    bad=1
  fi
  leaks="$(grep -nE '/Users/|/tmp/|/private/' "$file" || true)"
  if [[ -n "$leaks" ]]; then
    echo "absolute path leaked into the page:"
    printf '%s\n' "$leaks" | sed 's/^/    /'
    bad=1
  fi
  return "$bad"
}

# check_page <label> <file> — asserts 2/3/4 on a rendered page.
check_page() {
  local label="$1" v line
  if ! v="$(page_violations "$2")"; then
    while IFS= read -r line; do fail "$label: $line"; done <<<"$v"
    return 1
  fi
  return 0
}

echo "== census contract: every tools/audit/*.sh (gh faked unauthenticated)"
for script in tools/audit/*.sh; do
  name="$(basename "$script" .sh)"
  echo "- $name"
  rc=0
  PATH="$WORK/fakebin:$PATH" bash "$script" \
    > "$WORK/$name.md" 2> "$WORK/$name.err" || rc=$?
  if [[ $rc -ne 0 ]]; then
    fail "$name: exit $rc (a census degrades with exit 0, never fails)"
    if [[ -s "$WORK/$name.err" ]]; then sed 's/^/    /' "$WORK/$name.err"; fi
    continue
  fi
  if [[ -s "$WORK/$name.err" ]]; then
    fail "$name: wrote to stderr (census pages print to stdout only):"
    sed 's/^/    /' "$WORK/$name.err"
  fi
  check_page "$name" "$WORK/$name.md" || true
done

echo
echo "== ci-ledger degradation contract (fake gh, unauthenticated)"
if grep -q 'Not measured' "$WORK/ci-ledger.md"; then
  echo "- exit 0, says 'Not measured', header+sources intact"
else
  fail "ci-ledger(degraded): page does not say what was not measured"
fi

echo
echo "== cwd independence: fact-ledger from a foreign cwd"
rc=0
( cd "$WORK" && bash "$REPO/tools/audit/fact-ledger.sh" ) \
  > "$WORK/cwd.md" 2>"$WORK/cwd.err" || rc=$?
if [[ $rc -ne 0 ]]; then
  fail "fact-ledger(foreign cwd): exit $rc — scripts must re-root via \$0"
  if [[ -s "$WORK/cwd.err" ]]; then sed 's/^/    /' "$WORK/cwd.err"; fi
else
  check_page fact-ledger-foreign-cwd "$WORK/cwd.md" || true
  echo "- re-rooted and rendered"
fi

echo
echo "== FAIL-first: the checks go red on bad pages"
printf 'plain text first line\n\n## Sources\n\n- `cmd`\n' > "$WORK/bad-noh1.md"
printf '# Title\n\nbody without sources\n' > "$WORK/bad-nosrc.md"
printf '# Title\n\n## Sources\n\n(none)\n\nsee /Users/x/y\n' > "$WORK/bad-leak.md"
for bad in bad-noh1 bad-nosrc bad-leak; do
  if page_violations "$WORK/$bad.md" >/dev/null; then
    fail "$bad: accepted a bad page (gate cannot go red)"
  else
    echo "- $bad correctly rejected"
  fi
done

echo
if [[ "$FAILURES" -eq 0 ]]; then
  echo "audit-test: all green"
else
  echo "audit-test: $FAILURES failure(s)"
  exit 1
fi
