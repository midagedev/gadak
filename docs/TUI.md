# The terminal UI

```bash
gadak tui
```

Reads the same `~/.gadak/gadak.db` the web UI and your agent read. It never talks
to Jira on its own — sync is `gadak sync` or `gadak serve` — except when you
comment, transition, reassign, or edit a field, which go straight through and
then refresh that one issue in the mirror.

## Keys

| Key | Does |
| --- | --- |
| `j` / `k`, `↓` / `↑` | move the cursor |
| `g` / `G` | first / last row |
| `PgDown` / `Ctrl+D`, `PgUp` / `Ctrl+U` | page down / page up |
| `1` `2` `3` `4` | list: all / open / in progress / done · feed: all / assignee / reporter / mention · docs: updated / by author / spaces (`4` unused in docs) |
| `D` | docs mode (toggle): mirrored wiki pages |
| `/` | filter by key, summary, or assignee (local, per keystroke; matches are highlighted in the list) |
| `Ctrl+K` | command palette — fuzzy jump to any tab, action, saved view, or issue |
| `Enter` | open detail |
| `Esc` | back, or clear the filter / leave feed or views |
| `c` | comment |
| `t` / `s` | transition (`s` matches the web UI's status key) |
| `a` | assign |
| `e` | edit a field — pick from your editable fields (settings / field specs), then pick a value from what Jira allows on that issue; multi-selects preselect the current values. No free-text input: options, versions, and users only |
| `w` | watch / unwatch the current issue |
| `F` | personal feed (toggle): activity list; `Enter` opens the issue; `r` marks all read |
| `v` | saved views picker — apply a stored view (supported filters only) |
| `?` | help overlay (actual bindings only) |
| `r` | re-read the mirror from disk (in feed mode: mark all feed events read) |
| `q`, `Ctrl+C` | quit |

Multi-select (`x` in the web UI) is web-only for now — the TUI acts on the
cursor row. Batch status/assignee changes live in the web list's bulk bar.

Write keys are inert until a credential is configured (`gadak init`); the status
bar says so rather than failing at submit time.

### Mouse

The list is clickable: wheel scrolls, a click moves the cursor, a second click
on the selected row opens it, and the tab bar switches on click. The feed works
the same way. Everything remains fully keyboard-driven — the mouse is an
addition, never a requirement.

### Ambient neon

The header wordmark, active tab, and selected row breathe on a slow gradient
even when nothing is happening; the filter query is highlighted in matching
rows. All of it derives from one global tick and switches off automatically
under `NO_COLOR`, or explicitly with `GADAK_NO_ANIM=1`. Piped output was never
animated to begin with.

### Feed

`F` loads the personal feed from the mirror (`store.Feed`): recent activity on
issues you watch, are assigned to, reported, or were mentioned on (30-day window).
Unread count appears as a `feed N` chip on the list status bar (always the **all**
focus total). In feed mode, `1`–`4` switch focus tabs (`all` / `assignee` /
`reporter` / `mention`); each tab shows its unread badge when non-zero. `r` marks
every event read (all focus) and reloads the current focus list.

### Docs

`D` opens mirrored wiki pages from the same SQLite mirror. Inside docs mode,
`1` / `2` / `3` switch the list axis (same keys as issue status tabs and feed
focus — meaning depends on mode):

| Key | View | What it shows |
| --- | --- | --- |
| `1` | **Updated** (default) | Every page, flat, `updated_at` desc |
| `2` | **By author** | Author group headers (`▸ name (n)`); empty author is `(no author)`; pages within a group by `updated_at` desc |
| `3` | **Spaces** | Space-grouped parent/child tree (same nesting rules as the web space tree); parents show a muted direct-child count (unfiltered total) |

Each row is the title plus a dimmed meta clause: `author · relative time · in
space name` (space key when the mirror has no name yet). On **Updated** and **By
author**, a muted one-line body excerpt sits under the meta when the page has
one (`PageLite.excerpt` — empty bodies omit the line). **Spaces** never shows
excerpts (tree is for navigation, not discovery — same as the web UI). `/`
filters by title, space (key or name), or author (substring only — not
full-text); matching title spans are highlighted with the same accent as the
issue list (inert under `NO_COLOR`). On the Spaces tree, a filter also keeps
each hit's ancestors as muted path rows so the hierarchy still reads. `Enter`
opens plain-text detail (labels, body, comments, related issue keys /
backlinks); `Esc` / `D` leaves.

**Viewed** recency lives in the web UI's browser storage; the TUI does not track
visits yet. Full-text document search stays on the web UI / CLI.

**People** (web palette PEOPLE + person panel, `CommentsByAuthor`) is web-only;
the TUI has no person axis or activity-by-author surface.

**Search match hints** (`SearchResult.Matches` — body/comment snippet on FTS
hits) are CLI/web only. TUI `/` is a local substring filter over already-loaded
issue rows (key, summary, assignee) or docs (title, space, author); it never
calls `store.Search`, so there is no match field or snippet to surface.

#### v0.10 document-wave parity

| Web feature | TUI |
| --- | --- |
| Filter haystack: title + space + **author** | **supported** — `/` matches title, space key/name, author (and page key) |
| Filter match highlight on list titles | **supported** — `styleHighlight` (same as issue list); no-op under `NO_COLOR` (row still present) |
| Spaces tree: parent direct-child count (unfiltered total) | **supported** — muted count after title on parents with children |
| Spaces tree: filter keeps path ancestors, muted | **supported** — non-hit ancestors stay as muted path rows |
| Issue ↔ document cross-references (`item_refs`) | **supported** — issue detail: Related pages / Mentioned in; page detail: Related issues / Mentioned from |
| Document label chips on list rows | **unsupported** — narrow terminal width; labels show on page detail only |
| Document label filter (chip → narrow) | **unsupported** — no dedicated label-filter UI in the TUI |
| Document deeplink (URL) | **unsupported** — TUI has no address bar / shareable URL |

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
reported, not faked. If you need the full filter surface, `gadak sql` or the web
UI is faster.
