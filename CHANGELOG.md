# Changelog

## v0.15.0 — unreleased

<!-- DRAFT — finalize at tag time. Placeholders marked [PENDING] track rounds
     still in flight; remove or resolve every one before tagging. -->

The release that opens gadak outward. A view or an issue is now a link any
app can hand over, search is fast enough to drive somebody else's UI
keystroke by keystroke, and Raycast gets a documented way in. Inside, a dark
theme built to the same paper-and-ink standard as the light one — and the
first run of a new ritual: a full-codebase audit before every minor.

### A gadak is now an address

- **`gadak://` deep links** (GDK-119). The macOS app registers a URL scheme,
  so a piece of gadak travels as a link instead of a shell command:
  `gadak://view?issue=NMB-140`, `gadak://view/w/oss?pj=GDK&sc=inprogress`.
  `gadak views open` prints the link next to the http one. The grammar
  carries no verb and no payload — a link says *where to go*, never what to
  do — and the parser deliberately owns only the shape, so new actions are a
  handler-table entry, not a grammar change. This is the first release whose
  shipped artifact actually claims the scheme; the release check now tests
  the installed bundle, not the script that writes it.
- **Every place has a name in the URL** (GDK-124). The person panel, the
  personal feed and the settings tab join the issue, document and space
  params — nine place params in one reviewed registry
  (`web/src/lib/url-state.ts`). A param registered there is deep-linkable
  the same moment, with no Go change; compose and credential forms are
  deliberately excluded, and the registry is where that refusal is enforced.
- **Raycast, both doors** (GDK-117). `gadak mcp install raycast` prints the
  values for Raycast's *Install New Server* form (Raycast ≥1.98 speaks MCP
  over stdio but has no config file to write into). For the keystroke-fast
  path, the local search that a Raycast extension would sit on measures
  p50 ~2–4 ms over HTTP and ~24 ms per CLI spawn on the demo mirror — under
  a "feels instant" budget either way.
- **An issue can name its parent** (GDK-19 in part, toward GDK-86).
  `gadak create --parent KEY` and `gadak edit --parent KEY` write the
  sub-issue relationship through Jira; the mirror learns it on the next
  tick. Link types (`blocks`, …) and components editing remain open.
- **Typing an issue key finds that issue** (GDK-170). Server search used to
  index only title/body/comments — `NMB-140` returned four wiki pages that
  mention the issue and not the issue itself — and ranked with bare bm25. A
  key query is now a lookup promoted above FTS (case-insensitive, `nmb140`
  and prefix forms included, never evicted by the limit), FTS columns are
  weighted title > body > comment, and the web defers to the server's order
  instead of re-ranking it — so the CLI, the REST route, MCP and the list
  all give the same answer. `gadak search --explain` answers "why is this
  row above that one".

### A dark theme, and a place for the next one

- **Dark** (GDK-154, GDK-156, GDK-162). Warm ground, ink foregrounds, the
  same paper metaphor as light — with the anti-slop rule encoded as a CI
  contract (`tools/theme-check.mjs`): hue must stay warm, chroma inside the
  reference band, so a generic cool-gray dark cannot land by accident. A
  blocking boot script reads the stored preference before first paint (no
  flash), and adding a third theme is now one definition block plus a
  registry entry. The picker lives where the app's other per-browser
  settings already were — settings dialog and ⌘K palette, not new chrome.
- **Success and failure stop being told by colour alone** (GDK-158). Toasts
  carry per-kind icons and the breakdown bar carries glyphs, so a
  deuteranopic reader gets the same answer everyone else does.
- **Both palettes clear the same measured floors** (GDK-157, GDK-159,
  GDK-171). Status inks in both themes now pass pairwise ΔE separation in
  normal *and* deuteranopic vision — dark's in-progress and stale were ΔE
  0.008 apart, one colour twice. The search highlight gets its own token
  instead of borrowing a status colour (which vanished on the selected
  row), and each theme derives its own: the light mark carried light text
  at 1.18:1 when transplanted into dark. The gate now measures the text
  that actually sits on the mark, in both themes, so that class of
  transplant cannot land again.

### The list behaves like a AAA list

- **The right side of a row is a column you can scan** (GDK-128). Labels,
  staleness and the trailing strip sit in fixed-width slots instead of
  drifting up to 274 px per row; container queries retune the widths per
  regime instead of hiding information.
- **The last row stops being cut in half** (GDK-131). A flex scroller drops
  its own padding-bottom in scrollable overflow — one shared container rule
  (`.scroll-region`) now owns the bottom inset everywhere, instead of a
  per-panel `pb-3` that never worked.
- **Esc closes what it looks at** (GDK-132, GDK-133). The three list header
  menus close on Escape and outside-click through the same `dom-actions`
  owner every other menu uses, and the sidebar stops highlighting a view row
  while a feed or document screen owns the main column.
- **A covering panel declares itself** (GDK-127). Below 1440 px the detail
  panel overlays the list behind a scrim instead of silently sitting on top
  of live rows.
- **One concept, one Korean word** (GDK-135). The ko catalog stops mixing
  용어 for the same concept across dialogs, toasts and empty states.

### Honesty at the edges

- **A hosted snapshot no longer advertises verbs it cannot answer**
  (GDK-52). Server-dependent affordances (FTS, settings, docs freshness)
  key off the capability document instead of optimism, so the demo and any
  static mirror stop rendering dead buttons — and the e2e webServer names
  its shell instead of assuming one.
- **The legacy field mapping retires itself** (GDK-149). A config still
  carrying `fieldMap`/`editableFields` is rewritten to `fields` once, at
  load, with one stderr line saying so; exports stop emitting the legacy
  keys. And the rewrite is a convenience, not a precondition (GDK-173): on a
  read-only home it becomes a warning and the app runs on the in-memory
  mapping instead of refusing to start — a locked-down directory stays
  locked instead of being silently chmod-unlocked, and `gadak status` now
  names a config it cannot read instead of swallowing the error.
