# Demo media assets

Regenerable GIF/MP4 clips for the public README hero, social posts, and docs.
All assets are produced from **scripts** against the scrubbed snapshot
`examples/demo.db` — no hand-recorded screen captures, no real company data.

## Assets

| File | Source | Use |
| --- | --- | --- |
| `docs/media/web-demo.gif` | Playwright walkthrough | README hero (inline) |
| `docs/media/web-demo.mp4` | same recording, h264 | Twitter / LinkedIn / anywhere GIF is too heavy |
| `docs/media/tui.gif` | VHS tape `tools/tapes/tui.tape` | README / docs — terminal navigator |
| `docs/media/agent.gif` | VHS tape `tools/tapes/agent.tape` | README / docs — a real Claude Code session on the mirror |

## Size budget

| Asset | Budget | Notes |
| --- | --- | --- |
| `web-demo.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | GitHub README inline limit; palette 2-pass |
| `web-demo.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `tui.gif` / `agent.gif` | soft ≤ 5 MB each | VHS output + `gifsicle -O3 --colors 64` |

### Current committed sizes (re-measure after regen)

Measured 2026-08-06 via `ls -la docs/media/` (decimal MB = bytes/1e6):

| Asset | Size | Bytes (`ls -la`) | Duration | Resolution / fps |
| --- | --- | --- | --- | --- |
| `web-demo.gif` | 6.55 MB | 6549601 | 22.4 s | 960×600 @ 9 fps, 128-color palette |
| `web-demo.mp4` | 0.95 MB | 952142 | 22.4 s | 1024×640 h264 |
| `tui.gif` | 4.73 MB | 4728239 | 17.0 s | 1080×620, 77×24 cells, 64 colors |
| `agent.gif` | 305 KB | 304708 | 31.5 s | 1080×620, 77×24 cells, 64 colors |

## Readability comes first, and it costs bytes

Readers reported the text was too small in every clip. The only real lever is
the **logical** size of what is being recorded — a wider viewport or a wider
terminal makes the glyphs smaller, because the README renders these at a fixed
width (900 px for the web GIF, 800 px for the terminal ones).

So the recordings are deliberately small:

- Web: viewport **1024×640**, not 1280×800. 1024 is the floor that still matches
  Tailwind's `lg:` breakpoint, below which the row's epic chip and trailing
  strip disappear and the UI stops looking like itself.
- Terminals: **77 cols × 24 rows** (`Set Width 1080` / `Set FontSize 20`). Under
  ~72 cols the TUI's summary column collapses into ellipses; past ~90 the glyphs
  land under 13 px in the README.

**Web GIF tradeoff:** the walkthrough now carries six beats (search, unified
search with documents, the docs tree, epic grouping, palette, epic rollup) at a
larger logical scale, and that does not fit the prefer-≤5 MB target — it lands
around 7 MB against the 8 MB hard limit. `export-video.sh` starts at
`fps=9 width=960 colors=128` and steps down to `8/960/96`, then `8/900/64`.
Note that fewer colors does **not** reliably shrink a GIF here: bayer dither
noise compresses worse, so a 96-colour pass can come out larger than a 128 one.
Motion is the real cost — an instant `scrollTop` jump costs a fraction of a
one-second smooth scroll. Cut a beat or a scroll before cutting resolution, and
point anyone who wants a light asset at the MP4.

**Terminal GIFs:** VHS's own encoder is generous (7–8 MB for these clips), and
`Set Framerate` does not change the GIF's 25 fps frame table. Both tapes are
finished with `gifsicle -O3 --colors 64`, which roughly halves them with no
visible loss on a 64-colour terminal theme. `make media-tui` / `make media-agent`
do **not** run that pass yet — run it by hand (below) after recording.

## Regenerate everything

```bash
# Prerequisites (already on a typical gadak dev machine):
#   go, node ≥ 20, ffmpeg, vhs (brew install vhs), Playwright chromium

make media
```

Individual targets:

```bash
make media-web     # Playwright → webm → gif + mp4 (self-contained)
make media-tui     # VHS TUI navigator            → needs the gifsicle pass
make media-agent   # VHS Claude Code session      → needs setup + gifsicle pass
make media-prep    # build gadak + seed tools/tapes/.tmp from demo.db
```

The two terminal clips need steps `make` does not run:

```bash
# agent.gif only — an isolated HOME holding a copy of this machine's Claude
# Code credentials, so the take is a real session with no identity on screen.
bash tools/tapes/prepare-agent.sh
make media-agent
bash tools/tapes/prepare-agent.sh --clean     # removes the credential copy

# both terminal clips — VHS output is 2× the budget without this
gifsicle -O3 --colors 64 docs/media/tui.gif -o docs/media/tui.gif

# agent.gif also gets its idle tail cut. Model latency varies per take, so find
# the frame where the answer finishes and keep everything up to ~3 s after it
# (frames are 25 fps regardless of `Set Framerate`):
gifsicle --info docs/media/agent.gif | head -2          # total frame count
gifsicle -O3 --colors 64 docs/media/agent.gif '#0-786' -o docs/media/agent.gif
```

Outputs land in `docs/media/`. Commit them — the README references the paths
directly, and CI does not regenerate media.

## What each recording shows

### Web UI (`web-demo`)

~22 s of readable motion, viewport **1024×640** @ `deviceScaleFactor: 2`:

1. Boot — 534-issue list
2. Instant local search with per-keystroke narrowing and `<mark>` highlights
3. Sidebar **Documents** — the Viewed tab, then **Updated** (every page,
   newest edit first, rows reading `author · time · in space`), then one
   page open in the document panel with its breadcrumb trail
4. **Spaces** disclosure — a space as a flat list, then its Tree toggle,
   one branch opened
5. Sidebar **Epics** built-in view: the open backlog re-sectioned by epic in
   one click, headers carrying the epic key and summary

Re-shoot trap (2026-08-06): the demo config reuses an already-running 7877
server (`reuseExistingServer: true`), so a stale fixture prints an orange
`Sync delayed` chip into every frame. Restart the server (or re-run the
`GADAK_FRESHEN` block in `e2e/serve.sh` against `e2e/.tmp/home/gadak.db`)
before recording.

Note the video size in `e2e/demo/playwright.config.ts` must equal the viewport.
Playwright does not upscale into a larger video frame — it pins the capture in
the top-left corner and pads the rest black.

Config lives in `e2e/demo/playwright.config.ts` (separate from the E2E suite).
The demo test is gated by `GADAK_MEDIA=1` so a plain
`playwright test --config e2e/playwright.config.ts` still runs only the original
10 specs (the demo file is discovered as **skipped**).

No caption overlays are injected into the DOM — the app code under `web/src/`
is never touched by this pipeline.

### TUI (`tui.gif`)

List → `/` filter typing (live `pagination` highlights) → clear → `D` docs tree
→ `j` down the tree → Enter for a page (breadcrumb + body) → Esc → `D` back →
`Ctrl+K` palette → `q`.

### Agent (`agent.gif`)

Two scenes, both real:

1. `claude mcp add gadak -- gadak mcp` — the one line that teaches an MCP client
   the mirror exists.
2. A live Claude Code session answering *"Which epic has the most reopened
   issues?"* — the model calls `gadak_query` twice and answers with the epic
   keys. Nothing is scripted, so takes differ; re-run until one reads well.

The point of the clip: the agent answers a question **JQL cannot express**
(reopen history aggregated by epic) from a local file, without scraping a UI.

If the host has no Claude Code login, `prepare-agent.sh` refuses and the tape
cannot run — the earlier CLI-only version of this tape (`gadak search` / `gadak
sql` / `gadak issue` / `gadak status`) is in git history if you need a fallback.

## Font notes (Hangul alignment)

The demo seed keeps some Korean Jira status/type labels (`완료`, `진행 중`,
`버그`). Terminal columns need a **monospace CJK** font or Hangul cells
mis-width and the TUI drifts.

Tapes set:

```text
Set FontFamily "D2Coding, Menlo, Monaco, monospace"
```

D2Coding is present on the machine that produced the committed GIFs. On another
host, install one of:

- D2Coding
- Sarasa Mono K / Sarasa Gothic
- Noto Sans Mono CJK

Check with `fc-list :lang=ko family` or `system_profiler SPFontsDataType`. If no
CJK mono font is available, remove `Set FontFamily` and accept Latin-only
alignment — note that in the tape comment when you do.

## Privacy / no real PII

- Data source is always `examples/demo.db` (fictional people and
  `*.example.com` addresses).
- VHS tapes `source tools/tapes/.tmp/env.sh` under `Hide` so the visible prompt
  is a bare `$ ` — **no username, hostname, or home path**.
- `HOME` is redirected to `tools/tapes/.tmp/fake-home` for the recording shell.
- `agent.tape` goes further, because Claude Code prints paths and account
  state. `prepare-agent.sh` builds `/private/tmp/gadak-demo` (a neutral path, so
  `claude mcp add` has nothing identifying to print), copies the credential file
  mode 0600, and copies **only** `accountUuid` / `organizationUuid` /
  `billingType` / `seatTier` from the account block — `organizationName`,
  `displayName` and `emailAddress` are the operator's real identity and are
  dropped. It also unsets inherited `CLAUDE_CODE_*` variables, which otherwise
  put warning banners on screen that no ordinary user would see. Run
  `prepare-agent.sh --clean` when you are done: it holds a copy of a live
  credential.
- Do not re-record against a real Jira mirror. If a frame ever shows a real
  company, person, or domain, discard the asset and re-seed from `examples/`.

## When to re-record

Re-run `make media` whenever any of these change in a way that is visible:

- `examples/demo.db` is regenerated (`tools/seed-demo` or similar)
- Web UI layout / copy that appears in the walkthrough
- TUI keybindings or list columns
- CLI output shape for `search` / `sql` / `issue` / `status`

The mirror is disposable; the scripts are the source of truth. After regenerating,
spot-check frames with `ffprobe` and a mid-clip PNG extract before committing:

```bash
ffprobe -v error -show_entries format=duration,size -of default=nw=1 docs/media/web-demo.gif
ffmpeg -y -ss 00:00:02 -i docs/media/web-demo.gif -frames:v 1 /tmp/web-frame.png
```

## Layout of the pipeline

```text
e2e/demo/
  playwright.config.ts   # video on, fixed viewport — not the E2E suite
  web-demo.spec.ts       # GADAK_MEDIA=1 gated walkthrough
  export-video.sh        # webm → gif (palette 2-pass) + mp4
tools/tapes/
  prepare.sh             # build binary, seed GADAK_HOME from demo.db
  prepare-agent.sh       # isolated HOME + auth for the Claude Code take
  tui.tape / agent.tape  # VHS scripts
  .tmp/                  # disposable (gitignored)
docs/media/              # committed outputs
```
