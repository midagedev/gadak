# 0005 — Two surfaces over one store

Status: superseded (2026-08-13)
Date: 2026-08-04
Superseded: 2026-08-13

## Context

gadak originally shipped three renderers over one SQLite mirror: a web UI
(Svelte), a TUI (`gadak tui`, Bubble Tea), and a CLI that doubles as the agent
interface. The TUI existed so people who live in tmux could stay there. It
invited an obvious consolidation (render the TUI in the browser) which this
decision originally rejected — a character grid cannot carry ADF, attachments,
or a screen reader.

## Original decision (2026-08-04)

Keep the three surfaces distinct. Do not render the TUI in the browser as the
primary web experience. The TUI covers triage only and must not become a
second implementation of every web feature.

## Reversal (2026-08-13)

Drop the TUI. Two surfaces remain: the web UI (and the desktop window around
it), and the CLI / SQL / MCP interface for agents and scripts.

The TUI's cost was not the xterm.js question this decision originally
answered. It was the lockstep tax: every web-UI wave needed a TUI follow-up
in the same version, or an honest "unsupported" — a second keymap, a second
layout, a second neon look, a second GIF. That split attention across two
human products when the agent half of the product already has a surface (the
database). The web UI is the comfortable surface; the CLI is the agent
surface. A third renderer does not earn its keep.

## Why this is not the xterm.js plan coming back

The original reasons a character grid is a bad *primary* UI still hold. We
are not putting the TUI in the browser. We are deleting it.

## Consequences

- `gadak tui` is gone. `internal/tui` and the Charmbracelet dependency tree
  go with it. `go-runewidth` stays for CLI table alignment (`gadak fields`).
- Web-UI waves no longer carry a TUI parity obligation.
- WAL still lets `serve`, the desktop app, and an agent hold the file at once.
- The agent tape (`tools/tapes/agent.tape`) stays; the TUI tape does not.
