# Changelog

<sub>English · <a href="CHANGELOG.ko.md">한국어</a></sub>

## Unreleased

- A serve-scope pairing token now opens the whole mirror REST — everything
  the local web UI can call — instead of a 13-path allowlist. The origin
  passthrough stays origin-scope, and non-API paths stay behind the host
  guard. A leaked serve token cannot reach raw REST; a paired laptop cannot
  dump the mirror ([GDK-883]).
- `gadak serve` grew a terminal: one PTY session core behind
  `/api/v1/terminal/`, shared by the web pane, the desktop app, and a
  phone over the paired network — only renderers differ. Go owns the shell
  and the byte pump (a session is its own process group, so closing one
  takes its grandchildren with it), a 256 KiB ring replays scrollback to a
  client that reconnects inside a 60-second grace, and a slow client is
  dropped rather than allowed to stall the PTY or anyone else attached to
  it. Windows says so honestly — `ErrUnsupportedPlatform`, naming the
  ConPTY spike — instead of opening something that behaves unlike every
  other platform ([GDK-862]).
- A third pairing scope, and the sharpest one: `gadak pairing mint --scope
  terminal` opens a shell and nothing else, while a serve or origin token
  opens no shell at all — a leaked serve token leaks this workspace's
  data, a leaked terminal token leaks the machine. It is never a default,
  loopback still needs no token (an `--allow-remote` address does), and
  revoking it does not wait for the next request: the serve notices within
  a couple of seconds and closes the shells that token opened, telling the
  socket why ([GDK-863]).

## v0.17.3 — 2026-08-25

- The phone app grew from a skeleton into the whole MVP loop, built on
  two research rounds (market failure map of the official Jira mobile
  apps, and a measured gate table of what a phone could actually reach).
  The server side first: a paired DNS-named host now reads exactly a
  13-path mirror allowlist behind a new `serve` pairing scope — a one-way
  door, so a serve token cannot ride the origin passthrough and an origin
  token cannot dump the mirror — and `pairing mint` works on a connected
  workspace ([GDK-796], [GDK-797], [GDK-798]).
- On the phone itself: one native transport (no Origin, no CORS) with the
  token in the iOS Keychain behind three Rust commands; a quiet queue as
  the first screen — original status names with category ink, 44pt rows,
  stale-while-revalidate, Mine/All keyed on assignee ids; pairing that
  proves the connection it claims (QR scan, expired offers refuse to
  save, each failure teaches its own next move); search whose empty
  screen already answers (recent searches, saved-view chips that
  interpret only id axes locally); issue detail with a comment draft that
  outlives failure and a transition sheet speaking `transition_id`; and
  foreground feed polling promoted to local notifications only for
  assigned/mention/reopened — silent while the app is visible, badge off
  ([GDK-800], [GDK-801], [GDK-802], [GDK-799] app half).
- One navigation union wires it all (three tabs, Pair as onboarding, a
  notification tap lands on the issue and backs out to the queue) — and
  the wiring round measured a mount effect tracking its own completion,
  re-fetching ~55 times a second; `untrack` closed the class and the
  incident is on the backlog ([GDK-837]).
- The dimension-token wave: `ui.tokens` grows three palette-agnostic axes
  beside colors — `spacing`, `layout`, `type` — validated by ranges and
  cross-token relations instead of contrast math, with their own generated
  catalog chained into theme-check. app.css's leftover px forks converged
  onto `var()` consumption (a measured visual no-op), user overrides paint
  live — one `:root` rule after the palette cascade, a boot cache so a
  customized user never sees default geometry flash — and the JS geometry
  owners re-read the tokens in the same tick: the issue list dropped its
  42px constant for offset windowing (closing a 42/59 excerpt drift), every
  virtualized surface re-windows without a remount, and the docked floor
  recomputes and re-subscribes matchMedia ([GDK-842], [GDK-849], [GDK-850]).
- Setting one token no longer risks the rest: `ui.tokens.colors` /
  `.spacing` / `.layout` / `.type` are key-wise merge subpaths (a `null`
  deletes one key, a refused write leaves the config untouched), while the
  whole-object set keeps its replace semantics for compatibility — and the
  dimension names got their own discovery path,
  `gadak config get ui.tokens.dim-catalog`, listing tier, default, range
  and the relation sentences beside every token ([GDK-852], [GDK-853]).
- Dashboard links stopped destroying the wall: the frame→parent whitelist
  gains a second verb — `{type:'open', hash:'#/…'}` navigates the app
  itself to any of its own routes (issue detail, saved views, filtered
  lists, search), validated as a same-document fragment so it can never
  name an outside URL — and the sandbox now allows `target="_blank"
  rel="noopener"` so an external link opens a tab instead of navigating
  the frame away; the triage example and the agent skill teach both
  shapes ([GDK-854]).
- Your look is yours: token validation now **warns and saves** instead of
  refusing. Contrast, ΔEok, deuteranopia, ranges, cross-token relations
  and the locked tier all apply and say what they will look like — only
  what the machine cannot honor still refuses (a value that is not a hex
  or a length, a malformed shape, and `layout.docked-min`, which is
  recomputed from its three parts). The warnings gained the next move
  besides the diagnosis: a contrast line names the palettes that fail and
  points at `ui.tokensByTheme.<palette>`, and a type-step line prints the
  whole ladder that has to move together. Web writes carry the same
  verdicts back as `uiWarnings`, and the way out of any look is always one
  CLI line ([GDK-856], [GDK-857], [GDK-858]).

## v0.17.2 — 2026-08-25

- The color wave: a runtime token validator ported from theme-check with
  the app.css catalog as its single source ([GDK-785], [GDK-787]), and user
  color overrides end to end — `ui.tokens` / `ui.tokensByTheme` /
  `ui.dataColors` with catalog discovery (`ui.tokens.catalog`), write gates
  that refuse what the palettes themselves could not ship and teach the
  right key kind, live reflection in an open tab with no reload, and a boot
  cache that kills the palette flash ([GDK-786], [GDK-791]).

- An agent-authored dashboard is one HTML document plus registered
  datasources, saved like a view and rendered in the web tab inside a
  sandboxed frame. The host runs the queries (arbitrary SQL over a
  read-only mirror connection, or JQL) and pushes results in by
  postMessage; the frame's only verb back is `refresh`. Saves re-render an
  open tab in ≤1s and mirror deltas re-push data in ≤2s; uPlot ships
  embedded behind a fixed same-origin whitelist, so charts mean no CDN and
  no CSP widening ([GDK-781], [GDK-782], [GDK-792], [GDK-793]).

- Beyond uPlot, a dashboard can use any JavaScript library the *user*
  downloaded once: `gadak dashboards lib add <url>` fetches it (https
  only, ≤3 redirects each re-checked, ≤50 MiB), pins its sha384, and serves
  it from a local route that re-hashes the bytes on every request — a
  cache file modified after the add answers 500, never executes. Saves
  declare libs by id (`--lib`, ≤8); render injects them as deferred
  scripts and widens script-src with the local libs path only — the CSP
  still never names an external host, and an undeclared dashboard keeps
  the pre-808 policy byte for byte. three.js no longer ships embedded
  (−750 KB per binary); it is the documented `lib add` example instead
  ([GDK-808]).

- The phone app starts as a skeleton: Tauri v2 + Svelte in `mobile/`,
  sharing `web/src/app.css` so one token change retints both surfaces.
  Pairing and the queue screen are here; the server side is not — `pairing
  mint` still refuses a connected workspace and the pairing gate opens
  origin passthrough rather than the mirror REST the app reads
  ([GDK-797], [GDK-798] are the prerequisites). What is already load
  bearing is the offer decoder: Go and TypeScript read the same golden
  vectors, error strings included, so a one-sided change to the contract
  turns both suites red ([GDK-800]).

- A six-axis quality audit ran before this release, and the fixes landed
  in parallel chunks. In Go: flattening ADF to text has one owner, so the
  excerpt a list shows and the text FTS indexes can no longer drift
  ([GDK-814]); status categories live in their own package and the JQL
  layer stopped linking net/http ([GDK-686]); the write-vocabulary lock
  walks every origin interface instead of a hand-kept list ([GDK-687]); a
  parent-hint rejection is detected by error type, not by scanning the
  message ([GDK-819]); and create-source resolution has a single router
  shared by CLI and REST ([GDK-820]).

- The main column is one value now — the issue list, documents, a space,
  history, a dashboard or the feed — the same discriminated-union move
  the right panel made, so a surface can no longer forget to close a
  sibling and paint the list behind a dashboard ([GDK-815], [GDK-821]).
  A collapsed documents tree stays collapsed across sync reloads:
  expansion became deltas against a structural default instead of an
  absolute set an effect kept "fixing" ([GDK-817]). The dashboard joined
  the keyboard grammar — Esc closes it, the shortcut sheet says so, and
  loading gets the same skeleton grace as its siblings ([GDK-827]). One
  deliberate change rides along: the feed now takes the column instead
  of overlaying the document screen, so Esc from the feed lands on the
  list.

- The page id a read hands out is now the id a write accepts: TTY search
  prints the page key in the key column, `--json` help admits pages
  exist, the skill recipe emits the origin page id, and MCP search
  teaches `gadak_query` for page hits instead of a verb that dead-ends
  ([GDK-816]). The recents help stops overclaiming what it records
  ([GDK-828]).

- Fourteen error strings across three locales now name the next move,
  the mobile pairing error teaches `gadak pairing mint`, and Esc
  dismisses the top toast without stealing keys from editors or the
  media viewer ([GDK-828], [GDK-829]). The ko/ja catalogs converge on
  the vocabulary the rest of each locale already uses ([GDK-831]).

- Test weight moved down the ladder: the as-const arrays that own the
  grouping, sort and feed-focus unions also own their runtime guards
  ([GDK-825]), and six e2e specs became unit tests holding the same
  assertions ([GDK-826]).

- The audit's simplify chunk deleted 123 production lines nobody called —
  the og-image renderer, unused icon imports, test-only wrappers — with
  every test rewritten onto the real path instead of dropped ([GDK-822],
  [GDK-712]). Schema v38 drops the mirror-side saved_views/watches/
  favorites/feed_reads frozen by the v26 copy: an unprefixed table name
  in `gadak sql` now falls through to the ATTACHed local.db truth
  instead of a migration-time snapshot that answered quietly and wrong
  ([GDK-824]).

