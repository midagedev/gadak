# Fact ledger — what every front door must carry

`README.md`, `README.ko.md` and `README.ja.md` are **parallel editions, not
translations.** They share this list of facts. They do not share sentences,
paragraph order, section headings, or length — each is written for its own
reader, from this file.

Numbers, commands, paths, links and version strings below are contractual:
`tools/doc-checks.sh` asserts several of them, and the rest are load-bearing
claims a reader will act on. Everything else — how a fact is introduced, what
gets emphasised, what a section is called, what is left out — belongs to the
edition.

Last verified against the tree: 2026-09-08 (v0.21.0), re-verified the same
evening against source for the review round GDK-1632 (sync interval, token
storage and type, attribution, MCP clients, read exceptions, deletions).

---

## 1. Identity

- Name **gadak**. Brand line **"Find the thread in your backlog."** — under
  the wordmark at the top of all three READMEs, and the one string identical
  in every edition. It replaced "Follow the thread." on 2026-09-08 (brand
  round, GDK-1601): the old line read as a stock phrase to anyone who did not
  know the name is the Korean word 가닥, a strand pulled out of a tangle, and
  it named nothing the reader has. The new one gives the metaphor an object.
  It is positioning, not a promise that every answer is in the cache.
  It lives in **five** places, not the four an earlier count claimed: line 12
  of each README, this entry, and `tools/hosted-demo/build.mjs` — which sets
  it as the hosted demo's `<title>`, its OG/Twitter titles, and a visible
  line under the wordmark. The brand round of 2026-09-08 missed that file;
  a later change to the line must grep, not count.
- **The name may be explained once, in one sentence, and never above the first
  useful example** — the landing footer and the READMEs' maintainer section are
  the places. English: "gadak is Korean for a strand — a thread drawn from a
  tangle." Do not make pronunciation a prerequisite for the demo.
- License **Apache-2.0** (`LICENSE`, `NOTICE`).
- One binary. **No gadak account.**
- Maintainer: one person, currently.
- Repo `https://github.com/midagedev/gadak`. Site `https://gadak.dev`.

## 2. What it is (must be recoverable from every edition)

- Mirrors **Jira *and* Confluence** — issues, comments, history, wiki pages —
  into **one SQLite file on the reader's own machine**, indexed together.
- **Reads never touch the network.**
- Three surfaces on the same mirror: **desktop app**, **browser tab via
  `gadak serve`**, **CLI**. Plus **MCP** for shell-less hosts.
- **The mirror is a cache you can throw away.** Delete the directory and
  nothing is lost — **Jira stays the source of truth.** An edition may let
  the word *cache* carry both halves; it does not have to say "source of
  truth" in words, and should not repeat it (user decision 2026-09-08).
- Writes pass through to the origin first; the mirror refreshes after the
  origin accepts.
- UI language: English, Korean or Japanese, following browser/OS, switchable
  in Settings.

## 3. Status

- **Status: 0.21, still 0.x.** (`tools/doc-checks.sh` check 6 asserts the
  minor version appears as `Status: 0.21` / `상태: 0.21` / `状態: 0.21` in all
  three READMEs, and re-asserts on every tag.)
- Sync, read API, write-through, desktop, web, CLI and MCP are verified
  against a live site.

## 4. Install commands (verbatim; one `brew install` per fence)

macOS desktop app, CLI included:

```bash
brew install --cask midagedev/tap/gadak
```

CLI only:

```bash
brew install midagedev/tap/gadak-cli
```

First run:

```bash
gadak init && gadak sync && gadak serve
```

- The address `gadak serve` prints: `http://gadak.localhost:7777`
- A Jira site needs **one API token**
  (`https://id.atlassian.com/manage-profile/security/api-tokens`); it covers
  Jira and Confluence on the same site. It is a **user token created with no
  scopes** (`ATATT…`); a scoped token or an org key from admin.atlassian.com
  (`ATCTT…`) cannot sign in to a site URL, and the server rejects the `ATCTT`
  prefix before trying (`internal/server/onboarding.go:137`). gadak adds no
  elevation and no service account — the mirror sees what the account sees
  (`SECURITY.md`, "Permissions and scope"). Do not write "read-only token" or
  name scopes: there are none.
- **The reader picks the scope**: `--projects` for Jira, `--spaces` for the
  wiki (both comma-separated; `cmd/gadak/init.go:154,157`). **The wiki stays
  off until spaces are named** (`cmd/gadak/init.go:483`). `gadak init` prompts
  for projects only when stdin is a terminal and none were given; it never
  prompts for spaces. A scoped first run, verified against `gadak init --help`
  (`cmd/gadak/help.go:67-70`):

