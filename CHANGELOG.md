# Changelog

<sub>English · <a href="CHANGELOG.ko.md">한국어</a></sub>

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
- **`gadak link A B --type blocks` writes an issue link** ([GDK-19]).

### Write verbs an agent can trust

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

### Workspaces and pairing

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

- **Four type sizes, and nothing between them** ([GDK-129]). The screen
  had drifted 198 arbitrary pixel sizes past the declared scale — 190 of
  them `12px`, one pixel off both of its neighbours, which reads as noise
  rather than hierarchy. Every one is absorbed into the four tokens by
  role (metadata to micro, reading text to body), dialog titles join panel
  headings at the top step, the wordmark gets one owner instead of two
  sizes, and the theme gate now fails on any `text-[Npx]` utility so the
  fifth size cannot come back.
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
- `warnIfStale` reads the caller's connection instead of opening a second
  one ([GDK-314]), skill detection resolves home the way the installer does
  ([GDK-352]), and `ci-status` counts only default-branch push runs as the
  verdict ([GDK-432]).

### The public backlog

- **gadak's own backlog is published at `/gadak/backlog/`** ([GDK-389]),
  with one owner for "is this a tenant hostname" ([GDK-431]) and a real
  snapshot for returning visitors — the fake delta is gone ([GDK-440]).
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


## v0.16.1 — 2026-08-20

The release that finishes what 0.16 started. Standalone shipped as a working
origin and then spent a day proving how many ways two processes can disagree
about who owns a file; Linear arrived as a read-only client with nowhere to
plug in; and the documents an agent actually reads never learned the word
"standalone". All three are closed here.

### A third tracker, and it writes

- **Linear is a source, not a plan** ([GDK-263], [GDK-360], [GDK-361]). A `"linear"`
  block in the profile's `config.json` (`apiKey`, optional `teamIds`) and
  `gadak sync --source linear` mirrors issues, comments, labels and
  attachments beside Jira and Confluence. Writes route by the mirror's source
  for the key: comment, transition against the team's workflow states
  (id-keyed, never by display name), summary/priority/due-date edits,
  assign/unassign, and file uploads all pass through Linear's API and refresh
  the mirror row. What Linear cannot do yet — label edits, inline comment
  media, clearing a due date, state history — refuses honestly instead of
  half-applying, and comment bodies stay markdown rather than being stuffed
  into an ADF column they do not fit
  ([`internal/linear/MAPPING.md`](internal/linear/MAPPING.md)).
- **The write seam is an interface** ([GDK-359]). Jira, the standalone origin,
  and Linear now speak one vocabulary, so a verb added to one surface is not
  three implementations of the same guard.
- **A Linear-only profile syncs** ([GDK-361]). The credential gate is per source;
  a profile with no Atlassian token is no longer told it has nothing to do. A
  cross-source key collision no longer refuses a Jira write ([GDK-263] review).

### The wiki stops being read-only

- **Page edit, comment, and create write through the origin** ([GDK-380],
  [GDK-381], [GDK-382]) — `gadak page edit|comment|create` and the matching REST
  verbs, Confluence Cloud on a connected workspace and the in-process origin
  on a standalone one. Page ids get the same namespace mirror ids got, so a
  standalone page cannot collide with a real site's ([GDK-344]).

### One owner for the standalone origin

- **The desktop app advertises its origin too** ([GDK-340]), closing the half of
  [GDK-333] that shipped open: an app and a CLI could both hold the persist file.
- **The persist lock is the single truth for who may embed** ([GDK-343]), an
  acknowledged write is on disk before the response ([GDK-342]), a durable
  persist failure fails the write rather than being swallowed ([GDK-346]), and
  the fatal path flushes too ([GDK-348]).
- **Standalone failures stop masquerading as `credential_required`** ([GDK-345])
  — an origin that is busy says so, with its own toast, and the conversion
  copy says what conversion actually does ([GDK-347]).
- **Mirror ids get their own namespace and conversion drops the old mirror
  whole** ([GDK-241]), taking watches and favorites with it ([GDK-344]).

### The agent surfaces learn standalone

- **The embedded skill knows the mode exists** ([GDK-239], [GDK-363]). It could
  not before: the word appeared nowhere in the skill or `AGENTS.md`, the
  profile rule taught an agent to report an empty `site_host` and stop — which
  is what a healthy standalone profile looks like — and `AGENTS.md` claimed
  writes fail without a credential, which is false there. Kind now comes from
  `doctor`'s `workspace.kind`, confirm-before-writing is scoped to connected
  (a standalone write is a file on this machine), and the create path is
  spelled out where an agent reads it.
- **The CLI says which origin it means** ([GDK-364], [GDK-366], [GDK-371]). Write
  verbs stopped claiming every write lands "on Jira (needs a credential)",
  `init` stopped hiding `--standalone` from the usage line, and `serve`'s help
  matches when it actually syncs.
- **`transition` names the identifier the rules require** ([GDK-365]). The
  refusal listed target statuses by display name only — the one key this
  product tells everyone not to use — and now carries each target's
  `status_id`. `transition` also accepts the `status_id` the read path hands
  out ([GDK-313]).
- **kind and persist are on the agent's preflight** ([GDK-368], [GDK-376]).
  `status --json` and each `profiles --json` row carry `kind`, from the same
  `origin.Describe` doctor uses, and standalone `init` prints the persist path
  — the file to back up — where it is created.
- **The stale-mirror warning is closed at the source** ([GDK-367]). Standalone
  `init` fills the mirror, since that sync is in-process; a fill that fails
  warns instead of failing a workspace that already exists.
- **`issues_full` exposes `description_text`** ([GDK-312]) — the plain text was
  already in the mirror, and the view now hands it over.

### Documents that stop contradicting the product

- **The network gets its own page** ([GDK-601]): `docs/NETWORK.md` — every
  outbound connection with its trigger and off switch, the four surfaces
  that keep a mirror fresh without an agent managing sync, and the
  deliberate direction: pairing over a tailnet, and a team sharing one
  standalone workspace. Linked from the README; `SECURITY.md` stays the
  enforcement record. Every claim was adversarially fact-checked against
  the code before it shipped, and the stale-warning verb list in
  `AGENT_ACCESS.md` was corrected on the way (`sql` warns too, since
  [GDK-598]).