- Two places where the CLI took a typo and kept going. `gadak create GDK
  "…"` used to file an issue whose summary began with the project key,
  because the key was just another summary word; it is now refused with the
  `--project` spelling — but only when the first word is a key this
  workspace actually knows, so `create API timeout on sync` still files
  ([GDK-594]). And `config set projects` accepted any string: keys are now
  shape-checked and upper-cased at the one setter both the CLI and the
  settings endpoint pass through, `config set` asks the site and refuses a
  key that is not there (offline, it warns against the mirror and still
  saves), and `status` / `sync` name both sides of a scope that has drifted
  ([GDK-809]).

- The staleness warning names which source is stale. `warning: mirror last
  synced 154h ago` came from the *oldest* row across every synced source, so
  a quiet Confluence space made the whole mirror read as six days old while
  `status` showed a watermark ten minutes back — two surfaces, opposite
  stories, and no way to tell which to believe. It now reads `warning:
  confluence last synced 154h0m0s ago (synced_at 2026-08-18T03:13:21Z)`, and
  `status` prints the same `<source>.synced_at` strings in both text and
  JSON, so the two can be compared in one glance ([GDK-810]).

## v0.17.1 — 2026-08-24

The patch where the mirror learned to share. A day of dogfooding on a
20,000-issue mirror surfaced every way two gadak processes could wait on
one file, and the standalone record stopped being a YAML rewrite.

### The standalone record is a SQLite file

- The embedded tracker persists to `origin/issuetap.db` — real WAL SQLite,
  transactional per write — instead of rewriting the whole YAML on a
  debounce ([GDK-202]). An existing YAML seeds the database once and stays
  untouched as the rollback asset; export still speaks YAML.
- Backup is that file now: stop the app and copy it, or use
  `sqlite3 .backup` while it runs.

### Busy is a neighbor with a name

- A write that reached the origin no longer exits nonzero because the
  mirror re-read hit SQLITE_BUSY ([GDK-740]), and a busy refusal says who
  holds the profile — another gadak app, serve or CLI — instead of a bare
  error code; `gadak doctor` lists the holders ([GDK-754]).
- Visit and search history take their own local.db connection, so browsing
  no longer queues ten seconds behind a mirror writer ([GDK-753]), and MCP
  reads wait politely instead of failing instantly when a writer exists
  ([GDK-757]).
- Epic key recomputation scopes to the batch it touched instead of
  UPDATE-ing the whole table inside every upsert transaction — the
  ~140–850 ms per-page lock hold at 20k is gone; a full sync still sweeps
  once at the end ([GDK-755]).

### Hot paths, measured at 20k

- `gadak issue KEY` reads by key instead of loading the whole mirror
  ([GDK-747]); `search --jql` people resolution went narrow on the CLI and
  then on the server, which had kept the old path ([GDK-748], [GDK-756]);
  `doctor`'s raw customfield probe samples instead of scanning every
  document ([GDK-749]).

### The web at narrow widths

- An audit of every seam below 1100px ([GDK-758]) closed three clips: the
  stale-days chip (a Tailwind layer conflict — the hide now lives where it
  can win), trailing row columns that now fold in designed order instead
  of being cut mid-glyph by overflow, and a docked minimum that is the sum
  of its own tracks so the grid and the contract cannot drift ([GDK-766]).
  A CI probe keeps all three closed.

### Exclude is everywhere now

- Every visible filter axis takes NOT IN: a per-value ⊘ in the picker
  (Alt-click works too) replaced the modal Exclude toggle and the
  "No exclude" caption that had spread to every menu — the half-adoption
  [GDK-438] itself warned about ([GDK-771]). Copy JQL writes `not in` for
  the axes JQL can say and lists the mirror-only ones as omitted;
  `search --jql` compiles, resolves and matches the same negations, and
  `!=` now works wherever `not in` does.

### The site

- Landing media stopped being uniform full-screen video: two claims became
  core-region stills at recorded glyph scale, search plays cropped, and
  the flagship gained camera work as an ffmpeg post-process — the
  recording's pacing untouched ([GDK-751], [GDK-752]).
- A Korean browser is offered the Korean page — a suggestion strip, never
  a forced redirect, and it remembers your answer ([GDK-770]). Alongside:
  `llms.txt` for agents reading the site, an OG card that says what the
  page says, and a rate-limit row in the comparison table.

### Dashboards for agents


## v0.17.0 — 2026-08-23

The cycle where an agent's writes grew up. An issue now shows the PRs and
commits that implement it, the write verbs learned the vocabulary a coding
agent actually sends, and a workspace stopped being something you re-select
on every command. It closed with a pre-release audit across the network
seams, the MCP surface and the web UI.

### The development panel

- An issue knows its PRs, commits, deployments, builds and the people on
  them — mirrored on connected workspaces, writable on standalone ones
  ([GDK-496], [GDK-497], [GDK-592], [GDK-589]).
- `gadak dev scan` sweeps a repo's PRs into dev links in one pass, and
  `gadak dev link` writes one ([GDK-531], [GDK-538], [GDK-539]).
- The web renders a mirrored PR as a linked PR, opens a GitHub link in the
  in-app pane, and says why the panel is empty when it is ([GDK-495],
  [GDK-527], [GDK-555], [GDK-540]).
- Dev links survive the next sync, a fetch error, and a paired remote's
  incremental pull ([GDK-536], [GDK-541], [GDK-562], [GDK-537]).
- Linking is no longer a reason to leave the app: `gadak link A B --type
  blocks` and the detail panel both write issue links over one route
  ([GDK-19], [GDK-85]).

### Write verbs an agent can trust

- `create` and `edit` take `--field alias=value` for the custom fields a
  project requires, the create dialog learns what this project and type
  actually require, and `issue --editmeta` asks the origin what an issue can
  edit ([GDK-513], [GDK-254], [GDK-514]).
- `transition` carries `--resolution`, `--field` and `-m`; `edit` writes fix
  versions and components by name, keeping their ids ([GDK-509], [GDK-516],
  [GDK-517]).
- `assign` accepts a name or accountId beside an email ([GDK-515]).
- A write that reached the origin is a success even when the mirror could not
  be re-read right after ([GDK-740]).
- Bulk issue reads take many keys and `--keys -` with no silent drop, and
  REST gained the parent pair the CLI already had ([GDK-425], [GDK-328]).
- A rejected parent names the epics you could have picked, on both surfaces
  ([GDK-330], [GDK-635]).
- `gadak claim KEY` takes an issue as yours in one move, and `gadak issue`
  shows how long the work sat — `wait 3d · progress 5h`, computed at read
  time ([GDK-591]).
- A wrong-typed write field is a refusal, not an empty string ([GDK-643]).
- Sweeps behind those verbs: shared retry defaults, dead code proven
  unreferenced before deletion, one owner for the write-through re-read, and
  an adapter that refuses what it cannot do instead of stubbing it
  ([GDK-644], [GDK-647], [GDK-642], [GDK-641]).
- The two surfaces stopped drifting: the CLI no longer flattens a rich page
  body or files an issue outside the mirror, and one function answers "is
  this body rich" for both ([GDK-682], [GDK-666]).

### Standalone, and one vocabulary for origin writes

- A standalone workspace speaks your language, and a restricted issue is
  distinguishable from a public one ([GDK-597], [GDK-519]).
- A write carries its author: `GADAK_ACTOR`, a readable name for a nameless
  agent, and a badge marking bot work in the web ([GDK-586], [GDK-588],
  [GDK-593], [GDK-590]).
- `origin.Writer` stopped speaking Jira in its own signatures, so a verb
  added to one surface is not three implementations of the same guard
  ([GDK-665]).
- A write goes to the origin that owns the row: the two HTTP handlers that
  minted a Jira client regardless are routed now, with an AST gate against
  the next one ([GDK-681]).

### Workspaces and pairing

- `gadak workspace use <name>` stores a default, so a workspace is not
  re-selected on every command; `--workspace` is the name and `--profile`
  stays as its alias ([GDK-490]).
- Pairing tells the truth: tokens gate the origin passthrough, the status
  verbs learn what paired means, and a failure is not relabelled as a Jira
  401 ([GDK-433], [GDK-449], [GDK-453]).
- A bound workspace cannot be quietly rebound to a different site, and
  replacing an origin takes its derived rows with it ([GDK-452], [GDK-561],
  [GDK-418]).
- A mounted standalone workspace can create issues again ([GDK-677],
  [GDK-678]).

### The mirror's schema

- Fix versions keep their ids and the project's release catalog lands in the
  mirror; sprint is a column an agent can query ([GDK-532], [GDK-518]).
- Korean mid-compound search works: a `cjk_bigram` fourth FTS column finds
  the word inside the compound ([GDK-259], [GDK-444]).
- JQL `parent =` / `parent IN` filter on the mirror's own `parent_key`, and
  hierarchy survives the trip out ([GDK-521], [GDK-329]).
- Personal state moved out of the mirror, so `rm gadak.db` keeps your views,
  visits and search history ([GDK-105]).

### The web UI, made consistent

- One command registry instead of three surfaces that had to agree, and one
  object per UI string with every locale in it — including a full `ja`
  catalog ([GDK-674], [GDK-668], [GDK-626]).
- Esc closes the surface it was aimed at and nothing else, and the six modal
  dialogs have one owner ([GDK-617], [GDK-604], [GDK-316]).
- Four type sizes and nothing between them; "you are here" is an attribute
  rather than a paint colour; empty is a state, not a blank ([GDK-129],
  [GDK-613], [GDK-130]).
- A transition that needs a screen asks inline, components and parent edit in
  place, and a story shows its children ([GDK-83], [GDK-86], [GDK-121]).
- There is one kind of saved view, because there is no team: Enter saves, the
  server owns it when there is one, and views already in a browser move there
  ([GDK-437]).
- The empty Documents body reads the same six causes the sidebar reads, so it
  stops prescribing a switch that is already on ([GDK-738]).
