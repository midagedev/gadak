# Changelog

<sub>English · <a href="CHANGELOG.ko.md">한국어</a></sub>

## v0.19.0 — 2026-09-01

The release where the issues stand up as a board, and the terminal takes
its Beta mark off.

**The board.** The list you already filter is now also a board: one toggle
lays the same issues across columns — same filters, same thirteen grouping
axes, same search. Dragging a card is a real transition, with a menu when
more than one status matches; a move made anywhere else — another window,
an agent's `gadak transition` — flies across the board with a landing ring,
because on this screen the movement is the only evidence it happened. The
three status columns are always all three: "Done is empty" is an answer,
not a missing column ([GDK-1175], [GDK-1176], [GDK-1190]). And a view can
say it is a board: saved in board layout it reopens as one — from the app,
and from the CLI: `gadak views save "Sprint board" --jql '…' --layout
board`, with `views open` carrying the layout in the deeplink ([GDK-1248]).

**The terminal takes its Beta mark off** ([GDK-1024]) — and its sessions
belong to issues. `gadak claim` in a pane's shell binds that session to the
issue, and the tab wears the issue key ([GDK-1158]). A command in an
issue's body gets a ▶ that places it at that issue's shell prompt; the
board card and the ⌘K palette both open an issue's session ([GDK-1196],
[GDK-1197]). The pane lies down as a dock under the whole row, its chrome
is one row, and a tab can end its session ([GDK-1194], [GDK-1199],
[GDK-1200]). A focused terminal is no longer a keyboard trap either:
`Ctrl+Shift+[` and `]` walk the sessions, `` Ctrl+Shift+` `` steps out
without closing anything, and `Ctrl+Shift+O` opens the session's issue
([GDK-1250], [GDK-1251]).

**The phone holds more than one home.** A host roster with per-host caches
switches between paired machines; workspaces can be created and removed
from the web and the CLI; a bundled demo workspace shows every surface with
zero pairing. A glance strip answers "what moved while I was away", and the
terminal scrolls under a finger ([GDK-1097], [GDK-1096], [GDK-1098],
[GDK-1051], [GDK-871], [GDK-899]).

**The CLI learned the verbs sessions actually type.** `gadak list`, `next`,
`show`, `done`, `recent`, `pick` — measured against what blind sessions
reached for — plus `memory add`/`memory search` for agent notes,
`edit --type` to refile a misfiled issue, `unlink` (the reverse verb `link`
never had), and `workspaces rm`. A write confirms itself, and `edit -m`
refuses to silently flatten a formatted description ([GDK-992], [GDK-1030],
[GDK-1205], [GDK-1098], [GDK-1001]).

**Refusals got louder, writes got safer.** A JQL clause the mirror's subset
cannot express is refused out loud instead of silently widening the list
([GDK-1234]). Config and credential saves go through one atomic
stage-then-rename owner, so two saves can no longer torch each other
([GDK-1233], [GDK-1244]). A desktop boot that fails before the window opens
says so in a dialog instead of exiting silently ([GDK-1243]), and Linear
assignee edits go through the same write surface the UI advertises
([GDK-1235]). And `gadak link` now points the link the way Jira will
display it — the outward and inward ends were swapped, on the CLI and
REST both ([GDK-1204]; #79, thanks @wafe).

**Sync got faster on quiet mirrors.** Jira incremental answers
overlap-window echoes from the mirror instead of refetching them, and
Confluence incremental asks one CQL pair per tick and stops re-reading
bodies whose comments have not moved ([GDK-1075], [GDK-1074]).

Also: pairing offers render as a scannable QR and the desktop gets a
Devices tab ([GDK-1047]); the update dialog announces the version and
points at the release page instead of dumping raw markdown ([GDK-1246]); a
standalone workspace stops offering a Jira credential dialog that cannot
help it ([GDK-1122]); and the installed agent skill follows the binary,
once a day ([GDK-996]).

A full-codebase audit ran before this tag — parent [GDK-1128].

## v0.18.1 — 2026-08-26

The patch where the terminal learned to clean up after itself, written the
same day 0.18.0 shipped it.

**Closing a terminal closes everything it started.** A shell puts a
background job in its own process group, so closing a session used to leave
`sleep 999 &` — or a forgotten agent — running forever. The close now walks
every process on the session's terminal, once, while the walk can still be
trusted; a measured Linux failure where a second walk found *another*
session's shell is why it is once ([GDK-950]).

**`gadak views open` reaches every open window** — the CLI's focus used to
be consumed by whichever window polled first, and the window you were
looking at stayed put. And two `views open` in the same second no longer
lose the second one: the payload is deduped on what it says, not just when
it was written ([GDK-960], [GDK-981]).

**The terminal pane says why it cannot open** — no PTY on Windows, a token
without the terminal scope, a network drop — in words, with a retry, on the
web and on the phone from the same source ([GDK-944]).

**gadak keeps a log file, and doctor hands it to you.** A Finder-launched
app used to discard every diagnostic line; now `gadak doctor` names the file
and quotes its recent errors. Credentials are scrubbed before a line is
written ([GDK-967]).

Also: the agent skill teaches an agent to diagnose the wall it hit and to
report the friction instead of silently working around it ([GDK-968]); a
CLI exit path that used to leave a stale "open" marker behind clears it
([GDK-971]); the hosted demo's mirror is published as a file you can
download and open in your own gadak ([GDK-975]); and the benchmark tables
were re-measured on a quiet machine, on the corpus the demo actually ships.
A release audit of this delta ran before the tag — parent [GDK-980].

## v0.18.0 — 2026-08-26

**A terminal, inside gadak.** ⌘K → Terminal, or `Ctrl+\``. It is a real
shell in the same window as your issues — so you can run a coding agent
there and watch it move the board next to it. Same terminal in the web tab,
in the macOS app, and on a paired phone. Korean composition lands on the cursor, where a
terminal canvas does not make that automatic. It ships **Beta** — useful, and
we would rather name the rough edges than hide them ([GDK-862], [GDK-864],
[GDK-865], [GDK-892], [GDK-895], [GDK-956]).