- The install front door admits standalone ([GDK-271]): `INSTALL.md` no longer
  opens with "Atlassian Cloud only", and both it and the README carry the
  four-line no-account quickstart.
- `CONCEPT.md` teaches origins rather than "writes cannot be local" ([GDK-372]),
  the FAQ stops offering `rm -rf ~/.gadak` to users whose original lives there
  ([GDK-373]), `PROMISES.md` gains a ninth promise — the standalone origin is
  one plaintext YAML file, with a command that proves it — `MAINTENANCE.md`
  stops refusing a Windows shell it shipped ([GDK-374]), and `export`/`import`
  finally has the paragraph its verb has had since 0.14 ([GDK-375]).
- `doc-checks.sh` gains three gates so the front door cannot drift back.
- The Go test suite stopped pointing its fixture credential at a live
  Atlassian host ([GDK-304]).

### The second audit, and what it already fixed

A second six-axis read-only audit swept the whole codebase before the tag
([GDK-603]); its findings land as they are fixed. Two defects it caught
shipped the same day ([GDK-602], [GDK-604]), and the structural ones
followed:

- **One owner for SQL comment stripping** ([GDK-605]). The strip-and-read
  -the-first-keyword logic existed three times, and the config group-query
  copy had drifted — `SELECT/*x*/key` collapsed to one token and a
  double-quoted identifier could hide a second statement. `sqlhint` now
  exports the one implementation and both gates call it, so the drifted
  copy's two defects are fixed by deletion.
- **The test suite stops sleeping through refused dials** ([GDK-608]).
  Five tests aimed a fixture at a dead origin and paid production's full
  retry budget — 15 seconds of pure sleep each, 64% of the suite's test
  time and the reason CI's race step was the critical path. A test-owned
  retry seam (production defaults pinned by their own test) turns 75s of
  sleeping into 0.1s; the affected packages' wall clock fell from 52s to
  13s.
- **One policy for private directories** ([GDK-606]). Two functions with
  the same name disagreed about a directory you locked yourself: config
  respected an owner-locked (`0555`) home, store silently chmodded it back
  to writable — drift from the fix that only reached one copy. The one
  implementation now lives in `internal/fsperm`: old `0755` dirs are still
  tightened to `0700`, and a deliberately locked directory stays locked on
  every surface.
- **`gofmt` is a CI gate now** ([GDK-607]). Four files had drifted from
  canonical formatting with nothing to notice; they are reformatted and
  `gofmt -l` failing the build keeps it that way.
- **A bad cursor is an identity, not a sentence** ([GDK-609]). The history
  endpoint decided "this is a 400" by substring-matching the error message —
  reword the message and the client error silently becomes a 500. The store
  now exports `ErrInvalidCursor` and the handler keys on `errors.Is`; a test
  pins each parse-failure branch to the sentinel.
- **Counting issues stopped loading them** ([GDK-610]). The settings runtime
  panel materialized every issue just to take `len()` and sum a column; it
  now asks SQL for the two aggregates and threads the request context
  through. The rewrite surfaced why the naive fix is wrong — page comments
  share the comments table — and the equivalence test pins that divergence
  so the figure the panel shows never quietly changes meaning.
- **One shape for a fatal error** ([GDK-611]). One error path in `main`
  still went through `log.Fatalf`, which hard-codes exit 1 instead of
  routing through the exit-code contract every sibling branch honors. It
  now prints and exits like the rest — today's message is byte-identical,
  but a future coded error keeps its code instead of being flattened.

### The pre-tag audit, closed before the tag

Three read-only audit rounds over this release's own delta ([GDK-393]) filed
30 findings; the ones that survived verification are fixed in this release,
not deferred past it.

- **Linear's mirror carries what the origin has** ([GDK-394], 395, 397, 398,
  399, 405): attachments ride the issue query in (the write half existed
  first — this closes the claim above), the markdown body reaches the CLI
  and UI, `priority_rank` derives from Linear's integer id rather than an
  English label, comment pagination follows the cursor before the child
  list is replaced, a full sync reconciles deletions with the same
  refuse-to-empty guard as Jira, and `open`/attachment bytes follow the
  stored origin URL.
- **Linear's writes speak Linear's vocabulary** ([GDK-396], 401, 403, 406,
  407): per-key priority and user catalogs route by the row's origin, a
  Linear-only profile's UI writes open without a Jira token, `CreateIssue`
  refuses unsupported fields instead of half-creating, assign resolves
  through Linear user search, and the adapter finally has a test ladder.
  A key two sources both mint is now an explicit `key_ambiguous` refusal
  instead of a silent preference ([GDK-400]).
- **Wiki writes are honest under failure** ([GDK-408], 410, 404, 411, 412,
  413): Confluence and Linear rejections map to their own statuses instead
  of `502 jira_unavailable`, page edit takes an optional base version for
  optimistic locking (omitted stays last-write-wins, and the docs say so),
  the REST `adf`/`text` paths gain validation and a `format_loss` guard,
  and the document composer gates on credentials like issue comments do.