- A favourite's time column holds a time or nothing, and "All" is a tab filter
  rather than also a verb ([GDK-739]).
- The demo fixture can no longer silently lag the schema ([GDK-671]).
- A derivation is derived rather than repaired by an effect, in the link
  dialog, the create dialog and the sources tab ([GDK-692]).
- The Fields tab stopped drawing a field-mapping editor that could never hold
  anything, and `gadak config set fieldMap` refuses instead of planting a key
  the next load erases ([GDK-710]).
- Enter means one thing in all three narrowing fields — take what I typed to
  the fuller search — and the widen has one owner ([GDK-727]).
- Every phrase has one key and every concept one word, so a button reads the
  same wherever it is drawn ([GDK-730], [GDK-731], [GDK-735], [GDK-736]).
- A read that finishes quickly paints no skeleton at all: the anti-flash delay
  has one owner and all six read paths use it ([GDK-737]).

### MCP and the CLI surface

- Batch writes answer with an honest per-key envelope, and closing an issue
  is one round trip that is free to retry ([GDK-501], [GDK-500]).
- Picking work is a verb: `gadak pick` ([GDK-503]).
- CLI reads leave a trail `gadak recents` walks back ([GDK-502]).
- MCP keeps its own mirror fresh, and `gadak sync --if-stale 15m` is the
  session opener an agent can call blind ([GDK-599], [GDK-598]).

### Desktop

- A second launch raises the first window instead of starting a rival, on
  standalone too ([GDK-658]).
- `gadak://` links work on Windows, `install-cli` speaks Windows, Ctrl+W
  stops promising what it cannot do, and Windows no longer claims it notified
  you ([GDK-350], [GDK-353], [GDK-351], [GDK-349]).
- The wails pin moves to `v3.0.0-beta.12` ([GDK-639]).
- The Windows window carries the app menu the app builds ([GDK-700]).

### Network seams, audited

- The Linear proxy fetches only `uploads.linear.app`, redirects included, and
  the API key rides only there ([GDK-427], [GDK-558], [GDK-560]).
- An empty host is a non-loopback bind, so `serve` demands `--allow-remote`
  for it like any other exposure ([GDK-542]).
- The boot sequence has one owner, shared by `gadak serve` and the desktop
  app ([GDK-664]).
- A Linear-only workspace is a configured workspace, and Linear's rate limit
  is a retry rather than a death ([GDK-654], [GDK-263]).
- The Web Push client is gone: it called endpoints the server pins at 404,
  and vendor push services are outbound traffic this project does not make
  ([GDK-711]).

### The public backlog, and a front door at gadak.dev

- gadak's backlog is published, ships as one file, and publishes what each
  issue actually says ([GDK-389], [GDK-669], [GDK-430]).
- A GDK key on a public surface is never a dangling pointer, its link target
  must itself be published, and the gate reads the files on disk rather than
  only the tracked ones ([GDK-269], [GDK-675], [GDK-683]).
- The site has an apex with three doors behind it — landing, demo, backlog
  ([GDK-676]).
- The Windows warning has a page that explains it ([GDK-211]).

### Under the gates

- The e2e listen port has one owner, so two worktrees can run the browser
  suite at once ([GDK-672]).
- The hosted site builds the SPA once: `basePath()` reads its mount at
  runtime instead of a compile-time constant, and the gate hashes both asset
  trees ([GDK-673]).
- The e2e fixture is fresh or it is not — `local.db` is seeded with the
  mirror, so a saved view stops surviving into the next run ([GDK-742]).
- Agent onboarding is skill-first, and the repository is a Claude Code plugin
  marketplace ([GDK-8], [GDK-93]).
- An origin's "I cannot do that" is a value: a capability refusal is a 400
  with the origin's own sentence, not 502 `jira_unavailable`, and a transition
  refusal no longer forges a Jira HTTP error to get there ([GDK-685]).

## v0.16.1 — 2026-08-20

The release that finishes what 0.16 started. Standalone shipped as a working
origin and then spent a day proving how many ways two processes can disagree
about who owns a file; Linear arrived as a read-only client with nowhere to
plug in; and the documents an agent actually reads never learned the word
"standalone". All three are closed here.

### A third tracker, and it writes

- Linear is a source, not a plan: a `"linear"` block and `gadak sync --source
  linear` mirror issues, comments, labels and attachments, and writes route by
  the row's origin — what Linear cannot do yet refuses honestly
  ([GDK-263], [GDK-360], [GDK-361]).
- Jira, standalone and Linear speak one write vocabulary ([GDK-359]), and a
  Linear-only profile syncs; a cross-source key collision no longer refuses a
  Jira write ([GDK-361], [GDK-263]).

### The wiki stops being read-only

- Page edit, comment and create write through the origin, and page ids get
  the same namespace mirror ids got ([GDK-380], [GDK-381], [GDK-382],
  [GDK-344]).

### One owner for the standalone origin

- The desktop app advertises its origin too, so an app and a CLI cannot both
  hold the persist file ([GDK-340], [GDK-333]).
- The persist lock is who may embed, an acknowledged write is on disk before
  the response, a durable persist failure fails the write, and the fatal path
  flushes too ([GDK-343], [GDK-342], [GDK-346], [GDK-348]).
- Standalone failures stop masquerading as `credential_required`, and the
  conversion copy says what conversion actually does ([GDK-345], [GDK-347]).
- Mirror ids get their own namespace and conversion drops the old mirror
  whole, taking watches and favorites with it ([GDK-241], [GDK-344]).

### The agent surfaces learn standalone

- The embedded skill knows standalone exists, the CLI says which origin it
  means, and `transition` names each target's `status_id` — and accepts the
  one the read path hands out ([GDK-239], [GDK-363], [GDK-364], [GDK-366],
  [GDK-371], [GDK-365], [GDK-313]).
- Kind and persist are on the agent's preflight, standalone `init` fills the
  mirror, and `issues_full` exposes `description_text` ([GDK-368], [GDK-376],
  [GDK-367], [GDK-312]).

### Documents that stop contradicting the product

- `AGENTS.md` is the repository's development contract now, not a product
  manual ([GDK-8]).
- The network gets its own page, and the stale-warning verb list learned that
  `sql` warns too ([GDK-601], [GDK-598]).
- The install front door admits standalone; CONCEPT teaches origins, the FAQ
  stops offering `rm -rf ~/.gadak`, PROMISES gains a ninth promise,
  MAINTENANCE stops refusing a Windows shell it shipped, and export/import
  finally has its paragraph; the Go test suite stopped pointing a fixture
  credential at a live Atlassian host ([GDK-271], [GDK-372], [GDK-373],
  [GDK-374], [GDK-375], [GDK-304]).

### The second audit, and what it already fixed

- A second six-axis audit swept the codebase before the tag; two defects it
  caught shipped the same day ([GDK-603], [GDK-602], [GDK-604]).
- One owner each for SQL comment stripping, private directories, view names,
  `ray develop` output, and a fatal error ([GDK-605], [GDK-606], [GDK-612],
  [GDK-615], [GDK-611]).
- The test suite stops sleeping through refused dials, `gofmt` is a CI gate,
  a bad cursor is an identity, counting issues stopped loading them, dead
  code is deleted with its callers counted first, and six small helper copies
  collapsed to one owner each ([GDK-608], [GDK-607], [GDK-609], [GDK-610],
  [GDK-616], [GDK-619]).

### The pre-tag audit, closed before the tag

- Three audit rounds over this release's own delta filed 30 findings; the
  ones that survived verification are fixed here ([GDK-393]).
- Linear's mirror carries what the origin has, and Linear's writes speak
  Linear's vocabulary — a key two sources both mint is an explicit
  `key_ambiguous` refusal ([GDK-394], [GDK-396], [GDK-400]).
- Wiki writes are honest under failure ([GDK-408]).
- Standalone conversion has one owner: a busy lock names the holder's pid,
  and `gadak project create` grows a standalone workspace through the origin
  ([GDK-415], [GDK-421], [GDK-391]).
- `gadak_status` reports workspace kind, the top-level usage owns every
  command, a rejected `create --parent` explains the hierarchy rule, and the
  ko docs admit wiki writes exist ([GDK-420], [GDK-426], [GDK-424],
  [GDK-409]).

## v0.16.0 — 2026-08-19

The release where gadak stops needing an Atlassian account to be useful,
stops needing a Mac to run, and stops being read-only about the fields
people actually triage by. A workspace can now be standalone — its origin
is a minimal self-hosted Jira that travels with the app — and the issues
in any workspace, connected or standalone, can finally be edited where they
are read: due date, description, priority, assignee, and the custom fields
your site actually uses.

### A workspace without an Atlassian account

- A workspace can be standalone: its origin runs in-process, the mirror stays
  a disposable cache, and every write goes through the origin ([GDK-183]).
- A workspace is bound to one origin — connecting a credential cannot quietly
  retarget it ([GDK-238], [GDK-247]).
- Standalone wikis write through the origin's Confluence API, and the UI
  stops asking a standalone workspace for a token ([GDK-267], [GDK-237]).

### Windows and Linux

- Windows gets a portable pack and an installer path, `install-cli` that
  works there, and a first-launch fix for `gadak://` applied twice; a Scoop
  manifest is checkable without Windows ([GDK-209], [GDK-293], [GDK-246]).
- Linux gets a pack script symmetric with the dmg one, a tarball install
  next to brew, and an AUR packaging kit with a gate against version drift
  ([GDK-208], [GDK-229], [GDK-115]).
- Omarchy gets a bar widget for what changed in *your* mirror, plus an
  install recipe verified on a real guest ([GDK-116], [GDK-225]).

### Issues you can edit where you read them

- Field editors leave the QA cage: editability is the issue's own editmeta,
  and allowed values come from one place ([GDK-322], [GDK-323]).
- A due date is set and cleared from the detail panel ([GDK-223]); dates got
  a read surface first, and Jira's refusal of a bad due date is a sentence
  you can read ([GDK-249], [GDK-250], [GDK-251]).
- Descriptions edit as plain text, with a format-loss guard before anything
  rich is destroyed ([GDK-82]).
