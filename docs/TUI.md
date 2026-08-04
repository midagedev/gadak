# The terminal UI

```bash
scry tui
```

Reads the same `~/.scry/scry.db` the web UI and your agent read. It never talks
to Jira on its own — sync is `scry sync` or `scry serve --sync` — except when you
comment, transition, or reassign, which go straight through and then refresh that
one issue in the mirror.

## Keys

| Key | Does |
| --- | --- |
| `j` / `k`, `↓` / `↑` | move the cursor |
| `g` / `G` | first / last row |
| `1` `2` `3` `4` | all / open / in progress / done |
| `/` | filter by key, summary, or assignee (local, per keystroke) |
| `Enter` | open detail |
| `Esc` | back, or clear the filter |
| `c` | comment |
| `t` | transition |
| `a` | assign |
| `r` | re-read the mirror from disk |
| `q`, `Ctrl+C` | quit |

Write keys are inert until a credential is configured (`scry init`); the status
bar says so rather than failing at submit time.

## Fonts, and why Korean or Japanese columns can look wrong

Column alignment is computed in **terminal cells**, not characters: CJK
characters occupy two cells, so a Hangul summary is padded on its display width
(via `go-runewidth`). That is our side of the contract, and it is tested.

The other side is your font. A monospace font whose CJK glyphs do not advance at
exactly twice the Latin width will misalign every column after the first wide
character no matter how the padding is computed — the terminal reserves two cells
and the glyph draws over some other number of them. Most popular coding fonts
(JetBrains Mono, Fira Code, SF Mono) carry no Hangul or Kanji at all, so your
terminal silently substitutes a system font with unrelated metrics.

Fonts that are designed for this, with exact 1:2 metrics:

| Font | Notes |
| --- | --- |
| **D2Coding** | Korean-first, ligature variant available, widely used in Korea |
| **Sarasa Mono K / J / SC** | Iosevka + Source Han, per-language variants, very consistent |
| **Noto Sans Mono CJK KR / JP** | broad coverage, ships with many distributions |

Set one as your terminal's font and the grid lines up. If you cannot change the
font, narrower terminal widths help — the summary column truncates instead of
wrapping, so misalignment has less room to accumulate.

This is also the honest reason the web UI is not a terminal emulator in a
browser: there we could ship the font and guarantee the metrics, but a DOM gives
up rich text, images, find-in-page, and screen readers to get it. See
`decisions/0005-three-surfaces.md`.

## What it deliberately does not do

Triage is the scope: find the issue, read it, move it. Saved views, grouping,
bulk edits, attachments, and rich-text rendering stay in the web UI. If you find
yourself wanting those in the terminal, `scry sql` is usually the faster answer.