```bash
gadak init --projects ENG,PROD --spaces ENG
```

- **What keeps the mirror fresh**: `gadak serve` runs the incremental sync
  loop by default when a credential is configured (`cmd/gadak/serve.go:258`;
  `--no-sync` opts out). The default interval is **60 seconds**
  (`internal/config/config.go:382`, `DefaultSyncIntervalSec`; floor 15 s;
  `syncIntervalSec` in Settings → Sync or `gadak config`), plus an **hourly
  reconcile pass** that proves absence: an issue the account can no longer
  see, or that was deleted, is removed from the mirror on the next reconcile
  (`internal/sync/sync.go:899-970`; a scan that returns zero keys refuses to
  empty the mirror). Without `serve`, `gadak sync --watch` runs the same loop;
  there is no `gadak watch` verb.
- **The first full sync is the slow part**: 10.6 minutes for the benchmark
  site (3,323 issues + 457 pages, 2026-08-26, `docs/BENCHMARKS.md`); 26.4 s for
  the 534-issue demo. An edition that mentions first-sync cost cites one of
  those, never "a few minutes".
- No Atlassian account: `gadak init --local` starts a workspace on the
  built-in tracker.
- Migration: `gadak --workspace <new> migrate --from <old>`, and `--to linear`.
- Pairing a second machine: `gadak --workspace laptop init --pairing-code-stdin`.

## 5. Windows (all three editions must carry these)

- The desktop app is on the **Microsoft Store**:
  `https://apps.microsoft.com/detail/9NZW91TXH36G`. The Store signs it, so
  **neither SmartScreen nor Smart App Control objects**, and a Store install
  puts `gadak` on `PATH` (**since 0.20.2**).
- CLI without the Store: `gadak_<version>_windows_amd64.zip` (or `arm64`)
  from the latest release. The literal string **`windows-x64`** must appear
  (doc-checks check 13 greps for it) — it does, in the desktop zip name
  `Gadak-<version>-windows-x64.zip`.
- The release's desktop zip **stays unsigned**. A SmartScreen block is a
  missing signature, not a virus finding (`docs/WINDOWS-SIGNING.md`).
- **Never tell a reader to turn Smart App Control off.** doc-checks check 13
  fails on any phrasing that offers it as a workaround. The docs must say
  *do not* turn it off; the fallback is the Store.

## 6. The measurement (contractual — doc-checks check reads
`docs/BENCHMARKS.md` and requires the date, the corpus size and the figures
to appear in all three READMEs)

Measured **2026-08-26** against a live Atlassian Cloud site — a real work
project, **3,296 issues** — not a synthetic fixture. Medians. gadak figures
include full CLI process startup.

| Question | REST API | gadak | ratio |
| --- | ---: | ---: | ---: |
| Simple filter, 100 issues | 583 ms | 19 ms | 31× |
| One issue with its full history | 710 ms | 28 ms | 25× |
| Free-text search | 543 ms | 41 ms | 13× |
| Open issues per epic (`GROUP BY`) | 4,761 ms — 8 API pages, aggregated client-side | 22 ms — one query | 214× |
| A count over the change history | not expressible — ≈ 28 min of crawling | 14 ms | — |
| Rate limit | 429 + Retry-After | none — your disk | — |

The table must keep: the date `2026-08-26`, the corpus `3,296`, every `ms`
figure and every `×` ratio that the edition prints. An edition may print
**fewer rows** (a mobile-legible subset), but every row it prints must match.

Method, re-measurement history, and **the rows where gadak loses** — first
full sync, the watch tick on a quiet site, one sync interval of staleness —
are in `docs/BENCHMARKS.md`. Every edition links it.

- **"8 API pages" belongs to this measurement**, not to JQL in general: it is
  how many pages the 3,296-issue site returned for the epic count. Write "in
  this measurement, 8 pages", never "you have to fetch 8 pages" (review round
  2026-09-08, all three editions).
- **Three datasets, three names.** The benchmark is the **3,296-issue live
  site**; the live demo is **534 issues**; the flagship search recording is a
  **20,000-issue mirror scaled from the demo snapshot** (§15). An edition that
  prints two of these on one page labels each ("measured", "live demo",
  "recording") so a reader is not left reconciling them.
- The rate-limit row is about **mirror reads**: label it so, or leave it out.
  Sync and writes still meet Jira's limits.

## 7. The argument