- `p` opens a catalog priority menu wherever `s`/`a`/`l` already work, and
  the list's assignee menu finds the same people the detail picker does
  ([GDK-331], [GDK-332]).
- The palette can create an issue from the typed text, required create fields
  with obvious answers stop being questions, naming an action wins in every
  locale, and posting a comment finally says it landed ([GDK-217], [GDK-218],
  [GDK-302], [GDK-300], [GDK-301]).

### The demo, and the door

- The hosted demo opens on the product, and a Settings About tab and a macOS
  Help menu carry the same four feedback channels ([GDK-335], [GDK-336]).

### Updates, without an updater

- Update detection reaches the UI and says the right thing per platform —
  notify-only, no self-update; release notes render in the app, and upgrade
  instructions have one owner ([GDK-213], [GDK-214], [GDK-215], [GDK-216]).

### Groups

- `team_group` is classified by one read-only query over the mirror when the
  derived view is rebuilt, not on a keystroke; empty is unclassified, and
  `groupRules` stays the three-list form.

### Linear, measured before it is wired

- A read-only Linear GraphQL client landed as groundwork, deliberately not
  wired into workspaces yet ([GDK-263], [GDK-274], [GDK-258], [GDK-261]).

### The audit wave

- Localized names stop being keys: status, priority and type key on ids and
  categories everywhere ([GDK-275], [GDK-272], [GDK-248], [GDK-161]).
- One cold open stopped serialising everybody; a contended write died
  instantly because `busy_timeout` never saw it; a background sync no longer
  outlives the server that started it ([GDK-282], [GDK-305], [GDK-270]).
- Six dialogs share one shell contract; onboarding owns its pane; the Korean
  catalogue no longer disagrees with itself inside one header row
  ([GDK-297], [GDK-299], [GDK-298]).
- The mirror's downgrade notice got a ceiling and advice, the wiki write
  surface that was built but never wired got wired, and the Linear client
  detects a dead credential ([GDK-310], [GDK-267], [GDK-263], [GDK-274]).
- CI stopped lying about infrastructure: a stalled apt mirror, a retry that
  killed apt mid-configure, and an orphaned root apt-get holding the lock
  each fail fast and say which half failed ([GDK-308], [GDK-317], [GDK-319]).

### For agents

- Dogfooding friction is a backlog item, not something to route around; the
  FAQ's claim that agents cannot write the mirror matched the code again;
  CJK mid-compound search is app-layer bigrams ([GDK-312], [GDK-313],
  [GDK-314], [GDK-315], [GDK-306], [GDK-259]).

## v0.15.2 — 2026-08-17

The release where settings stop being a screen. Every field the dialog edits
is a CLI verb, so an agent can set up a workspace end to end — and the first
thing that travels that way is the look.

### Settings are an agent surface

- `gadak config list | get | set` is one path→validate table behind both the
  CLI and settings PUT, so the two can never disagree ([GDK-193]).
- Themes live in `config.json`, which is a per-workspace file: picking a
  theme in the UI and setting it from a terminal are the same act
  ([GDK-190]).

### Three darks, and one of them is yours

- `dark` is a neutral-cool charcoal now; `ink` is a new blue-black palette;
  `ember` preserves the previous warm dark byte for byte ([GDK-190]).

### Smaller things that were in the way

- A bare number finds the issue in any project, on every search surface
  ([GDK-186]).
- Settings → Integrations (desktop only) lists the surfaces gadak installs
  into with four-way truth, and the menu stops installing ([GDK-185],
  [GDK-189]).
- The ⌘K palette is never blank: an empty query shows recently updated
  issues under recently viewed, plus saved views ([GDK-184], [GDK-191]).
- The settings dialog stops repeating its This mirror block above every tab
  ([GDK-188]).

## v0.15.1 — 2026-08-17

- `gadak raycast install` embeds the Raycast extension and registers it, so
  a brew or app-bundle install does not need a checkout ([GDK-182]).
- The ⌘K palette home is never blank: an empty query shows recently updated
  issues under recently viewed, plus saved views ([GDK-184]).
- Settings → Integrations (desktop only) lists the agent surfaces gadak
  installs into, with honest detection and a live log whose verdict is the
  stream's final `exit=` line ([GDK-185]).

## v0.15.0 — 2026-08-17

The release that opens gadak outward. A view or an issue is now a link any
app can hand over, search is fast enough to drive somebody else's UI
keystroke by keystroke, and Raycast gets a documented way in. Inside, a dark
theme built to the same paper-and-ink standard as the light one — and the
first run of a new ritual: a full-codebase audit before every minor.

### A gadak is now an address

- `gadak://` deep links: a piece of gadak travels as a link instead of a
  shell command, and the grammar carries no verb and no payload ([GDK-119]).
- Every place has a name in the URL — nine place params in one reviewed
  registry ([GDK-124]).
- Raycast, both doors: MCP install prints the form values, and the local
  search a Raycast extension would sit on measures under a "feels instant"
  budget ([GDK-117]).
- The product produces the links it consumes: a copy-link action, `gadak
  issue KEY --link`, and a querystring without `#/` that used to boot the
  default view now lands where it pointed ([GDK-163], [GDK-164]).
- An issue can name its parent: `gadak create --parent` and `gadak edit
  --parent` write the sub-issue relationship through Jira ([GDK-19],
  [GDK-86]).
- Typing an issue key finds that issue, and search is fast enough to sit
  under someone else's keystroke — 20k-mirror worst case 1.6s → 110ms
  ([GDK-170], [GDK-166]).

### A dark theme, and a place for the next one

- Dark: warm ground, ink foregrounds, the same paper metaphor as light, no
  flash on first paint, and adding a third theme is now one definition block
  ([GDK-154], [GDK-156], [GDK-162]).
- Success and failure stop being told by colour alone ([GDK-158]).
- Both palettes clear the same measured floors: status inks separate in
  normal and deuteranopic vision, and the search highlight gets its own
  token ([GDK-157], [GDK-159], [GDK-171]).

### The list behaves like a AAA list

- The right side of a row is a column you can scan, and the last row stops
  being cut in half ([GDK-128], [GDK-131]).
- Esc closes what it looks at, a covering panel declares itself below
  1440 px, and one concept gets one Korean word ([GDK-132], [GDK-133],
  [GDK-127], [GDK-135]).
- A half-composed syllable is not a query ([GDK-169]).

### Honesty at the edges

- A hosted snapshot no longer advertises verbs it cannot answer ([GDK-52]).
- The legacy field mapping retires itself, and on a read-only home that
  rewrite is a warning, not a refusal to start ([GDK-149], [GDK-173]).
- Copy means copied, an attachment is fetched at most once, and the desktop
  app stops loading its runtime twice ([GDK-178], [GDK-177], [GDK-150]).

### The audit, and what it deleted

- First run of the per-minor full-codebase audit: eighteen findings fixed,
  the rest labeled `carryover-v0.15` ([GDK-125]).
- Timestamps get one owner, view-param keys become the type, and Svelte
  hygiene drops dead exports ([GDK-148], [GDK-147], [GDK-152]).
- Sixteen browser tests become units, the Go suite stops sleeping on wall
  clocks, and untested pure modules get real cases ([GDK-145], [GDK-144],
  [GDK-146]).
- Derived-field semantics get a single home whose SQL examples a test
  executes, and chosung search retires product-wide ([GDK-88], [GDK-89],
  [GDK-168]).

## v0.14.2 — 2026-08-16

The release about the first ten minutes and the day the token dies. Nothing
here is a new capability so much as an existing one that finally tells you
what it is doing.

- Every token trap is named before the paste, not after the 401
  ([GDK-69], [GDK-98]).
- A rejected token is recoverable without writing ([GDK-68]).
- Picking no projects is a choice ([GDK-99]).
- `gadak skill install` treats an upgrade as an upgrade, and the embedded
  skill knows the verbs the CLI has ([GDK-92], [GDK-91]).
- A quiet Confluence tick reads zero page bodies ([GDK-113]).
- `gadak issue <KEY> --derive` prints how the derived columns were computed
  ([GDK-111]).
- History keeps its order ([GDK-26]).
- Token expiry is warned about before the sync dies, the browse pane yields
  Escape, `gadak sql` warns on a stale mirror, `Open` repairs an `items_fts`
  this build cannot write, search-help `?` works on touch, `examples/compose`
  lands as pure shell, the Datasette Lite deep link is pinned, and
  `PROMISES.md` is gated against `SECURITY.md` ([GDK-67], [GDK-78], [GDK-90],
  [GDK-112], [GDK-53], [GDK-109], [GDK-101], [GDK-104]).
- The Node version has one owner a shell can read, and `tools/ci-status.sh`
  answers whether what you just pushed passed ([GDK-57]).

## v0.14.1 — 2026-08-15

One day of dogfooding gadak's own backlog through gadak, shipped as it
landed: the first CLI write verbs, a demo that finally works where people
actually tap it, and the removal of an updater that had never earned trust.

- The macOS app is notify-only again: the never-exercised in-app self-updater
  is gone, and v0.14.1 ships no `gadak-desktop-darwin-<arch>.zip`
  ([GDK-58], [GDK-61]).
- The first write verbs: `gadak create`, `gadak attach`, `gadak edit`.
- The hosted demo works inside in-app browsers, and the first paint is a
  static frame readable at phone width ([GDK-23], [GDK-51]).
- The browse pane yields, and boot keystrokes are held, not dropped
  ([GDK-76], [GDK-46]).
- Failures say what happened: a truncated key list says how many, and a
  rejected credential stops the watch loop for every source ([GDK-35],
  [GDK-24], [GDK-48]).
- Priority colors read the rank, not the account's language.
- MCP tools stop scanning the whole mirror per call; a web unit tier of
  100+ specs runs in ~300ms on every push.

## v0.14.0 — 2026-08-15

The maintainer-review release: seven builders of loved developer tools were
asked, per lens, why gadak would or would not be loved — and every confirmed
finding either shipped or got a bar written down. The theme is trust:
surfaces that fail loudly instead of silently, docs that match the code, and
measured numbers instead of adjectives.

- The first agent call succeeds, or says why not: `gadak_search`'s primary
  argument is `query`, every tool error starts with `ERROR:` and echoes the
  keys it received, and `gadak_issue` over the response cap sheds oldest
  comments and says `truncated`.
