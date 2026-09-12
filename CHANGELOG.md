# Changelog

<sub>English · <a href="CHANGELOG.ko.md">한국어</a></sub>

## Unreleased

**A self-hosted Jira is an origin type.** `gadak init --site <base-url>
--server` creates a workspace against a self-hosted Jira with a Personal
Access Token: no email, and the base URL may carry a context path. init asks
the site which Jira it is (`/rest/api/2/serverInfo`) and refuses a workspace
whose declared deployment does not match: a Server answer to Cloud's
`/rest/api/3` says nothing about whether that API exists, and the same route
gave 404, 401 and 302 depending only on which credential asked. The REST dialect belongs to the client: Cloud and the
built-in tracker keep v3, a Server origin gets v2, and the endpoints that
differ by more than a version — create metadata, JQL search, the approximate
count, the attachment media route — answer in their own shape or refuse by
name ([GDK-1635], [GDK-1640], [GDK-1636]). Under it, every shape Server sends
differently is read as Server sends it: wiki markup where Cloud sends ADF,
carried verbatim both ways ([GDK-1637]); users keyed by name with a plain
email ([GDK-1638]); attachments at the address Server states, returned
hash-for-hash on Jira Server 11.3.11 and answering `Range` with 206
([GDK-1639]); the sprint field as the Java `toString` of a
bean, `Sprint@4ffcc813[…,id=1,name=Sprint 1,…,state=ACTIVE,…]`, where Cloud
sends an object; the epic in the Epic Link field rather than `fields.parent`;
`/filter/favourite` where `/filter/my` answered 404 on every sync ([GDK-1650],
[GDK-1651], [GDK-1652]); `epic_key` derived from which type has a standard
child, since Server's issue types carry no hierarchy level ([GDK-1658]); and a
board's project key backfilled per board rather than left blank, because
Server answers the board location differently from Cloud ([GDK-1665]). Writes
stopped trusting a polite origin. Server answers a standard issue's `parent`
with 204 and changes nothing, so `edit --parent` sends the Epic Link there,
refuses by name on a Server without Jira Software, and every `edit` compares
the re-read row with what it asked ([GDK-1645]). A login page is no longer
mistaken for an answer: a 2xx of HTML where JSON was asked for is refused by
name — `gadak attach get` had written 257,592 bytes of HTML as a `.png` and
exited 0 — while error pages keep their status and body and Cloud's legitimate
redirect is still followed ([GDK-1648], [GDK-1644]); the guard keys on whether
there is a body, so Server's `POST /issueLink` answering 201 with `text/html`
and nothing in it is the 201 it always was ([GDK-1662]). Data Center's
rate-limit budget is read before the wall instead of after a 429 ([GDK-1646]).
`docs/SUPPORT_MATRIX.md` now reads Jira Cloud, Jira Server, Linear, Built-in,
and every cell in the new column was run — `tools/jira-server-lab/seed.sh`
plants the data and `measure.sh` runs one command per row against a Jira
Software 11.3.11 Data Center lab — with two honest refusals, two limits, and
"untested, therefore unclaimed" gone from the READMEs, the roadmap and the
product spec ([GDK-1634], [GDK-1641]).

**A sprint is an object, and a retrospective is a screen.** A sprint used to
exist only as `sprint_id` / `sprint_name` / `sprint_state` on each issue, so
an empty sprint did not exist and its goal, dates and board existed nowhere.
The mirror has `sprints` and `boards` tables from the Agile API — the one
surface where Cloud and Server answer the same shape — and the `gadak sprint` verbs — list, add, remove, create, start, close —
write through the origin and re-read rather than trusting what was sent ([GDK-1653], [GDK-1654],
[GDK-1655], [GDK-1657]). The tracker gadak carries serves Jira Software's own
Agile surface too — boards, sprints, the sprint field, JQL's `openSprints()`
family — so `gadak sprint` works with no Atlassian account and on a paired
workspace ([GDK-1666]), and Linear's cycles are sprints as well, listed with
one board per team, where `start`, `close` and `create` refuse by name because
a cycle begins and ends by its dates ([GDK-1667], [GDK-1678]). The `sprints`
table is the one owner of a sprint's state, so a closed sprint's finished
issues no longer read "active" forever and inflate every active-sprint query
([GDK-1661]), and each sprint state compiles to its own JQL function instead
of all three becoming `openSprints()` ([GDK-1216]). The board gained a scope
beside the layout switch — the active sprint by name, the backlog, or all —
carried in the URL, undone by the back button, kept by a saved view, with
Sprint and Sprint state axes on the filter bar and `sprint is EMPTY`
round-tripping as `sprint_state=none` ([GDK-1656]); the scope names its axis
rather than reading as sprints only because the demo's sprint is called
"Sprint 42" ([GDK-1682]); the controls appear for the teams that run sprints,
keyed on the board's own type ([GDK-1689]); the active sprint has a line of
its own with name, goal, dates, days left and a server-side progress bar,
where all of that used to live in a tooltip ([GDK-1709]); and a card that has
been through more than one sprint says so ([GDK-1711]). The history was
already in the mirror and unreachable — Jira records every sprint move under
whatever custom-field number the site assigned, so `where field = 'sprint'`
answered nothing — and sync normalises it now, filling `carryover_count`,
`first_sprint_id` and `first_sprint_at`, NULL rather than 0 where the origin
has no changelog ([GDK-1694]). The goal is readable
([GDK-1695]), and the demo mirror's sprints carry one ([GDK-1717]).

