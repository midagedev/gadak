# Demo media assets

Regenerable GIF/MP4 clips for the public README hero, social posts, and docs.
All assets are produced from **scripts** against the scrubbed snapshot
`examples/demo.db` — no hand-recorded screen captures, no real company data.

## Assets

| File | Source | Use |
| --- | --- | --- |
| `docs/media/web-demo.gif` | Playwright walkthrough | README hero (inline) |
| `docs/media/web-demo.mp4` | same recording, h264 | Twitter / LinkedIn / anywhere GIF is too heavy |
| `docs/media/agent.gif` | VHS tape `tools/tapes/agent.tape` | README / docs — the CLI an agent types against the mirror |

## Size budget

| Asset | Budget | Notes |
| --- | --- | --- |
| `web-demo.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | GitHub README inline limit; palette 2-pass |
| `web-demo.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `agent.gif` | soft ≤ 5 MB | VHS output + `gifsicle -O3 --colors 64` |

### Current committed sizes (re-measure after regen)

Measured 2026-08-14 via `ls -la docs/media/` (decimal MB = bytes/1e6):

| Asset | Size | Bytes (`ls -la`) | Duration | Resolution / fps |
| --- | --- | --- | --- | --- |
| `web-demo.gif` | 7.4 MB | 7368472 | 15.8 s | 960×600 @ 9 fps, 128-color palette |
| `web-demo.mp4` | 1.1 MB | 1073187 | 15.8 s | 1024×640 h264 |
| `agent.gif` | 143 KB | 143360 | 14.4 s | 1080×500, paper/ink theme, 64 colors |

## Readability comes first, and it costs bytes

Readers reported the text was too small in every clip. The only real lever is
the **logical** size of what is being recorded — a wider viewport or a wider
terminal makes the glyphs smaller, because the README renders these at a fixed
width (900 px for the web GIF, 800 px for the terminal ones).

So the recordings are deliberately small:

- Web: viewport **1024×640**, not 1280×800. 1024 is the floor that still matches
  Tailwind's `lg:` breakpoint, below which the row's epic chip and trailing
  strip disappear and the UI stops looking like itself.
- Terminals (agent tape): **77 cols × 24 rows** (`Set Width 1080` / `Set FontSize 20`).

**Web GIF tradeoff:** the 0.12 walkthrough is four beats (search, an open
issue, documents, epics) on the paper UI. `export-video.sh` starts at
`fps=9 width=960 colors=128` and steps down to `8/960/96`, then `8/900/64`.
Note that fewer colors does **not** reliably shrink a GIF here: bayer dither
noise compresses worse, so a 96-colour pass can come out larger than a 128 one.
Motion is the real cost — an instant `scrollTop` jump costs a fraction of a
one-second smooth scroll. Cut a beat or a scroll before cutting resolution, and
point anyone who wants a light asset at the MP4.

**Terminal GIFs:** VHS's own encoder is generous, and `Set Framerate` does not
change the GIF's 25 fps frame table. `make media-agent` finishes the take with
`gifsicle -O3 --colors 64` when `gifsicle` is on `PATH`, which roughly halves
the file with no visible loss on a 64-colour paper theme.

## Regenerate everything

```bash
# Prerequisites (already on a typical gadak dev machine):
#   go, node ≥ 20, ffmpeg, vhs (brew install vhs), Playwright chromium

make media
```

Individual targets:

```bash
make media-web     # Playwright → webm → gif + mp4 (self-contained)
make media-agent   # VHS CLI tape against demo.db → gifsicle when present
make media-prep    # build gadak + seed tools/tapes/.tmp from demo.db
make brand         # logo, wordmarks, favicons, OG card
```

Outputs land in `docs/media/`. Commit them — the README references the paths
directly, and CI does not regenerate media.

## What each recording shows

### Web UI (`web-demo`)

~20 s of readable motion, viewport **1024×640** @ `deviceScaleFactor: 2`:

1. Boot — paper list, 가 mark, 534 issues, labels on the rows
2. Instant local search (`pagination`) with per-keystroke narrowing and `<mark>`
3. **NMA-123** open beside the list — title, priority, labels, reopen badge
4. Sidebar **Documents** — Viewed, then one page open with its breadcrumb
   and the issues it cites
5. Sidebar **Epics**: the same backlog re-sectioned by epic

The in-app Jira/Confluence pane is desktop-only (a native WKWebView). This
clip is the browser tab against `examples/demo.db`. The spaces tree was
dropped from this take — it did not earn its bytes next to the issue-detail
beat.

Re-shoot trap (2026-08-06): a leftover :7877 from an earlier e2e run is not
freshened, and "Sync delayed" prints into every frame. The demo config now
starts a fresh server (`reuseExistingServer: false`) and `GADAK_FRESHEN=1`
in `e2e/serve.sh` updates the snapshot clock. Stop a leftover with
`pkill -f 'e2e/.tmp/gadak'` if a recording still shows the orange chip.

Note the video size in `e2e/demo/playwright.config.ts` must equal the viewport.
Playwright does not upscale into a larger video frame — it pins the capture in
the top-left corner and pads the rest black.

Config lives in `e2e/demo/playwright.config.ts` (separate from the E2E suite).
The demo test is gated by `GADAK_MEDIA=1` so a plain
`playwright test --config e2e/playwright.config.ts` still runs only the original
10 specs (the demo file is discovered as **skipped**).

No caption overlays are injected into the DOM — the app code under `web/src/`
is never touched by this pipeline.

### Agent (`agent.gif`)

Three scenes, each cleared before the next so a TSV never scrolls off:

1. `gadak status` — 534 issues, watermark, no network
2. `gadak sql` — reopen counts, a field Jira does not expose and JQL cannot
   aggregate
3. `gadak search 'idempotency'` — one index, tab-separated hits

The tape is deterministic: VHS types the commands, the binary prints against
`examples/demo.db`. No model, no credential, no take-to-take drift. Paper/ink
theme so the clip sits next to the web GIF. Search titles wrap at 77 cols —
that is the readability tradeoff in the sizing note above, not a recording
bug.

`tools/tapes/prepare-agent.sh` still exists if you want a live Claude Code
take. The README clip does not use it.

## Font notes (Hangul alignment)

The demo seed keeps some Korean Jira status/type labels (`완료`, `진행 중`,
`버그`). Terminal columns need a **monospace CJK** font or Hangul cells
mis-width and the agent tape drifts.

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
- Do not re-record against a real Jira mirror. If a frame ever shows a real
  company, person, or domain, discard the asset and re-seed from `examples/`.
- `prepare-agent.sh` (optional live Claude take only) copies a credential into
  an isolated HOME. Run `--clean` afterwards; it is not on the README path.

## When to re-record

Re-run `make media` whenever any of these change in a way that is visible:

- `examples/demo.db` is regenerated (`tools/seed-demo` or similar)
- Web UI layout / copy that appears in the walkthrough
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
  prepare-agent.sh       # optional — isolated HOME for a live Claude take
  agent.tape             # VHS script (CLI against demo.db)
  .tmp/                  # disposable (gitignored)
docs/media/              # committed outputs
```