- The pipe is a promise now: `issues_full` + the RECIPES queries, `gadak sql`
  stdout, and `views open --keys -`.
- `gadak export` / `gadak import` round-trip the rows you would actually
  miss — saved views, watches, favorites — with no credentials and no site
  URL.
- Measured, with the losing rows: a live-site benchmark against a
  2,853-issue Cloud project (42× on a simple filter, 162× on the epic
  GROUP BY, reopen count ~20 minutes over REST vs 14.5 ms locally).
- The settings dialog stops lying about empty project selection, a cut
  web-push toggle, and config that already reloads on the next tick.
- The hosted demo lands on Epic breakdown.
- `brew install midagedev/tap/gadak` is the app now; `gadak-cli` is the
  CLI-only formula.
- Docs told the truth again, including a Korean README, and a repo
  `CLAUDE.md` so every session starts from the same contract.

## v0.13.0 — 2026-08-14

The release that puts search, history and the agent's window in one place.
⌘K searches the whole mirror, what you opened lives in a file the mirror
cannot take with it, and `gadak views open --keys -` puts an agent's answer
on the running window.

- One search box that searches everything: ⌘K queries every issue and
  document in one FTS index, ignoring the filter chips on the list, and the
  box above the list keeps its old job ("narrow this list").
- History lives in `~/.gadak/local.db`, beside the mirror: issues, documents
  and searches on one timeline, and an agent can join visits to issues in a
  single `gadak sql`.
- The issue list stops losing to the document screen: "show me the list" has
  one owner.
- The window follows the agent: a `keys` axis makes an arbitrary set of
  issue keys a first-class view; `gadak views open` opens *in gadak*,
  `gadak open` leaves for Jira.
- MCP gains a fifth tool, `gadak_show`, so a host without a shell can present
  too.
- Confluence space scope is now real: each space carries its own watermark,
  a newly selected space backfills in full, and every successful pass removes
  the spaces that left the scope.
- The account-id bug class is closed, not patched: people resolve to account
  ids across JQL, saved views, filters and the member directory; changelog
  and attachment authors gain `author_id`.
- A profile name could no longer escape the home directory, and the browser
  guard wraps the whole mux.