**A token that opens a shell and nothing else.** `gadak pairing mint --scope
terminal` is the only kind that opens one; a `serve` or `origin` token opens
none — though a `serve` token now reaches the whole mirror REST, not a
13-path allowlist. Revoke a terminal token and the shells it opened close
within seconds, and are told why. Loopback still needs no token at all
([GDK-863], [GDK-883]).

**The phone app is something you can use.** Issues in your own saved views,
wiki pages beside them, search that shows page hits, an issue detail that
lands on the thread, and notifications only for what is actually yours —
assigned, mentioned, reopened. It reaches the terminal too, over a second
token, and unpairing forgets that shell too. Internal TestFlight builds ship
in one command ([GDK-805], [GDK-867], [GDK-870], [GDK-879], [GDK-884],
[GDK-885], [GDK-886], [GDK-887], [GDK-888], [GDK-905], [GDK-906], [GDK-907],
[GDK-908], [GDK-910]).

**`gadak create --type Bug` works on a non-English Jira.** It used to fail
once on every localised site and make you look up the type id. Names, ids,
`epic`/`subtask`, and a small locale table all resolve now; two matches is a
clear error rather than a guess ([GDK-741]).

**The wiki's scope is stated before you install.** It was always opt-in and
always per-space — `gadak init --spaces ENG,PROD`, or Settings → Sources —
but nothing said so until you had already installed and run a sync. Someone
told us they had not tried gadak because a whole Confluence looked like too
much to mirror. It never was ([GDK-964]).

**The changelog you are reading is on the site now**, in both languages,
rendered from the repository rather than copied — plus the search-engine
basics the site never had: canonical links, a sitemap, and structured data
for the releases.

**Smaller things.** A dashboard link opens an issue without losing the wall
([GDK-880]). `gadak open` no longer scans ports to find a running serve
([GDK-859]). WebSocket upgrades are origin-checked ([GDK-860]). One stale
cached row can no longer blank the whole UI ([GDK-835]). `gadak dev --help`
lists every verb it has, and `gadak pairing` lists, like every other list
command ([GDK-946], [GDK-947]). The agent skill now carries a complete,
working dashboard example instead of pointing at a file that was never
installed beside it ([GDK-963]).

## v0.17.3 — 2026-08-25

**The phone stopped being a skeleton.** A paired iPhone reads your mirror
over its own `serve` pairing scope — a one-way door, so that token cannot
ride the origin passthrough and an origin token cannot dump the mirror — and
`pairing mint` works on a connected workspace. On the phone: a quiet queue as
the first screen with real status names, pairing that proves the connection
it claims and explains each failure, search that answers before you type
(recent searches, your saved views as chips), an issue detail whose comment
draft survives a failed send, and notifications only for assigned, mentioned
and reopened — silent while you are looking at the app ([GDK-796], [GDK-797],
[GDK-798], [GDK-799], [GDK-800], [GDK-801], [GDK-802], [GDK-837]).

**Spacing, layout and type are yours too, not just colour.** `ui.tokens`
grew three more axes, and setting one no longer risks the rest — each is a
key-wise merge, so a bad write leaves the config untouched. One token is a
path of its own: `gadak config set ui.tokens.type.terminal 15px`, no JSON and
no quoting, with an unknown name refused rather than stored as a typo. `gadak
config get ui.tokens.dim-catalog` lists every name with its default, its
range, and what it has to move together with ([GDK-842], [GDK-849],
[GDK-850], [GDK-852], [GDK-853]).

**Your look is yours: validation warns and saves instead of refusing.**
Contrast, colour distance, deuteranopia and the token relations all still run
and still tell you what you are about to get — but only what the machine
genuinely cannot honour is rejected. The warnings carry the next move, not
just the diagnosis: a contrast line names the palettes that fail and where to
fix them, a type line prints the whole ladder that has to move together. The
way out of any look is always one CLI line ([GDK-856], [GDK-857], [GDK-858]).

**A dashboard link can open an issue.** A wall can now navigate the app to
any of its own routes — an issue, a saved view, a filtered list, a search —
and an external link opens a new tab instead of replacing the wall
([GDK-854]).

## v0.17.2 — 2026-08-25

**Your agent can build you a dashboard.** One HTML document plus named
queries, saved like a view and rendered full-tab in the running web UI. The
host runs the SQL (or JQL) and hands the rows in; the page itself never
reaches the network. Saving re-renders an open tab in about a second, and new
mirror data re-pushes on its own. Charts work offline — uPlot ships inside
gadak, so there is no CDN and no loosened policy ([GDK-781], [GDK-782],
[GDK-792], [GDK-793]).

**And any chart library you want, downloaded once.** `gadak dashboards lib
add <url>` fetches a library, pins its hash, and serves it locally — re-hashed
on every request, so a file tampered with after the fact fails closed instead
of running. Dashboards name the libraries they use; ones that do not are
unaffected. three.js stopped shipping inside the binary and became the
documented example instead, which is 750 KB off every download ([GDK-808]).

**Colours are yours.** `ui.tokens`, `ui.tokensByTheme` and `ui.dataColors`
let you repaint the window, with `ui.tokens.catalog` to discover the names.
Writes are checked against the same contrast rules the shipped palettes had
to pass, an open tab repaints without a reload, and a boot cache means a
customised install never flashes the default palette first ([GDK-785],
[GDK-786], [GDK-787], [GDK-791]).

