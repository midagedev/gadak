#!/usr/bin/env python3
"""Regenerate a CHANGELOG edition's reference-link tail from its own body.

The three editions (CHANGELOG.md / .ko.md / .ja.md) are one history: each
release section cites its keys as [GDK-nnn], and the tail at the bottom
defines every cited key exactly once as

    [GDK-nnn]: https://gadak.dev/backlog/#/?ks=GDK-nnn

The tail is derivation, not prose — given the body there is exactly one
correct form for it: the set of bracket-cited keys, in numeric order, each
with the public-backlog URL. It used to be maintained by hand, and a hand
maintained list drifts: on 2026-09-11 the three tails each held 659
definitions whose order was ascending for the first ~600 and then 54
append-time descents deep — each round's new keys landed at the bottom
instead of in place, three files diverging the same way, with no check
noticing because the set (not the order) is what the existing key check
compares.

Usage:

    python3 tools/changelog-tails.py --check            # all three editions
    python3 tools/changelog-tails.py --write CHANGELOG.md
    python3 tools/changelog-tails.py --check CHANGELOG.md CHANGELOG.ko.md

--check names the drift per file and exits 1; --write rewrites the tail in
place and leaves the body byte-identical (the body is joined back from the
exact lines it was split on). After a --write, --check is green and a
second --write changes nothing — that idempotence is the exit criterion
for handing the tail to this tool. Run tools/changelog-preserve.sh
against HEAD afterwards to confirm nothing but the tail moved.

The boundary between body and tail is the first line matching
^\\[(GDK-\\d+|#\\d+)\\]:. Everything from that line down is regenerated;
everything above it is preserved untouched. The generated set comes from
the bracket citations in the body — plain-text mentions of a key (no
brackets) do not count and generate nothing; the bracket form is the
citation contract.
"""

import argparse
import re
import sys
from pathlib import Path

FILES = ("CHANGELOG.md", "CHANGELOG.ko.md", "CHANGELOG.ja.md")
TAIL_START = re.compile(r"^\[(?:GDK-\d+|#\d+)\]:")
DEF = re.compile(r"^\[(GDK-\d+)\]:")
CITE = re.compile(r"\[(GDK-\d+)\]")
URL = "https://gadak.dev/backlog/#/?ks={}"


def parse(path: Path):
    """(body_lines, cited_keys, tail_lines), or None if the file has no tail."""
    lines = path.read_text(encoding="utf-8").splitlines()
    start = next((i for i, ln in enumerate(lines) if TAIL_START.match(ln)), None)
    if start is None:
        return None
    body, tail = lines[:start], lines[start:]
    cited = CITE.findall("\n".join(body))
    return body, cited, tail


def canonical(keys):
    """The one correct tail: every cited key once, numeric order."""
    return [
        f"[{k}]: {URL.format(k)}"
        for k in sorted(set(keys), key=lambda k: int(k.split("-")[1]))
    ]


def keynum(k):
    return int(k.split("-")[1])


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("files", nargs="*", help="edition(s); default: all three")
    mode = ap.add_mutually_exclusive_group()
    mode.add_argument("--write", action="store_true", help="rewrite the tail in place")
    mode.add_argument("--check", action="store_true", help="report drift, exit 1 on any (default)")
    args = ap.parse_args()
    if args.write and not args.files:
        ap.error("--write names the file(s) to rewrite; the default set is check-only on purpose")

    targets = [Path(f) for f in args.files] or [Path(f) for f in FILES]
    drift = False
    for path in targets:
        parsed = parse(path)
        if parsed is None:
            print(f"{path}: no reference-link tail (no [GDK-nnn]: line) — nothing to generate")
            drift = True
            continue
        body, cited, tail = parsed
        want = canonical(cited)
        have = [ln for ln in tail if ln.startswith("[GDK-")]
        extra = [ln for ln in tail if TAIL_START.match(ln) and not ln.startswith("[GDK-")]
        if extra:
            # A [#n]: GitHub reference or anything else living in the tail
            # would be silently deleted by --write; refuse instead.
            print(f"{path}: the tail holds non-GDK definitions this tool does not own:")
            for ln in extra[:3]:
                print(f"  {ln}")
            drift = True
            continue
        if have == want:
            print(f"{path}: tail is the generated form ({len(want)} keys, numeric order)")
            continue

        # Name the drift precisely: set first (the body and the tail
        # disagree about what exists), then order.
        hkeys = {m.group(1) for m in map(DEF.match, have) if m}
        wkeys = {m.group(1) for m in map(DEF.match, want) if m}
        gone = sorted(wkeys - hkeys, key=keynum)
        gained = sorted(hkeys - wkeys, key=keynum)
        if gone or gained:
            for k in gone[:5]:
                print(f"{path}: body cites {k} but the tail does not define it")
            for k in gained[:5]:
                print(f"{path}: tail defines {k} but the body does not cite it")
            if len(gone) > 5 or len(gained) > 5:
                print(f"{path}: … and {max(len(gone), len(gained)) - 5} more set differences")
        else:
            hnums = [int(m.group(1).split("-")[1]) for m in map(DEF.match, have) if m]
            descents = sum(1 for a, b in zip(hnums, hnums[1:]) if b < a)
            print(
                f"{path}: tail set is right but {descents} definition(s) sit after a "
                "higher-numbered key (append drift) — regenerate with --write"
            )
        drift = True

        if args.write:
            # join(body) is byte-identical to the original prefix: the split
            # was on newlines of an all-\n file, and nothing above the tail
            # is touched. The blank separator line before the tail, if the
            # file had one, is the last (empty) body line and survives.
            out = "\n".join(body) + "\n" + "\n".join(want) + "\n"
            path.write_text(out, encoding="utf-8")
            print(f"{path}: tail rewritten ({len(want)} keys, numeric order); body untouched")

    if drift and not args.write:
        sys.exit(1)


if __name__ == "__main__":
    main()