- **JQL has no `GROUP BY`.** Past a page size the API stops being slow and
  starts being unable: it hands rows, never the aggregate.
- The canonical query:

```bash
gadak sql "select epic_key, count(*) from issues_full where resolved_at is null
           and epic_key <> '' group by epic_key order by 2 desc"
```

- Datasette Lite runs that same query on the demo snapshot in the browser with
  nothing installed (link in `README.md`, keep it intact if used).

## 8. Agents

- `gadak skill install` — Claude Code by default; `codex`, `agents`, `cursor`,
  `gemini`, `opencode`, `grok` install the same file elsewhere. It copies one
  `SKILL.md`; **no process is started** — the agent runs short-lived `gadak`
  commands (`cmd/gadak/skill.go:3-6`).
- **Two MCP registration commands, and they are not interchangeable**
  (GDK-1633, 2026-09-08):
  - `gadak mcp install claude` execs **Claude Code's** `claude mcp add gadak --
    <absolute exe> [--profile p] mcp` (`cmd/gadak/mcp_install.go:36`). It
    registers with Claude Code, which reads `~/.claude.json`. Claude Desktop
    never sees it.
  - `gadak mcp install claude-desktop` writes the `gadak` entry into Claude
    Desktop's `claude_desktop_config.json` (macOS
    `~/Library/Application Support/Claude/`, Windows `%APPDATA%\Claude\`,
    Linux `~/.config/Claude/`). **This is the line for "hosts without a
    shell (Claude Desktop)".**
  Until 2026-09-08 every front door taught the first command for Claude
  Desktop; three independent reviews caught it. Do not write "for Claude
  Desktop: `gadak mcp install claude`" again. `claude mcp add gadak -- gadak
  mcp` by hand is equivalent to the `claude` client minus the absolute path
  and the workspace pin (`docs/AGENT_SETUP.md`).
- Reference `docs/MIRROR.md`; one paste per host `docs/AGENT_SETUP.md`.
- **The two rules that carry most of the value:**
  1. Filter on `status_category` and `priority_rank`, **never a display
     name** — Jira translates those per account, so `priority = High` is
     silently zero rows on a Korean-language site.
  2. SQL answers while the window presents:
     `gadak sql --no-header "…" | gadak views open --keys -`, and
     `gadak views open --jql '…'` lands pasted JQL as chips.
- Writes (`create`, `edit`, `comment`, `transition`, `claim`, `link`, and the
  wiki `page` verbs) go through the origin before the mirror refreshes.
- **Attribution — say exactly this much and no more** (`internal/origin/trailer.go`,
  `internal/origin/transport.go:31`): on the **built-in tracker** the agent is
  recorded as the write's author. On **Jira Cloud and Linear** the identity
  travels inside the body as one trailing line — `— via gadak · Claude Code
  (claude:…)` — on **three shapes only**: a comment, a transition's comment when
  one was given, and a created issue's description. **Assigns and edits carry no
  attribution on Jira/Linear**, web writes carry no trailer, and **nothing about
  pull requests is implemented**. "Every agent write carries the agent's name"
  and "its comments and linked PRs carry the bot's name" were both on the
  landing until 2026-09-08 and are both overstated; write "an agent's comments
  and the issues it creates carry its name" (off switch `gadak config set
  actor.trailer false`).
- **An agent that reads the mirror sends what it reads to whatever model it
  talks to.** gadak itself sends nothing. Scope the mirror to what the agent
  should see.

## 9. Origins covered

Three origins, one set of verbs:

- **Atlassian Cloud**
- **Linear** (a `"linear"` block in the workspace config, `gadak sync --source linear`)
- **the built-in tracker** that travels with the app

Reads, writes, hierarchy, wiki, attachments, history and the board layout work
on all three. What each origin refuses, with a code citation per cell:
`docs/SUPPORT_MATRIX.md` (every edition links it; neither README restates the
table).

**Three things appear on no origin at all**: sprints as a UI, Jira dashboards,
and Jira's notification inbox. Those stay in Jira.

**The Atlassian origin is Cloud.** Jira Server and Data Center are untested and
therefore unclaimed (`docs/PAIN_POINTS.md:59`, `docs/project/ROADMAP.md:347`).
Every edition must say so where it tells the reader to connect — an on-prem
reader who finds out by failing is a lost reader, and in Japan the Data Center
share makes this the first question (review round 2026-09-08).

## 10. Good fit / bad fit (an edition may compress, not contradict)

- Yes: daily search latency, an agent over tracker *and* wiki, offline reads.
- No: sprint planning, admin, a page editor in the UI, or when a minute of
  staleness matters. `docs/CONCEPT.md#good-fit--bad-fit`.

