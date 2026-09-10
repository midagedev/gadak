#!/usr/bin/env bash
# Cross-pins for grammars that live in two places (the mirror family the
# GDK-27 census filed as one class: a regex copied to a second owner ages
# separately, and nothing tells the next editor of either side).
#
#   bash tools/mirror-pins.sh
#
# Every pin extracts BOTH copies from the live source and compares them, so
# the check cannot be satisfied by editing a list here — a drift fails with
# both values and both file:line positions, and whoever edits one side is
# told about the other. Known, deliberate shape differences are normalized
# away and named below; everything else must be equal.
#
#   1. Atlassian host allowlist, tools/backlog-scrub-check.sh ↔
#      scripts/scan-internal.sh — compared as a set (GDK-1107). The two
#      gates must agree on what counts as a documentation placeholder, or
#      the same host is a leak in one file and fine in the other.
#   2. Home-directory pattern, same pair (GDK-1107). Declared delta,
#      normalized: the scrubber matches the path interior, the scanner
#      anchors both ends with `/`.
#   3. Profile-name grammar, internal/config/config.go (regexp + separate
#      length cap) ↔ internal/deeplink/deeplink.go (GDK-1105). Declared
#      delta, normalized: config is `*` plus a code-level cap, deeplink is
#      `{0,N-1}` with the cap inside — the effective maximum must match.
#      Char classes compare as sorted sets (`._-` vs `_.-` is order only).
#   4. UI-token grammar family, internal/config/{tokencheck/dimcheck,uitokens,
#      settings}.go ↔ web/src/lib/user-tokens.ts (GDK-1108): dim values,
#      font stack bodies plus the family-count/total-length caps, palette id
#      — bodies verbatim, anchors included. A widened Go gate silently drops
#      user styles after reload; a widened web side submits values the server
#      refuses.
#
# The host-extraction regexes differ by more than the allowlist (scan-internal
# forces an alphanumeric first character; the scrubber also accepts `.` and
# `_` there, which only over-captures into the same allowlist). That delta is
# deliberately not pinned: both directions are safe, and the allowlist — the
# surface that decides — is pinned exactly.
#
# Exit 0 = every pair agrees; 1 = drift (named on stdout). Run standalone or
# via tools/doc-checks.sh check 50.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

python3 - <<'PY'
import re
import sys
from pathlib import Path

findings = []


def where(text, m):
    return text.count("\n", 0, m.start()) + 1


# ── 1+2. backlog-scrub-check.sh ↔ scripts/scan-internal.sh (GDK-1107) ──────
scrub = Path("tools/backlog-scrub-check.sh").read_text()
scan = Path("scripts/scan-internal.sh").read_text()

# 1. The host allowlist, as a set of placeholder names.
scrub_hosts = None
m = re.search(r"grep -vxE '\(([^)]+)\)\\\.atlassian\\\.net'", scrub)
if m:
    scrub_hosts = m.group(1).split("|")
else:
    findings.append("tools/backlog-scrub-check.sh: no `grep -vxE '(a|b|c)\\.atlassian\\.net'` allowlist")

scan_hosts = None
case = re.search(
    r"\n[ \t]+([a-z][a-z0-9-]*\.atlassian\.net(?:\|[a-z][a-z0-9-]*\.atlassian\.net)+)\)", scan)
if case:
    scan_hosts = [h[: -len(".atlassian.net")] for h in case.group(1).split("|")]
else:
    findings.append("scripts/scan-internal.sh: no `your-site.atlassian.net|…)` case allowlist")

if scrub_hosts is not None and scan_hosts is not None:
    only_scrub = sorted(set(scrub_hosts) - set(scan_hosts))
    only_scan = sorted(set(scan_hosts) - set(scrub_hosts))
    if only_scrub or only_scan:
        findings.append(
            "host allowlist differs between tools/backlog-scrub-check.sh and "
            f"scripts/scan-internal.sh — only in backlog: {only_scrub}, only in scan: {only_scan}; "
            "the two gates must agree on what is a placeholder")

# 2. The home-directory pattern, trailing-slash delta normalized:
#    (/Users/|/home/)[…]+/  and  (/Users|/home)/[…]+  are the same pin.
scrub_home = re.search(r"grep -ohE '(\((?:/Users\|/home)\)/[^']*)'", scrub)
scan_home = re.search(r"PAT_HOMEPATH='([^']+)'", scan)
if not scrub_home:
    findings.append("tools/backlog-scrub-check.sh: no `grep -ohE '(/Users…'` home pattern")
if not scan_home:
    findings.append("scripts/scan-internal.sh: no PAT_HOMEPATH='…' home pattern")
if scrub_home and scan_home:
    def norm_home(p):
        # Declared delta: the scrubber spells the slash after the alternation
        # ((/Users|/home)/…), the scanner inside it ((/Users/|/home/)…) and
        # anchors the end with /. Stripping the separators leaves the parts
        # that matter — the alternation names and the character class.
        return p.replace("/", "")

    s, n = norm_home(scrub_home.group(1)), norm_home(scan_home.group(1))
    if s != n:
        findings.append(
            f"home pattern differs: backlog-scrub-check.sh:{where(scrub, scrub_home)} "
            f"{scrub_home.group(1)!r} vs scan-internal.sh:{where(scan, scan_home)} "
            f"{scan_home.group(1)!r} (only the slash placement is allowed to differ)")

# ── 3. profile grammar: config.go ↔ deeplink.go (GDK-1105) ────────────────
cfg = Path("internal/config/config.go").read_text()
dl = Path("internal/deeplink/deeplink.go").read_text()