- **The desktop app stops loading its runtime twice** (GDK-150). The wails
  runtime is injected server-side only, a dock-icon click reopens the closed
  window, and the desktop module finally builds and tests in CI on macOS.

### The audit, and what it deleted

First run of the per-minor full-codebase audit (GDK-125/126; the procedure
is now `docs/runbooks/release-audit.md`). Eighteen findings fixed in this
release; the rest carry `carryover-v0.15` labels. Highlights, best measured
in lines removed:

- Timestamps get one owner — `config.ISOMilli` replaces 34 quoted format
  literals across 19 files (GDK-148); `VIEW_PARAM_KEYS` becomes the type
  instead of feeding a mirror list and the drift test both die (GDK-147);
  Svelte hygiene drops positional list keys, a toast-host reach-in and eight
  dead exports (GDK-152).
- The test pyramid gets enforced downward: sixteen browser tests become
  vitest units — one of them was asserting a contract that no longer
  existed (GDK-145); the Go suite stops sleeping on wall clocks, ~12 s
  faster (GDK-144); the three untested pure modules and the Jira URL
  composition get real cases (GDK-146).
- `docs/DERIVE.md` becomes the single home for derived-field semantics, and
  its SQL examples are executed by a test, so the doc cannot drift from the
  code it documents (GDK-88, GDK-89).
- Chosung (초성) search retires, product-wide (GDK-168). It existed only in
  the web while the CLI, REST, MCP and Raycast all lacked it, its cost sat
  on the hottest per-keystroke path, and a chosung hit could never highlight
  *why* it matched. ~144 lines removed, nothing added in their place; a
  jamo-only query is now a plain miss on every surface, which at least is
  the same answer everywhere.

## v0.14.2 — 2026-08-16

The release about the first ten minutes and the day the token dies. Nothing
here is a new capability so much as an existing one that finally tells you
what it is doing.

- **Every token trap is named before the paste, not after the 401** (GDK-69,
  GDK-98). Atlassian's token page offers three things that look like one, and
  two of them cannot sign in to a site URL: a *scoped* token — which that page
  now recommends first — and an org key from admin.atlassian.com. gadak only
  ever said so after the rejection. Both the web form and `gadak init` now say
  it up front. Behind a 401, the one trap that is recognisable from the token
  itself (the ATCTT prefix) is named outright; the rest share a message that
  hands you a check you can run, because Jira answers all of them identically
  and inventing a distinction would be worse than admitting there isn't one.
- **A rejected token is recoverable without writing** (GDK-68). Only the write
  path used to offer the replace-token dialog, so a person who reads the
  mirror saw a dead freshness chip and a technical error string. The sync
  progress document now carries `error_code`, classified by the one function
  that already owned that rule, and the chip, the palette and the empty-mirror
  CTA all reach the same dialog. A wiki-only 401 deliberately does *not*
  count: the Jira pass authenticated with the same token moments earlier, so
  that is a permission gap, not a dead credential.
- **Picking no projects is a choice** (GDK-99). The CLI and settings have
  always read an empty project list as "everything this account can see". The
  first-run wizard was the only surface calling it illegal — Start sat
  disabled next to its own "Select none" button — which forced a decision the
  product does not require, and the wrong one on a large site, where the
  picker is truncated and "select all" was never "everything".
- **`gadak skill install` treats an upgrade as an upgrade** (GDK-92). After
  `brew upgrade` the installed skill is the previous release's own copy, so it
  differed, so the one-liner in our own docs turned red. Provenance is now
  decided by content hash — an install receipt, plus a frozen table of the
  digests shipped before receipts existed. A file *you* wrote is still
  refused; that refusal is the feature. `doctor` grew skill and MCP lines, so
  "is my skill current?" is one command.
- **The embedded skill knows the verbs the CLI has** (GDK-91). It described
  reads plus comment/transition/assign and stopped there, so an agent with the
  skill loaded answered "gadak cannot create issues" or reached for the REST
  API. v0.14.1 shipped `create`, `attach` and `edit`; the file agents read
  never learned.
- **A quiet Confluence tick reads zero page bodies** (GDK-113). A sync tick
  took 21.4s, and 19.4s of it re-read 71 unchanged wiki pages: minute-grained
  CQL kept returning the same cluster forever, and nothing decided between a
  search hit and a body fetch. One owner decides now, and `gadak sync` prints
  the tally so the next person can check without adding printlns.
- **`gadak issue <KEY> --derive`** (GDK-111) prints how the derived columns
  were computed — the changelog by status *category*, and the rows behind
  `reopen_count`, `resolved_at`, `reopen_reason` and `epic_key`. It calls the
  same derivation the sync path calls; a second copy would agree with the
  first only until one of them changed.
- **History keeps its order** (GDK-26): "Show issues in list" no longer
  regroups by status, which is the one thing that pane exists to show.
- Also: token expiry is recorded and warned about before the sync dies
  (GDK-67/70), the browse pane yields Escape and stops outliving its document
  (GDK-78/79/80), `gadak sql` warns on a stale mirror and `gadak_query` flags
  display-name zero rows (GDK-90), `Open` repairs an `items_fts` this build
  cannot write (GDK-112), the search-help `?` works on touch (GDK-53),
  `examples/compose` lands as pure shell (GDK-109), the Datasette Lite deep
  link is pinned (GDK-101), and `PROMISES.md` is gated against `SECURITY.md`
  (GDK-104).
- **Process, because it failed twice in one day** (GDK-57): the Node version
  had five owners and none a shell could read — `.nvmrc` is the single one
  now — and `tools/ci-status.sh` answers "did what I just pushed pass?", which
  is the question that went unasked while main sat red for an hour.

## v0.14.1 — 2026-08-15

One day of dogfooding gadak's own backlog through gadak, shipped as it
landed: the first CLI write verbs, a demo that finally works where people
actually tap it, and the removal of an updater that had never earned trust.