`gadak retro`'s document had been served for a surface that never came. The
palette's *Weekly retro* now opens a calm table — one column per week, one row
per metric with its definition underneath, a cell that holds issues a door
onto that list, four, eight or twelve weeks ([GDK-1660]) — and it reads as a
report: the running bucket's four numbers above it with their step from the
bucket before, a sparkline on every row, a delta in every cell, coloured only
where the team agreed which way is better ([GDK-1712]). It opens with a sentence — closed, unplanned, reopened,
oldest in progress, each number a door — and under it what a retrospective is
actually held on: the decisions labelled `retro-action` with
the metric each named then and now ([GDK-1453]), the age of everything in
progress as bars against a p85 line ([GDK-1721]), a per-day density strip
naming the bucket's surprises ([GDK-1722]), what closed by type and by epic
with each cycle time as a dot ([GDK-1723]), the issues you opened that nothing
moved beside the ones that moved unseen ([GDK-1725]), and the full table
folded at the foot ([GDK-1724]); every section unfolds three lines on what it is
and why a retro reads it, and `--explain` prints the same ([GDK-1726]). It can be cut by sprint instead of by ISO week — `gadak retro
--by-sprint`, one column per sprint with the running one marked ([GDK-1693]) —
and offers a board picker where several boards carry sprints instead of "could
not load" ([GDK-1713]). The numbers under it became answerable: the demo
mirror's issue histories had been written seconds apart so every cycle time
read 0.0d, and it carried no reading history at all ([GDK-1720]); an empty
cell says why it is empty and the definitions are no longer cut off
([GDK-1679], [GDK-1681]); the demo mirror has a status catalog derived from
its own issues ([GDK-1680]); the definitions read in the reader's language and
name the session gap ([GDK-1692]); a cell under a day says hours or minutes
instead of `0.0d` ([GDK-1683]); and `status_changed_at` no longer drifts onto
an instant no transition happened ([GDK-1684]). The report knows who "I" am on
the built-in tracker, where a write is attributed to an actor slug and not a
credential ([GDK-1427], [GDK-1729]). Reopens are shown by whether the origin
can answer at all, because "reopened 0" read as "this team has no regressions"
when nobody could tell ([GDK-1690]), and `reopen_count` counts the reopen most
teams actually fire: where resolved statuses sit in the in-progress category,
a done→new-only rule read 59% of real reopens as 0, so the rule keys on the
category the transition leaves and stored rows are recomputed on upgrade
([GDK-1753]). The mismatch row stopped
firing on ordinary Korean ([GDK-1428]), the surprises the CLI prints name the
work rather than only its key ([GDK-1746]), and the demo mirror's issues carry
priority ids so an id-keyed surface can be checked against the one mirror
everybody opens ([GDK-1492], [GDK-1524]). Under all of it the mirror learned
how long an issue was flagged — schema v51 adds `blocked_hours` and
`blocked_since` — and grew two local tables of its own, `sessions` and
`agent_writes` ([GDK-1449], [GDK-1439], [GDK-1440], [GDK-1756], [GDK-1303],
[GDK-1429]); a visit stamps what it saw ([GDK-1451]); and the demo fixture's
wiki pages ride the same time window as its issues ([GDK-1731]).