cap = re.search(r"maxProfileNameLen = (\d+)", cfg)
cfg_re = re.search(r"profileNameRe = regexp\.MustCompile\(`([^`]+)`\)", cfg)
dl_re = re.search(r"profilePattern = regexp\.MustCompile\(`([^`]+)`\)", dl)
if not (cap and cfg_re and dl_re):
    findings.append("could not extract the profile grammar (maxProfileNameLen / "
                    "profileNameRe / profilePattern) — a rename made this pin blind")

SHAPE = re.compile(r"^\^\[([^\]]+)\]\[([^\]]+)\](?:\{0,(\d+)\}|\*)\$$")


def parse_shape(body, origin):
    m = SHAPE.match(body)
    if not m:
        findings.append(f"{origin}: profile grammar is not two classes plus a "
                        f"repetition: {body!r}")
    return m


def as_set(cls):
    """[A-Za-z0-9._-] → the set of characters the class covers, order-free."""
    out = set()
    i = 0
    while i < len(cls):
        if i + 2 < len(cls) and cls[i + 1] == "-":
            out.update(chr(c) for c in range(ord(cls[i]), ord(cls[i + 2]) + 1))
            i += 3
        else:
            out.add(cls[i])
            i += 1
    return out


if cap and cfg_re and dl_re:
    cs = parse_shape(cfg_re.group(1), "internal/config/config.go")
    ds = parse_shape(dl_re.group(1), "internal/deeplink/deeplink.go")
    if cs and ds:
        if as_set(cs.group(1)) != as_set(ds.group(1)):
            findings.append("profile first-char class differs: config.go "
                            f"{cs.group(1)!r} vs deeplink.go {ds.group(1)!r}")
        if as_set(cs.group(2)) != as_set(ds.group(2)):
            findings.append("profile rest class differs (as a char set, order-free): "
                            f"config.go {cs.group(2)!r} vs deeplink.go {ds.group(2)!r}")
        cfg_cap = int(cap.group(1))
        if ds.group(3) is None:
            findings.append("deeplink.go profilePattern lost its {0,N} bound — "
                            "an unbounded link grammar has no cap to compare")
        else:
            dl_cap = int(ds.group(3)) + 1
            if cfg_cap != dl_cap:
                findings.append(
                    f"profile effective max length differs: config.go {cfg_cap} vs "
                    f"deeplink.go {dl_cap} — a gadak:// link would accept a name the "
                    "workspace layer refuses, or vice versa")

# ── 4. ui-token grammar family (GDK-1108) ──────────────────────────────────
dim = Path("internal/config/tokencheck/dimcheck.go").read_text()
uit = Path("internal/config/uitokens.go").read_text()
stg = Path("internal/config/settings.go").read_text()
ts = Path("web/src/lib/user-tokens.ts").read_text()


def go_re(text, name, origin):
    m = re.search(name + r"\s*=\s*regexp\.MustCompile\(`([^`]+)`\)", text)
    if not m:
        findings.append(f"{origin}: cannot extract {name} — a rename made this pin blind")
    return m


def ts_re(text, name):
    # `/^body$/` — the anchors are part of the comparison, so they are put
    # back exactly once on the TypeScript side.
    m = re.search(r"const " + name + r" = /\^(.*)\$/[ \t]*$", text, re.M)
    if not m:
        findings.append(f"web/src/lib/user-tokens.ts: cannot extract {name} — "
                        "a rename made this pin blind")
    return m


for go_name, go_text, go_file, ts_name in [
    ("dimPxRe", dim, "internal/config/tokencheck/dimcheck.go", "DIM_PX_RE"),
    ("dimUnitlessRe", dim, "internal/config/tokencheck/dimcheck.go", "DIM_UNITLESS_RE"),
    ("fontIdentRe", uit, "internal/config/uitokens.go", "FONT_IDENT_RE"),
    ("fontQuotedInnerRe", uit, "internal/config/uitokens.go", "FONT_QUOTED_INNER_RE"),
    ("themeIdentRe", stg, "internal/config/settings.go", "PALETTE_RE"),
]:
    g, t = go_re(go_text, go_name, go_file), ts_re(ts, ts_name)
    if g and t:
        ts_body = "^" + t.group(1) + "$"
        if g.group(1) != ts_body:
            findings.append(
                f"{go_file}:{where(go_text, g)} {go_name} = {g.group(1)!r} vs "
                f"web/src/lib/user-tokens.ts:{where(ts, t)} {ts_name} = /{ts_body}/ "
                "— the server gate and the web mirror disagree")

# Font-stack caps: the counts are part of the mirrored grammar.
go_fam = re.search(r"maxFontFamilies = (\d+)", uit)
go_len = re.search(r"maxFontStackLen = (\d+)", uit)
ts_fam = re.search(r"families\.length > (\d+)", ts)
ts_len = re.search(r"v\.length > (\d+)", ts)
if not (go_fam and go_len and ts_fam and ts_len):
    findings.append("could not extract the font-stack caps (maxFontFamilies / "
                    "maxFontStackLen / the two `>` checks in isFontStack) — "
                    "a rename made this pin blind")
else:
    if go_fam.group(1) != ts_fam.group(1):
        findings.append(f"font family-count cap differs: uitokens.go "
                        f"{go_fam.group(1)} vs user-tokens.ts {ts_fam.group(1)}")
    if go_len.group(1) != ts_len.group(1):
        findings.append(f"font total-length cap differs: uitokens.go "
                        f"{go_len.group(1)} vs user-tokens.ts {ts_len.group(1)}")

if findings:
    print("mirror-pins: a mirrored pair drifted — both owners are named; reconcile them in one commit:")
    for f in findings:
        print("  - " + f)
    sys.exit(1)
print("mirror-pins: host allowlist, home path, profile grammar and the ui-token "
      "family (5 bodies + 2 caps) agree")
PY