- **The macOS app is notify-only again.** Removed the never-exercised in-app
  self-updater (Wails `pkg/updater`): digest verification was fail-open and
  the swap was non-atomic (GDK-58/59/60). The sidebar banner still names a
  newer release; installing it is `brew upgrade --cask gadak` or a new dmg.
  v0.14.1 ships no `gadak-desktop-darwin-<arch>.zip`, so a v0.14.0 app in
  the wild cannot self-swap. Docs realigned (GDK-61/64). Found on the way:
  the desktop banner had been silent in every release build because
  `server.Version` was never assigned there — now wired.
- **The first write verbs: `gadak create`, `gadak attach`, `gadak edit`.**
  Create takes `--project`, `--type`, `--priority`, labels, a description
  from stdin, files to attach, and `--batch -` for JSON lines — everything
  this backlog's own migration to Jira needed. Unknown flags are rejected
  instead of being folded into the summary.
- **The hosted demo works inside in-app browsers** (GDK-23, GDK-51). The
  snapshot service worker is gone — an in-page fetch adapter serves the
  frozen mirror, so the X/Slack webviews that blocked workers now boot. And
  the first paint is no longer 4px text: a static first frame (claim,
  tap-to-load demo video, a selectable `brew install`, the repo link) is
  injected at build time and reads at phone width before any JS arrives.
- **The browse pane yields** (GDK-76/77). At the shipped window size the
  in-app browser pane sat over the command palette and every dialog; toasts
  painted under the native page. Stacking now has one owner and the palette
  is frontmost and clickable while browsing.
- **Boot keystrokes are held, not dropped** (GDK-46). `j`/`k`/`x` pressed
  while the startup view is still committing replay in order once keys are
  ready, instead of silently acting on the wrong list.
- **Failures say what happened.** A failed write reports in the reader's
  language, not a Go error chain; a truncated key list says how many keys
  were given and shown (GDK-35); a rejected credential stops the watch loop
  for every source — Confluence included — and leaves a visible trace
  (GDK-24, GDK-48).
- **Priority colors read the rank, not the account's language** — a Korean
  Jira no longer renders every priority as the fallback color.
- **Faster agent surface**: MCP tools stop scanning the whole mirror per
  call; attachment ownership is one query.
- **A web unit tier**: 100+ component/logic specs run in ~300ms on every
  push, alongside the browser e2e set; the HTTP transport and the secret
  scanner got their first tests.

## v0.14.0 — 2026-08-15

The maintainer-review release: seven builders of loved developer tools were
asked, per lens, why gadak would or would not be loved — and every confirmed
finding either shipped or got a bar written down. The theme is trust:
surfaces that fail loudly instead of silently, docs that match the code, and
measured numbers instead of adjectives.

- **The first agent call succeeds, or says why not.** A small model called
  `gadak_search` with `{query: …}` — the argument name of every search tool
  on earth — got a terse error, and reported it as "no results". That was a
  schema defect, not a model defect: the primary argument is now `query`
  (`text`/`q` stay as aliases), every tool error starts with `ERROR:` and
  echoes the argument keys it actually received, and the MCP instructions
  teach `gadak_query` as the default tool instead of telling the model to
  leave (`If you have a shell…` is install-time advice and now lives only in
  the docs). `gadak_issue` over the response cap sheds oldest comments and
  says `truncated` instead of dying whole.
- **The pipe is a promise now.** Three things are contracted while 0.x:
  `issues_full` + the RECIPES queries, `gadak sql` stdout (header TSV by
  default, `--no-header` to omit, `--json` one object per row, never a banner
  on stdout), and `views open --keys -` (stdin, comma/whitespace, first-seen
  order). A typo'd flag like `--pretty` used to be silently joined into the
  SQL — blank output, exit 0; it is now a loud usage error that names the
  token.
- **`gadak export` / `gadak import`.** The rows you would actually miss —
  saved views, watches, favorites — leave in one JSON (no credentials, no
  site URL) and come back with upsert semantics. The round-trip is the test:
  export, delete the mirror, resync, import, the named view is back.
- **Measured, with the losing rows.** A live-site benchmark against a
  2,853-issue Cloud project: 42× on a simple filter, 162× on the epic
  GROUP BY (7 API pages vs one query), and the reopen count — ~20 minutes of
  changelog crawling over REST vs 14.5 ms locally. Also printed: minutes of
  first sync, 6.6 s per watch tick, one interval of staleness.
  `docs/BENCHMARKS.md` has the method; `tools/bench-live.py` runs it against
  your own mirror.
- **The settings dialog stops lying.** Emptying the project selection said
  "no issue is mirrored" while the backend syncs *everything the account can
  see* — the copy now says so, and says the full sync starts on save. The
  web-push toggle (a deliberately cut feature whose endpoints 404) is gone.
  "Applies after restarting gadak serve" was false — config reloads on the
  next tick. Copy now branches on one `surface()` (serve / desktop / hosted):
  the app names its own sync loop, hides the in-tab notification row (the
  menu-bar notifications already cover it), and the sqlite3 button says
  "paste in a terminal".
- **The hosted demo lands on Epic breakdown** — open work grouped by epic,
  the README's "which epic is stuck?" on screen before a single click,
  instead of a bare all-open list.
- **`brew install midagedev/tap/gadak` is the app now.** The cask carries
  Gadak.app *and* puts the bundled CLI on PATH; `gadak-cli` is the CLI-only
  formula (macOS + Linux). The old hold on casks — unsigned binaries — was
  resolved when notarization shipped in v0.13; only the comment didn't know.
- **Docs told the truth again**: a 40-finding census against the code fixed
  shipped-but-documented-as-pending features, the over-broad "schema is a
  public contract" phrasing (it is three promises, stated in one shared
  sentence everywhere), Rovo comparison honesty (it does search both sources
  now; it still cannot aggregate), and the numbers that rot — enforced from
  now on by `tools/doc-checks.sh`. Also: a Korean README (`README.ko.md`),
  and a repo `CLAUDE.md` so every session and agent starts from the same
  contract.

## v0.13.0 — 2026-08-14

