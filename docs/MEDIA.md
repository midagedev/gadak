# Demo media assets

Regenerable GIF/MP4 clips for the public README hero, social posts, and docs.
All assets are produced from **scripts** against the scrubbed snapshot
`examples/demo.db` — no hand-recorded screen captures, no real company data.

## Assets

| File | Source | Use |
| --- | --- | --- |
| `docs/media/web-demo.gif` | Playwright walkthrough | README tour (folded `<details>`, not inline hero) |
| `docs/media/web-demo.mp4` | same recording, h264 | Twitter / LinkedIn / anywhere GIF is too heavy |
| `docs/media/search.gif` | Playwright `e2e/demo/search-demo.spec.ts` | README — ⌘K All search (ignores filters) |
| `docs/media/search.mp4` | same recording, h264 | Twitter / LinkedIn / anywhere GIF is too heavy |
| `docs/media/agent.gif` | Playwright split `e2e/demo/agent-demo.spec.ts` | README — two beats: `sql` piped into `views open --keys -`, then `views open --jql`; the paper list follows both |
| `docs/media/agent.mp4` | same recording, h264 | Twitter / LinkedIn / anywhere GIF is too heavy |
| `docs/media/mcp.gif` | VHS tape `tools/tapes/mcp.tape` | README — Claude Code registers `gadak mcp` and answers a question JQL cannot express |
| `docs/media/mcp.mp4` | same tape, second `Output` line | Twitter / LinkedIn / anywhere GIF is too heavy |

## Size budget

| Asset | Budget | Notes |
| --- | --- | --- |
| `web-demo.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | GitHub README inline limit; palette 2-pass |
| `web-demo.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `search.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | same README inline budget as the hero (900 px render) |
| `search.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `agent.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | same README inline budget as the hero |
| `agent.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `mcp.gif` | **≤ 3.5 MB** | VHS + `gifsicle -O3 --colors 64`; cut the idle tail if still over |
| `mcp.mp4` | soft ≤ 8 MB | VHS `Output` (h264) |

### Current committed sizes (re-measure after regen)

Measured 2026-08-14 via `ls -la docs/media/` (decimal MB = bytes/1e6):

| Asset | Size | Bytes (`ls -la`) | Duration | Resolution / fps |
| --- | --- | --- | --- | --- |
| `web-demo.gif` | 7.4 MB | 7368472 | 15.8 s | 960×600 @ 9 fps, 128-color palette |
| `web-demo.mp4` | 1.1 MB | 1073187 | 15.8 s | 1024×640 h264 |
| `search.gif` | 3.7 MB | 3707159 | 7.4 s | 960×600 @ 9 fps, 128-color palette |
| `search.mp4` | 0.53 MB | 527808 | 7.4 s | 1024×640 h264 |
| `agent.gif` | 5.8 MB | 5814162 | 18.7 s | 960×758 @ 9 fps, 128-color palette |
| `agent.mp4` | 0.69 MB | 687886 | 18.7 s | 1024×808 h264 |
| `mcp.gif` | 0.36 MB | 357824 | 24.9 s | 1080×620 @ 25 fps, 64 colors (gifsicle) |
| `mcp.mp4` | 0.28 MB | 276463 | 24.9 s | 1080×620 h264 |

## Readability comes first, and it costs bytes

Readers reported the text was too small in every clip. The only real lever is
the **logical** size of what is being recorded — a wider viewport or a wider
terminal makes the glyphs smaller, because the README renders these at a fixed
width (900 px for the web GIF, 800 px for the terminal ones).

So the recordings are deliberately small:

- Web: viewport **1024×640**, not 1280×800. 1024 is the floor that still matches
  Tailwind's `lg:` breakpoint, below which the row's epic chip and trailing
  strip disappear and the UI stops looking like itself.
- Agent clip: **1024×808** — a 168 px paper terminal stacked on the same
  1024×640 app frame as the hero. Do not scale the iframe; the list has to
  match the hero's glyph size.

**Web GIF tradeoff:** the 0.12 walkthrough is four beats (search, an open
issue, documents, epics) on the paper UI. `export-video.sh` starts at
`fps=9 width=960 colors=128` and steps down to `8/960/96`, then `8/900/64`.
Note that fewer colors does **not** reliably shrink a GIF here: bayer dither
noise compresses worse, so a 96-colour pass can come out larger than a 128 one.
Motion is the real cost — an instant `scrollTop` jump costs a fraction of a
one-second smooth scroll. Cut a beat or a scroll before cutting resolution, and
point anyone who wants a light asset at the MP4.

**Terminal GIFs:** VHS's own encoder is generous, and `Set Framerate` does not
change the GIF's 25 fps frame table. `make media-mcp` finishes the take with
`gifsicle -O3 --colors 64` when `gifsicle` is on `PATH`, which roughly halves
the file with no visible loss on a 64-colour paper theme.