## 11. Safety claims (each is checkable; do not soften, do not add)

- **No telemetry**, no analytics, no gadak account or server.
- **Outbound traffic is exactly five destinations**, and `SECURITY.md` is the
  authority for the list (`docs/PROMISES.md` promise 2 pins the same five, and
  `tools/doc-checks.sh` check 8 asserts the two files agree). Do not
  paraphrase this list shorter — an edition that enumerates must enumerate
  all five:
  1. the reader's own Atlassian site, for sync
  2. Linear, when a workspace has a Linear source
  3. a paired home `serve`, when the workspace is bound to one
  4. `gh`, only when the reader runs `gadak dev scan`
  5. a library download, only when the reader asks for one
  **Loopback is not on that list** — it is a local bind, not an outbound
  destination. `docs/NETWORK.md` walks every connection and its off switch.
- Reads do not open a connection. The exceptions are four origin-asking verbs:
  `gadak issue --editmeta`, `gadak fields`, `gadak api` (a passthrough), and
  viewing an attachment (`docs/NETWORK.md:21`).
- **gadak never queues a write in the mirror** (`docs/NETWORK.md:29`): a write
  the origin did not accept fails then and there.
- `SECURITY.md` cites **file paths**, not `path:line`. An edition must not
  claim line-level citations.
- **Credentials never reach SQLite, a log, or a snapshot.** The API token
  lives in `~/.gadak/config.json` (per workspace
  `~/.gadak/profiles/<name>/config.json`) — **the same path on every OS**
  (`%USERPROFILE%\.gadak` on Windows); there is **no OS keychain** on the
  desktop (the phone app is the only Keychain user). It is written atomically
  with mode `0600` (`internal/atomicfile`, `internal/config/config.go:908`) in a
  `0700` directory, and sent only as the `Authorization` header to the reader's
  own site — the transport rejects any other host
  (`internal/atlhttp/transport.go:45`). Gates: `docs/PROMISES.md` promise 7
  (snapshot scan), `TestCredentialLifecycle`, `TestExportWhitelistCoversAllConfigFields`.
- **"gadak only talks to what you configured" is true** (GDK-1626,
  2026-09-08). It was not before: gadak used to look for a new version on
  GitHub once a day unless the reader turned that off, so a fresh install made
  one call nobody had asked for, and the review round that day found the claim
  on two landings. That lookup is gone — there is no update check, no
  `updateCheck` setting, and no `internal/selfupdate`. **Do not restore the
  absence as copy**: the front door does not say gadak "does not check for
  updates," because a page that announces what it does not do is advertising,
  not information. Upgrading is `brew upgrade` / a new dmg / a newer zip, and
  that is all a reader needs.
- The 0.x contract is **three promises**, not the whole schema:
  `issues_full` + the RECIPES queries, `gadak sql` stdout format, and
  `gadak views open --keys -` semantics
  (`specs/000-product/data-model.md`).
- What a reader does not have to take on trust, each with the command that
  checks it: `docs/PROMISES.md` — **eleven** items since GDK-1626 (an edition
  that prints the count prints eleven, or leaves the count out).

## 12. Demo

- Live demo `https://gadak.dev/demo/` — **534 issues**, in the browser, no
  install, no account. (doc-checks check 3 pins this count against
  `examples/demo.db`.)

## 13. Feedback and backlog

- File a **GitHub issue**: `https://github.com/midagedev/gadak/issues`. The
  maintainer mirrors it to the backlog.
- Commit `GDK-nnn` keys resolve on the public backlog
  `https://gadak.dev/backlog/`.
- **Never paste real issue data, tokens, or site URLs into a public issue.**
  When an edition asks for an agent example, it asks for a *sanitized*
  description of the question — never "paste the question you asked".
- A bug report needs: Jira deployment type (Cloud), the gadak commit, and the
  command that was run. **A use report needs none of that**: ask what question
  gadak answered and whether the reader used it again; do not prescribe the
  form of the testimony ("N issues, this happened") or say which verdict the
  maintainer prefers (review round 2026-09-08, all three editions).
- Starting points: `.github/CONTRIBUTING.md`,
  `docs/project/GOOD_FIRST_ISSUES.md`. Why the next features are the ones they
  are: `docs/project/THEORY.md`.

## 14. Documentation links every edition should reach (naming may differ)