- **One search box that searches everything.** ⌘K — or the new **Search ⌘K**
  button in the list toolbar, because a shortcut nobody can see is a feature
  nobody finds — queries the whole mirror: every issue title, body and comment,
  every document title and body, in one FTS index, *ignoring the filter chips
  on the list*. Each row says which field matched and shows the snippet. The
  box above the list keeps its old job and now says so ("narrow this list"):
  the two searches were the same control before, which is exactly why nobody
  could tell what was being searched. The server could always answer this; only
  the UI was hiding it.
- **History, in a file the mirror cannot take with it.** What you opened and
  what you searched for are now recorded — in a *second* SQLite file,
  `~/.gadak/local.db`, beside the mirror. That separation is the point: the
  mirror stays a cache you can delete without losing anything gadak wrote for
  you, and `export-static` and snapshots cannot leak your reading history.
  The sidebar's recents header opens a first-class **History** view: issues,
  documents and searches on one timeline, grouped by day, with a visit count
  once you have opened something twice. Searches replay when clicked. The
  store `ATTACH`es `local.db` when it opens the mirror, so an agent joins
  `local.visits` to `issues` in a single `gadak sql` — "the issues I looked at
  this week" is a query, not a feature request. There is no clear-history
  button yet: the delete endpoint is not written, and a button that cannot do
  what it says is worse than none.
- **The issue list stops losing to the document screen.** From a document,
  "Assigned to me" changed the URL and left you where you were. The main
  column's occupants — feed, space, documents, list — were independent
  latches, so every "show me the list" call site had to remember to drop all
  of them, and one did not. That intent now has a single owner and every path
  goes through it. Opening an issue deliberately does not: that is a panel,
  not a column, and a test pins the difference.
- **The window follows the agent.** A `keys` axis (`ks=` in the URL) makes an
  arbitrary set of issue keys a first-class view, so an agent can hand you the
  answer instead of pasting a table:
  `gadak sql "…" | tail -n +2 | gadak views open --keys -` puts exactly those
  issues, in that order, on the running app or `serve` tab. `gadak views open
  NMB-140` focuses one issue. Two verbs that read alike now differ plainly:
  `gadak views open` opens *in gadak*, `gadak open` leaves for Jira.
- **MCP gains a fifth tool, `gadak_show`**, so a host without a shell (Claude
  Desktop) can present too — pass one of `jql` / `keys` / `issue` / `name` and
  the running window applies it. The MCP contract is restated to match what it
  actually does: no writes to the mirror or to Jira; presentation is a
  permitted local act, ranked below SQL. SQL answers; show presents.
- **Confluence space scope is now real.** Narrowing the space list used to stop
  *fetching* a space without ever *removing* it, and widening it only pulled
  pages newer than a shared watermark — so a mirror could hold thousands of
  pages from spaces you had deselected while the space you did select showed a
  handful of documents. Each space now carries its own watermark (schema
  **v19**): a newly selected or restored space backfills in full, one space's
  failure cannot skip another's history, and every successful pass removes the
  spaces that left the scope. Found on a real work mirror; the manual repair in
  `docs/runbooks/confluence-space-scope.md` is now a fallback, not the fix.
- **The account-id bug class is closed, not patched.** #1 fixed one surface;
  the same defect — an optional Jira field used as identity — was open on eight
  more. People now resolve to account ids across JQL, saved views, the import
  of Jira filters, the member directory, and the web's filters and caches, with
  email kept as a fallback for rows written before ids were stored. On a site
  that hides `emailAddress`, `assignee = currentUser()` no longer returns
  nothing and email-less teammates no longer vanish from ⌘K, avatars and
  grouping. Changelog and attachment authors gain `author_id` (schema **v20**),
  so same-named people stop colliding in the feed.
- **Security.** A profile name could escape the home directory — `--profile
  ../../.ssh` wrote a token-bearing `config.json` there and chmodded the
  directory; profile names are now validated where paths are built, not at the
  call sites. The browser guard also ran only *inside* the API handler, leaving
  `/config.json`, `/healthz` and `/api/v1/workspaces` reachable by a
  DNS-rebinding page, which exposed your site URL, project keys and every
  profile's name; the guard now wraps the whole mux.