**`gadak sql` stopped answering from a stale copy.** An unprefixed table name
now falls through to the live `local.db` rather than a snapshot frozen at
migration time, which had been answering quietly and wrong ([GDK-824]).

**The staleness warning names which source is stale.** `mirror last synced
154h ago` came from the oldest row across every source, so one quiet
Confluence space made the whole mirror read as six days old while `status`
showed a watermark ten minutes back — two screens, opposite stories, no way
to tell which to believe. It now names the source and prints the same
timestamp `status` does ([GDK-810]).

**A page id you read is a page id you can write to.** Search prints it, the
JSON help admits pages exist, and the agent recipes emit the id the origin
actually accepts ([GDK-816]).

**Two typos the CLI used to swallow.** `gadak create GDK "…"` filed an issue
whose title began with a project key; it is now refused with the `--project`
spelling, and only when the first word really is a key this workspace knows.
`config set projects` accepted any string; keys are shape-checked, the site
is asked whether they exist, and `status` and `sync` name both sides of a
scope that has drifted ([GDK-594], [GDK-809]).

**Smaller things.** A dashboard can no longer paint over the issue list
([GDK-815], [GDK-821]), and it answers Esc like everything else ([GDK-827]).
A collapsed documents tree stays collapsed across a sync ([GDK-817]). The
feed takes the column instead of overlaying it, so Esc from the feed lands on
the list. Fourteen error messages across three languages now name the next
move rather than only the problem ([GDK-828], [GDK-829], [GDK-831]). The
excerpt a list shows and the text search indexes come from one place, so they
cannot drift apart ([GDK-814]).

## v0.17.1 — 2026-08-24

The patch where the mirror learned to share. A day of using gadak on a
20,000-issue mirror found every way two gadak processes could end up waiting
on one file.

**A standalone workspace keeps its record in SQLite.** The embedded tracker
writes to `origin/issuetap.db`, one transaction per write, instead of
rewriting a whole YAML file on a timer. An existing YAML seeds it once and is
left alone as your rollback; export still speaks YAML. Backup is that file:
stop the app and copy it, or use `sqlite3 .backup` while it runs ([GDK-202]).

**"Database is busy" tells you who is holding it.** A write that reached
Jira no longer fails just because the local re-read collided with another
process, and a genuine refusal names the neighbour — another app, a `serve`,
a CLI — instead of an error code. `gadak doctor` lists them. Browsing history
took its own connection, so reading no longer queues behind a sync, and an
agent's reads wait politely rather than failing instantly ([GDK-740],
[GDK-753], [GDK-754], [GDK-757], [GDK-755]).

**Faster where it was slowest, measured at 20k issues.** `gadak issue KEY`
reads that one key instead of loading the mirror. `search --jql` resolves
people narrowly, on the CLI and on the server. `doctor` samples instead of
scanning every document ([GDK-747], [GDK-748], [GDK-749], [GDK-756]).

**Exclude works on every filter.** A ⊘ on any value in any picker (Alt-click
too), replacing a modal toggle that only some menus had. Copy JQL writes
`not in` where JQL can say it and tells you which axes it left out, and
`search --jql` matches the same negations ([GDK-771]).

**Narrow windows stop clipping.** An audit of every seam below 1100px closed
three: a chip that would not hide, row columns cut mid-character, and a
minimum width that disagreed with the layout it was supposed to describe. A
CI check keeps all three closed ([GDK-758], [GDK-766]).

**On gadak.dev:** a Korean browser is offered the Korean page — a suggestion,
never a redirect, and it remembers your answer ([GDK-770]). Plus `llms.txt`
for agents reading the site, and landing media that shows the product at
readable scale instead of full-screen video ([GDK-751], [GDK-752]).

## v0.17.0 — 2026-08-23

The cycle where an agent's writes grew up. An issue shows the code that
implements it, the write verbs learned what a coding agent actually sends,
and a workspace stopped being something you re-select on every command.

**An issue knows its pull requests.** PRs, commits, deployments, builds and
the people on them, mirrored from a connected site and writable on a
standalone one. `gadak dev scan` sweeps a repository's PRs into an issue in
one pass, `gadak dev link` writes one, and the web opens a GitHub link
in-app. Those links survive the next sync. When the panel is empty it says
why ([GDK-495], [GDK-496], [GDK-497], [GDK-527], [GDK-531], [GDK-536],
[GDK-537], [GDK-538], [GDK-539], [GDK-540], [GDK-541], [GDK-555], [GDK-562],
[GDK-589], [GDK-592]). Issue links are no longer a reason to leave the app:
`gadak link A B --type blocks`, or the detail panel ([GDK-19], [GDK-85]).

**Write verbs that do what a project requires.** `create` and `edit` take
`--field alias=value` for required custom fields, and the create dialog
learns what this project and this issue type actually demand. `transition`
carries `--resolution`, `--field` and a comment. `edit` writes fix versions
and components by name. `assign` takes a name or an account id, not only an
email. A wrong-typed field is refused rather than written as an empty string.
A rejected parent lists the epics you could have picked ([GDK-254],
[GDK-330], [GDK-509], [GDK-513], [GDK-514], [GDK-515], [GDK-516], [GDK-517],
[GDK-635], [GDK-643]).

**A write that reached Jira counts as a success** even if the local re-read
right afterwards did not ([GDK-740]). Bulk reads take many keys, or `--keys
-`, with nothing silently dropped ([GDK-328], [GDK-425]). `gadak claim KEY`
takes an issue in one move, and `gadak issue` shows how long the work sat —
`wait 3d · progress 5h` ([GDK-591]).

**Writes carry who made them.** `GADAK_ACTOR` names the agent, and the web
marks bot work with a badge, so a machine's edit is not indistinguishable
from yours. A standalone workspace speaks your language, and a restricted
issue looks different from a public one ([GDK-519], [GDK-586], [GDK-588],
[GDK-590], [GDK-593], [GDK-597]).