`CHANGELOG.md` · `docs/INSTALL.md` · `docs/DESKTOP.md` · `docs/SHOWCASE.md` ·
`docs/MIRROR.md` · `docs/MCP.md` · `docs/AGENT_SETUP.md` · `docs/RECIPES.md` ·
`docs/DASHBOARDS.md` · `SECURITY.md` · `docs/FAQ.md` · `docs/MAINTENANCE.md` ·
`docs/README.md` · `docs/ARCHITECTURE.md` · `docs/EXTENDING.md` ·
`docs/BENCHMARKS.md` · `docs/SUPPORT_MATRIX.md` · `docs/WINDOWS-SIGNING.md`

The Korean edition links `CHANGELOG.ko.md`. The Japanese changelog is
published in English — say so where it is linked.

## 15. Media assets

`docs/media/` **does** carry locale recordings for the README heroes:
`terminal-hero.ko.gif` / `terminal-hero.ja.gif` (plus `.mp4` and posters),
`scale.*`, `search.*`, `og.*`. The Korean and Japanese READMEs already point
at their own take — keep it that way and describe what that take shows in
that language.

`web-demo.gif` is English-only and shared by all three.

**The "20,000 issues" in the flagship clip's caption** is the recording's
corpus, not a benchmark: `e2e/demo/scale-demo.spec.ts` records against
`examples/demo.db` scaled by `gadak snapshot --scale 20000`. It is one screen
above the benchmark's real 3,296-issue site, so a caption that prints it must
make clear it is the recording's mirror — or leave the number out.

The **site landing** serves the same per-locale takes: `site/src/i18n.ts`
`MEDIA_LOCALES` lists `terminal-hero.mp4` and its poster for ko and ja (since
2026-09-07), and the built `/ko/` and `/ja/` pages reference
`terminal-hero.ko.mp4` / `terminal-hero.ja.mp4`. GDK-1607 was filed on the
opposite claim without checking the built page and was closed as invalid on
2026-09-08 — verify against `site/dist` before re-filing.

## 16. The front door's shape (review round 2026-09-08, GDK-1632)

Three independent editorial reviews of the landing and the READMEs converged
on the same defects; these are the rules that came out of them. They bind the
landing (`site/src/i18n.ts`) and all three READMEs.

- **The hero names the job, not a feeling.** "Same Jira. No waiting." told a
  reader arriving from a search result nothing about the tool and promised no
  waiting for operations that still wait on sync. Each edition's heading says
  what its reader came for (`site/src/tagline.js`); the brand line (§1) sits
  under the wordmark and does not explain the product.
- **Prove it before asking for a token.** The canonical query (§7) and the
  Datasette Lite link sit on the landing, not only in the README, wherever
  the edition leads with SQL.
- **One "before you connect work data" block**, in this order of the reader's
  questions: which Jira (Cloud), how much is copied (`--projects`/`--spaces`,
  wiki off), what stays local and how fresh (one SQLite file, first sync, one
  interval behind), where the token is and is not, what leaves the machine
  (no telemetry; only what you configured; the full list in `SECURITY.md`),
  what a write does (origin first; a rejected write fails, nothing queued),
  what changes with an agent (it sends what it reads to its model). Heading
  it as a verdict — "Why this is safe to try", "안심하고 써도 되는 이유" — is
  banned: the reader makes that call.
- **No framing devices.** "The first question in every thread", "Fast is a
  measurement, not an adjective", "the honest where-gadak-loses table", "One
  vocabulary between you and the agent", "half the reason gadak exists", "SQL
  answers while the window presents" and their ko/ja twins are argument
  scaffolding and self-praise; each was named by a reviewer as the sentence
  most likely to read as machine-written. Say the fact, link the evidence.
- **The Rovo comparison is capability and operating cost, not a scoreboard.**
  Hosting is not why Rovo lacks an aggregate; say "no native aggregation tool,
  no offline reads". The row gadak loses is a real cost — a local binary and an
  initial sync — not a staged concession. Cite the 22 ms with its corpus and
  date wherever it appears.
- **Every edition ends by asking what happened** (§13 wording) and says what
  must stay out of a public report.
- **Every edition carries status where a first-time reader finds it**: 0.21
  / 0.x, one maintainer, Apache-2.0, the three promises, and the work that
  stays in Jira.
- **README order**: definition and status → the query → install → before
  connecting (or the security block) → agents → origins and limits → status →
  feedback → docs → license. Migration, the built-in tracker and pairing are
  "other workspaces", after the first Jira path, never inside it.