**The tool says what it knows — to a person, to an agent, and to itself.**
Answers that used to rest on an assumption now come from the tool. `gadak
doctor` prints a `binary` line beside the version: the executable's real path
with the symlink resolved, and the kind of signature on it, `developer-id` for
the released app and `adhoc` for whatever the local toolchain signed — the
pair that separates "the release is broken" from "you are not running the
release" ([GDK-1798], [GDK-1794]). A mirror that is behind says so where an
agent reads: the staleness verdict moved into one owner both callers share, so
the MCP read tools append it to their result instead of a stderr no MCP host
can see ([GDK-599]). The CLI says when its
skill file is behind ([GDK-493], [GDK-1438], [GDK-1215], [GDK-1663],
[GDK-1130], [GDK-1133]). The origin states what it can do, in a capabilities
block ([GDK-1152]). A Confluence space says what it will cost before you
mirror it, and one the origin cannot count draws nothing rather than a zero it
cannot stand behind ([GDK-965]). `gadak doctor` and `gadak status --json` say
whether this workspace mirrors the development panel at all, so an empty
`dev_links` can no longer mean two things ([GDK-1496]). Four verbs stopped
answering a paired workspace with a sentence written for somebody else's
origin — `project create` asked whether the built-in tracker runs *here* and
refused while blaming Jira, a site the workspace does not use ([GDK-1793]).
The built-in origin stopped saying the same kind of thing to itself: a
database somebody else had written under its persist path was refused with
the version message, which told the reader to upgrade a binary that was
already current and to restore a copy nobody had taken; it now names the file
it actually found, and three guarantees — a refusal moves nothing, a forward
migration keeps the issue graph, every refusal names both versions and the
way out — are tests ([GDK-243]).
The audit before this tag
found four places where the tool said something untrue and fixed them at the
source: `gadak transition --field` advertised `(repeatable)` and silently kept
only the last value, so a repeat is now refused by name ([GDK-1804]); the
agent skill named a persist file that does not exist, denied the stored
current workspace `gadak workspace use` sets, and said `dev link` refuses on
a paired workspace where it passes ([GDK-1803]); the
v51 backfill wrote `blocked_hours = 0` — which the docs define as "never
flagged" — onto every row it could not read, and writes NULL now ([GDK-1805]);
a refused dev open migrated `local.db` anyway, so the forward policy moved to
where both files pass ([GDK-1806]); and *Clear history* left two of its four
tables, which one list now drives for both the prune and the clear
([GDK-1807]). And gadak no longer asks GitHub once a day whether a newer release exists: no
background check, no `updateCheck` setting, no sidebar banner, outbound
destinations from six to five, and `docs/PROMISES.md` drops the claim that
checked it ([GDK-1626]). That document was rewritten as four questions instead
of a numbered inventory — does it report on me, can I get my data back out,
what can reach it while it runs, when does it go to the network — and gained a
twelfth claim about *when*: a read verb answers from the disk and opens no
socket, verified with the transport and the DNS resolver replaced by hooks
that fail on use ([GDK-1792]). The
front door says what gadak is before it says how fast it is — the landing
heading in every language is a job, "Query your Jira backlog with SQL.", and
the trust section became the facts a reader needs before handing over a token
([GDK-1601], [GDK-1622]). It also answers the question a reader arrives with:
"why not the official Rovo MCP?" had been the site's sixth section and line 222
of the README, so the comparison now sits directly under the hero and carries
the two axes it was missing, rate limits and who maintains it, while all three
READMEs answer it in their opening — Korean and Japanese, which had no
comparison at all, gained one ([GDK-1824]). Then the first sync got cheaper and stopped being
waited on: a pass ends with one stderr line of requests by kind and `gadak api
--headers` prints every response header ([GDK-1672]); the Confluence pass runs
through a bounded pool, `gadak sync --concurrency`, default 4, at most 8
([GDK-1673]); and the documented first run is `gadak init && gadak serve`,
where the window opens while the mirror fills newest-first under a band
reading `Recent issues first · 1,200 / 3,514 · wiki next` ([GDK-1677]). A dev
build stopped deciding for the installed release ([GDK-1687], [GDK-1697]), and
the service installer validates serve flags at install time rather than
crash-looping under KeepAlive ([GDK-1267]).