**A workspace stays chosen.** `gadak workspace use <name>` stores a default.
Pairing tells the truth about what it is and what failed, a bound workspace
cannot be quietly repointed at a different site, and replacing an origin
takes its derived rows with it ([GDK-418], [GDK-433], [GDK-449], [GDK-452],
[GDK-453], [GDK-490], [GDK-561], [GDK-677], [GDK-678]).

**Korean search finds the word inside the compound.** Plus: fix versions keep
their ids and the project's release catalog reaches the mirror, sprint is a
column you can query, and JQL `parent =` / `parent IN` filter locally
([GDK-259], [GDK-444], [GDK-518], [GDK-521], [GDK-532], [GDK-329]).

**`rm gadak.db` no longer costs you anything.** Saved views, visits and
search history moved out of the mirror, which is supposed to be the throwaway
half ([GDK-105]).

**For agents.** `gadak pick` chooses work. `gadak recents` walks back what
the CLI has read. `gadak sync --if-stale 15m` is the session opener an agent
can call blind. Batch writes answer per key, honestly. Closing an issue is
one round trip that is safe to retry ([GDK-500], [GDK-501], [GDK-502],
[GDK-503], [GDK-598], [GDK-599]).

**The window, made consistent.** Esc closes the thing it was aimed at and
nothing else. Four type sizes and nothing between them. Empty is a state with
words, not a blank. A transition that needs a screen asks inline; components
and parent edit in place; a story shows its children. There is one kind of
saved view. A read that finishes quickly paints no skeleton at all. And a
full Japanese catalog ([GDK-83], [GDK-86], [GDK-121], [GDK-129], [GDK-130],
[GDK-316], [GDK-437], [GDK-604], [GDK-613], [GDK-617], [GDK-626], [GDK-737],
[GDK-738], [GDK-739]).

**Desktop.** A second launch raises the window you have instead of starting a
rival. On Windows: `gadak://` links work, `install-cli` speaks Windows, and
the app stops claiming it notified you when it did not ([GDK-349],
[GDK-350], [GDK-351], [GDK-353], [GDK-658], [GDK-700]).

**Network, audited.** An empty host counts as a non-loopback bind, so `serve`
demands `--allow-remote` for it like any other exposure ([GDK-542]). Linear's
rate limit is a retry rather than a death, and a Linear-only workspace is a
configured workspace ([GDK-263], [GDK-654]). The Web Push client is gone: it
called endpoints the server answers 404 to, and vendor push services are
outbound traffic this project does not make ([GDK-711]).

**gadak's own backlog is public**, at gadak.dev, with a front door and a demo
beside it — and a page explaining that Windows warning ([GDK-211],
[GDK-389], [GDK-676]).

## v0.16.1 — 2026-08-20

The release that finishes what 0.16 started.

**Linear is a third tracker, and gadak writes to it.** A `"linear"` block in
your workspace config and `gadak sync --source linear` mirror issues,
comments, labels and attachments. Writes route by whichever origin owns the
row, and what Linear cannot do yet refuses honestly instead of half-applying.
Jira, standalone and Linear all answer the same write verbs ([GDK-263],
[GDK-359], [GDK-360], [GDK-361]).

**The wiki stops being read-only.** Create a page, edit its title or body,
comment on it — all through the origin, from the CLI or the REST API
([GDK-344], [GDK-380], [GDK-381], [GDK-382]).

**Two gadak processes stop fighting over a standalone workspace.** The
desktop app advertises its origin the way `serve` does, so an app and a CLI
cannot both hold the record file. An acknowledged write is on disk before you
get the answer, and a write that could not be persisted fails instead of
pretending. A standalone failure no longer reports itself as a missing
credential, and converting a workspace says what conversion actually does to
your local-only issues ([GDK-241], [GDK-333], [GDK-340], [GDK-342],
[GDK-343], [GDK-345], [GDK-346], [GDK-347], [GDK-348]).

**Agents learn that standalone exists.** The embedded skill knows the word,
the CLI says which origin it means, and `transition` names each target's
`status_id` — and accepts the one the read path just handed out, which is the
loop an agent kept failing on. `issues_full` gained `description_text`, and a
standalone `init` fills the mirror so nothing starts empty ([GDK-239],
[GDK-312], [GDK-313], [GDK-363], [GDK-364], [GDK-365], [GDK-366], [GDK-367],
[GDK-368], [GDK-371], [GDK-376]).

**Docs that stop contradicting the product.** The install page admits
standalone exists, the FAQ stops telling you to `rm -rf ~/.gadak`, the
network gets its own page, and export/import finally has a paragraph
([GDK-271], [GDK-372], [GDK-373], [GDK-374], [GDK-375], [GDK-601]).

## v0.16.0 — 2026-08-19

The release where gadak stops needing an Atlassian account to be useful,
stops needing a Mac to run, and stops being read-only about the fields people
actually triage by.

**A workspace without an Atlassian account.** Standalone: the origin is a
minimal tracker that runs inside gadak and travels with it. The mirror is
still a disposable cache and every write still goes through the origin — the
only change is who the origin is. A workspace is bound to one origin, so
connecting a credential cannot quietly repoint it somewhere else. Standalone
wikis write through the same path ([GDK-183], [GDK-237], [GDK-238],
[GDK-247], [GDK-267]).

**Windows and Linux.** Windows gets a portable pack, an installer path, a
working `install-cli`, a Scoop manifest, and `gadak://` links that survive
first launch. Linux gets a tarball install beside brew and an AUR packaging
kit. Omarchy gets a bar widget showing what changed in *your* mirror
([GDK-115], [GDK-116], [GDK-208], [GDK-209], [GDK-225], [GDK-229],
[GDK-246], [GDK-293]).