- The macOS window can be dragged (#2, thanks @wafe).
- Sync and cache coherence: comment-only wiki edits reach the mirror, an
  unchanged page no longer bumps the version, issue→page links are read from
  raw ADF, a deleted issue is tombstoned by a single-item sync, and changelog
  fields are identified by id rather than a lower-cased localized name.
- CLI and server honesty: an unknown `--profile` errors with the real list,
  an empty `GADAK_*` variable no longer shadows its `SCRY_*` fallback, the
  attachment cache is keyed by site and issue, and a failed mirror re-read
  after an upload returns the 502 the contract specifies.
- Person filters no longer depend on Jira email visibility (#1, thanks
  @elppaaa).
- JQL in, JQL out: paste a Jira navigator URL or a `jql=` clause and the
  matching chips apply; Copy JQL is the way back; the unsupported subset is
  listed and never silently dropped.
- Jira saved filters land in the sidebar, and `gadak views` lists, shows,
  opens and saves them.
- Claude usage is back on the README.

## v0.12.0 — 2026-08-13

The look-and-rename release. gadak is a strand (가닥): uncoated paper, sumi
ink, one 쪽빛 thread — and the leftover crystal-ball dashboard is gone. The
same cut puts labels and priority on the issue, workspaces in the desktop
app, and the CLI verbs an agent actually needs.

- Paper, not a dark dashboard. The mark is 가 drawn as two strokes. The TUI
  is gone.
- Labels stay visible on the list, edit on the issue, and apply to a
  selection from the bulk bar (`l`, same place as `s` / `a`).
- Priority is a verb now: the chip opens the site catalog and writes by id;
  names are not accepted.
- The title is editable.
- Renamed to gadak: binary, home directory (`~/.gadak`), env prefix
  (`GADAK_*`), MCP tools, module path and desktop bundle id; an existing
  `~/.scry` tree is renamed on first launch, and `SCRY_*` is still read when
  the `GADAK_*` equivalent is unset.
- `gadak profiles` is an inventory now; there is deliberately no `switch`.
- Workspaces work in the desktop app, and every profile with a credential
  gets a sync loop.
- Document lists no longer freeze on a large mirror (4,433ms → 68ms on a
  10,000-page window).
- The native title bar is gone; window controls move into the sidebar's
  first row.
- `gadak skill install` embeds the Claude Code skill without MCP; a desktop
  menu installs the CLI; `gadak install-cli` puts the running binary on PATH.
- `gadak doctor` prints redacted diagnostics for bug reports; `gadak api` is
  the raw Atlassian REST escape hatch, refused at a foreign host.

## v0.9.0 — 2026-08-06

The people-and-visual-foundation release. A name in ⌘K opens a person, every
search hit says why it matched, and the chrome finally sits on one type
scale and one orb.

- The people axis: type a name in ⌘K and a person panel opens — web-only
  this version.
- Search says why it matched, with a snippet of the field that hit.
- Page list excerpt: a one-line body preview on every page.
- A visual foundation: a real type scale, muted text at 6.2:1, one
  monochrome icon family, and an avatar palette where red stays reserved for
  meaning.
- One orb everywhere: the wordmark's sphere sits on the x-height, and every
  icon derives from that same drawing; the crescent logo retires.
- Geometry, not just color: a two-step height grid, corner radius that
  follows nesting, pinned detail-panel headers, and consecutive comments by
  one author grouped under a single header.
- The demo has more than one person in it.

## v0.8.0 — 2026-08-06

- Gadak.app — the macOS desktop app: the web UI in its own signed, notarized
  window, with no local server at all; a second launch focuses the running
  window, and the bundle carries the CLI.
- Sync starts after in-app onboarding, without a restart.

## v0.7.0 — 2026-08-06

The release that pins an agent to the right mirror and puts a face on the
product. MCP install writes the current profile into the host, the local API
stops being an open proxy, and the README leads with the live demo.

- `gadak mcp install <client>` pins the current profile and absolute binary
  path into an MCP host registration.
- Browser guard on the local API: reject cross-origin writes and
  DNS-rebinding reads.
- Space names, a docs UX wave (Viewed / Updated / By author), and an epics
  built-in view.
- Mirror file permissions tighten to `0600` / `0700` on open.
- A face: wordmark, logo, and a favicon the app never had.
- The demo speaks English (Korean narrative pages remain for CJK search).
- `docs/FAQ.md` answers the hard questions with receipts.
- `serve` opens `http://gadak.localhost` when the resolver maps it; a busy
  listen port hands off to a running gadak or falls back to a free port.
- Keyboard triage, a freshness chip, a warm-boot cache, and an interaction
  performance gate against a 10k-issue fixture.
- Confluence sync hardened for real sites.

## v0.6.0 — 2026-08-06

The wiki release. Confluence pages join the items spine, show up in the web
UI and the TUI, and issues grow an honest epic hierarchy.

- Confluence page labels, collected on fetch and shown everywhere pages
  appear.
- Epic hierarchy: a derived `epic_key` — the nearest level-1 ancestor — so a
  sub-task groups under its epic rather than its story.
- Confluence page mirror: second source on the items spine, with docs in the
  web UI and a TUI docs navigator (`D`).
- Epic hierarchy in the web UI: group labels, row chips, breadcrumb, and
  rollup.
- Phones render the desktop layout instead of a squeezed column.

## v0.5.0 — 2026-08-05

- Workspaces: `serve` mounts every profile under `/w/<name>/`.
- TUI neon look: ambient animation, mouse support, palette, and match
  highlight.
- Search prefix match, so inflected Korean is found.

## v0.4.0 — 2026-08-05

- TUI custom-field edit, with Jira-allowed values only.
- Update notice: daily anonymous check on every surface, with opt-out.
- Hosted-demo service-worker handshake: time out cleanly and say so when the
  browser cannot run the demo.

## v0.3.0 — 2026-08-05

- Field auto-discovery: the first full sync discovers and configures custom
  fields itself.
- Filter axes from discovered fields, including multi-select editors.
- Sync progress carries a real total; projects are optional on sync.
- Sync history behind the sidebar timestamp.

## v0.2.1 — 2026-08-05

- Sign and notarize the macOS release binaries.
- Hosted demo: local write simulation that says the change was not saved,
  and copy that identifies the surface as a demo.

## v0.2.0 — 2026-08-05

Team config sharing, a zero-install hosted demo, a personal watch feed,
and the storage schema plus the HTTP, sync and agent contracts.

- Team config sharing: `gadak team export` / `import` writes the views,
  field map, group rules and thresholds a team agrees on; credentials never
  travel, and a file containing credential keys is refused on import.
- Rate-limit visibility: our own call volume, shown in `gadak status` and
  the settings runtime panel, hidden while the count is zero.
- `gadak fields` reports which custom fields are actually populated.
- `gadak snapshot` builds a shareable copy; `--spread` restates timestamps
  across a window while preserving every issue's internal ordering, `--scale`
  clones issues onto new keys, `--now` pins the clock, and a credential scan
  runs before the file is published.
- Per-command help, generated from the FlagSet so it cannot drift.
- TUI parity: feed focus tabs, saved-view sort/dir/group_by, and priority
  sorting keyed on `priority_rank`.
- Favorites live in the mirror, so `gadak sql` and agents can see them; the
  hosted demo falls back to local storage.
- The `presence` client stack is gone.
- Zero-install hosted demo: a static snapshot served by a demo-only service
  worker — no binary, no account.
- Retention loop: `gadak serve` starts the sync watch loop by default when a
  credential is configured; `gadak install-service` writes a launchd agent or
  systemd user unit; one OS desktop notification may fire for new personal
  feed events.
- Personal watch feed, computed from the mirror at query time over a 30-day
  window.
- The demo Jira seeder moved from Python to Go; the web application was
  extracted from an internal deployment into this repository.
- Built-in views key on axes that mean the same thing on every Jira site;
  resolution and reopen detection key on status *category*, not a localized
  name.
- `gadak serve` serves the built UI and refuses a non-loopback bind without
  `--allow-remote`.
- The storage schema, plus HTTP, sync and agent contracts, and the SQLite
  implementation with WAL, FTS5, and the derived-field calculator.

[GDK-8]: https://gadak.dev/backlog/#/?ks=GDK-8
[GDK-19]: https://gadak.dev/backlog/#/?ks=GDK-19
[GDK-23]: https://gadak.dev/backlog/#/?ks=GDK-23
[GDK-24]: https://gadak.dev/backlog/#/?ks=GDK-24
[GDK-26]: https://gadak.dev/backlog/#/?ks=GDK-26
[GDK-35]: https://gadak.dev/backlog/#/?ks=GDK-35
[GDK-46]: https://gadak.dev/backlog/#/?ks=GDK-46
[GDK-48]: https://gadak.dev/backlog/#/?ks=GDK-48
[GDK-51]: https://gadak.dev/backlog/#/?ks=GDK-51
[GDK-52]: https://gadak.dev/backlog/#/?ks=GDK-52
[GDK-53]: https://gadak.dev/backlog/#/?ks=GDK-53
[GDK-57]: https://gadak.dev/backlog/#/?ks=GDK-57
[GDK-58]: https://gadak.dev/backlog/#/?ks=GDK-58
[GDK-61]: https://gadak.dev/backlog/#/?ks=GDK-61
[GDK-67]: https://gadak.dev/backlog/#/?ks=GDK-67
[GDK-68]: https://gadak.dev/backlog/#/?ks=GDK-68
[GDK-69]: https://gadak.dev/backlog/#/?ks=GDK-69
[GDK-76]: https://gadak.dev/backlog/#/?ks=GDK-76
[GDK-78]: https://gadak.dev/backlog/#/?ks=GDK-78
[GDK-82]: https://gadak.dev/backlog/#/?ks=GDK-82
[GDK-83]: https://gadak.dev/backlog/#/?ks=GDK-83
[GDK-85]: https://gadak.dev/backlog/#/?ks=GDK-85
[GDK-86]: https://gadak.dev/backlog/#/?ks=GDK-86
[GDK-88]: https://gadak.dev/backlog/#/?ks=GDK-88
[GDK-89]: https://gadak.dev/backlog/#/?ks=GDK-89
[GDK-90]: https://gadak.dev/backlog/#/?ks=GDK-90
[GDK-91]: https://gadak.dev/backlog/#/?ks=GDK-91
[GDK-92]: https://gadak.dev/backlog/#/?ks=GDK-92
[GDK-93]: https://gadak.dev/backlog/#/?ks=GDK-93
[GDK-98]: https://gadak.dev/backlog/#/?ks=GDK-98
[GDK-99]: https://gadak.dev/backlog/#/?ks=GDK-99
[GDK-101]: https://gadak.dev/backlog/#/?ks=GDK-101
[GDK-104]: https://gadak.dev/backlog/#/?ks=GDK-104
[GDK-105]: https://gadak.dev/backlog/#/?ks=GDK-105
[GDK-109]: https://gadak.dev/backlog/#/?ks=GDK-109
[GDK-111]: https://gadak.dev/backlog/#/?ks=GDK-111
[GDK-112]: https://gadak.dev/backlog/#/?ks=GDK-112
[GDK-113]: https://gadak.dev/backlog/#/?ks=GDK-113
[GDK-115]: https://gadak.dev/backlog/#/?ks=GDK-115
[GDK-116]: https://gadak.dev/backlog/#/?ks=GDK-116
[GDK-117]: https://gadak.dev/backlog/#/?ks=GDK-117
[GDK-119]: https://gadak.dev/backlog/#/?ks=GDK-119
[GDK-121]: https://gadak.dev/backlog/#/?ks=GDK-121
[GDK-124]: https://gadak.dev/backlog/#/?ks=GDK-124
[GDK-125]: https://gadak.dev/backlog/#/?ks=GDK-125
[GDK-127]: https://gadak.dev/backlog/#/?ks=GDK-127
[GDK-128]: https://gadak.dev/backlog/#/?ks=GDK-128
[GDK-129]: https://gadak.dev/backlog/#/?ks=GDK-129
[GDK-130]: https://gadak.dev/backlog/#/?ks=GDK-130
[GDK-131]: https://gadak.dev/backlog/#/?ks=GDK-131
[GDK-132]: https://gadak.dev/backlog/#/?ks=GDK-132
[GDK-133]: https://gadak.dev/backlog/#/?ks=GDK-133
[GDK-135]: https://gadak.dev/backlog/#/?ks=GDK-135
[GDK-144]: https://gadak.dev/backlog/#/?ks=GDK-144
[GDK-145]: https://gadak.dev/backlog/#/?ks=GDK-145
[GDK-146]: https://gadak.dev/backlog/#/?ks=GDK-146
[GDK-147]: https://gadak.dev/backlog/#/?ks=GDK-147
[GDK-148]: https://gadak.dev/backlog/#/?ks=GDK-148
[GDK-149]: https://gadak.dev/backlog/#/?ks=GDK-149
[GDK-150]: https://gadak.dev/backlog/#/?ks=GDK-150
[GDK-152]: https://gadak.dev/backlog/#/?ks=GDK-152
[GDK-154]: https://gadak.dev/backlog/#/?ks=GDK-154
[GDK-156]: https://gadak.dev/backlog/#/?ks=GDK-156
[GDK-157]: https://gadak.dev/backlog/#/?ks=GDK-157
[GDK-158]: https://gadak.dev/backlog/#/?ks=GDK-158
[GDK-159]: https://gadak.dev/backlog/#/?ks=GDK-159
[GDK-161]: https://gadak.dev/backlog/#/?ks=GDK-161
[GDK-162]: https://gadak.dev/backlog/#/?ks=GDK-162
[GDK-163]: https://gadak.dev/backlog/#/?ks=GDK-163
[GDK-164]: https://gadak.dev/backlog/#/?ks=GDK-164
[GDK-166]: https://gadak.dev/backlog/#/?ks=GDK-166
[GDK-168]: https://gadak.dev/backlog/#/?ks=GDK-168
[GDK-169]: https://gadak.dev/backlog/#/?ks=GDK-169
[GDK-170]: https://gadak.dev/backlog/#/?ks=GDK-170
[GDK-171]: https://gadak.dev/backlog/#/?ks=GDK-171
[GDK-173]: https://gadak.dev/backlog/#/?ks=GDK-173
[GDK-177]: https://gadak.dev/backlog/#/?ks=GDK-177
[GDK-178]: https://gadak.dev/backlog/#/?ks=GDK-178
[GDK-182]: https://gadak.dev/backlog/#/?ks=GDK-182
[GDK-183]: https://gadak.dev/backlog/#/?ks=GDK-183
[GDK-184]: https://gadak.dev/backlog/#/?ks=GDK-184
[GDK-185]: https://gadak.dev/backlog/#/?ks=GDK-185
[GDK-186]: https://gadak.dev/backlog/#/?ks=GDK-186
[GDK-188]: https://gadak.dev/backlog/#/?ks=GDK-188
[GDK-189]: https://gadak.dev/backlog/#/?ks=GDK-189
[GDK-190]: https://gadak.dev/backlog/#/?ks=GDK-190
[GDK-191]: https://gadak.dev/backlog/#/?ks=GDK-191
[GDK-193]: https://gadak.dev/backlog/#/?ks=GDK-193
[GDK-208]: https://gadak.dev/backlog/#/?ks=GDK-208
[GDK-209]: https://gadak.dev/backlog/#/?ks=GDK-209
[GDK-211]: https://gadak.dev/backlog/#/?ks=GDK-211
[GDK-213]: https://gadak.dev/backlog/#/?ks=GDK-213
[GDK-214]: https://gadak.dev/backlog/#/?ks=GDK-214
[GDK-215]: https://gadak.dev/backlog/#/?ks=GDK-215
[GDK-216]: https://gadak.dev/backlog/#/?ks=GDK-216
[GDK-217]: https://gadak.dev/backlog/#/?ks=GDK-217
[GDK-218]: https://gadak.dev/backlog/#/?ks=GDK-218
[GDK-223]: https://gadak.dev/backlog/#/?ks=GDK-223
[GDK-225]: https://gadak.dev/backlog/#/?ks=GDK-225
[GDK-229]: https://gadak.dev/backlog/#/?ks=GDK-229
[GDK-237]: https://gadak.dev/backlog/#/?ks=GDK-237
[GDK-238]: https://gadak.dev/backlog/#/?ks=GDK-238
[GDK-239]: https://gadak.dev/backlog/#/?ks=GDK-239
[GDK-241]: https://gadak.dev/backlog/#/?ks=GDK-241
[GDK-246]: https://gadak.dev/backlog/#/?ks=GDK-246
[GDK-247]: https://gadak.dev/backlog/#/?ks=GDK-247
[GDK-248]: https://gadak.dev/backlog/#/?ks=GDK-248
[GDK-249]: https://gadak.dev/backlog/#/?ks=GDK-249
[GDK-250]: https://gadak.dev/backlog/#/?ks=GDK-250
[GDK-251]: https://gadak.dev/backlog/#/?ks=GDK-251
[GDK-254]: https://gadak.dev/backlog/#/?ks=GDK-254
[GDK-258]: https://gadak.dev/backlog/#/?ks=GDK-258
[GDK-259]: https://gadak.dev/backlog/#/?ks=GDK-259
[GDK-261]: https://gadak.dev/backlog/#/?ks=GDK-261
[GDK-263]: https://gadak.dev/backlog/#/?ks=GDK-263
[GDK-267]: https://gadak.dev/backlog/#/?ks=GDK-267
[GDK-269]: https://gadak.dev/backlog/#/?ks=GDK-269
[GDK-270]: https://gadak.dev/backlog/#/?ks=GDK-270
[GDK-271]: https://gadak.dev/backlog/#/?ks=GDK-271
[GDK-272]: https://gadak.dev/backlog/#/?ks=GDK-272
[GDK-274]: https://gadak.dev/backlog/#/?ks=GDK-274
[GDK-275]: https://gadak.dev/backlog/#/?ks=GDK-275
[GDK-282]: https://gadak.dev/backlog/#/?ks=GDK-282
[GDK-293]: https://gadak.dev/backlog/#/?ks=GDK-293
[GDK-297]: https://gadak.dev/backlog/#/?ks=GDK-297
[GDK-298]: https://gadak.dev/backlog/#/?ks=GDK-298
[GDK-299]: https://gadak.dev/backlog/#/?ks=GDK-299
[GDK-300]: https://gadak.dev/backlog/#/?ks=GDK-300
[GDK-301]: https://gadak.dev/backlog/#/?ks=GDK-301
[GDK-302]: https://gadak.dev/backlog/#/?ks=GDK-302
[GDK-304]: https://gadak.dev/backlog/#/?ks=GDK-304
[GDK-305]: https://gadak.dev/backlog/#/?ks=GDK-305
[GDK-306]: https://gadak.dev/backlog/#/?ks=GDK-306
[GDK-308]: https://gadak.dev/backlog/#/?ks=GDK-308
[GDK-310]: https://gadak.dev/backlog/#/?ks=GDK-310
[GDK-312]: https://gadak.dev/backlog/#/?ks=GDK-312
[GDK-313]: https://gadak.dev/backlog/#/?ks=GDK-313
[GDK-314]: https://gadak.dev/backlog/#/?ks=GDK-314
[GDK-315]: https://gadak.dev/backlog/#/?ks=GDK-315
[GDK-316]: https://gadak.dev/backlog/#/?ks=GDK-316
[GDK-317]: https://gadak.dev/backlog/#/?ks=GDK-317
[GDK-319]: https://gadak.dev/backlog/#/?ks=GDK-319
[GDK-322]: https://gadak.dev/backlog/#/?ks=GDK-322
[GDK-323]: https://gadak.dev/backlog/#/?ks=GDK-323
[GDK-328]: https://gadak.dev/backlog/#/?ks=GDK-328
[GDK-329]: https://gadak.dev/backlog/#/?ks=GDK-329
[GDK-330]: https://gadak.dev/backlog/#/?ks=GDK-330
[GDK-331]: https://gadak.dev/backlog/#/?ks=GDK-331
[GDK-332]: https://gadak.dev/backlog/#/?ks=GDK-332
[GDK-333]: https://gadak.dev/backlog/#/?ks=GDK-333
[GDK-335]: https://gadak.dev/backlog/#/?ks=GDK-335
[GDK-336]: https://gadak.dev/backlog/#/?ks=GDK-336
[GDK-340]: https://gadak.dev/backlog/#/?ks=GDK-340
[GDK-342]: https://gadak.dev/backlog/#/?ks=GDK-342
[GDK-343]: https://gadak.dev/backlog/#/?ks=GDK-343
[GDK-344]: https://gadak.dev/backlog/#/?ks=GDK-344
[GDK-345]: https://gadak.dev/backlog/#/?ks=GDK-345
[GDK-346]: https://gadak.dev/backlog/#/?ks=GDK-346
[GDK-347]: https://gadak.dev/backlog/#/?ks=GDK-347
[GDK-348]: https://gadak.dev/backlog/#/?ks=GDK-348
[GDK-349]: https://gadak.dev/backlog/#/?ks=GDK-349
[GDK-350]: https://gadak.dev/backlog/#/?ks=GDK-350
[GDK-351]: https://gadak.dev/backlog/#/?ks=GDK-351
[GDK-353]: https://gadak.dev/backlog/#/?ks=GDK-353
[GDK-359]: https://gadak.dev/backlog/#/?ks=GDK-359
[GDK-360]: https://gadak.dev/backlog/#/?ks=GDK-360
[GDK-361]: https://gadak.dev/backlog/#/?ks=GDK-361
[GDK-363]: https://gadak.dev/backlog/#/?ks=GDK-363
[GDK-364]: https://gadak.dev/backlog/#/?ks=GDK-364
[GDK-365]: https://gadak.dev/backlog/#/?ks=GDK-365
[GDK-366]: https://gadak.dev/backlog/#/?ks=GDK-366
[GDK-367]: https://gadak.dev/backlog/#/?ks=GDK-367
[GDK-368]: https://gadak.dev/backlog/#/?ks=GDK-368
[GDK-371]: https://gadak.dev/backlog/#/?ks=GDK-371
[GDK-372]: https://gadak.dev/backlog/#/?ks=GDK-372
[GDK-373]: https://gadak.dev/backlog/#/?ks=GDK-373
[GDK-374]: https://gadak.dev/backlog/#/?ks=GDK-374
[GDK-375]: https://gadak.dev/backlog/#/?ks=GDK-375
[GDK-376]: https://gadak.dev/backlog/#/?ks=GDK-376
[GDK-380]: https://gadak.dev/backlog/#/?ks=GDK-380
[GDK-381]: https://gadak.dev/backlog/#/?ks=GDK-381
[GDK-382]: https://gadak.dev/backlog/#/?ks=GDK-382
[GDK-389]: https://gadak.dev/backlog/#/?ks=GDK-389
[GDK-391]: https://gadak.dev/backlog/#/?ks=GDK-391
[GDK-393]: https://gadak.dev/backlog/#/?ks=GDK-393
[GDK-394]: https://gadak.dev/backlog/#/?ks=GDK-394
[GDK-396]: https://gadak.dev/backlog/#/?ks=GDK-396
[GDK-400]: https://gadak.dev/backlog/#/?ks=GDK-400
[GDK-408]: https://gadak.dev/backlog/#/?ks=GDK-408
[GDK-409]: https://gadak.dev/backlog/#/?ks=GDK-409
[GDK-415]: https://gadak.dev/backlog/#/?ks=GDK-415
[GDK-418]: https://gadak.dev/backlog/#/?ks=GDK-418
[GDK-420]: https://gadak.dev/backlog/#/?ks=GDK-420
[GDK-421]: https://gadak.dev/backlog/#/?ks=GDK-421
[GDK-424]: https://gadak.dev/backlog/#/?ks=GDK-424
[GDK-425]: https://gadak.dev/backlog/#/?ks=GDK-425
[GDK-426]: https://gadak.dev/backlog/#/?ks=GDK-426
[GDK-427]: https://gadak.dev/backlog/#/?ks=GDK-427
[GDK-430]: https://gadak.dev/backlog/#/?ks=GDK-430
[GDK-433]: https://gadak.dev/backlog/#/?ks=GDK-433
[GDK-437]: https://gadak.dev/backlog/#/?ks=GDK-437
[GDK-444]: https://gadak.dev/backlog/#/?ks=GDK-444
[GDK-449]: https://gadak.dev/backlog/#/?ks=GDK-449
[GDK-452]: https://gadak.dev/backlog/#/?ks=GDK-452
[GDK-453]: https://gadak.dev/backlog/#/?ks=GDK-453
[GDK-490]: https://gadak.dev/backlog/#/?ks=GDK-490
[GDK-495]: https://gadak.dev/backlog/#/?ks=GDK-495
[GDK-496]: https://gadak.dev/backlog/#/?ks=GDK-496
[GDK-497]: https://gadak.dev/backlog/#/?ks=GDK-497
[GDK-500]: https://gadak.dev/backlog/#/?ks=GDK-500
[GDK-501]: https://gadak.dev/backlog/#/?ks=GDK-501
[GDK-502]: https://gadak.dev/backlog/#/?ks=GDK-502
[GDK-503]: https://gadak.dev/backlog/#/?ks=GDK-503
[GDK-509]: https://gadak.dev/backlog/#/?ks=GDK-509
[GDK-513]: https://gadak.dev/backlog/#/?ks=GDK-513
[GDK-514]: https://gadak.dev/backlog/#/?ks=GDK-514
[GDK-515]: https://gadak.dev/backlog/#/?ks=GDK-515
[GDK-516]: https://gadak.dev/backlog/#/?ks=GDK-516
[GDK-517]: https://gadak.dev/backlog/#/?ks=GDK-517
[GDK-518]: https://gadak.dev/backlog/#/?ks=GDK-518
[GDK-519]: https://gadak.dev/backlog/#/?ks=GDK-519
[GDK-521]: https://gadak.dev/backlog/#/?ks=GDK-521
[GDK-527]: https://gadak.dev/backlog/#/?ks=GDK-527
[GDK-531]: https://gadak.dev/backlog/#/?ks=GDK-531
[GDK-532]: https://gadak.dev/backlog/#/?ks=GDK-532
[GDK-536]: https://gadak.dev/backlog/#/?ks=GDK-536
[GDK-537]: https://gadak.dev/backlog/#/?ks=GDK-537
[GDK-538]: https://gadak.dev/backlog/#/?ks=GDK-538
[GDK-539]: https://gadak.dev/backlog/#/?ks=GDK-539
[GDK-540]: https://gadak.dev/backlog/#/?ks=GDK-540
[GDK-541]: https://gadak.dev/backlog/#/?ks=GDK-541
[GDK-542]: https://gadak.dev/backlog/#/?ks=GDK-542
[GDK-555]: https://gadak.dev/backlog/#/?ks=GDK-555
[GDK-558]: https://gadak.dev/backlog/#/?ks=GDK-558
[GDK-560]: https://gadak.dev/backlog/#/?ks=GDK-560
[GDK-561]: https://gadak.dev/backlog/#/?ks=GDK-561
[GDK-562]: https://gadak.dev/backlog/#/?ks=GDK-562
[GDK-586]: https://gadak.dev/backlog/#/?ks=GDK-586
[GDK-588]: https://gadak.dev/backlog/#/?ks=GDK-588
[GDK-589]: https://gadak.dev/backlog/#/?ks=GDK-589
[GDK-590]: https://gadak.dev/backlog/#/?ks=GDK-590
[GDK-591]: https://gadak.dev/backlog/#/?ks=GDK-591
[GDK-592]: https://gadak.dev/backlog/#/?ks=GDK-592
[GDK-593]: https://gadak.dev/backlog/#/?ks=GDK-593
[GDK-597]: https://gadak.dev/backlog/#/?ks=GDK-597
[GDK-598]: https://gadak.dev/backlog/#/?ks=GDK-598
[GDK-599]: https://gadak.dev/backlog/#/?ks=GDK-599
[GDK-601]: https://gadak.dev/backlog/#/?ks=GDK-601
[GDK-602]: https://gadak.dev/backlog/#/?ks=GDK-602
[GDK-603]: https://gadak.dev/backlog/#/?ks=GDK-603
[GDK-604]: https://gadak.dev/backlog/#/?ks=GDK-604
[GDK-605]: https://gadak.dev/backlog/#/?ks=GDK-605
[GDK-606]: https://gadak.dev/backlog/#/?ks=GDK-606
[GDK-607]: https://gadak.dev/backlog/#/?ks=GDK-607
[GDK-608]: https://gadak.dev/backlog/#/?ks=GDK-608
[GDK-609]: https://gadak.dev/backlog/#/?ks=GDK-609
[GDK-610]: https://gadak.dev/backlog/#/?ks=GDK-610
[GDK-611]: https://gadak.dev/backlog/#/?ks=GDK-611
[GDK-612]: https://gadak.dev/backlog/#/?ks=GDK-612
[GDK-613]: https://gadak.dev/backlog/#/?ks=GDK-613
[GDK-615]: https://gadak.dev/backlog/#/?ks=GDK-615
[GDK-616]: https://gadak.dev/backlog/#/?ks=GDK-616
[GDK-617]: https://gadak.dev/backlog/#/?ks=GDK-617
[GDK-619]: https://gadak.dev/backlog/#/?ks=GDK-619
[GDK-626]: https://gadak.dev/backlog/#/?ks=GDK-626
[GDK-635]: https://gadak.dev/backlog/#/?ks=GDK-635
[GDK-639]: https://gadak.dev/backlog/#/?ks=GDK-639
[GDK-641]: https://gadak.dev/backlog/#/?ks=GDK-641
[GDK-642]: https://gadak.dev/backlog/#/?ks=GDK-642
[GDK-643]: https://gadak.dev/backlog/#/?ks=GDK-643
[GDK-644]: https://gadak.dev/backlog/#/?ks=GDK-644
[GDK-647]: https://gadak.dev/backlog/#/?ks=GDK-647
[GDK-654]: https://gadak.dev/backlog/#/?ks=GDK-654
[GDK-658]: https://gadak.dev/backlog/#/?ks=GDK-658
[GDK-664]: https://gadak.dev/backlog/#/?ks=GDK-664
[GDK-665]: https://gadak.dev/backlog/#/?ks=GDK-665
[GDK-666]: https://gadak.dev/backlog/#/?ks=GDK-666
[GDK-668]: https://gadak.dev/backlog/#/?ks=GDK-668
[GDK-669]: https://gadak.dev/backlog/#/?ks=GDK-669
[GDK-671]: https://gadak.dev/backlog/#/?ks=GDK-671
[GDK-672]: https://gadak.dev/backlog/#/?ks=GDK-672
[GDK-673]: https://gadak.dev/backlog/#/?ks=GDK-673
[GDK-674]: https://gadak.dev/backlog/#/?ks=GDK-674
[GDK-675]: https://gadak.dev/backlog/#/?ks=GDK-675
[GDK-676]: https://gadak.dev/backlog/#/?ks=GDK-676
[GDK-677]: https://gadak.dev/backlog/#/?ks=GDK-677
[GDK-678]: https://gadak.dev/backlog/#/?ks=GDK-678
[GDK-681]: https://gadak.dev/backlog/#/?ks=GDK-681
[GDK-682]: https://gadak.dev/backlog/#/?ks=GDK-682
[GDK-683]: https://gadak.dev/backlog/#/?ks=GDK-683
[GDK-685]: https://gadak.dev/backlog/#/?ks=GDK-685
[GDK-692]: https://gadak.dev/backlog/#/?ks=GDK-692
[GDK-710]: https://gadak.dev/backlog/#/?ks=GDK-710
[GDK-711]: https://gadak.dev/backlog/#/?ks=GDK-711
[GDK-727]: https://gadak.dev/backlog/#/?ks=GDK-727
[GDK-738]: https://gadak.dev/backlog/#/?ks=GDK-738
[GDK-739]: https://gadak.dev/backlog/#/?ks=GDK-739
[GDK-740]: https://gadak.dev/backlog/#/?ks=GDK-740
[GDK-737]: https://gadak.dev/backlog/#/?ks=GDK-737
[GDK-736]: https://gadak.dev/backlog/#/?ks=GDK-736
[GDK-735]: https://gadak.dev/backlog/#/?ks=GDK-735
[GDK-731]: https://gadak.dev/backlog/#/?ks=GDK-731
[GDK-730]: https://gadak.dev/backlog/#/?ks=GDK-730
[GDK-700]: https://gadak.dev/backlog/#/?ks=GDK-700
[GDK-742]: https://gadak.dev/backlog/#/?ks=GDK-742
[GDK-438]: https://gadak.dev/backlog/#/?ks=GDK-438
[GDK-771]: https://gadak.dev/backlog/#/?ks=GDK-771
[GDK-202]: https://gadak.dev/backlog/#/?ks=GDK-202
[GDK-747]: https://gadak.dev/backlog/#/?ks=GDK-747
[GDK-748]: https://gadak.dev/backlog/#/?ks=GDK-748
[GDK-749]: https://gadak.dev/backlog/#/?ks=GDK-749
[GDK-751]: https://gadak.dev/backlog/#/?ks=GDK-751
[GDK-752]: https://gadak.dev/backlog/#/?ks=GDK-752
[GDK-753]: https://gadak.dev/backlog/#/?ks=GDK-753
[GDK-754]: https://gadak.dev/backlog/#/?ks=GDK-754
[GDK-755]: https://gadak.dev/backlog/#/?ks=GDK-755
[GDK-756]: https://gadak.dev/backlog/#/?ks=GDK-756
[GDK-757]: https://gadak.dev/backlog/#/?ks=GDK-757
[GDK-758]: https://gadak.dev/backlog/#/?ks=GDK-758
[GDK-766]: https://gadak.dev/backlog/#/?ks=GDK-766
[GDK-770]: https://gadak.dev/backlog/#/?ks=GDK-770
[GDK-785]: https://gadak.dev/backlog/#/?ks=GDK-785
[GDK-786]: https://gadak.dev/backlog/#/?ks=GDK-786
[GDK-787]: https://gadak.dev/backlog/#/?ks=GDK-787
[GDK-791]: https://gadak.dev/backlog/#/?ks=GDK-791
[GDK-781]: https://gadak.dev/backlog/#/?ks=GDK-781
[GDK-782]: https://gadak.dev/backlog/#/?ks=GDK-782
[GDK-792]: https://gadak.dev/backlog/#/?ks=GDK-792
[GDK-793]: https://gadak.dev/backlog/#/?ks=GDK-793
[GDK-808]: https://gadak.dev/backlog/#/?ks=GDK-808
[GDK-797]: https://gadak.dev/backlog/#/?ks=GDK-797
[GDK-798]: https://gadak.dev/backlog/#/?ks=GDK-798
[GDK-800]: https://gadak.dev/backlog/#/?ks=GDK-800
[GDK-594]: https://gadak.dev/backlog/#/?ks=GDK-594
[GDK-809]: https://gadak.dev/backlog/#/?ks=GDK-809
[GDK-810]: https://gadak.dev/backlog/#/?ks=GDK-810
[GDK-796]: https://gadak.dev/backlog/#/?ks=GDK-796
[GDK-799]: https://gadak.dev/backlog/#/?ks=GDK-799
[GDK-801]: https://gadak.dev/backlog/#/?ks=GDK-801
[GDK-802]: https://gadak.dev/backlog/#/?ks=GDK-802
[GDK-837]: https://gadak.dev/backlog/#/?ks=GDK-837
[GDK-822]: https://gadak.dev/backlog/#/?ks=GDK-822
[GDK-824]: https://gadak.dev/backlog/#/?ks=GDK-824
[GDK-712]: https://gadak.dev/backlog/#/?ks=GDK-712
[GDK-814]: https://gadak.dev/backlog/#/?ks=GDK-814
[GDK-815]: https://gadak.dev/backlog/#/?ks=GDK-815
[GDK-816]: https://gadak.dev/backlog/#/?ks=GDK-816
[GDK-817]: https://gadak.dev/backlog/#/?ks=GDK-817
[GDK-819]: https://gadak.dev/backlog/#/?ks=GDK-819
[GDK-820]: https://gadak.dev/backlog/#/?ks=GDK-820
[GDK-821]: https://gadak.dev/backlog/#/?ks=GDK-821
[GDK-825]: https://gadak.dev/backlog/#/?ks=GDK-825
[GDK-826]: https://gadak.dev/backlog/#/?ks=GDK-826
[GDK-827]: https://gadak.dev/backlog/#/?ks=GDK-827
[GDK-828]: https://gadak.dev/backlog/#/?ks=GDK-828
[GDK-829]: https://gadak.dev/backlog/#/?ks=GDK-829
[GDK-831]: https://gadak.dev/backlog/#/?ks=GDK-831
[GDK-686]: https://gadak.dev/backlog/#/?ks=GDK-686
[GDK-687]: https://gadak.dev/backlog/#/?ks=GDK-687
[GDK-842]: https://gadak.dev/backlog/#/?ks=GDK-842
[GDK-849]: https://gadak.dev/backlog/#/?ks=GDK-849
[GDK-850]: https://gadak.dev/backlog/#/?ks=GDK-850
[GDK-852]: https://gadak.dev/backlog/#/?ks=GDK-852
[GDK-853]: https://gadak.dev/backlog/#/?ks=GDK-853
[GDK-854]: https://gadak.dev/backlog/#/?ks=GDK-854
[GDK-856]: https://gadak.dev/backlog/#/?ks=GDK-856
[GDK-857]: https://gadak.dev/backlog/#/?ks=GDK-857
[GDK-858]: https://gadak.dev/backlog/#/?ks=GDK-858
[GDK-862]: https://gadak.dev/backlog/#/?ks=GDK-862
[GDK-863]: https://gadak.dev/backlog/#/?ks=GDK-863
[GDK-883]: https://gadak.dev/backlog/#/?ks=GDK-883