To an agent, the surface caught up with the product. `gadak comment edit <KEY>
<ID> -m "…"` replaces a comment's body and `gadak comment rm <KEY> <ID>
--yes` removes it, on Jira, Linear and the built-in tracker, taking whatever
id a read handed you — `gadak sql` prints `jira:91653`, `gadak issue` prints
`91653`, both accepted ([GDK-1647]). Every write verb takes `--dry-run`: one
JSON line carrying the ids the resolutions found, and nothing reaches the
origin. `gadak link KEY <url> --title` writes a remote link through the origin
([GDK-530]); repeating `--field` for one alias adds a value instead of
replacing the last one, the silent drop the rest of the CLI refuses
([GDK-18]); the create dialog fills what the origin requires and keeps Create
disabled rather than sending a create Jira will reject ([GDK-533]); and a
comment can be restricted where the origin has restrictions ([GDK-528]). On
the built-in tracker attachment bytes live beside the database, one
content-addressed file each, streaming both ways, with the cap now `gadak
config set attachmentMaxMB <n>`, default 1 GiB, up from a hard-coded 32 MiB
([GDK-1617]), and uploads carry the type their filename says rather than
`application/octet-stream`, with `gadak backup` a `.tar` that refuses to write
one with attachments missing ([GDK-1277]). An optional capability no longer
disappears when the actor trailer is on ([GDK-1655]). `gadak mcp install
claude-desktop` registers with Claude Desktop, and `gadak mcp install claude`
says what it is — every front door had taught the Desktop user a command that
runs Claude *Code*'s `claude mcp add`, which Desktop never reads ([GDK-1633]).
An agent with no shell can read and set the design tokens ([GDK-769]), read
the week with `gadak_retro` ([GDK-1404]), and leaves a trail doing it:
`gadak_issue` and `gadak_search` record the visit and the search the way the
CLI's do, and `gadak_recents` walks it back, where a host with no shell used to
have zero rows after a compaction ([GDK-631]). The skill asks for a URL where
it used to get prose, because `main 1693107` records the fact in a shape
nothing can read ([GDK-529]). The session roster says what each shell is
doing, reading the window title a shell sets for itself through the OSC
scanner that already had to know where such a string ends ([GDK-1389]). The
secret scanner covers the shapes it always claimed to: `internal/secretscan`
owned seven patterns while the script pointing it at the repository grepped
for two, so a real `ghp_` token in a fixture left the gate green ([GDK-1110],
[GDK-1797]). The settings dialog got the gate its registry never had
([GDK-9]). The public-backlog export refuses a kept description carrying a
credential-shaped string ([GDK-1260]). Two doors onto one write stopped
disagreeing: `gadak link KEY <url>` and `gadak ref KEY <url>` mint the same
remote link, so both now take `--title`, `--as` and `--dry-run` — the plans
are byte-identical — and `gadak unlink KEY <url>` removes what `link` created,
so the verb that makes a remote link unmakes it ([GDK-1816]). An agent with no
shell can say which mirror answered: `gadak_status` carries `workspace`,
`workspace_source` and `actor` with the CLI's own field names ([GDK-1813]),
and every tool refuses an unknown argument by name, where two did and seven
quietly answered a different question because the published contract and the
enforced one were separate objects ([GDK-1812]). `gadak sprint show` reached
the skill, the tool descriptions and the top-level help, which none of them
knew ([GDK-1814]) — and then the burn-up itself reached MCP as `gadak_sprint`,
the one answer in that theme `gadak_query` could not stand in for, because the
daily series is replayed from the changelog rather than stored ([GDK-1826]).

To itself: before the version was cut, the whole tree was read twice. The
audit now starts from a census rather than a reading — five scripts under
`tools/audit/`, each printing its own source commands, with a contract test
doc-checks runs ([GDK-1707], [GDK-1206]) — and the first pass found things the
release would otherwise have shipped: what landed since v0.21.0 reached the
skill and the MCP tools ([GDK-1700]); twenty-four English strings that had
escaped the catalog became keys in all three languages ([GDK-1704]); ten
places where the docs and the site disagreed with the code were fixed at the
copy ([GDK-1705]); the retro table takes the width it needs ([GDK-1706]); and
the CI run got cheaper, with the static-analysis gate skipping a push that
touches no Go and the browser shards dealt by measured seconds instead of file
order ([GDK-1702], [GDK-1698]). The reading rounds followed. The Go tree lost
code that lived in the wrong room ([GDK-688], [GDK-689], [GDK-718],
[GDK-1113], [GDK-1245], [GDK-1319], [GDK-1332]); the server stopped paying per
request for things it could know once ([GDK-1674], [GDK-1547], [GDK-1004],
[GDK-307], [GDK-1413], [GDK-978], [GDK-1429], [GDK-936], [GDK-954]); the gates
got cheaper and stopped drifting apart ([GDK-1227], [GDK-1488], [GDK-1759],
[GDK-1107], [GDK-1105], [GDK-1108], [GDK-1485], [GDK-1546], [GDK-1518],
[GDK-1554]); the web tree lost its last document-wide lookups and its last
effects that read what they write ([GDK-645], [GDK-693], [GDK-630],
[GDK-696], [GDK-941], [GDK-942], [GDK-698], [GDK-829], [GDK-1324],
[GDK-1326], [GDK-1732], [GDK-1187], [GDK-1101], [GDK-1455], [GDK-1135]); the
test pyramid moved weight down a rung ([GDK-1757], [GDK-723], [GDK-724],
[GDK-725], [GDK-1327], [GDK-1328], [GDK-1147], [GDK-1502], [GDK-1758]); the
demo fixture exercises what the code maps ([GDK-1755], [GDK-114], [GDK-45],
[GDK-25], [GDK-1349]); the CLI and the skill lost duplicate vocabulary
([GDK-1534], [GDK-1535], [GDK-1545], [GDK-1520], [GDK-1528], [GDK-136],
[GDK-1588], [GDK-1242], [GDK-976], [GDK-487]); the documentation gained gates
where it had habits ([GDK-670], [GDK-1288], [GDK-524], [GDK-1003],
[GDK-1625], [GDK-1604]); `doctor` says what the store already knew and a
paired serve says which version it is ([GDK-1549], [GDK-596], [GDK-1273],
[GDK-768], [GDK-1760], [GDK-722]); the phone's network boundary moved into
Rust ([GDK-897], [GDK-875], [GDK-890], [GDK-952], [GDK-1525], [GDK-1550],
[GDK-1552], [GDK-1529], [GDK-1530], [GDK-1551]); five pieces of
infrastructure stopped trusting luck ([GDK-1761], [GDK-783], [GDK-506],
[GDK-234], [GDK-1407]); and thirty-three Go identifiers nothing outside their
package used went lowercase, with three web modules dropping exports nothing
imports ([GDK-1141], [GDK-1232]). Then a second pass closed what the first had
only measured — `gadak workspace export` writes version 2 now, carrying the
visit and search history the cache rule excuses ([GDK-1762], [GDK-1769],
[GDK-1778], [GDK-1771], [GDK-1775], [GDK-1772]), took the complexity census at its word, remaking `gadak migrate` to Jira
and to Linear as a staged pipeline ([GDK-1774], [GDK-1780], [GDK-1779], [GDK-1777], [GDK-1773], [GDK-1781]) —
put the test suite itself on the list ([GDK-1782], [GDK-1785], [GDK-1786],
[GDK-1783], [GDK-1776], [GDK-1764]), and, on the surfaces, gave the terminal
pane and the phone's shell one socket driver where they had shared a
ninety-line skeleton by copy ([GDK-1767], [GDK-1768], [GDK-1765], [GDK-1788],
[GDK-1763], [GDK-1787]). Three grammars became one — the `gadak://` pointer
had a parser in the CLI and another in the server ([GDK-1316]), the Cloud
category fold had a hand-written copy beside it ([GDK-1315]), the page-id case
policy differed between the store and sync ([GDK-1104]) — each held there now
by a gate that names any second copy, as are three more duplicate
implementations ([GDK-927], [GDK-928], [GDK-1320]) and four lists that
described the code ([GDK-1482], [GDK-832], [GDK-921], [GDK-923], [GDK-1483]).
A folded group resolves the same way from the CLI, a REST write and `gadak
claim` ([GDK-1521]). The test harness stopped trusting a port number or a side
file ([GDK-1789], [GDK-1555]), the scheme test asks the artifact ([GDK-919]),
and three e2e cases stopped re-proving a unit ([GDK-720]).

