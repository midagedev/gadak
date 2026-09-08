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

Last verified against the tree: 2026-09-08 (v0.21.0).

---

## 1. Identity

- Name **gadak**. Tagline "Follow the thread."
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
  nothing is lost — **Jira stays the source of truth.**
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
  Jira and Confluence on the same site.
- **The reader picks the scope**: `--projects` for Jira, `--spaces` for the
  wiki. **The wiki stays off until spaces are named.**
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

- `gadak skill install` — Claude Code by default; `codex`, `cursor`, `gemini`,
  `opencode`, `grok` install the same file elsewhere.
- `gadak mcp install claude` — for hosts without a shell (Claude Desktop).
- Reference `docs/MIRROR.md`; one paste per host `docs/AGENT_SETUP.md`.
- **The two rules that carry most of the value:**
  1. Filter on `status_category` and `priority_rank`, **never a display
     name** — Jira translates those per account, so `priority = High` is
     silently zero rows on a Korean-language site.
  2. SQL answers while the window presents:
     `gadak sql --no-header "…" | gadak views open --keys -`, and
     `gadak views open --jql '…'` lands pasted JQL as chips.
- Writes (`create`, `edit`, `comment`, `transition`, `claim`, `link`, and the
  wiki `page` verbs) go through the origin before the mirror refreshes, and
  **every agent write carries the agent's name**.
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

## 10. Good fit / bad fit (an edition may compress, not contradict)

- Yes: daily search latency, an agent over tracker *and* wiki, offline reads.
- No: sprint planning, admin, a page editor in the UI, or when a minute of
  staleness matters. `docs/CONCEPT.md#good-fit-bad-fit`.

## 11. Safety claims (each is checkable; do not soften, do not add)

- **No telemetry**, no analytics, no gadak account or server.
- **Outbound traffic is exactly six destinations**, and `SECURITY.md` is the
  authority for the list (`docs/PROMISES.md` promise 2 pins the same six, and
  `tools/doc-checks.sh` check 8 asserts the two files agree). Do not
  paraphrase this list shorter — an edition that enumerates must enumerate
  all six:
  1. the reader's own Atlassian site, for sync
  2. GitHub Releases, at most one anonymous version-check GET per day
     (`updateCheck: false` turns it off — `internal/config/config.go:191`)
  3. Linear, when a workspace has a Linear source
  4. a paired home `serve`, when the workspace is bound to one
  5. `gh`, only when the reader runs `gadak dev scan`
  6. a library download, only when the reader asks for one
  **Loopback is not on that list** — it is a local bind, not an outbound
  destination. `docs/NETWORK.md` walks every connection and its off switch.
- Reads do not open a connection. The exceptions are four origin-asking verbs:
  `gadak issue --editmeta`, `gadak fields`, `gadak api` (a passthrough), and
  viewing an attachment (`docs/NETWORK.md:21`).
- **gadak never queues a write in the mirror** (`docs/NETWORK.md:29`): a write
  the origin did not accept fails then and there.
- `SECURITY.md` cites **file paths**, not `path:line`. An edition must not
  claim line-level citations.
- **Credentials never reach SQLite, a log, or a snapshot.**
- The 0.x contract is **three promises**, not the whole schema:
  `issues_full` + the RECIPES queries, `gadak sql` stdout format, and
  `gadak views open --keys -` semantics
  (`specs/000-product/data-model.md`).
- What a reader does not have to take on trust, each with the command that
  checks it: `docs/PROMISES.md`.

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
- A bug report needs: Jira deployment type (Cloud), the gadak commit, and the
  command that was run.
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

The **site landing** is the gap: `site/src/i18n.ts` `MEDIA_LOCALES` does not
register the agent clip, so `/ko/` and `/ja/` show the English recording while
the caption says a Korean sentence drives it (GDK-1607). That is a site
defect, not a README one.
