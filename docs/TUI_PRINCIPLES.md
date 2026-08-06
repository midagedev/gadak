# TUI principles

The standard every TUI wave is measured against — the terminal counterpart of
[`UX_PRINCIPLES.md`](UX_PRINCIPLES.md), which still applies (speed budgets,
opinionated defaults, vocabulary). This file covers what is terminal-specific.
Specs for TUI rounds quote the relevant sections; disagreements end in either
a fixed change or a dated revision here.

Sources are the observable rules of TUIs worth copying — lazygit, k9s, fzf,
tig, gh-dash — plus the Charm (Bubble Tea/Bubbles/Lip Gloss) contracts scry
is built on, clig.dev, and the NO_COLOR standard. Links at the end.
(Established 2026-08-06.)

## 0. Lineage: resource browser, not pager

`?` opens help and `/` narrows the current list. That places scry in the
lazygit/k9s lineage, not the less/vim/tig lineage where `?` is reverse
search. This is a declared choice: scry's `/` is not a pager search (there is
no `n`/`N` and there should not be), it is a filter over a live list, k9s
style. Requests that assume pager conventions are judged against this line.

## 1. Help is generated from bindings — hand-written help always drifts

The `bubbles/key` pattern (`WithKeys` + `WithHelp`, matched via
`key.Matches`) exists precisely so "help text stays synchronized with actual
keybindings." Every key the TUI answers to lives in `keys.go`'s `keyMap`;
handling a key by comparing `msg.String()` directly is how bindings escape
the help overlay, the docs table, and the tests at once.

`docs/TUI.md` promises the `?` overlay shows "actual bindings only" — that
promise is only checkable if the overlay is generated. A help-coverage test
(every `keyMap` binding appears in `helpLines()`) is the regression fence.

## 2. Discovery is two-tier: ambient hints plus on-demand reference

lazygit runs a persistent bottom line *and* a `?` menu, and even prints
jump-keys in panel titles (`showPanelJumps`); Bubbles' help bubble has
short/full modes for the same reason. One tier is not enough: with only a
reference, nobody opens it; with only hints, advanced keys die.

scry has the status-bar strip and the `?` overlay. The remaining move from
this pattern: mode labels carry their own keys (`docs (D)`, `feed (F)`) —
with two domains in one app, the domain-switch keys deserve ambient display.

## 3. Going somewhere and narrowing here are different axes

k9s separates `:` (navigate to a resource) from `/` (filter the current
view); fzf's whole model is query-narrows, enter-selects. scry's `Ctrl+K`
palette is the navigate axis and `/` the narrow axis — keep them unmixed.

Filters grow small grammars (k9s: `!` negation, `-l` labels, `-f` fuzzy;
fzf: multi-term, prefix operators). When `/` needs more power, extend it with
operators rather than adding a second filter surface.

## 4. Esc steps back once; quitting is a separate, deliberate act

k9s: esc "bails out of view, command, or filter mode" — one step. lazygit
ships `quitOnTopLevelReturn: false` *as the default*: at top level, esc does
nothing. tig splits `q` (pop one view) from `Q` (quit all). The only tools
where esc quits are single-screen ones (fzf) — if there is a view stack,
esc ≠ quit.

scry's esc already steps back (detail → list, clear filter, leave feed).
Open decision, recorded 2026-08-06: `q` currently quits from anywhere,
including detail views where "close this" is the likelier intent. If revised,
tig's `q`/`Q` split is the precedent; until then, esc is the documented way
to step back.

## 5. Color is a status vocabulary, and the terminal outranks us

- **One color per state, no decorative color.** k9s skins bind colors to a
  status enum (new/modify/error/kill/completed); scry's
  `colNew/colIP/colDone/colReopen` follows the same shape. clig.dev: "if
  everything is a different color, then the color means nothing."
- **Degrade honestly.** Lip Gloss downsamples TrueColor → 256 → 16 → mono and
  strips ANSI on non-TTY — but only through its own writers. Status colors
  should survive 16-color terminals distinguishably; pin `Complete()` values
  if downsampling guesses wrong.
