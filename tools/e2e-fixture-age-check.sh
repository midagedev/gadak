#!/usr/bin/env bash
# Census gate for e2e assertions that lean on the committed fixture's age
# (GDK-1670).
#
# The defect this closes is a class, not a line. `make demo-fixture` stamps
# examples/demo.db with the moment it ran, and several UI values are a
# function of the distance between that moment and now: the freshness chip's
# wording, a duration chip, a "last 90 days" window. A spec that asserts one
# of those without saying so is green on the machine that wrote it and red on
# the next schemaVNN commit, which regenerates the fixture as a gate — that is
# how e2e/docs-ux.spec.ts 'sync status at rest' failed on the schemaV46 round
# with "Synced 8m ago".
#
# The rule: every age-shaped expected value must DECLARE its premise. Put a
# `fixture-age:` comment within the 30 lines above the assertion saying which
# of the two it is —
#
#   fixture-age: stubbed — the response is patched, so this does not read the
#                fixture's clock at all.
#   fixture-age: constant — the value is fixed in the fixture's data (a span
#                between two stored timestamps), not a distance from now.
#
# A site with no declaration fails this check. The declaration is not a
# waiver: writing it is the moment the author has to decide which kind the
# assertion is, which is exactly the thinking that was missing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Age-shaped expected values: relative-time wording inside a string literal
# or a regex. Comment lines are prose about the premise, not assertions.
PATTERN='([0-9]+[mhdw] ago|just now|Waited [0-9]|Synced [0-9]|last [0-9]+ days|p85 [0-9]+d)'

undeclared=0
while IFS=: read -r file line _; do
  [ -n "${file:-}" ] || continue
  # The declaration window: the 30 lines above the assertion, plus its own.
  from=$((line - 30)); [ "$from" -lt 1 ] && from=1
  if sed -n "${from},${line}p" "$file" | grep -q 'fixture-age:'; then
    continue
  fi
  echo "e2e-fixture-age-check: $file:$line asserts an age-shaped value with no 'fixture-age:' declaration above it" >&2
  sed -n "${line}p" "$file" | sed 's/^/    /' >&2
  undeclared=$((undeclared + 1))
done < <(grep -nE "$PATTERN" e2e/*.spec.ts | grep -vE '^[^:]+:[0-9]+: *(//|\*|/\*)')

if [ "$undeclared" -gt 0 ]; then
  echo "e2e-fixture-age-check: $undeclared undeclared site(s). Declare the premise (stubbed / constant) or stub the response." >&2
  exit 1
fi
echo "e2e-fixture-age-check: OK (every age-shaped assertion declares its premise)"
