# Changelog — long form

The reasoning behind each entry: what the defect was, why the fix has the
shape it has, and what now prevents it from coming back. [CHANGELOG.md](../CHANGELOG.md)
is the readable list; this file is the record it summarizes, kept verbatim.

From v0.17 onward the long form lives here. Releases before that still carry
their long form in `CHANGELOG.md` itself.

## Unreleased

The cycle where an agent's writes grow up and the development panel arrives.
An issue now shows the PRs and commits that implement it — mirrored on
connected workspaces, writable on standalone ones — and the write verbs
learned the vocabulary a coding agent actually sends: custom fields by
alias, resolutions on transitions, fix versions and components by name,
assignees by whatever identity is at hand. The cycle closed with a
pre-release audit across the network seams, the MCP surface and the web UI;
the fixes are all here.

### The development panel

- **An issue knows its PRs** ([GDK-496], [GDK-497]). The development panel
  reaches the mirror as `dev_links`: connected workspaces mirror Jira's
  dev-status, and a standalone origin accepts links from its own CLI —
  issuetap serves the same `/rest/dev-status` shapes Jira Cloud does, so a
  client written against Cloud works unchanged. `devStatus` is a per-machine
  choice and is never team-exported.
- **`gadak dev scan`** sweeps the current repo's PRs into dev links in one
  pass ([GDK-531]), and **`gadak dev link`** writes one; standalone or
  paired workspaces may write, the git hook keeps its workspace, and scan
  reports what it did cleanly ([GDK-538], [GDK-539], [GDK-554], [GDK-570]).
- **A mirrored PR link is a linked PR** in the web UI ([GDK-495]), a GitHub
  PR or commit link opens in the in-app browser pane ([GDK-527]), and the
  panel's empty state names why it is empty — feature off, not mirrored, or
  truly no PRs — with a chip for a declined PR ([GDK-555], [GDK-540]).
- **Dev links survive sync** ([GDK-536], [GDK-541], [GDK-562], [GDK-537]).
  A standalone link survives the next rewrite, a fetch error preserves what
  the mirror has instead of deleting it, writing links bumps the mirror
  version so a running UI sees them, and issuetap moves the issue's
  `updated` stamp so a paired remote's incremental sync picks the link up.
- **`gadak link A B --type blocks` writes an issue link** ([GDK-19]), and the
  detail panel writes one too ([GDK-85]). Linked issues was a read-only list,
  which made linking — one of the most frequent things anyone does in Jira —
  the reason to leave the app. The panel now carries a type select and a key
  field over the same route the CLI uses: origin first
  (`POST /api/v1/issues/{key}/link/`), then both issues re-read into the
  mirror, then the list renders server truth rather than the typed guess, so a
  link the origin reshaped shows up reshaped. `GET …/linktypes/` serves the
  catalog, a frozen workspace refuses the POST, and a missing credential is
  the same 409 `credential_required` the comment and transition routes give.
  The link-type resolver — id, name, outward or inward description, with the
  symmetric-type and ambiguity guards — moved to
  `internal/origin/linkresolve.go`: it used to live in `package main` where
  the HTTP handler could not import it, so the REST route arrived as a copy
  with a parity test holding the two in step. One owner needs no parity test.
  Deleting a link is [GDK-680]: the block is that `origin.IssueLinker` has no
  unlink verb, and adding one is a promise to all three origins.

### Write verbs an agent can trust

- **A fourth principles sweep, each fix a single owner** ([GDK-644]).
  The retry defaults every origin client copied (5 attempts, 1s backoff,
  60s timeout) live in one place now, and a contract test keeps the jira,
  linear and confluence constructors on it. Importing the config package
  no longer selects a workspace — each binary reads the environment
  itself, so a test importing config cannot inherit the shell's profile.
  Error strings stop repeating their function names, a retry wait stops
  leaking a timer on cancel, sync notifications ride the watch context,
  and the Linear refusal message stops claiming the origin is read-only
  — it has not been since the write verbs landed.
- **A third dead-code sweep, every deletion proven unreferenced**
  ([GDK-647]). Five stats getters nothing read (and their write-only
  counters), the deprecated `serve --sync` no-op alias (passing it now
  errors instead of silently doing nothing — `--no-sync` is unchanged), a
  dev-scan helper that only its own test exercised, an i18n key no surface
  renders, three unused helpers, and 26 web exports nothing imported.
  The 20k-row search fixture moved behind a `searchscale` build tag, so
  the default store test run stops compiling it. Kept on purpose: the
  `scry_locale` read-fallback (still a live compatibility path) and the
  push client tree (its own open decision).
- **An origin adapter no longer stubs what it cannot do** ([GDK-641]).
  `origin.Writer` was a 17-method producer interface,
  so the Linear adapter filled five verbs with "not supported" stubs.
  Writer is now the twelve verbs every origin actually implements;
  versions, links, create-field metadata, and inline comment media are
  optional faces callers type-assert — a missing face returns the exact
  error string the stubs used to, owned in one place, and the comment
  path's degrade-and-continue is unchanged.
- **A REST parent rejection explains itself like the CLI's** ([GDK-635]).
  The CLI has long appended the mirror's hierarchy answer to Jira's
  localized parent 400 — which level the rejected parent sits at, what it
  can actually hold, and up to three open epics to pick instead. REST's two
  parent paths (create body, `PUT {key}/parent/`) passed the bare origin
  error through. Detection and hint assembly moved to one owner
  (`internal/parenthint`); both surfaces call it, the origin text stays
  intact with the hint appended, and an unrelated 400 grows no hint.
- **One owner for the write-through re-read** ([GDK-642]). After a write,
  the issue is re-read from the origin that owns it — and that routing
  (Linear row → Linear re-read, everything else → Jira) existed as two
  word-for-word copies, one in the CLI and one in REST. `sync.RefreshIssue`
  is the single owner now; both surfaces call it, and a structural test
  fails on any third copy of the pair, so the two paths cannot drift the
  way they did before this audit.
- **`create` and `edit` take `--field alias=value`** for the custom fields a
  project requires ([GDK-513]), the create dialog learns what this project
  and type actually require ([GDK-254]), and **`issue --editmeta` asks the
  origin what this issue can edit** ([GDK-514]).
- **`transition` carries `--resolution`, `--field` and `-m`** ([GDK-509]),
  and the REST transition resolves the same identifiers the CLI does — one
  shared resolver, not a second implementation ([GDK-341]). GET transitions
  emits the mapped category token the write side accepts ([GDK-564]).
- **`edit --fix-version` writes release membership by id** ([GDK-516],
  [GDK-123]) and **`--component +Name / -Name` rides the label grammar**
  ([GDK-517]). A label starting with `+`/`-` is refused instead of silently
  recorded ([GDK-545]).
- **`assign` accepts name and accountId next to email** ([GDK-515]), a
  comment's `@Name` reaches the person it names ([GDK-510]), and `comment`
  takes a positional body like `create`'s SUMMARY ([GDK-315]).
- **Bulk issue reads**: many keys and `--keys -`, with no silent drop
  ([GDK-425]).
- **REST gets the parent pair the CLI already had** ([GDK-328]). The
  create body takes `parent` and `PUT {key}/parent/` sets or clears one —
  the same `fields.parent` wire the CLI sends, so issuetap works unchanged.
  A value that is not a Jira key, or the issue itself, is refused before it
  reaches the origin. The key-shape check that was already copied twice
  (CLI positional keys, MCP `gadak_show`) now has one owner in
  `internal/fields`, and the new endpoints are its third and fourth
  callers, not its third copy.
- **A parent rejection now names the epics you could have picked**
  ([GDK-330]). The hierarchy hint already explained *why* the origin
  refused (`NMB-1 is level 0 — a standard issue can only sit under an
  epic`); it now ends with up to three open epics from the same project,
  straight from the mirror. Deliberately still not a preflight: the
  origin keeps deciding — a local guess would false-reject on Premium
  same-level hierarchies — and the origin's own 400 stays verbatim above
  the hint.
- **`gadak claim KEY` takes an issue as yours** — assignee plus the
  in-progress transition in one step ([GDK-591]). On standalone and paired
  workspaces that is one atomic call on the origin's claim endpoint, and a
  claim another actor already holds is refused with the holder's name and
  its own exit code (75), so parallel agents stop colliding through
  "[claim]" comments. Connected Cloud has no such endpoint; there the claim
  judges locally and writes the two calls as a fallback, with a one-line
  warning that the pair is not atomic. `--take-over` replaces the holder.