The app itself grew up at the edges. Under 900 pixels there is now one narrow
regime: the sidebar's narrow width had been redeclared in five places under a
760-pixel media query, so an 800-pixel window kept a 272-pixel sidebar and
squeezed the list into what was left; the step sits at 899 and the terminal
overlay shares the boundary ([GDK-1369], [GDK-1091]), while the shell's height
falls through `100vh`, `100dvh`, `100svh` so an in-app tab bar no longer eats
the bottom row ([GDK-54]). The terminal's shape stopped being the window's
to decide: the dock became that sheet whenever a detail panel was open under
1420 pixels, a floor derived when the pane was a column in the list's own row
and left standing when the pane moved to a band underneath, so opening an
issue on a laptop covered the list the terminal was there to drive
([GDK-1833]) — and width now only picks the default, since the roster
header's control and the palette pin either shape, the choice survives a
resize that would have chosen the other, and in the full shape the session
rows are a block in the app sidebar rather than a second rail beside the pane
([GDK-1835]). The list reads at every width it is given: the
furniture on the right takes its fixed widths first, so a window 30% narrower
used to cost the title 60% of its width and clip mid-verb at 1000px — the
strip is now priced against a width at which a title is still a title, 222px
to 302px at 1000, 166px to 336px at 800, and 1440 unchanged to the pixel
([GDK-1791]) — and a label chip that cannot show six characters folds into the
`+N` badge ([GDK-1744]). The seam between columns is a grip, writing the same
two tokens `gadak config set ui.tokens.layout.sidebar 300px` writes, so the
pointer and the CLI are one value in one file ([GDK-759]); the grips are
reachable from the palette, which had been about 150 tab stops deep
([GDK-1796]); the terminal dock's grip is now that same grip — arrow keys
resize it, Shift for a single pixel, Backspace puts it back, a screen reader
hears a slider with a live value, and the target is hand-sized rather than
4px — and the gate certifying that every draggable seam has a keyboard door
now reads a registry that can see all three of them, where its axis list had
named only the two that already did ([GDK-1815]); and the list column
gained a width of its own, `ui.tokens.layout.list`
([GDK-769]). Reading the Korean and Japanese clips frame by
frame before cutting them found what copy review had not: `Backlog` and
`Selected for Development` stood in English one line above 진행 중, because
the built-in tracker's translated catalogs spelled every status id but those
two; the compact relative time shared its key with the long one, so たった今
wrapped to two lines in a column priced for `2d`; the same English word was
`内蔵` on one screen and `組み込み` on the next; and the sidebar's Documents
section held a row called Documents in all three languages ([GDK-1837]). A
second pass over the corrected frames found the screen telling the same moment
two ways — an issue whose status said 진행 중 carried `0s` beside a column
calling it 방금, because both duration formatters collapsed a sub-second span
to zero rather than saying it was under a second — a filter menu too narrow to
show the value it was opened to choose, and one English word translated two
ways in Japanese, `未担当` where the rest of the product says `未割り当て`
([GDK-1839]). The
posters went the same way — the still a reader meets before pressing play was
cut at a timestamp measured once on an English take, and both localized clips
had been shipping a blank one; the frame is now chosen by looking for ink in
it, and an export that cannot find any refuses ([GDK-1836]). The clips
themselves started there too: four exporters cut their head at a constant, and
measuring the first inked frame per take put it at 2.56s, 2.60s and 2.72s for
the three languages against a 2.4 that was right for none of them, while the
terminal's 2.2 had been cutting away the whole two-second beat the clip opens
on ([GDK-1838]). A fifth constant from that same take set where the camera pulls
back out, and the landing loops these clips, so all three languages snapped at
the loop point — Japanese ended a fifth of the way through the movement with the
sidebar sliced down its middle, Korean never reached the cue and ended zoomed,
and only English happened not to look broken doing it; the pull-out is now
counted back from the end of the take, and an export whose last frames are still
moving refuses ([GDK-1841]). Reading the frames also found the recording aiming
at one row while another was picked: the pointer keeps wherever the last click
left it, and the exclude affordance is drawn on hover, so a beat that never
hovered held ⊘ over Highest for six tenths of a second and then checked High
([GDK-1842]). The frame a crop keeps is the other half of this: framing the
search palette meant cropping past the sidebar, but the palette begins 47 pixels
before the sidebar ends, so the offset that finally excluded the sidebar took
those pixels off the palette instead and cut every key in a clip whose subject is
finding things — `NMB-74` read `IB-74`, and in Korean the query someone had typed
read `시 방편`. There is no offset that holds both; the clip ships the frame that
was recorded, and an export that is not the size of its take refuses
([GDK-1839]). The view-settings menu scrolled as one piece, so an 80vh cap landed
inside a column row at the window heights a laptop actually has — the catalog
scrolls on its own now and stops on a row boundary, the rule a dashboard list
already followed ([GDK-1840], [GDK-1745]). One header was still reading `1,775`
and `1069` in the same row, the count [GDK-1560] grouped everywhere except the
one place nothing had looked. Twenty-one sites
spelling the same four utilities became one
`.section-label` recipe, and Korean and Japanese keep their case ([GDK-141]);
the QA teal became a `--color-status-qa` token from the avatar family
([GDK-160]); the palette keeps its icon rail even for rows without one
([GDK-143]); the settings tabs are a tablist where nine plain buttons told a
screen reader nothing ([GDK-138], [GDK-1134]); the palette knows where the
main column can go ([GDK-137], [GDK-732]); three places the keyboard could not
reach are reachable ([GDK-734], [GDK-733], [GDK-1093]); deleting a saved view
and opening the Jira filter show on focus as well as hover ([GDK-728]); the
overlays carry the same back arrow ([GDK-729]); a piece of chrome looks like
what it is ([GDK-142]); two totals of different scope stopped sitting side by
side ([GDK-1092]); the sub-issue rollup's *Show completed* box is drawn only
when there are completed children to draw it for ([GDK-1795]); a count is
grouped for the locale wherever it appears ([GDK-1560]); the dashboard frame
carries the app's background rather than white ([GDK-1598]); an exact issue key
in the palette resolves to that row ([GDK-1255]); a dashboard list no longer
ends in a row cut through its glyphs ([GDK-1745]); and the clipboard
fallback's premise is scoped to the build it was measured on ([GDK-1114]),
with the scoped-token hint on a 401 already there ([GDK-73]). The settings
window stopped saying "mirror" to the person reading it ([GDK-1286],
[GDK-1285], [GDK-1629], [GDK-1268], [GDK-1494]) and onboarding calls the local
copy a cache throughout ([GDK-1323]) — in Korean and Japanese the freshness
chip beside it says so too, where it had been the one thing on that screen
still using the other word ([GDK-1817]); personal history became something you
can see and clear ([GDK-106], [GDK-1752], [GDK-957], [GDK-1183]). On the
phone, the controls live in the catalog rather than the markup ([GDK-1150]),
the detail header gained a share button ([GDK-877], [GDK-1136]), the terminal
session sheet opens with the last issue's key filled in ([GDK-1527],
[GDK-911]), a refusal says which refusal it was ([GDK-1121]), a copy that
fails raises a toast ([GDK-1504]), what you read lands in the same visit
history the window writes ([GDK-1538]), and three more things moved
([GDK-803], [GDK-804], [GDK-873]). Wiki page attachments reach the mirror
([GDK-1750], [GDK-1541]); a comment that quotes CSS no longer asks the origin
who `@media` is ([GDK-1125], [GDK-1544], [GDK-21]); search finds labels and
word forms ([GDK-1021]); three sync corrections landed ([GDK-1507],
[GDK-1457], [GDK-1490]); `gadak migrate` no longer holds the archive in memory
([GDK-1618]) and stopped swallowing two failures ([GDK-1318]); there is a way
out of the built-in tracker that deletes nothing ([GDK-378]); the pairing
dialog decides a loopback refusal from the error's type ([GDK-1317]); the
session strip's boundary has one seat ([GDK-1548], [GDK-1561], [GDK-1132],
[GDK-1691], [GDK-1710]); the demo fixture has one content original and one
artifact ([GDK-1751], [GDK-1517], [GDK-47]); the e2e suite serves a
Linear-shaped mirror beside the Jira one ([GDK-1298]); and the Raycast
extension gadak installs stopped copying an install command the tap does not
answer and stopped rendering nothing for a search that matched nothing
([GDK-1800]).
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