- **Standalone conversion has one owner** ([GDK-415], 416, 417, 419, 390):
  the CLI refuses to convert a workspace another process has open (and a
  busy lock names the holder's pid — [GDK-421]), CLI and HTTP conversion
  share one cleanup, the local-data guard counts pages as well as issues,
  and `init --standalone --projects` actually seeds every requested key.
  `gadak project create` grows a standalone workspace by a project at
  runtime, through the origin ([GDK-391]).
- Smaller honesty fixes from the same rounds: `gadak_status` reports the
  workspace kind for shell-less hosts ([GDK-420]), the top-level usage owns
  every command ([GDK-426]), a rejected `create --parent` explains the
  hierarchy rule from the mirror ([GDK-424]), and the ko README/architecture
  docs admit the wiki writes exist ([GDK-409]).

## v0.16.0 — 2026-08-19

The release where gadak stops needing an Atlassian account to be useful,
stops needing a Mac to run, and stops being read-only about the fields
people actually triage by. A workspace can now be standalone — its origin
is a minimal self-hosted Jira that travels with the app — and the issues
in any workspace, connected or standalone, can finally be edited where
they are read: due date, description, priority, assignee, and the custom
fields your site actually uses.

### A workspace without an Atlassian account

- **Standalone workspaces** ([GDK-183]). `gadak` can create a workspace whose
  origin runs in-process — a deliberately minimal self-hosted Jira
  (`issuetap`) instead of an Atlassian site. The mirror rules are unchanged:
  the origin owns the data, the mirror stays a disposable cache, and every
  write goes through the origin. The persist file is the thing you back up.
- **A workspace is bound to one origin** — connecting a credential cannot
  quietly retarget an existing workspace, on the CLI path and the HTTP path
  alike ([GDK-238], [GDK-247]). A different origin is a new workspace, not a
  settings edit.
- **Standalone wikis** hold documents, written through the origin's
  Confluence API like everything else ([GDK-267]); page version history is
  mirrored as stamps, never bodies. The UI says which workspace it is
  looking at and stops asking a standalone one for a token ([GDK-237]).

### Windows and Linux

- **Windows**: a portable pack and an installer path ([GDK-209]), `install-cli`
  that works there, surfaces that tell the truth on that platform, and a
  first-launch fix — the `gadak://` deep link used to be applied twice, the
  first time too early to work ([GDK-293]). A Scoop manifest is checkable
  without Windows ([GDK-246]).
- **Linux**: a pack script symmetric with the dmg one ([GDK-208]), a tarball
  install documented next to brew ([GDK-229]), and an AUR packaging kit with
  a gate against version drift ([GDK-115]).
- **Omarchy**: a bar widget that answers the one question no cloud plugin
  can — what changed in *your* mirror ([GDK-116]) — plus an install recipe
  verified on a real guest ([GDK-225]).

### Issues you can edit where you read them

- **Field editors leave the QA cage** ([GDK-322], [GDK-323]). Inline editing
  existed but only rendered behind a QA feature flag. Editability is now
  decided by the issue's own editmeta: option and multi-select fields get
  the same dropdown grammar as the assignee picker, and the kind matrix
  grows text, number, and date. What a workspace can edit and which values
  are allowed comes from one place — Jira's editmeta on connected
  workspaces, the origin's field registry on standalone ones — so the
  selects and multi-selects show your site's real allowed values.
- **A due date is a row, not just a column** — set it, clear it, from the
  detail panel (the endpoint had existed since [GDK-223] with no UI on top).
- **Descriptions are editable as plain text** ([GDK-82]) — with a guard for
  the case that matters: a description holding tables, media, or marks gets
  a format-loss banner and an explicit "Save as plain text" label before
  anything is destroyed. Simple paragraphs just edit.
- **Priority joins the triage keys** ([GDK-331]): `p` opens a catalog
  priority menu wherever `s`/`a`/`l` already work — bulk bar, cursor row,
  detail. And the list's assignee menu now finds the same people the detail
  picker does — one shared candidate ranking with server-search fallback,
  so a user you can assign in the detail you can assign from the list
  ([GDK-332]).
- **Dates got a read surface first**: a due column, due sorting, date
  filter axes, and one owner for the "which calendar day is this?" question
  so UTC and local stop disagreeing at the KST boundary ([GDK-249], [GDK-250]);
  Jira's refusal of a bad due date is a sentence you can read ([GDK-251]).
- The palette can **create an issue from the typed text** without shadowing
  actions ([GDK-217]), required create fields with obvious answers stop being
  questions ([GDK-218]), and the create dialog says it cannot write instead
  of spinning on Loading ([GDK-302]). Naming an action in the palette now
  wins in every locale — typing `settings` opens Settings even when an
  issue title contains the word, and `,` opens it from anywhere ([GDK-300]).
  Posting a comment finally says it landed ([GDK-301]).

### The demo, and the door

- **The hosted demo opens on the product** ([GDK-335]). The full-screen gate
  page is gone — the issue list is the first paint. Its contents (the
  claim, the brew command, the 60-second video, the repo link) moved into
  an About popover on the demo banner, joined by the feedback channels.
- **The product says how to reach us** ([GDK-336]): a Settings About tab and
  a macOS Help menu carry the same four channels — the GitHub repo, the
  issue tracker, email, and X.

### Updates, without an updater

- Update detection reaches the UI and says the right thing per platform —
  notify-only, no self-update ([GDK-213], [GDK-214]). Release notes render in
  the app, and upgrade instructions have one owner ([GDK-215], [GDK-216]).

### Groups

- **`groupQuery`** classifies `team_group` with one read-only `SELECT`/`WITH`
  over the mirror (`issues_full`, `json_each`, `REGEXP`). Runs when the derived
  view is rebuilt, not on a keystroke. Empty group = unclassified; NULL or a
  missing key falls through to existing `groupRules` and the assignee's member
  group. The query is team-exportable. `groupRules` stays the three-list
  form — do not grow it into a DSL. Settings PUT omits-to-preserve so older
  clients cannot wipe a stored query.

### Linear, measured before it is wired

- A read-only Linear GraphQL client landed as groundwork for a second origin
  kind: viewer, teams, workflow states, cursor-paged issues, with rate-budget
  accounting and dead-credential detection ([GDK-263], [GDK-274]). Its fixtures
  are scrubbed captures from the live API using the exact queries it ships.
  It is deliberately **not wired into workspaces yet** — what a Linear origin
  means for the workspace model is its own decision ([GDK-258]), and running
  two origin kinds side by side is post-0.16 ([GDK-261]).

### The audit wave

The pre-minor full-codebase audit ran again and its findings landed before
the tag, the worst ones first:

- **Localized names stop being keys.** A priority filter keyed on a display
  name was zero rows on a Korean account; status names guessed at
  categories; the create dialog sent a priority by name while the gate that
  should have caught it stayed green ([GDK-275], [GDK-272], [GDK-248], [GDK-161]).
  Status, priority, and type now key on ids and categories everywhere.
- **One cold open stopped serialising everybody** — three mutexes were held
  across disk IO ([GDK-282]); a contended write died instantly because
  `busy_timeout` never saw it ([GDK-305]); a background sync no longer
  outlives the server that started it ([GDK-270]).
- **Six dialogs, one shell contract** ([GDK-297]) — same header, same close
  affordance, a footer that cannot paint over the last row. Onboarding owns
  its pane instead of sitting inside armed app chrome, and its step-4
  counter stops contradicting the sidebar ([GDK-299]). The Korean catalogue
  no longer disagrees with itself inside one header row ([GDK-298]).
- **The mirror's downgrade notice** got a ceiling and advice ([GDK-310]), the
  wiki write surface that was built but never wired got wired ([GDK-267]),
  and the Linear client — read-only by contract — detects a dead credential
  it previously could not see ([GDK-263], [GDK-274]).
- CI stopped lying about infrastructure: a stalled apt mirror, a retry that
  killed apt mid-configure, and an orphaned root apt-get holding the lock
  were each made to fail fast and say which half failed — the installer,
  never the tests ([GDK-308], [GDK-317], [GDK-319]).

### For agents

- Dogfooding friction is a backlog item, not something to route around —
  the write gaps an agent hits are filed as they happen ([GDK-312], [GDK-313],
  [GDK-314], [GDK-315]). The FAQ's claim that agents cannot write the mirror
  matched the code again ([GDK-306]). Decision 0009: CJK mid-compound search
  is app-layer bigrams ([GDK-259]).

## v0.15.2 — 2026-08-17

The release where settings stop being a screen. Every field the dialog
edits is a CLI verb, so an agent can set up a workspace end to end — and
the first thing that travels that way is the look.

### Settings are an agent surface

- **`gadak config list | get | set`** ([GDK-193]). One path→validate table
  behind both the CLI and `PUT /api/settings`, so the two can never disagree
  about what a setting accepts. `gadak config list` prints every editable
  path with its current value; an unknown path exits 64 and prints the list.
  Credentials stay with `gadak init`. The skill documents the verb.
- **Themes live in `config.json`** ([GDK-190]), which is a per-workspace file:
  `gadak --profile oss config set appearance.theme ink` dresses that
  workspace and leaves the others alone. The window writes through the same
  API, so picking a theme in the UI and setting it from a terminal are the
  same act. One browser's `/` and `/w/oss` tabs keep their own look.

### Three darks, and one of them is yours

- **`dark` is a neutral-cool charcoal now** ([GDK-190]). The old ground was
  amber-brown and read as a tint nobody asked for; the ink stays barely warm
  so the window is still paper and ink rather than a grey panel.
- **`ink`** is a new blue-black palette with a cyan-blue accent.
- **`ember`** preserves the previous warm dark byte for byte — if that was
  your look, pick it and nothing changed. `theme-check` now discovers
  palettes from the CSS and holds each to its own ground contract.

### Smaller things that were in the way

- **A bare number finds the issue** ([GDK-186]). Typing `4152` matches
  `CRWN-4152` in any project: the exact number ranks with key-exact, a
  shorter digit run is a number prefix. One code path, so the CLI, the
  Raycast extension, ⌘K's server search and MCP all get it.
- **Settings → Integrations** ([GDK-185], desktop only) lists the surfaces
  gadak installs into — command line tool, Raycast extension, Claude Code
  skill, Claude Desktop MCP — with four-way truth (installed, not
  installed, unknown, failed), the exact command it runs, and a live log.
  The verdict is the stream's final `exit=` line, never silence.
- **The menu stops installing** ([GDK-189]). Tools → Install Command Line
  Tool is gone (it is a row in Integrations now) and the app menu gains
  Settings… ⌘,.
- **The ⌘K palette is never blank** ([GDK-184]). An empty query shows
  recently updated issues under recently viewed, plus saved views — a fresh
  profile opens onto a list. View rows carry a kind glyph and say what they
  open ([GDK-191]).
- The settings dialog stops repeating its **This mirror** block above every
  tab; it lives at the foot of Sync, the tab its facts are about ([GDK-188]).
- The desktop install log strips ANSI color codes — `ray develop` colors
  its output even into a pipe.

## v0.15.1 — 2026-08-17

- **`gadak raycast install`** ([GDK-182]). Embeds the Raycast extension and
  registers it with a one-shot `npx ray develop`, so a brew or app-bundle
  install does not need a checkout while the store review is pending.
- **⌘K palette home is never blank** ([GDK-184]). An empty query now shows
  Recently updated issues (from the already-loaded pool — still zero
  requests per keystroke) under Recently viewed, plus saved views, so a
  fresh profile opens onto a list instead of an empty box.
- **Settings → Integrations** ([GDK-185], desktop app only). One tab lists
  the agent surfaces gadak installs into — Raycast extension, Claude Code
  skill, Claude Desktop MCP — with honest detection (installed, not
  installed, or *unknown*, each a distinct state), the exact command it is
  about to run, and a live log of the install. The verdict comes from the
  stream's final `exit=<code>` line, never from silence: a broken stream
  reads as "result unknown", not success. The routes live on the desktop
  app's own mux, so `gadak serve` and the hosted demo never offer the tab.

## v0.15.0 — 2026-08-17

The release that opens gadak outward. A view or an issue is now a link any
app can hand over, search is fast enough to drive somebody else's UI
keystroke by keystroke, and Raycast gets a documented way in. Inside, a dark
theme built to the same paper-and-ink standard as the light one — and the
first run of a new ritual: a full-codebase audit before every minor.

### A gadak is now an address

- **`gadak://` deep links** ([GDK-119]). The macOS app registers a URL scheme,
  so a piece of gadak travels as a link instead of a shell command:
  `gadak://view?issue=NMB-140`, `gadak://view/w/oss?pj=GDK&sc=inprogress`.
  `gadak views open` prints the link next to the http one. The grammar
  carries no verb and no payload — a link says *where to go*, never what to
  do — and the parser deliberately owns only the shape, so new actions are a
  handler-table entry, not a grammar change. This is the first release whose
  shipped artifact actually claims the scheme; the release check now tests
  the installed bundle, not the script that writes it.
- **Every place has a name in the URL** ([GDK-124]). The person panel, the
  personal feed and the settings tab join the issue, document and space
  params — nine place params in one reviewed registry
  (`web/src/lib/url-state.ts`). A param registered there is deep-linkable
  the same moment, with no Go change; compose and credential forms are
  deliberately excluded, and the registry is where that refusal is enforced.
- **Raycast, both doors** ([GDK-117]). `gadak mcp install raycast` prints the
  values for Raycast's *Install New Server* form (Raycast ≥1.98 speaks MCP
  over stdio but has no config file to write into). For the keystroke-fast
  path, the local search that a Raycast extension would sit on measures
  p50 ~2–4 ms over HTTP and ~24 ms per CLI spawn on the demo mirror — under
  a "feels instant" budget either way.