- **The macOS window can be dragged** (#2, thanks @wafe). It never could: the
  Wails runtime module was not loaded, so `--wails-draggable` was inert, and
  with the native title bar hidden there was no fallback strip — dragging the
  header selected text instead. The runtime is now loaded, the list toolbar is
  a second drag handle, and drag regions suppress text selection.
- **Sync and cache coherence.** Comment-only edits on a wiki page reach the
  mirror (one `type=comment` query per space per pass); an unchanged page no
  longer bumps the version, so a quiet wiki stops invalidating the browser's
  bootstrap every 60 seconds; issue→page links are read from raw ADF, so link
  marks and inline cards count; a deleted issue is tombstoned by a single-item
  sync instead of lingering until the hourly reconcile; changelog fields are
  identified by id rather than a lower-cased localized name, so a Korean
  account records status transitions and reopen counts; and field discovery
  bumps the version it changes, so an open client stops 304-ing past new
  custom fields.
- **CLI and server honesty.** An unknown `--profile` errors with the list of
  real ones instead of minting an empty home; an empty `GADAK_*` variable no
  longer shadows its `SCRY_*` fallback; a leftover `~/.scry` beside `~/.gadak`
  says so rather than silently abandoning the old mirror; `install-service`
  writes one unit per profile and propagates a systemd failure; `team import`
  cannot leave views behind when the save fails; `gadak init` stores the
  account identity the web onboarding already stored; `views open` raises the
  window that belongs to the profile you asked for rather than whichever one
  was up; a workspace credentialed after `serve` started begins syncing; the
  attachment cache is keyed by site and issue, so a site switch cannot serve
  the wrong bytes and an unrelated issue key cannot fetch a cached one — and
  the snapshot importer writes under that same key, which it did not at first,
  leaving every seeded image unreachable on a profile with a site set; and a
  failed mirror re-read after an upload returns the 502 the contract specifies
  instead of a 200 that claims otherwise.
- **Person filters no longer depend on Jira email visibility** (#1, thanks
  @elppaaa). Assignee, reporter, current-user, and grouping filters prefer
  Jira account IDs while still accepting email-valued saved views when the
  issue or member directory retains that alias. Existing browser issue caches
  refresh once to receive the additive reporter ID; current caches and write
  metadata stay warm. Jira Cloud sites that hide `emailAddress` now keep
  their full people facets, and assigned issues no longer appear as
  unassigned merely because email is absent.
- **JQL in, JQL out.** Paste a Jira navigator URL or a dashboard `jql=`
  clause into the search box (or `gadak search --jql '…'` / the URL itself)
  and the matching chips apply against the mirror. **Copy JQL** on the filter
  bar is the way back into Jira. The subset is AND of `=`, `IN`, `IS EMPTY`,
  date comparisons, `text ~`, `currentUser()`, and `ORDER BY`. Everything
  else — WAS, sprint, cross-field OR, saved filter ids — is listed and never
  silently dropped. `POST /api/v1/issues/jql/` and `jql/emit/` are the same
  parser the CLI uses. Flags may sit on either side of the query
  (`gadak search --jql '…' --json`). The TUI is gone as of 0.12, so there is
  no TUI follow-up for this wave.
- **Claude usage is back on the README.** `gadak skill install` (or MCP),
  then a question Jira cannot answer, then the SQL the session actually runs.
- **Jira saved filters land in the sidebar.** Sync pulls the account's owned
  and starred filters (`GET /filter/my?includeFavourites=true`), compiles each
  JQL with the same subset as paste, and lists them under **Jira filters**.
  Schema **v18** adds `source_queries` for those rows. Dashboards stay in Jira
  — they are gadget layouts; the filters behind them come across when you own
  or star them. Partial JQL is listed, never dropped. Each row has an
  open-in-Jira control (`/issues/?filter=<id>`) — the desktop app takes it in
  the in-app browser; `gadak serve` opens a tab. Snapshots leave the table
  empty (site-specific names).
- **`gadak views`.** List Jira filters and saved views, `show` one, `open` it
  in the running desktop app or serve tab (`#/?pj=…&sc=…`), or `save` a JQL
  as a named view. `gadak view` is the same command. An agent can put the
  human on a filter without describing the chips. `--no-open` / `GADAK_NO_OPEN`
  writes the hash and leaves the window alone (tests, scripts). A named
  `--profile` is forwarded to Gadak.app so the file and the window match.
  The README agent clip is this loop: the command types, the list follows.

## v0.12.0 — 2026-08-13

- **Paper, not a dark dashboard.** The leftover scry look — glowing orb,
  near-black canvas, electric indigo — was a crystal ball wearing a new name.
  gadak is a strand (가닥): uncoated paper, sumi ink, one 쪽빛 thread. The mark
  is 가 drawn as two strokes — ㄱ the thread you follow, ㅏ the other one —
  not a typeset syllable. 16px favicon is just the ㄱ. Wordmarks, app icon,
  and OG card use the same drawing; the web UI follows the same tokens.
  The TUI is gone. It was a second product to keep in lockstep with the web UI,
  and the energy is better spent on one surface people actually live in.
- **Labels, on the list and on the issue.** The list used to fold every chip
  away once the detail panel opened, so a labelled issue read as unlabelled.
  One chip always stays. On the issue itself you can add and remove labels;
  `PUT /api/v1/issues/<key>/labels/` replaces the array, writes through to
  Jira, and re-reads the row. An empty array clears. Trim and de-dupe are
  server-side. There is no `gadak label` — use the UI or curl.
- **Labels on a selection.** The bulk bar (and `l`, same place as `s` / `a`)
  adds a label to every selected issue that does not already have it, or
  removes one that does. The list stays put. Skip / fail counts use the same
  toast as the other batch verbs.
- **Priority is a verb now.** The detail chip used to reprint a name. It
  opens the site catalog (`GET priorities/`) and writes
  `PUT /api/v1/issues/<key>/priority/` by id. `null` clears. Names are not
  accepted — Jira translates them per account. Team-managed projects that
  have no priority field fail at Jira, and the toast says so.
- **The title is editable.** Click it. Enter saves, Esc restores.
  `PUT /api/v1/issues/<key>/summary/` trims; empty and >255 runes are refused
  here so Jira never sees them.
- **Renamed to gadak.** The `scry` name collided with an existing enterprise
  company and a crowded search space. The binary, home directory (`~/.gadak`),
  env prefix (`GADAK_*`), MCP tools (`gadak_query` and friends), module path,
  and desktop bundle id all changed. An existing `~/.scry` tree and `scry.db`
  are renamed on first launch. `SCRY_*` environment variables are still read
  when the `GADAK_*` equivalent is unset. Team-share files still accept the
  old `scry_team_config` version key.
- **`gadak profiles` is an inventory now** — active marker, issue and document
  counts, last sync, and the site host (host only; never the URL, email or
  token), plus `--json`. There is deliberately no `switch`: the CLI writes to
  Jira, so the target stays in the command you ran (`--profile`) or in the
  shell you ran it from (`GADAK_PROFILE`), never in a file shared by every
  terminal. `skills/gadak/SKILL.md` states the rule for agents, which cannot see
  ambient state at all.
- **Workspaces work in the desktop app, and mounted mirrors now sync.** The
  sidebar's profile switcher was a `serve`-only feature: the app served neither
  `/w/<profile>/` nor `GET /api/v1/workspaces`, so the list came back empty and
  the section hid itself — indistinguishable from a broken feature. The routing
  moved to `internal/workspace`, shared by both. And the loop that keeps a
  mirror fresh used to run for the launch profile only, so anything you switched
  to was quietly stale; every profile with a credential now gets one.
  `--no-sync` still turns off all of them. Jira API volume scales with the
  number of configured profiles — see `docs/DESKTOP.md`.
- **Document lists no longer freeze on a large mirror.** All three Documents
  tabs, a space's flat list and its tree rendered one row per document, so
  opening the view or switching a tab rebuilt the whole mirror with the UI
  blocked. They are windowed now, like the issue list has always been. On a
  10,000-page mirror in the desktop app's WebKit: **4,433ms → 68ms**, 90,013
  DOM nodes → 249. Scrolling was never the slow part, which is why this read as
  a freeze rather than as slowness.
- **The perf fixture has documents.** It never did — `gadak snapshot` copies the
  issue axis only, so no budget could see the document lists, which is how the
  above shipped. New `docsTabSwitch` budget over a 5,000-page fixture. Note for
  anyone using `gadak snapshot` to share a mirror: it still drops pages and
  spaces, so what you hand over has no documents in it.
- **Desktop: the native title bar is gone.** It spent 32px of window height
  repeating a word the sidebar already shows. The window controls move into the
  sidebar's first row, which reserves their corner and drags the window; the
  same bundle served by `gadak serve` is unchanged (`config.desktop`, served only
  by the app, is what separates them).
- **`gadak skill install` — Claude Code skill without MCP.** Embeds
  `skills/gadak/SKILL.md` in the binary and installs it to
  `~/.claude/skills/gadak/` (or `./.claude/skills/gadak/` with `--project`, or
  `--dir <path>/gadak/`). Same content for brew users with no checkout.
  Identical file → already installed; differing content refuses unless
  `--force`. Prefer this when the agent has a shell; MCP remains for hosts
  that cannot spawn processes. See `docs/AGENT_SETUP.md`.
- **Desktop menu: Install Command Line Tool…** macOS **Tools** menu runs the
  same symlink install as `gadak install-cli` against the CLI inside the app
  bundle (`Contents/Resources/bin/gadak`) — no terminal, no sudo. Conflict
  offers Replace / Cancel; when the install dir is off PATH, the export
  one-liner is copied to the clipboard. See `docs/DESKTOP.md`.
- **`gadak install-cli` — put the running binary on PATH.** Shared
  `internal/clitool` package (CLI + desktop). Default dir prefers a PATH
  entry: `~/.local/bin` if present on PATH, else `/usr/local/bin` when
  writable, else `~/.local/bin` (no sudo; `--dir` / `--force` / `--print`).
  After a desktop-only install you can still run
  `/Applications/Gadak.app/Contents/Resources/bin/gadak install-cli`, or use
  the menu above. Warns when the install directory is not on `$PATH` and
  points at `gadak mcp install claude` next.

- **`gadak doctor` — redacted diagnostics for bug reports.** Prints versions,
  profile path (`~/…`), schema/migration level, mirror row counts, sync
  freshness (watermark presence + classified last error only), last-day
  `api_usage`, and Jira shape as counts (projects, custom-field mappings,
  Confluence spaces, status categories). No tokens, hostnames, emails,
  project keys, field names, or raw error text. Works with no mirror and no
  credential. `--json` for the same document. Paste into issues; see
  `SUPPORT.md` and the bug report template.
- **`gadak api` — raw Atlassian REST escape hatch.** Call any site-relative
  path with the stored credential when the mirror does not cover the
  endpoint (watchers, worklogs, sprints, user search, Confluence REST under
  `/wiki/`, …). Read (`GET`/`HEAD`) by default; other methods need
  `--write`. Absolute URLs are refused so the token cannot be aimed at a
  foreign host. Response body is printed unchanged; non-2xx still writes the
  body and exits 1. Usage counts flush into `api_usage`. CLI only — not on
  MCP. See `docs/AGENT_ACCESS.md` and `SECURITY.md`.

## v0.9.0 — 2026-08-06

- **The people axis.** Type a name in the ⌘K palette and a PEOPLE group
  appears; selecting opens a person panel — their recent comments across
  issues and wiki pages (`GET people/{author_id}/comments/`), plus one-click
  Assigned / Reported / Docs-by-author entries whose counts match what they
  open. Web-only this version (TUI.md says so).
- **Search says why it matched.** Every hit carries
  `matches[key] = {field: title|body|comment, snippet}` — in the API, `gadak
  search` (human and `--json`), and MCP `gadak_search`. The web UI shows the
  matched comment or body line with the query highlighted; highlighting went
  word-level to match how FTS actually matches. Comment search always worked —
  now it looks like it.
- **Page list excerpt (schema v15).** `pages.excerpt` — a one-line body preview
  (≤200 runes from ADF plain text) on every `PageLite`; shown on the web and
  TUI activity doc lists (navigation surfaces stay bare); backfilled from
  existing `body_adf` on migrate. The bundled demo's page bodies were
  anglicized where the new line looks (two Korean CJK-search anchors remain,
  below the fold).
- **A visual foundation.** A real type scale (8px retired), muted text at
  6.2:1, the accent reduced to links and what's yours (one screen measured
  29→1 accent nodes), dark-canvas overlay shadows that actually render, one
  monochrome icon family replacing every emoji in the chrome, and an avatar
  palette where red stays reserved for meaning.
