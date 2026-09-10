#!/usr/bin/env bash
# skill-overwrite-probe — does an unattended skill write spare a copy that is
# already installed? One command, with the real binary.
#
#   tools/skill-overwrite-probe.sh [options] [--] [gadak args…]
#
# Two rounds in a row (GDK-1531, GDK-1539) hand-built the same ~30-line
# harness in session scratch to answer this by measurement instead of code
# reading, and each threw it away. This is that harness, promoted (GDK-1546):
# it builds the binary, plants a stale-but-ours SKILL.md (with the receipt
# gadak leaves beside it) in a throwaway HOME, runs the given gadak args, and
# compares digests.
#
#   default            dev build, plain HOME   → PRESERVED is correct
#                     (the dev-build guard: a checkout build never replaces)
#   --gadak-home       GADAK_HOME set           → PRESERVED is correct
#                     (the isolation guard: a scratch workspace never touches
#                      the real machine's skill — GDK-1611)
#   --release          version stamped          → OVERWRITTEN is correct
#                     (the control: the guards are selective, not a blanket
#                      refusal — a release binary does refresh its own copy)
#
# The planted copy is deliberately *stale-ours*: byte-different from the
# embedded skill, with a matching receipt, so the classifier sees exactly the
# case an unattended write would take. The probe prints the binary's own
# `skill install --print` verdict on the plant (nothing is written by that)
# and the refusal line from the run, so a PRESERVED can be read as caused by
# the guard, not by an unread destination.
#
# Exit 0 = the run answered the question (either verdict); 1 = an --expect
# was given and the verdict disagrees; 2 = the harness itself failed.
set -euo pipefail

usage() { echo "usage: tools/skill-overwrite-probe.sh [--release] [--gadak-home] [--expect preserved|overwritten] [--] [gadak args…]" >&2; exit 2; }

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  sed -n '2,/^set -euo pipefail$/p' "$0" | sed '$d'
  exit 0
fi

BUILD=dev
SET_GADAK_HOME=0
EXPECT=""
VERB=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --release) BUILD=release; shift ;;
    --gadak-home) SET_GADAK_HOME=1; shift ;;
    --expect) [[ -n "${2:-}" ]] || usage; EXPECT="$2"; shift 2 ;;
    --) shift; while [[ $# -gt 0 ]]; do VERB+=("$1"); shift; done ;;
    -*) usage ;;
    *) VERB+=("$1"); shift ;;
  esac
done
[[ ${#VERB[@]} -gt 0 ]] || VERB=(init --local)

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
command -v go >/dev/null || { echo "skill-overwrite-probe: go not on PATH" >&2; exit 2; }

sha256() {
  if command -v shasum >/dev/null; then shasum -a 256 "$1" | awk '{print $1}'
  else sha256sum "$1" | awk '{print $1}'; fi
}

WORK="$(mktemp -d "${TMPDIR:-/tmp}/gadak-skill-probe.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# ── build ──────────────────────────────────────────────────────────────────
# The default build carries version "0.0.0-dev" — the exact provenance of both
# incidents. --release stamps a version the dev-build classifier reads as a
# cut release (same ldflag shape as .goreleaser.yaml).
LDFLAGS=()
[[ "$BUILD" == release ]] && LDFLAGS=(-ldflags "-X main.version=9.9.9-probe")
echo "== go build ./cmd/gadak ($BUILD)"
(cd "$ROOT" && go build -o "$WORK/gadak" "${LDFLAGS[@]:+${LDFLAGS[@]}}" ./cmd/gadak)

# ── sandbox + plant ────────────────────────────────────────────────────────
SB="$WORK/sandbox"
HOME_ISOL="$SB/home"
mkdir -p "$HOME_ISOL/.claude/skills/gadak" "$SB/tmp"
SKILL_DIR="$HOME_ISOL/.claude/skills/gadak"
SKILL="$SKILL_DIR/SKILL.md"

# stale-ours: differs from anything gadak embeds, with a receipt naming this
# exact digest, so DestStatus says "stale", not "identical" (vacuous) or
# "conflict" (a file the guards are not about).
printf '# gadak skill — probed sentinel copy\n\nThis byte sequence is the probe plant; the receipt beside it names it,\nso the classifier reads stale-ours.\n' > "$SKILL"
DIGEST="$(sha256 "$SKILL")"
printf '{\n  "sha256": "%s",\n  "gadak_version": "0.0.1",\n  "installed_at": "2026-01-01T00:00:00Z",\n  "source": "release"\n}\n' "$DIGEST" \
  > "$SKILL_DIR/.gadak-skill.json"

RUNENV=(env HOME="$HOME_ISOL" TMPDIR="$SB/tmp" PATH="$PATH")
if [[ "$SET_GADAK_HOME" == 1 ]]; then
  mkdir -p "$SB/gadak-home"
  RUNENV+=("GADAK_HOME=$SB/gadak-home")
fi

# The classifier's own view of the plant — --print writes nothing, and the
# skill verb is the one command the daily-sync hook skips.
CLASSIFY="$("${RUNENV[@]}" "$WORK/gadak" skill install --print 2>&1 | grep -E '^status:' || true)"
echo "== plant: $SKILL"
echo "== classifier says: ${CLASSIFY:-<no status line — plant is wrong>}"
case "$CLASSIFY" in
  *"stale"*) ;;
  *) echo "skill-overwrite-probe: the planted copy did not classify as stale — the verdict below would be vacuous" >&2; exit 2 ;;
esac

# ── run ────────────────────────────────────────────────────────────────────
echo "== run: gadak ${VERB[*]}"
set +e
"${RUNENV[@]}" "$WORK/gadak" "${VERB[@]}" > "$WORK/out" 2> "$WORK/err"
RC=$?
set -e
echo "== gadak exit: $RC (the verb's own exit is reported, not judged)"

# ── verdict ────────────────────────────────────────────────────────────────
AFTER="$(sha256 "$SKILL")"
echo "== refusal/skill lines from the run:"
grep -i 'skill' "$WORK/err" 2>/dev/null | sed 's/^/   [err] /' || true
grep -i 'skill' "$WORK/out" 2>/dev/null | sed 's/^/   [out] /' || true
echo "== before: $DIGEST"
echo "== after:  $AFTER"
if [[ "$AFTER" == "$DIGEST" ]]; then
  VERDICT=PRESERVED
else
  VERDICT=OVERWRITTEN
fi
AXES="build=$BUILD gadak-home=$([[ $SET_GADAK_HOME == 1 ]] && echo set || echo unset)"
echo "verdict: $VERDICT ($AXES, verb: gadak ${VERB[*]})"

if [[ -n "$EXPECT" ]]; then
  WANT="$(echo "$EXPECT" | tr '[:lower:]' '[:upper:]')"
  if [[ "$VERDICT" != "$WANT" ]]; then
    echo "skill-overwrite-probe: expected $WANT, got $VERDICT" >&2
    exit 1
  fi
fi
