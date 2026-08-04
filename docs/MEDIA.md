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
| `docs/media/agent.gif` | VHS tape `tools/tapes/agent.tape` | README / docs — agent SQL story |

## Size budget

| Asset | Budget | Notes |
| --- | --- | --- |
| `web-demo.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | GitHub README inline limit; palette 2-pass |
| `web-demo.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `tui.gif` / `agent.gif` | soft ≤ 5 MB each | VHS framerate 12, modest terminal size |

### Current committed sizes (re-measure after regen)

| Asset | Size | Duration | Resolution / fps |
| --- | --- | --- | --- |
| `web-demo.gif` | ~5.7 MB | ~33 s | 800×500 @ 10 fps, 128-color palette |
| `web-demo.mp4` | ~1.8 MB | ~33 s | 1280×800 h264 |
| `tui.gif` | ~1.3 MB | ~13 s | 1100×620 |
| `agent.gif` | ~402 KB | ~19 s | 1000×560 |

**Web GIF tradeoff:** a full UI (sidebar + list + detail) at 33 s does not fit the
prefer-≤5 MB target at readable quality. Defaults in `export-video.sh` are
`fps=10`, `width=800`, `max_colors=128` (palette 2-pass). The script steps down
to `720@10/96` then `640@8/64` if still over 8 MB. Prefer shortening
`e2e/demo/web-demo.spec.ts` beats before dropping further. For social posts use
the MP4 — much better quality per byte.

## Regenerate everything

```bash
# Prerequisites (already on a typical scry dev machine):
#   go, node ≥ 20, ffmpeg, vhs (brew install vhs), Playwright chromium

make media
```

Individual targets:

```bash
make media-web     # Playwright → webm → gif + mp4
make media-tui     # VHS TUI navigator
make media-agent   # VHS CLI / agent commands
make media-prep    # build scry + seed tools/tapes/.tmp from demo.db
```

Outputs land in `docs/media/`. Commit them — the README references the paths
directly, and CI does not regenerate media.

## What each recording shows

### Web UI (`web-demo`)

~20–30 s of readable motion, viewport **1280×800** @ `deviceScaleFactor: 2`:

1. Boot — 519-issue list
2. Instant local search with per-keystroke narrowing and `<mark>` highlights
3. Filter chips (category + assignee) and changing counts
4. ⌘K command palette → issue key → detail panel
5. Scroll comments / history in the detail panel
6. Built-in sidebar view (Stale)

Config lives in `e2e/demo/playwright.config.ts` (separate from the E2E suite).
The demo test is gated by `SCRY_MEDIA=1` so a plain
`playwright test --config e2e/playwright.config.ts` still runs only the original
10 specs (the demo file is discovered as **skipped**).

No caption overlays are injected into the DOM — the app code under `web/src/`
is never touched by this pipeline.

### TUI (`tui.gif`)

List → `/` filter typing → tab `2` (open) / `3` (in progress) → Enter detail →
Esc → `q`.

### Agent / CLI (`agent.gif`)

```text
scry search "pagination" --limit 5
scry sql "select project_key, count(*) … group by project_key …"
scry issue NMB-110
scry status --json
```

The point of the clip: an agent answers with **one SQL shot** against the local
mirror, not by scraping a browser.

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
  web-demo.spec.ts       # SCRY_MEDIA=1 gated walkthrough
  export-video.sh        # webm → gif (palette 2-pass) + mp4
tools/tapes/
  prepare.sh             # build binary, seed SCRY_HOME from demo.db
  tui.tape / agent.tape  # VHS scripts
  .tmp/                  # disposable (gitignored)
docs/media/              # committed outputs
```
