# Demo media assets

Regenerable GIF/MP4 clips for the public README hero, social posts, and docs.
All assets are produced from **scripts** against the scrubbed snapshot
`examples/demo.db` — no hand-recorded screen captures, no real company data.

One asset bends the first half of that rule without touching the second:
`raycast.*` records a native Raycast overlay handing off to the installed
app through a `gadak://` link, which no headless browser can drive. The
setup *and* the take are still a script (`tools/record-raycast.sh` — it
reseeds the demo profile from the snapshot first, captures only the app
window's rectangle, and scrubs the signed-in account line in the encode),
but the script runs on a live screen, so review the frames before
committing a regen.

**A capture workspace is frozen.** Reseeding the mirror is not enough on
its own — the profile's `config.json` is where a real site and token survive
between takes, and the app syncs on open. That is how 71 real rows once
landed on top of the scrubbed ones under the same `external_id`, with the
fictional author names replaced by real ones (GDK-181, caught in review
before the shot). `"frozen": true` in that config (`gadak config set frozen
true`) makes the workspace refuse every request to the origin — pulls and
writes alike (GDK-507); `tools/record-raycast.sh` sets it before it launches
anything. Do the same for any capture profile you build by hand — `gadak
status` and `gadak doctor` both say whether it is on. (The VHS fixture under
`tools/tapes/` needs no latch: `prepare.sh` rewrites its config from scratch
each run with a fake credential.)

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
| `docs/media/raycast.gif` | scripted live take `tools/record-raycast.sh` | README — Raycast searches the mirror per keystroke, Enter opens the hit via `gadak://` |
| `docs/media/raycast.mp4` | same take, h264 | Twitter / LinkedIn / anywhere GIF is too heavy |
| `docs/media/tokens.gif` | Playwright split `e2e/demo/tokens-demo.spec.ts` | README — CLI `config set ui.tokens` / `ui.dataColors` retints an open tab; locked `bg-base` is refused |
| `docs/media/tokens.mp4` | same recording, h264 | Twitter / LinkedIn / anywhere GIF is too heavy |
| `docs/media/dashboards.gif` | Playwright split `e2e/demo/dashboards-demo.spec.ts` | README — CLI `dashboards save` / `open` renders the triage wall; a second save swaps the open frame |
| `docs/media/dashboards.mp4` | same recording, h264 | Twitter / LinkedIn / anywhere GIF is too heavy |
| `docs/media/tokens-vertical.mp4` | Playwright stacked `e2e/demo/tokens-demo.spec.ts` via `tokens-vertical.config.ts` | social/vertical, 4:5 for X feeds — README uses the landscape cut |
| `docs/media/dashboards-vertical.mp4` | Playwright stacked `e2e/demo/dashboards-demo.spec.ts` via `dashboards-vertical.config.ts` | social/vertical, 4:5 for X feeds — README uses the landscape cut |
| `docs/media/claude-drive.gif` | VHS `tools/tapes/claude-drive.tape` + Playwright serve tab | README — live Claude Code session (skill) retints an open tab and opens a chart dashboard |
| `docs/media/claude-drive.mp4` | same take, h264 | Twitter / LinkedIn / anywhere GIF is too heavy |
| `docs/media/claude-drive-vertical.mp4` | same tape + stacked chrome (`record-claude-drive.sh vertical`) | social/vertical, 4:5 — README uses the landscape cut |
| `docs/media/claude-dashboards-vertical.mp4` | VHS `tools/tapes/claude-dashboards.tape` + the same serve tab (`record-claude-drive.sh vertical claude-dashboards`) | social/vertical, 4:5 — the dashboards half of the flagship, ending on a key clicked off the wall that opens the issue in the app (the `open` verb, GDK-854) |
| `docs/media/claude-dashboards-vertical.gif` (+`-poster.png`) | 430-wide reduction of that mp4 | README — GitHub strips `<video>` from markdown (measured 2026-08-25 via `gh api /markdown`), so the README pair ships as GIF; the poster is the landing's still |
| `docs/media/claude-tokens-vertical.mp4` | VHS `tools/tapes/claude-tokens.tape` + the same serve tab (`record-claude-drive.sh vertical claude-tokens`) | social/vertical, 4:5 — the team-look half: colours plus the dimension axes, including a token saved with a warning the agent then acts on (GDK-858) |
| `docs/media/claude-tokens-vertical.gif` (+`-poster.png`) | 430-wide reduction of that mp4 | README — same reason as the dashboards GIF |
| `docs/media/scale.mp4` (+`scale.gif`, `scale-poster.png`) | Playwright `e2e/demo/scale-demo.spec.ts` + post-process camera work `e2e/demo/export-scale.sh` (`make media-scale` — deliberately outside the `make media` aggregate: the committed artifacts are what the site ships) | landing flagship — record-time counts focus over a 20k-issue snapshot |
| `docs/media/hero.mp4` (+`hero-poster.png`) | two-camera LIVE shoot, one command: `e2e/demo/record-hero.sh` (the desk take `record-hero-desk.sh`, with the phone take `record-hero-phone.sh` running inside its away-wait), cut by `e2e/demo/cut-hero.sh`. Not in `make media` and not reproducible from a checkout alone — it needs a live Claude Code login AND a booted iOS simulator with a dev build of the phone app | the 0.19 hero — one serve, one terminal session: the desk hands work to an agent and walks away, a phone closes an issue while the chair is empty, and the desk comes back to the scrollback and the board |
| `docs/media/roundtrip.mp4` (+`roundtrip-poster.png`) | Playwright `e2e/demo/roundtrip.spec.ts` via `e2e/demo/record-roundtrip.sh --live --dark`, cut by `e2e/demo/cut-roundtrip.sh`. Like the hero, not in `make media` and not reproducible from a checkout alone — the four shells run a live Claude Code login against `e2e/demo/shop-fixture/`; nothing in the terminals is scripted, so takes vary and the rig retries until the beat contract holds | the 0.19 release cut — four shells bound to four issues investigate on their own (tabs named by issue key in the bottom dock), and clicking a card's session brings that shell back, findings standing, then another |
| `docs/media/groupby.gif` / `groupby.mp4` (+`groupby-poster.png`) | Playwright `e2e/demo/groupby-demo.spec.ts` via `e2e/demo/export-groupby.sh` | group-by exhibit motion cut — the landing exhibit itself ships the still below |
| `docs/media/groupby-still.png` | `e2e/demo/site-stills.mjs` | landing group-by exhibit (2x still; see the landing policy table) |
| `docs/media/history.gif` / `history.mp4` (+`history-poster.png`) | Playwright `e2e/demo/history-demo.spec.ts` via `e2e/demo/export-history.sh` | history exhibit motion cut — the landing exhibit itself ships the still below |
| `docs/media/history-still.png` | `e2e/demo/site-stills.mjs` | landing history exhibit (2x still; see the landing policy table) |

## Landing media policy (gadak.dev, GDK-751)

The landing keeps a **video only where motion is the claim** (typing-speed
search, a command changing the visible view); every claim that reads from a
single frame ships a **core-region crop still** instead of a full-screen clip.
Kept videos are framed to the action region, not the whole window — the
landing's exhibit column is narrow, and full-frame 1280 px recordings render
glyphs unreadably small there.

Current state:

| Landing slot | Asset | Policy |
| --- | --- | --- |
| flagship (hero) | `scale.mp4` (+`scale-poster.png`) | video kept, with **post-process camera work** (`export-scale.sh`): smoothstep push-in to the palette for the typing beats, out for the regroup, in to the counts band for the chip narrowing, back to full frame at the loop point. Source pacing untouched — only the crop moves |
| search exhibit | `search.mp4` (+`search-poster.png`) | video kept, mp4 cropped to the palette+detail region (`export-search.sh`); the README `search.gif` stays full-frame |
| group-by exhibit | `groupby-still.png` | still — assignee filter submenu with live counts over the epic-grouped list (2x, 1240×1110) |
| history exhibit | `history-still.png` | still — NMB-139 header badges + bot comment + changelog with the Reopened marker (2x, 880×1540) |
| agent exhibit | `agent.mp4` | video kept as recorded — the command→view causality is the claim |
| agent proof | `mcp.mp4` (+`mcp-poster.png`) | video — the claim is a conversation flow, so the exhibit plays the tape (was `mcp-still.png` until the user call of 2026-08-24) |
| skill drive | `claude-drive-vertical.mp4` (+`claude-drive-vertical-poster.png`) | video — a command changing the visible view (retint + dashboard landing in an open tab) is the motion rule's own example; added for v0.17.2, vertical 4:5 cut capped at 540px in the column (user calls 2026-08-25) |

Regenerate the two app stills against the standard e2e fixture (the history
still needs serve.sh's NMB-139 enrichment):

```bash
GADAK_FRESHEN=1 bash e2e/serve.sh &        # :7877 fixture
node e2e/demo/site-stills.mjs              # → docs/media/{groupby,history}-still.png
```

The MCP still is a frame of the committed clip (re-pick the timestamp after a
re-record so the answer is fully printed; the crop drops the personal
usage-limit status line):

```bash
ffmpeg -y -ss 23.5 -i docs/media/mcp.mp4 -frames:v 1 -vf "crop=1080:450:0:0" \
  docs/media/mcp-still.png
```

`site/public/media` is a symlink to `docs/media` — stills land on the site by
existing there. The landing references them via `MediaSlot still=…` with a
`displayWidth` equal to the crop's CSS-pixel width (half the PNG width).

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
| `raycast.gif` | **≤ 3.5 MB** | dark overlay + small motion area compress well; 960 px @ 10 fps lands ~1 MB |
| `raycast.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `tokens.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | split 1744×672 scaled to 1280 @ 9 fps, 128-color palette |
| `tokens.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `dashboards.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | same split and palette ladder as tokens |
| `dashboards.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `tokens-vertical.mp4` | soft ≤ 8 MB | 1080×1350 h264 `yuv420p` + `faststart`; no GIF (1080-wide GIF would miss the 8 MiB budget) |
| `dashboards-vertical.mp4` | soft ≤ 8 MB | same 4:5 stack as tokens-vertical; no GIF |
| `claude-drive.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | split 1880×720 scaled to 1280 @ 9 fps, then `gifsicle -O3 --colors 64` |
| `claude-drive.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `claude-drive-vertical.mp4` | soft ≤ 8 MB | 1080×1350 h264 `yuv420p` + `faststart`; no GIF |
| `claude-dashboards-vertical.mp4` | soft ≤ 8 MB | same 4:5 stack as claude-drive-vertical; no GIF |
| `claude-tokens-vertical.mp4` | soft ≤ 8 MB | same 4:5 stack as claude-drive-vertical |
| `claude-{dashboards,tokens}-vertical.gif` | soft ≤ 4 MB each | 430-wide @ 9 fps, `palettegen=max_colors=64:stats_mode=diff` then `gifsicle -O3 --colors 64`. 430 rather than 540: two of them sit side by side in one README row, and the pair has to fit the 900 px column the other exhibits use |
| `scale.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | same README inline budget as the hero |
| `scale.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart`; camera work is a post-process crop, source pacing untouched |
| `{groupby,history}.gif` | **≤ 8 MB** (prefer ≤ 5 MB) | same palette ladder as tokens |
| `{groupby,history}.mp4` | soft ≤ 8 MB | h264 `yuv420p` + `faststart` |
| `{groupby,history}-still.png` | soft ≤ 1 MB | 2x app stills; regenerate via `node e2e/demo/site-stills.mjs` against the standard e2e fixture |

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
| `raycast.gif` | 0.98 MB | 982043 | 13.2 s | 960×579 @ 10 fps, 128-color palette (measured 2026-08-17) |
| `raycast.mp4` | 0.26 MB | 255880 | 13.2 s | 1088×656 h264 (measured 2026-08-17) |
| `tokens.gif` | 3.2 MB | 3185180 | 20.6 s | 1280×493 @ 9 fps, 128-color palette (measured 2026-08-24) |
| `tokens.mp4` | 0.76 MB | 762298 | 20.5 s | 1744×672 h264 (measured 2026-08-24) |
| `dashboards.gif` | 3.1 MB | 3142466 | 28.4 s | 1280×493 @ 9 fps, 128-color palette (measured 2026-08-24) |
| `dashboards.mp4` | 0.80 MB | 803739 | 28.4 s | 1744×672 h264 (measured 2026-08-24) |
| `tokens-vertical.mp4` | 1.1 MB | 1099309 | 20.7 s | 1080×1350 h264 (measured 2026-08-24) |
| `dashboards-vertical.mp4` | 2.1 MB | 2122855 | 41.0 s | 1080×1350 h264 (measured 2026-08-24) |
| `claude-drive.gif` | 4.0 MB | 3958051 | 26.6 s | 1280×490 @ 9 fps, 64 colors (gifsicle; measured 2026-08-24) |
| `claude-drive.mp4` | 2.0 MB | 1999568 | 26.6 s | 1880×720 h264 25 fps (measured 2026-08-24) |
| `claude-drive-vertical.mp4` | 2.1 MB | 2076793 | 30.9 s | 1080×1350 h264 (measured 2026-08-24) |
| `claude-dashboards-vertical.mp4` | 1.7 MB | 1659163 | 25.6 s | 1080×1350 h264 (measured 2026-08-25) |
| `claude-tokens-vertical.mp4` | 1.2 MB | 1166774 | 18.2 s | 1080×1350 h264 (measured 2026-08-25) |
| `claude-dashboards-vertical.gif` | 1.3 MB | 1320334 | 25.6 s | 430×538 @ 9 fps, 64 colors (gifsicle; measured 2026-08-25) |
| `claude-tokens-vertical.gif` | 0.97 MB | 969087 | 18.2 s | 430×538 @ 9 fps, 64 colors (gifsicle; measured 2026-08-25) |
| `hero.mp4` | 1.7 MB | 1723424 | 25.9 s | 1920×1080 h264 30 fps (measured 2026-08-30) |
| `roundtrip.mp4` | 3.1 MB | 3292613 | 21.2 s | 1920×1296 h264 30 fps (measured 2026-08-31) |

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
#   vhs + a Claude Code login — only for `make media-mcp` and
#   `bash e2e/demo/record-claude-drive.sh`, which `make media` deliberately
#   excludes (a live take spends your own model quota)

make media
```

Individual targets:

```bash
make media-web     # Playwright → webm → gif + mp4 (self-contained)
make media-search  # Playwright: ⌘K All search → search.gif + search.mp4
make media-agent   # Playwright split: sql \| views open --keys + paper list → gif + mp4
make media-mcp     # VHS: claude mcp add + live Claude Code session on the mirror
make media-prep    # build gadak + seed tools/tapes/.tmp from demo.db
bash e2e/demo/record-promo.sh  # tokens + dashboards split (not in `make media`)
bash e2e/demo/record-vertical.sh  # tokens + dashboards 1080×1350 (not in `make media`; README stays landscape)
bash e2e/demo/record-claude-drive.sh  # flagship: live Claude Code × serve tab (not in `make media`; needs vhs + Claude login)
bash e2e/demo/record-claude-drive.sh vertical  # same take, 1080×1350 (mp4 only)
bash e2e/demo/record-hero.sh  # 0.19 hero: desk + phone interleaved, one serve (not in make media; needs a Claude login AND a booted iOS simulator)
tools/record-raycast.sh  # scripted LIVE take (not in make media): Raycast → gadak:// — needs Raycast + installed app
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
   `gadak sql --no-header "select key from issues where status_category='inprogress' order by status_changed_at asc limit 5" \`
   `| gadak views open --keys -`
   ("stuck the longest in progress" — universal, where a reopen workflow is
   team-specific; `--no-header` keeps the header row from becoming a fake key)
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

### Tokens (`tokens.gif`)

~21 s, viewport **1744×672** (720 px paper terminal | 1024×640 app iframe),
`deviceScaleFactor: 2`. Sibling of the agent split, left/right instead of
stacked. Regen: `bash e2e/demo/record-promo.sh`. The capture home is frozen
(`gadak config set frozen true`) before the first frame.

1. Boot on `/#/?lb=quick-win&sc=new%2Cinprogress` — list already painted,
   **Labels: quick-win** chip in the filter bar
2. Terminal types `gadak config set ui.tokens '{"colors":{"accent":"#7a4bd0"}}'`
   (the value in `docs/CONFIGURATION.md`). On Enter the test runs that string;
   `--color-accent` becomes `#7a4bd0` without a reload (**+ New issue** goes
   purple)
3. `gadak config set ui.dataColors '{"label":{"quick-win":"#2e7d32"}}'` —
   row label chips pick up the green tint
4. `gadak config set ui.tokens '{"colors":{"bg-base":"#000000"}}'` is refused
   (`locked` in the CLI error); the open tab keeps the purple accent

Commands are real (`spawnSync` of the typed string). Loopback `web\t` lines
are dropped, same as the agent clip.

### Dashboards (`dashboards.gif`)

~28 s, same split viewport as tokens. SQL keys `status_category`, never a
localized status name.

1. Boot on the default paper list
2. Terminal types `gadak dashboards save triage --html examples/dashboards/triage.html`
   with `--datasource` `by_status` and `monthly_opened` (the two queries in
   `e2e/demo/dashboards-demo.spec.ts`). Output is `saved\t<id>\ttriage`
3. `gadak dashboards open triage` writes `hash\tdash=<id>`; the iframe opens
   the wall (count cards, uPlot monthly line, sidebar **DASHBOARDS / triage**)
4. A second save with `e2e/.tmp/triage-v2.html` (`<h1>Triage · v2</h1>`)
   replaces the open frame (`data-render-gen` moves; the h1 reads **Triage · v2**)
5. **Refresh** once, then a four-click burst (the host's 2 s throttle)

`tools/tapes/prepare-promo.sh` latches `frozen true` on a capture home (the
demo specs also call `gadak config set frozen true` themselves).

### Vertical social (`tokens-vertical.mp4`, `dashboards-vertical.mp4`)

Same bits as the landscape tokens / dashboards takes, re-shot stacked at
**1080×1350** (4:5 — X feed max without crop). Mac bar 48 + terminal band 340
+ web 962. Regen: `bash e2e/demo/record-vertical.sh` (port 7890). **mp4 only**
— no GIF. README keeps the landscape cut.

The nested dashboard iframe is narrower than 960 px beside the app sidebar, so
the vertical dashboards take injects capture-only CSS (`applyVerticalDashboardLayout`
in `e2e/demo/promo-split.ts`) to keep four count cards in a row and pin the
uPlot chart in frame. `examples/dashboards/triage.html` is not edited.

### Claude drive (`claude-drive.gif`)

26.6 s, flagship landscape split (**1880×720**, 720 px VHS terminal |
1160×688 serve tab; tokens/dashboards stay 1744×672). Paper chrome from
`promo-split.ts` (`FLAGSHIP_L_*`). Vertical sibling
`claude-drive-vertical.mp4` is **1080×1350** (bar 48 + Claude TUI band 520 +
web 782), 30.9 s. Regen: `bash e2e/demo/record-claude-drive.sh` (landscape) or
`bash e2e/demo/record-claude-drive.sh vertical`. Not in `make media` — it
needs `vhs` and a Claude Code login, same reason as `make media-mcp`. The
capture home is a throwaway directory (`/private/tmp/gadak-claude-drive/gadak-home`),
seeded from `examples/demo.db`, with `gadak config set frozen true`
(`tools/tapes/prepare-claude-drive.sh`). `gadak skill install` writes the
embedded skill into that isolated `HOME` (not `~/.claude`). Run
`bash tools/tapes/prepare-claude-drive.sh --clean` afterwards — the isolated
HOME holds a 0600 copy of this machine's credentials (from
`~/.claude/.credentials.json` when present, otherwise the macOS keychain
service `Claude Code-credentials`).

Nothing on the left is scripted output: VHS types `claude`, then one English
prompt, and Sleeps while the model works. Commands and the HTML it writes are
the model's. The orchestrator re-runs up to 3 takes until the result contract
holds (two colour changes reflected in the open tab, plus a saved dashboard
whose HTML contains uPlot/canvas and whose frame actually opened). After the
wall opens, Playwright sends a mouse-wheel into the dashboard iframe until a
chart canvas is in view (camera work, not an edit of Claude's HTML).

Left/right start epochs are logged and stacked (`e2e/demo/export-claude-drive.sh`).
Static holds ≥ 0.6 s are sped to 0.5 s (`e2e/demo/static-cut.py`). The head
does not blanket-protect 0–18 s: `$ claude` (~1.2 s) and the measured
prompt-typing window stay 1×, as do colour changes and `dashboard_open`
± 1.2 s (`e2e/demo/edit-claude-drive.py`); the TUI boot freeze between
them uses the same static-run rule.
A paper-coloured drawbox covers the Claude TUI footer (`manual mode on`) so a
parent-session harness leak does not land in the public frame. Claude's own
usage-limit banner is left in the conversation.

#### Split cuts (`claude-dashboards-vertical.mp4`, `claude-tokens-vertical.mp4`)

The flagship asks for the whole job in one take; the two split clips ask for
half each, so a social-length cut can hold on one thing. Same rig, same
geometry, one argument:

```bash
bash e2e/demo/record-claude-drive.sh vertical claude-dashboards
bash e2e/demo/record-claude-drive.sh vertical claude-tokens
```

Each clip carries its own result contract, because "a take finished" is not
"a take is shippable" — measured 2026-08-25, a dashboards take passed the old
chart-exists contract on a wall whose cards all read 0. The dashboards
contract now requires the `dashboard_data` beat (the wall painted real
numbers) **and** `dashboard_link_nav` (a key on the wall was clicked and the
app opened that issue); the tokens contract requires a colour change plus a
dimension override in the reopened tab.

The recorder's serve deliberately listens inside `serveProbePorts()`' sweep
(7796 landscape, 7795 vertical). On an out-of-range port `dashboards open`
finds no serve and the take records the agent hunting the port by hand — a
rig artifact standing in for a product failure. The discovery gap itself is
real and filed as GDK-859; the port choice only keeps it out of the frame.

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
  tokens.config.ts       # 1744×672 L/R split, port 7888
  tokens-demo.spec.ts    # GADAK_MEDIA=1 gated tokens take
  export-tokens.sh       # webm → tokens.gif + tokens.mp4
  dashboards.config.ts   # same split as tokens
  dashboards-demo.spec.ts
  export-dashboards.sh
  record-promo.sh        # unattended tokens + dashboards regen
  tokens-vertical.config.ts  # 1080×1350 stacked, port 7890
  dashboards-vertical.config.ts
  record-vertical.sh     # unattended vertical regen (mp4 only)
  export-vertical.sh     # webm → *-vertical.mp4, no GIF
  promo-split.ts         # paper terminal + iframe chrome (landscape + vertical)
  claude-drive.config.ts # 1880×720 flagship landscape, port 7796 (vertical 1080×1350 on 7795)
  claude-drive-web.spec.ts
  record-claude-drive.sh # VHS + Playwright, up to 3 live takes
  export-claude-drive.sh # epoch offset stack + palette gif
  edit-claude-drive.py   # protect timestamps + tail trim for static-cut
  static-cut.py          # clip-agnostic: compress static runs ≥0.6s to 0.5s
tools/tapes/
  prepare.sh             # build binary, seed GADAK_HOME from demo.db
  prepare-promo.sh       # `gadak config set frozen true` on a capture home
  prepare-agent.sh       # isolated HOME + auth for the live Claude MCP take
  prepare-claude-drive.sh # frozen demo home + skill install + Claude auth copy
  agent.tape             # optional CLI-only VHS (not the README clip)
  mcp.tape               # VHS: claude mcp add + live Claude Code session
  claude-drive.tape      # VHS: live Claude Code drives ui.* + dashboards
  .tmp/                  # disposable (gitignored)
docs/media/              # committed outputs
```