[GDK-9]: https://gadak.dev/backlog/#/?ks=GDK-9
[GDK-18]: https://gadak.dev/backlog/#/?ks=GDK-18
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
[GDK-106]: https://gadak.dev/backlog/#/?ks=GDK-106
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
[GDK-243]: https://gadak.dev/backlog/#/?ks=GDK-243
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
[GDK-528]: https://gadak.dev/backlog/#/?ks=GDK-528
[GDK-529]: https://gadak.dev/backlog/#/?ks=GDK-529
[GDK-530]: https://gadak.dev/backlog/#/?ks=GDK-530
[GDK-531]: https://gadak.dev/backlog/#/?ks=GDK-531
[GDK-532]: https://gadak.dev/backlog/#/?ks=GDK-532
[GDK-533]: https://gadak.dev/backlog/#/?ks=GDK-533
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
[GDK-631]: https://gadak.dev/backlog/#/?ks=GDK-631
[GDK-635]: https://gadak.dev/backlog/#/?ks=GDK-635
[GDK-643]: https://gadak.dev/backlog/#/?ks=GDK-643
[GDK-645]: https://gadak.dev/backlog/#/?ks=GDK-645
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
[GDK-759]: https://gadak.dev/backlog/#/?ks=GDK-759
[GDK-766]: https://gadak.dev/backlog/#/?ks=GDK-766
[GDK-768]: https://gadak.dev/backlog/#/?ks=GDK-768
[GDK-769]: https://gadak.dev/backlog/#/?ks=GDK-769
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
[GDK-877]: https://gadak.dev/backlog/#/?ks=GDK-877
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
[GDK-936]: https://gadak.dev/backlog/#/?ks=GDK-936
[GDK-941]: https://gadak.dev/backlog/#/?ks=GDK-941
[GDK-942]: https://gadak.dev/backlog/#/?ks=GDK-942
[GDK-944]: https://gadak.dev/backlog/#/?ks=GDK-944
[GDK-946]: https://gadak.dev/backlog/#/?ks=GDK-946
[GDK-947]: https://gadak.dev/backlog/#/?ks=GDK-947
[GDK-950]: https://gadak.dev/backlog/#/?ks=GDK-950
[GDK-952]: https://gadak.dev/backlog/#/?ks=GDK-952
[GDK-954]: https://gadak.dev/backlog/#/?ks=GDK-954
[GDK-956]: https://gadak.dev/backlog/#/?ks=GDK-956
[GDK-957]: https://gadak.dev/backlog/#/?ks=GDK-957
[GDK-960]: https://gadak.dev/backlog/#/?ks=GDK-960
[GDK-963]: https://gadak.dev/backlog/#/?ks=GDK-963
[GDK-964]: https://gadak.dev/backlog/#/?ks=GDK-964
[GDK-965]: https://gadak.dev/backlog/#/?ks=GDK-965
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
[GDK-1136]: https://gadak.dev/backlog/#/?ks=GDK-1136
[GDK-1141]: https://gadak.dev/backlog/#/?ks=GDK-1141
[GDK-1147]: https://gadak.dev/backlog/#/?ks=GDK-1147
[GDK-1149]: https://gadak.dev/backlog/#/?ks=GDK-1149
[GDK-1150]: https://gadak.dev/backlog/#/?ks=GDK-1150
[GDK-1152]: https://gadak.dev/backlog/#/?ks=GDK-1152
[GDK-1158]: https://gadak.dev/backlog/#/?ks=GDK-1158
[GDK-1172]: https://gadak.dev/backlog/#/?ks=GDK-1172
[GDK-1174]: https://gadak.dev/backlog/#/?ks=GDK-1174
[GDK-1175]: https://gadak.dev/backlog/#/?ks=GDK-1175
[GDK-1176]: https://gadak.dev/backlog/#/?ks=GDK-1176
[GDK-1180]: https://gadak.dev/backlog/#/?ks=GDK-1180
[GDK-1182]: https://gadak.dev/backlog/#/?ks=GDK-1182
[GDK-1183]: https://gadak.dev/backlog/#/?ks=GDK-1183
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
[GDK-1268]: https://gadak.dev/backlog/#/?ks=GDK-1268
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
[GDK-1303]: https://gadak.dev/backlog/#/?ks=GDK-1303
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
[GDK-1389]: https://gadak.dev/backlog/#/?ks=GDK-1389
[GDK-1390]: https://gadak.dev/backlog/#/?ks=GDK-1390
[GDK-1391]: https://gadak.dev/backlog/#/?ks=GDK-1391
[GDK-1394]: https://gadak.dev/backlog/#/?ks=GDK-1394
[GDK-1395]: https://gadak.dev/backlog/#/?ks=GDK-1395
[GDK-1396]: https://gadak.dev/backlog/#/?ks=GDK-1396
[GDK-1398]: https://gadak.dev/backlog/#/?ks=GDK-1398
[GDK-1399]: https://gadak.dev/backlog/#/?ks=GDK-1399
[GDK-1400]: https://gadak.dev/backlog/#/?ks=GDK-1400
[GDK-1401]: https://gadak.dev/backlog/#/?ks=GDK-1401
[GDK-1404]: https://gadak.dev/backlog/#/?ks=GDK-1404
[GDK-1407]: https://gadak.dev/backlog/#/?ks=GDK-1407
[GDK-1413]: https://gadak.dev/backlog/#/?ks=GDK-1413
[GDK-1427]: https://gadak.dev/backlog/#/?ks=GDK-1427
[GDK-1428]: https://gadak.dev/backlog/#/?ks=GDK-1428
[GDK-1429]: https://gadak.dev/backlog/#/?ks=GDK-1429
[GDK-1438]: https://gadak.dev/backlog/#/?ks=GDK-1438
[GDK-1439]: https://gadak.dev/backlog/#/?ks=GDK-1439
[GDK-1440]: https://gadak.dev/backlog/#/?ks=GDK-1440
[GDK-1449]: https://gadak.dev/backlog/#/?ks=GDK-1449
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
[GDK-1494]: https://gadak.dev/backlog/#/?ks=GDK-1494
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
[GDK-1555]: https://gadak.dev/backlog/#/?ks=GDK-1555
[GDK-1560]: https://gadak.dev/backlog/#/?ks=GDK-1560
[GDK-1561]: https://gadak.dev/backlog/#/?ks=GDK-1561
[GDK-1588]: https://gadak.dev/backlog/#/?ks=GDK-1588
[GDK-1598]: https://gadak.dev/backlog/#/?ks=GDK-1598
[GDK-1601]: https://gadak.dev/backlog/#/?ks=GDK-1601
[GDK-1604]: https://gadak.dev/backlog/#/?ks=GDK-1604
[GDK-1617]: https://gadak.dev/backlog/#/?ks=GDK-1617
[GDK-1618]: https://gadak.dev/backlog/#/?ks=GDK-1618
[GDK-1622]: https://gadak.dev/backlog/#/?ks=GDK-1622
[GDK-1625]: https://gadak.dev/backlog/#/?ks=GDK-1625
[GDK-1626]: https://gadak.dev/backlog/#/?ks=GDK-1626
[GDK-1629]: https://gadak.dev/backlog/#/?ks=GDK-1629
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
[GDK-1752]: https://gadak.dev/backlog/#/?ks=GDK-1752
[GDK-1753]: https://gadak.dev/backlog/#/?ks=GDK-1753
[GDK-1755]: https://gadak.dev/backlog/#/?ks=GDK-1755
[GDK-1756]: https://gadak.dev/backlog/#/?ks=GDK-1756
[GDK-1757]: https://gadak.dev/backlog/#/?ks=GDK-1757
[GDK-1758]: https://gadak.dev/backlog/#/?ks=GDK-1758
[GDK-1759]: https://gadak.dev/backlog/#/?ks=GDK-1759
[GDK-1760]: https://gadak.dev/backlog/#/?ks=GDK-1760
[GDK-1761]: https://gadak.dev/backlog/#/?ks=GDK-1761
[GDK-1762]: https://gadak.dev/backlog/#/?ks=GDK-1762
[GDK-1763]: https://gadak.dev/backlog/#/?ks=GDK-1763
[GDK-1764]: https://gadak.dev/backlog/#/?ks=GDK-1764
[GDK-1765]: https://gadak.dev/backlog/#/?ks=GDK-1765
[GDK-1767]: https://gadak.dev/backlog/#/?ks=GDK-1767
[GDK-1768]: https://gadak.dev/backlog/#/?ks=GDK-1768
[GDK-1769]: https://gadak.dev/backlog/#/?ks=GDK-1769
[GDK-1771]: https://gadak.dev/backlog/#/?ks=GDK-1771
[GDK-1772]: https://gadak.dev/backlog/#/?ks=GDK-1772
[GDK-1773]: https://gadak.dev/backlog/#/?ks=GDK-1773
[GDK-1774]: https://gadak.dev/backlog/#/?ks=GDK-1774
[GDK-1775]: https://gadak.dev/backlog/#/?ks=GDK-1775
[GDK-1776]: https://gadak.dev/backlog/#/?ks=GDK-1776
[GDK-1777]: https://gadak.dev/backlog/#/?ks=GDK-1777
[GDK-1778]: https://gadak.dev/backlog/#/?ks=GDK-1778
[GDK-1779]: https://gadak.dev/backlog/#/?ks=GDK-1779
[GDK-1780]: https://gadak.dev/backlog/#/?ks=GDK-1780
[GDK-1781]: https://gadak.dev/backlog/#/?ks=GDK-1781
[GDK-1782]: https://gadak.dev/backlog/#/?ks=GDK-1782
[GDK-1783]: https://gadak.dev/backlog/#/?ks=GDK-1783
[GDK-1785]: https://gadak.dev/backlog/#/?ks=GDK-1785
[GDK-1786]: https://gadak.dev/backlog/#/?ks=GDK-1786
[GDK-1787]: https://gadak.dev/backlog/#/?ks=GDK-1787
[GDK-1788]: https://gadak.dev/backlog/#/?ks=GDK-1788
[GDK-1789]: https://gadak.dev/backlog/#/?ks=GDK-1789
[GDK-1791]: https://gadak.dev/backlog/#/?ks=GDK-1791
[GDK-1792]: https://gadak.dev/backlog/#/?ks=GDK-1792
[GDK-1793]: https://gadak.dev/backlog/#/?ks=GDK-1793
[GDK-1794]: https://gadak.dev/backlog/#/?ks=GDK-1794
[GDK-1795]: https://gadak.dev/backlog/#/?ks=GDK-1795
[GDK-1796]: https://gadak.dev/backlog/#/?ks=GDK-1796
[GDK-1797]: https://gadak.dev/backlog/#/?ks=GDK-1797
[GDK-1798]: https://gadak.dev/backlog/#/?ks=GDK-1798
[GDK-1800]: https://gadak.dev/backlog/#/?ks=GDK-1800
[GDK-1803]: https://gadak.dev/backlog/#/?ks=GDK-1803
[GDK-1804]: https://gadak.dev/backlog/#/?ks=GDK-1804
[GDK-1805]: https://gadak.dev/backlog/#/?ks=GDK-1805
[GDK-1806]: https://gadak.dev/backlog/#/?ks=GDK-1806
[GDK-1807]: https://gadak.dev/backlog/#/?ks=GDK-1807
[GDK-1812]: https://gadak.dev/backlog/#/?ks=GDK-1812
[GDK-1813]: https://gadak.dev/backlog/#/?ks=GDK-1813
[GDK-1814]: https://gadak.dev/backlog/#/?ks=GDK-1814
[GDK-1815]: https://gadak.dev/backlog/#/?ks=GDK-1815
[GDK-1816]: https://gadak.dev/backlog/#/?ks=GDK-1816
[GDK-1817]: https://gadak.dev/backlog/#/?ks=GDK-1817
[GDK-1824]: https://gadak.dev/backlog/#/?ks=GDK-1824
[GDK-1826]: https://gadak.dev/backlog/#/?ks=GDK-1826
[GDK-1833]: https://gadak.dev/backlog/#/?ks=GDK-1833
[GDK-1835]: https://gadak.dev/backlog/#/?ks=GDK-1835
[GDK-1836]: https://gadak.dev/backlog/#/?ks=GDK-1836
[GDK-1837]: https://gadak.dev/backlog/#/?ks=GDK-1837
[GDK-1838]: https://gadak.dev/backlog/#/?ks=GDK-1838
[GDK-1839]: https://gadak.dev/backlog/#/?ks=GDK-1839
[GDK-1840]: https://gadak.dev/backlog/#/?ks=GDK-1840
[GDK-1841]: https://gadak.dev/backlog/#/?ks=GDK-1841
[GDK-1842]: https://gadak.dev/backlog/#/?ks=GDK-1842
