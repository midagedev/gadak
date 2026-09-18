#!/usr/bin/env python3
"""Japanese copy carries no ASCII space beside a Japanese character.

    tools/ja-spacing.py --check     # exit 1 and name every offending line
    tools/ja-spacing.py --write     # remove them

Japanese is set solid. A reader told us the site's Japanese pages read as
though Korean or English spacing had been applied to them (2026-09-13), and
the count was the answer: 154 ASCII spaces sat next to a Japanese character
on `/ja/` alone. Two separate habits put them there.

The first is 和欧間 spacing — a space wherever Latin meets Japanese, as in
"Jira の課題を SQL で集計する". Some Japanese technical writing does this;
mainstream Japanese sites do not, and the effect at this density is a page
that looks mis-set. The second is not a convention at all but an error:
"534 件", "3.7 分", "1 つ" put a space between a numeral and its Japanese
counter, which no style permits.

So the rule here is one rule, not two: a space goes only between two Latin
runs. "4,761 ms" keeps its space (both sides Latin), "Server / Data Center"
keeps its spaces, "534 件" loses it, "Jira と Confluence" loses both.

What it never touches: the inside of a code span (a shell command's spaces
are the command), the indentation at the head of a source line, a run that
crosses a line break, and anything outside a Japanese region — the `ja:`
block of a copy catalogue, or a file under a `ja/` path.

A line break is the same defect wearing source clothes — markdown and HTML
both collapse it into a rendered space — but it is not this tool's to fix:
in a source file the break is also indentation and structure. Prose wrapped
between two Japanese characters is rejoined where it is rendered instead
(site/src/lib/changelog.ts for the markdown pages) or in the file by hand.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

# CJK ideographs, kana, and the full-width punctuation that sets with them.
JA = (
    "　-〿"  # 、。「」〈〉 and the ideographic space
    "぀-ゟ"  # hiragana
    "゠-ヿ"  # katakana
    "㐀-䶿一-鿿豈-﫿"  # han
    "！-｠"  # full-width forms
)
IS_JA = re.compile(f"[{JA}]")

# Each entry is a file and, when the file is not Japanese end to end, the
# region inside it that is. The region is given as the line that opens it
# and the line that closes it, because both catalogues are hand-indented
# and a brace matcher would be a bigger thing than this needs.
TARGETS: list[tuple[str, tuple[str, str] | None]] = [
    ("site/src/i18n.ts", ("  ja: {", "  },")),
    ("site/src/tagline.js", ("  ja: {", "  },")),
    ("site/src/pages/ja/index.astro", None),
    ("site/src/pages/ja/install.astro", None),
    ("site/src/pages/ja/changelog.astro", None),
    # Rendered at /ja/changelog/ by site/src/lib/changelog.ts, so it is a
    # page of the Japanese site and reads under the same rule. Its 80-column
    # wrapping is a second, separate source of rendered spaces; that half is
    # joined at render time, not here.
    ("CHANGELOG.ja.md", None),
]

# The app's own message catalogues. These are the Japanese of every desk
# screen and — through mobile/src/lib/i18n.ts, which re-exports this catalogue
# — every phone screen, which makes them by far the largest Japanese surface
# in the tree. They were absent until GDK-2012, and the reason is visible in
# their shape: a catalogue is one key per {en, ko, ja}, so its Japanese is the
# value on every `ja:` line and nothing else. That is not a contiguous region,
# so the (open line, close line) model above cannot name it. They are masked
# from the other side instead — see catalogue_mask.
CATALOGUES: list[str] = [
    "web/src/lib/i18n/messages/common.ts",
    "web/src/lib/i18n/messages/detail.ts",
    "web/src/lib/i18n/messages/fields.ts",
    "web/src/lib/i18n/messages/list.ts",
    "web/src/lib/i18n/messages/personal.ts",
    "web/src/lib/i18n/messages/settings.ts",
    "web/src/lib/i18n/messages/shell.ts",
    "web/src/lib/i18n/messages/write.ts",
]

JA_OPEN = re.compile(r"^(\s*ja:\s*)(['\"`])")


def catalogue_mask(text: str) -> list[tuple[int, int]]:
    """Every byte of a catalogue that is not inside a ja string's own text.

    Protecting the complement is how a non-contiguous region is expressed with
    the machinery this file already has: fix() then sees exactly the Japanese
    and nothing else, and the en and ko values — which legitimately carry
    spaces everywhere — cannot be touched even by a bug here, because the
    assert in fix() only ever permits a deletion inside what it was shown.

    A value may run past its line (a template literal wrapped by the
    formatter), so the opening quote is carried until the line that closes it.
    """
    out: list[tuple[int, int]] = []
    pos = 0
    quote = ""  # the delimiter of a ja value still open from an earlier line
    for line in text.split("\n"):
        end = pos + len(line)
        if quote:
            close = line.find(quote)
            if close < 0:
                pos = end + 1
                continue  # the whole line is Japanese
            out.append((pos + close, end))
            quote = ""
            pos = end + 1
            continue
        m = JA_OPEN.match(line)
        if not m:
            out.append((pos, end))
            pos = end + 1
            continue
        a = pos + m.end()
        out.append((pos, a))
        close = line.find(m.group(2), m.end())
        if close < 0:
            quote = m.group(2)  # runs onto the next line
        else:
            out.append((pos + close, end))
        pos = end + 1
    return out

TAG = re.compile(r"<[^>]+>")
FENCE = re.compile(r"^\s*(```|~~~)")


def protected(text: str) -> list[tuple[int, int]]:
    """Ranges whose spaces are the content: code spans and fenced blocks.

    Nothing is substituted out of the text — an earlier version swapped each
    span for a sentinel and put it back afterwards, and one escaped backtick
    (`Ctrl+Shift+\\``) mispaired the delimiters, swallowed two release
    sections of CHANGELOG.ja.md and put them back somewhere else. A mask
    cannot do that: every character stays where it is and the only edit this
    file ever makes is the deletion of a space.

    A backtick span may cross a line — an 80-column wrapper puts them there —
    so the open delimiter is carried to the next line and released at the
    blank line that ends the paragraph, which is as far as a span can reach.
    An unbalanced delimiter therefore mis-marks at most one paragraph, and a
    mis-marked span means a space is *kept*, never that text moves.
    """
    out: list[tuple[int, int]] = []
    for m in re.finditer(r"<code\b[^>]*>.*?</code>", text, re.S):
        out.append(m.span())
    pos, fenced, carried = 0, False, -1
    for line in text.split("\n"):
        if FENCE.match(line):
            fenced = not fenced
            out.append((pos, pos + len(line)))
        elif fenced:
            out.append((pos, pos + len(line)))
        elif not line.strip():
            carried = -1  # a code span cannot cross a blank line
        else:
            ticks = [pos + m.start() for m in re.finditer("`", line)]
            if carried >= 0 and ticks:
                out.append((carried, ticks[0] + 1))
                ticks = ticks[1:]
                carried = -1
            for a, b in zip(ticks[::2], ticks[1::2]):
                out.append((a, b + 1))
            if len(ticks) % 2:
                carried = ticks[-1]
        pos += len(line) + 1
    return out


def region(text: str, bounds: tuple[str, str] | None) -> tuple[int, int]:
    """Character offsets of the Japanese part of the file."""
    if bounds is None:
        return 0, len(text)
    open_line, close_line = bounds
    start = text.index(open_line + "\n") + len(open_line) + 1
    end = text.index("\n" + close_line + "\n", start) + 1
    return start, end


def neighbours(text: str, i: int) -> tuple[str, str]:
    """The rendered characters either side of the whitespace run at `i`.

    Markup that renders as nothing is skipped, so adjacency is read the way
    a reader sees it: `接続先。</strong> Atlassian` is 。then A, and so is
    `…しました。** gadak`. Both were counted as Latin-on-both-sides while
    the skip list held HTML tags only, which is how a bold lead-in kept its
    space through the first pass.
    """
    MARKUP_END = re.compile(r"(<[^>]+>|\*{1,3}|_{1,3})[ \t]*$")
    MARKUP_START = re.compile(r"[ \t]*(<[^>]+>|\*{1,3}|_{1,3})")
    j = i
    while j > 0 and text[j - 1] in " \t":
        j -= 1
    left = text[:j]
    while True:
        m = MARKUP_END.search(left)
        if not m:
            break
        left = left[: m.start()]
    k = i
    while k < len(text) and text[k] in " \t":
        k += 1
    right = text[k:]
    while True:
        m = MARKUP_START.match(right)
        if not m:
            break
        right = right[m.end() :]
    return (left[-1] if left else ""), (right[0] if right else "")


def runs(text: str, keep: list[tuple[int, int]]) -> list[tuple[int, int]]:
    """Every whitespace run this file may delete, as (start, end) offsets."""
    out = []
    for m in re.finditer(r"[ \t]+", text):
        i, j = m.span()
        if i == 0 or text[i - 1] == "\n":
            continue  # leading indentation renders as nothing
        if text[j : j + 1] in ("", "\n"):
            continue  # trailing whitespace is not a rendered space either
        if any(a <= i < b for a, b in keep):
            continue
        left, right = neighbours(text, i)
        if IS_JA.search(left) or IS_JA.search(right):
            out.append((i, j))
    return out


def fix(text: str, extra: list[tuple[int, int]] | None = None) -> tuple[str, list[tuple[int, int]]]:
    """Delete every rendered space run that touches a Japanese character.

    `extra` adds protected ranges beyond the code spans protected() finds —
    it is how a catalogue names the part of itself that is not Japanese.
    """
    found = runs(text, protected(text) + (extra or []))
    out, last = [], 0
    for i, j in found:
        out.append(text[last:i])
        last = j
    out.append(text[last:])
    fixed = "".join(out)
    # The only difference this file is allowed to make is whitespace. An
    # edit that changes anything else is the bug above, and it refuses.
    assert "".join(fixed.split()) == "".join(text.split()), "ja-spacing moved text"
    return fixed, found


def main() -> int:
    mode = sys.argv[1] if len(sys.argv) > 1 else "--check"
    if mode not in ("--check", "--write"):
        print(__doc__)
        return 2
    total = 0
    sources = [(rel, bounds, False) for rel, bounds in TARGETS]
    sources += [(rel, None, True) for rel in CATALOGUES]
    for rel, bounds, catalogue in sources:
        path = ROOT / rel
        if not path.exists():
            print(f"ja-spacing: missing {rel}")
            return 2
        text = path.read_text(encoding="utf-8")
        start, end = region(text, bounds)
        body = text[start:end]
        fixed, found = fix(body, catalogue_mask(body) if catalogue else None)
        if not found:
            continue
        total += len(found)
        if mode == "--write":
            path.write_text(text[:start] + fixed + text[end:], encoding="utf-8")
            print(f"ja-spacing: {rel}: {len(found)} space(s) removed")
        else:
            base = text[:start].count("\n")
            seen: set[int] = set()
            for i, _ in found:
                line = base + body[:i].count("\n") + 1
                if line in seen:
                    continue
                seen.add(line)
                ctx = body.split("\n")[body[:i].count("\n")].strip()[:90]
                print(f"FAIL {rel}:{line}: {ctx}")
    if mode == "--write":
        print(f"ja-spacing: {total} space(s) removed across {len(sources)} files")
        return 0
    if total:
        print()
        print(f"ja-spacing: {total} ASCII space(s) beside a Japanese character.")
        print("  Japanese is set solid: a space goes between two Latin runs and")
        print("  nowhere else. Run tools/ja-spacing.py --write.")
        return 1
    print(f"ja-spacing: {len(sources)} Japanese sources, no space beside a Japanese character")
    return 0


if __name__ == "__main__":
    sys.exit(main())