## Regenerate everything

```bash
# Prerequisites (already on a typical gadak dev machine):
#   go, node ≥ 20, ffmpeg, Playwright chromium
#   vhs + a Claude Code login — only for `make media-mcp`, which `make media`
#   deliberately excludes (a live take spends your own model quota)

make media
```

Individual targets:

```bash
make media-web     # Playwright → webm → gif + mp4 (self-contained)
make media-search  # Playwright: ⌘K All search → search.gif + search.mp4
make media-agent   # Playwright split: sql \| views open --keys + paper list → gif + mp4
make media-mcp     # VHS: claude mcp add + live Claude Code session on the mirror
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

### Unified search (`search.gif`)

~7.5 s, same viewport **1024×640** @ `deviceScaleFactor: 2` as the hero
(README render width 900 px):

1. Boot on `/#/?pj=NMS` — paper list, one chip **Project: NMS**, 157 issues,
   toolbar **Search ⌘K** in frame
2. ⌘K opens the palette (`Searches every issue and document.`)
3. Type `work` (local Documents + Issues sections highlight title hits), then
   `around` — the usearch.spec.ts comment-only token. Local rows empty; after
   the 250 ms debounce (`UNIFIED_DEBOUNCE_MS` in `web/src/lib/unified-search.ts`)
   **ALL SEARCH** fills with issue rows and **Comment match**
   snippets
4. Enter on the first unified hit (**NMA-36**, not in the NMS chip) opens
   the issue; the last ~1.8 s hold the detail (the matching comment is on
   screen) beside the still-active NMS chip