**Edit an issue where you read it.** Due dates set and cleared from the
detail panel, with Jira's refusal shown as a sentence you can read.
Descriptions edit as plain text, with a guard before anything rich gets
destroyed. `p` opens a priority menu wherever `s`/`a`/`l` already work. What
is editable comes from the issue's own metadata rather than a fixed list, so
your site's custom fields are included ([GDK-82], [GDK-223], [GDK-249],
[GDK-250], [GDK-251], [GDK-322], [GDK-323], [GDK-331], [GDK-332]).

**The palette can file an issue** from whatever you just typed, required
fields with obvious answers stop being questions, and posting a comment
finally tells you it landed ([GDK-217], [GDK-218], [GDK-300], [GDK-301],
[GDK-302]).

**A non-English Jira stops silently returning nothing.** Status, priority and
issue type key on ids and categories everywhere instead of display names —
`status = 'In Progress'` is zero rows on a Korean account, and that class of
quiet wrong answer is closed ([GDK-161], [GDK-248], [GDK-272], [GDK-275]).
Korean mid-compound search works too ([GDK-259]).

**Updates that tell you, and do not act.** gadak notices a new release, says
the right thing for your platform, and renders the notes in the app. It never
updates itself ([GDK-213], [GDK-214], [GDK-215], [GDK-216]).

**Smaller things.** A cold open no longer serialises everyone behind it, a
contended write waits instead of dying instantly, and a background sync stops
outliving the server that started it ([GDK-270], [GDK-282], [GDK-305]). The
hosted demo opens on the product, and feedback channels live in Settings and
the macOS Help menu ([GDK-335], [GDK-336]). A read-only Linear client landed
as groundwork, deliberately not wired to workspaces yet — that is 0.16.1
([GDK-258], [GDK-261], [GDK-263], [GDK-274]).

## v0.15.2 — 2026-08-17

The release where settings stop being a screen.

**Every field the settings dialog edits is also a CLI verb.** `gadak config
list | get | set` and the settings API go through one table, so they cannot
disagree — which means an agent can set up a workspace end to end. Themes live
in the workspace config file, so picking one in the UI and setting it from a
terminal are the same act ([GDK-190], [GDK-193]).

**Three darks, and one of them is yours.** `dark` is a neutral-cool charcoal,
`ink` is a new blue-black, and `ember` keeps the previous warm dark exactly as
it was ([GDK-190]).

**Smaller things.** A bare number finds that issue in any project, on every
search surface ([GDK-186]). The settings dialog stops repeating its mirror
block above every tab ([GDK-188]). Menus stopped installing things behind
your back — Settings → Integrations does that, and says what is already
installed ([GDK-189], [GDK-191]).

## v0.15.1 — 2026-08-17

- `gadak raycast install` carries the Raycast extension inside the binary, so
  a brew or app install needs no checkout ([GDK-182]).
- The ⌘K palette is never blank: an empty query shows recently updated issues
  under recently viewed, plus your saved views ([GDK-184]).
- Settings → Integrations (desktop) lists the agent surfaces gadak installs
  into, detects honestly what is already there, and shows a live log
  ([GDK-185]).

## v0.15.0 — 2026-08-17

The release that opens gadak outward. A view or an issue is a link any app
can hand over, search is fast enough to sit under someone else's keystroke,
and there is a dark theme built to the same standard as the light one.

**A piece of gadak travels as a link.** `gadak://` deep links, with every
place in the app addressable — and gadak produces the links it consumes: a
copy-link action in the UI, `gadak issue KEY --link` on the CLI ([GDK-119],
[GDK-124], [GDK-163], [GDK-164]).

**Search fast enough to drive another app's UI.** Typing an issue key finds
that issue; on a 20,000-issue mirror the worst case went from 1.6s to 110ms.
That is what makes a launcher extension feel local ([GDK-117], [GDK-166],
[GDK-170]).

**An issue can name its parent.** `gadak create --parent` and `gadak edit
--parent` write the sub-issue relationship through Jira ([GDK-19], [GDK-86]).

**A dark theme, done properly.** Warm ground, ink foregrounds, the same paper
metaphor as light, and no flash on first paint. Both palettes clear the same
measured floors: status colours stay distinguishable in normal and
colour-blind vision, and success and failure are never told by colour alone
([GDK-154], [GDK-156], [GDK-157], [GDK-158], [GDK-159], [GDK-162],
[GDK-171]).

**The list behaves like a list.** The right side of a row is a column you can
scan, the last row stops being cut in half, Esc closes what you are looking
at, and a panel that covers the list says so ([GDK-128], [GDK-131],
[GDK-132], [GDK-133]).

**Korean typing stopped fighting the search box.** A half-composed syllable is
not a query, and chosung matching is gone product-wide — it was matching
things you did not mean ([GDK-168], [GDK-169]).

**Honesty at the edges.** The hosted demo stops advertising verbs it cannot
answer ([GDK-52]). A read-only home is a warning, not a refusal to start
([GDK-149], [GDK-173]). Copy means copied, an attachment is fetched at most
once, and the desktop app stops loading its runtime twice ([GDK-150],
[GDK-177], [GDK-178]).

## v0.14.2 — 2026-08-16

The release about the first ten minutes, and the day your token dies. Nothing
here is a new capability so much as an existing one that finally tells you
what it is doing.

**Every token trap is named before you paste, not after the 401** — and a
rejected token is recoverable without having to write anything first
([GDK-68], [GDK-69], [GDK-98]). Expiry is warned about before the sync dies
([GDK-67]).

**Picking no projects is a choice, not an unfinished form** ([GDK-99]).

