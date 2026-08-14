# v0.13 — the dedicated browser, and the agent that drives it

Owner decision, 2026-08-14. This wave sharpens the product statement and
closes the gaps between it and the tree. The census behind every file:line
claim here was taken against `610e350`.

## The statement

**gadak is the dedicated browser for your issue tracker.** Stop piling Jira
tabs in Chrome; this window is where Jira lives on your machine. What the
mirror models — lists, detail, search — is answered natively, instantly.
What it deliberately does not model — boards, dashboards, arbitrary pages —
is *contained*: an in-app tab on desktop, the system browser under `serve`.
Reimplement nothing; contain everything. And the window has a second user:
your coding agent points at it (`gadak views open`) instead of pasting a
markdown table, so you and your agent look at the same work.

Hierarchy, in order of what the product *is*:

1. **The mirror is the body.** Speed, offline, SQL — the moat. (Unchanged
   since CONCEPT.md v1.)
2. **The browser feel is the packaging.** It is how the value is pitched
   ("stop having fifteen Jira tabs"), never a feature list of its own.
3. **The agent handoff is the differentiator.** "Agent points, human sees"
   is the part nobody else has.

## Guardrails (each one blocks a known failure mode)

- **G-a. Hierarchy.** A feature that grows the shell but not the mirror or
  the handoff is default-rejected. The pitch word is "where your Jira
  lives", not "browser" — the browser word buys universality expectations
  (SSO, every page perfect) that an embedded WebView cannot honor.
- **G-b. Escape hatch, not floor.** No native surface may *require* the
  in-app browser. Feature requests aimed at the browser pane (history,
  bookmarks, extensions, persistent tabs) are default-rejected. The pane
  owns tabs, a rectangle, and post-close resync — nothing else.
- **G-c. Anti-tab.** In-app tabs are session-scoped consumables. Retrieval
  ("I'll need this again") is the mirror's job: search, recents, favorites
  (UX_PRINCIPLES §6). The tab strip must never become a second sidebar.
  The success metric is retrievals that needed no tab, not tab count.

## Decisions taken with this wave

- **ROADMAP "Next" sequencing.** The 2026-08-07 "no feature waves until a
  user shows up" stance is amended, not ignored: PR #1 (external user, real
  bug, real fix) is the first arrival signal, and this wave is aimed at the
  two audiences that arrival proved exist — agent-driven use and daily
  in-app living. The "collect questions, watch an install" work continues
  in parallel; it is not displaced by this wave.
- **Agent interface ranking.** The database stays the agent's *answer*
  interface (contracts/agent.md). `gadak views open` is the *presentation*
  interface. SQL answers; views present. MCP presentation (`gadak_show`)
  is **deferred pending an owner call** — it contradicts the written
  "MCP is read-only, no fourth tool" contract and needs that contract
  revised first, not silently violated.
- **`gadak open` stays the Jira escape hatch** (system browser to
  `/browse/KEY`); `gadak views open` is the "open in gadak" verb. Docs must
  say this out loud — the two names collide in the mind.
- **The desktop-only truth is stated, not hidden.** The contained-browser
  half of the concept exists only in Gadak.app (WKWebView; Atlassian
  forbids iframes). `gadak serve` users get native surfaces plus system
  tabs. SECURITY.md must mention the WKWebView cookie session as a second
  credential surface.

## What the census found (drives the task list)

Reports: session scratchpad `census/01–05`, taken 2026-08-14. Key gaps:

1. `key in (A, B)` compiles into one text needle → **empty list** everywhere
   (internal/jql/compile.go:439-450, untested, emit loses the axis). Blocks
   the core agent flow "SQL result → show these N keys".
2. A pasted `/browse/KEY-123` URL is classified as JQL, `preventDefault`ed,
   then **silently swallowed** (SearchBox.svelte onPaste + `not_jql`).
3. Clicking an issue/page link inside a body opens an **in-app Jira tab**
   (desktop) or a Chrome tab (serve) — never the native panel the mirror
   already has (desktop-links.ts classifyAtlassianLink kinds issue/page).
4. `views open` cannot focus one issue's detail, take a key list, or read
   stdin (cmd/gadak/views.go:128-147).
5. Multi-profile miss: `GET ui-focus/` always reads the *process* profile's
   file (internal/server/focus.go → uifocus.Take → config.Dir), so
   `/w/<name>/` tabs can never be focused; `openServeOnHash` never prefixes
   `/w/<name>/` either.
6. Retrieval gaps vs the anti-tab thesis: browse-tab visits never enter
   recents; sidebar recents exclude docs; imported Jira filters are absent
   from the ⌘K palette; `?filter=<id>`-only URLs dead-end even when the
   filter is synced.
7. Agent surfaces teach *answering*, not *showing*: SKILL.md's trigger list
   has no "show the user" trigger; Cursor/Codex setup blocks omit `views`
   entirely; nothing prints a gadak view URL.
8. The 500 ms ui-focus poll runs even in hidden tabs (App.svelte onMount);
   pause-when-hidden is the cheap correct fix (census option C; fsnotify→
   Wails event is the desktop-native follow-up if it ever matters).
9. Doc rot: CONCEPT.md predates the concept; ARCHITECTURE.md still lists
   the removed `presence/` stack; internal/jql/hash.go `HashURL` has zero
   callers.

## Contracts pinned for the implementation tracks

- **`keys` axis.** Go `jql.Filter.Keys []string` (`json:"keys"`, empty
  slice marshals `[]`); web `ViewFilters.keys`; URL param `ks` (verified
  free). `key/issuekey/issue = X | IN (…)` compiles to Keys — never to `q`.
  Emit: one key → `key = K`, several → `key in (K1, K2)`. Match: exact,
  case-insensitive on the key. Order is meaning: the UI lists keys in the
  given order when no explicit sort is set (agent ranking survives), and
  the `q`→relevance auto-promote must not fire for a keys-only filter.
  Cap 500 keys with a loud error.
- **Omnibox rule (paste routing).** A pasted Atlassian URL routes like a
  dedicated browser's address bar: `/browse/KEY` → native selection when
  the key is in the pool, else a toast naming the miss; `jql=` → chips
  (unchanged); `?filter=<id>` only → apply the synced `source_queries` row
  when present, else the existing toast; wiki page URL with a mirrored id →
  native `pages.select`; any other same-site URL → in-app tab (desktop) /
  system browser (serve). Nothing is ever silently swallowed.
- **Body-link rule (click routing).** Links the mirror models open native
  (`selection.select` / `pages.select`); the detail-header key stays the
  explicit "open on Jira" escape hatch; unmodeled same-site links keep
  today's containment behavior.