`consequences` (usearch's page-body token) is not in this take: the last beat
has to be an issue detail, and that token matches pages only.

Config lives in `e2e/demo/search.config.ts`. Same `GADAK_MEDIA=1` gate as the
other demo specs.

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

One take, two panes, two beats, ~19 s, viewport **1024×808**:

1. A paper terminal types
   `gadak sql "select key from issues where reopen_count>0 order by key limit 5" \`
   `| tail -n +2 | gadak views open --keys -`
   (`tail -n +2` drops the `gadak sql` header row so it does not become a
   fake key — `skills/gadak/SKILL.md`, `docs/RECIPES.md`)
2. The real app (iframe, same 1024×640 frame as the hero) sits underneath
   on the default **All open** view (368 issues)
3. On Enter the test runs that pipe against the serve fixture (`--no-open`);
   the iframe polls `GET ui-focus/` and the list becomes those five keys
   (**NMA-1, NMA-100, NMA-102, NMA-11, NMA-118**), with a **5 keys** chip
   and **5 issues**. Sort reads **Given order**. Default `group_by` is still
   status category (`jql.Hash` with an empty Display does not emit `g=none`),
   so the five rows section into In progress / New
4. The terminal clears and types
   `gadak views open --jql 'project = NMA AND priority = High AND resolution is EMPTY'`.
   The hash is `pj=NMA&pr=High&sc=new%2Cinprogress` and the list lands on four
   chips — **Project: NMA**, **Priority: High**, and the two status categories
   that `resolution is EMPTY` becomes (decision 0007) — at **22 issues**

Beat 4 exists because the two beats are different promises: the pipe says any
answer you can compute is a view, the JQL says you do not have to leave the
query language you already know to get one. A reader who saw only the pipe
would conclude gadak requires SQL.

The command is typed in the wrapper, not in the app — `web/src/` stays
untouched. The on-screen output is only the `hash\tks=…` line; the `web\t`
URL `views open` also prints is dropped so a loopback address does not land
in the frame. No live model, no credential, no take-to-take drift. `--no-open`
keeps the take from launching Gadak.app or a second tab.

`tools/tapes/agent.tape` is the older CLI-only SQL take (status / reopen /
search). It is not what the README embeds.

### MCP (`mcp.gif`)

Two scenes, both real, VHS tape `tools/tapes/mcp.tape`, **1080×620**,
font 20 (same 77×24 / 800 px README convention as the other terminal tapes):

1. `claude mcp add gadak -- gadak mcp` — the one line that teaches an MCP
   client the mirror exists (`docs/AGENT_SETUP.md`).
2. A live Claude Code session answering *"Search our Jira and wiki for
   idempotency. Answer in 3 lines."* — the question Jira cannot answer at
   all, because the wiki is a second search. The model calls `gadak_search`
   (and only the MCP tools `prepare-agent.sh` pre-allows). Nothing is
   scripted, so takes differ; re-run until one reads well.

   **Not a `reopen_count` question, deliberately.** That field already
   carries the README quick start and the agent clip; a third use made the
   demo read as if it were the only derived column gadak has.

   Take notes worth keeping: pin the answer length (*"Answer in 3 lines"* —
   "Brief." is not binding, and a long answer scrolls the question out of
   frame). Extended thinking is off and `Task`/`Glob`/`Grep` are denied in
   `prepare-agent.sh`; without those denies a take spawned an Explore
   subagent and searched the *codebase*. Haiku was tried for speed and
   rejected: it called `gadak_search` with the wrong argument name and then
   reported "zero results" instead of the error.

Theme is **gadak-paper** (same JSON as `tools/tapes/agent.tape`). Claude
Code's own UI is ink-on-paper on that background; the only token that
vanishes is the collapsed tool-call *count* ("Called gadak  times" —
the number is a bright color). The question, the "Calling gadak…" line,
and the answer stay readable, so this take does not fall back to a dark
theme.

Everything visible comes from `examples/demo.db`. On that snapshot
`idempotency` matches **7 issues and 4 wiki pages** through the one FTS
index, which is why the answer can put **NMA-142** and the Confluence page
*API Platform Brief — Idempotency* (PROD space) in the same sentence — the
join Jira and Confluence never make for you.

`make media-mcp` requires `vhs` (fails with `media-mcp: vhs required
(brew install vhs)` when missing), runs `prepare.sh` then
`prepare-agent.sh`, records the tape, and runs `gifsicle -O3 --colors 64`
when `gifsicle` is on `PATH`. Timing is not deterministic: if the GIF is
still over 3.5 MB, cut the idle tail after the answer lands (VHS GIF
frames are 25 fps regardless of `Set Framerate`):

```bash
gifsicle --info docs/media/mcp.gif | head -2          # total frame count
gifsicle -O3 --colors 64 docs/media/mcp.gif '#0-N' -o docs/media/mcp.gif
```

#### Re-shoot procedure (credential copy)

```bash
# 1–2. seed the throwaway mirror + isolated HOME, write pin/strip helpers
#     into that HOME, record, gifsicle. This is the supported path — raw
#     `vhs tools/tapes/mcp.tape` needs those helpers already in
#     /private/tmp/gadak-demo/ (make writes them; VHS cannot parse
#     escaped $/" inside Type).
make media-mcp

# 3. inspect frames; re-run make media-mcp until a take reads
ffmpeg -y -ss 00:00:02 -i docs/media/mcp.gif -frames:v 1 /tmp/mcp-frame.png

# 4. LAST — remove the isolated HOME, including the credential copy
bash tools/tapes/prepare-agent.sh --clean
```

`prepare-agent.sh` refuses if this machine has no Claude Code login
(`~/.claude/.credentials.json`). Do not skip step 4: the isolated HOME
at `/private/tmp/gadak-demo` holds a 0600 copy of that file.

### Layout of the pipeline (agent)

```text
e2e/demo/
  agent.config.ts      # 1024×808, video on, fresh :7877
  agent-demo.spec.ts   # GADAK_MEDIA=1 gated split
  export-agent.sh      # webm → gif (palette 2-pass) + mp4
  search.config.ts     # 1024×640, separate output dir
  search-demo.spec.ts  # GADAK_MEDIA=1 gated ⌘K take
  export-search.sh     # webm → search.gif + search.mp4
```

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
- `prepare-agent.sh` (required for `make media-mcp`) copies a credential into
  an isolated HOME at `/private/tmp/gadak-demo`. Run `--clean` afterwards —
  it is the last step of every MCP re-shoot, including `make media`.

## When to re-record

Re-run `make media` whenever any of these change in a way that is visible:

- `examples/demo.db` is regenerated (`tools/seed-demo` or similar)
- Web UI layout / copy that appears in the walkthrough
- CLI output shape for `search` / `sql` / `issue` / `status` / `views`
- MCP tool names or the `claude mcp add gadak -- gadak mcp` registration line

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
  search.config.ts       # 1024×640, separate output dir
  search-demo.spec.ts    # GADAK_MEDIA=1 gated ⌘K All-search take
  export-search.sh       # webm → search.gif + search.mp4
  agent.config.ts        # 1024×808 split, separate output dir
  agent-demo.spec.ts     # GADAK_MEDIA=1 gated agent-focus take
  export-agent.sh        # webm → agent.gif + agent.mp4
tools/tapes/
  prepare.sh             # build binary, seed GADAK_HOME from demo.db
  prepare-agent.sh       # isolated HOME + auth for the live Claude MCP take
  agent.tape             # optional CLI-only VHS (not the README clip)
  mcp.tape               # VHS: claude mcp add + live Claude Code session
  .tmp/                  # disposable (gitignored)
docs/media/              # committed outputs
```
