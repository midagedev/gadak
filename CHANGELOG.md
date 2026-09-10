# Changelog

<sub>English · <a href="CHANGELOG.ko.md">한국어</a></sub>

## Unreleased

**A self-hosted Jira is an origin type.** `gadak init --site <base-url>
--server` creates a workspace against a self-hosted Jira with a Personal
Access Token: no email, and the base URL may carry a context path. init asks
the site which Jira it is (`/rest/api/2/serverInfo`) and refuses a workspace
whose declared deployment does not match, because a Server instance's answer
to Cloud's `/rest/api/3` says nothing about whether that API exists — measured,
the same route gave 404, 401 and 302 depending only on which credential
asked. The REST dialect belongs to the client: Cloud and the built-in tracker
keep v3, a Server origin gets v2, and the endpoints that differ by more than
a version (create metadata, JQL search, the approximate count, the attachment
media route) answer in their own shape or refuse by name. Measured against a
live Jira Server 11.3.11, `gadak sync` fills the mirror ([GDK-1635],
[GDK-1640], [GDK-1636]). Under it, every shape Server sends differently is
read as Server sends it: wiki markup where Cloud sends ADF, carried verbatim
both ways so a `gadak create -m` no longer stores ADF JSON in a Server
description ([GDK-1637]); users keyed by name with a plain email where Cloud
keys by accountId, so `assignee_id` fills on both and `gadak assign` and user
search speak each dialect's parameter ([GDK-1638]); attachments that live only
at the address Server states, with no `/attachment/content` route — the mirror
keeps that URL reduced to a site-relative path, an address on any other site
is refused rather than fetched, `gadak attach get` returns the original bytes
hash-for-hash on Jira Server 11.3.11, and the app's proxy answers `Range`
with 206 ([GDK-1639]); the
sprint field as the Java `toString` of a bean, `Sprint@4ffcc813[…,id=1,name=Sprint 1,…,state=ACTIVE,…]`, where
Cloud sends an object, so `sprint_id`, `sprint_name` and `sprint_state` are
no longer blank on Server and `ACTIVE`
becomes the `active` queries ask for; the epic in the Epic Link field rather
than `fields.parent`; `/filter/favourite` where `/filter/my` answered 404 on
every sync ([GDK-1650], [GDK-1651], [GDK-1652]); and, with no hierarchy level
on Server's issue types, `epic_key` derived from which type has a standard
child — no extra request, and no keying on the display name "Epic"
([GDK-1658]). Writes stopped trusting a polite origin. Server answers a
standard issue's `parent` with 204 and changes nothing, so `edit --parent`
(and the web's parent editor) send the Epic Link there, refuse by name on a
Server without Jira Software, and every `edit` compares the re-read row with
what it asked: a field that reads the same before and after is reported as
dropped ([GDK-1645]). A login page is no longer mistaken for an answer — Go
follows redirects, so an origin bouncing to its login page answered 200 with
HTML, `gadak api` printed it, a status code cannot see a login page at 200: JSON calls failed with `invalid character '<'`; a 2xx of HTML where JSON was asked for is refused by name, error pages
keep their status and body, and `gadak attach get`, which had written 257,592
bytes of HTML as a `.png` and exited 0. It now compares what was served against
the type the mirror recorded (an `.html` attachment still downloads, and
Cloud's legitimate redirect is still followed) ([GDK-1648], [GDK-1644]). The
guard keys on the one question every case agrees on — is there a body — so
Server's answer to `POST /issueLink` with 201, `text/html` and no body,
which the old guard refused as a login page, is the 201 it always was
([GDK-1662]). Data Center's rate-limit budget is read before the wall instead of after a
429. The
Server client waits out the interval DC states on every response when the
bucket is nearly spent, honours a 429's Retry-After once, and leaves Cloud's
requests unchanged ([GDK-1646]). `docs/SUPPORT_MATRIX.md` reads Jira Cloud,
Jira Server, Linear, Built-in, and every cell in the new column was run:
`tools/jira-server-lab/seed.sh` plants the data each row needs and
`tools/jira-server-lab/measure.sh` runs one command per row against a Jira
Software 11.3.11 Data Center lab the runbook brings up. Two rows are honest
refusals (the wiki: no Confluence Server client), two carry a limit (the
board→project mapping is empty there, and the development panel's refusal
still speaks of Cloud), and the READMEs, the roadmap and the product spec
drop "untested, therefore unclaimed" ([GDK-1634], [GDK-1641]).

**A sprint is an object.** A sprint used to exist only as `sprint_id` /
`sprint_name` / `sprint_state` on each issue, so an empty sprint did not
exist and its goal, dates and board existed nowhere. The mirror has `sprints`
and `boards` tables from the Agile API — the one surface where Cloud and
Server answer the same shape — and `gadak sprint list` reads them (a Linear
workspace lists its cycles there too, so `gadak sprint list` works on every
origin with sprints);
`gadak sprint add`, `remove`, `create`, `start` and `close` write through the
origin and re-read the sprint and its issues rather than trusting what was
sent; a Linear or built-in workspace refuses them by name ([GDK-1653],
[GDK-1654], [GDK-1655], [GDK-1657]). The tracker gadak carries serves Jira
Software's own Agile surface too — boards, sprints, the sprint field, JQL's
`openSprints()` family — so `gadak sprint` works with no Atlassian account
and on a paired workspace, and closing a sprint sweeps its unfinished issues
to the backlog the way Jira does ([GDK-1666]). The sprint controls now appear for the teams that
run sprints rather than for everyone: the axis is the board's own type, so a
kanban team on a site that also has scrum boards no longer sees a scope
control about someone else's cadence, and a cross-project view keeps the old
answer because it has no one team to ask about ([GDK-1689]). The demo mirror's
sprints carry a goal, so the strip's goal line is finally in the frame it was
written for ([GDK-1717]). The `sprints` table is the one
owner of a sprint's state: each issue derives `sprint_state` from it on every
tick and the listing runs on quiet ticks too, so a closed sprint's finished
issues — never re-read by an incremental sync, their `sprint_state` frozen —
no longer read "active" forever and inflate every active-sprint query
([GDK-1661]); each sprint state compiles to its own JQL function instead of
all three becoming `openSprints()` (measured on Jira 11.3.11 with one active
sprint and one future one), and the parser reads all three back ([GDK-1216]).
The board knows about sprints: a scope beside the layout switch (the active
sprint by name, the backlog, or all) that the URL carries, the back button
undoes and a saved view keeps; `gadak views open --jql 'sprint in
openSprints()'` lands on the same board; the filter bar gains a Sprint axis
and a Sprint state axis, the detail panel shows the sprint, and `sprint is EMPTY` round-trips as `sprint_state=none`; the demo fixture carries three
derived sprints so the scope has something to show ([GDK-1656]). The scope
names its axis — it read as sprints only because the demo's active sprint is
called "Sprint 42"; on a Linear origin the same row is "Cycle 1 · Backlog ·
All" ([GDK-1682]). Linear's cycles are sprints as well: a Linear workspace
fills the same three columns, lists its cycles as `sprints` rows with one
board per team, and `sprint add` / `remove` write through (measured against
a live team), while `start`, `close` and `create` refuse by name because a
cycle begins and ends by its dates and Linear generates them from the team's
cadence ([GDK-1667], [GDK-1678]). The history was already in the mirror and
unreachable: Jira records every sprint move in the changelog — on one
measured site the second most common row, behind status and ahead of links —
under whatever custom-field number the site assigned, so `where field = 'sprint'` answered nothing. Sync normalises the field with the id it already
discovers, and three columns follow: `carryover_count`, `first_sprint_id`
and `first_sprint_at`, so "what was added after this sprint began" is a join
against `sprints.start_at`; an origin with no changelog reads NULL rather
than 0, and an existing mirror is recognised without a re-sync ([GDK-1694]).
The sprint's goal is readable — `gadak sprint list` carries it and the
board's scope names it beside the end date ([GDK-1695]) — and the retro can
be cut by sprint instead of by ISO week, `gadak retro --by-sprint` or a
fourth segment on the range control, one column per sprint with the running
one marked, on every origin that has sprints ([GDK-1693]). On the board,
the active sprint has a line of its own between the toolbar and the
columns — name, goal, dates, days left, and a progress bar counted
server-side over the whole sprint, in points too where the workspace maps
them — where before all of that lived in a tooltip ([GDK-1709]); and a card
or row that has been through more than one sprint says so with a quiet
mark, read from the `carryover_count` the mirror had been deriving for no
one ([GDK-1711]).

**Elsewhere: a retro screen, comments you can take back, attachments the
size of real ones, and one fewer outbound call.** `gadak retro`'s document was served for a surface that never came; the
palette's *Weekly retro* opens a calm table, one column per week, one row per metric with its
definition underneath; a cell that holds issues is a door onto that list;
four, eight or twelve weeks ([GDK-1660]). The table now reads as a report:
the running bucket's four numbers stand above it with their step from the
bucket before, every row carries a sparkline, every cell its delta —
coloured only where the team agreed which way is better, never red, and
never on the bucket still running — and the definitions fold behind one
toggle ([GDK-1712]); a sprint cut on a workspace where several boards
carry sprints offers a board picker instead of "could not load"
([GDK-1713]). The report now opens with a sentence — closed, unplanned, reopened, oldest in progress, each number a door — and under it the materials a retrospective is actually held on: the decisions labelled `retro-action` with the metric each named then and now ([GDK-1453]), the age of everything in progress as bars against a p85 line ([GDK-1721]), a per-day density strip with the bucket's surprises named — reopened, churned, and under a sprint cut joined after the start or carried in ([GDK-1722]) — what closed by type and by epic with the cycle time of each as a dot and the unplanned share ([GDK-1723]), the issues you opened that nothing moved beside the ones that moved unseen ([GDK-1725]), and the full table folded at the foot ([GDK-1724]); every section unfolds three lines saying what it is, why a retro reads it and how, and the CLI prints the same sections with `--explain` ([GDK-1726]). All of it works on weekly buckets; only the two sprint surprises need the sprint cut. The demo mirror finally has flow to show: its issue histories had been written seconds apart, so every cycle time read 0.0d, and it carried no reading history at all — the snapshot now draws each issue a lifetime in days and seeds a month of visits, so the demo's retro has cycle percentiles, sessions and resume times ([GDK-1720]). It says why a cell is empty (a
mirror with no `status_catalog` cannot say what `closed` means, and the
reason now travels with the payload and prints under the table, including on
a cold mirror), the definitions are no longer cut off or naming a repository
file ([GDK-1679], [GDK-1681]), the demo mirror has a status catalog derived
from its own issues so `closed`, `in progress` and both wip-age rows have a
value ([GDK-1680]), the definitions read in the reader's language and name
the session gap the report ran with ([GDK-1692]), a cell under a day says
hours or minutes instead of `0.0d` ([GDK-1683]), and `status_changed_at` no
longer drifts in a snapshot onto an instant no transition happened
([GDK-1684]). The report knows who "I" am on the built-in tracker, where a
write is attributed to the actor slug and not to a credential: the resume row
counted any agent's writes as the reader's own, and the footer now says which
identifier decided ([GDK-1427]) — the demo and the e2e serve derive that
identity from the mirror they serve, so the cell is no longer a dash there
([GDK-1729]). Reopens are shown by whether the origin can answer at all
rather than by the count: an origin that supplies no changelog leaves
`reopen_count` at zero forever, and "reopened 0" read as "this team has no
regressions" when nobody could tell, so the clause is dropped and the reason
said ([GDK-1690]). The mismatch row stopped firing on ordinary Korean —
"검토 완료 후 진행" schedules the work and does not claim it is done, and a
done word inside a clause about work still to come is no longer counted
([GDK-1428]); the surprises the CLI prints name the work rather than only its
key ([GDK-1746]). The demo mirror's issues carry priority ids, so a surface
that keys by id — the phone's priority sheet, the web filter's id path — can
be checked against the one mirror everybody opens instead of a mock
([GDK-1492], [GDK-1524]). An optional capability no longer disappears when the actor
trailer is on: the wrapper that signs an agent's comments embeds the writer,
and an embedded interface promotes only what it declares, so versions, issue
links, create-field catalogs, media refs and sprints were invisible to the
capability check, which now looks through the wrapper ([GDK-1655]).
`gadak comment edit <KEY> <ID> -m "…"` replaces a comment's body and
`gadak comment rm <KEY> <ID> --yes` removes it, on Jira, Linear and the
built-in tracker; the id is whatever a read handed you (`gadak sql` prints
`jira:91653`, `gadak issue` prints `91653`, both accepted), an edit sends
what a post sends, and the actor trailer survives it without doubling
([GDK-1647]). The write verbs meet the reader where the reads left them:
`gadak create` resolves its project and issue type through one catalog path
on every origin — a lone project needs no `--project`, a parent key names
its own project, a localized type name answers to its English spelling, and
an ambiguous type asks the mirror before refusing — and `create --json`
names the default that chose each field. Every write verb takes `--dry-run`:
one JSON line carrying the ids the resolutions found — a transition that
would change nothing says so, and a link that does not exist still refuses —
and nothing reaches the origin. A `link` or `unlink` refusal quotes the
phrase as the issue displays it, a query that trips over an unqualified
column in a `json_each` join is told both tables it could have meant, and
the one-line help for reading an issue says its comments and history are
already in what it prints. On the built-in tracker, attachment bytes live in a directory
beside the database, one content-addressed file each, streaming both ways
(`gadak attach`, `gadak attach get`, and the app, where `Range` makes video
seeking work); the upload cap is `gadak config set attachmentMaxMB <n>`,
default 1 GiB, up from a hard-coded 32 MiB; opening a built-in workspace
with this build moves the bytes out once, keeps `issuetap.db.pre-v2.bak`
beside it, and is safe with a serve up ([GDK-1617]). `gadak attach` uploads carry the type
their filename says instead of `multipart.CreateFormFile`'s
`application/octet-stream`, so an origin that keeps what it is told no longer stores a PNG and an MP4
under one generic type and a screenshot is a thumbnail again, and
`gadak backup` is a `.tar` of the database and the bytes that refuses to
write one with attachments missing — `docs/runbooks/backup-restore.md` has
the restore ([GDK-1277]). gadak no longer asks GitHub once a day whether a
newer release exists: no background check, no `updateCheck` setting, no
sidebar banner; outbound destinations go from six to five and
`docs/PROMISES.md` is eleven claims rather than twelve; upgrading is
`brew upgrade`, a new dmg, a newer zip, and Settings → Sync still shows the
command ([GDK-1626]). `gadak mcp install claude-desktop` registers with
Claude Desktop by merging a `gadak` entry into `claude_desktop_config.json`
(macOS, Windows and Linux paths; other entries kept byte for byte; an
unparsable file refused; `--dry-run` prints the write), and
`gadak mcp install claude` says what it is — the command every front door
had taught as "for hosts without a shell (Claude Desktop):
`gadak mcp install claude`" runs Claude *Code*'s `claude mcp add`, which
Desktop never reads, so a Desktop user registered nothing or was told
`claude` was not on `PATH`; the
Integrations card that probed the `claude` CLI is two cards, one per host, and the wizard, `gadak init`'s
next-steps, the help, the README and the site name the right command
([GDK-1633]). The front door says what gadak is before it says how fast it
is: the landing heading in every language is a job, "Query your Jira backlog
with SQL.", the canonical `GROUP BY` runs in the browser under it, and the
trust section became the facts a reader needs before handing over a token —
Jira Cloud only, `--projects`/`--spaces` with the wiki off until named, one
SQLite file that needs a first full sync (10.6 minutes on the benchmark
site) and then trails Jira by one interval (60 s by default, with an hourly
reconcile), where the token lives, that a rejected write fails rather than
queues, which four reads still ask Jira, and that an agent forwards what it
reads to its model; "8 API pages" is labelled as this measurement's number,
the attribution claim shrank to what the code does, and the READMEs follow
the same order under `docs/project/FACT_LEDGER.md` §16 ([GDK-1601],
[GDK-1622]). Then that first sync got cheaper and stopped being waited on. A
pass ends with one stderr line of requests by kind and the wall time on
each, and `gadak api --headers` prints every response header ([GDK-1672]);
the Confluence pass fetches page bodies, comments and versions through a
bounded pool (`gadak sync --concurrency`, default 4, at most 8; 1 is the old
serial pass) while one writer commits in listing order, a 429 halves the
width, and the summary names `concurrency=<width>/<min>` ([GDK-1673]); the
documented first run is `gadak init && gadak serve`, the window opens while
the sync fills the mirror newest-first under a band that reads
`Recent issues first · 1,200 / 3,514 · wiki next`, `gadak status --json` carries
`first_sync`, every read verb prints one `first sync in progress` line on
stderr while it is true, and the skill says to work with what has landed
([GDK-1677]). And a dev build stopped deciding for the installed release: a
`0.0.0-dev` binary refuses to migrate a mirror forward, naming both versions
and both ways out (`GADAK_DEV_MIGRATE=1`, or a copy of the profile), the
backlog snapshot script syncs a `sqlite3 .backup` copy under a scratch home
([GDK-1687]), and with `GADAK_HOME` unset a dev build lives in `~/.gadak-dev`
while the release keeps `~/.gadak`, which `gadak doctor` prints as `home`
with the reason ([GDK-1697]). Before the version is cut, the audit
now starts from a census rather than a reading, and this cycle it found
things the release would otherwise have shipped: what landed since v0.21.0
reached the skill and the MCP tools, so an agent can see a self-hosted Jira
workspace, edit and delete its own comments, and start a first sync
([GDK-1700]); twenty-four English strings that had escaped the catalog on the
phone screens and two web panels became keys in all three languages, and the
phone's dates stopped being an English month table ([GDK-1704]); ten places
where the docs and the site disagreed with the code were fixed at the copy
([GDK-1705]); the retro table takes the width it needs ([GDK-1706]); and the
CI run got cheaper — the static-analysis gate skips a push that touches no
Go, the browser shards are dealt by measured seconds instead of file order,
and five assertions that never needed a browser moved to the unit suite
([GDK-1702], [GDK-1698]). The service installer now carries serve flags
given after -- into the unit's ExecStart and validates them at install time
with serve's own parser — an address serve refuses would otherwise install
fine and crash-loop under KeepAlive ([GDK-1267]). The public-backlog export
now refuses a kept description that carries a credential-shaped string, on
the same patterns the shell scan greps for, instead of publishing it and
failing later in the pipeline ([GDK-1260]). `reopen_count` now counts the reopen most
teams actually fire: in a workflow whose resolved statuses sit in the
in-progress category (QA testing → Reopened), a done→new-only rule read 59%
of real reopens as 0. The rule keys on the status category the transition
leaves, every stored row is recomputed once on upgrade, and docs/DERIVE.md
is where it lives. `gadak init` under a throwaway `GADAK_HOME` no longer
rewrites the real skills directory — the automatic sync is skipped with one
line saying why, while `gadak skill install` still writes where it is
pointed. `gadak doctor` shows the site host (the token stays masked) and
calls the documented placeholders a configuration error rather than a
healthy workspace. And a read that names an issue this workspace never
mirrors gets one stderr line naming the workspace that answered and what it
holds, where an empty answer used to be indistinguishable from no data. The list reads at every width it is given: a
label chip that cannot show six characters folds into the `+N` badge whose
tooltip names every label, and the deploy column, where no label of the set
fits at all, keeps its dot and drops the word rather than cutting it
mid-glyph ([GDK-1744]). A count is grouped for the locale wherever it
appears, because `t()` formats a numeric parameter rather than each of
eighteen call sites remembering to ([GDK-1560]). The dashboard frame carries
the app's background rather than white, so opening the tab no longer flashes
([GDK-1598]). An exact issue key in the palette resolves to that row instead
of an empty search line ([GDK-1255]), and onboarding calls the local copy a
cache throughout rather than switching between two words for it inside one
dialog ([GDK-1323]). The phone's controls live in the catalog rather than in its
markup, and a test that walks both the templates and the modules is what keeps
them there ([GDK-1150]). A server that refuses now says which refusal it was —
an origin check that does not know this phone, a rejected pairing, an offer
carrying no serve scope ([GDK-1121]) — and a copy that fails raises a toast
instead of nothing at all ([GDK-1504]). What you read on the phone lands in the
same visit history the window writes ([GDK-1538]). An empty `dev_links` table used to mean two very
different things — no pull requests, or a workspace that never asks — and
nothing said which. `gadak doctor` and `gadak status --json` now report
whether this workspace mirrors the development panel at all, off the same
flag sync consults, so the two can never disagree ([GDK-1496]). A folded
group resolves the same way from the CLI, a REST write and `gadak claim`,
because the mirror-usage tiebreak has one owner instead of three
([GDK-1521]). And a board on a self-hosted Jira knows its project: Server
answers the board location differently from Cloud, so the key is backfilled
per board rather than left blank ([GDK-1665]). `migrate` stopped swallowing two failures: a dev-links
count that errored reported success anyway, and a Linear assignee that could
not be resolved vanished from the report rather than being named in it — the
run still continues, it just says who it could not place ([GDK-1318]). The
pairing dialog decides a loopback refusal from the error's type rather than
by matching the sentence it happens to carry ([GDK-1317]). And three
grammars became one: the `gadak://` pointer had a parser in the CLI and
another in the server ([GDK-1316]), the Cloud category fold had a hand-written
copy beside it ([GDK-1315]), and the page-id case policy differed between
the store and sync ([GDK-1104]). Each has a single owner now, held there by a
gate that names any second copy.

The CLI itself says when its skill file is behind: a read verb prints one
stderr line when the installed copy differs from this build's, so an agent
learns it without ever deciding to run doctor ([GDK-493]). "Mine" is in the
database: `local.me` holds the workspace's own identity and two views,
`my_open` and `handed_off`, join it to the issues, so an agent asks what is
its own in plain SQL with no `:me` to substitute ([GDK-1438]). A linked
issue's line reads as the link type's own sentence — the backend renders it
from the link catalog, and the web no longer shows the wire token for a
direction spelling it did not name ([GDK-1215]). A Jira Server workspace is
refused in its own words: the wiki says Confluence Server is not implemented
rather than asking for an email, and a dev-link write names the deployment
it actually has ([GDK-1663]). Four timestamp parsers with four layout tables
became one, so a Jira-stamped instant no longer reads as "no date" in the
calendar ([GDK-1130]); and two `$effect` blocks that wrote state — the
new-issue dialog's defaults and the document panel's write-through overlay —
are derivations now ([GDK-1133]).

The session strip's boundary has one seat: the `X-Gadak-Session-Boundary`
header, on bootstrap and delta alike — the body field that overlapped it for
one release is gone ([GDK-1548]). `gadak migrate` carries the source's
locale, so a Korean built-in workspace migrates into one that still shows
Korean status and type chips under its Korean prose, and the report says
which locale came along ([GDK-1561]). The phone's wire types are picks of
the desk's, so a field's optionality cannot drift between the two clients
again ([GDK-1132]). A sync against a paired serve too old to know sprints
says so — the Agile API's 501 is the origin's age, not a failed sync, and
the log names the upgrade ([GDK-1691]). And a sprint has a burn-up:
`GET sprints/{id}/burnup/` and `gadak sprint show <id>` print the same daily
scope, started and done series, reconstructed from the changelog and
withheld on an origin that keeps none ([GDK-1710]); the chart is next.

A dashboard list no longer ends in a row cut through its glyphs: the wall's
height is the panel's, so the label-ratio example and the skill's copyable
snippet give the list its own scroll region floored to whole rows, with a
`+N more` line shown only while rows are hidden, and an e2e gate measures
the cut-row count at both window sizes ([GDK-1745]).

A comment that quotes CSS no longer asks the origin who `@media` is: at-rule
names, code spans and package paths never become mention sites, and the one
warning left says the comment was saved as typed ([GDK-1125], [GDK-1544]);
an `@email` already becomes a real mention node, which is now pinned by a
test rather than by a comment that said "add it when someone asks"
([GDK-21]).

The release audit's census is five scripts under `tools/audit/`, each
printing its own source commands, with a contract test that doc-checks runs
([GDK-1707]); and the semantic-axis audit — is any other mapping mirrored
the way link direction was — is a document with two candidate defects and
four notes ([GDK-1206]).

The phone remembers the last issue it opened, so the terminal session sheet
opens with that key filled in ([GDK-1527]), and the bottom-sheet inset is
measured again — the formula is one function, the stylesheet is pinned to
it, and the viewport walk opens a sheet that really opens ([GDK-911]).

The demo fixture has one content original and one artifact —
`examples/demo-source.db` builds `examples/demo.db` with a pinned clock, and
`make demo-fixture-check` fails when two builds differ or the committed file
is not what the source builds ([GDK-1751]); the fixture's media nodes carry
`alt` the way real Jira Cloud's do (measured: 24 of 24 on a live site), so
Dana's comment attachments render as images instead of chips, and a gate
keeps every media node joined to its attachment ([GDK-1517]); the sync
fixture localizes priorities and issue types the way it already localized
statuses, so keying on a display name goes red on all three axes ([GDK-47]).

The e2e suite serves a Linear-shaped mirror beside the Jira one:
`examples/demo-linear.db` is built by replaying a deterministic dataset
through the production Linear sync against a loopback stub, so the mapping
in the file is the connector's own, and `e2e/linear.spec.ts` renders origin
deep links, link labels in both directions, the write refusal and the
freshness chip against it on the suite's second port ([GDK-1298]).

Wiki page attachments reach the mirror: the sync lists a page's attachments
beside its body, they live in the same table as an issue's under the page's
own key, the page detail carries them the way the issue detail does — one
builder owns both wire shapes, and a gate keeps an empty list `[]` rather
than `null` — and `pages/<key>/attachments/<id>/content/` streams the bytes
through the same handler as an issue's, so a page's media nodes render as
images on the desk and the phone instead of unresolved chips ([GDK-1750],
[GDK-1541]). A built-in origin that has not grown the attachment routes is
measured once per sync and skipped with one summary line, not a failure.

The reopen verdict has one owner. `reopen_count` in SQL counted an
in-progress → new move as a reopen and the feed painted it red, while the
history timeline in the web left the same row grey, because the web kept a
done-only rule of its own. The server now stamps each history row with
`is_reopen` from the same function that derives the column, the timeline reads
that field and nothing else, and a source-level gate keeps a second spelling of
the predicate from coming back ([GDK-1753]).

A visit now stamps what it saw. `local.visits.seen_updated_at` is the issue's
`updated_at` at the moment you opened it, so "what changed since I last
looked" is a local join over the mirror rather than a sync, and the recipe for
it sits in the Mine section of `docs/RECIPES.md`. Reads from the CLI are an
agent looking, not you returning, and stay out of the answer ([GDK-1451]).

The demo fixture's wiki pages ride the same time window as its issues. The
snapshot spread used to skip pages, so the fixture shipped two pages updated
before they were created and twenty page comments outside their page's span;
pages and their comments are now placed on the same window, and a gate over
the committed fixture keeps created ≤ updated and every comment inside its
page's span ([GDK-1731]).

`gadak migrate` no longer holds the archive in memory. The seed document
used to inline every attachment as base64 and then marshal the whole thing a
second time, so a workspace big enough to be worth migrating was the one that
could not be; attachment bytes now stream from the origin straight into the
seed file as the writer reaches them, the size cap and the missing-file
accounting are unchanged, and a test pins that memory does not grow with the
archive ([GDK-1618]).

There is a way out of the built-in tracker that deletes nothing. Until
now the only path from a locally originated backlog to a real Jira site was
`init --replace-local`, whose own help says converting drops those issues.
`gadak --workspace <jira workspace> migrate --from <workspace> --to jira
--project KEY` carries them out instead, the way `--to linear` already did
for Linear: issues with their descriptions, types, priorities, labels,
parents, links, comments and attachment bytes land in the project, the status
is set by one transition into the same category (never by name), and a re-run
is idempotent through the same footer. Change history, wiki pages,
authorship and assignees stay behind, and the report says so ([GDK-378]).

Three sync corrections. A link is stored on both ends, and an incremental
window that carried only one of them used to leave the far end's row behind
forever, so `open_blockers` stayed high and `gadak ready` hid an issue nothing
blocks; the store now deletes the counterpart of every link an issue just lost
and recomputes the touched rows, `gadak doctor` counts one-sided links, and a
Linear relation list the origin paged deletes nothing ([GDK-1507]). A full
pass rewrites every row again, so a derived column whose rule changed since
the issue's last edit is recomputed instead of waiting for someone to touch
the issue, without the reported changed count inflating to the fetched count
([GDK-1457]). The divergence probe reads Jira Cloud's approximate count as an
approximation, agreeing inside one percent of the origin's own number instead
of escalating to a full key scan every tick on the sites where that scan
costs the most ([GDK-1490]).

Search finds labels and word forms. An issue whose only mention of
`payments` was its label was invisible to `gadak search payments`, and
`uploads` did not find `upload`; `items_fts` now carries a labels column,
ranked between title and body, and stems English with porter while the CJK
two-rune contract is unchanged, so `retries` reaches every `retry` and a label
alone is enough to be found. The mirror schema moves up one version and the
index is rebuilt on the first open. Every writer of that index in the tree,
including the fixture translator no Go test compiles, is now checked by a
census gate that reads the canonical column list out of the schema
([GDK-1021]).

Three things on the phone side. The REST responses the phone decodes are
now a golden the server test writes and a phone test consumes, so a removed
field or a changed type is red on both ends ([GDK-803]). The pairing token
is asserted absent from the server's log, bodies and headers across nine gate
branches and from every console call the phone makes, with a source rule on
each side that follows the argument list across line breaks ([GDK-804]). And
`gadak://` links open an issue on the phone: the Go parser emits its grammar
as a vector table the phone parser replays, a cold-launch link is held until
the app is paired, another app's scheme passes in silence, and the scheme
carries no verb, so the worst a hostile link does is show the wrong issue
([GDK-873]).

Three duplicate implementations collapse to one owner each. A byte count is
rendered by one function everywhere, so `gadak snapshot` stops printing
`1024.0 MB` for a gigabyte and a negative size stops leaking through
([GDK-927]). The single-statement SELECT/WITH check the MCP query tool and the
saved group query both make is one walk now, with each surface keeping its own
refusal sentence because they address different readers ([GDK-928]). And the
migrate package's own sorted-keys helper gives way to the standard library
([GDK-1320]).

Four lists that described the code stop being lists. The web asked
`hasServerVerb(verb)` before rendering a server-backed entry point, but the
answer never depended on the verb and never had since the function was born,
so the verb table, its type and its diagnostic report are gone and the
question is `hasServer()` ([GDK-1482]). The grouping and sorting menus
derived their options from hand-kept arrays that could silently omit an axis;
they now come from a map the compiler checks, so a new axis without a label
is a build error ([GDK-832]). A 29-name field list nothing read, which had
already drifted from the catalog it claimed to describe, is replaced by a
test that asks the two real sources instead ([GDK-921]), and two interfaces
left over from the removed push stack are gone ([GDK-923], [GDK-1483]).

Three places the keyboard could not reach. A comment's Reply button appeared
on hover alone, so tabbing to it moved nothing into view ([GDK-734]).
Favourite rows could be reordered by dragging but not by typing; Alt+↑↓ now
steps a row, and the gesture, its no-wrap edges and its step arithmetic are
one owner shared with the sidebar sections, so the second list cannot drift
into a different key ([GDK-733]). And the file paths in Settings broke
mid-word rather than at their separators ([GDK-1093]).

The scheme test asks the artifact. `TestBundleRegistersTheScheme` read
`build-app.sh`, and LaunchServices never reads `build-app.sh` — it reads
`Contents/Info.plist` out of the packed app, so a plist the script no longer
produces was invisible. A new test parses that plist inside the bundle and
the release workflow mounts the dmg to run it; the script check keeps its
place under a name that says what it actually measures ([GDK-919]).

The settings tabs are a tablist. Nine plain buttons told a screen reader
nothing about which one was showing — `aria-selected` read null on every tab
— and leaving the header cost nine Tab presses. They now carry the role's
whole contract: one Tab stop, Left/Right and Home/End between tabs, and the
panel names the tab that filled it. Every close control draws its × with the
icon component instead of one of four byte-identical hand-rolled SVGs, and
names itself the same way twice; the rule those follow is written down and
measured rather than decided per file ([GDK-138]). One flag stopped being
state: whether the Confluence turn-on button reads armed is a function of the
click and of the source being off, so an effect that cleared it when a space
arrived no longer decides the label by when it ran ([GDK-1134]).

The palette knows where the main column can go. Its §3 promise — every action
the app can do is registered, and that is auditable — was auditable only by
reading. The destinations now have one owner, a union and a runtime array
bound to each other by the compiler, and a test asks that list and the
palette's own rows whether they agree; an exception needs a written reason,
and a reason that has gone stale is a failure too. Returning to the issue
list was the hole it found ([GDK-137]). Copying an issue's link and marking
the feed read joined the registry, each moving out of the component that
privately owned it, and saving the current view is one row rather than a
policy repeated per surface ([GDK-732]). Deleting a team view stayed out on
purpose, and the principles file now says why a destructive action may be
absent: the list is two clicks and an owner check away from a shared, final
deletion, and a palette row collapses all three into one Enter on a view the
reader cannot see.

Three e2e cases stopped re-proving a unit. What an integration's output means
— a run that stops before its status, an `exit=` line in the middle of the
log, an exit 0 the detection still contradicts — is a parser's verdict, and
`web/src/lib/integrations.test.ts` already holds each one; running them again
in a browser was cost, not coverage. The page-comment shortcut chip is the
same: one unit already asserts it has a single owner and that both composers
render it. What keeps them down there is a lint — an e2e title that repeats a
unit's title exactly is red — plus a paragraph in `e2e/README.md` giving the
test the lint cannot apply: take the browser out of the sentence, and see
whether anything is left ([GDK-720]).

Two totals of different scope stopped sitting side by side. The Documents
header read "Documents 0" on an account with 71 wiki pages: the badge counted
the tab it was on, the library held the rest, and nothing said the number had
been narrowed. The denominator beside a screen's name is now the library on
every tab, one owner, and a narrowed number is written as a fraction of it —
the rule is in the principles file rather than re-decided per screen
([GDK-1092]). The empty state stopped saying its own title twice: the hint
line is the only room a screen has for the next move, and on a search with no
matches that move is the search the reader has not run yet — Enter, over
bodies and comments. A test reads the screens rather than a hand-kept list, so
a pair written next year is measured the day it is written ([GDK-1091]). And
the two panels that laid out their own "not found" block by hand use the same
empty state as everything else, so the message sits where every other one
does.

A piece of chrome now looks like what it is. In the dark palette the seam
between the terminal and the list measured 1.28:1 against the panel behind
it, so two windows read as one body; the fix is not a brighter token but a
second word — a boundary between two regions is `border-strong` (2.11:1 in
dark, 1.77 in light), a divider inside one surface stays `border-subtle`, and
a test names the six seams and checks the two tokens stay two words in all
four palettes ([GDK-1093]). The epic progress bar drew its empty half in the
elevated ground it sat on, 1.00:1 — an underline, not a proportion; one
`MeterBar` now owns track and fill, and the QA-impact bar and the priority
menu's distribution use it too. A status dot came in three sizes depending on
the file; `StatusDot` decides colour and size, with the list's lead dot the
one documented exception. 'Unassigned' (a value that is absent) and 'Add a
label' (an action) were byte-identical muted italics; the two costumes are
constants that share no class, and nothing may spell either by hand. And the
focus trap learned what the browser already knew — a roving tabindex parks
`-1` on real buttons, and the trap stopped at them anyway ([GDK-142]).

The Go tree lost some code that lived in the wrong room. Picking a transition
by name is a use case, not an HTTP concern, so it left the Jira client for
`internal/transition`, and the origin package no longer imports the client to
ask it ([GDK-688]); seven identity wrappers on the Jira writer went with it,
and a zero-value workspace registry opens and binds its origin instead of
panicking on a nil map ([GDK-689]). `gadak fields` prints the fragment you
would paste into the config — a `FieldSpec`, not the alias map `--apply`
would then discard — and never proposes a field the classifier refuses; the
`--json` key is byte-identical ([GDK-718]). `doctor` and the MCP status tool
counted the shared comments table as one figure, which disagreed with the
settings runtime on every mirror with wiki comments; both now report
`issue_comments` and `page_comments` ([GDK-1113]). A label holding a comma is
warned about on create and edit, flag or batch, from one helper ([GDK-1245]);
the "origin too old" hint keys on a typed 501 instead of a substring
([GDK-1319]); and the refusal a Jira workspace gives without a site has a
test ([GDK-1332]).

The server stopped paying per request for things it could know once. The
Jira issue pass fetched each issue's comment and changelog overflow one at a
time; it now rides the same ordered fetch pool the Confluence pass already
used, up to eight wide, with the 429 back-off wired to each fetch and one
summary line saying how wide it actually ran ([GDK-1674]). `LastSessionEnd`
walked three hundred thousand visits on every bootstrap and delta poll —
113 ms a call; pinned to its index it is 1.3 ms, and an `EXPLAIN QUERY PLAN`
test keeps the planner from wandering back ([GDK-1547]). The 700 ms probe
budget lived in two files that had to agree by hand; `origin.ProbeTimeout`
owns it and a test reads both sources ([GDK-1004]). The store learned to say
how large the mirror's `-wal` sidecar is ([GDK-307]) and whether one priority
holds seventy percent of the open work ([GDK-1413]) — helpers with their
thresholds tested, waiting for `doctor` to print them. Two questions were
answered by measurement rather than code: `mmap_size` buys about seven
percent on a warm cache, so the DSN stays as it is ([GDK-978]); the cycle-time
p85 costs 60 ms at fifty thousand closed issues, recorded as the number that
would reopen the question ([GDK-1429]). And the teardown window the audit
asked about no longer exists — the advertise file it worried about went with
GDK-936 ([GDK-954]).

The gates got cheaper and stopped drifting apart. `tools/doc-checks.sh`
starts its six delegated gates together the moment they are unblocked and
replays each one's output in its original place, and an exit trap prints
where the time went — 74 s to 42 s on a quiet machine ([GDK-1227],
[GDK-1488]); `complexity.sh` no longer goes red on a deleted file that is
still in the index ([GDK-1759]). Four regexes that lived in two places each
and had to agree by hand — the tenant-host allowlist, the home-path pattern,
the profile-name grammar the deep link mirrors, the ui-token family the Go
and web sides both parse — are read from both live sources by
`tools/mirror-pins.sh`, and a divergence names both positions ([GDK-1107],
[GDK-1105], [GDK-1108]); `scan-internal` keeps its two-pattern tree scope on
purpose, with the measurement that shows why written down ([GDK-1110]). Four
probes that rounds had built by hand are tools now: `export-census.sh` lists
exported Go identifiers nothing outside their package uses — and its seed
script had been writing its index inside the tree it scanned, so every run
after the first reported a hollow zero; the honest count is thirty-one
([GDK-1485]); `skill-overwrite-probe.sh` plants a stale skill in a throwaway
home and says PRESERVED or OVERWRITTEN ([GDK-1546]); `adf-render.mjs` prints
the rendered HTML for one issue's description and comments ([GDK-1518]); and
`scope-sheet.mjs` prints the phone's scope sheet from the demo snapshot
([GDK-1554]).

The web tree lost its last document-wide lookups and its last effects that
read what they write. The global chords found their targets with
`document.querySelector` at dispatch time; components now register the
element themselves, an unmounted target is a null the dispatch already
treats as "not spent", and the GDK-645 sweep that had carved keymap out as an
exception no longer has one ([GDK-693]); the scope picker's document-level
click closes through the shared outside-click action, and a sweep forbids
the next `svelte:document onclick` ([GDK-630]). The viewport regime had one
`$state` in the app shell and another in the detail panel; one module owns
it, and a sweep keeps it that way ([GDK-696]). Selecting an issue records
the visit and marks it read where the selection is written, instead of two
untracked effects watching the key and writing back to the store they read
([GDK-941]); the docs view consumes a focus-author request through a nonce
rather than nulling the field it reads ([GDK-942]); and BulkBar's five batch
runners share one loop ([GDK-698]). Smaller: rows in the issue list are
tab-reachable on the same terms as document and history rows ([GDK-829]),
the onboarding source picker is a real radiogroup — one tab stop, arrows move
selection and focus, and its first e2e run caught the arrow moving from the
wrong anchor ([GDK-1324]); a cross-workspace reference that cannot be
resolved is a plain span, and one that can is an `<a>` ([GDK-1326]); the
empty-state hint has room for a sentence ([GDK-1732]); the right panel says
whether it is open in the DOM, since it stays mounted when closed
([GDK-1187]); the boot-time token mirror in `index.html` carries the fonts
axis the runtime does, with a parity test ([GDK-1101]); the locale tag has
one owner ([GDK-1455]); and the history store's boundary is documented as
the write path it is rather than moved into its one reader, because it has
four callers ([GDK-1135]).

The test pyramid moved some weight down. A held e2e port is named before the
build starts — port, pid, worktree, and the `pkill` that frees it — and the
serve stamp carries the pid, so a suite that inherits another worktree's
server is refused by name rather than run against the wrong code
([GDK-1757]); three fixed waits became signals (a database delete that
rejects on `onblocked`, a focus poll counted through a wrapped `fetch`, a
frozen digest instead of a spawned process) ([GDK-723]); the bench smoke
seeds fifty rows instead of a thousand and the static export test carries
its own one-row mirror instead of the whole demo ([GDK-724]). Six dialogs
times two viewports in the browser became a registry unit test plus two
representative shapes ([GDK-725]); the router's coalescing window is ten
fake-timer cases in vitest with one smoke per rule family left in Playwright
([GDK-1327]); a component renders under `svelte/server` with no new
dependency, as a pilot whose limits are written in its header ([GDK-1328]);
the desktop chrome spec asserts the relation to the traffic lights rather
than exact pixels ([GDK-1147]); and the integrations tests poll to a deadline
with the gate released in cleanup, so a sibling can no longer inherit a leak
([GDK-1502]). Workers stay at one; a census gate now says which specs mutate
and the design for per-worker isolation is written down with the measured
worst files ([GDK-1758]).

The demo fixture exercises what the code maps. `examples/demo.db` had no
`dev_links` row, so the PR chip's open/merged/declined mapping had never run
against the fixture; a `tools/demo-enrich` seeder in the `make demo-fixture`
pipeline now plants one of each, a clone relation and a wiki-URL comment that
becomes a page reference, a store test pins the fixture's content and an e2e
spec shows the chips ([GDK-1755], [GDK-114]). `tools/seed-demo` resolves
symbolic references — an issue's `ref:` and a document's `{{ref:…}}` — after
creation, so authored documents can join issues without hard-coded keys, and
an unresolved reference is an error ([GDK-45]); the Confluence sync test
asserts that `version.by` becomes `author_id` ([GDK-25]). The demo recording
specs have a rot gate, `npm run test:e2e:demo`, outside the CI set; its first
run caught two specs that had already rotted — one stale against the retro
folding UI, one with a hard-coded port ([GDK-1349]). The FTS rebuild on first
open is measured at 73 ms and is the designed repair path; making the
committed file carry `contentless_delete=1` would break the snapshot's
Datasette Lite contract, so that question is left open with the numbers
([GDK-1756]).

The CLI and the skill surface lost some duplicate vocabulary. The skill's
status word and card label had two spellings, one in `doctor` and one in the
integrations table; `internal/skillinstall` owns both now ([GDK-1534]). The
app's integrations tab always refused a conflicting install because it had
no `--force`; Replace is asked for twice — an armed button on the second
tap — and the argv is built in one place for the app and the CLI
([GDK-1535]). A dev build's first `init` printed the skill refusal twice, from
the daily auto-sync hook and from the install itself; one line ([GDK-1545]).
The skill receipt survives a CRLF checkout and a symlinked destination, so
a file that is ours by content is recognised as ours ([GDK-1520]). The
terminal font family was resolved in two renderers; `protocol.ts` owns it,
with the phone renderer delegating and a pin on `app.css` ([GDK-1528]). The
three sync-status string families — sidebar, freshness chip, sync — are one
`sync.*` family with byte-identical text, so the e2e strings did not move
([GDK-136]); the remaining toast punctuation drift was aligned to the
current rule rather than the one the issue was written under, which GDK-1588
had since reversed ([GDK-1242]). Mentions no longer read `@3×` or `@media`
as a person ([GDK-976]), and `source: "jira"` on a built-in-tracker row is
documented as the stored slug it is, since the write pickers key on it
([GDK-487]).

The documentation gained gates where it had habits. The CHANGELOG reference
tails — 659 definitions per edition, appended by hand in the order work
landed — are generated now: `tools/changelog-tails.py` rewrites them in
numeric order from the keys the body cites, and a check refuses a tail that
drifted ([GDK-670]). "Connected" and "standalone" left the two sentences
where they still stood in for a category, and a check keeps them out of
reader-facing prose while leaving wire values and dated records alone
([GDK-1288]). The two meanings of *watch* — Jira's site-side watchers, which
sync never copies, and the local follows that `export` carries — are told
apart in the skill and the agent-access page ([GDK-524]). `CONTRIBUTING`
no longer opens with five documents to read before a first pull request;
nothing does, and the links are on-need ([GDK-1003]). Three more checks
watch the front door: every README link resolves and a path-shaped label is
the path it points at, the site's video captions name the locale they
serve ([GDK-1625]), and the three README editions may not share both a
paragraph count and a heading sequence — measured 49/43/81 paragraphs today,
with the em-dash density capped at the frontier it sits on ([GDK-1604]).

`doctor` says what the store already knew, and a paired serve says which
version it is. The wal-sidecar size, the priority concentration and the last
session boundary reach `doctor` — the sidecar is measured before the mirror
is opened, since opening it checkpoints the file away ([GDK-307], [GDK-1413],
[GDK-1549]); the local schema skew has one owner in the store, `doctor`
carries the remediation, and the stderr line fires once per path
([GDK-596]). A paired workspace records the serve's version at pairing
time, every serve response carries `X-Gadak-Version`, `status` and `doctor`
name the skew, and a 501 from an older home is folded into an upgrade hint
at the one choke point every paired round trip passes ([GDK-1273]). A
built-in workspace can export its seed: `gadak workspaces export` and
`GET /api/v1/origin/export` share one gate that refuses a connected or
remote origin by name ([GDK-768]). The retro test stopped reading the wall
clock — anchored mid-week, it is green under `TZ=Asia/Tokyo` and `TZ=UTC`
alike ([GDK-1760]); the probe budget's second literal is gone ([GDK-1004]);
`IssueLite`'s field names are checked by reflection ([GDK-722]); and the
integrations probe reads exit 0 through a runner seam ([GDK-723]).

The phone's network boundary moved into Rust. The websocket dial used to be
a JavaScript allowlist over a process-wide `websocket:default` grant; the
grant and the plugin are gone, and `shell.rs` owns the URL, the scope
verdict, the dial and the bearer, with the webview passing only a session id
([GDK-897]). The search plate shows what the snapshot already carried — a
due date on the detail meta line and the last five issues you looked at
([GDK-875]) — and a document row whose title already shows the query no
longer repeats that title as its snippet, by the same rule the web uses
([GDK-890]). A serve without a credential is read as read-only from the
bootstrap, not after the first refusal ([GDK-952]); the description editor's
Save wears the accent only when armed ([GDK-1525]). The viewport gate reads
a median row height instead of the first row, and the search-results plate
has a density floor of its own ([GDK-1550]); the shell spec waits on the PTY
echo through CDP frames rather than a page-side poll that starved the socket
([GDK-1552]). Two audit items were already closed on HEAD — per-session
scrollback ([GDK-1529]) and separate slots for a session's name and its
bound key ([GDK-1530]) — and the document row keeps one line, since none of
the fixture's 71 titles truncates ([GDK-1551]).

Five pieces of infrastructure stopped trusting luck. The hard-coded-host
scan now reads `e2e/demo/*.spec.ts` too — the demo rot gate had just caught
a port literal there that the scan should have ([GDK-1761]). The desktop's
CSP is asserted as one exact string through the real mux, a doc check
refuses a fenced dashboard example that compares a display name, and a pin
keeps that check's grammar identical to the runtime warning's; the other
three dashboard gates were already on HEAD ([GDK-783]). The cask publish
step waits for the tap formula to be at least the cask's version before it
pushes, and logs the mirror schema level each tag ships ([GDK-506]). The
Linux desktop job caches its GTK `.deb`s and apt lists, so an Ubuntu mirror
outage replays the cache instead of failing the run ([GDK-234]). And the
Windows executable declares PerMonitorV2 DPI awareness through a committed
manifest and `.syso` pair, with a regenerator and a byte check in the pack
script ([GDK-1407]).

Thirty-three Go identifiers that nothing outside their package ever used
went lowercase, and three web modules dropped exports nothing imports; the
census the previous round promoted is what named them, and it now reports
eleven where it reported thirty-seven — the rest are either a migration
another issue has already promised or a symbol whose real fate is deletion,
which this round did not decide ([GDK-1141]). The simplification bucket was
re-measured item by item: the CI-status test is wired after all, the
calendar instrumentation has consumers, the `e2e` class assertions were
already gone, and one hand-run diagnostic script is left to the user
([GDK-1232]).

The second audit pass before 0.22 closed what its first pass had only
measured. A paired serve that answers 501 to a route it predates is now read
as an answer, not a transient: the retry ladder returns on the first attempt
instead of walking every rung against a home that will say the same thing
([GDK-1762]). `gadak workspace export` writes version 2 and carries the
visit and search history the invariants had excused it from, and import
accepts both versions ([GDK-1769]). Under the hood, six compound commands
route their verbs through one dispatcher where each used to keep a switch of
its own, `agent.go` is five files by topic with every declaration kept, and
`doctor` collects its report through nine named probes; `init` gives its
credential resolution a function of its own ([GDK-1778], [GDK-1771],
[GDK-1775], [GDK-1772]).

Under 900 pixels there is one narrow regime. The sidebar's narrow width was
redeclared in five places under a 760-pixel media query, so an 800-pixel
window kept a 272-pixel sidebar and squeezed the list into what was left;
the narrow value now rides the inline token install, `app.css` defines
`--layout-sidebar` nowhere, the step sits at 899, and the terminal overlay
shares the same boundary ([GDK-1369], [GDK-1091]). The shell's height falls
through `100vh`, `100dvh`, `100svh`, so an in-app tab bar no longer eats the
bottom row ([GDK-54]). Section labels — twenty-one sites spelling the same
four utilities — are one `.section-label` recipe, and Korean and Japanese
keep their case, so the hierarchy comes from size, weight and tracking
rather than from capitals ([GDK-141]). The QA teal was the only off-palette
neon in either theme; it is a `--color-status-qa` token from the avatar
family now, measured against done and in-progress ([GDK-160]). Deleting a
saved view and opening the Jira filter show on focus as well as hover
([GDK-728]); the document and person overlays carry the same back arrow as
the issue overlay ([GDK-729]); the palette keeps its icon rail even for
rows without one ([GDK-143]); the clipboard fallback's premise is scoped to
the build it was measured on ([GDK-1114]); and the scoped-token hint on a
401 was already there ([GDK-73]).

## v0.21.0 — 2026-09-08

**What happened while you were away, answered from the mirror.** Every
status change, comment and visit is already there, so 0.21 answers the
questions you ask on coming back (what changed, how long has this sat, did we
get faster) from that data, in numbers with their definition beside them.
Everything here is read-only over the mirror. Nothing new leaves the
machine. `gadak retro` gives a weekly retrospective: sessions, resume time,
how old the work in progress is, what closed and how long it took, and
*mismatch* (the newest comment says done, the status does not). Every row
prints its definition, and `--open` puts the issues behind a cell on the
app. The list opens with one dim line, *7 changed · 2 of them yours*, and
the detail panel says what moved on that issue since you last opened it; the
boundary is your own previous session, and that line no longer vanishes on a
quiet morning ([GDK-1537]). The stale mark learns its threshold from the
workspace's last 90 days of cycle times; `list`, `ready` and `next` carry
`age_days`; the durations chip shows the team's p85 on hover; a
done-sounding comment under an open status offers *Move to done*. Five
built-in views ([GDK-1493]) hold the sidebar now: My issues, Handed off, All
open, Unassigned new, Reopened. Aging, Epics, Stale and the rest leave it,
one *Save as view* away.

**Agent writes say so, and the skill installs on seven hosts.** A comment,
transition or issue written from the CLI against Jira Cloud or Linear ends
with *— via gadak · Name*, so a team can tell an agent's write from the
person whose credential it used. `actor.trailer=false` turns it off. And
`gadak skill install
--client codex|agents|cursor|gemini|opencode|grok` beside `claude`
([GDK-1508]).

**Japanese, the phone, and a round of fixes.** The site reads at `/ja/`, the
app picks Japanese type under `:lang(ja)`, and every landing clip, including
the live-Claude hero, is recorded in en, ko and ja over a mirror in that
language ([GDK-1500], [GDK-1501]). On the phone (TestFlight), descriptions,
comments and wiki pages render like the desk; assignee, priority, summary
and description are edited from the header; one pairing scan carries both
tokens ([GDK-1497], [GDK-1498]).

A serve host's mirror could drift from its origin until `--full`; reconcile
now looks both ways and `doctor` flags a short mirror ([GDK-1400]).
`gadak transition KEY inprogress` no longer refuses two destinations with
one display name ([GDK-1356]). Clicking the terminal to focus it no longer
opens the key under the pointer ([GDK-1186]). `gadak
migrate` keeps priorities when the mirror has no priority ids ([GDK-1491]). One Esc closes a picker,
not the panel ([GDK-1401]).

## v0.20.2 — 2026-09-06

**The Windows app is on the Microsoft Store.** The desktop app is on the
[Microsoft Store](https://apps.microsoft.com/detail/9NZW91TXH36G). The Store
signs it, so neither SmartScreen nor Smart App Control objects, and every
install guide leads with it ([GDK-1380]). `gadak-desktop.exe` is a
GUI-subsystem image at last: no console window beside the app (every zip
since 0.16 and the first Store package had one), and the pack refuses to
ship one again. The Store package declares an app-execution alias, so a
Store install puts `gadak` on `PATH`, and ja-JP as a third language.

**Linear relations arrive, and one field has one name.** Linear relations
are mirrored as `links` (blocks, duplicate, related), so the linked-issues
panel and the blockers recipes work on a Linear workspace ([GDK-1299]). One
field, one name: the breakdown axes read the same labels as the filter and
the columns ([GDK-1399]), and the link-type catalog is fetched once per
workspace, so a linked issue's direction phrase no longer flashes its bare
type name on every open ([GDK-1297]). `wiki`, `wiki.enabled` and
`wiki.spaces` are aliases of the `confluence.*` config paths, and
`config get`/`set` says where the value lives ([GDK-1289]). In the terminal, an
issue key printed under the resting pointer is clickable at once, not after
the mouse moves ([GDK-1172]).

**Fixes, and the window speaks three languages.** A Confluence page-version
write that fails degrades the mirror instead of killing the wiki pass
([GDK-1307]); the transition refusal for a number that is both a transition
id and a status id names the two forms that cannot collide ([GDK-1305]); and
two CI flakes are gone, the pairing mint test and the create-dialog e2e
([GDK-1306], [GDK-1295]). The window speaks English, Korean and Japanese,
and the READMEs and install pages now say so.

## v0.20.1 — 2026-09-03

**Bodies are markdown, and a formatted one survives the round trip.**
Markdown in, ADF out. `create -m`, `edit -m`, `comment`, the page verbs and
the web editors take markdown (headings, lists, code, tables, links) and
store what Jira or the Built-in tracker holds
([decision 0012](docs/decisions/0012-markdown-is-the-editing-source.md),
[GDK-1384], [GDK-1383]). The detail renders every body as markdown, so the
issues that showed as one grey wall since the cutover read as their blocks
again, and the editor has Write / Preview ([GDK-1385]). Linear gets the
same formatting on the wire ([GDK-1386]).

An edit of a formatted body is lossless. `gadak issue` prints the markdown
([GDK-1394]) with a placeholder for each node markdown cannot say (a
panel, a mention, an image, a coloured run), and `edit -m -` puts every
node back where its marker stands; delete a marker and that node goes, and
the write says so; a stale marker is refused ([GDK-1396]). `edit --adf-file`
and `comment --adf-file` send an ADF document as it is ([GDK-1395]).
`migrate --from` carries bodies as ADF, not flattened text ([GDK-1382]).

**One menu for the list, real names for the shells.** The list toolbar has
one view-settings menu for layout, sort, columns and "save as view", where
the board and columns buttons used to share a glyph ([GDK-1391]), and the
status breakdown axis is "Status", second, on every origin ([GDK-1390]). In
the terminal a shell is "shell 3", not eight hex digits; rename it with F2
or a double-click; the issue header opens or focuses the issue's shell
([GDK-1387], [GDK-1195], [GDK-1388]).

**A Store package on Windows, and fixes.** `build-windows.ps1 --msix` packs
a Microsoft Store package; the zip is unchanged ([GDK-1380]).
`close --resolution` works on a new Built-in workspace ([GDK-1347]);
`edit --batch` takes `label` / `component` / `fix_version` ([GDK-1259]);
avatar initials skip punctuation ([GDK-1351]); placeholder markers quoted in
code are text ([GDK-1398]); and a terminal resize the kernel did not take is
re-issued until it holds, which was the CI flake ([GDK-1192]).

## v0.20.0 — 2026-09-03

**The shell is in the window, and it is the issue's shell.** The terminal
dock is a band across the whole window now: one horizontal seam under the
sidebar, the list and a docked detail alike, a quarter of the height by
default, every column of the width ([GDK-1352]). The session roster is a
column on its left, as wide as the sidebar above it, so the window keeps
reading as navigation on the left and content on the right ([GDK-1355]).
The dock has an appearance of its own, dark under every app theme, because
xterm's sixteen are a dark-ground palette and nine of them fell under 3.0:1
on paper. Set `follow` if you want it in the app's colours;
a **Settings → Terminal** tab covers that, the font, scrollback and the
cursor ([GDK-1357]). The light palette declares its own sixteen (brick, moss,
ochre, indigo, plum, teal, readable greys for the two whites) as tokens
like any other, overridable through `ui.tokens.colors` ([GDK-1358]). The
block cursor is ink on its ground rather than a fill ([GDK-1359]), and the
pane paints its ground under the last row too ([GDK-1354]).

`gadak claim` typed in that pane binds the shell to the issue, and the
roster tab is named by the key. The README and the landing show exactly
that now, in gadak's own pane: the paper-terminal composites are retired
([GDK-1353]).

**A pass over the surface, and back works everywhere.** The sidebar is a
return path rather than a table of contents: a workspace row on top, quiet
sections, one footer row ([GDK-1335]). The list toolbar is one row until you
narrow the view, rows are 36px, the stale age is a number rather than a box,
and the palette has one entry point ([GDK-1336]). Every column screen
(Documents, spaces, History, dashboards, the feed) shares one header band
([GDK-1339]). The detail is one property list under a chip row that keeps
only what changes ([GDK-1337]), its prose has a type scale and a
wide-reading toggle ([GDK-1311]), and the board's recent actors are faces
([GDK-1338]). Every control reacts and nothing takes longer than 160ms;
hover is a wash of ink over whatever it sits on ([GDK-1341]). Copy link to
this view pastes the Jira navigator URL first and the app links after, the
way the issue detail's copy link already did ([GDK-1343]); the key of an
issue you have opened goes quiet, and one changed since carries a dot
([GDK-1344]); the breakdown legend folds to `+N more` instead of squeezing
labels to slivers ([GDK-1348]); Switch to *workspace* is a palette action
([GDK-1340]). The first run opens on "where do your issues live?" with
nothing preselected, says what gadak is before it asks for anything, and on
the built-in tracker the empty list invites the first issue instead of
promising a sync ([GDK-1342], [GDK-1345], [GDK-1286], [GDK-1287]).

The first inbound round shipped in full. @woojing brought five reports
(discussion #80, issue #85), and every one is in this release. Thank you.
Every place change goes through history (a linked issue, a person, a view,
a query), so the browser's back button does what it says ([GDK-1296]). The
detail's copy link pastes the origin's page first ([GDK-1290]); the settings
dialog keeps one width across tabs ([GDK-1291]); a dock click on a
fullscreen window stays fullscreen ([GDK-1294]); and a `gadak://` link
naming a workspace this machine lacks is refused in a dialog instead of
replacing the app with a 404 page ([GDK-1309]). Along the way, issue links
open on their own origin, one branch per tracker with no fallback across
them ([GDK-1149], [GDK-1308]).

**Leaving, keeping, and what was fixed.** `migrate --to linear` sends a
mirror into a Linear team, idempotently ([GDK-1265]). `gadak backup` copies
the built-in tracker's record with the serve running ([GDK-1277]). A paired
workspace mirrors the home origin's wiki ([GDK-1276]), and `pairing mint`
refuses a loopback offer that could never bind ([GDK-1266]). Confluence sync
excludes personal spaces only, so team space types stay ([GDK-1302]).
`docs/SUPPORT_MATRIX.md` is the one table for Jira, Linear and the
built-in tracker, a citation per cell ([GDK-1300]), and the built-in
tracker is named once across the CLI and the docs ([GDK-1285],
[GDK-1288]).

`edit -m -` refuses an empty stdin instead of clearing the description
([GDK-1360]). `migrate` onto the built-in tracker kept every status name
and lost every status category on a mirror whose `status_catalog` had never
been filled: Done and In Progress arrived as open issues, and `claim` had no
in-progress state to land on ([GDK-1361]). A write typed into a named
workspace's pane no longer opens
with `warning: workspace: … (from GADAK_WORKSPACE)`, which the serve had
set to name the window's own
workspace ([GDK-1362]). The freshness chip names the origin's tracker, not
Jira ([GDK-1325]); the phone's pairing tab says Built-in ([GDK-1321]). Three
defects the pre-tag audit found are in: `init --replace-standalone` is
accepted again, an issue with no origin page no longer advertises "Open in
Built-in", and `migrate --to linear` maps a team whose only done state is
canceled ([GDK-1312], [GDK-1313], [GDK-1314]).

## v0.19.3 — 2026-09-02

**Two words for two questions.** A workspace answered "what kind are you?"
with one word, and that word was carrying two facts at once: which tracker
the origin is, and whether it runs here. A paired workspace made the seam
visible: it reported `connected`, a word that reads as someone else's
tracker, while its origin was gadak's own tracker one machine away, and the
only way to tell was a separate pairing object. So the field is two now.
`origin_type` is `jira`, `linear` or `gadak`, and `transport` is `local`
(in this process) or `remote` (across a serve API). No new vocabulary was
invented, because `sources` already keyed on those tracker names ([GDK-1278],
[GDK-1279], [GDK-1280]).

`gadak status --json`, `doctor --json`, `workspaces --json`, the served
config and the MCP status tool all carry both. `kind` still ships and still
means what it meant, because the skill, the phone and the desktop shell all
read it; it is simply the coarser field now.

The flag that creates such a workspace follows: **`gadak init --local`**.
`--standalone` keeps working, because a flag already sitting in someone's
script is a contract. It is translated rather than registered as an alias,
though, so the help teaches one name instead of two ([GDK-1281]). The word
is gone from the app's copy, the docs, the skill an agent reads, and the source
itself: identifiers, filenames, test names ([GDK-1283], [GDK-1282]). Four
places keep the old spelling on purpose, because they are wire contracts
rather than vocabulary: the stored `kind` value, the `standalone_data_present`
error code, the `replace_standalone` request field, and the
`onboarding/standalone/` route.

**Fixed.** `migrate` warned and carried on when it could not read the
source's attachment bytes, and everything downstream reported success,
including the count table, which reads `attachments 26 26` because it counts
rows and bytes are what went missing. Since the procedure for leaving a
tracker is *freeze the source, then migrate*, that was the ordinary path,
not an edge case. It refuses before exporting now, naming both ways forward
([GDK-1275]). A workspace migrated out of a localized site arrived with two
issue types rendering as "Epic" and two statuses as "In Progress": the
seeded defaults stood beside the migrated catalog under ids the display
overlay maps to the same word. That left `--type Epic` with no answer at
all ([GDK-1284]).

## v0.19.2 — 2026-09-01

**Leaving a tracker, data in hand.** `gadak --workspace <new> migrate --from
<old>` exports a synced workspace's mirror into a brand-new standalone
workspace: issues, comments, the full changelog, links, attachment bytes,
and wiki pages, ending with a source-vs-migrated count table that includes
the derived columns. Reopen counts and epic keys have to re-derive from the
migrated history, so equality there is the real proof. Losses are reported
rather than silent: bodies migrate as plain text and the report says how
many code blocks, media nodes and tables that flattened; links and parents
outside the migrated set are counted as dropped. Measured on this project's
own backlog: 1,268 issues, 3,811 history rows, 26 of 26 attachments, every
row equal ([GDK-1264]).

That first full export moved the seed document to JSON: YAML's emitter
writes block scalars its own parser rejects on real tracker text, where a
value that starts with newlines, nested deep enough, fails to load back
([GDK-1269]). The embedded tracker now proves its own snapshots by parsing
them before they leave.

**An issue can point at an issue in another workspace.** `gadak ref STD-1
work/NMA-9` records a pointer, stored as a Jira remote issue link on your
own origin. Nothing is written to the workspace you point at, so a personal
note about a team ticket stays personal. The other half is what makes it
worth having: `gadak ref STD-1 --list`, and the issue's References section in
the app, show the target's **current** status, assignee and summary, read
out of that workspace's own mirror on this machine. No network call, no
second tab. A target this machine does not mirror still lists, and the row
says so ([GDK-1032]).

**The app now says you can make a workspace.** Creating, pairing and
removing workspaces has lived in Settings → Workspaces since 0.19, but the
sidebar's workspace section only appeared once you already had two, so the
main surface never said the feature existed. It now shows with one workspace
and carries a "New workspace" row into that panel ([GDK-1270]).

## v0.19.1 — 2026-09-01

The patch where the intuitive query becomes the correct one, written the
day after 0.19.0 shipped.

**`SELECT key, summary, status FROM issues` just works now.** The mirror's
most common agent miss was structural: the intuitive name held a narrow
internal table while the answer lived on `issues_full`. The names now match
the intent: `issues` is the full view (title and description included),
`issues_full` stays as a compatibility alias, and the physical table steps
back to an internal name ([GDK-1258]). A `SELECT *` on `issues` therefore
carries two more columns than before. When a query does name a column
that lives elsewhere, the error says where: `column "summary" exists on
issues — query issues`, instead of the silence that used to cost three
guesses ([GDK-974]).

**`gadak claim` survives a board with two in-progress lanes.** On a
workflow where two transitions land in progress, every bare claim used to
refuse with both candidates named and no way to pick one. `--transition
<id|name>` chooses the lane, on the atomic route and the Cloud fallback
alike. A destination outside in-progress is refused, because claim is
not a general transition ([GDK-1174]). The claim's other half got faster
too: the terminal strip now renames the session on the tick the write
landed, instead of waiting out its own two-second poll ([GDK-1182]).

**Two agents on one standalone workspace no longer mint the same id.**
Comment, history, and attachment ids came from per-process counters seeded
once at open, so three concurrent writers handed the same id to three
different issues. The counters now live in the working copy itself, one
atomic allocation per id, with no persist format change ([GDK-1180]).

Also: the AUR verify script's scratch directory moves inside the repo,
where Docker Desktop can actually mount it. A green container run that
left no `.SRCINFO` on the host now fails loudly ([GDK-1256]).

## v0.19.0 — 2026-09-01

The release where the issues become a board, and the terminal takes
its Beta mark off.

**The board.** The list you already filter is now also a board: one toggle
lays the same issues across columns, with the same filters, the same
thirteen grouping axes and the same search. Dragging a card is a real
transition, with a menu when more than one status matches. A move made
anywhere else, in another window or from an agent's `gadak transition`,
flies across the board with a landing ring, because on this screen the
movement is the only evidence it happened. The three status columns are
always all three: "Done is empty" is an answer, not a missing column
([GDK-1175], [GDK-1176], [GDK-1190]). A view can say it is a board: saved in
board layout it reopens as one, from the app and from the CLI: `gadak views
save "Sprint board" --jql '…' --layout
board`, with `views open` carrying the layout in the deeplink ([GDK-1248]).

The terminal takes its Beta mark off ([GDK-1024]), and its sessions
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

**The phone holds more than one home, and the CLI learned the verbs
sessions actually type.** A host roster with per-host caches switches
between paired machines; workspaces can be created and removed from the web
and the CLI; a bundled demo workspace shows every surface with zero pairing.
A glance strip answers "what moved while I was away", and the terminal
scrolls under a finger ([GDK-1097], [GDK-1096], [GDK-1098], [GDK-1051],
[GDK-871], [GDK-899]).

On the CLI: `gadak list`, `next`, `show`, `done`, `recent` and `pick`,
measured against what blind sessions reached for, plus
`memory add`/`memory search` for agent notes,
`edit --type` to refile a misfiled issue, `unlink` (the reverse verb `link`
never had), and `workspaces rm`. A write confirms
itself, and `edit -m` refuses to silently flatten a formatted description
([GDK-992], [GDK-1030], [GDK-1205], [GDK-1098], [GDK-1001]).

**Refusals got louder, writes got safer, and sync got faster.** A JQL clause
the mirror's subset cannot express is refused out loud instead of silently
widening the list ([GDK-1234]). Config and credential saves go through one
atomic stage-then-rename owner, so two saves can no longer torch each other
([GDK-1233], [GDK-1244]). A desktop boot that fails before the window opens
says so in a dialog instead of exiting silently ([GDK-1243]), and Linear
assignee edits go through the same write surface the UI advertises
([GDK-1235]). `gadak link` now points the link the way Jira will
display it; the outward and inward ends were swapped, on the CLI and
REST both ([GDK-1204]; #79, thanks @wafe). On quiet mirrors, Jira
incremental answers overlap-window echoes from the mirror instead of
refetching them, and Confluence incremental asks one CQL pair per tick and
stops re-reading bodies whose comments have not moved ([GDK-1075],
[GDK-1074]).

Also: pairing offers render as a scannable QR and the desktop gets a
Devices tab ([GDK-1047]); the update dialog announces the version and
points at the release page instead of dumping raw markdown ([GDK-1246]); a
standalone workspace stops offering a Jira credential dialog that cannot
help it ([GDK-1122]); and the installed agent skill follows the binary,
once a day ([GDK-996]). A full-codebase audit ran before this tag (parent
[GDK-1128]).

## v0.18.1 — 2026-08-26

**Closing a terminal closes everything it started.** The terminal 0.18.0
shipped learned to clean up after itself the same day. A shell puts a
background job in its own process group, so closing a session used to leave
`sleep 999 &`, or a forgotten agent, running forever. The close now walks
every process on the session's terminal once, while the walk can still be
trusted. It walks once because in a measured Linux failure a second walk
found *another* session's shell ([GDK-950]). When the pane cannot open at
all it says why in words, with a retry: no PTY on Windows, a token without
the terminal scope, a network drop, on the web and on the phone from the
same source ([GDK-944]).

**`gadak views open` reaches every open window.** The CLI's focus used to
be consumed by whichever window polled first, and the window you were
looking at stayed put. Two `views open` in the same second no longer
lose the second one: the payload is deduped on what it says, not just when
it was written ([GDK-960], [GDK-981]). A CLI exit path that used to leave a
stale "open" marker behind clears it ([GDK-971]).

**gadak keeps a log file, and doctor hands it to you.** A Finder-launched
app used to discard every diagnostic line; now `gadak doctor` names the file
and quotes its recent errors. Credentials are scrubbed before a line is
written ([GDK-967]). The agent skill teaches an agent to diagnose the wall
it hit and to report the friction instead of silently working around it
([GDK-968]), and the hosted demo's mirror is published as a file you can
download and open in your own gadak ([GDK-975]). The benchmark tables were
re-measured on a quiet machine, on the corpus the demo actually ships, and a
release audit of this delta ran before the tag (parent [GDK-980]).

## v0.18.0 — 2026-08-26

**A terminal, inside gadak.** ⌘K → Terminal, or `Ctrl+\``. It is a real
shell in the same window as your issues, so you can run a coding agent
there and watch it move the board next to it. Same terminal in the web tab,
in the macOS app, and on a paired phone. Korean composition lands on the
cursor, where a terminal canvas does not make that automatic. It ships
as **Beta**: useful, with rough edges named ([GDK-862], [GDK-864],
[GDK-865], [GDK-892], [GDK-895], [GDK-956]).

A token can open a shell and nothing else. `gadak pairing mint --scope
terminal` is the only kind that opens one; a `serve` or `origin` token opens
none, though a `serve` token now reaches the whole mirror REST rather than a
13-path allowlist. Revoke a terminal token and the shells it opened close
within seconds, and are told why. Loopback still needs no token at all
([GDK-863], [GDK-883]).

**The phone app is something you can use.** Issues in your own saved views,
wiki pages beside them, search that shows page hits, an issue detail that
lands on the thread, and notifications only for what is actually yours:
assigned, mentioned, reopened. It reaches the terminal too, over a second
token, and unpairing forgets that shell too. Internal TestFlight builds ship
in one command ([GDK-805], [GDK-867], [GDK-870], [GDK-879], [GDK-884],
[GDK-885], [GDK-886], [GDK-887], [GDK-888], [GDK-905], [GDK-906], [GDK-907],
[GDK-908], [GDK-910]).

**gadak says what it will mirror before you install it.** The wiki was
always opt-in and always per-space (`gadak init --spaces ENG,PROD`, or
Settings → Sources), but nothing said so until you had already installed and
run a sync. Someone told us they had not tried gadak because a whole
Confluence looked like too much to mirror. It never was ([GDK-964]). The
changelog you are reading is on the site now, in both languages, rendered
from the repository rather than copied, plus the search-engine basics the
site never had: canonical links, a sitemap, and structured data for the
releases. `gadak dev --help` lists every verb it has, and `gadak pairing`
lists, like every other list command ([GDK-946], [GDK-947]). The agent skill
carries a complete, working dashboard example instead of pointing at a file
that was never installed beside it ([GDK-963]).

`gadak create --type Bug` works on a non-English Jira. It used to fail
once on every localised site and make you look up the type id. Names, ids,
`epic`/`subtask`, and a small locale table all resolve now; two matches is a
clear error rather than a guess ([GDK-741]). A dashboard link opens an issue
without losing the wall ([GDK-880]). `gadak open` no longer scans ports to
find a running serve ([GDK-859]). WebSocket upgrades are origin-checked
([GDK-860]). One stale cached row can no longer blank the whole UI
([GDK-835]).

## v0.17.3 — 2026-08-25

**The phone stopped being a skeleton.** A paired iPhone reads your mirror
over its own `serve` pairing scope, a one-way door: that token cannot ride
the origin passthrough, and an origin token cannot dump the mirror.
`pairing mint` works on a connected workspace. On the phone: a quiet queue as
the first screen with real status names, pairing that proves the connection
it claims and explains each failure, search that answers before you type
(recent searches, your saved views as chips), an issue detail whose comment
draft survives a failed send, and notifications only for assigned, mentioned
and reopened, silent while you are looking at the app ([GDK-796], [GDK-797],
[GDK-798], [GDK-799], [GDK-800], [GDK-801], [GDK-802], [GDK-837]).

**Spacing, layout and type are yours too, not just colour.** `ui.tokens`
grew three more axes, and setting one no longer risks the rest: each is a
key-wise merge, so a bad write leaves the config untouched. One token is a
path of its own: `gadak config set ui.tokens.type.terminal 15px`, no JSON and
no quoting, with an unknown name refused rather than stored as a typo. `gadak
config get ui.tokens.dim-catalog` lists every name with its default, its
range, and what it has to move together with ([GDK-842], [GDK-849],
[GDK-850], [GDK-852], [GDK-853]).

Validation warns and saves instead of refusing. Contrast, colour distance,
deuteranopia and the token relations all still run and still tell you what
you are about to get, but only what the machine genuinely cannot honour is
rejected. The warnings carry the next move as well as the diagnosis: a
contrast line names the palettes that fail and where to fix them, a type line
prints the whole ladder that has to move together. Any look can be undone
with one CLI line ([GDK-856], [GDK-857], [GDK-858]).

**A dashboard link can open an issue.** A wall can now navigate the app to
any of its own routes (an issue, a saved view, a filtered list, a search),
and an external link opens a new tab instead of replacing the wall
([GDK-854]).

## v0.17.2 — 2026-08-25

**Your agent can build you a dashboard.** One HTML document plus named
queries, saved like a view and rendered full-tab in the running web UI. The
host runs the SQL (or JQL) and hands the rows in; the page itself never
reaches the network. Saving re-renders an open tab in about a second, and new
mirror data re-pushes on its own. Charts work offline: uPlot ships inside
gadak, so there is no CDN and no loosened policy ([GDK-781], [GDK-782],
[GDK-792], [GDK-793]). A dashboard can no longer paint over the issue list
([GDK-815], [GDK-821]), and it answers Esc like everything else ([GDK-827]).

Any chart library you want, downloaded once. `gadak dashboards lib
add <url>` fetches a library, pins its hash, and serves it locally, re-hashed
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

**Every answer names the source it came from.** `gadak sql` stopped
answering from a stale copy: an unprefixed table name now falls through to
the live `local.db` rather than a snapshot frozen at migration time, which
had been answering quietly and wrong ([GDK-824]). The staleness warning
names which source is stale: `mirror last synced
154h ago` came from the
oldest row across every source, so one quiet Confluence space made the whole
mirror read as six days old while `status` showed a watermark ten minutes
back: two screens, opposite stories, and no way to tell which to believe. It
now names the source and prints the same timestamp `status` does
([GDK-810]). A page id you read is a page id you can write to: search prints
it, the JSON help admits pages exist, and the agent recipes emit the id the
origin actually accepts ([GDK-816]).

Two typos the CLI used to swallow. `gadak create GDK "…"` filed an issue
whose title began with a project key; it is now refused with the `--project`
spelling, and only when the first word really is a key this workspace knows.
`config set projects` accepted any string; keys are shape-checked, the site
is asked whether they exist, and `status` and `sync` name both sides of a
scope that has drifted ([GDK-594], [GDK-809]). A collapsed documents tree
stays collapsed across a sync ([GDK-817]). The feed takes the column instead
of overlaying it, so Esc from the feed lands on the list. Fourteen error
messages across three languages now name the next move rather than only the
problem ([GDK-828], [GDK-829], [GDK-831]). The excerpt a list shows and the
text search indexes come from one place, so they cannot drift apart
([GDK-814]).

## v0.17.1 — 2026-08-24

**Two gadak processes can share one mirror.** A day of using gadak on a
20,000-issue mirror found every way two of them could end up waiting on one
file. A standalone workspace keeps its record in SQLite: the embedded
tracker writes to `origin/issuetap.db`, one transaction per write, instead of
rewriting a whole YAML file on a timer. An existing YAML seeds it once and is
left alone as your rollback; export still speaks YAML. Backup is that file:
stop the app and copy it, or use `sqlite3 .backup` while it runs ([GDK-202]).

"Database is busy" tells you who is holding it. A write that reached
Jira no longer fails just because the local re-read collided with another
process, and a genuine refusal names the neighbour (another app, a `serve`,
a CLI) instead of an error code. `gadak doctor` lists them. Browsing history
took its own connection, so reading no longer queues behind a sync, and an
agent's reads wait for the file rather than failing instantly ([GDK-740],
[GDK-753], [GDK-754], [GDK-757], [GDK-755]). It is faster where it was
slowest, measured at 20k issues: `gadak issue KEY` reads that one key instead
of loading the mirror, `search --jql` resolves people narrowly, on the CLI
and on the server, and `doctor` samples instead of scanning every document
([GDK-747], [GDK-748], [GDK-749], [GDK-756]).

**Exclude works on every filter.** A ⊘ on any value in any picker (Alt-click
too), replacing a modal toggle that only some menus had. Copy JQL writes
`not in` where JQL can say it and tells you which axes it left out, and
`search --jql` matches the same negations ([GDK-771]). Narrow windows stop
clipping: an audit of every seam below 1100px closed three, a chip that would
not hide, row columns cut mid-character, and a minimum width that disagreed
with the layout it was supposed to describe. A CI check keeps all three
closed ([GDK-758], [GDK-766]).

**On gadak.dev:** a Korean browser is offered the Korean page: a suggestion,
never a redirect, and the answer is remembered ([GDK-770]). Plus `llms.txt`
for agents reading the site, and landing media that shows the product at
readable scale instead of full-screen video ([GDK-751], [GDK-752]).

## v0.17.0 — 2026-08-23

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
[GDK-635], [GDK-643]). A write that reached Jira counts as a success even if
the local re-read right afterwards did not ([GDK-740]). Bulk reads take many
keys, or `--keys
-`, with nothing silently dropped ([GDK-328], [GDK-425]). `gadak claim KEY`
takes an issue in one move, and `gadak issue` shows how
long the work sat: `wait 3d · progress 5h` ([GDK-591]).

Writes carry who made them. `GADAK_ACTOR` names the agent, and the web
marks bot work with a badge, so a machine's edit can be told apart from
yours. A standalone workspace speaks your language, and a restricted
issue looks different from a public one ([GDK-519], [GDK-586], [GDK-588],
[GDK-590], [GDK-593], [GDK-597]). For agents there is more: `gadak pick`
chooses work, `gadak recents` walks back what the CLI has read,
`gadak sync --if-stale 15m` is the session opener an agent can call blind, batch writes
answer per key, honestly, and closing an issue is one round trip that is safe
to retry ([GDK-500], [GDK-501], [GDK-502], [GDK-503], [GDK-598],
[GDK-599]).

**A workspace stays chosen, and the window behaves the same everywhere.**
`gadak workspace use <name>` stores a default. Pairing tells the truth about
what it is and what failed, a bound workspace cannot be quietly repointed at
a different site, and replacing an origin takes its derived rows with it
([GDK-418], [GDK-433], [GDK-449], [GDK-452], [GDK-453], [GDK-490],
[GDK-561], [GDK-677], [GDK-678]). `rm gadak.db` no longer costs you
anything: saved views, visits and search history moved out of the mirror,
which is supposed to be the throwaway half ([GDK-105]). Korean search finds
the word inside the compound; fix versions keep their ids and the project's
release catalog reaches the mirror, sprint is a column you can query, and JQL
`parent =` / `parent IN` filter locally ([GDK-259], [GDK-444], [GDK-518],
[GDK-521], [GDK-532], [GDK-329]).

Esc closes the thing it was aimed at and nothing else. Four type sizes and
nothing between them. Empty is a state with words, not a blank. A transition
that needs a screen asks inline; components and parent edit in place; a story
shows its children. There is one kind of saved view. A read that finishes
quickly paints no skeleton at all. And a full Japanese catalog ([GDK-83],
[GDK-86], [GDK-121], [GDK-129], [GDK-130], [GDK-316], [GDK-437], [GDK-604],
[GDK-613], [GDK-617], [GDK-626], [GDK-737], [GDK-738], [GDK-739]). On the
desktop, a second launch raises the window you have instead of starting a
second one; on Windows, `gadak://` links work, `install-cli` speaks Windows,
and the app stops claiming it notified you when it did not ([GDK-349],
[GDK-350], [GDK-351], [GDK-353], [GDK-658], [GDK-700]).

The network was audited too. An empty host counts as a non-loopback bind, so
`serve` demands `--allow-remote` for it like any other exposure ([GDK-542]).
Linear's rate limit is retried rather than fatal, and a Linear-only workspace
is a configured workspace ([GDK-263], [GDK-654]). The Web Push client is
gone: it called endpoints the server answers 404 to, and vendor push services
are outbound traffic this project does not make ([GDK-711]). gadak's own
backlog is public, at gadak.dev, with a front door, a demo beside it, and a
page explaining that Windows warning ([GDK-211], [GDK-389], [GDK-676]).

## v0.16.1 — 2026-08-20

**Linear is a third tracker, and gadak writes to it.** This release finishes
what 0.16 started. A `"linear"` block in
your workspace config and `gadak sync --source linear` mirror issues,
comments, labels and attachments. Writes route by whichever origin owns the
row. What Linear cannot do yet is refused outright rather than half-applied.
Jira, standalone and Linear all answer the same write verbs ([GDK-263],
[GDK-359], [GDK-360], [GDK-361]). The wiki stops being read-only in the same
release: you can create a page, edit its title or body, and comment on it,
all through the origin, from the CLI or the REST API ([GDK-344], [GDK-380],
[GDK-381], [GDK-382]).

**Two gadak processes can no longer both write a standalone workspace.** The
desktop app advertises its origin the way `serve` does, so an app and a CLI
cannot both hold the record file. An acknowledged write is on disk before you
get the answer, and a write that could not be persisted returns an error
instead of success. A standalone failure no longer reports itself as a missing
credential, and converting a workspace says what conversion actually does to
your local-only issues ([GDK-241], [GDK-333], [GDK-340], [GDK-342],
[GDK-343], [GDK-345], [GDK-346], [GDK-347], [GDK-348]).

**Agents and docs learn that standalone exists.** The embedded skill knows
the word, and the CLI says which origin it means. `transition` names each
target's `status_id` and accepts the one the read path just handed out; that
round trip is where agents kept failing. `issues_full` gained
`description_text`, and a standalone `init` fills the mirror so nothing
starts empty ([GDK-239], [GDK-312], [GDK-313], [GDK-363], [GDK-364],
[GDK-365], [GDK-366], [GDK-367], [GDK-368], [GDK-371], [GDK-376]). The
install page covers standalone, the FAQ no longer tells you to
`rm -rf ~/.gadak`, the network has its own page, and export/import finally has a
paragraph ([GDK-271], [GDK-372], [GDK-373], [GDK-374], [GDK-375],
[GDK-601]).

## v0.16.0 — 2026-08-19

**A workspace without an Atlassian account.** Standalone: the origin is a
minimal tracker that runs inside gadak and travels with it. The mirror is
still a disposable cache, and every write still goes through the origin. The
only change is who the origin is. A workspace is bound to one origin, so
connecting a credential cannot quietly repoint it somewhere else. Standalone
wikis write through the same path ([GDK-183], [GDK-237], [GDK-238],
[GDK-247], [GDK-267]). A read-only Linear client landed as groundwork,
deliberately not wired to workspaces yet. That is 0.16.1 ([GDK-258],
[GDK-261], [GDK-263], [GDK-274]).

**Windows and Linux.** gadak stops needing a Mac to run. Windows gets a
portable pack, an installer path, a working `install-cli`, a Scoop manifest,
and `gadak://` links that survive first launch. Linux gets a tarball install
beside brew and an AUR packaging kit. Omarchy gets a bar widget showing what
changed in *your* mirror ([GDK-115], [GDK-116], [GDK-208], [GDK-209],
[GDK-225], [GDK-229], [GDK-246], [GDK-293]). gadak notices a new release,
says the right thing for your platform, and renders the notes in the app. It
never updates itself ([GDK-213], [GDK-214], [GDK-215], [GDK-216]). A cold
open no longer serialises everyone behind it, a contended write waits instead
of failing immediately, and a background sync stops outliving the server that
started it ([GDK-270], [GDK-282], [GDK-305]). The hosted demo opens on the
product, and feedback channels live in Settings and the macOS Help menu
([GDK-335], [GDK-336]).

**Edit an issue where you read it.** Due dates set and cleared from the
detail panel, with Jira's refusal shown as a sentence you can read.
Descriptions edit as plain text, with a guard before anything rich gets
destroyed. `p` opens a priority menu wherever `s`/`a`/`l` already work. What
is editable comes from the issue's own metadata rather than a fixed list, so
your site's custom fields are included ([GDK-82], [GDK-223], [GDK-249],
[GDK-250], [GDK-251], [GDK-322], [GDK-323], [GDK-331], [GDK-332]). The
palette can file an issue from whatever you just typed. Required fields with
obvious answers are no longer asked, and posting a comment now confirms that
it was posted ([GDK-217], [GDK-218], [GDK-300], [GDK-301], [GDK-302]).

A non-English Jira stops silently returning nothing. Status, priority and
issue type key on ids and categories everywhere instead of display names.
`status = 'In Progress'` returns zero rows on a Korean account; that class of
quiet wrong answer is closed ([GDK-161], [GDK-248], [GDK-272], [GDK-275]).
Korean mid-compound search works too ([GDK-259]).

## v0.15.2 — 2026-08-17

**Every field the settings dialog edits is also a CLI verb.** `gadak config
list | get | set` and the settings API go through one table, so they cannot
disagree. An agent can now set up a workspace end to end. Themes live in the
workspace config file, so picking one in the UI and setting it from a terminal
do the same thing ([GDK-190], [GDK-193]).

**Three dark themes.** `dark` is a neutral-cool charcoal, `ink` is a new
blue-black, and `ember` keeps the previous warm dark exactly as it was
([GDK-190]).

**Search and menus stop surprising you.** A bare number finds that issue in
any project, on every search surface ([GDK-186]). The settings dialog stops
repeating its mirror block above every tab ([GDK-188]). Menus no longer
install things without asking. Settings → Integrations does that, and lists
what is already installed ([GDK-189], [GDK-191]).

## v0.15.1 — 2026-08-17

**Three ways in, none of them needing a checkout.** `gadak raycast install`
carries the Raycast extension inside the binary, so a brew or app install
needs no checkout ([GDK-182]). The ⌘K palette is never blank: an empty query
shows recently updated issues under recently viewed, plus your saved views
([GDK-184]). Settings → Integrations (desktop) lists the agent surfaces
gadak installs into, detects honestly what is already there, and shows a live
log ([GDK-185]).

## v0.15.0 — 2026-08-17

**Any part of gadak can be handed over as a link.** `gadak://` deep links,
with every place in the app addressable. gadak also produces the links it
consumes: a copy-link action in the UI, and `gadak issue KEY --link` on the
CLI ([GDK-119], [GDK-124], [GDK-163], [GDK-164]). Search is fast enough to
drive another app's UI: typing an issue key finds that issue, and on a
20,000-issue mirror the worst case went from 1.6s to 110ms. That is what
makes a launcher extension feel local ([GDK-117], [GDK-166], [GDK-170]).

**A dark theme.** Warm ground, ink foregrounds, the same paper metaphor as
light, and no flash on first paint. Both palettes clear the same
measured floors: status colours stay distinguishable in normal and
colour-blind vision, and success and failure are never told by colour alone
([GDK-154], [GDK-156], [GDK-157], [GDK-158], [GDK-159], [GDK-162],
[GDK-171]).

**The list behaves like a list.** The right side of a row is a column you can
scan, the last row stops being cut in half, Esc closes what you are looking
at, and a panel that covers the list says so ([GDK-128], [GDK-131],
[GDK-132], [GDK-133]). Korean input works in the search box: a half-composed
syllable is not a query, and chosung matching is gone product-wide, because
it matched things you did not mean ([GDK-168], [GDK-169]). An issue can name
its parent, with `gadak create --parent` and `gadak edit
--parent` writing
the sub-issue relationship through Jira ([GDK-19], [GDK-86]).

At the edges, the hosted demo stops advertising verbs it cannot answer
([GDK-52]). A read-only home is a warning, not a refusal to start
([GDK-149], [GDK-173]). Copy means copied, an attachment is fetched at most
once, and the desktop app stops loading its runtime twice ([GDK-150],
[GDK-177], [GDK-178]).

## v0.14.2 — 2026-08-16

**A rejected or expiring token says so up front.** Every token trap is named
before you paste rather than after the 401 arrives, and a rejected token is
recoverable without having to write anything first ([GDK-68], [GDK-69],
[GDK-98]). Expiry is warned about before the sync fails ([GDK-67]).

**Setup and the skill install finish where you leave them.** Picking no
projects is a choice, not an unfinished form ([GDK-99]).
`gadak skill install` treats an upgrade as an upgrade, and the embedded
skill knows the verbs the CLI actually has ([GDK-91], [GDK-92]).

**Existing surfaces report what they are doing.** A tick over an unchanged
Confluence reads zero page bodies ([GDK-113]), and
`gadak issue <KEY> --derive` shows where the derived columns came from
([GDK-111]). History
keeps its order ([GDK-26]), `gadak sql` warns on a stale mirror ([GDK-90]),
and opening a mirror repairs a search index this build cannot write
([GDK-112]). The browse pane yields to Escape ([GDK-78]), and search help
works on touch ([GDK-53]).

## v0.14.1 — 2026-08-15

**The first CLI write verbs:** `gadak create`, `gadak attach`, `gadak edit`.
One day of using gadak on gadak's own backlog, shipped as it landed.

**The macOS app is notify-only, and the demo works where people tap it.**
The in-app self-updater, which was never exercised, is gone, and this release
deliberately ships no desktop zip ([GDK-58], [GDK-61]). The hosted demo runs
inside in-app browsers, with a first paint readable at phone width
([GDK-23], [GDK-51]).

**Failures say what happened.** A truncated key list says how many were
dropped, and a rejected credential stops the watch loop for every source
rather than one ([GDK-24], [GDK-35], [GDK-48]). Priority colours read the
rank, not the account's language. Keystrokes during boot are held rather than
dropped ([GDK-46], [GDK-76]).

## v0.14.0 — 2026-08-15

**Agents get a surface they can build on.** `gadak_search` takes `query`,
every tool error starts with `ERROR:` and echoes the keys it got, and a
response over the size cap sheds the oldest comments and says `truncated`
rather than lying about completeness. Three things are a promise you can
build on: `issues_full` plus the RECIPES queries, `gadak sql` stdout, and
`views open --keys -`.

**Numbers, with the rows where gadak loses.** Measured against a live
2,853-issue Cloud project: 42× on a simple filter, 162× on an epic
`GROUP BY`, and a reopen count that takes about twenty minutes over REST
against 14.5 ms locally.

**`brew install midagedev/tap/gadak` is the app now**; `gadak-cli` is the
CLI-only formula. `gadak export` / `gadak import` round-trip what you would
actually miss: saved views, watches, favourites, in a file that carries no
credentials and no site URL. Plus a Korean README, and a settings dialog that
stops claiming things about project selection that were not true.

## v0.13.0 — 2026-08-14

**One search box that searches everything, and a history that outlives the
mirror.** ⌘K queries every issue and document in one index, ignoring the
filter chips on the list; the box above the list keeps its old job, narrowing
what is already there. Issues, documents and searches land on one timeline in
`~/.gadak/local.db`, so throwing away the mirror does not throw that away,
and an agent can join what you visited to what you have in a single
`gadak sql`.

**The window follows the agent, and a Jira URL brings its filters with it.**
An arbitrary set of issue keys is a first-class view, so
`gadak views open --keys -` puts an agent's answer on your running window.
`gadak views open`
opens in gadak; `gadak open` leaves for Jira. Hosts without a shell get the
same through a new MCP tool.

A navigator URL or a `jql=` clause applies the matching chips, Copy JQL is
the way back, and anything in the unsupported subset is listed rather than
silently dropped. Your Jira saved filters show up in the sidebar, and
`gadak views` lists, shows, opens and saves them.

**Wiki scope is per space, and the rest is repair.** Each Confluence space
carries its own watermark, a newly selected space backfills in full, and a
space that leaves the scope is removed. Comment-only wiki edits reach the
mirror, an unchanged page stops bumping its version, and a deleted issue is
tombstoned by a single-item sync. An unknown `--profile` errors with the real
list, and a failed mirror re-read after an upload returns the documented
error.

Two fixes came from outside. People are matched by account id rather than by
email, so person filters no longer depend on your site making email addresses
visible, across JQL, saved views, filters and the member directory (#1,
thanks @elppaaa). The macOS window can be dragged (#2, thanks @wafe).

## v0.12.0 — 2026-08-13

**Paper, not a dark dashboard.** gadak is a strand (가닥): uncoated paper,
sumi ink, one 쪽빛 thread. The mark is 가 drawn as two strokes, and the
crystal-ball dashboard and the TUI are gone.

The rename reaches the binary, home directory (`~/.gadak`), environment
prefix (`GADAK_*`), MCP tools, module path and desktop bundle id. An existing
`~/.scry` tree is renamed on first launch, and `SCRY_*` is still read
wherever the `GADAK_*` equivalent is unset.

**Labels, priority and the title are editable.** Labels stay visible on the
list, edit on the issue, and apply to a selection from the bulk bar (`l`,
beside `s` and `a`). Priority writes by id from the site's own catalog.
Workspaces work in the desktop app, and every workspace with a credential
gets its own sync loop. Document lists stopped freezing on a large mirror:
4,433 ms to 68 ms on a 10,000-page window.

**Smaller things.** The native title bar is gone; window controls moved into
the sidebar. `gadak skill install` embeds the Claude Code skill without
needing MCP, and `gadak install-cli` puts the running binary on your PATH.
`gadak doctor` prints redacted diagnostics you can paste into a bug report.
`gadak api` is the raw Atlassian REST escape hatch, refused at a foreign
host.

## v0.9.0 — 2026-08-06

**A name in ⌘K opens a person.** The people axis is web-only this version,
and the demo has more than one person in it. Search says why it matched, with
a snippet of the field that hit, and every page in the list carries a
one-line body preview.

**One type scale and one orb.** A real type scale, muted text at 6.2:1, one
monochrome icon family, and an avatar palette where red stays reserved for
meaning. The wordmark's sphere sits on the x-height, every icon derives from
that same drawing, and the crescent logo is retired. The geometry matches: a
two-step height grid, corner radius that follows nesting, pinned detail-panel
headers, and consecutive comments by one author grouped under a single
header.

## v0.8.0 — 2026-08-06

**Gadak.app, the macOS desktop app.** The web UI in its own signed, notarized
window, with no local server at all; a second launch focuses the running
window, and the bundle carries the CLI. Sync starts after in-app onboarding,
without a restart.

## v0.7.0 — 2026-08-06

**An agent gets pinned to the right mirror.** `gadak mcp install <client>`
pins the current profile and absolute binary path into an MCP host
registration. The local API stops being an open proxy: cross-origin writes
and DNS-rebinding reads are rejected, and mirror file permissions tighten to
`0600` / `0700` on open.

**The window gets faster and easier to live in.** `serve` opens
`http://gadak.localhost` when the resolver maps it; a busy listen port hands
off to a running gadak or falls back to a free port. Space names, a docs UX
wave (Viewed / Updated / By author), and an epics built-in view. Keyboard
triage, a freshness chip, a warm-boot cache, and an interaction performance
gate against a 10k-issue fixture. Confluence sync hardened for real sites.

**A face:** wordmark, logo, and a favicon the app never had. The README leads
with the live demo, the demo speaks English (Korean narrative pages remain
for CJK search), and `docs/FAQ.md` answers the hard questions with evidence.

## v0.6.0 — 2026-08-06

**Confluence pages join the items spine.** A second source on that spine,
with docs in the web UI and a TUI docs navigator (`D`). Page labels are
collected on fetch and shown everywhere pages appear.

**Issues grow an honest epic hierarchy.** A derived `epic_key` (the nearest
level-1 ancestor) groups a sub-task under its epic rather than its story, and
in the web UI it becomes group labels, row chips, breadcrumb, and rollup.
Phones render the desktop layout instead of a squeezed column.

## v0.5.0 — 2026-08-05

**Workspaces, a neon TUI, and Korean that is found.** `serve` mounts every
profile under `/w/<name>/`. The TUI gets ambient animation, mouse support, a
palette, and match highlight. Search does prefix match, so inflected Korean
is found.

## v0.4.0 — 2026-08-05

**Three surfaces say more about what they are doing.** The TUI edits custom
fields, with Jira-allowed values only. An update notice does a daily
anonymous check on every surface, with opt-out. The hosted-demo
service-worker handshake times out cleanly and says so when the browser
cannot run the demo.

## v0.3.0 — 2026-08-05

**Custom fields configure themselves.** The first full sync discovers and
configures custom fields itself, and the filter axes come from what it found,
including multi-select editors. Sync progress carries a real total, projects
are optional on sync, and sync history sits behind the sidebar timestamp.

## v0.2.1 — 2026-08-05

**Signed binaries, and a demo that says it is one.** The macOS release
binaries are signed and notarized. The hosted demo simulates writes locally,
says the change was not saved, and carries copy that identifies the surface
as a demo.

## v0.2.0 — 2026-08-05

**The foundation, and a demo with nothing to install.** The storage schema,
plus HTTP, sync and agent contracts, and the SQLite implementation with WAL,
FTS5, and the derived-field calculator. `gadak serve` serves the built UI and
refuses a non-loopback bind without `--allow-remote`. Built-in views key on
axes that mean the same thing on every Jira site: resolution and reopen
detection key on status *category*, not a localized name. The hosted demo is
a static snapshot served by a demo-only service worker: no binary, no
account.

**Keeping up with the site.** `gadak serve` starts the sync watch loop by
default when a credential is configured, and `gadak install-service` writes a
launchd agent or systemd user unit; one OS desktop notification may fire for
new personal feed events. The personal watch feed is computed from the mirror
at query time over a 30-day window. gadak's own call volume shows in
`gadak status` and the settings runtime panel, hidden while the count is
zero.

**Handing a workspace to someone else, and the command line that does it.**
`gadak team export` / `import` writes the views, field map, group rules and
thresholds a team agrees on; credentials never travel, and a file containing
credential keys is refused on import. `gadak snapshot` builds a shareable
copy: `--spread` restates timestamps across a window while preserving every
issue's internal ordering, `--scale` clones issues onto new keys, `--now`
pins the clock, and a credential scan runs before the file is published.

`gadak fields` reports which custom fields are actually populated. Per-command
help is generated from the FlagSet, so it cannot drift. Favorites live in the
mirror, so `gadak sql` and agents can see them; the hosted demo falls back to
local storage. TUI parity: feed focus tabs, saved-view sort/dir/group_by, and
priority sorting keyed on `priority_rank`.

[GDK-19]: https://gadak.dev/backlog/#/?ks=GDK-19
[GDK-21]: https://gadak.dev/backlog/#/?ks=GDK-21
[GDK-23]: https://gadak.dev/backlog/#/?ks=GDK-23
[GDK-24]: https://gadak.dev/backlog/#/?ks=GDK-24
[GDK-25]: https://gadak.dev/backlog/#/?ks=GDK-25
[GDK-26]: https://gadak.dev/backlog/#/?ks=GDK-26
[GDK-35]: https://gadak.dev/backlog/#/?ks=GDK-35
[GDK-45]: https://gadak.dev/backlog/#/?ks=GDK-45
[GDK-46]: https://gadak.dev/backlog/#/?ks=GDK-46
[GDK-47]: https://gadak.dev/backlog/#/?ks=GDK-47
[GDK-48]: https://gadak.dev/backlog/#/?ks=GDK-48
[GDK-51]: https://gadak.dev/backlog/#/?ks=GDK-51
[GDK-52]: https://gadak.dev/backlog/#/?ks=GDK-52
[GDK-53]: https://gadak.dev/backlog/#/?ks=GDK-53
[GDK-54]: https://gadak.dev/backlog/#/?ks=GDK-54
[GDK-58]: https://gadak.dev/backlog/#/?ks=GDK-58
[GDK-61]: https://gadak.dev/backlog/#/?ks=GDK-61
[GDK-67]: https://gadak.dev/backlog/#/?ks=GDK-67
[GDK-68]: https://gadak.dev/backlog/#/?ks=GDK-68
[GDK-69]: https://gadak.dev/backlog/#/?ks=GDK-69
[GDK-73]: https://gadak.dev/backlog/#/?ks=GDK-73
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
[GDK-114]: https://gadak.dev/backlog/#/?ks=GDK-114
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
[GDK-136]: https://gadak.dev/backlog/#/?ks=GDK-136
[GDK-137]: https://gadak.dev/backlog/#/?ks=GDK-137
[GDK-138]: https://gadak.dev/backlog/#/?ks=GDK-138
[GDK-141]: https://gadak.dev/backlog/#/?ks=GDK-141
[GDK-142]: https://gadak.dev/backlog/#/?ks=GDK-142
[GDK-143]: https://gadak.dev/backlog/#/?ks=GDK-143
[GDK-149]: https://gadak.dev/backlog/#/?ks=GDK-149
[GDK-150]: https://gadak.dev/backlog/#/?ks=GDK-150
[GDK-154]: https://gadak.dev/backlog/#/?ks=GDK-154
[GDK-156]: https://gadak.dev/backlog/#/?ks=GDK-156
[GDK-157]: https://gadak.dev/backlog/#/?ks=GDK-157
[GDK-158]: https://gadak.dev/backlog/#/?ks=GDK-158
[GDK-159]: https://gadak.dev/backlog/#/?ks=GDK-159
[GDK-160]: https://gadak.dev/backlog/#/?ks=GDK-160
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
[GDK-202]: https://gadak.dev/backlog/#/?ks=GDK-202
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
[GDK-234]: https://gadak.dev/backlog/#/?ks=GDK-234
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
[GDK-307]: https://gadak.dev/backlog/#/?ks=GDK-307
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
[GDK-378]: https://gadak.dev/backlog/#/?ks=GDK-378
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
[GDK-487]: https://gadak.dev/backlog/#/?ks=GDK-487
[GDK-490]: https://gadak.dev/backlog/#/?ks=GDK-490
[GDK-493]: https://gadak.dev/backlog/#/?ks=GDK-493
[GDK-495]: https://gadak.dev/backlog/#/?ks=GDK-495
[GDK-496]: https://gadak.dev/backlog/#/?ks=GDK-496
[GDK-497]: https://gadak.dev/backlog/#/?ks=GDK-497
[GDK-500]: https://gadak.dev/backlog/#/?ks=GDK-500
[GDK-501]: https://gadak.dev/backlog/#/?ks=GDK-501
[GDK-502]: https://gadak.dev/backlog/#/?ks=GDK-502
[GDK-503]: https://gadak.dev/backlog/#/?ks=GDK-503
[GDK-506]: https://gadak.dev/backlog/#/?ks=GDK-506
[GDK-509]: https://gadak.dev/backlog/#/?ks=GDK-509
[GDK-513]: https://gadak.dev/backlog/#/?ks=GDK-513
[GDK-514]: https://gadak.dev/backlog/#/?ks=GDK-514
[GDK-515]: https://gadak.dev/backlog/#/?ks=GDK-515
[GDK-516]: https://gadak.dev/backlog/#/?ks=GDK-516
[GDK-517]: https://gadak.dev/backlog/#/?ks=GDK-517
[GDK-518]: https://gadak.dev/backlog/#/?ks=GDK-518
[GDK-519]: https://gadak.dev/backlog/#/?ks=GDK-519
[GDK-521]: https://gadak.dev/backlog/#/?ks=GDK-521
[GDK-524]: https://gadak.dev/backlog/#/?ks=GDK-524
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
[GDK-594]: https://gadak.dev/backlog/#/?ks=GDK-594
[GDK-596]: https://gadak.dev/backlog/#/?ks=GDK-596
[GDK-597]: https://gadak.dev/backlog/#/?ks=GDK-597
[GDK-598]: https://gadak.dev/backlog/#/?ks=GDK-598
[GDK-599]: https://gadak.dev/backlog/#/?ks=GDK-599
[GDK-601]: https://gadak.dev/backlog/#/?ks=GDK-601
[GDK-604]: https://gadak.dev/backlog/#/?ks=GDK-604
[GDK-613]: https://gadak.dev/backlog/#/?ks=GDK-613
[GDK-617]: https://gadak.dev/backlog/#/?ks=GDK-617
[GDK-626]: https://gadak.dev/backlog/#/?ks=GDK-626
[GDK-630]: https://gadak.dev/backlog/#/?ks=GDK-630
[GDK-635]: https://gadak.dev/backlog/#/?ks=GDK-635
[GDK-643]: https://gadak.dev/backlog/#/?ks=GDK-643
[GDK-654]: https://gadak.dev/backlog/#/?ks=GDK-654
[GDK-658]: https://gadak.dev/backlog/#/?ks=GDK-658
[GDK-670]: https://gadak.dev/backlog/#/?ks=GDK-670
[GDK-676]: https://gadak.dev/backlog/#/?ks=GDK-676
[GDK-677]: https://gadak.dev/backlog/#/?ks=GDK-677
[GDK-678]: https://gadak.dev/backlog/#/?ks=GDK-678
[GDK-688]: https://gadak.dev/backlog/#/?ks=GDK-688
[GDK-689]: https://gadak.dev/backlog/#/?ks=GDK-689
[GDK-693]: https://gadak.dev/backlog/#/?ks=GDK-693
[GDK-696]: https://gadak.dev/backlog/#/?ks=GDK-696
[GDK-698]: https://gadak.dev/backlog/#/?ks=GDK-698
[GDK-700]: https://gadak.dev/backlog/#/?ks=GDK-700
[GDK-711]: https://gadak.dev/backlog/#/?ks=GDK-711
[GDK-718]: https://gadak.dev/backlog/#/?ks=GDK-718
[GDK-720]: https://gadak.dev/backlog/#/?ks=GDK-720
[GDK-722]: https://gadak.dev/backlog/#/?ks=GDK-722
[GDK-723]: https://gadak.dev/backlog/#/?ks=GDK-723
[GDK-724]: https://gadak.dev/backlog/#/?ks=GDK-724
[GDK-725]: https://gadak.dev/backlog/#/?ks=GDK-725
[GDK-728]: https://gadak.dev/backlog/#/?ks=GDK-728
[GDK-729]: https://gadak.dev/backlog/#/?ks=GDK-729
[GDK-732]: https://gadak.dev/backlog/#/?ks=GDK-732
[GDK-733]: https://gadak.dev/backlog/#/?ks=GDK-733
[GDK-734]: https://gadak.dev/backlog/#/?ks=GDK-734
[GDK-737]: https://gadak.dev/backlog/#/?ks=GDK-737
[GDK-738]: https://gadak.dev/backlog/#/?ks=GDK-738
[GDK-739]: https://gadak.dev/backlog/#/?ks=GDK-739
[GDK-740]: https://gadak.dev/backlog/#/?ks=GDK-740
[GDK-741]: https://gadak.dev/backlog/#/?ks=GDK-741
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
[GDK-768]: https://gadak.dev/backlog/#/?ks=GDK-768
[GDK-770]: https://gadak.dev/backlog/#/?ks=GDK-770
[GDK-771]: https://gadak.dev/backlog/#/?ks=GDK-771
[GDK-781]: https://gadak.dev/backlog/#/?ks=GDK-781
[GDK-782]: https://gadak.dev/backlog/#/?ks=GDK-782
[GDK-783]: https://gadak.dev/backlog/#/?ks=GDK-783
[GDK-785]: https://gadak.dev/backlog/#/?ks=GDK-785
[GDK-786]: https://gadak.dev/backlog/#/?ks=GDK-786
[GDK-787]: https://gadak.dev/backlog/#/?ks=GDK-787
[GDK-791]: https://gadak.dev/backlog/#/?ks=GDK-791
[GDK-792]: https://gadak.dev/backlog/#/?ks=GDK-792
[GDK-793]: https://gadak.dev/backlog/#/?ks=GDK-793
[GDK-796]: https://gadak.dev/backlog/#/?ks=GDK-796
[GDK-797]: https://gadak.dev/backlog/#/?ks=GDK-797
[GDK-798]: https://gadak.dev/backlog/#/?ks=GDK-798
[GDK-799]: https://gadak.dev/backlog/#/?ks=GDK-799
[GDK-800]: https://gadak.dev/backlog/#/?ks=GDK-800
[GDK-801]: https://gadak.dev/backlog/#/?ks=GDK-801
[GDK-802]: https://gadak.dev/backlog/#/?ks=GDK-802
[GDK-803]: https://gadak.dev/backlog/#/?ks=GDK-803
[GDK-804]: https://gadak.dev/backlog/#/?ks=GDK-804
[GDK-805]: https://gadak.dev/backlog/#/?ks=GDK-805
[GDK-808]: https://gadak.dev/backlog/#/?ks=GDK-808
[GDK-809]: https://gadak.dev/backlog/#/?ks=GDK-809
[GDK-810]: https://gadak.dev/backlog/#/?ks=GDK-810
[GDK-814]: https://gadak.dev/backlog/#/?ks=GDK-814
[GDK-815]: https://gadak.dev/backlog/#/?ks=GDK-815
[GDK-816]: https://gadak.dev/backlog/#/?ks=GDK-816
[GDK-817]: https://gadak.dev/backlog/#/?ks=GDK-817
[GDK-821]: https://gadak.dev/backlog/#/?ks=GDK-821
[GDK-824]: https://gadak.dev/backlog/#/?ks=GDK-824
[GDK-827]: https://gadak.dev/backlog/#/?ks=GDK-827
[GDK-828]: https://gadak.dev/backlog/#/?ks=GDK-828
[GDK-829]: https://gadak.dev/backlog/#/?ks=GDK-829
[GDK-831]: https://gadak.dev/backlog/#/?ks=GDK-831
[GDK-832]: https://gadak.dev/backlog/#/?ks=GDK-832
[GDK-835]: https://gadak.dev/backlog/#/?ks=GDK-835
[GDK-837]: https://gadak.dev/backlog/#/?ks=GDK-837
[GDK-842]: https://gadak.dev/backlog/#/?ks=GDK-842
[GDK-849]: https://gadak.dev/backlog/#/?ks=GDK-849
[GDK-850]: https://gadak.dev/backlog/#/?ks=GDK-850
[GDK-852]: https://gadak.dev/backlog/#/?ks=GDK-852
[GDK-853]: https://gadak.dev/backlog/#/?ks=GDK-853
[GDK-854]: https://gadak.dev/backlog/#/?ks=GDK-854
[GDK-856]: https://gadak.dev/backlog/#/?ks=GDK-856
[GDK-857]: https://gadak.dev/backlog/#/?ks=GDK-857
[GDK-858]: https://gadak.dev/backlog/#/?ks=GDK-858
[GDK-859]: https://gadak.dev/backlog/#/?ks=GDK-859
[GDK-860]: https://gadak.dev/backlog/#/?ks=GDK-860
[GDK-862]: https://gadak.dev/backlog/#/?ks=GDK-862
[GDK-863]: https://gadak.dev/backlog/#/?ks=GDK-863
[GDK-864]: https://gadak.dev/backlog/#/?ks=GDK-864
[GDK-865]: https://gadak.dev/backlog/#/?ks=GDK-865
[GDK-867]: https://gadak.dev/backlog/#/?ks=GDK-867
[GDK-870]: https://gadak.dev/backlog/#/?ks=GDK-870
[GDK-871]: https://gadak.dev/backlog/#/?ks=GDK-871
[GDK-873]: https://gadak.dev/backlog/#/?ks=GDK-873
[GDK-875]: https://gadak.dev/backlog/#/?ks=GDK-875
[GDK-879]: https://gadak.dev/backlog/#/?ks=GDK-879
[GDK-880]: https://gadak.dev/backlog/#/?ks=GDK-880
[GDK-883]: https://gadak.dev/backlog/#/?ks=GDK-883
[GDK-884]: https://gadak.dev/backlog/#/?ks=GDK-884
[GDK-885]: https://gadak.dev/backlog/#/?ks=GDK-885
[GDK-886]: https://gadak.dev/backlog/#/?ks=GDK-886
[GDK-887]: https://gadak.dev/backlog/#/?ks=GDK-887
[GDK-888]: https://gadak.dev/backlog/#/?ks=GDK-888
[GDK-890]: https://gadak.dev/backlog/#/?ks=GDK-890
[GDK-892]: https://gadak.dev/backlog/#/?ks=GDK-892
[GDK-895]: https://gadak.dev/backlog/#/?ks=GDK-895
[GDK-897]: https://gadak.dev/backlog/#/?ks=GDK-897
[GDK-899]: https://gadak.dev/backlog/#/?ks=GDK-899
[GDK-905]: https://gadak.dev/backlog/#/?ks=GDK-905
[GDK-906]: https://gadak.dev/backlog/#/?ks=GDK-906
[GDK-907]: https://gadak.dev/backlog/#/?ks=GDK-907
[GDK-908]: https://gadak.dev/backlog/#/?ks=GDK-908
[GDK-910]: https://gadak.dev/backlog/#/?ks=GDK-910
[GDK-911]: https://gadak.dev/backlog/#/?ks=GDK-911
[GDK-919]: https://gadak.dev/backlog/#/?ks=GDK-919
[GDK-921]: https://gadak.dev/backlog/#/?ks=GDK-921
[GDK-923]: https://gadak.dev/backlog/#/?ks=GDK-923
[GDK-927]: https://gadak.dev/backlog/#/?ks=GDK-927
[GDK-928]: https://gadak.dev/backlog/#/?ks=GDK-928
[GDK-941]: https://gadak.dev/backlog/#/?ks=GDK-941
[GDK-942]: https://gadak.dev/backlog/#/?ks=GDK-942
[GDK-944]: https://gadak.dev/backlog/#/?ks=GDK-944
[GDK-946]: https://gadak.dev/backlog/#/?ks=GDK-946
[GDK-947]: https://gadak.dev/backlog/#/?ks=GDK-947
[GDK-950]: https://gadak.dev/backlog/#/?ks=GDK-950
[GDK-952]: https://gadak.dev/backlog/#/?ks=GDK-952
[GDK-954]: https://gadak.dev/backlog/#/?ks=GDK-954
[GDK-956]: https://gadak.dev/backlog/#/?ks=GDK-956
[GDK-960]: https://gadak.dev/backlog/#/?ks=GDK-960
[GDK-963]: https://gadak.dev/backlog/#/?ks=GDK-963
[GDK-964]: https://gadak.dev/backlog/#/?ks=GDK-964
[GDK-967]: https://gadak.dev/backlog/#/?ks=GDK-967
[GDK-968]: https://gadak.dev/backlog/#/?ks=GDK-968
[GDK-971]: https://gadak.dev/backlog/#/?ks=GDK-971
[GDK-974]: https://gadak.dev/backlog/#/?ks=GDK-974
[GDK-975]: https://gadak.dev/backlog/#/?ks=GDK-975
[GDK-976]: https://gadak.dev/backlog/#/?ks=GDK-976
[GDK-978]: https://gadak.dev/backlog/#/?ks=GDK-978
[GDK-980]: https://gadak.dev/backlog/#/?ks=GDK-980
[GDK-981]: https://gadak.dev/backlog/#/?ks=GDK-981
[GDK-992]: https://gadak.dev/backlog/#/?ks=GDK-992
[GDK-996]: https://gadak.dev/backlog/#/?ks=GDK-996
[GDK-1001]: https://gadak.dev/backlog/#/?ks=GDK-1001
[GDK-1003]: https://gadak.dev/backlog/#/?ks=GDK-1003
[GDK-1004]: https://gadak.dev/backlog/#/?ks=GDK-1004
[GDK-1021]: https://gadak.dev/backlog/#/?ks=GDK-1021
[GDK-1024]: https://gadak.dev/backlog/#/?ks=GDK-1024
[GDK-1030]: https://gadak.dev/backlog/#/?ks=GDK-1030
[GDK-1032]: https://gadak.dev/backlog/#/?ks=GDK-1032
[GDK-1047]: https://gadak.dev/backlog/#/?ks=GDK-1047
[GDK-1051]: https://gadak.dev/backlog/#/?ks=GDK-1051
[GDK-1074]: https://gadak.dev/backlog/#/?ks=GDK-1074
[GDK-1075]: https://gadak.dev/backlog/#/?ks=GDK-1075
[GDK-1091]: https://gadak.dev/backlog/#/?ks=GDK-1091
[GDK-1092]: https://gadak.dev/backlog/#/?ks=GDK-1092
[GDK-1093]: https://gadak.dev/backlog/#/?ks=GDK-1093
[GDK-1096]: https://gadak.dev/backlog/#/?ks=GDK-1096
[GDK-1097]: https://gadak.dev/backlog/#/?ks=GDK-1097
[GDK-1098]: https://gadak.dev/backlog/#/?ks=GDK-1098
[GDK-1101]: https://gadak.dev/backlog/#/?ks=GDK-1101
[GDK-1104]: https://gadak.dev/backlog/#/?ks=GDK-1104
[GDK-1105]: https://gadak.dev/backlog/#/?ks=GDK-1105
[GDK-1107]: https://gadak.dev/backlog/#/?ks=GDK-1107
[GDK-1108]: https://gadak.dev/backlog/#/?ks=GDK-1108
[GDK-1110]: https://gadak.dev/backlog/#/?ks=GDK-1110
[GDK-1113]: https://gadak.dev/backlog/#/?ks=GDK-1113
[GDK-1114]: https://gadak.dev/backlog/#/?ks=GDK-1114
[GDK-1121]: https://gadak.dev/backlog/#/?ks=GDK-1121
[GDK-1122]: https://gadak.dev/backlog/#/?ks=GDK-1122
[GDK-1125]: https://gadak.dev/backlog/#/?ks=GDK-1125
[GDK-1128]: https://gadak.dev/backlog/#/?ks=GDK-1128
[GDK-1130]: https://gadak.dev/backlog/#/?ks=GDK-1130
[GDK-1132]: https://gadak.dev/backlog/#/?ks=GDK-1132
[GDK-1133]: https://gadak.dev/backlog/#/?ks=GDK-1133
[GDK-1134]: https://gadak.dev/backlog/#/?ks=GDK-1134
[GDK-1135]: https://gadak.dev/backlog/#/?ks=GDK-1135
[GDK-1141]: https://gadak.dev/backlog/#/?ks=GDK-1141
[GDK-1147]: https://gadak.dev/backlog/#/?ks=GDK-1147
[GDK-1149]: https://gadak.dev/backlog/#/?ks=GDK-1149
[GDK-1150]: https://gadak.dev/backlog/#/?ks=GDK-1150
[GDK-1158]: https://gadak.dev/backlog/#/?ks=GDK-1158
[GDK-1172]: https://gadak.dev/backlog/#/?ks=GDK-1172
[GDK-1174]: https://gadak.dev/backlog/#/?ks=GDK-1174
[GDK-1175]: https://gadak.dev/backlog/#/?ks=GDK-1175
[GDK-1176]: https://gadak.dev/backlog/#/?ks=GDK-1176
[GDK-1180]: https://gadak.dev/backlog/#/?ks=GDK-1180
[GDK-1182]: https://gadak.dev/backlog/#/?ks=GDK-1182
[GDK-1186]: https://gadak.dev/backlog/#/?ks=GDK-1186
[GDK-1187]: https://gadak.dev/backlog/#/?ks=GDK-1187
[GDK-1190]: https://gadak.dev/backlog/#/?ks=GDK-1190
[GDK-1192]: https://gadak.dev/backlog/#/?ks=GDK-1192
[GDK-1194]: https://gadak.dev/backlog/#/?ks=GDK-1194
[GDK-1195]: https://gadak.dev/backlog/#/?ks=GDK-1195
[GDK-1196]: https://gadak.dev/backlog/#/?ks=GDK-1196
[GDK-1197]: https://gadak.dev/backlog/#/?ks=GDK-1197
[GDK-1199]: https://gadak.dev/backlog/#/?ks=GDK-1199
[GDK-1200]: https://gadak.dev/backlog/#/?ks=GDK-1200
[GDK-1204]: https://gadak.dev/backlog/#/?ks=GDK-1204
[GDK-1205]: https://gadak.dev/backlog/#/?ks=GDK-1205
[GDK-1206]: https://gadak.dev/backlog/#/?ks=GDK-1206
[GDK-1215]: https://gadak.dev/backlog/#/?ks=GDK-1215
[GDK-1216]: https://gadak.dev/backlog/#/?ks=GDK-1216
[GDK-1227]: https://gadak.dev/backlog/#/?ks=GDK-1227
[GDK-1232]: https://gadak.dev/backlog/#/?ks=GDK-1232
[GDK-1233]: https://gadak.dev/backlog/#/?ks=GDK-1233
[GDK-1234]: https://gadak.dev/backlog/#/?ks=GDK-1234
[GDK-1235]: https://gadak.dev/backlog/#/?ks=GDK-1235
[GDK-1242]: https://gadak.dev/backlog/#/?ks=GDK-1242
[GDK-1243]: https://gadak.dev/backlog/#/?ks=GDK-1243
[GDK-1244]: https://gadak.dev/backlog/#/?ks=GDK-1244
[GDK-1245]: https://gadak.dev/backlog/#/?ks=GDK-1245
[GDK-1246]: https://gadak.dev/backlog/#/?ks=GDK-1246
[GDK-1248]: https://gadak.dev/backlog/#/?ks=GDK-1248
[GDK-1250]: https://gadak.dev/backlog/#/?ks=GDK-1250
[GDK-1251]: https://gadak.dev/backlog/#/?ks=GDK-1251
[GDK-1255]: https://gadak.dev/backlog/#/?ks=GDK-1255
[GDK-1256]: https://gadak.dev/backlog/#/?ks=GDK-1256
[GDK-1258]: https://gadak.dev/backlog/#/?ks=GDK-1258
[GDK-1259]: https://gadak.dev/backlog/#/?ks=GDK-1259
[GDK-1260]: https://gadak.dev/backlog/#/?ks=GDK-1260
[GDK-1264]: https://gadak.dev/backlog/#/?ks=GDK-1264
[GDK-1265]: https://gadak.dev/backlog/#/?ks=GDK-1265
[GDK-1266]: https://gadak.dev/backlog/#/?ks=GDK-1266
[GDK-1267]: https://gadak.dev/backlog/#/?ks=GDK-1267
[GDK-1269]: https://gadak.dev/backlog/#/?ks=GDK-1269
[GDK-1270]: https://gadak.dev/backlog/#/?ks=GDK-1270
[GDK-1273]: https://gadak.dev/backlog/#/?ks=GDK-1273
[GDK-1275]: https://gadak.dev/backlog/#/?ks=GDK-1275
[GDK-1276]: https://gadak.dev/backlog/#/?ks=GDK-1276
[GDK-1277]: https://gadak.dev/backlog/#/?ks=GDK-1277
[GDK-1278]: https://gadak.dev/backlog/#/?ks=GDK-1278
[GDK-1279]: https://gadak.dev/backlog/#/?ks=GDK-1279
[GDK-1280]: https://gadak.dev/backlog/#/?ks=GDK-1280
[GDK-1281]: https://gadak.dev/backlog/#/?ks=GDK-1281
[GDK-1282]: https://gadak.dev/backlog/#/?ks=GDK-1282
[GDK-1283]: https://gadak.dev/backlog/#/?ks=GDK-1283
[GDK-1284]: https://gadak.dev/backlog/#/?ks=GDK-1284
[GDK-1285]: https://gadak.dev/backlog/#/?ks=GDK-1285
[GDK-1286]: https://gadak.dev/backlog/#/?ks=GDK-1286
[GDK-1287]: https://gadak.dev/backlog/#/?ks=GDK-1287
[GDK-1288]: https://gadak.dev/backlog/#/?ks=GDK-1288
[GDK-1289]: https://gadak.dev/backlog/#/?ks=GDK-1289
[GDK-1290]: https://gadak.dev/backlog/#/?ks=GDK-1290
[GDK-1291]: https://gadak.dev/backlog/#/?ks=GDK-1291
[GDK-1294]: https://gadak.dev/backlog/#/?ks=GDK-1294
[GDK-1295]: https://gadak.dev/backlog/#/?ks=GDK-1295
[GDK-1296]: https://gadak.dev/backlog/#/?ks=GDK-1296
[GDK-1297]: https://gadak.dev/backlog/#/?ks=GDK-1297
[GDK-1298]: https://gadak.dev/backlog/#/?ks=GDK-1298
[GDK-1299]: https://gadak.dev/backlog/#/?ks=GDK-1299
[GDK-1300]: https://gadak.dev/backlog/#/?ks=GDK-1300
[GDK-1302]: https://gadak.dev/backlog/#/?ks=GDK-1302
[GDK-1305]: https://gadak.dev/backlog/#/?ks=GDK-1305
[GDK-1306]: https://gadak.dev/backlog/#/?ks=GDK-1306
[GDK-1307]: https://gadak.dev/backlog/#/?ks=GDK-1307
[GDK-1308]: https://gadak.dev/backlog/#/?ks=GDK-1308
[GDK-1309]: https://gadak.dev/backlog/#/?ks=GDK-1309
[GDK-1311]: https://gadak.dev/backlog/#/?ks=GDK-1311
[GDK-1312]: https://gadak.dev/backlog/#/?ks=GDK-1312
[GDK-1313]: https://gadak.dev/backlog/#/?ks=GDK-1313
[GDK-1314]: https://gadak.dev/backlog/#/?ks=GDK-1314
[GDK-1315]: https://gadak.dev/backlog/#/?ks=GDK-1315
[GDK-1316]: https://gadak.dev/backlog/#/?ks=GDK-1316
[GDK-1317]: https://gadak.dev/backlog/#/?ks=GDK-1317
[GDK-1318]: https://gadak.dev/backlog/#/?ks=GDK-1318
[GDK-1319]: https://gadak.dev/backlog/#/?ks=GDK-1319
[GDK-1320]: https://gadak.dev/backlog/#/?ks=GDK-1320
[GDK-1321]: https://gadak.dev/backlog/#/?ks=GDK-1321
[GDK-1323]: https://gadak.dev/backlog/#/?ks=GDK-1323
[GDK-1324]: https://gadak.dev/backlog/#/?ks=GDK-1324
[GDK-1325]: https://gadak.dev/backlog/#/?ks=GDK-1325
[GDK-1326]: https://gadak.dev/backlog/#/?ks=GDK-1326
[GDK-1327]: https://gadak.dev/backlog/#/?ks=GDK-1327
[GDK-1328]: https://gadak.dev/backlog/#/?ks=GDK-1328
[GDK-1332]: https://gadak.dev/backlog/#/?ks=GDK-1332
[GDK-1335]: https://gadak.dev/backlog/#/?ks=GDK-1335
[GDK-1336]: https://gadak.dev/backlog/#/?ks=GDK-1336
[GDK-1337]: https://gadak.dev/backlog/#/?ks=GDK-1337
[GDK-1338]: https://gadak.dev/backlog/#/?ks=GDK-1338
[GDK-1339]: https://gadak.dev/backlog/#/?ks=GDK-1339
[GDK-1340]: https://gadak.dev/backlog/#/?ks=GDK-1340
[GDK-1341]: https://gadak.dev/backlog/#/?ks=GDK-1341
[GDK-1342]: https://gadak.dev/backlog/#/?ks=GDK-1342
[GDK-1343]: https://gadak.dev/backlog/#/?ks=GDK-1343
[GDK-1344]: https://gadak.dev/backlog/#/?ks=GDK-1344
[GDK-1345]: https://gadak.dev/backlog/#/?ks=GDK-1345
[GDK-1347]: https://gadak.dev/backlog/#/?ks=GDK-1347
[GDK-1348]: https://gadak.dev/backlog/#/?ks=GDK-1348
[GDK-1349]: https://gadak.dev/backlog/#/?ks=GDK-1349
[GDK-1351]: https://gadak.dev/backlog/#/?ks=GDK-1351
[GDK-1352]: https://gadak.dev/backlog/#/?ks=GDK-1352
[GDK-1353]: https://gadak.dev/backlog/#/?ks=GDK-1353
[GDK-1354]: https://gadak.dev/backlog/#/?ks=GDK-1354
[GDK-1355]: https://gadak.dev/backlog/#/?ks=GDK-1355
[GDK-1356]: https://gadak.dev/backlog/#/?ks=GDK-1356
[GDK-1357]: https://gadak.dev/backlog/#/?ks=GDK-1357
[GDK-1358]: https://gadak.dev/backlog/#/?ks=GDK-1358
[GDK-1359]: https://gadak.dev/backlog/#/?ks=GDK-1359
[GDK-1360]: https://gadak.dev/backlog/#/?ks=GDK-1360
[GDK-1361]: https://gadak.dev/backlog/#/?ks=GDK-1361
[GDK-1362]: https://gadak.dev/backlog/#/?ks=GDK-1362
[GDK-1369]: https://gadak.dev/backlog/#/?ks=GDK-1369
[GDK-1380]: https://gadak.dev/backlog/#/?ks=GDK-1380
[GDK-1382]: https://gadak.dev/backlog/#/?ks=GDK-1382
[GDK-1383]: https://gadak.dev/backlog/#/?ks=GDK-1383
[GDK-1384]: https://gadak.dev/backlog/#/?ks=GDK-1384
[GDK-1385]: https://gadak.dev/backlog/#/?ks=GDK-1385
[GDK-1386]: https://gadak.dev/backlog/#/?ks=GDK-1386
[GDK-1387]: https://gadak.dev/backlog/#/?ks=GDK-1387
[GDK-1388]: https://gadak.dev/backlog/#/?ks=GDK-1388
[GDK-1390]: https://gadak.dev/backlog/#/?ks=GDK-1390
[GDK-1391]: https://gadak.dev/backlog/#/?ks=GDK-1391
[GDK-1394]: https://gadak.dev/backlog/#/?ks=GDK-1394
[GDK-1395]: https://gadak.dev/backlog/#/?ks=GDK-1395
[GDK-1396]: https://gadak.dev/backlog/#/?ks=GDK-1396
[GDK-1398]: https://gadak.dev/backlog/#/?ks=GDK-1398
[GDK-1399]: https://gadak.dev/backlog/#/?ks=GDK-1399
[GDK-1400]: https://gadak.dev/backlog/#/?ks=GDK-1400
[GDK-1401]: https://gadak.dev/backlog/#/?ks=GDK-1401
[GDK-1407]: https://gadak.dev/backlog/#/?ks=GDK-1407
[GDK-1413]: https://gadak.dev/backlog/#/?ks=GDK-1413
[GDK-1427]: https://gadak.dev/backlog/#/?ks=GDK-1427
[GDK-1428]: https://gadak.dev/backlog/#/?ks=GDK-1428
[GDK-1429]: https://gadak.dev/backlog/#/?ks=GDK-1429
[GDK-1438]: https://gadak.dev/backlog/#/?ks=GDK-1438
[GDK-1451]: https://gadak.dev/backlog/#/?ks=GDK-1451
[GDK-1453]: https://gadak.dev/backlog/#/?ks=GDK-1453
[GDK-1455]: https://gadak.dev/backlog/#/?ks=GDK-1455
[GDK-1457]: https://gadak.dev/backlog/#/?ks=GDK-1457
[GDK-1482]: https://gadak.dev/backlog/#/?ks=GDK-1482
[GDK-1483]: https://gadak.dev/backlog/#/?ks=GDK-1483
[GDK-1485]: https://gadak.dev/backlog/#/?ks=GDK-1485
[GDK-1488]: https://gadak.dev/backlog/#/?ks=GDK-1488
[GDK-1490]: https://gadak.dev/backlog/#/?ks=GDK-1490
[GDK-1491]: https://gadak.dev/backlog/#/?ks=GDK-1491
[GDK-1492]: https://gadak.dev/backlog/#/?ks=GDK-1492
[GDK-1493]: https://gadak.dev/backlog/#/?ks=GDK-1493
[GDK-1496]: https://gadak.dev/backlog/#/?ks=GDK-1496
[GDK-1497]: https://gadak.dev/backlog/#/?ks=GDK-1497
[GDK-1498]: https://gadak.dev/backlog/#/?ks=GDK-1498
[GDK-1500]: https://gadak.dev/backlog/#/?ks=GDK-1500
[GDK-1501]: https://gadak.dev/backlog/#/?ks=GDK-1501
[GDK-1502]: https://gadak.dev/backlog/#/?ks=GDK-1502
[GDK-1504]: https://gadak.dev/backlog/#/?ks=GDK-1504
[GDK-1507]: https://gadak.dev/backlog/#/?ks=GDK-1507
[GDK-1508]: https://gadak.dev/backlog/#/?ks=GDK-1508
[GDK-1517]: https://gadak.dev/backlog/#/?ks=GDK-1517
[GDK-1518]: https://gadak.dev/backlog/#/?ks=GDK-1518
[GDK-1520]: https://gadak.dev/backlog/#/?ks=GDK-1520
[GDK-1521]: https://gadak.dev/backlog/#/?ks=GDK-1521
[GDK-1524]: https://gadak.dev/backlog/#/?ks=GDK-1524
[GDK-1525]: https://gadak.dev/backlog/#/?ks=GDK-1525
[GDK-1527]: https://gadak.dev/backlog/#/?ks=GDK-1527
[GDK-1528]: https://gadak.dev/backlog/#/?ks=GDK-1528
[GDK-1529]: https://gadak.dev/backlog/#/?ks=GDK-1529
[GDK-1530]: https://gadak.dev/backlog/#/?ks=GDK-1530
[GDK-1534]: https://gadak.dev/backlog/#/?ks=GDK-1534
[GDK-1535]: https://gadak.dev/backlog/#/?ks=GDK-1535
[GDK-1537]: https://gadak.dev/backlog/#/?ks=GDK-1537
[GDK-1538]: https://gadak.dev/backlog/#/?ks=GDK-1538
[GDK-1541]: https://gadak.dev/backlog/#/?ks=GDK-1541
[GDK-1544]: https://gadak.dev/backlog/#/?ks=GDK-1544
[GDK-1545]: https://gadak.dev/backlog/#/?ks=GDK-1545
[GDK-1546]: https://gadak.dev/backlog/#/?ks=GDK-1546
[GDK-1547]: https://gadak.dev/backlog/#/?ks=GDK-1547
[GDK-1548]: https://gadak.dev/backlog/#/?ks=GDK-1548
[GDK-1549]: https://gadak.dev/backlog/#/?ks=GDK-1549
[GDK-1550]: https://gadak.dev/backlog/#/?ks=GDK-1550
[GDK-1551]: https://gadak.dev/backlog/#/?ks=GDK-1551
[GDK-1552]: https://gadak.dev/backlog/#/?ks=GDK-1552
[GDK-1554]: https://gadak.dev/backlog/#/?ks=GDK-1554
[GDK-1560]: https://gadak.dev/backlog/#/?ks=GDK-1560
[GDK-1561]: https://gadak.dev/backlog/#/?ks=GDK-1561
[GDK-1598]: https://gadak.dev/backlog/#/?ks=GDK-1598
[GDK-1601]: https://gadak.dev/backlog/#/?ks=GDK-1601
[GDK-1604]: https://gadak.dev/backlog/#/?ks=GDK-1604
[GDK-1617]: https://gadak.dev/backlog/#/?ks=GDK-1617
[GDK-1618]: https://gadak.dev/backlog/#/?ks=GDK-1618
[GDK-1622]: https://gadak.dev/backlog/#/?ks=GDK-1622
[GDK-1625]: https://gadak.dev/backlog/#/?ks=GDK-1625
[GDK-1626]: https://gadak.dev/backlog/#/?ks=GDK-1626
[GDK-1633]: https://gadak.dev/backlog/#/?ks=GDK-1633
[GDK-1634]: https://gadak.dev/backlog/#/?ks=GDK-1634
[GDK-1635]: https://gadak.dev/backlog/#/?ks=GDK-1635
[GDK-1636]: https://gadak.dev/backlog/#/?ks=GDK-1636
[GDK-1637]: https://gadak.dev/backlog/#/?ks=GDK-1637
[GDK-1638]: https://gadak.dev/backlog/#/?ks=GDK-1638
[GDK-1639]: https://gadak.dev/backlog/#/?ks=GDK-1639
[GDK-1640]: https://gadak.dev/backlog/#/?ks=GDK-1640
[GDK-1641]: https://gadak.dev/backlog/#/?ks=GDK-1641
[GDK-1644]: https://gadak.dev/backlog/#/?ks=GDK-1644
[GDK-1645]: https://gadak.dev/backlog/#/?ks=GDK-1645
[GDK-1646]: https://gadak.dev/backlog/#/?ks=GDK-1646
[GDK-1647]: https://gadak.dev/backlog/#/?ks=GDK-1647
[GDK-1648]: https://gadak.dev/backlog/#/?ks=GDK-1648
[GDK-1650]: https://gadak.dev/backlog/#/?ks=GDK-1650
[GDK-1651]: https://gadak.dev/backlog/#/?ks=GDK-1651
[GDK-1652]: https://gadak.dev/backlog/#/?ks=GDK-1652
[GDK-1653]: https://gadak.dev/backlog/#/?ks=GDK-1653
[GDK-1654]: https://gadak.dev/backlog/#/?ks=GDK-1654
[GDK-1655]: https://gadak.dev/backlog/#/?ks=GDK-1655
[GDK-1656]: https://gadak.dev/backlog/#/?ks=GDK-1656
[GDK-1657]: https://gadak.dev/backlog/#/?ks=GDK-1657
[GDK-1658]: https://gadak.dev/backlog/#/?ks=GDK-1658
[GDK-1660]: https://gadak.dev/backlog/#/?ks=GDK-1660
[GDK-1661]: https://gadak.dev/backlog/#/?ks=GDK-1661
[GDK-1662]: https://gadak.dev/backlog/#/?ks=GDK-1662
[GDK-1663]: https://gadak.dev/backlog/#/?ks=GDK-1663
[GDK-1665]: https://gadak.dev/backlog/#/?ks=GDK-1665
[GDK-1666]: https://gadak.dev/backlog/#/?ks=GDK-1666
[GDK-1667]: https://gadak.dev/backlog/#/?ks=GDK-1667
[GDK-1672]: https://gadak.dev/backlog/#/?ks=GDK-1672
[GDK-1673]: https://gadak.dev/backlog/#/?ks=GDK-1673
[GDK-1674]: https://gadak.dev/backlog/#/?ks=GDK-1674
[GDK-1677]: https://gadak.dev/backlog/#/?ks=GDK-1677
[GDK-1678]: https://gadak.dev/backlog/#/?ks=GDK-1678
[GDK-1679]: https://gadak.dev/backlog/#/?ks=GDK-1679
[GDK-1680]: https://gadak.dev/backlog/#/?ks=GDK-1680
[GDK-1681]: https://gadak.dev/backlog/#/?ks=GDK-1681
[GDK-1682]: https://gadak.dev/backlog/#/?ks=GDK-1682
[GDK-1683]: https://gadak.dev/backlog/#/?ks=GDK-1683
[GDK-1684]: https://gadak.dev/backlog/#/?ks=GDK-1684
[GDK-1687]: https://gadak.dev/backlog/#/?ks=GDK-1687
[GDK-1689]: https://gadak.dev/backlog/#/?ks=GDK-1689
[GDK-1690]: https://gadak.dev/backlog/#/?ks=GDK-1690
[GDK-1691]: https://gadak.dev/backlog/#/?ks=GDK-1691
[GDK-1692]: https://gadak.dev/backlog/#/?ks=GDK-1692
[GDK-1693]: https://gadak.dev/backlog/#/?ks=GDK-1693
[GDK-1694]: https://gadak.dev/backlog/#/?ks=GDK-1694
[GDK-1695]: https://gadak.dev/backlog/#/?ks=GDK-1695
[GDK-1697]: https://gadak.dev/backlog/#/?ks=GDK-1697
[GDK-1698]: https://gadak.dev/backlog/#/?ks=GDK-1698
[GDK-1700]: https://gadak.dev/backlog/#/?ks=GDK-1700
[GDK-1702]: https://gadak.dev/backlog/#/?ks=GDK-1702
[GDK-1704]: https://gadak.dev/backlog/#/?ks=GDK-1704
[GDK-1705]: https://gadak.dev/backlog/#/?ks=GDK-1705
[GDK-1706]: https://gadak.dev/backlog/#/?ks=GDK-1706
[GDK-1707]: https://gadak.dev/backlog/#/?ks=GDK-1707
[GDK-1709]: https://gadak.dev/backlog/#/?ks=GDK-1709
[GDK-1710]: https://gadak.dev/backlog/#/?ks=GDK-1710
[GDK-1711]: https://gadak.dev/backlog/#/?ks=GDK-1711
[GDK-1712]: https://gadak.dev/backlog/#/?ks=GDK-1712
[GDK-1713]: https://gadak.dev/backlog/#/?ks=GDK-1713
[GDK-1717]: https://gadak.dev/backlog/#/?ks=GDK-1717
[GDK-1720]: https://gadak.dev/backlog/#/?ks=GDK-1720
[GDK-1721]: https://gadak.dev/backlog/#/?ks=GDK-1721
[GDK-1722]: https://gadak.dev/backlog/#/?ks=GDK-1722
[GDK-1723]: https://gadak.dev/backlog/#/?ks=GDK-1723
[GDK-1724]: https://gadak.dev/backlog/#/?ks=GDK-1724
[GDK-1725]: https://gadak.dev/backlog/#/?ks=GDK-1725
[GDK-1726]: https://gadak.dev/backlog/#/?ks=GDK-1726
[GDK-1729]: https://gadak.dev/backlog/#/?ks=GDK-1729
[GDK-1731]: https://gadak.dev/backlog/#/?ks=GDK-1731
[GDK-1732]: https://gadak.dev/backlog/#/?ks=GDK-1732
[GDK-1744]: https://gadak.dev/backlog/#/?ks=GDK-1744
[GDK-1745]: https://gadak.dev/backlog/#/?ks=GDK-1745
[GDK-1746]: https://gadak.dev/backlog/#/?ks=GDK-1746
[GDK-1750]: https://gadak.dev/backlog/#/?ks=GDK-1750
[GDK-1751]: https://gadak.dev/backlog/#/?ks=GDK-1751
[GDK-1753]: https://gadak.dev/backlog/#/?ks=GDK-1753
[GDK-1755]: https://gadak.dev/backlog/#/?ks=GDK-1755
[GDK-1756]: https://gadak.dev/backlog/#/?ks=GDK-1756
[GDK-1757]: https://gadak.dev/backlog/#/?ks=GDK-1757
[GDK-1758]: https://gadak.dev/backlog/#/?ks=GDK-1758
[GDK-1759]: https://gadak.dev/backlog/#/?ks=GDK-1759
[GDK-1760]: https://gadak.dev/backlog/#/?ks=GDK-1760
[GDK-1761]: https://gadak.dev/backlog/#/?ks=GDK-1761
[GDK-1762]: https://gadak.dev/backlog/#/?ks=GDK-1762
[GDK-1769]: https://gadak.dev/backlog/#/?ks=GDK-1769
[GDK-1771]: https://gadak.dev/backlog/#/?ks=GDK-1771
[GDK-1772]: https://gadak.dev/backlog/#/?ks=GDK-1772
[GDK-1775]: https://gadak.dev/backlog/#/?ks=GDK-1775
[GDK-1778]: https://gadak.dev/backlog/#/?ks=GDK-1778