- **One orb everywhere.** The wordmark's sphere sits on the x-height now
  (was 22% high — measured), gains a core-glow treatment, and every icon
  (favicon, app icon, dmg) derives from that same SVG. The crescent logo
  retires.
- **Geometry, not just color.** Every control lands on a two-step height grid
  (32px primary / 24px secondary — one toolbar row used to hold four heights),
  corner radius follows nesting depth, the two native selects wear the app's
  own chevron, and panel spacing sits on a 4/8/12/16/20 scale. Detail-panel
  headers are pinned by structure now (the old sticky header slid off after a
  screen of scrolling), consecutive comments by one author group under a
  single header with each continuation keeping a visible timestamp, and
  document body headings step 20/15/13 instead of hiding a 1px hierarchy.
  Every text glyph pretending to be an icon (✕ ✓ ›) became a real one.
- **The demo has more than one person in it.** The bundled snapshot's
  comments and reports were all Alex Kim's; they now spread across four
  personas with linked emails, so the people axis is explorable out of the
  box.

## v0.8.0 — 2026-08-06

- **Gadak.app — the macOS desktop app.** The web UI in its own signed,
  notarized window (`Gadak-<version>-arm64.dmg` on every release), with **no
  local server at all**: the window reaches the mirror in-process, so ports,
  addresses, and conflicts stop existing as UX. First launch runs the same
  in-window setup as the browser; a second launch focuses the running window.
  The bundle carries the CLI (`Contents/Resources/bin/gadak`) so a
  desktop-only install can still wire up an agent — see `docs/DESKTOP.md`.
