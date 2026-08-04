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
| `Esc` | back, or clear the filter / leave feed or views |
| `c` | comment |
| `t` | transition |
| `a` | assign |
| `w` | watch / unwatch the current issue |
| `F` | personal feed (toggle): activity list; `Enter` opens the issue; `r` marks all read |
| `v` | saved views picker — apply a stored view (supported filters only) |
| `?` | help overlay (actual bindings only) |
| `r` | re-read the mirror from disk (in feed mode: mark all feed events read) |
| `q`, `Ctrl+C` | quit |

Write keys are inert until a credential is configured (`scry init`); the status
bar says so rather than failing at submit time.

### Feed

`F` loads the personal feed from the mirror (`store.Feed`): recent activity on
issues you watch, are assigned to, reported, or were mentioned on (30-day window).
Unread count appears as a `feed N` chip on the list status bar. In the feed list,
`r` marks every event read and reloads.

### Saved views

`v` lists rows from `saved_views`. Applying a view maps what the TUI can honour:

- `status_category` → tab / category filter
- `assignee_email` / `assignee` → assignee filter
- `q` / `text` → the `/` text filter
- `unassigned` → unassigned-only

Anything else (labels, sort, group_by, stale, …) is ignored and the status bar
says `unsupported filter ignored: …`. That is intentional honesty, not a silent
drop.

### Narrow terminals

Below 40 columns the list shows only key + summary, and the status bar shortens
its help strip.

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

Triage is the scope: find the issue, read it, move it, watch it, skim the feed.
Grouping, bulk edits, attachments, and rich-text rendering stay in the web UI.
Saved views apply only the filters the TUI can express; the rest is reported, not
faked. If you need the full filter surface, `scry sql` or the web UI is faster.
