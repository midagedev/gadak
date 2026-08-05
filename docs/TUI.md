# The terminal UI

```bash
scry tui
```

Reads the same `~/.scry/scry.db` the web UI and your agent read. It never talks
to Jira on its own — sync is `scry sync` or `scry serve` — except when you
comment, transition, or reassign, which go straight through and then refresh that
one issue in the mirror.

## Keys

| Key | Does |
| --- | --- |
| `j` / `k`, `↓` / `↑` | move the cursor |
| `g` / `G` | first / last row |
| `1` `2` `3` `4` | list: all / open / in progress / done · feed: all / assignee / reporter / mention |
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
Unread count appears as a `feed N` chip on the list status bar (always the **all**
focus total). In feed mode, `1`–`4` switch focus tabs (`all` / `assignee` /
`reporter` / `mention`); each tab shows its unread badge when non-zero. `r` marks
every event read (all focus) and reloads the current focus list.

### Saved views

`v` lists rows from `saved_views`. Applying a view maps what the TUI can honour:

**Filters**

- `status_category` → tab / category filter
- `assignee_email` / `assignee` → assignee filter
- `q` / `text` → the `/` text filter
- `unassigned` → unassigned-only

**Display** (`display` object, same shape as the web UI)

- `sort`: `updated`, `created`, `priority`, `reopen_count` (default direction
  `desc`). Empty / missing field values always sort last. `relevance` is not
  supported (no text ranking in the TUI) and is reported as `sort=relevance`.
  `priority` sorts on `priority_rank` — the position in your site's own priority
  list — never on the name, because Jira localizes priority names per account
  language. That is also what keeps the same saved view in the same order here
  and in the web UI.
- `dir`: `asc` or `desc`
- `group_by`: `status`, `status_category`, `assignee`, `priority` only. Other
  values are reported as `group_by=<value>` and grouping stays off. Group headers
  are screen lines (`▸ label (n)`); the cursor moves issue-to-issue only.

When a non-default sort is active, a short `sort:…` chip appears next to the view
name (omitted below 40 columns). Leaving the view with `Esc` restores default
list order (updated desc, no grouping).

Anything else (labels, stale, columns, unknown keys, …) is ignored and the status
bar says `unsupported filter ignored: …`. That is intentional honesty, not a
silent drop.

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
Bulk edits, attachments, and rich-text rendering stay in the web UI. Saved views
apply filters plus the sort/group keys listed above; unsupported values are
reported, not faked. If you need the full filter surface, `scry sql` or the web
UI is faster.