- **Sync starts after in-app onboarding.** `serve` (and the app) began the
  background watch loop only when a credential existed at boot; finishing
  first-run setup now kicks it off without a restart.

## v0.7.0 — 2026-08-06

- **`gadak mcp install <client>`.** Pins the current profile and absolute binary
  path into an MCP host registration so clients that do not inherit shell env
  cannot silently attach to the default mirror. `claude` runs
  `claude mcp add` (or prints a manual command if the binary is missing);
  `cursor` / `codex` / `json` print paste-ready config; `--dry-run` prints
  without registering. See `docs/MCP.md` and `docs/AGENT_SETUP.md`.
- **Browser guard on the local API.** Reject cross-origin writes and
  DNS-rebinding reads so a page opened in the browser cannot drive the
  loopback mirror as an open proxy.
- **Space names (schema v14).** `spaces` table and `PageLite.space_name`;
  settings APIs to list Confluence spaces and persist `confluence.spaces`.
- **Docs UX wave.** Space names in the UI, unified recents, scope pickers, and
  a Recently edited view for mirrored pages — landing in a final recency-first
  shape: Viewed / Updated / By author tabs replace the sidebar space tree, in
  both the web UI and the TUI docs mode.
- **Epics built-in view.** The open backlog grouped by epic, one click from
  the sidebar.
- **Mirror file permissions.** The database and its WAL/SHM sidecars are
  chmodded to `0600` and data directories to `0700` on open; older installs
  are tightened the next time gadak opens them.
- **A face.** Wordmark, logo, and a favicon the app never had; the README
  leads with the live demo and a hero clip.
- **Demo speaks English.** The bundled snapshot's statuses, types, titles, and
  space homes read as English product data (Korean narrative pages remain for
  CJK search); page authors spread across five personas.
- **`docs/FAQ.md`.** The hard questions answered with receipts — site load
  math, admin visibility, single-maintainer risk, agent data exposure.
- **`gadak.localhost`.** `serve` opens `http://gadak.localhost` when the resolver
  maps it to loopback.
- **Port-conflict handling.** On a busy listen port, hand off to a running
  gadak or fall back to a free port instead of failing opaquely.
- **Keyboard triage.** Sprint cleanup from the keyboard without touching the
  mouse; TUI `s` aliases `t` for transition (parity with the web flow).
- **Freshness chip.** Surface the server↔Jira leg and pull the mirror on focus.
- **Warm-boot cache.** Chunked IndexedDB writes and an honest warm-boot metric
  for durable bootstrap.
- **Interaction performance gate.** Budget tests against a 10k-issue fixture.
- **TUI page-scroll keys.** Register page-scroll bindings in `keyMap` so help
  and docs match what the navigator actually does.
- Confluence sync hardened for real sites (team spaces by default, chunked
  CQL, quoted space keys, tolerated 404s).

## v0.6.0 — 2026-08-06

- **Confluence page labels (schema v13).** `pages.labels` (JSON array,
  alphabetical) collected via `expand=metadata.labels` on the page fetch and
  exposed on `PageLite` everywhere pages appear (list, detail, search). First
  label page only (≤25) — real pages carry single-digit label counts.
- **Epic hierarchy (schema v11/v12).** `issues.hierarchy_level` (source tree
  rank, backfilled from raw) and a derived `issues.epic_key` — the nearest
  level-1 ancestor via `parent_key`, recomputed after every upsert batch, so a
  sub-task groups under its epic rather than its story. `IssueLite` now carries
  `epic_key` and `parent_key` separately; the TUI supports `group_by=epic`
  (label `KEY summary`, `(no epic)` bucket); `issues_full` is rebuilt (v12) to
  expose the new columns; snapshots carry them.
- **Confluence page mirror.** Second source on the items spine — sync, pages
  API, unified search, and a demo snapshot that carries the Nimbus wiki beside
  the issue backlog.
- **Docs in the web UI.** Mirrored wiki pages as a sidebar tree, document
  panel, and unified search.
- **TUI docs navigator.** `D` toggles a space-grouped page tree.
- **Epic hierarchy in the web UI.** Group labels, row chips, breadcrumb, and
  rollup over the honest `epic_key`.
- **Mobile viewport.** Phones render the desktop layout instead of a squeezed
  column.

## v0.5.0 — 2026-08-05

- **Workspaces.** `serve` mounts every profile under `/w/<name>/`; the web UI
  workspace picker switches between mounted profile mirrors.
- **TUI neon look.** Ambient animation, mouse support, palette, and match
  highlight.
- **Search prefix match.** Bare terms prefix-match so inflected Korean (and
  similar morphology) is found.

## v0.4.0 — 2026-08-05

- **TUI custom-field edit.** Edit discovered custom fields with Jira-allowed
  values only; detail shows the discovered set.
- **Update notice.** Daily anonymous check on every surface, with opt-out.
- **Hosted-demo service-worker handshake.** Time out cleanly and say so when
  the browser cannot run the demo.

## v0.3.0 — 2026-08-05

- **Field auto-discovery.** The first full sync discovers and configures
  custom fields itself.
- **Filter axes from discovered fields.** Per-project scope; the detail panel
  renders whatever fields the site actually has, including multi-select
  `array<option>` editors.
- **Sync progress denominator.** Projects are optional on sync, and progress
  lines carry a real total.
- **Sync history.** Activity behind the sidebar timestamp.

## v0.2.1 — 2026-08-05

- Sign and notarize the macOS release binaries.
- Hosted demo: local write simulation that says the change was not saved, and
  copy that identifies the surface as a demo (no token prompt).

## v0.2.0 — 2026-08-05