- **`gadak issue` shows how long work sat**: a `durations` line —
  `wait 3d · progress 5h` — computed from the changelog at read time
  ([GDK-591]). Wait is created → first entry into in-progress; progress is
  the latest in-progress entry → done (or now). No stored column: the
  origin's status catalog is cached (`status_catalog`, schema v34) so the
  changelog's bare status ids resolve on every workspace.
- **The audit pass over the agent verbs** ([GDK-551], [GDK-565], [GDK-546],
  [GDK-556]): write verbs resolve the identities the read side emits, name
  their exits, and round-trip their fields.

### Standalone and issuetap

- **A second desktop launch raises the first window, standalone included**
  ([GDK-658]). The desktop app used to take the standalone persist lock
  before wails could check for an already-running instance, so a second
  launch died on "workspace busy" instead of handing off — the desktop
  twin of the serve command's live-owner check ([GDK-468] class). The
  single-instance check now runs first; the persist lock, loopback
  listener and advertise file are taken only by the instance that won,
  and a source-order test plus a real two-process test hold the sequence.
- **A standalone workspace speaks your language** ([GDK-597]). `gadak
  config set locale ko` and the workspace's own tracker localizes status
  and issue-type names — while priority names stay English, which is what
  a live Korean Cloud site actually serves. Changing the locale rebuilds
  the mirror, because cached display names would otherwise stay in the old
  language. A connected workspace is unaffected: its language belongs to
  the Atlassian account.
- **Claiming an issue is one move** ([GDK-591]). `gadak claim KEY` assigns
  the acting agent and moves the issue in progress; on an issuetap origin
  that is atomic, so a second agent racing for the same issue is refused
  and told who holds it. `--take-over` is the deliberate handover, and a
  connected Jira says out loud that its two calls could interleave.
  `gadak issue` shows how long an issue waited and how long it has been in
  progress, computed from the changelog rather than stored.
- **The web shows which worker was a bot** ([GDK-590]). One badge marks a
  bot on comments, history and PR links — keyed on the account type, never
  a name. Group or filter a list by actor to see what one agent touched,
  and an issue's detail says how long it waited and how long it has been in
  progress.
- **The panel carries deployments and builds** ([GDK-592]). `gadak dev
  deploy KEY --env production --state successful` and `gadak dev build KEY
  --state failed --number 592` write the other two kinds of development
  link; `gadak issue` lists them apart from pull requests, and a sync that
  rewrites an issue's PR links leaves them alone. On a standalone origin
  the Cloud-shaped summary blocks that used to serve zeroes now carry real
  counts, so a client written against Cloud reads them unchanged.
- **A dev link knows its people** ([GDK-589]). `dev_links` carries the PR
  author (the human), the writing actor (the bot), and the head branch
  (schema v33); `gadak dev scan` sends them from `gh`, `dev link` takes
  `--author`/`--branch` (branch defaults to the current one), and the
  origin stamps the actor server-side — a client cannot forge it.
- **A nameless agent gets a readable name** ([GDK-593]). An actor slug
  with no display name is assigned a deterministic alias from an
  adjective-animal dictionary — `claude:354bff2b` shows up as "Spirited
  Lion (Claude Code)", the same name on every restart. An explicit name
  always wins.
- **A write carries its author** ([GDK-586], [GDK-588]). Set `GADAK_ACTOR`
  (`slug|display name`), or a workspace-default `actor` in config, and every
  write to an issuetap origin — embedded, served, or paired — attributes to
  that agent (`accountType: "agent"`), auto-created on first sight. Claude
  Code sessions are detected without any setup. Connected Jira/Linear never
  sees the header; unset keeps the seeded user, exactly as before.
- **Standalone records carry wall time** and the skill names the seeded
  user ([GDK-369]).
- **Restricted and internal comments are distinguishable, and writable** —
  in the store, and on the REST surface ([GDK-511], [GDK-528]).
- **Parent hierarchy** rides the issuetap pin, and unsupported project
  subpaths answer 501 instead of pretending ([GDK-505], [GDK-512]).
- **A forgotten session no longer loses its persist lock to the GC**
  ([GDK-484]), **a frozen workspace refuses every pull** so a scrubbed
  fixture stays scrubbed ([GDK-181]), and **a mirror written by a newer
  gadak is a cache to rebuild, not a dead end** ([GDK-498]).

### One vocabulary for origin writes

- **`origin.Writer` stopped speaking Jira in its own signatures** ([GDK-665]).
  Every write verb named `jira.Transition`, `jira.User`, `jira.FieldMeta` and
  six more, so a second tracker had to import the first one's HTTP package to
  implement the interface — and Linear's writer did, 28 times. The Writer now
  names `origin.` DTOs, adapters convert at the boundary, `cmd/gadak/create.go`
  no longer type-asserts `*jira.Client`, and Linear's `WorkflowState.type` →
  `status_category` collapse has one owner in `internal/linear/status.go`
  instead of being restated per call site. An AST test locks the interface's
  vocabulary (measured red on the previous source: eight `jira.` selectors).
  Being honest about the shape of the win: those DTOs are type *aliases* of
  the Jira payload structs, because distinct types would make
  `internal/server` and three more packages name origin types and the HTTP
  JSON is a contract. A new origin no longer writes `jira.` anywhere; the
  struct layout is still inherited. Promoting the aliases to real types is
  weighed in [GDK-679], which also records that the vocabulary gate passes on
  an alias.

### Workspaces and pairing