- **No color must lose no information.** NO_COLOR (set and non-empty — the
  standard's exact test, which `neon.go` already implements) means every
  state still needs a non-color signal: the status *word*, not a tinted dot.
- **Respect the terminal's background.** k9s ships a `default` color for
  exactly this; painted backgrounds belong to active/selected elements only.

## 6. Latency is admitted, not hidden — and the admission must not flicker

fzf dims the input, hides the cursor, and shows `(..)` while waiting, and
debounces that very indicator so it never flickers. clig.dev: print something
within 100ms; stuck progress reads as a crash.

scry's read path is local and instant (see UX §1 — no spinners). The places
this principle bites are the writes that go to Jira — comments, transitions,
assignments, field edits — plus disk re-reads and heavy filters. Those need
fzf-style treatment: visible waiting state, debounced, with cancel alive
(§9). A locked UI during a Jira round-trip is a violation.

## 7. Narrow terminals get re-layout before truncation

lazygit's `portraitMode: auto` restacks panels below 84 columns instead of
deleting information; Bubbles' help truncates only "gracefully"; fzf never
demands the full screen (`--height`). Truncation is the last resort.

Bubble Tea guarantees an initial `WindowSizeMsg`, so there is no
"unknown size" frame to design for. (Windows never reports subsequent
resizes — a documented platform limit to remember.)

## 8. CJK width is a locale problem, and it is ours

No surveyed source covers this axis; scry must, because its users type
Korean. Two distinct layers:

- **Fonts** (covered in `docs/TUI.md`): glyph advance width is outside our
  control — same reason lazygit ships nerd-font icons *off* by default.
  Cell-based padding via `go-runewidth` is the contract, and it is tested.
- **Locale**: `runewidth`'s default condition treats East Asian *ambiguous*
  characters (box drawing `─│`, symbols `●★`) as narrow; under a CJK locale
  (`EastAsianWidth: true`) they occupy two cells. Borders or chips built
  from ambiguous-width characters will misalign *only for CJK users*.
  Prefer ASCII or unambiguous glyphs in structural chrome, and test width
  math under both conditions.
- Width thresholds counted in cells hit CJK users earlier — a 40-column
  minimum holds half as many Korean characters. Breakpoints must be chosen
  with that in mind.

## 9. Escape hatches stay alive

Ctrl+C works always (clig.dev: "always make Ctrl-C still work"); fzf accepts
abort keys even while in `wait`. During any in-flight Jira write, cancel must
still respond. Destructive-versus-safe confirmation follows UX §7 — k9s even
documents which keys confirm (`ctrl-d`) and which do not (`ctrl-k`).

## 10. Parity with the web UI means visible differences, not equal features

Every web-UI feature wave gets a TUI follow-up in the same version: parity
where the TUI can express it, an honest "unsupported" report where it cannot
(standing rule in `AGENTS.md`). The TUI already models this — saved views
print `unsupported filter ignored: …` instead of silently dropping filters,
and write keys are inert-with-explanation *before* submission when no
credential exists. Silence is the only wrong answer.

## Verification hooks

Principles that do not compile into checks regress: help-coverage test (§1),
NO_COLOR snapshot with state words asserted (§5), narrow-width render golden
(§7), CJK width tests under both `EastAsianWidth` conditions (§8).

## Sources

[lazygit](https://github.com/jesseduffield/lazygit)
([keybindings](https://github.com/jesseduffield/lazygit/blob/master/docs/keybindings/Keybindings_en.md),
[config](https://github.com/jesseduffield/lazygit/blob/master/docs/Config.md)) ·
[k9s commands](https://k9scli.io/topics/commands/) ·
[k9s skins](https://k9scli.io/topics/skins/) ·
[fzf](https://github.com/junegunn/fzf) ·
[Bubble Tea](https://github.com/charmbracelet/bubbletea) ·
[Bubbles](https://github.com/charmbracelet/bubbles) ·
[Lip Gloss](https://github.com/charmbracelet/lipgloss) ·
[go-runewidth](https://pkg.go.dev/github.com/mattn/go-runewidth) ·
[UAX #11 East Asian Width](http://www.unicode.org/reports/tr11/) ·
[tig manual](https://jonas.github.io/tig/doc/manual.html) ·
[gh-dash](https://github.com/dlvhdr/gh-dash) ·
[clig.dev](https://clig.dev/) ·
[no-color.org](https://no-color.org/)