- **Team config sharing.** `gadak team export` writes the views, field map,
  group rules and thresholds a team agrees on into one file to commit next to
  the code; `gadak team import` merges it into a profile (`--dry-run` prints the
  same plan the apply path runs, `--overwrite` replaces conflicts). Export is
  whitelist-only and a reflection test forces every new `Config` field to be
  classified as shareable or private. Credentials, account identity and
  per-machine preferences never travel; `members` ships only with
  `--with-members` because it carries email addresses. A file containing
  credential keys is refused on import rather than silently ignored.
- **Rate-limit visibility (schema v6).** The Jira client counts outbound
  attempts, 429s, 5xx, retries and backoff wait; each sync pass flushes them
  into `api_usage` (one row per UTC day). Shown in `gadak status`, `status
  --json`, `GET settings/` and the settings runtime panel — hidden while the
  count is zero. This is our own call volume, not Jira's remaining point
  budget, which the site does not expose. The retry policy itself is unchanged.
- **`gadak fields`.** Reports which custom fields are actually populated, by
  listing the site's fields and probing a stratified, deterministic sample of
  mirrored issues with `fields=*all`. Fields with real usage that are missing
  from `fieldMap` come with a paste-ready fragment; fields at zero are listed
  as the bloat. Not one SQL query over the mirror — the mirror only stores what
  `fieldMap` already names.
- **`gadak snapshot` (T6.4).** Builds a shareable copy by creating a fresh
  schema and copying rows into it, so dropped tables leave no residue.
  Personal state and `sync_state` counters stay behind. `--spread` restates
  timestamps across a window while preserving every issue's internal ordering,
  `--scale` clones issues onto new keys for benchmark fixtures, `--now` pins
  the clock for reproducible builds, and a credential scan runs before the file
  is published.
- **Per-command help.** Every subcommand answers `--help` with a summary, the
  real call shape including positional arguments, its flags, working examples
  and related commands, exiting 0. Flag lines are generated from the FlagSet so
  they cannot drift from the registration site.
- **TUI parity.** Feed focus tabs (`1`–`4`) with per-tab unread badges, and
  saved-view `sort` / `dir` / `group_by` support. Priority sorting keys on
  `priority_rank` rather than the localized priority name.
- Favorites live in the mirror (`GET/PUT/DELETE favorites/`) instead of only in
  browser storage, so `gadak sql` and agents can see them; the hosted demo,
  which has no writable API, falls back to local storage.
- Removed the `presence` client stack and its feature flag: the server has
  answered those endpoints with a permanent 404 since extraction, and the
  security model (one user, loopback) has no room for it.

- **Zero-install hosted demo (v0.3).** Static snapshot of `examples/demo.db`
  (bootstrap + 519 detail JSON + attachment bytes) served by a demo-only service
  worker on GitHub Pages — no binary, no account. `gadak export-static`,
  `make hosted-demo`, `e2e/hosted/`, `.github/workflows/pages.yml`. ADR 0004
  addendum: static JSON+SW instead of sqlite-wasm (client already boots from
  bootstrap JSON; FTS is client-side typing search for the demo).
- **Retention loop (v0.3).** `gadak serve` starts the sync watch loop by default
  when a credential is configured (`--no-sync` opts out; `--sync` kept as a
  deprecated alias). `gadak install-service` writes a launchd agent or systemd
  user unit for `serve --no-open`. After each successful watch cycle, one OS
  desktop notification may fire for new personal-feed events (macOS
  `osascript`, Linux `notify-send`; config `notify`, default true; body is the
  issue title only). Schema v5: `sync_state.first_sync_at`, `sync_count`,
  `last_notified_at`, and the `issues_full` view (`summary` + issues columns).
- Personal watch feed: `GET /api/v1/issues/feed/` and `POST …/feed/read/` compute
  activity from the mirror at query time (status/assignee/fields changelog,
  comments, attachments, issue creates) over a 30-day window, with local
  `feed_reads` receipts (schema v4). Relevance is watched · assignee · reporter ·
  mention; self-actions are excluded. `account_id` is stored on credential
  verify. `features.feed` defaults on. In-tab browser Notifications fire when
  unread grows (no VAPID/push).
- Ported the demo Jira seeder from Python to Go (`go run ./tools/seed-demo`),
  removing the last Python dependency. Flag contract, category-ladder
  transitions, and repair idempotency are unchanged.
- Extracted the web application from an internal deployment into this
  repository, replacing every hardcoded internal value with a runtime
  configuration document (`config.json`) fetched before mount.
- Generalized built-in views to axes that mean the same thing on every Jira site
  (`status_category`, `unassigned`, `reopened`, `stale`, `updated_from`),
  replacing presets that referenced internal project keys, status names, and team
  groupings.
- Replaced name-matching rules for resolution and reopen detection with status
  *category* rules, which are stable across sites and account languages. Dropped
  the internal `working_hours_in_status` field, which no code ever populated.
- Added `gadak serve`: serves the built UI, the runtime config document, and
  `/healthz`. Refuses to bind a non-loopback address without `--allow-remote`,
  because the mirror has no authentication.
- Added a Jira seeding tool for populating a throwaway Jira site with releases,
  components, issues, transition history, comments, and links — either generated
  or projected from a dataset file (`tools/seed-demo`).
- Specified the storage schema as a public contract, plus the HTTP, sync, and
  agent contracts under `specs/000-product/`.
- Implemented that schema in `internal/store`: SQLite (pure-Go driver, so the
  binary needs no cgo) with WAL, a migration runner keyed on `PRAGMA
  user_version` that refuses a database written by a newer gadak, an FTS5 index
  over titles, bodies and comment text, and the derived-field calculator
  (`status_changed_at`, `resolved_at`, `reopen_count`, `reopened_at`,
  `assignee_changed_at`, `comment_count`, `priority_rank`).
- Added `issues.reporter_email`, which the client filters and groups on but the
  first schema draft only had for the assignee.
- Schema additions over the first draft of `data-model.md`, all documented there:
  a `deleted_items` tombstone table so `delta` can report deletions,
  `contentless_delete=1` on `items_fts` so one row can be replaced,
  `items(synced_at)` and `issues(key)` indexes, and `ON DELETE CASCADE` from
  every child table to `items`. Corrected the first example query, which joined
  on a column the spine does not have.
