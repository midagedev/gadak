# Changelog

<sub>English · <a href="CHANGELOG.ko.md">한국어</a></sub>

## Unreleased

The cycle where an agent's writes grew up. An issue now shows the PRs and
commits that implement it, the write verbs learned the vocabulary a coding
agent actually sends, and a workspace stopped being something you re-select
on every command. It closed with a pre-release audit across the network
seams, the MCP surface and the web UI.

Every entry in full detail, with the reasoning: [docs/changelog-detail.md](docs/changelog-detail.md#unreleased).

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
- Saved views live on the server by default, and views already in a browser
  move there once ([GDK-437]).
- The demo fixture can no longer silently lag the schema ([GDK-671]).

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

### Network seams, audited

- The Linear proxy fetches only `uploads.linear.app`, redirects included, and
  the API key rides only there ([GDK-427], [GDK-558], [GDK-560]).
- An empty host is a non-loopback bind, so `serve` demands `--allow-remote`
  for it like any other exposure ([GDK-542]).
- The boot sequence has one owner, shared by `gadak serve` and the desktop
  app ([GDK-664]).
- A Linear-only workspace is a configured workspace, and Linear's rate limit
  is a retry rather than a death ([GDK-654], [GDK-263]).

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
- Agent onboarding is skill-first, and the repository is a Claude Code plugin
  marketplace ([GDK-8], [GDK-93]).

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

- **`AGENTS.md` is the repository's development contract now, not a product
  manual** ([GDK-8]). The filename is a convention every coding agent looks
  for at a repo root, and this one had grown into 513 lines of which 439 were
  the mirror cookbook — so an agent sent to change gadak read a SQL reference
  first, and an agent sent to *use* gadak had to be pointed at a file named
  for contributors. The cookbook moved verbatim to `docs/MIRROR.md` (same
  `#using-the-mirror` anchor, so existing links resolve), `AGENTS.md` keeps
  the two-audience dispatcher and `## Developing gadak`, and the prose that
  introduced `AGENTS.md` as the home of SQL/CLI/REST — in both READMEs,
  `AGENT_SETUP.md`, `AGENT_ACCESS.md`, `EXTENDING.md` and the CLI's own usage
  line — names the new page. Two doc-checks hold it: one fails when a
  product-manual heading reappears in `AGENTS.md`, the other when the two
  files stop pointing at each other.
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
- **One interpreter for view names** ([GDK-612]). The CLI and the MCP tool
  each carried a copy of the name-to-view resolution — the copies had
  already drifted: the same missing name drew different guidance from each
  surface, and MCP could not report a saved view's applied clauses at all.
  `internal/views` now owns the quartet; both surfaces import it, the
  richer error survives, and the dropped field came back.
- **One judge for `ray develop` output** ([GDK-615]). The production watch
  loop and the function the tests exercised were parallel implementations —
  the tests were verifying a replica while the real loop ran unverified,
  and the two already disagreed: the replica dropped blank lines from
  error transcripts, and the real loop could race a fast exit against its
  own success signal and call a successful install a failure. Production
  now feeds the one watched function the tests drive.
- **Dead code is deleted, with its callers counted first** ([GDK-616]).
  A staticcheck-and-deadcode sweep, every deletion preceded by a logged
  zero-caller census: eight test-only wrapper shims rewired to the real
  functions and removed, six dead one-offs deleted (one — the pairing TTL
  default — turned out to be a constant nobody wired to its flag, fixed
  instead of deleted), an unimported Svelte component and a deprecated
  API alias dropped, and the createmeta fallback that existed inline
  twice now lives once and fetches the catalog only when it needs it.
  Net −108 lines.
- **Six small owners, one each** ([GDK-619]). A sweep of byte-identical
  helper copies and near-misses: the date literal check turned out to
  exist three times, not two, and now lives once in `internal/fields`;
  name-or-id formatting, identity-from-config, and profile normalization
  each collapsed to one exported owner; source kinds became constants
  (five bare `"linear"`-family literals swept); a settings-import plan
  threads its caller's context instead of burying `Background()`; and a
  provably dead nil-client defense — the constructors always install one —
  is deleted in all five places it had been pasted.
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

[GDK-19]: https://gadak.dev/backlog/#/?ks=GDK-19
[GDK-23]: https://gadak.dev/backlog/#/?ks=GDK-23
[GDK-24]: https://gadak.dev/backlog/#/?ks=GDK-24
[GDK-26]: https://gadak.dev/backlog/#/?ks=GDK-26
[GDK-269]: https://gadak.dev/backlog/#/?ks=GDK-269
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
[GDK-601]: https://gadak.dev/backlog/#/?ks=GDK-601
[GDK-602]: https://gadak.dev/backlog/#/?ks=GDK-602
[GDK-604]: https://gadak.dev/backlog/#/?ks=GDK-604
[GDK-603]: https://gadak.dev/backlog/#/?ks=GDK-603
[GDK-605]: https://gadak.dev/backlog/#/?ks=GDK-605
[GDK-606]: https://gadak.dev/backlog/#/?ks=GDK-606
[GDK-607]: https://gadak.dev/backlog/#/?ks=GDK-607
[GDK-608]: https://gadak.dev/backlog/#/?ks=GDK-608
[GDK-609]: https://gadak.dev/backlog/#/?ks=GDK-609
[GDK-610]: https://gadak.dev/backlog/#/?ks=GDK-610
[GDK-611]: https://gadak.dev/backlog/#/?ks=GDK-611
[GDK-613]: https://gadak.dev/backlog/#/?ks=GDK-613
[GDK-612]: https://gadak.dev/backlog/#/?ks=GDK-612
[GDK-615]: https://gadak.dev/backlog/#/?ks=GDK-615
[GDK-616]: https://gadak.dev/backlog/#/?ks=GDK-616
[GDK-617]: https://gadak.dev/backlog/#/?ks=GDK-617
[GDK-618]: https://gadak.dev/backlog/#/?ks=GDK-618
[GDK-619]: https://gadak.dev/backlog/#/?ks=GDK-619
[GDK-620]: https://gadak.dev/backlog/#/?ks=GDK-620
[GDK-621]: https://gadak.dev/backlog/#/?ks=GDK-621
[GDK-626]: https://gadak.dev/backlog/#/?ks=GDK-626
[GDK-632]: https://gadak.dev/backlog/#/?ks=GDK-632
[GDK-634]: https://gadak.dev/backlog/#/?ks=GDK-634
[GDK-636]: https://gadak.dev/backlog/#/?ks=GDK-636
[GDK-637]: https://gadak.dev/backlog/#/?ks=GDK-637
[GDK-639]: https://gadak.dev/backlog/#/?ks=GDK-639
[GDK-643]: https://gadak.dev/backlog/#/?ks=GDK-643
[GDK-648]: https://gadak.dev/backlog/#/?ks=GDK-648
[GDK-649]: https://gadak.dev/backlog/#/?ks=GDK-649
[GDK-644]: https://gadak.dev/backlog/#/?ks=GDK-644
[GDK-658]: https://gadak.dev/backlog/#/?ks=GDK-658
[GDK-468]: https://gadak.dev/backlog/#/?ks=GDK-468
[GDK-645]: https://gadak.dev/backlog/#/?ks=GDK-645
[GDK-642]: https://gadak.dev/backlog/#/?ks=GDK-642
[GDK-635]: https://gadak.dev/backlog/#/?ks=GDK-635
[GDK-641]: https://gadak.dev/backlog/#/?ks=GDK-641
[GDK-647]: https://gadak.dev/backlog/#/?ks=GDK-647
[GDK-650]: https://gadak.dev/backlog/#/?ks=GDK-650
[GDK-651]: https://gadak.dev/backlog/#/?ks=GDK-651
[GDK-652]: https://gadak.dev/backlog/#/?ks=GDK-652
[GDK-654]: https://gadak.dev/backlog/#/?ks=GDK-654
[GDK-655]: https://gadak.dev/backlog/#/?ks=GDK-655
[GDK-656]: https://gadak.dev/backlog/#/?ks=GDK-656
[GDK-662]: https://gadak.dev/backlog/#/?ks=GDK-662
[GDK-663]: https://gadak.dev/backlog/#/?ks=GDK-663
[GDK-677]: https://gadak.dev/backlog/#/?ks=GDK-677
[#52]: https://github.com/midagedev/gadak/issues/52
[GDK-675]: https://gadak.dev/backlog/#/?ks=GDK-675
[GDK-633]: https://gadak.dev/backlog/#/?ks=GDK-633
[GDK-86]: https://gadak.dev/backlog/#/?ks=GDK-86
[GDK-598]: https://gadak.dev/backlog/#/?ks=GDK-598
[GDK-599]: https://gadak.dev/backlog/#/?ks=GDK-599
[GDK-81]: https://gadak.dev/backlog/#/?ks=GDK-81
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
[GDK-109]: https://gadak.dev/backlog/#/?ks=GDK-109
[GDK-111]: https://gadak.dev/backlog/#/?ks=GDK-111
[GDK-112]: https://gadak.dev/backlog/#/?ks=GDK-112
[GDK-113]: https://gadak.dev/backlog/#/?ks=GDK-113
[GDK-115]: https://gadak.dev/backlog/#/?ks=GDK-115
[GDK-116]: https://gadak.dev/backlog/#/?ks=GDK-116
[GDK-117]: https://gadak.dev/backlog/#/?ks=GDK-117
[GDK-119]: https://gadak.dev/backlog/#/?ks=GDK-119
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
[GDK-258]: https://gadak.dev/backlog/#/?ks=GDK-258
[GDK-259]: https://gadak.dev/backlog/#/?ks=GDK-259
[GDK-261]: https://gadak.dev/backlog/#/?ks=GDK-261
[GDK-263]: https://gadak.dev/backlog/#/?ks=GDK-263
[GDK-267]: https://gadak.dev/backlog/#/?ks=GDK-267
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
[GDK-391]: https://gadak.dev/backlog/#/?ks=GDK-391
[GDK-393]: https://gadak.dev/backlog/#/?ks=GDK-393
[GDK-394]: https://gadak.dev/backlog/#/?ks=GDK-394
[GDK-396]: https://gadak.dev/backlog/#/?ks=GDK-396
[GDK-400]: https://gadak.dev/backlog/#/?ks=GDK-400
[GDK-408]: https://gadak.dev/backlog/#/?ks=GDK-408
[GDK-409]: https://gadak.dev/backlog/#/?ks=GDK-409
[GDK-415]: https://gadak.dev/backlog/#/?ks=GDK-415
[GDK-420]: https://gadak.dev/backlog/#/?ks=GDK-420
[GDK-421]: https://gadak.dev/backlog/#/?ks=GDK-421
[GDK-424]: https://gadak.dev/backlog/#/?ks=GDK-424
[GDK-426]: https://gadak.dev/backlog/#/?ks=GDK-426
[GDK-418]: https://gadak.dev/backlog/#/?ks=GDK-418
[GDK-430]: https://gadak.dev/backlog/#/?ks=GDK-430
[GDK-105]: https://gadak.dev/backlog/#/?ks=GDK-105
[GDK-121]: https://gadak.dev/backlog/#/?ks=GDK-121
[GDK-123]: https://gadak.dev/backlog/#/?ks=GDK-123
[GDK-172]: https://gadak.dev/backlog/#/?ks=GDK-172
[GDK-181]: https://gadak.dev/backlog/#/?ks=GDK-181
[GDK-254]: https://gadak.dev/backlog/#/?ks=GDK-254
[GDK-339]: https://gadak.dev/backlog/#/?ks=GDK-339
[GDK-71]: https://gadak.dev/backlog/#/?ks=GDK-71
[GDK-100]: https://gadak.dev/backlog/#/?ks=GDK-100
[GDK-341]: https://gadak.dev/backlog/#/?ks=GDK-341
[GDK-349]: https://gadak.dev/backlog/#/?ks=GDK-349
[GDK-350]: https://gadak.dev/backlog/#/?ks=GDK-350
[GDK-351]: https://gadak.dev/backlog/#/?ks=GDK-351
[GDK-180]: https://gadak.dev/backlog/#/?ks=GDK-180
[GDK-200]: https://gadak.dev/backlog/#/?ks=GDK-200
[GDK-255]: https://gadak.dev/backlog/#/?ks=GDK-255
[GDK-316]: https://gadak.dev/backlog/#/?ks=GDK-316
[GDK-353]: https://gadak.dev/backlog/#/?ks=GDK-353
[GDK-354]: https://gadak.dev/backlog/#/?ks=GDK-354
[GDK-352]: https://gadak.dev/backlog/#/?ks=GDK-352
[GDK-369]: https://gadak.dev/backlog/#/?ks=GDK-369
[GDK-389]: https://gadak.dev/backlog/#/?ks=GDK-389
[GDK-425]: https://gadak.dev/backlog/#/?ks=GDK-425
[GDK-427]: https://gadak.dev/backlog/#/?ks=GDK-427
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
[GDK-483]: https://gadak.dev/backlog/#/?ks=GDK-483
[GDK-484]: https://gadak.dev/backlog/#/?ks=GDK-484
[GDK-485]: https://gadak.dev/backlog/#/?ks=GDK-485
[GDK-486]: https://gadak.dev/backlog/#/?ks=GDK-486
[GDK-489]: https://gadak.dev/backlog/#/?ks=GDK-489
[GDK-490]: https://gadak.dev/backlog/#/?ks=GDK-490
[GDK-428]: https://gadak.dev/backlog/#/?ks=GDK-428
[GDK-500]: https://gadak.dev/backlog/#/?ks=GDK-500
[GDK-501]: https://gadak.dev/backlog/#/?ks=GDK-501
[GDK-502]: https://gadak.dev/backlog/#/?ks=GDK-502
[GDK-503]: https://gadak.dev/backlog/#/?ks=GDK-503
[GDK-495]: https://gadak.dev/backlog/#/?ks=GDK-495
[GDK-496]: https://gadak.dev/backlog/#/?ks=GDK-496
[GDK-497]: https://gadak.dev/backlog/#/?ks=GDK-497
[GDK-498]: https://gadak.dev/backlog/#/?ks=GDK-498
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
[GDK-589]: https://gadak.dev/backlog/#/?ks=GDK-589
[GDK-590]: https://gadak.dev/backlog/#/?ks=GDK-590
[GDK-591]: https://gadak.dev/backlog/#/?ks=GDK-591
[GDK-592]: https://gadak.dev/backlog/#/?ks=GDK-592
[GDK-597]: https://gadak.dev/backlog/#/?ks=GDK-597
[GDK-591]: https://gadak.dev/backlog/#/?ks=GDK-591
[GDK-592]: https://gadak.dev/backlog/#/?ks=GDK-592
[GDK-593]: https://gadak.dev/backlog/#/?ks=GDK-593
[GDK-586]: https://gadak.dev/backlog/#/?ks=GDK-586
[GDK-588]: https://gadak.dev/backlog/#/?ks=GDK-588
[GDK-211]: https://gadak.dev/backlog/#/?ks=GDK-211
[GDK-669]: https://gadak.dev/backlog/#/?ks=GDK-669
[GDK-676]: https://gadak.dev/backlog/#/?ks=GDK-676
[GDK-8]: https://gadak.dev/backlog/#/?ks=GDK-8
[GDK-674]: https://gadak.dev/backlog/#/?ks=GDK-674
[GDK-85]: https://gadak.dev/backlog/#/?ks=GDK-85
[GDK-680]: https://gadak.dev/backlog/#/?ks=GDK-680
[GDK-665]: https://gadak.dev/backlog/#/?ks=GDK-665
[GDK-679]: https://gadak.dev/backlog/#/?ks=GDK-679
[GDK-668]: https://gadak.dev/backlog/#/?ks=GDK-668
[GDK-671]: https://gadak.dev/backlog/#/?ks=GDK-671
[GDK-678]: https://gadak.dev/backlog/#/?ks=GDK-678

[GDK-664]: https://gadak.dev/backlog/#/?ks=GDK-664

[GDK-666]: https://gadak.dev/backlog/#/?ks=GDK-666
[GDK-672]: https://gadak.dev/backlog/#/?ks=GDK-672
[GDK-681]: https://gadak.dev/backlog/#/?ks=GDK-681
[GDK-682]: https://gadak.dev/backlog/#/?ks=GDK-682
[GDK-683]: https://gadak.dev/backlog/#/?ks=GDK-683
