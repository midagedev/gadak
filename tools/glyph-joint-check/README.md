# glyph-joint-check — do box glyphs join under font stack X on engine Y? (GDK-1043)

Standing tool promoted from the GDK-1043 investigation scratch (2026-08-28).
Static page + Playwright at deviceScaleFactor 2: loads @xterm/xterm from this
repo's node_modules (inlined into the page), renders a box-drawing pattern
through the same terminal options the pane ships (fontSize 13, no lineHeight,
no customGlyphs toggles), screenshots the terminal element, and judges the
joints NUMERICALLY from the PNG — dark = luminance < 128. No gadak serve, no
ports, no repo files touched outside `out/`.

## macOS only — and CI must never assert font pixels

The verdicts are about CoreText font resolution on this platform: WebKit
resolves `ui-monospace` to SF Mono (box-glyph ink 15.31css < the 16css cell
xterm derives → 1px seam per row), Chromium resolves it to Menlo (ink 15.14 ≥
cell 15.00 → joins). Linux fontconfig resolves the same stacks differently,
so a Linux run can explore but must never gate. The order-contract gate CI
runs instead is `web/src/lib/terminal/font-stack.test.ts` — it pins the stack
as text, not pixels.

## Requirements

- Node + this repo's node_modules (`npm ci` at the repo root — the tool loads
  `playwright` and `@xterm/xterm` from there).
- Playwright browsers: `npx playwright install chromium webkit` (webkit is
  the engine the desktop app and the phone actually run).

## Run (from the repo root)

```sh
node tools/glyph-joint-check/judge.mjs --engine=webkit              # all configs
node tools/glyph-joint-check/judge.mjs --engine=webkit menlo-first   # one config
node tools/glyph-joint-check/judge.mjs --engine=chromium baseline
node tools/glyph-joint-check/judge.mjs --engine=webkit --stack="Menlo, monospace"
TH=100 node tools/glyph-joint-check/judge.mjs --engine=webkit baseline
```

`--stack=` runs one ad-hoc config named `ad-hoc` — that is the "stack X on
engine Y" door for any future candidate. Shots and verdict JSONs land in
`tools/glyph-joint-check/out/` (untracked; delete freely). `TH` picks the
luminance threshold (default 128); every result also records its verdict at
TH=100 and TH=160, and no verdict should flip across those.

## Configs

- `baseline` — `--font-mono` as shipped (`ui-monospace` first). WebKit
  resolves it to SF Mono → BROKEN with vgaps=15 and floating `└┴┘` arms:
  this is the GDK-1043 defect, kept runnable so a regression is one command.
- `menlo-first` — `--font-mono-terminal`, the fix: JOINED, vgaps=0, 한글
  inked, on both engines.
- `font-sfmono-only` — SF Mono alone: the broken resolution isolated.
- `font-monaco` — the rejected second candidate (narrower cell, vgaps=150
  class: a sans-class face reaching box glyphs destroys the grid).

## Verdict model

JOINED = zero interior gap rows in every vertical member (a gap row = no dark
pixel within ±1px of the stroke column, counting only between the member's
first and last ink row) + every adjacent horizontal band pair overlapping
(±1px) + the └ ┴ ┘ ink reaching the ─ band (±1px) + 한글 cells inked.
The void above/below a glyph inside its own cell (e.g. └'s bottom void) is
font design, not a seam — it is reported as `cornerFloat`, not counted.
Each verdict JSON also carries the cell metrics, a per-family canvas ink
probe, and the font-identity ranking (which family's ink fingerprint the
terminal's own glyphs match).
