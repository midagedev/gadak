# 0005 — Three surfaces over one store, not a terminal in a browser

Status: accepted
Date: 2026-08-04

## Context

gadak has a web UI (Svelte, DOM), a TUI (`gadak tui`, Bubble Tea), and a CLI that
doubles as the agent interface. That is three renderers for one SQLite mirror,
and it invites an obvious consolidation: build the TUI well, then serve it in the
browser through xterm.js and delete the web app. One layout, one keymap, one set
of bugs.

The question came up for a real reason — CJK column alignment in a terminal is
genuinely fragile, and a browser lets you ship the font instead of hoping the
user picked a good one.

## Decision

Keep the three surfaces distinct. Do not render the TUI in the browser as the
primary web experience.

## Why

xterm.js is not the weak part of that plan. Its mouse support is real: it
implements the SGR 1006 protocol, Bubble Tea reads it through
`tea.WithMouseCellMotion()`, and click-to-select, drag, and wheel scrolling all
work the way they do for anyone running lazygit inside VS Code. Shipping a
bundled font (D2Coding, Sarasa Mono K, Noto Sans Mono CJK) would make CJK cell
metrics *more* deterministic than a native terminal, where we cannot control
whether Hangul advances at exactly twice the Latin width.

The weak part is what a character grid cannot represent:

| Capability | DOM | Terminal grid |
| --- | --- | --- |
| Rich text (Jira ADF: tables, panels, code blocks) | native | flattened to plain text |
| Attachment thumbnails, avatars | native | needs sixel; not portable |
| Find-in-page (⌘F) | native | absent |
| Screen readers | semantic tree | a canvas with an a11y shim |
| Real hyperlinks | native | addon, URLs only |
| Responsive layout | native | fixed cell grid |
| Text selection while the app owns the mouse | native | needs a modifier key |

Replacing a working DOM app with a canvas would trade all of that for keymap
unification, and add a PTY bridge (WebSocket plus a pty library) to maintain for
what is otherwise a single-user local tool.

The three surfaces also serve genuinely different readers, which is the deeper
reason they should look different:

- **Web UI** — the comfortable surface. Mouse and keyboard, rich rendering,
  the one you leave open all day.
- **TUI** — for people who live in tmux and want the mirror without leaving it.
  Column alignment is computed in terminal cells (`go-runewidth`), not runes, so
  Hangul and CJK summaries stay aligned; the remaining variable is the user's own
  font, which the docs address.
- **CLI / SQL** — for agents and scripts. No layout at all.

## Consequences

- Keymaps stay deliberately similar (`j/k`, `Enter`, `Esc`, `/`) but are not
  shared code. Divergence where a surface's medium demands it is expected, not a
  bug.
- The TUI must not become a second implementation of every web feature. It
  covers triage: list, filter, detail, and the three write actions.
- xterm.js keeps one legitimate future use: a zero-install playground on a
  landing page, where a visitor drives the TUI in a browser tab against the demo
  snapshot. That is a marketing asset, not a product surface, and it does not
  change this decision.