- **`gadak workspace use <name>` stores a default, and one function decides
  which workspace you are in** ([GDK-490]). `--workspace`/`-w` is the
  canonical selector with `--profile`/`-p` as a permanent alias (no removal
  planned), `GADAK_WORKSPACE` is canonical with `GADAK_PROFILE` and
  `SCRY_PROFILE` as aliases — and the resolution order is now four deep:
  flag, env, stored default, root. The stored layer lives inside `Profile()`
  rather than beside it, because a second owner of "which workspace" is how a
  surface starts disagreeing with the one next to it. It is a 0600
  `default-workspace` file at the home root, not a field in `config.json`
  (that file is the root profile's own credential document) and not a row in
  the mirror (the mirror is a disposable cache). A stored name whose
  workspace does not exist is refused with the available names instead of
  falling back to the root — silently reading a different tracker is the
  failure this product does not get to have — and `workspace use --clear`
  still runs in that state, so the refusal is recoverable. Bare
  `gadak workspace` answers "why this workspace" by naming what selected it.
- **A mounted standalone workspace can create issues again** ([GDK-677],
  reported as [#52]). The origin routing decision compared a probe answer
  against the process-global profile, so a standalone profile mounted under
  `/w/<name>/` — with a stale advertise file left by once serving it as the
  primary, on the same default port — was approved for routing and its
  create-metadata calls landed on the primary profile's origin route:
  `not_found`. A Config now carries the profile it was loaded for and the
  routing compares against that, so the mount builds its own embedded
  session. The same change upgrades a cross-profile refusal: asking for a
  workspace another live serve owns now routes to that owner instead of
  failing busy.
- **`--workspace` is the name, `--profile` is the alias**, and writes say
  which one they used ([GDK-490]).
- **Pairing tokens gate the origin passthrough** ([GDK-433]), a pairing
  credential counts as a credential ([GDK-442]), and MCP, REST create and
  sync reuse the pairing owners the CLI already has ([GDK-485]).
- **A bound workspace cannot be quietly rebound**: init refuses a different
  site ([GDK-452], [GDK-561]), the guard steps aside for a paired tailnet
  host ([GDK-443]), `_home` is not a device ([GDK-450]), and example
  endpoints stop looking like someone's real tailnet ([GDK-433], [GDK-452]).
- **Pairing tells the truth**: the status verbs learn what paired means and
  failures name their cause ([GDK-449], [GDK-453]), a paired write 401
  answers with the pairing error instead of `credential_rejected`
  ([GDK-543]), and the credentials dialog stops asking for what it already
  has ([GDK-455]).

### The mirror's schema

- **A restricted issue is distinguishable from a public one** ([GDK-519]).
  `issues.security_level_id` / `security_level` project Jira's issue
  security level (id is the key; the name is display-only and localizes).
  An issue the credential cannot see still never enters the mirror; the
  gap was that a restricted issue the credential *can* see looked public,
  so an agent had no warning before quoting it. `gadak issue KEY --json`
  carries both fields (null when unrestricted); the text form prints a
  line only when a level is set. Existing rows stay NULL until the next
  sync — no backfill.

- **`gadak init` and `gadak install-cli` install the Claude Code skill when
  `~/.claude` already exists** ([GDK-93]). A missing `~/.claude` is a skip
  (`install-cli` still prints `gadak skill install` as the next step); a
  file gadak did not write is left in place and stderr names
  `gadak skill install --force`. Skill failure is a warning — init and
  install-cli still exit 0. `init --json` grows `"skill":
  "installed"|"skipped"|"failed"`.
- **The repository is a Claude Code plugin marketplace** ([GDK-93]).
  `claude plugin marketplace add midagedev/gadak` then
  `claude plugin install gadak@gadak` installs the same skill without the
  gadak binary — the plugin's source is the repo's `skills/gadak/`, so
  there is one skill body, not two. Verified end to end against Claude
  Code 2.1.234: strict manifest validation, a local marketplace add, an
  install whose cached SKILL.md is byte-identical to the repo's, and a
  clean uninstall.

- **Fix versions keep their id, and the project's release catalog lands in
  the mirror** ([GDK-532]). `issues.fix_version_ids` stores the same-order
  ids next to the existing name array `fix_versions` (that name array stays
  the 0.x recipe key). A `versions` table (`id`, `project_key`, `name`,
  `released`, `archived`, `release_date`) is filled on full sync and
  reconcile from `GET /project/{key}/versions`, not every incremental tick;
  a catalog GET failure is a warning and the issue pass still commits. Join
  on id — names rename. The next sync fills existing rows; the migration
  does not backfill.
- **Sprint is a column an agent can query** ([GDK-518]). `issues.sprint_id`
  / `sprint_name` / `sprint_state` project one sprint per issue (active over
  future over closed, then the larger id) from the site's gh-sprint field,
  discovered via `GET /field` — no hardcoded customfield id, no Agile REST,
  no board catalog. `gadak search --jql 'sprint in openSprints()'` and
  `sprint = <id>` apply; a name comparison stays listed as unsupported.
- **Personal state moves out of the mirror** — `rm gadak.db` keeps your
  views, visit history and search history ([GDK-105]).
- **Korean mid-compound search**: a `cjk_bigram` fourth FTS column finds the
  word inside the compound ([GDK-259]), and a fresh mirror is born with the
  canonical `items_fts` ([GDK-444]).
- **JQL `parent =` / `parent IN` filters by the mirror's `parent_key`**
  ([GDK-521]), the column that says why an issue closed gets a stable id
  ([GDK-520]), `schema_version` has one owner — `PRAGMA user_version`
  ([GDK-526]) — and custom-field mapping state is visible instead of silent
  ([GDK-522]).
- **Hierarchy survives the trip out of the mirror** ([GDK-329]). The
  mirror has stored `hierarchy_level` since v11, but nothing carried it:
  `IssueLite` now projects it as stored (epic 1, standard 0, sub-task −1,
  never omitted — like `priority_rank`), so the web can tell a sub-task
  apart without a localized type name. On the way in, createmeta issue
  types kept only id and name; a dedicated parse type now carries Jira's
  `subtask` and `hierarchyLevel` end to end — client, REST, web types —
  and issuetap already sent both, so standalone gets it for free. The
  contract doc catches up: `IssueLite` gains `hierarchy_level` and the
  `parent_key` it had omitted all along. No UI behavior changes here —
  the creation dialog learning "this type needs a parent" is its own
  issue.

- **A bot worker is identifiable in data, not by name** ([GDK-590]). The
  origin's account catalog lands in the mirror as `users` (v36): every
  account a sync pass already reads — assignee, reporter, creator, comment
  and changelog authors — keeps its `account_type`, where standalone
  issuetap says `agent` for the accounts behind `X-Issuetap-Actor` and
  Cloud says `app` for Connect accounts. One judgement function reads that
  axis; nothing guesses from display names. The `issue_actors` view unions
  the three actor seats (`comments` ∪ `changelog` ∪ `dev_links`), so
  "issues this bot touched" is one query (the RECIPES example joins on
  account ids). REST: member rows carry `account_type` / `is_bot` and now
  include actors who never held the assignee or reporter seat, history
  entries carry `author_id`, and a comment names its author's
  `author_account_type`. No backfill — the catalog refills on the next
  sync.

### The web UI, made consistent

- **One command registry instead of three surfaces that had to agree**
  ([GDK-674]). A keyboard command lived in three places — the keymap that
  dispatches it, the palette row that runs it, the cheat-sheet row that
  documents it — with no shared source, so keeping them in step was a review
  habit and drift was invisible until someone noticed a key the sheet promised
  and nothing handled (that is [GDK-652], and it was found by eye). All 55
  commands are now declared once in `web/src/lib/commands.ts` with their
  chords, their phase, their scope, their dispatch, their palette row and
  their help caption; the keymap still owns DOM dispatch and the palette still
  wires its own hosts, but neither invents the list any more.
  `npx vitest run web/src/lib/commands.test.ts -t dumpKey` prints the whole
  surface as text, which is the debugging tool this did not have.
  **Nothing on screen moved.** Unifying the source surfaced nine
  cross-surface disagreements (a palette row that never taught `p`, help-only
  rows that are local handlers, two different captions for "open in Jira",
  Esc's real cascade against the sheet's single line) and every one of them
  was resolved by adopting what the keymap actually dispatches and leaving the
  display exactly as it was — a refactor that changes the pixels is not a
  refactor.

- **A UI string is one object with every locale in it** ([GDK-668]). The
  catalog was three hand-kept files — 1048 keys × en/ko/ja — so every copy
  change meant editing three places and hoping, and a locale left behind
  was invisible until someone browsing in that language found it. A key is
  now one `{en, ko, ja}` object, grouped by domain rather than by language;
  omitting a locale is a type error at build time. `t()` is unchanged (the
  per-locale tables are derived once at load), and the copy itself did not
  move: a `(key, locale, string)` dump of all 3144 rows is byte-identical
  across the migration.
- **The demo fixture cannot silently lag the schema again** ([GDK-671]).
  `examples/demo.db` was five migrations behind the code, and nothing was
  red: the e2e runner opened its own copy, which migrated it at runtime and
  hid the gap, while the Makefile comment claimed the file was current. The
  committed fixture is rebaselined (534 issues, 634 comments, 71 pages and
  every derived row count unchanged — this is a migrate, not a
  regeneration), a store test compares the committed file's schema version
  against this binary's migration level without opening it, and the e2e
  runner now refuses a stale fixture instead of papering over it. It still
  opens the copy for one honest reason: the portable snapshot needs its FTS
  index made writable.
- **One noun for the wiki object, and seven smaller splits closed**
  ([GDK-652]). The same mirrored Confluence object was a Document in the
  column, a "page" in the detail copy, and "Wiki pages" in settings — now
  it is a document in all three locales (browser-tab "page" stays, it is a
  different object), and the majority counts that decided each locale are
  locked in the catalog gate. The in-field clear-X shares one name (and
  SearchBox finally tells screen readers what it is), every arrow-left
  back button says the same thing, the bulk bar shows the kbd chips the
  palette was already teaching (`s`/`a`/`l`/`Esc` — `p` has no palette
  badge, so no chip), loading copy uses one verb per locale, four empty
  states get their icons, Korean counts selected issues in 건 like every
  other issue count, and onboarding's first sync speaks the same line as
  the freshness chip.
- **The main-column views answer the same keys** ([GDK-651]). Esc now
  closes documents, history, and the feed — visible layer first, the order
  the screen paints them — instead of working only on the issue list's
  overlays. The palette registers the sibling views (`Open documents`,
  `Open feed` next to `Open history`) and the cursor row gains
  favorite/watch toggles there. The feed's signed-out empty state grows the
  settings button the sidebar already had, and the shortcut sheet stops
  pretending: a new section documents Tab as the way through those rows
  (they have no j/k cursor — a real one measured at ~250 lines across
  three views, documented instead).
- **One footer under every comment box** ([GDK-650]). The page comment
  form had its own submit button and no shortcut chip, so the `{mod} ↵`
  chord worked there but nothing said so. Both composers now render one
  shared footer — same chip, same posting state, the issue side keeps its
  attach button as the footer's leading slot — and a unit gate holds the
  chip to a single owner so a page-local spelling cannot come back.
- **Four type sizes, and nothing between them** ([GDK-129]). The screen
  had drifted 198 arbitrary pixel sizes past the declared scale — 190 of
  them `12px`, one pixel off both of its neighbours, which reads as noise
  rather than hierarchy. Every one is absorbed into the four tokens by
  role (metadata to micro, reading text to body), dialog titles join panel
  headings at the top step, the wordmark gets one owner instead of two
  sizes, and the theme gate now fails on any `text-[Npx]` utility so the
  fifth size cannot come back.
- **Tests moved down the ladder, and two got honest** ([GDK-620]). A
  static lint that only reads files no longer rides the browser runner —
  it is a vitest unit now. The i18n key-parity test re-proved what the
  type system already enforces and is replaced by the axis types cannot
  see (a blank value in one locale; placeholder counts, not just sets).
  The dialog-registry test compared a list against itself — a seventh
  dialog without a row stayed green; it now walks the source tree for
  every component that imports the shell and demands a row for each.
- **The ladder sweep ran again — five e2e cases stepped down, none lost**
  ([GDK-649]). The docs pane's six empty states were being enumerated in a
  browser; the branching is a pure function now, vitest walks the
  combinations, and the e2e keeps the representative paths. The IndexedDB
  cache-upgrade path runs as a unit over fake-indexeddb, the dialog-shell
  source walk joined the unit-file convention, a settings assertion that
  could not fail in a browser became a source scan, and the demo fixture's
  issue count — a literal `534` in twenty-nine spec files — has one owner
  in the e2e helpers, so regenerating the fixture changes one line.
- **The test suite stops paying for probes it does not measure**
  ([GDK-648]). One Linux catalog test was quietly running a real `claude
  mcp get` (two seconds a run); the rule the file already stated — catalog
  tests stub the probe — is now enforced by the package's `TestMain`, so
  the next missed stub panics instead of costing wall-clock. The SQLite
  blocked-BEGIN detector waits 80ms on a 50ms test budget instead of a
  second on production's, and the hung-sources e2e aborts its routes
  instead of pinning teardown on a 60-second fulfill.
- **The palette's advertised `?` works now** ([GDK-618]). The footer has
  always said `?` opens the shortcuts sheet, but the global keymap ignores
  keys while a modal owns the screen and the palette never claimed the key
  — it just typed into the query. On an empty query the palette now hands
  the screen to the sheet; mid-query `?` stays a search character.
- **A component stops asking the document where its own children are**
  ([GDK-645]). The detail panel found its comment box with a global
  selector — with two composers on screen it could blur the wrong one —
  and the quick-comment dialog did the same to focus its own; both now
  bind the instance they render, and a source sweep holds every component
  to that rule. The palette's highlight is a derived clamp instead of an
  effect rewriting state, the sidebar's in-flight drag no longer lives on
  the persisted store, and every viewport subscriber shares one
  matchMedia. Kept deliberately: the keymap's testid dispatch (it cannot
  know what is mounted) and the effects that guard typing or perform IO.
- **Esc closes the menu it was aimed at — and nothing else** ([GDK-617]).
  The breakdown menu had no Esc handling at all, so the keystroke fell
  through to the shell keymap and cleared the selection with the menu
  still open. It and two more hand-rolled dismissals (the sync-history
  popover, the notification bell) now ride the shared outside-click/Esc
  owner and spend the key; a field editor finds its own portaled menu by
  reference instead of a global selector that answers for the wrong
  instance once two editors mount; and the "updated within 24h" accent
  ticks with the wall clock instead of freezing at mount.
- **One key, one spelling** ([GDK-621]). The submit-comment chord read
  `⌘ ↵` on the cheat sheet, `{mod}Enter` under the composer, and the word
  "Enter" in two kbd hint lines — now every keycap surface prints the
  sheet's glyphs, in all three locales. Every close X says "Close (Esc)"
  (the two stragglers joined, and the now-dead catalog keys are gone), the
  sheet's Detail section stops describing the issue-`o` with the
  document's caption, and a new gate holds catalog strings and the sheet
  to the same notation so the third spelling cannot come back.
- **"You are here" is an attribute, not a paint color** ([GDK-613]). Six
  e2e specs asserted the active sidebar row and settings tab by their
  background utility class, so renaming a palette token would have turned
  them red with nothing broken. The active rows and tabs now carry
  `aria-current` on the same condition that paints them — screen readers
  gain the same signal the tests read.
- **Empty is a state, not a blank** ([GDK-130]). The settings tables for
  members, groups, products, and rules used to render their column headers
  over nothing; each now says what's missing and what to do next, with the
  add button right below. The three loading leftovers — new-issue dialog,
  history, integrations — share one `LoadingState` with the empty-state
  geometry instead of three ad-hoc paragraphs, and the last italic
  empty-state lost its slant.
- **The web UI speaks Japanese** ([GDK-626]). A full `ja` catalog joins
  `en`/`ko` — Japanese Jira Cloud vocabulary (課題, 進行中, 担当者), every
  count in `{n}件` — selectable in Settings and the palette, detected from
  the browser language. The standalone tracker already answered `locale
  ja`; now the UI above it does too. Two new gates guard all three
  catalogs: placeholder tokens must survive translation exactly, and
  settled Japanese terms allow no competing spelling — the same containment
  the Korean catalog earned in the last audit.
- **One Esc closes one surface** ([GDK-604]). Escape over an enlarged
  attachment closed the viewer *and* the detail panel behind it (and
  dropped a bulk selection on the way); the person and document panels had
  the same double-spend. The keymap now yields to an open media viewer,
  the viewer consumes the key and traps focus like every other dialog, and
  the panels decline an Esc another surface already spent — pinned by an
  e2e negotiation spec.
- **A transition that needs a screen asks for it inline** ([GDK-83]). GET
  transitions now carries each transition's required screen fields —
  required only, options included — so picking "Resolve" opens a small
  form in the dropdown (a select for closed lists, a text input otherwise)
  instead of failing after the fact. A 400 the form could not prevent
  turns the error toast actionable: the refusal message plus an "Open in
  Jira" button, offered only when the workspace has an origin URL to open.
  A transition with no required fields sends immediately, exactly as
  before.
- **Desktop first-run opens the token page inside the app** ([GDK-71]).
  The onboarding token link opens Atlassian's token page in the browse
  pane and focuses the paste field, so the token round-trip stays in one
  window; a sibling link keeps the system browser as the IdP escape hatch.
  Serve and hosted keep the new-tab behavior.
- **The first run lands on the epic breakdown** ([GDK-100]). A fresh
  workspace with no saved view opens on the built-in Epics view instead of
  a bare all-open replica; from the second run the last-used view wins, and
  a configured team-group preset still beats the generic default.
- **The stale chip is a marker, not a stain** ([GDK-200]). The amber wash
  behind the hourglass read as a muddy smear on the dark and ink grounds;
  the chip now sits on the neutral elevated fill with an amber hairline,
  so the glyph and day count carry the meaning and the mid/loud ladder
  moves to ring weight instead of stain depth. Vision-verified before/after.
- **One owner for the dialog shell** ([GDK-316]). The six same-class modal
  dialogs now render through a single DialogShell component that owns the
  backdrop, panel chrome, header X, and footer — pixel-identical by
  measurement — and a source gate fails any new dialog that copies the
  shell classes instead of using it.
- **JSON and SQL agree on the key's name** ([GDK-255]). Every JSON surface
  that said `issue_key` now also carries `key` — the name `issues_full`
  answers to — derived at marshal time so the two can never drift, and a
  `no such column` from `gadak sql` suggests the nearest real column
  (`issue_key` → `did you mean "key"?`) on the error instead of leaving
  the agent to guess.
- **doctor catches a lying schema stamp** ([GDK-180]). A mirror whose
  `user_version` says one schema while its tables say another (the
  live-copy frankenstein) used to die in raw SQL errors; `gadak doctor`
  now diffs the file against what this build's migrations produce and
  says "mirror is damaged — delete the mirror file and run gadak sync".
- **`install-cli` speaks Windows** ([GDK-353]). The default directory is
  `%LOCALAPPDATA%\Programs\gadak` instead of the unix `~/.local/bin`, the
  permission hint stops recommending sudo, and installing records where
  `gadak-desktop.exe` lives so `views open` from the copied CLI can still
  find the app.
- **Ctrl+W stops promising what it cannot do** ([GDK-351]). The Close Tab
  accelerator only acts on the in-app browse pane, which is macOS-only, so
  builds without the pane no longer create the menu item at all — its
  presence now derives from the same fact as the pane itself.
- **A `gadak://` link works on Windows** ([GDK-350]). The desktop app now
  registers the scheme in `HKCU\SOFTWARE\Classes\gadak` on first launch and
  rewrites the entry whenever its own path changes, so clicking a link in a
  browser or Slack opens the app — the portable pack itself still never
  touches the registry, and `--unregister-gadak-protocol` removes the key.
- **The MCP process keeps its own mirror fresh** ([GDK-599]). `gadak mcp`
  is a resident process, so it now runs the same incremental watch loop
  `gadak serve` does — an agent on an MCP host reads a fresh mirror
  without ever knowing sync exists. `--no-sync` opts out; all diagnostics
  stay on stderr, never the JSON-RPC stream.
- **`gadak sync --if-stale 15m` is the agent's session opener** ([GDK-598]).
  Fresh mirror → instant no-op with no network; a source older than the
  threshold — or whose last sync failed — gets one incremental pass, so a
  died sync self-heals on the next session. The staleness warning now
  watches every source (a Linear-only mirror used to never warn) and
  recommends the exact command.
- **`o` opens the origin from the keyboard** ([GDK-81]). The escape hatch
  stops being click-only: `o` on the list cursor or open detail opens the
  issue's Jira page, and on a selected wiki page its source — the exact
  same opener as the header links, so a workspace with no site URL makes
  it a quiet no-op. Listed in the `?` sheet and as a ⌘K action.
- **Components and parent edit inline, no setup required** ([GDK-86]).
  The detail panel's components row becomes an inline multi-select and
  the parent row an issue-key typeahead (local mirror suggestions,
  self-parent refused) whenever the issue's editmeta allows them —
  built-in system aliases, so neither needs a configured field mapping
  the way custom fields do. An origin whose editmeta omits the field
  keeps its row read-only; the parent write is byte-identical to
  `gadak edit --parent`.
- **The comment shortcut stops being Mac-only copy** ([GDK-354]). The
  composer's kbd hint renders the platform's own modifier — ⌘Enter on
  macOS, CtrlEnter elsewhere — instead of a hard-coded ⌘, `gadak raycast
  install` refuses non-macOS hosts before unpacking anything (and the
  desktop integrations list stops offering Raycast anywhere but macOS),
  and the offboarding and update docs speak PowerShell and the portable
  zip alongside `rm -rf` and brew.
- **Windows stops pretending it notified you** ([GDK-349]). OS desktop
  notifications have one capability owner; on a platform that cannot fire
  them the watch loop no longer consumes pending events, the settings copy
  drops its "always run" claim, and the in-tab browser toggle appears on
  Windows desktop instead of being hidden behind a false premise.
- **A story shows its child issues** ([GDK-121]). The detail panel's child
  section now falls back to direct `parent_key` children when the issue has
  no epic-keyed descendants, so sub-tasks stop being invisible from their
  parent story. An epic keeps its rollup unchanged.
- **Saved views live on the server by default**, and localStorage views move
  there once ([GDK-437]). Sidebar sections collapse and go where you drag
  them ([GDK-434], [GDK-435]), and the project filter grows a NOT IN axis —
  include narrows, exclude wins ([GDK-438]) — which survives Copy JQL
  ([GDK-441]).
- **The consistency sweeps**: every zero state names its cause, the catalogs
  and panels say each thing once and in one voice, errors answer with the
  way out, standalone stops speaking Jira, and one name for each thing
  everywhere a person reads it ([GDK-460]–[GDK-483], [GDK-486], [GDK-489],
  [GDK-504], [GDK-446], [GDK-447], [GDK-448], [GDK-451], [GDK-454],
  [GDK-456]).
- **A failed page comment speaks the catalog, not the wire** ([GDK-637]).
  The page composer caught its own POST and painted `e.message` — often
  `credential_required` or a raw status line — in a color token that does
  not exist, so the error rendered as body copy. Posting now lives on the
  write store beside issue comments (same toast, same way out), and a new
  source gate proves every `text-*` color class names a real `@theme` token.
- **The v0.17 audit sweep**: empty states, terms and affordances line up
  ([GDK-548], [GDK-559], [GDK-572]), sync and search failures read as
  sentences, not wire codes ([GDK-566], [GDK-549]), a saved view arms before
  delete and a partly-applied view says so every time it is named
  ([GDK-567], [GDK-573], [GDK-504]). That sweep also broke the bulk bar's
  Deselect button — the tag closed early and its attributes rendered as
  text, which svelte-check cannot see — caught by the next audit and fixed
  with an e2e that pins the button's exact accessible name and click
  ([GDK-602]).
- **Raycast**: saved views and people ride the deeplinks the app already
  answers ([GDK-172]). **Desktop**: mailto links reach the mail client from
  the webview ([GDK-339]).

### MCP and the CLI surface

- **Batch writes, with an honest per-key envelope** ([GDK-501],
  [GDK-428]). `--batch -` (JSON lines on stdin, create's idiom) now works
  on comment, transition, assign, and edit — one shared engine, capped at
  50 lines per call as an origin courtesy. A failing line does not stop
  the batch: every line is tried, and the output is one row per key —
  `key, ok, changed, error` as TSV or `--json` lines — with a non-zero
  exit if anything failed, so an agent never reconstructs "what actually
  happened" from a half-dead shell loop. Mass labeling is now
  `{"key":…}` lines plus `--label +x` once. `transition --batch
  --dry-run` prints the transition id each line would fire (or the
  already-there no-op) and writes nothing — the preview runs the same
  core code as the real write, so it cannot drift.
- **Closing is one round trip, and retrying it is free** ([GDK-500]).
  `gadak close KEY -m "why"` transitions to the done category and posts
  the evidence comment in the same origin POST — it is sugar over
  `transition KEY done`, with no logic of its own. The real fix is
  underneath, in the shared transition core: asking for a status category
  the issue is already in used to be an error ("no transition matching"),
  which read as a false failure to every retrying agent. It is a no-op
  success now — exit 0, `changed: false`, and nothing written (not even
  the comment, so a retry can never double-post) — on the CLI and REST
  alike, the two surfaces that share the core. A named transition that
  does not exist still errors, and
  reopening stays `transition KEY inprogress`; there is no `gadak reopen`.
- **Picking work is a verb now** ([GDK-503]). The best way to pick the
  next issue used to be retyping the same `issues_full` query — and its
  one trap (display names localize; `status = 'In Progress'` can be zero
  rows) lived only in the docs. `gadak recipes save NAME "select …"`
  names a read-only mirror query in `local.db` (validated by running it
  once at save time), `recipes run NAME` prints it through exactly
  `gadak sql`'s path and formats, `show` round-trips the source, and
  `gadak next` runs the recipe named `next` — a report, not occupancy.
  No private ranking engine: rank still comes from `priority_rank` and
  `status_category`. And the trap now lives where the fingers are: the
  `sql`, `search`, and `recipes` help all say to key on categories and
  ranks, not display names.
- **The no-op holds on self-loop workflows too** ([GDK-632]). Dogfooding
  the batch on a real site found the gap the fakes could not: a workflow
  that keeps a done→done transition available while the issue is already
  done let a retry fire the transition again — and would have re-posted
  its comment. A category target now checks the origin's current status
  before firing, not only after a pick miss; that costs a category write
  one status read, and named targets never pay it. The retry that exposed
  it now answers `changed: false` on the same site.
- **CLI reads leave a trail, and `gadak recents` walks it back**
  ([GDK-502]). `gadak issue` and `gadak search` now append the same
  visit/search rows the UI has always written to `local.db` — best-effort,
  so a history that cannot take the row never fails the read — and the new
  `recents` verb lists the keys this workspace touched, newest first,
  deduped. For an agent whose context was just compacted, it is the one
  command that says which keys were in play; the skill teaches it as the
  first command after a compaction. Visits stay local: nothing here goes
  to the origin.
- **MCP reaches read parity**: `status` shows frozen, `issue` carries
  `description_text` and dev links ([GDK-568], [GDK-569], [GDK-552]).
- **`gadak sql` warns on a display-name zero-row** like MCP already does
  ([GDK-553]).
- **Help usage and examples reflow to multiple lines**, and the top-level
  usage catches up with the verbs that exist ([GDK-575], [GDK-544]). The
  skill teaches this cycle's write flags and stops saying a field cannot
  exist ([GDK-544], [GDK-571]); earlier, the front door learned to name the
  third way in ([GDK-457], [GDK-458]).
- **The docs catch up to the code**: the outbound list, the dev-link
  section, and the teaching copy ([GDK-557], [GDK-547]). A GDK key is a link
  now, in every doc a reader opens.

### Network seams, audited

- **The Linear proxy fetches only `uploads.linear.app`** — redirects
  included, and the API key rides only there ([GDK-427], [GDK-558],
  [GDK-560]).
- **An empty host is a non-loopback bind**, so `serve` demands
  `--allow-remote` for it like any other exposure ([GDK-542]).
- **A kicked sync pulls Linear too, guards against overlapping runs, and
  refuses a rebind** ([GDK-563], [GDK-574], [GDK-561]); the next-tick
  deadline stops racing loaded runners ([GDK-534]), and a Watch that exits
  clears its flag so it can be revived ([GDK-541]).
- **The desktop app's sync survives a credential error** ([GDK-663]). Watch
  deliberately returns on fatal auth (no hot-loop on a 401), and serve, MCP
  and workspace mounts each wrapped it in their own restart loop — the
  desktop app alone called it bare, so one rejected credential silently
  stopped sync for the life of the process. The re-entry policy now has one
  owner, `syncer.WatchLoop`, all four callers use it, and an AST test pins
  the desktop to the loop.
- **The boot sequence has one owner** ([GDK-664]). `gadak serve` and the
  desktop app each carried their own copy of the same startup — config,
  store, attachment cache, standalone persist, advertise, watch loops,
  version stamp — and the copies kept drifting apart one incident at a
  time ([GDK-644], [GDK-658], [GDK-663] were all two-copies bugs). The
  sequence now lives in `internal/apprun`, and the two intentional order
  differences survive as options rather than a merged order: serve's
  live-owner check still runs before the persist lock ([GDK-468]), and the
  desktop still lets wails single-instance exit a second launch before
  persist is taken ([GDK-658]). Sequence contracts moved from per-main AST
  tests into apprun's own tests; the desktop tests now pin only that the
  caller calls in.
- `warnIfStale` reads the caller's connection instead of opening a second
  one ([GDK-314]), skill detection resolves home the way the installer does
  ([GDK-352]), and `ci-status` counts only default-branch push runs as the
  verdict ([GDK-432]).
- **A wrong-typed write field is a refusal, not an empty string**
  ([GDK-643]). The Linear adapter's field extractors swallowed type
  mismatches (`s, _ := v.(string)`), so a malformed value could reach
  Linear as `""`; they now return an error naming the field and the type
  they got, before any network call. The Linear API key became a private
  field assigned only by the constructor — a `%+v` on the client can no
  longer print it — and `jira.New`'s comment now matches the gate that
  actually guards it.
- **The Linear storage PUT rides the client with the 60-second timeout**
  instead of `http.DefaultClient`, so a stalled storage host can no longer
  hang `gadak attach` forever ([GDK-636]).
- **The desktop pin moves to wails `v3.0.0-beta.12`** ([GDK-639]). Our own
  #6000 is in the pin now, so a Linux single-argument `gadak://` launch
  defers to the `ApplicationLaunchedWithUrl` event like Windows instead of
  hand-applying argv, and the Close Tab menu finds the Window submenu by
  role, not by its English label.
- **A Linear-only workspace is a configured workspace** ([GDK-654],
  [GDK-655], [GDK-656]). "Is this configured?" now has one owner that counts
  a Linear API key: status and every verb stop warning `not configured — run
  gadak init` on a workspace whose writes were succeeding, `status --json`'s
  top-level `watermark`/`sync_count` follow the issue source that actually
  ran (per-source rows under `sources.jira` / `sources.linear`), and a
  leftover never-synced Jira row no longer poisons freshness. `create`
  routes to Linear — by the mirrored project's source, or always on a
  Linear-only workspace — instead of demanding a Jira init, and `page
  create` says the real reason (Linear has no wiki). And the assign hint's
  own advice works: a UUID from `issues.assignee_id` now reaches Linear's
  user lookup as an id. The Jira-family gates did not loosen: a Linear key
  is not a site token anywhere Jira, Confluence or the standalone origin is
  the one being asked.
- **`edit --fix-version <name>` works on standalone workspaces**
  ([GDK-662]). Name resolution asks the origin for the project's version
  catalog, and issuetap answered its honest 501 — a hard error — while its
  store already derived exactly that catalog from the issues. issuetap now
  serves `GET /project/{key}/versions` and `/components` from the derived
  catalog (sorted, missing project 404s, `/roles` stays 501), so the same
  client code that works against Cloud works against standalone.
- **On standalone, `--fix-version +name` creates the version it names**
  ([GDK-678]). Fixing the catalog exposed the next step: a name that is
  not in the catalog yet was still a refusal, and since the catalog is
  derived from the issues, a brand-new version is *always* not in it — so
  planning a release on a fresh workspace meant a REST detour for the first
  one and, once one existed, a `400 unknown fixVersions` for the second.
  Whether a name may be minted is now the origin's capability, not a
  guess: `CreatesVersionsByName` sits on the version-catalog face, true for
  issuetap (in-process, routed through a live serve, or paired) and false
  for Cloud Jira — where refusing is correct, because creating a version
  there is a separate project-admin permission. An id that misses still
  refuses everywhere: an id is a pointer, not a request.
- **Linear's rate limit is a retry, not a death** ([GDK-263]). Linear
  reports a drained bucket as HTTP 400 or 200 with a GraphQL `RATELIMITED`
  code — not a 429 — and the client used to die on the first one. One
  function now owns the retry decision (HTTP status plus GraphQL code):
  `RATELIMITED` waits and retries within the existing budget, mutations
  included, because Linear documents it as a pre-execution rejection. A
  GraphQL error now surfaces its messages only, so an `extensions` blob can
  never carry a credential into the error string. And labels and
  attachments grew the `Complete*` follow-ups comments already had — an
  issue with more than one inline page mirrors all of them.

### The public backlog

- **gadak's own backlog is published at `/gadak/backlog/`** ([GDK-389]),
  with one owner for "is this a tenant hostname" ([GDK-431]) and a real
  snapshot for returning visitors — the fake delta is gone ([GDK-440]).
- **A GDK key on a public surface is never a dangling pointer** ([GDK-269]).
  Commits, comments and docs cite `GDK-nnn` freely, but a reader could not
  resolve one the snapshot did not carry. A doc-check now fails when a key
  cited on any tracked public surface is neither on the published backlog
  nor in a private-key allowlist with a written reason — measured red on 21
  keys before the fix, all 21 published. Rewriting the comments that
  delegate their *reason* to a key is its own, bigger issue ([GDK-633]).
- **The snapshot index and its detail pages ship as one set** ([GDK-634]).
  Publishing that gate found the next gap the same day: a regenerated
  snapshot writes new per-issue detail JSONs as untracked files, and a
  commit that carries the index without them publishes keys whose detail
  is a 404 — which is exactly how the 21 keys above first went out. A
  second doc-check now compares the index's keys against the
  *git-tracked* detail files, both directions, and it caught its first
  real case (these very release notes' issues) before the commit that
  ships it.
- **A link's target must itself be published** ([GDK-675]). The scrub
  rebuilt every surface from a whitelist except one: `linked_issues` passed
  through verbatim, and a link entry carries its target's key and summary —
  so one relates-link from a published issue to a private one put that
  issue's summary on the public surface. The CI gate caught it only by
  accident (a link's `"type"` collided with the ADF node allowlist). Links
  are now rebuilt too — published targets only, whitelisted fields only —
  the gate names the assertion instead of tripping over it, and doc-checks
  runs the scrub gate on the *committed* snapshot, so a red snapshot can no
  longer ride a green commit.
- **The published snapshot is one file now** ([GDK-669]). The public
  backlog rode in git as 613 exploded JSON files, and every feature commit
  carried two to four of them as freight — the "forgot to `git add` the new
  detail files" class needed three separate doc-checks to police. The
  tracked form is a single `examples/backlog-snapshot.tar.gz` (380 KB for
  what was 1.48 MB), packed from the exact bytes the viewer will fetch, with
  a one-line `MANIFEST` (`tar -xOf … MANIFEST` answers "what's in it"
  without jq). Pages and the local hosted build unpack it at build time;
  the viewer's `detail/<KEY>.json` URLs are unchanged. The three gates —
  no dangling cited key ([GDK-269]), index/detail consistency ([GDK-634]),
  the scrub on the committed snapshot ([GDK-675]) — all read the archive
  now, each measured red on an injected violation before going green.
- **The public backlog publishes what each issue actually says** ([GDK-430]).
  The scrub treated every content surface as one axis and forced descriptions to
  null with the comments and the attachments, so the published page was a list
  of headlines: the `file:line`, the failure scenario and the fix all live in the
  description. They are not one axis — comments and history carry other people's
  words and actions, a description carries only the reporter's — so `--scrub`
  gained a `--keep-description` door that opens nothing else, and the whitelist
  stays closed for a caller that says nothing. The leak gate grew with the new
  surface: a home directory path fails it, an address that is neither a
  documentation placeholder nor the maintainer's own published feedback channel
  fails it, and a description carrying anything but paragraphs and text — an ADF
  `mention` is an account id and a display name, an `inlineCard` is a URL —
  fails it too

- **Converting a workspace stops handing your personal rows to the new site**
  ([GDK-418]). An issue key is not globally unique — `init --standalone` seeds
  project `STD` and a real site's project can be `STD` too — so a row naming the
  old origin's `STD-1` did not go stale when the origin was replaced, it rebound
  to whatever the new site had at that key. Plugin enrichments, feed read marks
  (so a brand-new issue arrived already read), per-project field usage, sync
  runs and the picker's cache of account and issue-type ids all survived a
  conversion that had already dropped the mirror they described. What conversion
  removes is now derived from a classification every table carries, and a test
  enumerates `sqlite_master` and fails on a table missing from it — the previous
  hand-maintained list is exactly how four tables added by later migrations
  slipped past it. Saved views survive, because you wrote them, and a view whose
  query names a retired project is reported by name instead of silently reading
  the new site's. Visit and search history survives too, stamped with the
  generation it was recorded in: the timeline shows the current one, and the
  retired rows stay readable with `gadak sql`

### A front door at gadak.dev

- **The site has an apex, and three doors behind it** ([GDK-676]). Everything
  public was reachable only as `midagedev.github.io/gadak/…`, so an account
  name sat inside every URL the project printed — and pointing a domain at it
  did not just work: GitHub redirects a project path to the custom domain with
  the *repo segment dropped*, so `gadak.dev` was served the `index.html` of an
  app whose assets were all built for `/gadak/`, and the front page of the
  product was a broken page. The published tree is rooted at `/` now: a
  landing page at the apex, the live demo at `/demo/`, the public backlog at
  `/backlog/`. Each app is built against its own base path, because
  `basePath()` resolves at build time and cannot be re-pointed afterwards.
  The landing page has nothing to load — no script, no web font, no
  analytics, the product's own paper-and-indigo tokens inlined — and a
  build-time gate fails the site when its markup stops linking both apps: a
  front door that reaches neither is worse than no front door at all.

### Windows, unsigned but verifiable

- **The Windows warning has a page now** ([GDK-211]).
  `docs/WINDOWS-SIGNING.md` says what the release actually is — no
  Authenticode signature on either Windows zip, SmartScreen and Smart App
  Control behaving exactly as they should — and shows how to verify the file
  you downloaded: the CLI zip against `checksums.txt`, the desktop zip
  against the GitHub Releases API's per-asset digest (measured to match the
  checksums file where both exist). It does not claim the unsigned zip is
  safe; it shows which bytes you have. The signing plan is SignPath
  Foundation, and the application draft with its requirements-gap table
  lives in `docs/runbooks/signpath-application.md` — one gap (PE
  `VERSIONINFO` metadata) must land before the first signed tag.


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
[GDK-71]: https://gadak.dev/backlog/#/?ks=GDK-71
[GDK-76]: https://gadak.dev/backlog/#/?ks=GDK-76
[GDK-78]: https://gadak.dev/backlog/#/?ks=GDK-78
[GDK-81]: https://gadak.dev/backlog/#/?ks=GDK-81
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
[GDK-100]: https://gadak.dev/backlog/#/?ks=GDK-100
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
[GDK-123]: https://gadak.dev/backlog/#/?ks=GDK-123
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
[GDK-172]: https://gadak.dev/backlog/#/?ks=GDK-172
[GDK-173]: https://gadak.dev/backlog/#/?ks=GDK-173
[GDK-177]: https://gadak.dev/backlog/#/?ks=GDK-177
[GDK-178]: https://gadak.dev/backlog/#/?ks=GDK-178
[GDK-180]: https://gadak.dev/backlog/#/?ks=GDK-180
[GDK-181]: https://gadak.dev/backlog/#/?ks=GDK-181
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
[GDK-200]: https://gadak.dev/backlog/#/?ks=GDK-200
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
[GDK-255]: https://gadak.dev/backlog/#/?ks=GDK-255
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
[GDK-339]: https://gadak.dev/backlog/#/?ks=GDK-339
[GDK-340]: https://gadak.dev/backlog/#/?ks=GDK-340
[GDK-341]: https://gadak.dev/backlog/#/?ks=GDK-341
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
[GDK-352]: https://gadak.dev/backlog/#/?ks=GDK-352
[GDK-353]: https://gadak.dev/backlog/#/?ks=GDK-353
[GDK-354]: https://gadak.dev/backlog/#/?ks=GDK-354
[GDK-359]: https://gadak.dev/backlog/#/?ks=GDK-359
[GDK-360]: https://gadak.dev/backlog/#/?ks=GDK-360
[GDK-361]: https://gadak.dev/backlog/#/?ks=GDK-361
[GDK-363]: https://gadak.dev/backlog/#/?ks=GDK-363
[GDK-364]: https://gadak.dev/backlog/#/?ks=GDK-364
[GDK-365]: https://gadak.dev/backlog/#/?ks=GDK-365
[GDK-366]: https://gadak.dev/backlog/#/?ks=GDK-366
[GDK-367]: https://gadak.dev/backlog/#/?ks=GDK-367
[GDK-368]: https://gadak.dev/backlog/#/?ks=GDK-368
[GDK-369]: https://gadak.dev/backlog/#/?ks=GDK-369
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
[GDK-428]: https://gadak.dev/backlog/#/?ks=GDK-428
[GDK-430]: https://gadak.dev/backlog/#/?ks=GDK-430
[GDK-431]: https://gadak.dev/backlog/#/?ks=GDK-431
[GDK-432]: https://gadak.dev/backlog/#/?ks=GDK-432
[GDK-433]: https://gadak.dev/backlog/#/?ks=GDK-433
[GDK-434]: https://gadak.dev/backlog/#/?ks=GDK-434
[GDK-435]: https://gadak.dev/backlog/#/?ks=GDK-435
[GDK-437]: https://gadak.dev/backlog/#/?ks=GDK-437
[GDK-438]: https://gadak.dev/backlog/#/?ks=GDK-438
[GDK-440]: https://gadak.dev/backlog/#/?ks=GDK-440
[GDK-441]: https://gadak.dev/backlog/#/?ks=GDK-441
[GDK-442]: https://gadak.dev/backlog/#/?ks=GDK-442
[GDK-443]: https://gadak.dev/backlog/#/?ks=GDK-443
[GDK-444]: https://gadak.dev/backlog/#/?ks=GDK-444
[GDK-446]: https://gadak.dev/backlog/#/?ks=GDK-446
[GDK-447]: https://gadak.dev/backlog/#/?ks=GDK-447
[GDK-448]: https://gadak.dev/backlog/#/?ks=GDK-448
[GDK-449]: https://gadak.dev/backlog/#/?ks=GDK-449
[GDK-450]: https://gadak.dev/backlog/#/?ks=GDK-450
[GDK-451]: https://gadak.dev/backlog/#/?ks=GDK-451
[GDK-452]: https://gadak.dev/backlog/#/?ks=GDK-452
[GDK-453]: https://gadak.dev/backlog/#/?ks=GDK-453
[GDK-454]: https://gadak.dev/backlog/#/?ks=GDK-454
[GDK-455]: https://gadak.dev/backlog/#/?ks=GDK-455
[GDK-456]: https://gadak.dev/backlog/#/?ks=GDK-456
[GDK-457]: https://gadak.dev/backlog/#/?ks=GDK-457
[GDK-458]: https://gadak.dev/backlog/#/?ks=GDK-458
[GDK-460]: https://gadak.dev/backlog/#/?ks=GDK-460
[GDK-468]: https://gadak.dev/backlog/#/?ks=GDK-468
[GDK-483]: https://gadak.dev/backlog/#/?ks=GDK-483
[GDK-484]: https://gadak.dev/backlog/#/?ks=GDK-484
[GDK-485]: https://gadak.dev/backlog/#/?ks=GDK-485
[GDK-486]: https://gadak.dev/backlog/#/?ks=GDK-486
[GDK-489]: https://gadak.dev/backlog/#/?ks=GDK-489
[GDK-490]: https://gadak.dev/backlog/#/?ks=GDK-490
[GDK-495]: https://gadak.dev/backlog/#/?ks=GDK-495
[GDK-496]: https://gadak.dev/backlog/#/?ks=GDK-496
[GDK-497]: https://gadak.dev/backlog/#/?ks=GDK-497
[GDK-498]: https://gadak.dev/backlog/#/?ks=GDK-498
[GDK-500]: https://gadak.dev/backlog/#/?ks=GDK-500
[GDK-501]: https://gadak.dev/backlog/#/?ks=GDK-501
[GDK-502]: https://gadak.dev/backlog/#/?ks=GDK-502
[GDK-503]: https://gadak.dev/backlog/#/?ks=GDK-503
[GDK-504]: https://gadak.dev/backlog/#/?ks=GDK-504
[GDK-505]: https://gadak.dev/backlog/#/?ks=GDK-505
[GDK-509]: https://gadak.dev/backlog/#/?ks=GDK-509
[GDK-510]: https://gadak.dev/backlog/#/?ks=GDK-510
[GDK-511]: https://gadak.dev/backlog/#/?ks=GDK-511
[GDK-512]: https://gadak.dev/backlog/#/?ks=GDK-512
[GDK-513]: https://gadak.dev/backlog/#/?ks=GDK-513
[GDK-514]: https://gadak.dev/backlog/#/?ks=GDK-514
[GDK-515]: https://gadak.dev/backlog/#/?ks=GDK-515
[GDK-516]: https://gadak.dev/backlog/#/?ks=GDK-516
[GDK-517]: https://gadak.dev/backlog/#/?ks=GDK-517
[GDK-518]: https://gadak.dev/backlog/#/?ks=GDK-518
[GDK-519]: https://gadak.dev/backlog/#/?ks=GDK-519
[GDK-520]: https://gadak.dev/backlog/#/?ks=GDK-520
[GDK-521]: https://gadak.dev/backlog/#/?ks=GDK-521
[GDK-522]: https://gadak.dev/backlog/#/?ks=GDK-522
[GDK-526]: https://gadak.dev/backlog/#/?ks=GDK-526
[GDK-527]: https://gadak.dev/backlog/#/?ks=GDK-527
[GDK-528]: https://gadak.dev/backlog/#/?ks=GDK-528
[GDK-531]: https://gadak.dev/backlog/#/?ks=GDK-531
[GDK-532]: https://gadak.dev/backlog/#/?ks=GDK-532
[GDK-534]: https://gadak.dev/backlog/#/?ks=GDK-534
[GDK-536]: https://gadak.dev/backlog/#/?ks=GDK-536
[GDK-537]: https://gadak.dev/backlog/#/?ks=GDK-537
[GDK-538]: https://gadak.dev/backlog/#/?ks=GDK-538
[GDK-539]: https://gadak.dev/backlog/#/?ks=GDK-539
[GDK-540]: https://gadak.dev/backlog/#/?ks=GDK-540
[GDK-541]: https://gadak.dev/backlog/#/?ks=GDK-541
[GDK-542]: https://gadak.dev/backlog/#/?ks=GDK-542
[GDK-543]: https://gadak.dev/backlog/#/?ks=GDK-543
[GDK-544]: https://gadak.dev/backlog/#/?ks=GDK-544
[GDK-545]: https://gadak.dev/backlog/#/?ks=GDK-545
[GDK-546]: https://gadak.dev/backlog/#/?ks=GDK-546
[GDK-547]: https://gadak.dev/backlog/#/?ks=GDK-547
[GDK-548]: https://gadak.dev/backlog/#/?ks=GDK-548
[GDK-549]: https://gadak.dev/backlog/#/?ks=GDK-549
[GDK-551]: https://gadak.dev/backlog/#/?ks=GDK-551
[GDK-552]: https://gadak.dev/backlog/#/?ks=GDK-552
[GDK-553]: https://gadak.dev/backlog/#/?ks=GDK-553
[GDK-554]: https://gadak.dev/backlog/#/?ks=GDK-554
[GDK-555]: https://gadak.dev/backlog/#/?ks=GDK-555
[GDK-556]: https://gadak.dev/backlog/#/?ks=GDK-556
[GDK-557]: https://gadak.dev/backlog/#/?ks=GDK-557
[GDK-558]: https://gadak.dev/backlog/#/?ks=GDK-558
[GDK-559]: https://gadak.dev/backlog/#/?ks=GDK-559
[GDK-560]: https://gadak.dev/backlog/#/?ks=GDK-560
[GDK-561]: https://gadak.dev/backlog/#/?ks=GDK-561
[GDK-562]: https://gadak.dev/backlog/#/?ks=GDK-562
[GDK-563]: https://gadak.dev/backlog/#/?ks=GDK-563
[GDK-564]: https://gadak.dev/backlog/#/?ks=GDK-564
[GDK-565]: https://gadak.dev/backlog/#/?ks=GDK-565
[GDK-566]: https://gadak.dev/backlog/#/?ks=GDK-566
[GDK-567]: https://gadak.dev/backlog/#/?ks=GDK-567
[GDK-568]: https://gadak.dev/backlog/#/?ks=GDK-568
[GDK-569]: https://gadak.dev/backlog/#/?ks=GDK-569
[GDK-570]: https://gadak.dev/backlog/#/?ks=GDK-570
[GDK-571]: https://gadak.dev/backlog/#/?ks=GDK-571
[GDK-572]: https://gadak.dev/backlog/#/?ks=GDK-572
[GDK-573]: https://gadak.dev/backlog/#/?ks=GDK-573
[GDK-574]: https://gadak.dev/backlog/#/?ks=GDK-574
[GDK-575]: https://gadak.dev/backlog/#/?ks=GDK-575
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
[GDK-618]: https://gadak.dev/backlog/#/?ks=GDK-618
[GDK-619]: https://gadak.dev/backlog/#/?ks=GDK-619
[GDK-620]: https://gadak.dev/backlog/#/?ks=GDK-620
[GDK-621]: https://gadak.dev/backlog/#/?ks=GDK-621
[GDK-626]: https://gadak.dev/backlog/#/?ks=GDK-626
[GDK-632]: https://gadak.dev/backlog/#/?ks=GDK-632
[GDK-633]: https://gadak.dev/backlog/#/?ks=GDK-633
[GDK-634]: https://gadak.dev/backlog/#/?ks=GDK-634
[GDK-635]: https://gadak.dev/backlog/#/?ks=GDK-635
[GDK-636]: https://gadak.dev/backlog/#/?ks=GDK-636
[GDK-637]: https://gadak.dev/backlog/#/?ks=GDK-637
[GDK-639]: https://gadak.dev/backlog/#/?ks=GDK-639
[GDK-641]: https://gadak.dev/backlog/#/?ks=GDK-641
[GDK-642]: https://gadak.dev/backlog/#/?ks=GDK-642
[GDK-643]: https://gadak.dev/backlog/#/?ks=GDK-643
[GDK-644]: https://gadak.dev/backlog/#/?ks=GDK-644
[GDK-645]: https://gadak.dev/backlog/#/?ks=GDK-645
[GDK-647]: https://gadak.dev/backlog/#/?ks=GDK-647
[GDK-648]: https://gadak.dev/backlog/#/?ks=GDK-648
[GDK-649]: https://gadak.dev/backlog/#/?ks=GDK-649
[GDK-650]: https://gadak.dev/backlog/#/?ks=GDK-650
[GDK-651]: https://gadak.dev/backlog/#/?ks=GDK-651
[GDK-652]: https://gadak.dev/backlog/#/?ks=GDK-652
[GDK-654]: https://gadak.dev/backlog/#/?ks=GDK-654
[GDK-655]: https://gadak.dev/backlog/#/?ks=GDK-655
[GDK-656]: https://gadak.dev/backlog/#/?ks=GDK-656
[GDK-658]: https://gadak.dev/backlog/#/?ks=GDK-658
[GDK-662]: https://gadak.dev/backlog/#/?ks=GDK-662
[GDK-663]: https://gadak.dev/backlog/#/?ks=GDK-663
[GDK-664]: https://gadak.dev/backlog/#/?ks=GDK-664
[GDK-665]: https://gadak.dev/backlog/#/?ks=GDK-665
[GDK-668]: https://gadak.dev/backlog/#/?ks=GDK-668
[GDK-669]: https://gadak.dev/backlog/#/?ks=GDK-669
[GDK-671]: https://gadak.dev/backlog/#/?ks=GDK-671
[GDK-674]: https://gadak.dev/backlog/#/?ks=GDK-674
[GDK-675]: https://gadak.dev/backlog/#/?ks=GDK-675
[GDK-676]: https://gadak.dev/backlog/#/?ks=GDK-676
[GDK-677]: https://gadak.dev/backlog/#/?ks=GDK-677
[GDK-678]: https://gadak.dev/backlog/#/?ks=GDK-678
[GDK-679]: https://gadak.dev/backlog/#/?ks=GDK-679
[GDK-680]: https://gadak.dev/backlog/#/?ks=GDK-680