**`gadak skill install` treats an upgrade as an upgrade**, and the embedded
skill knows the verbs the CLI actually has ([GDK-91], [GDK-92]).

**A quiet wiki costs almost nothing to sync** — a tick over an unchanged
Confluence reads zero page bodies ([GDK-113]).

**`gadak issue <KEY> --derive` shows where the derived columns came from**
([GDK-111]).

**Smaller things.** History keeps its order ([GDK-26]). `gadak sql` warns on
a stale mirror ([GDK-90]). Opening a mirror repairs a search index this build
cannot write ([GDK-112]). The browse pane yields to Escape ([GDK-78]).
Search help works on touch ([GDK-53]).

## v0.14.1 — 2026-08-15

One day of using gadak on gadak's own backlog, shipped as it landed.

**The first CLI write verbs:** `gadak create`, `gadak attach`, `gadak edit`.

**The macOS app is notify-only.** The in-app self-updater — never exercised,
never earned — is gone, and this release deliberately ships no desktop zip
([GDK-58], [GDK-61]).

**The hosted demo works where people actually tap it:** inside in-app
browsers, with a first paint readable at phone width ([GDK-23], [GDK-51]).

**Failures say what happened.** A truncated key list says how many were
dropped, and a rejected credential stops the watch loop for every source
rather than one ([GDK-24], [GDK-35], [GDK-48]). Priority colours read the
rank, not the account's language. Keystrokes during boot are held rather than
dropped ([GDK-46], [GDK-76]).

## v0.14.0 — 2026-08-15

The release about trust: surfaces that fail loudly instead of silently, docs
that match the code, and measured numbers instead of adjectives.

**The first agent call succeeds, or says why not.** `gadak_search` takes
`query`, every tool error starts with `ERROR:` and echoes the keys it got,
and a response over the size cap sheds the oldest comments and says
`truncated` rather than lying about completeness.

**Three things are a promise you can build on:** `issues_full` plus the
RECIPES queries, `gadak sql` stdout, and `views open --keys -`.

**`gadak export` / `gadak import` round-trip what you would actually miss** —
saved views, watches, favourites — carrying no credentials and no site URL.

**Numbers, with the rows where gadak loses.** Measured against a live
2,853-issue Cloud project: 42× on a simple filter, 162× on an epic
`GROUP BY`, and a reopen count that takes about twenty minutes over REST
against 14.5 ms locally.

**`brew install midagedev/tap/gadak` is the app now**; `gadak-cli` is the
CLI-only formula. Plus a Korean README, and a settings dialog that stops
claiming things about project selection that were not true.

## v0.13.0 — 2026-08-14

The release that puts search, history and the agent's window in one place.

**One search box that searches everything.** ⌘K queries every issue and
document in one index, ignoring the filter chips on the list. The box above
the list keeps its old job: narrow what is already there.

**History is a file beside the mirror.** Issues, documents and searches on
one timeline in `~/.gadak/local.db` — so throwing away the mirror does not
throw that away, and an agent can join what you visited to what you have in
a single `gadak sql`.

**The window follows the agent.** An arbitrary set of issue keys is a
first-class view, so `gadak views open --keys -` puts an agent's answer on
your running window. `gadak views open` opens in gadak; `gadak open` leaves
for Jira. Hosts without a shell get the same through a new MCP tool.

**Paste a Jira URL, get the filters.** A navigator URL or a `jql=` clause
applies the matching chips, Copy JQL is the way back, and anything in the
unsupported subset is listed rather than silently dropped. Your Jira saved
filters show up in the sidebar, and `gadak views` lists, shows, opens and
saves them.

**Wiki scope became real.** Each Confluence space carries its own watermark,
a newly selected space backfills in full, and a space that leaves the scope
is removed.