- **The product produces the links it consumes** ([GDK-163], [GDK-164]). The
  consumer side worked all along; nothing emitted an issue link. Now the
  issue detail carries a copy-link action (gadak:// plus the http form),
  `gadak issue KEY --link` prints both through the same composer `views
  open` uses, and docs/DESKTOP.md states the issue-link form as a contract
  an extension author can build against. And the querystring shape external
  tools actually paste — `/?issue=NMB-140`, no `#/` — used to boot the
  default view and silently drop the param; at boot those params now
  promote into the hash and the link lands where it pointed.
- **An issue can name its parent** ([GDK-19] in part, toward [GDK-86]).
  `gadak create --parent KEY` and `gadak edit --parent KEY` write the
  sub-issue relationship through Jira; the mirror learns it on the next
  tick. Link types (`blocks`, …) and components editing remain open.
- **Typing an issue key finds that issue** ([GDK-170]). Server search used to
  index only title/body/comments — `NMB-140` returned four wiki pages that
  mention the issue and not the issue itself — and ranked with bare bm25. A
  key query is now a lookup promoted above FTS (case-insensitive, `nmb140`
  and prefix forms included, never evicted by the limit), FTS columns are
  weighted title > body > comment, and the web defers to the server's order
  instead of re-ranking it — so the CLI, the REST route, MCP and the list
  all give the same answer. `gadak search --explain` answers "why is this
  row above that one".
- **Search is fast enough to sit under someone else's keystroke** ([GDK-166]).
  On a 20k-item mirror a single letter cost up to 1.6 s — not the FTS scan
  but three per-row column probes re-reading the index for every returned
  row. The profile, not the hypothesis, picked the cut: rank resolves first,
  payload work happens on at most `limit` rows, and the same query now
  answers in ~110 ms worst-case (14–37× depending on shape), with common
  tokens inside the 50 ms instant-search budget. A deterministic 20k fixture
  and a budget gate keep it that way, and `--explain` reports the query's
  wall time so the next slow keystroke names itself.

### A dark theme, and a place for the next one

- **Dark** ([GDK-154], [GDK-156], [GDK-162]). Warm ground, ink foregrounds, the
  same paper metaphor as light — with the anti-slop rule encoded as a CI
  contract (`tools/theme-check.mjs`): hue must stay warm, chroma inside the
  reference band, so a generic cool-gray dark cannot land by accident. A
  blocking boot script reads the stored preference before first paint (no
  flash), and adding a third theme is now one definition block plus a
  registry entry. The picker lives where the app's other per-browser
  settings already were — settings dialog and ⌘K palette, not new chrome.
- **Success and failure stop being told by colour alone** ([GDK-158]). Toasts
  carry per-kind icons and the breakdown bar carries glyphs, so a
  deuteranopic reader gets the same answer everyone else does.
- **Both palettes clear the same measured floors** ([GDK-157], [GDK-159],
  [GDK-171]). Status inks in both themes now pass pairwise ΔE separation in
  normal *and* deuteranopic vision — dark's in-progress and stale were ΔE
  0.008 apart, one colour twice. The search highlight gets its own token
  instead of borrowing a status colour (which vanished on the selected
  row), and each theme derives its own: the light mark carried light text
  at 1.18:1 when transplanted into dark. The gate now measures the text
  that actually sits on the mark, in both themes, so that class of
  transplant cannot land again.

### The list behaves like a AAA list

- **The right side of a row is a column you can scan** ([GDK-128]). Labels,
  staleness and the trailing strip sit in fixed-width slots instead of
  drifting up to 274 px per row; container queries retune the widths per
  regime instead of hiding information.
- **The last row stops being cut in half** ([GDK-131]). A flex scroller drops
  its own padding-bottom in scrollable overflow — one shared container rule
  (`.scroll-region`) now owns the bottom inset everywhere, instead of a
  per-panel `pb-3` that never worked.
- **Esc closes what it looks at** ([GDK-132], [GDK-133]). The three list header
  menus close on Escape and outside-click through the same `dom-actions`
  owner every other menu uses, and the sidebar stops highlighting a view row
  while a feed or document screen owns the main column.
- **A covering panel declares itself** ([GDK-127]). Below 1440 px the detail
  panel overlays the list behind a scrim instead of silently sitting on top
  of live rows.
- **One concept, one Korean word** ([GDK-135]). The ko catalog stops mixing
  용어 for the same concept across dialogs, toasts and empty states.
- **A half-composed syllable is not a query** ([GDK-169]). Typing 딥링크
  flashed the list empty on alternating keystrokes because every IME
  intermediate (딥ㄹ, 딥리) was committed as a real search. One shared
  helper now owns the rule for the search box and the palette: nothing
  commits while composition is active, composition end commits the final
  text, and Enter stays with the IME instead of jumping. English typing is
  untouched.

### Honesty at the edges

- **A hosted snapshot no longer advertises verbs it cannot answer**
  ([GDK-52]). Server-dependent affordances (FTS, settings, docs freshness)
  key off the capability document instead of optimism, so the demo and any
  static mirror stop rendering dead buttons — and the e2e webServer names
  its shell instead of assuming one.
- **The legacy field mapping retires itself** ([GDK-149]). A config still
  carrying `fieldMap`/`editableFields` is rewritten to `fields` once, at
  load, with one stderr line saying so; exports stop emitting the legacy
  keys. And the rewrite is a convenience, not a precondition ([GDK-173]): on a
  read-only home it becomes a warning and the app runs on the in-memory
  mapping instead of refusing to start — a locked-down directory stays
  locked instead of being silently chmod-unlocked, and `gadak status` now
  names a config it cannot read instead of swallowing the error.
- **Copy means copied** ([GDK-178]). Every copy affordance used to confirm
  before checking: inside the desktop webview `navigator.clipboard` rejects,
  so the button toasted "copied" over an unchanged pasteboard — and a
  workspace page (`/w/<profile>`) didn't even know it was in the desktop
  app, so every desktop-only transport was dead there. One owner now moves
  text to the clipboard (through the app itself on desktop), the toast
  reports what actually happened, and workspace pages carry the desktop
  flag. Verified by clicking the installed build and reading the pasteboard.
- **An attachment is fetched at most once, as promised** ([GDK-177]). The
  attachment cache's single-flight had a window where a caller arriving
  just after a download finished refetched the same file. CI caught it as
  a flaky test; the assertion was right and the window was real. The cache
  now answers from disk inside the lock.
- **The desktop app stops loading its runtime twice** ([GDK-150]). The wails
  runtime is injected server-side only, a dock-icon click reopens the closed
  window, and the desktop module finally builds and tests in CI on macOS.

### The audit, and what it deleted

First run of the per-minor full-codebase audit ([GDK-125]/126; the procedure
is now `docs/runbooks/release-audit.md`). Eighteen findings fixed in this
release; the rest carry `carryover-v0.15` labels. Highlights, best measured
in lines removed:

- Timestamps get one owner — `config.ISOMilli` replaces 34 quoted format
  literals across 19 files ([GDK-148]); `VIEW_PARAM_KEYS` becomes the type
  instead of feeding a mirror list and the drift test both die ([GDK-147]);
  Svelte hygiene drops positional list keys, a toast-host reach-in and eight
  dead exports ([GDK-152]).
- The test pyramid gets enforced downward: sixteen browser tests become
  vitest units — one of them was asserting a contract that no longer
  existed ([GDK-145]); the Go suite stops sleeping on wall clocks, ~12 s
  faster ([GDK-144]); the three untested pure modules and the Jira URL
  composition get real cases ([GDK-146]).
- `docs/DERIVE.md` becomes the single home for derived-field semantics, and
  its SQL examples are executed by a test, so the doc cannot drift from the
  code it documents ([GDK-88], [GDK-89]).
- Chosung (초성) search retires, product-wide ([GDK-168]). It existed only in
  the web while the CLI, REST, MCP and Raycast all lacked it, its cost sat
  on the hottest per-keystroke path, and a chosung hit could never highlight
  *why* it matched. ~144 lines removed, nothing added in their place; a
  jamo-only query is now a plain miss on every surface, which at least is
  the same answer everywhere.

## v0.14.2 — 2026-08-16

The release about the first ten minutes and the day the token dies. Nothing
here is a new capability so much as an existing one that finally tells you
what it is doing.

- **Every token trap is named before the paste, not after the 401** ([GDK-69],
  [GDK-98]). Atlassian's token page offers three things that look like one, and
  two of them cannot sign in to a site URL: a *scoped* token — which that page
  now recommends first — and an org key from admin.atlassian.com. gadak only
  ever said so after the rejection. Both the web form and `gadak init` now say
  it up front. Behind a 401, the one trap that is recognisable from the token
  itself (the ATCTT prefix) is named outright; the rest share a message that
  hands you a check you can run, because Jira answers all of them identically
  and inventing a distinction would be worse than admitting there isn't one.
- **A rejected token is recoverable without writing** ([GDK-68]). Only the write
  path used to offer the replace-token dialog, so a person who reads the
  mirror saw a dead freshness chip and a technical error string. The sync
  progress document now carries `error_code`, classified by the one function
  that already owned that rule, and the chip, the palette and the empty-mirror
  CTA all reach the same dialog. A wiki-only 401 deliberately does *not*
  count: the Jira pass authenticated with the same token moments earlier, so
  that is a permission gap, not a dead credential.
- **Picking no projects is a choice** ([GDK-99]). The CLI and settings have
  always read an empty project list as "everything this account can see". The
  first-run wizard was the only surface calling it illegal — Start sat
  disabled next to its own "Select none" button — which forced a decision the
  product does not require, and the wrong one on a large site, where the
  picker is truncated and "select all" was never "everything".
- **`gadak skill install` treats an upgrade as an upgrade** ([GDK-92]). After
  `brew upgrade` the installed skill is the previous release's own copy, so it
  differed, so the one-liner in our own docs turned red. Provenance is now
  decided by content hash — an install receipt, plus a frozen table of the
  digests shipped before receipts existed. A file *you* wrote is still
  refused; that refusal is the feature. `doctor` grew skill and MCP lines, so
  "is my skill current?" is one command.
- **The embedded skill knows the verbs the CLI has** ([GDK-91]). It described
  reads plus comment/transition/assign and stopped there, so an agent with the
  skill loaded answered "gadak cannot create issues" or reached for the REST
  API. v0.14.1 shipped `create`, `attach` and `edit`; the file agents read
  never learned.
- **A quiet Confluence tick reads zero page bodies** ([GDK-113]). A sync tick
  took 21.4s, and 19.4s of it re-read 71 unchanged wiki pages: minute-grained
  CQL kept returning the same cluster forever, and nothing decided between a
  search hit and a body fetch. One owner decides now, and `gadak sync` prints
  the tally so the next person can check without adding printlns.
- **`gadak issue <KEY> --derive`** ([GDK-111]) prints how the derived columns
  were computed — the changelog by status *category*, and the rows behind
  `reopen_count`, `resolved_at`, `reopen_reason` and `epic_key`. It calls the
  same derivation the sync path calls; a second copy would agree with the
  first only until one of them changed.
- **History keeps its order** ([GDK-26]): "Show issues in list" no longer
  regroups by status, which is the one thing that pane exists to show.
- Also: token expiry is recorded and warned about before the sync dies
  ([GDK-67]/70), the browse pane yields Escape and stops outliving its document
  ([GDK-78]/79/80), `gadak sql` warns on a stale mirror and `gadak_query` flags
  display-name zero rows ([GDK-90]), `Open` repairs an `items_fts` this build
  cannot write ([GDK-112]), the search-help `?` works on touch ([GDK-53]),
  `examples/compose` lands as pure shell ([GDK-109]), the Datasette Lite deep
  link is pinned ([GDK-101]), and `PROMISES.md` is gated against `SECURITY.md`
  ([GDK-104]).
- **Process, because it failed twice in one day** ([GDK-57]): the Node version
  had five owners and none a shell could read — `.nvmrc` is the single one
  now — and `tools/ci-status.sh` answers "did what I just pushed pass?", which
  is the question that went unasked while main sat red for an hour.

## v0.14.1 — 2026-08-15

One day of dogfooding gadak's own backlog through gadak, shipped as it
landed: the first CLI write verbs, a demo that finally works where people
actually tap it, and the removal of an updater that had never earned trust.

- **The macOS app is notify-only again.** Removed the never-exercised in-app
  self-updater (Wails `pkg/updater`): digest verification was fail-open and
  the swap was non-atomic ([GDK-58]/59/60). The sidebar banner still names a
  newer release; installing it is `brew upgrade --cask gadak` or a new dmg.
  v0.14.1 ships no `gadak-desktop-darwin-<arch>.zip`, so a v0.14.0 app in
  the wild cannot self-swap. Docs realigned ([GDK-61]/64). Found on the way:
  the desktop banner had been silent in every release build because
  `server.Version` was never assigned there — now wired.
- **The first write verbs: `gadak create`, `gadak attach`, `gadak edit`.**
  Create takes `--project`, `--type`, `--priority`, labels, a description
  from stdin, files to attach, and `--batch -` for JSON lines — everything
  this backlog's own migration to Jira needed. Unknown flags are rejected
  instead of being folded into the summary.
- **The hosted demo works inside in-app browsers** ([GDK-23], [GDK-51]). The
  snapshot service worker is gone — an in-page fetch adapter serves the
  frozen mirror, so the X/Slack webviews that blocked workers now boot. And
  the first paint is no longer 4px text: a static first frame (claim,
  tap-to-load demo video, a selectable `brew install`, the repo link) is
  injected at build time and reads at phone width before any JS arrives.
- **The browse pane yields** ([GDK-76]/77). At the shipped window size the
  in-app browser pane sat over the command palette and every dialog; toasts
  painted under the native page. Stacking now has one owner and the palette
  is frontmost and clickable while browsing.
- **Boot keystrokes are held, not dropped** ([GDK-46]). `j`/`k`/`x` pressed
  while the startup view is still committing replay in order once keys are
  ready, instead of silently acting on the wrong list.
- **Failures say what happened.** A failed write reports in the reader's
  language, not a Go error chain; a truncated key list says how many keys
  were given and shown ([GDK-35]); a rejected credential stops the watch loop
  for every source — Confluence included — and leaves a visible trace
  ([GDK-24], [GDK-48]).
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

[GDK-19]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-19
[GDK-23]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-23
[GDK-24]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-24
[GDK-26]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-26
[GDK-35]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-35
[GDK-46]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-46
[GDK-48]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-48
[GDK-51]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-51
[GDK-52]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-52
[GDK-53]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-53
[GDK-57]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-57
[GDK-58]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-58
[GDK-61]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-61
[GDK-67]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-67
[GDK-68]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-68
[GDK-69]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-69
[GDK-76]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-76
[GDK-78]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-78
[GDK-82]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-82
[GDK-83]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-83
[GDK-601]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-601
[GDK-602]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-602
[GDK-604]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-604
[GDK-603]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-603
[GDK-605]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-605
[GDK-606]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-606
[GDK-607]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-607
[GDK-608]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-608
[GDK-609]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-609
[GDK-610]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-610
[GDK-611]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-611
[GDK-626]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-626
[GDK-86]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-86
[GDK-598]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-598
[GDK-599]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-599
[GDK-81]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-81
[GDK-88]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-88
[GDK-89]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-89
[GDK-90]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-90
[GDK-91]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-91
[GDK-92]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-92
[GDK-93]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-93
[GDK-98]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-98
[GDK-99]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-99
[GDK-101]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-101
[GDK-104]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-104
[GDK-109]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-109
[GDK-111]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-111
[GDK-112]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-112
[GDK-113]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-113
[GDK-115]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-115
[GDK-116]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-116
[GDK-117]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-117
[GDK-119]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-119
[GDK-124]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-124
[GDK-125]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-125
[GDK-127]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-127
[GDK-128]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-128
[GDK-129]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-129
[GDK-131]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-131
[GDK-132]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-132
[GDK-133]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-133
[GDK-135]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-135
[GDK-144]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-144
[GDK-145]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-145
[GDK-146]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-146
[GDK-147]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-147
[GDK-148]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-148
[GDK-149]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-149
[GDK-150]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-150
[GDK-152]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-152
[GDK-154]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-154
[GDK-156]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-156
[GDK-157]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-157
[GDK-158]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-158
[GDK-159]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-159
[GDK-161]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-161
[GDK-162]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-162
[GDK-163]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-163
[GDK-164]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-164
[GDK-166]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-166
[GDK-168]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-168
[GDK-169]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-169
[GDK-170]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-170
[GDK-171]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-171
[GDK-173]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-173
[GDK-177]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-177
[GDK-178]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-178
[GDK-182]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-182
[GDK-183]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-183
[GDK-184]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-184
[GDK-185]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-185
[GDK-186]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-186
[GDK-188]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-188
[GDK-189]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-189
[GDK-190]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-190
[GDK-191]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-191
[GDK-193]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-193
[GDK-208]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-208
[GDK-209]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-209
[GDK-213]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-213
[GDK-214]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-214
[GDK-215]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-215
[GDK-216]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-216
[GDK-217]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-217
[GDK-218]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-218
[GDK-223]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-223
[GDK-225]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-225
[GDK-229]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-229
[GDK-237]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-237
[GDK-238]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-238
[GDK-239]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-239
[GDK-241]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-241
[GDK-246]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-246
[GDK-247]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-247
[GDK-248]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-248
[GDK-249]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-249
[GDK-250]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-250
[GDK-251]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-251
[GDK-258]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-258
[GDK-259]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-259
[GDK-261]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-261
[GDK-263]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-263
[GDK-267]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-267
[GDK-270]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-270
[GDK-271]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-271
[GDK-272]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-272
[GDK-274]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-274
[GDK-275]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-275
[GDK-282]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-282
[GDK-293]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-293
[GDK-297]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-297
[GDK-298]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-298
[GDK-299]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-299
[GDK-300]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-300
[GDK-301]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-301
[GDK-302]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-302
[GDK-304]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-304
[GDK-305]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-305
[GDK-306]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-306
[GDK-308]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-308
[GDK-310]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-310
[GDK-312]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-312
[GDK-313]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-313
[GDK-314]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-314
[GDK-315]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-315
[GDK-317]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-317
[GDK-319]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-319
[GDK-322]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-322
[GDK-323]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-323
[GDK-331]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-331
[GDK-332]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-332
[GDK-333]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-333
[GDK-335]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-335
[GDK-336]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-336
[GDK-340]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-340
[GDK-342]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-342
[GDK-343]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-343
[GDK-344]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-344
[GDK-345]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-345
[GDK-346]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-346
[GDK-347]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-347
[GDK-348]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-348
[GDK-359]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-359
[GDK-360]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-360
[GDK-361]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-361
[GDK-363]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-363
[GDK-364]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-364
[GDK-365]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-365
[GDK-366]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-366
[GDK-367]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-367
[GDK-368]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-368
[GDK-371]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-371
[GDK-372]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-372
[GDK-373]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-373
[GDK-374]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-374
[GDK-375]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-375
[GDK-376]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-376
[GDK-380]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-380
[GDK-381]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-381
[GDK-382]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-382
[GDK-391]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-391
[GDK-393]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-393
[GDK-394]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-394
[GDK-396]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-396
[GDK-400]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-400
[GDK-408]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-408
[GDK-409]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-409
[GDK-415]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-415
[GDK-420]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-420
[GDK-421]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-421
[GDK-424]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-424
[GDK-426]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-426
[GDK-418]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-418
[GDK-430]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-430
[GDK-105]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-105
[GDK-121]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-121
[GDK-123]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-123
[GDK-172]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-172
[GDK-181]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-181
[GDK-254]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-254
[GDK-339]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-339
[GDK-71]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-71
[GDK-100]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-100
[GDK-341]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-341
[GDK-349]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-349
[GDK-350]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-350
[GDK-351]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-351
[GDK-180]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-180
[GDK-200]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-200
[GDK-255]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-255
[GDK-316]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-316
[GDK-353]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-353
[GDK-354]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-354
[GDK-352]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-352
[GDK-369]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-369
[GDK-389]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-389
[GDK-425]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-425
[GDK-427]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-427
[GDK-431]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-431
[GDK-432]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-432
[GDK-433]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-433
[GDK-434]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-434
[GDK-435]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-435
[GDK-437]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-437
[GDK-438]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-438
[GDK-440]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-440
[GDK-441]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-441
[GDK-442]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-442
[GDK-443]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-443
[GDK-444]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-444
[GDK-446]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-446
[GDK-447]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-447
[GDK-448]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-448
[GDK-449]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-449
[GDK-450]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-450
[GDK-451]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-451
[GDK-452]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-452
[GDK-453]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-453
[GDK-454]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-454
[GDK-455]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-455
[GDK-456]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-456
[GDK-457]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-457
[GDK-458]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-458
[GDK-460]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-460
[GDK-483]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-483
[GDK-484]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-484
[GDK-485]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-485
[GDK-486]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-486
[GDK-489]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-489
[GDK-490]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-490
[GDK-495]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-495
[GDK-496]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-496
[GDK-497]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-497
[GDK-498]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-498
[GDK-504]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-504
[GDK-505]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-505
[GDK-509]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-509
[GDK-510]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-510
[GDK-511]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-511
[GDK-512]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-512
[GDK-513]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-513
[GDK-514]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-514
[GDK-515]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-515
[GDK-516]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-516
[GDK-517]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-517
[GDK-518]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-518
[GDK-519]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-519
[GDK-520]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-520
[GDK-521]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-521
[GDK-522]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-522
[GDK-526]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-526
[GDK-527]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-527
[GDK-528]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-528
[GDK-531]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-531
[GDK-532]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-532
[GDK-534]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-534
[GDK-536]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-536
[GDK-537]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-537
[GDK-538]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-538
[GDK-539]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-539
[GDK-540]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-540
[GDK-541]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-541
[GDK-542]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-542
[GDK-543]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-543
[GDK-544]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-544
[GDK-545]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-545
[GDK-546]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-546
[GDK-547]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-547
[GDK-548]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-548
[GDK-549]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-549
[GDK-551]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-551
[GDK-552]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-552
[GDK-553]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-553
[GDK-554]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-554
[GDK-555]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-555
[GDK-556]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-556
[GDK-557]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-557
[GDK-558]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-558
[GDK-559]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-559
[GDK-560]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-560
[GDK-561]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-561
[GDK-562]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-562
[GDK-563]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-563
[GDK-564]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-564
[GDK-565]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-565
[GDK-566]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-566
[GDK-567]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-567
[GDK-568]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-568
[GDK-569]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-569
[GDK-570]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-570
[GDK-571]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-571
[GDK-572]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-572
[GDK-573]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-573
[GDK-574]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-574
[GDK-575]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-575
[GDK-589]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-589
[GDK-590]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-590
[GDK-591]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-591
[GDK-592]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-592
[GDK-597]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-597
[GDK-591]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-591
[GDK-592]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-592
[GDK-593]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-593
[GDK-586]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-586
[GDK-588]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-588