**People are matched by account id, not by email.** Person filters no longer
depend on your site making email addresses visible (#1, thanks @elppaaa) —
across JQL, saved views, filters and the member directory.

**The macOS window can be dragged** (#2, thanks @wafe).

**Smaller things.** Comment-only wiki edits reach the mirror, an unchanged
page stops bumping its version, and a deleted issue is tombstoned by a
single-item sync. An unknown `--profile` errors with the real list. A failed
mirror re-read after an upload returns the documented error instead of
pretending.

## v0.12.0 — 2026-08-13

The look-and-rename release. gadak is a strand (가닥): uncoated paper, sumi
ink, one 쪽빛 thread.

**Paper, not a dark dashboard.** The mark is 가 drawn as two strokes. The
crystal-ball dashboard and the TUI are gone.

**Renamed to gadak** — binary, home directory (`~/.gadak`), environment
prefix (`GADAK_*`), MCP tools, module path and desktop bundle id. An existing
`~/.scry` tree is renamed on first launch, and `SCRY_*` is still read
wherever the `GADAK_*` equivalent is unset.

**Labels and priority became things you change.** Labels stay visible on the
list, edit on the issue, and apply to a selection from the bulk bar (`l`,
beside `s` and `a`). Priority writes by id from the site's own catalog. The
title is editable.

**Workspaces work in the desktop app**, and every workspace with a credential
gets its own sync loop.

**Document lists stopped freezing on a large mirror** — 4,433 ms to 68 ms on
a 10,000-page window.

**Smaller things.** The native title bar is gone; window controls moved into
the sidebar. `gadak skill install` embeds the Claude Code skill without
needing MCP, and `gadak install-cli` puts the running binary on your PATH.
`gadak doctor` prints redacted diagnostics you can paste into a bug report.
`gadak api` is the raw Atlassian REST escape hatch, refused at a foreign
host.

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
[GDK-90]: https://gadak.dev/backlog/#/?ks=GDK-90
[GDK-91]: https://gadak.dev/backlog/#/?ks=GDK-91
[GDK-92]: https://gadak.dev/backlog/#/?ks=GDK-92
[GDK-98]: https://gadak.dev/backlog/#/?ks=GDK-98
[GDK-99]: https://gadak.dev/backlog/#/?ks=GDK-99
[GDK-105]: https://gadak.dev/backlog/#/?ks=GDK-105
[GDK-111]: https://gadak.dev/backlog/#/?ks=GDK-111
[GDK-112]: https://gadak.dev/backlog/#/?ks=GDK-112
[GDK-113]: https://gadak.dev/backlog/#/?ks=GDK-113
[GDK-115]: https://gadak.dev/backlog/#/?ks=GDK-115
[GDK-116]: https://gadak.dev/backlog/#/?ks=GDK-116
[GDK-117]: https://gadak.dev/backlog/#/?ks=GDK-117
[GDK-119]: https://gadak.dev/backlog/#/?ks=GDK-119
[GDK-121]: https://gadak.dev/backlog/#/?ks=GDK-121
[GDK-124]: https://gadak.dev/backlog/#/?ks=GDK-124
[GDK-128]: https://gadak.dev/backlog/#/?ks=GDK-128
[GDK-129]: https://gadak.dev/backlog/#/?ks=GDK-129
[GDK-130]: https://gadak.dev/backlog/#/?ks=GDK-130
[GDK-131]: https://gadak.dev/backlog/#/?ks=GDK-131
[GDK-132]: https://gadak.dev/backlog/#/?ks=GDK-132
[GDK-133]: https://gadak.dev/backlog/#/?ks=GDK-133
[GDK-149]: https://gadak.dev/backlog/#/?ks=GDK-149
[GDK-150]: https://gadak.dev/backlog/#/?ks=GDK-150
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
[GDK-270]: https://gadak.dev/backlog/#/?ks=GDK-270
[GDK-271]: https://gadak.dev/backlog/#/?ks=GDK-271
[GDK-272]: https://gadak.dev/backlog/#/?ks=GDK-272
[GDK-274]: https://gadak.dev/backlog/#/?ks=GDK-274
[GDK-275]: https://gadak.dev/backlog/#/?ks=GDK-275
[GDK-282]: https://gadak.dev/backlog/#/?ks=GDK-282
[GDK-293]: https://gadak.dev/backlog/#/?ks=GDK-293
[GDK-300]: https://gadak.dev/backlog/#/?ks=GDK-300
[GDK-301]: https://gadak.dev/backlog/#/?ks=GDK-301
[GDK-302]: https://gadak.dev/backlog/#/?ks=GDK-302
[GDK-305]: https://gadak.dev/backlog/#/?ks=GDK-305
[GDK-312]: https://gadak.dev/backlog/#/?ks=GDK-312
[GDK-313]: https://gadak.dev/backlog/#/?ks=GDK-313
[GDK-316]: https://gadak.dev/backlog/#/?ks=GDK-316
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
[GDK-418]: https://gadak.dev/backlog/#/?ks=GDK-418
[GDK-425]: https://gadak.dev/backlog/#/?ks=GDK-425
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
[GDK-604]: https://gadak.dev/backlog/#/?ks=GDK-604
[GDK-613]: https://gadak.dev/backlog/#/?ks=GDK-613
[GDK-617]: https://gadak.dev/backlog/#/?ks=GDK-617
[GDK-626]: https://gadak.dev/backlog/#/?ks=GDK-626
[GDK-635]: https://gadak.dev/backlog/#/?ks=GDK-635
[GDK-643]: https://gadak.dev/backlog/#/?ks=GDK-643
[GDK-654]: https://gadak.dev/backlog/#/?ks=GDK-654
[GDK-658]: https://gadak.dev/backlog/#/?ks=GDK-658
[GDK-676]: https://gadak.dev/backlog/#/?ks=GDK-676
[GDK-677]: https://gadak.dev/backlog/#/?ks=GDK-677
[GDK-678]: https://gadak.dev/backlog/#/?ks=GDK-678
[GDK-711]: https://gadak.dev/backlog/#/?ks=GDK-711
[GDK-738]: https://gadak.dev/backlog/#/?ks=GDK-738
[GDK-739]: https://gadak.dev/backlog/#/?ks=GDK-739
[GDK-740]: https://gadak.dev/backlog/#/?ks=GDK-740
[GDK-737]: https://gadak.dev/backlog/#/?ks=GDK-737
[GDK-700]: https://gadak.dev/backlog/#/?ks=GDK-700
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
[GDK-824]: https://gadak.dev/backlog/#/?ks=GDK-824
[GDK-814]: https://gadak.dev/backlog/#/?ks=GDK-814
[GDK-815]: https://gadak.dev/backlog/#/?ks=GDK-815
[GDK-816]: https://gadak.dev/backlog/#/?ks=GDK-816
[GDK-817]: https://gadak.dev/backlog/#/?ks=GDK-817
[GDK-821]: https://gadak.dev/backlog/#/?ks=GDK-821
[GDK-827]: https://gadak.dev/backlog/#/?ks=GDK-827
[GDK-828]: https://gadak.dev/backlog/#/?ks=GDK-828
[GDK-829]: https://gadak.dev/backlog/#/?ks=GDK-829
[GDK-831]: https://gadak.dev/backlog/#/?ks=GDK-831
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
[GDK-864]: https://gadak.dev/backlog/#/?ks=GDK-864
[GDK-835]: https://gadak.dev/backlog/#/?ks=GDK-835
[GDK-892]: https://gadak.dev/backlog/#/?ks=GDK-892
[GDK-895]: https://gadak.dev/backlog/#/?ks=GDK-895
[GDK-865]: https://gadak.dev/backlog/#/?ks=GDK-865
[GDK-805]: https://gadak.dev/backlog/#/?ks=GDK-805
[GDK-964]: https://gadak.dev/backlog/#/?ks=GDK-964
[GDK-956]: https://gadak.dev/backlog/#/?ks=GDK-956
[GDK-950]: https://gadak.dev/backlog/#/?ks=GDK-950
[GDK-960]: https://gadak.dev/backlog/#/?ks=GDK-960
[GDK-981]: https://gadak.dev/backlog/#/?ks=GDK-981
[GDK-944]: https://gadak.dev/backlog/#/?ks=GDK-944
[GDK-967]: https://gadak.dev/backlog/#/?ks=GDK-967
[GDK-968]: https://gadak.dev/backlog/#/?ks=GDK-968
[GDK-971]: https://gadak.dev/backlog/#/?ks=GDK-971
[GDK-975]: https://gadak.dev/backlog/#/?ks=GDK-975
[GDK-980]: https://gadak.dev/backlog/#/?ks=GDK-980
[GDK-963]: https://gadak.dev/backlog/#/?ks=GDK-963
[GDK-741]: https://gadak.dev/backlog/#/?ks=GDK-741
[GDK-946]: https://gadak.dev/backlog/#/?ks=GDK-946
[GDK-947]: https://gadak.dev/backlog/#/?ks=GDK-947
[GDK-859]: https://gadak.dev/backlog/#/?ks=GDK-859
[GDK-860]: https://gadak.dev/backlog/#/?ks=GDK-860
[GDK-880]: https://gadak.dev/backlog/#/?ks=GDK-880
[GDK-884]: https://gadak.dev/backlog/#/?ks=GDK-884
[GDK-885]: https://gadak.dev/backlog/#/?ks=GDK-885
[GDK-886]: https://gadak.dev/backlog/#/?ks=GDK-886
[GDK-887]: https://gadak.dev/backlog/#/?ks=GDK-887
[GDK-888]: https://gadak.dev/backlog/#/?ks=GDK-888
[GDK-905]: https://gadak.dev/backlog/#/?ks=GDK-905
[GDK-906]: https://gadak.dev/backlog/#/?ks=GDK-906
[GDK-907]: https://gadak.dev/backlog/#/?ks=GDK-907
[GDK-908]: https://gadak.dev/backlog/#/?ks=GDK-908
[GDK-910]: https://gadak.dev/backlog/#/?ks=GDK-910
[GDK-867]: https://gadak.dev/backlog/#/?ks=GDK-867
[GDK-870]: https://gadak.dev/backlog/#/?ks=GDK-870
[GDK-879]: https://gadak.dev/backlog/#/?ks=GDK-879
[GDK-1175]: https://gadak.dev/backlog/#/?ks=GDK-1175
[GDK-1176]: https://gadak.dev/backlog/#/?ks=GDK-1176
[GDK-1190]: https://gadak.dev/backlog/#/?ks=GDK-1190
[GDK-1248]: https://gadak.dev/backlog/#/?ks=GDK-1248
[GDK-1024]: https://gadak.dev/backlog/#/?ks=GDK-1024
[GDK-1158]: https://gadak.dev/backlog/#/?ks=GDK-1158
[GDK-1196]: https://gadak.dev/backlog/#/?ks=GDK-1196
[GDK-1197]: https://gadak.dev/backlog/#/?ks=GDK-1197
[GDK-1194]: https://gadak.dev/backlog/#/?ks=GDK-1194
[GDK-1199]: https://gadak.dev/backlog/#/?ks=GDK-1199
[GDK-1200]: https://gadak.dev/backlog/#/?ks=GDK-1200
[GDK-1250]: https://gadak.dev/backlog/#/?ks=GDK-1250
[GDK-1251]: https://gadak.dev/backlog/#/?ks=GDK-1251
[GDK-1097]: https://gadak.dev/backlog/#/?ks=GDK-1097
[GDK-1096]: https://gadak.dev/backlog/#/?ks=GDK-1096
[GDK-1098]: https://gadak.dev/backlog/#/?ks=GDK-1098
[GDK-1051]: https://gadak.dev/backlog/#/?ks=GDK-1051
[GDK-871]: https://gadak.dev/backlog/#/?ks=GDK-871
[GDK-899]: https://gadak.dev/backlog/#/?ks=GDK-899
[GDK-992]: https://gadak.dev/backlog/#/?ks=GDK-992
[GDK-1030]: https://gadak.dev/backlog/#/?ks=GDK-1030
[GDK-1205]: https://gadak.dev/backlog/#/?ks=GDK-1205
[GDK-1001]: https://gadak.dev/backlog/#/?ks=GDK-1001
[GDK-1234]: https://gadak.dev/backlog/#/?ks=GDK-1234
[GDK-1233]: https://gadak.dev/backlog/#/?ks=GDK-1233
[GDK-1244]: https://gadak.dev/backlog/#/?ks=GDK-1244
[GDK-1243]: https://gadak.dev/backlog/#/?ks=GDK-1243
[GDK-1235]: https://gadak.dev/backlog/#/?ks=GDK-1235
[GDK-1075]: https://gadak.dev/backlog/#/?ks=GDK-1075
[GDK-1074]: https://gadak.dev/backlog/#/?ks=GDK-1074
[GDK-1047]: https://gadak.dev/backlog/#/?ks=GDK-1047
[GDK-1246]: https://gadak.dev/backlog/#/?ks=GDK-1246
[GDK-1122]: https://gadak.dev/backlog/#/?ks=GDK-1122
[GDK-996]: https://gadak.dev/backlog/#/?ks=GDK-996
[GDK-1128]: https://gadak.dev/backlog/#/?ks=GDK-1128
[GDK-1204]: https://gadak.dev/backlog/#/?ks=GDK-1204
