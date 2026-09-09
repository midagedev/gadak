# Origin support matrix

One table answers "does this work on my origin?". A gadak workspace has
exactly one origin — **Jira** (Atlassian Cloud), **Linear**, or **Built-in**
(the tracker that travels with the app, from 0.16). The mirror is a cache on
all three; every write passes through the origin and the mirror row is
re-read after it lands.

Every cell carries a footnote pointing at the code that makes it true — a
`path:line` in this repository, or a line in the Built-in origin's
compatibility inventory (module
`github.com/midagedev/issuetap@v0.0.0-20260908083721-0d34fcd95793`, cited
below as `issuetap/docs/COMPATIBILITY.md`). A Built-in cell is never "same
as Jira": it means the Jira REST verb exists and the Built-in origin
implements the route.

Jira **Server / Data Center** is the fourth origin type (`gadak init
--server`, a base URL and a Personal Access Token; GDK-1635). Its column was
measured row by row on a Jira Software 11.3.11 Data Center instance seeded
for exactly that: `tools/jira-server-lab/seed.sh` plants the data every row
needs (an epic with epic links, a sub-task, components, versions, a blocking
link, a remote link, an attachment, a role-restricted comment, a worklog,
transitions through done and a reopen, three sprints in the three states,
and hundreds of plain issues for the rate-limit run), and
`tools/jira-server-lab/measure.sh` runs one command per row and keeps the
output; `docs/runbooks/jira-server-lab.md` brings the instance up. Where a
Server cell shares a footnote with the Cloud one, the code path is the same
and the measurement said so; a Server-only footnote names the dialect
difference and the commit that closed it. Confluence Server has no client
here (GDK-1664), so the two wiki rows are refusals.

Markers:

- ✅ — works
- ◐ — works, with a limitation the footnote names
- — — not on this origin; gadak refuses with a sentence rather than
  half-applying

| Capability | Jira Cloud | Jira Server | Linear | Built-in |
| --- | --- | --- | --- | --- |
| **Read** · issue sync | ✅[^1] | ✅[^117] | ✅[^2] | ✅[^3] |
| **Read** · full-text search (`gadak search`) | ✅[^4] | ✅[^4] | ✅[^4] | ✅[^4] |
| **Read** · SQL (`gadak sql`, `issues_full` + RECIPES) | ✅[^5] | ✅[^5] | ✅[^6] | ✅[^5] |
| **Read** · `--jql` / pasted Jira URL | ✅[^7] | ✅[^7] | ✅[^8] | ✅[^7] |
| **Read** · comments | ✅[^9] | ✅[^118] | ✅[^10] | ✅[^9] |
| **Read** · attachment bytes | ✅[^11] | ✅[^119] | ✅[^12] | ✅[^13] |
| **Read** · attachment download (`gadak attach get`) | ✅[^110] | ✅[^119] | ✅[^111] | ✅[^112] |
| **Read** · history → `status_changed_at`, `reopen_count`, `started_at` / `cycle_hours` (time-in-status computed, never stored) | ✅[^14] | ✅[^120] | ◐[^15] | ✅[^16] |
| **Read** · issue links | ✅[^17] | ✅[^17] | ✅[^106] | ✅[^19] |
| **Read** · `gadak ready` / `open_blockers` — blocking-link catalog | ✅[^107] | ✅[^107] | ✅[^108] | ✅[^109] |
| **Read** · remote issue links / cross-workspace refs (`ref`) | —[^20] | —[^20] | —[^20] | ◐[^21] |
| **Read** · development-panel links (`dev`) | ◐[^22] | ◐[^121] | —[^23] | ✅[^24] |
| **Read** · labels | ✅[^25] | ✅[^25] | ✅[^26] | ✅[^25] |
| **Read** · components | ✅[^25] | ✅[^25] | —[^27] | ✅[^28] |
| **Read** · fix versions + `versions` catalog | ✅[^29] | ✅[^29] | —[^30] | ✅[^31] |
| **Read** · sprints (columns `sprint_id`/`sprint_name`/`sprint_state`) | ✅[^32] | ✅[^32] | ✅[^33] | ✅[^139] |
| **Read** · boards + sprints as rows (`boards`, `sprints`, `gadak sprint list`) | ✅[^135] | ◐[^122] | ✅[^141] | ✅[^139] |
| **Read** · sprint carry-over (`carryover_count`, `first_sprint_id` / `first_sprint_at`) | ✅[^143] | ✅[^143] | —[^144] | ✅[^145] |
| **Read** · retro by sprint (`gadak retro --by-sprint`, `?by=sprint`) | ✅[^146] | ✅[^146] | ✅[^146] | ✅[^146] |
| **Write** · sprint — add / remove / create / start / close | ✅[^136] | ✅[^123] | ◐[^142] | ✅[^140] |
| **Read** · custom fields (`fields --apply`) | ✅[^35] | ✅[^124] | —[^36] | ◐[^37] |
| **Read** · issue type | ✅[^38] | ✅[^38] | —[^39] | ✅[^40] |
| **Read** · hierarchy — `parent_key` / `epic_key` | ✅[^41] | ✅[^125] | ◐[^42] | ✅[^43] |
| **Read** · wiki pages | ✅[^44] | —[^126] | —[^45] | ✅[^46] |
| **Read** · origin web URL — `gadak open`, web key anchor, copy link | ✅[^47] | ✅[^47] | ✅[^48] | ◐[^49] |
| **Read** · view link — toolbar / palette "Copy link to this view" | ✅[^103] | ✅[^103] | ◐[^104] | ◐[^105] |
| **Write** · create issue | ✅[^50] | ✅[^127] | ◐[^51] | ✅[^52] |
| **Write** · comment — visibility / internal | ✅[^53] | ✅[^128] | ◐[^54] | ✅[^55] |
| **Write** · comment edit / delete (`comment edit` / `comment rm`) | ✅[^114] | ✅[^137] | ✅[^115] | ✅[^116] |
| **Write** · transition — screen fields | ✅[^56] | ✅[^129] | ◐[^57] | ✅[^58] |
| **Write** · assign / unassign | ✅[^59] | ✅[^130] | ✅[^60] | ✅[^61] |
| **Write** · label edits | ✅[^62] | ✅[^62] | —[^63] | ✅[^64] |
| **Write** · component edits | ✅[^62] | ✅[^62] | —[^63] | ✅[^65] |
| **Write** · priority edit | ✅[^62] | ✅[^62] | ✅[^66] | ✅[^64] |
| **Write** · due date — set | ✅[^67] | ✅[^67] | ✅[^67] | ✅[^67] |
| **Write** · due date — clear | ✅[^68] | ✅[^68] | —[^69] | ✅[^68] |
| **Write** · summary / description edit | ✅[^70] | ✅[^131] | ◐[^71] | ✅[^70] |
| **Write** · custom-field edit | ✅[^35] | ✅[^124] | —[^72] | ◐[^37] |
| **Write** · issue type edit (`edit --type`) | ✅[^73] | ✅[^73] | —[^74] | ✅[^75] |
| **Write** · parent set / clear | ✅[^76] | ✅[^76] | —[^77] | ✅[^78] |
| **Write** · attachment upload | ✅[^79] | ✅[^119] | ✅[^80] | ✅[^113] |
| **Write** · link / unlink issues | ✅[^81] | ✅[^132] | —[^18] | ✅[^82] |
| **Write** · wiki write — page create / edit / comment | ✅[^83] | —[^126] | —[^45] | ✅[^84] |
| **Write** · `claim` | ◐[^85] | ◐[^133] | —[^86] | ✅[^87] |
| **Write** · worklog (`gadak api --write`) | ✅[^88] | ✅[^88] | —[^89] | —[^90] |
| **Write** · `migrate --from` (source) | ✅[^91] | ✅[^91] | ◐[^92] | ✅[^93] |
| **Write** · `migrate --to` (destination) | —[^94] | —[^94] | ◐[^94] | ✅[^95] |
| **Surface** · agent surfaces — skill / MCP / SQL | ✅[^96] | ✅[^96] | ✅[^96] | ✅[^96] |
| **Surface** · board layout (0.19) | ✅[^97] | ✅[^97] | ✅[^97] | ✅[^97] |
| **Surface** · board sprint scope + Sprint axes (0.22) | ✅[^138] | ✅[^138] | ✅[^138] | ✅[^138] |
| **Surface** · `views open --keys -` | ✅[^98] | ✅[^98] | ✅[^98] | ✅[^98] |
| **Surface** · watch feed + OS alerts | ✅[^99] | ✅[^134] | ◐[^100] | ✅[^99] |
| **Surface** · in-process origin (no network to the tracker) | —[^101] | —[^101] | —[^101] | ✅[^102] |

[^1]: Atlassian Cloud REST (`internal/jira/client.go:165`), mirrored by the
    Jira-family sync pass (`internal/sync/run.go:50`, `internal/sync/sync.go:153`).

[^2]: Read-only GraphQL at `api.linear.app` (`internal/linear/client.go`),
    mirrored by the Linear pass (`internal/sync/run.go:62`,
    `internal/sync/linear.go`).

[^3]: The same Jira-family pass, answered in-process by the origin
    (`internal/origin/transport.go:102`) or by a paired serve (`:108`).

[^4]: Runs on the mirror — SQLite FTS over `issues_full`, origin-agnostic
    (`cmd/gadak/agent.go:1167`).

[^5]: The mirror's schema is the contract (`specs/000-product/data-model.md`);
    queries never touch the origin.

[^6]: Same schema, but columns Linear does not map (issue type, components,
    fix versions, custom fields) read empty/NULL
    (`internal/sync/linear.go:218`, `internal/linear/MAPPING.md`). The sprint
    columns are mapped from Cycle (footnote 33); `epic_key` is the parent
    chain only — Linear Projects stay unmapped.

[^7]: The documented JQL subset (`docs/decisions/0007-jql-subset.md`),
    evaluated in-memory over mirror rows (`cmd/gadak/agent.go:1336`) — the
    origin is not queried, so the subset is the same on every origin.

[^8]: Same in-memory evaluation; a clause over a column Linear does not
    populate matches nothing rather than erroring.

[^9]: Comments mirror with the issue (`internal/sync/sync.go:843`).

[^10]: Fetched as follow-up passes after the issue list — comments, labels,
    attachments each round-trip once (`internal/sync/linear.go:90`); comment
    bodies are Linear markdown.

[^11]: Serve proxies `cfg.Site + /rest/api/3/attachment/content/{id}`
    (`internal/server/attachment.go:314`), passing the browser's `Range`
    through and relaying 206. Measured against a live Cloud site: the media
    target advertises `Accept-Ranges: bytes` and answers `bytes=100-199`
    with 206 and the right slice, on an image and on a 33 MB QuickTime
    alike — so seeking works here too. It sends no `ETag`, so a conditional
    request never becomes a 304 on this origin; gadak forwards
    `If-None-Match` anyway, which is what the Built-in origin needs
    ([^13]).

[^12]: Serve fetches `uploads.linear.app` with the workspace's Linear API key
    (`internal/server/attachment.go:379`).

[^13]: The origin serves the bytes from disk with `Accept-Ranges` and an
    `ETag` (`issuetap/docs/COMPATIBILITY.md:76`) and serve streams them
    through `origin.Client`, passing `Range` on and relaying 206
    (`internal/server/attachment.go:453`) — so seeking in a video works on
    the path that does not go through the byte cache (GDK-1617). Until
    GDK-1613 the proxy concatenated `cfg.Site`, which a Built-in workspace
    does not have — every view answered 502 on both transports, in-process
    and paired.

[^14]: Changelog events (`internal/jira/client.go:206`) feed
    `status_changed_at` and `reopen_count`, and since v43 `started_at` /
    `cycle_hours` — all four derive from the same history in one owner
    (`internal/store/derive.go`); time-in-status is computed from
    `status_changed_at`, never stored as a column
    (`specs/000-product/data-model.md`).

[^15]: Linear's read path carries no state history — `status_changed_at`,
    `reopen_count` and `reopened_at` stay NULL
    (`internal/sync/linear.go:46`). Every Linear batch says so
    (`Batch.NoHistory`, `internal/sync/linear.go:144`, `:430`), which is what
    stops the born-in-progress rule from guessing a start off an empty
    changelog. The flow columns do fill: the issue's own `startedAt` /
    `completedAt` / `canceledAt` stamps ride the batch as Derive hints
    (`internal/sync/linear.go:287`), and the NoHistory hint rule fills
    `started_at` / `resolved_at` / `cycle_hours` from them
    (`internal/store/derive.go:153`). A stamp is not a timeline — no
    synthetic changelog comes from it. An existing mirror row fills when its
    Linear `updated_at` next moves; a full sync re-reads but does not
    rewrite unchanged rows (`Batch.Force` is write-through only).

[^16]: The origin keeps a changelog and serves it
    (`issuetap/docs/COMPATIBILITY.md:71`); the same columns derive from it.

[^17]: `issuelinks` in the issue payload plus the link-type catalog
    (`internal/jira/write.go:234`, `:212`).

[^18]: The write half refuses with `ErrNoIssueLinks`
    (`internal/origin/writer.go:101`); reading relations is [^106].

[^19]: Link-type catalog and both-direction elements
    (`issuetap/docs/COMPATIBILITY.md:59`, `:75`).

[^20]: `ref` needs an origin that stores remote links where gadak can read
    them back; every non-Built-in origin is refused
    (`internal/origin/writer.go:104`, `:169`).

[^21]: Works embedded and paired (`cmd/gadak/ref.go:118`,
    `internal/jira/remotelink.go:51`); the sync pass refreshes the mirror only
    when the origin is embedded — on a paired workspace the list updates when
    `ref` writes, not on sync (`internal/origin/writer.go:191`).

[^22]: Opt-in: `dev_status` in config gates the fetch and the panel
    (`internal/sync/sync.go:1361`).

[^23]: Linear exposes no development panel to mirror — the Linear record
    builder has no dev-link half (`internal/sync/linear.go`).

[^24]: Always fetched, embedded or paired (`internal/sync/sync.go:1367`);
    `dev link|deploy|build` writes pass through (`cmd/gadak/dev.go:55`,
    `issuetap/docs/COMPATIBILITY.md:77`).

[^25]: Mirrored with the issue row (`internal/sync/sync.go:843`).

[^26]: Labels arrive as a follow-up fetch (`internal/sync/linear.go:93`).

[^27]: Not a Linear concept in gadak's mapping — the columns stay empty
    (`internal/sync/linear.go:218`).

[^28]: A per-project catalog derived from the project's issues
    (`issuetap/docs/COMPATIBILITY.md:75`).

[^29]: `GET /project/{key}/versions` (`internal/jira/write.go:181`).

[^30]: `ErrNoVersionCatalog` (`internal/origin/writer.go:100`); the columns
    stay empty (`internal/sync/linear.go:218`).

[^31]: The catalog is derived from the project's issues, and this is the one
    origin where a write may mint a version by name (GDK-678,
    `internal/origin/writer.go:51`).

[^32]: The sprint columns come from the Jira Software sprint field, discovered
    per site (`internal/sync/sprint.go:18`, `:75`); a site without it syncs
    empty. Cloud sends an object array, Server the Java `toString` of its own
    bean — both are read, and the state is lowercased so `active` means the
    same thing on either (`internal/sync/sprint.go:191`, GDK-1650). The
    state on an issue row is re-derived from the `sprints` table on every
    tick (`internal/store/write.go:1730`, GDK-1661): a sprint closing moves
    no `updated` on the done issues that stay in it, so the projection
    alone went stale.

[^33]: Linear's Cycle is the sprint, and it maps whole (GDK-1667).
    `Issue.cycle { id number name startsAt endsAt completedAt }` fills the
    three columns (`internal/sync/linear.go:267`); the state is derived from
    the dates rather than stored — `completedAt` set or `endsAt` past is
    `closed`, a window containing now is `active`, a future `startsAt` is
    `future` (`internal/linear/client.go:640`). Cycle UUIDs become the
    mirror's INTEGER sprint space through one FNV-1a derive
    (`internal/linear/client.go:623`), with the UUID kept in
    `sprints.external_id` (schemaV46) so a write can walk it back.

[^34]: The origin's issue model has no sprint field — the editable set
    carries none (`issuetap/docs/COMPATIBILITY.md:72`).

[^135]: `/rest/agile/1.0/board` and `/board/{id}/sprint`, which answer the same
    shape on Cloud and Server — measured on Jira Software 11.3.11 DC
    (`internal/jira/agile.go:14`, `internal/sync/agile.go:16`, GDK-1654). A
    site without Jira Software has no Agile API and syncs no boards; that is
    silent, not an error.

[^136]: `gadak sprint` (`cmd/gadak/sprint.go:32`) over
    `POST /rest/agile/1.0/sprint/{id}/issue`, `/backlog/issue`, `/sprint` and
    `/sprint/{id}` (`internal/jira/agile.go:118`, GDK-1655). Every state
    change re-reads the sprint listing and the issues that were in it, so the
    mirror states what the origin holds rather than what gadak sent.

[^35]: `GET /field` catalog (`internal/jira/client.go:311`); editable kinds
    `text`, `number`, `date`, `option`, `user`, `multi_option` /
    `version_array`, gated by the issue's editmeta and the configured field
    allowlist. Cascading selects and textarea custom fields have no editor.

[^36]: No custom-field mapping exists (`internal/linear/MAPPING.md`).

[^37]: Only fields declared by the workspace's data exist, and there is no
    field-creation route (`issuetap/docs/COMPATIBILITY.md:72` — "fixture
    custom fields").

[^38]: From create/edit metadata (`internal/jira/write.go:379`, `:322`).

[^39]: `issue_type` maps from nothing (`internal/linear/MAPPING.md:82`).

[^40]: Editable set with allowed values (`issuetap/docs/COMPATIBILITY.md:72`).

[^41]: `parent_key` mirrors the direct parent; `epic_key` (nearest
    hierarchy-level-1 ancestor) is derived at sync (`internal/sync/sync.go:843`).

[^42]: `parent_key` is a true mapping from `Issue.parent`; `epic_key` stays
    NULL — "epic" is a Jira-ism that would lie whenever a team nests
    sub-issues (`internal/linear/MAPPING.md:137`).

[^43]: A parent must exist and sit exactly one hierarchy level above the child
    (`issuetap/docs/COMPATIBILITY.md:75`).

[^44]: Confluence Cloud through the wiki client (`internal/origin/origin.go:408`,
    `internal/sync/confluence.go`). A team-spaces fix landed on main after
    this table's base (17e48607).

[^45]: Linear provides no wiki — the client refuses with one sentence
    (`internal/origin/origin.go:70`).

[^46]: `/wiki/rest/api` spaces, CQL, pages, versions, comments
    (`issuetap/docs/COMPATIBILITY.md:78`).

[^47]: One resolver per surface, and it branches on the origin type with no
    fallback across them (GDK-1308): Jira is `cfg.Site + /browse/KEY`
    (`web/src/lib/issue-origin.ts:29`, `cmd/gadak/agent.go:2761`); the
    header key anchor and the copy-link paste lead with it
    (`web/src/components/detail/DetailHeader.svelte:126`).

[^48]: Linear has no site; the page Linear itself minted is stored on the
    row (`items.url`) by sync, and `gadak open`, the key anchor, copy-link
    and the palette all open that (`web/src/lib/issue-origin.ts:32`,
    `cmd/gadak/agent.go:2746`; GDK-1149). A row without a stored url is a
    missing link, never a Jira URL.

[^49]: There is no origin page — the Built-in tracker's page is this app.
    `gadak open` focuses the running serve on the issue
    (`cmd/gadak/agent.go:2779`); the web has no origin link and copy-link
    pastes app links only.

[^50]: `POST /issue` (`internal/jira/write.go:331`). A CLI (agent) create
    ends the description with the actor trailer `— via gadak · <actor>`
    (`internal/origin/trailer.go:50`, off with `actor.trailer false`); the web
    create does not (`internal/server/write.go:217`).

[^51]: Create works; assignee, labels, parent, and issue type are refused on
    create (`internal/origin/linearwriter.go:297`, `cmd/gadak/create.go:377`).
    The CLI create carries the same actor trailer as Jira[^50], rendered to
    markdown with the body.

[^52]: `issuetap/docs/COMPATIBILITY.md:75`, with the same parent-hierarchy
    rule.

[^53]: ADF body with optional visibility and internal flag
    (`internal/jira/write.go:256`). A CLI (agent) comment carries the actor
    trailer as its last paragraph (`internal/origin/trailer.go:107`); the web
    comment does not — the person is the author.

[^54]: Neither visibility nor internal (`internal/origin/linearwriter.go:84`);
    the body is serialized back to markdown (`:95`, `adf.Markdown` — the
    identity on the markdown subset, GDK-1386); mentions and inline media
    degrade to their text.

[^55]: `visibility` plus the `sd.public.comment` internal mapping
    (`issuetap/docs/COMPATIBILITY.md:75`).

[^56]: `POST /issue/{key}/transitions` with fields and comment
    (`internal/jira/write.go:116`). A transition comment from the CLI carries
    the actor trailer (`internal/origin/trailer.go:111`); a transition without
    a comment gains none.

[^57]: Linear transitions carry no screen fields
    (`internal/origin/linearwriter.go:73`).

[^58]: `fields.resolution` and `update.comment` are honored, screen-checked
    (`issuetap/docs/COMPATIBILITY.md:70`). The seeded workflow puts an optional
    `resolution` on the done transition's screen (`internal/origin/origin.go:739`,
    GDK-1347); a workspace seeded before 0.20.1 has none, and its screen 400 is
    reworded in gadak's terms (`cmd/gadak/agent.go:2394`).

[^59]: `PUT /issue/{key}/assignee` (`internal/jira/write.go:276`).

[^60]: The fields path carries assign and unassign
    (`internal/origin/linearwriter.go:178`); refused only at create time
    (`:297`).

[^61]: `POST /issue/{key}/assignee` (`issuetap/docs/COMPATIBILITY.md:75`).

[^62]: The two-part edit — `fields` replaces, `update` carries add/remove
    operations (`internal/jira/write.go:293`).

[^63]: The `update` half is refused outright (`internal/origin/linearwriter.go:203`)
    and any field outside the editable set is refused (`:190`); labels are
    deliberately absent from Linear edit metadata.

[^64]: Editable set (`issuetap/docs/COMPATIBILITY.md:72`).

[^65]: Full replace on `fields`, `add`/`remove`/`set` on `update`, by id or by
    name (`issuetap/docs/COMPATIBILITY.md:75`).

[^66]: Linear's 0-4 scale; clearing a priority maps to 0, "No priority"
    (`internal/origin/linearwriter.go:160`, `:32`).

[^67]: `edit --due YYYY-MM-DD` → `fields.duedate` (`cmd/gadak/edit.go:355`);
    Linear takes the date as-is (`internal/origin/linearwriter.go:169`).

[^68]: `edit --due none` → `fields.duedate = nil` (`cmd/gadak/edit.go:353`).

[^69]: "clearing a due date is not supported yet"
    (`internal/origin/linearwriter.go:171`).

[^70]: `fields.summary` / `fields.description` (`cmd/gadak/edit.go:305`,
    `internal/jira/write.go:286`).

[^71]: The description is serialized back to markdown on write
    (`internal/origin/linearwriter.go:157`, create `:324`) — headings,
    lists, tables and marks survive; panels, media and mentions degrade to
    text (GDK-1386). On Jira and Built-in, a body's nodes markdown cannot
    carry stand as placeholders in the editing source and `edit -m` puts
    them back (GDK-1396, decision 0012 addendum 1); a text with none over a
    body that has them is refused without `--force-plain`. Linear bodies
    have no such nodes, and a placeholder in a Linear edit is refused. `edit
    --adf-file` / `comment --adf-file` send a document as it is and skip
    that guard (GDK-1395) — on Linear that document is still serialized to
    markdown on the way in.

[^72]: Any field outside Linear's editable set is refused
    (`internal/origin/linearwriter.go:190`).

[^73]: `edit --type` → `fields.issuetype` (`cmd/gadak/edit.go:301`).

[^74]: `ErrNoIssueTypes` — Linear has no issue types
    (`internal/origin/writer.go:133`).

[^75]: `issuetype` with allowed values (`issuetap/docs/COMPATIBILITY.md:72`).

[^76]: `create --parent` / `edit --parent KEY|none` → `fields.parent`
    (`cmd/gadak/create.go:538`, `cmd/gadak/edit.go:357`). Jira has no
    dedicated REST parent route — the edit fields path is the only road.
    Jira Server keys only a sub-task's parent there; a standard issue's
    epic is the Epic Link custom field, and a `parent` sent for one is
    answered 204 and dropped (measured on 11.3.11). The client rewrites it
    at the dialect (`internal/jira/epiclink.go:95`), refusing by name on a
    Server with no Epic Link field. After any edit the refreshed row is
    compared with what was asked before it is printed as success
    (`cmd/gadak/edit_verify.go:19`, GDK-1645).

[^77]: Refused on create (`cmd/gadak/create.go:377`) and on edit
    (`internal/origin/linearwriter.go:190`).

[^78]: Same `fields.parent`, with hierarchy validation and honest 400s
    (`issuetap/docs/COMPATIBILITY.md:75`).

[^79]: `POST /issue/{key}/attachments` multipart (`internal/jira/write.go:497`),
    streamed through a pipe rather than buffered. The part declares its type
    from the filename (`internal/jira/write.go:454`): Cloud sniffs
    server-side, but an origin
    that keeps what it is told stored every screenshot as
    `application/octet-stream` and the app then had no thumbnail to show
    (GDK-1617).

[^80]: URL-first: reserve storage, PUT the bytes, confirm
    (`internal/origin/linearwriter.go:366`).

[^81]: `POST /issueLink`, `DELETE /issueLink/{id}`, catalog via
    `GET /issueLinkType` (`internal/jira/write.go:222`, `:246`, `:212`).

[^82]: Same routes (`issuetap/docs/COMPATIBILITY.md:59`, `:75`).

[^83]: `gadak page create|edit|comment` → Confluence REST through the wiki
    client (`cmd/gadak/page.go:189`, `:410`, `:326`;
    `internal/origin/origin.go:408`).

[^84]: `POST /wiki/rest/api/content`, `PUT …/{id}` with a version check
    (`issuetap/docs/COMPATIBILITY.md:83`).

[^85]: No atomic claim route on Cloud — the fallback runs assignee +
    transition as two calls and says so (`internal/claim/claim.go:9`,
    `cmd/gadak/agent.go:2556`).

[^86]: Refused before any call: claim is a Jira-workflow verb and the Linear
    writer does not implement it (`cmd/gadak/agent.go:2539`).

[^87]: One atomic mutation — the origin's own extension route
    (`issuetap/docs/COMPATIBILITY.md:71`).

[^88]: The `api` verb passes any Jira REST route through the origin client
    (`cmd/gadak/api.go:140`).

[^89]: The `api` verb needs a Jira-family credential; a Linear-only workspace
    is refused (`internal/origin/origin.go:139`).

[^90]: Unknown Jira routes get the honest 501 `unsupported_endpoint`
    (`issuetap/internal/api/jira.go:89`).

[^91]: The export reads the mirror; attachment bytes come from the origin's
    attachment route (`cmd/gadak/migrate.go:23`).

[^92]: Linear attachment URLs are not byte-fetchable the way Jira's are — a
    workspace with attachments refuses without `--skip-attachments`
    (`cmd/gadak/migrate.go:89`).

[^93]: The export's byte fetch uses the same passthrough route
    (`cmd/gadak/migrate.go:105`). Bodies leave as the origin's ADF beside
    their text (`internal/migrate/migrate.go:83`), so headings, lists and
    paragraph breaks arrive as written (GDK-1382).

[^94]: `--to linear --team KEY` sends a mirror's issues into a Linear team
    through the Linear workspace the command runs in (`cmd/gadak/migrate.go:56`,
    `internal/migrate/linear.go`; GDK-1265): issues, comments, parents and
    relations land, idempotent on re-run via a `gadak-migrate: KEY` footer;
    change history, wiki pages, dev links, custom fields and sprints stay
    behind and the report says so. Jira is not a destination.

[^95]: The migrate command creates a fresh Built-in workspace as its target,
    which must not exist yet (`cmd/gadak/migrate.go:23`). The fixture's
    `descriptionAdf` / `bodyAdf` slots are stored verbatim when they parse
    as a document; a body without one is wrapped as a single paragraph
    (`issuetap/internal/store/store.go`, `fixtureBody`; GDK-1382).

[^96]: All three surfaces run against the mirror; the MCP tools expose no
    write verb on any origin (`internal/mcp/tools.go:24`), and `gadak_status`
    reports which origin the workspace has.

[^97]: The board is the same filtered list laid out as columns, saved per view
    (`web/src/lib/view-config.ts:141`); a status-axis drag is a real
    transition where transitions exist.

[^98]: Mirror-side contract — reads saved views and emits keys; no origin call
    (`specs/000-product/data-model.md`).

[^99]: The Jira-family source notifies — OS alerts on macOS and Linux
    (`internal/sync/run.go:59`).

[^100]: Linear issues are mirrored by the same loop but never notify
    (`internal/sync/run.go:70`). Jira's own notification inbox, rules, and
    email are not mirrored on any origin.

[^101]: The origin is a remote host — `https://<site>.atlassian.net` for Jira,
    `https://api.linear.app` for Linear (`internal/linear/client.go`).

[^102]: Embedded in the same process (`internal/origin/transport.go:102`), or
    one hop to a paired serve (`:108`).

[^117]: Server keeps REST v2 only (a v3 route answers 401, not 404) and the
    classic `POST /search` paged by `startAt`/`total`
    (`internal/jira/client.go:228`, GDK-1636); `expand` is a list there, a
    string on Cloud (`internal/jira/client.go:249`). Users key by `name`,
    not `accountId` (GDK-1638); the credential is a PAT Bearer, because
    basic auth is off by default on 11.x (GDK-1640). Measured: 409 issues in
    one 3 s full sync, then quiet incremental ticks. Data Center states its
    rate-limit budget on every response (`X-RateLimit-*`); the Server client
    spaces requests from it before a 429 (`internal/httppolicy/ratebudget.go`,
    `internal/jira/client.go:147`, GDK-1646) — Cloud publishes no such headers
    and its path is untouched.

[^118]: v2 comment bodies are wiki-markup strings and land in `body_text`
    as-is; the visibility block is read into `comments.visibility_type` /
    `visibility_value` (`internal/sync/sync.go:1189`). Measured: a comment
    restricted to the Administrators role mirrors as `role` /
    `Administrators`.

[^119]: Attachment bytes live under `/secure/attachment/{id}/{filename}` on
    Server, not `/rest/api/3/attachment/content` (`internal/jira/types.go:110`);
    `gadak attach get` streamed a 50-byte seed file back byte for byte.
    Uploads carry `X-Atlassian-Token: no-check` or Server answers 403
    (`internal/jira/write.go:613`).

[^120]: The changelog arrives through the v2 `expand` list and feeds the same
    derivations. Measured: an issue moved To Do → In Progress → Done → To Do
    reads `reopen_count` 1, `started_at` set, and the Done → To Do row in
    `changelog`; Server's own Done transition sets the resolution by
    post-function, so `cycle_hours` fills when the issue is done.

[^121]: The same opt-in (`devStatus`); Server has the dev-status API and the
    sync ran clean with it on (no dev tool is connected to the lab, so 0
    rows). The refusal `gadak dev link` prints there still speaks of Jira
    Cloud's GitHub app — GDK-1663.

[^122]: Boards and sprints list the same way (footnote 135), but Server's
    board listing carries no `location`, so `boards.project_key` is empty
    there — the board→project mapping needs `/board/{id}/project` or a
    per-project listing (GDK-1665). Sprints themselves are complete: three
    seeded states read back as `closed` / `active` / `future`.

[^123]: `gadak sprint create <board> <name>`, `add`, `remove` measured on
    the lab (issue row follows: `future` after add, empty after remove);
    `start` and `close` measured the day before on the same instance
    (`sprint start 3` → the issue reads `active`; `sprint close 2` →
    `closed`). Same routes as footnote 136; the write returns 201/204 with
    an empty `text/html` body, which the page guard now lets through
    (GDK-1662).

[^124]: `gadak fields --apply` mapped Sprint, Story Points and the three Epic
    fields from the Server catalog; `story_points` read back 3/5/2/8 and
    `edit --field story_points=13` landed on a Story. Server honours field
    contexts: the same edit on a Task is refused as "not editable" because
    Story Points is not in that type's context — Jira's answer, passed
    through.

[^125]: Server publishes no `hierarchyLevel` on issue types; sub-tasks come
    from the `subtask` flag (`internal/sync/sync.go:1152`) and epics are
    derived from who has a standard child (`internal/store/write.go:1754`,
    GDK-1658). A standard issue's epic is the Epic Link custom field, read
    into `parent_key` / `epic_key` (GDK-1651). Measured: epic → story →
    sub-task reads levels 1 / 0 / −1 with both keys filled.

[^126]: There is no Confluence Server / Data Center client — the wiki client
    speaks Confluence Cloud's API and a Server wiki is a separate site with
    v1 REST (GDK-1664). `gadak page create` on a Server workspace refuses,
    today with the wrong sentence ("site, email and token are required";
    GDK-1663).

[^127]: `gadak create` through the per-project createmeta route
    (`internal/jira/write.go:444`, GDK-1636); the description goes out as
    wiki markup and comes back byte for byte (`internal/jira/write.go:279`,
    GDK-1637). An Epic needs Epic Name (`gh-epic-label`) or Server answers
    400 — the lab seed sets it. Measured: a Bug with priority, label and a
    formatted body created and re-read.

[^128]: `--visibility role=NAME` measured (a comment only Administrators can
    read mirrors with that visibility). `--internal` is a Service Management
    property and there is no JSM on the lab, so it is not measured on
    Server.

[^129]: `gadak transition KEY` lists the transitions; `done` / `new` moved
    an issue there and back, and Server's Done transition set Resolution by
    post-function. A field the transition screen does not carry
    (`--resolution` on the scrum template's Done) is Server's own 400,
    passed through unchanged — the same as Cloud; a screen carrying one was
    not configured on the lab.

[^130]: Server's assignee body is `{"name": …}` (`internal/jira/write.go:304`,
    GDK-1638); `gadak assign KEY dana` and `assign KEY -` measured, the row
    reading the username as `assignee_id`.

[^131]: The description round-trips as wiki markup (`h2.` / `*` lists came
    back as typed; GDK-1637); summary edits are the same v2 PUT. After any
    edit the re-read row is compared with what was asked before it is
    printed (GDK-1645).

[^132]: Same routes as Cloud; Server answers the link POST with 201 and an
    empty `text/html` body, which the page guard refused until GDK-1662
    (`internal/atlhttp/transport.go:237`). Measured after the fix: link,
    `links` rows on both issues, unlink.

[^133]: Same two-call fallback as Cloud (footnote 85) and the same warning
    sentence; measured on the lab: assignee plus the in-progress
    transition, `atomic: false`.

[^134]: The Server workspace's issue source is `jira`, the same source id
    the notifier keys on (`internal/sync/run.go:59`), and the feed is
    computed from the mirror. The sync loop was exercised on the lab (a
    comment made through the API arrived on the next tick); the OS alert
    itself was not fired on Server.

[^137]: The same `PUT` / `DELETE …/comment/{id}` on the v2 base; measured:
    `comment edit` re-read the new text, `comment rm --yes` removed the
    row.
[^138]: The board's Active sprint · Backlog · All is a filter on
    `sprint_state` (`web/src/components/board/SprintScope.svelte`), so it is
    in the URL, in saved views and in `views open --jql`; the filter bar's
    Sprint and Sprint state axes read the same columns, and `sprint is
    EMPTY` round-trips as `sprint_state=none` (`internal/jql/match.go`,
    GDK-1656). The control appears only when the mirror has a sprint row
    (`GET /api/v1/issues/sprints/`), so an origin without sprints never
    shows it — which is what the two refusal cells mean today.
[^141]: One `boards` row per in-scope team (`type = 'cycles'`, `project_key`
    = the team key) and every one of that team's cycles as a `sprints` row,
    walked each tick (`internal/sync/linear.go:279`, `importLinearCycles`). A
    cycle the listing does not carry — another team's cycle on an in-scope
    issue — keeps its issue-side projection and gets no `sprints` row.

[^142]: `sprint add` / `remove` go through `issueUpdate`
    (`internal/origin/linearwriter.go:511` onward); `remove` sends
    `cycleId: null` because an omitted field means unchanged. Measured on a
    live Linear team with cycles switched on: an add fills the issue's three
    sprint columns and the `sprints` row's count, a remove empties them
    again, and `sprint in futureSprints()` finds the issue in between.
    **Three verbs refuse by name.** `start` and `close`
    (`internal/origin/writer.go:356`): a cycle begins and ends by its dates,
    so the place to move one is the cycle's dates. `create`
    (`internal/origin/writer.go:364`, GDK-1678): Linear answers `cycleCreate`
    with "Cycle creation is not supported." — cycles come from the team's
    cadence setting. The writer's `UpdateSprint` maps name/goal and the
    window onto `cycleUpdate`; no CLI verb reaches it yet.

[^139]: The built-in tracker serves the same Agile surface — one scrum
    board per project, created lazily — plus `customfield_10020` in Cloud's
    object-array shape and JQL's three sprint functions, so gadak reads it
    through exactly the code path Jira uses (issuetap
    `internal/api/agile.go`, `docs/decisions/0002-agile-api-surface.md`,
    GDK-1666). Measured end to end on a standalone workspace: create, add,
    start, `sprint in openSprints()`, close.

[^140]: Same routes as footnote 136 against the in-process origin. Closing
    a sprint sweeps its unfinished issues to the backlog, the way Jira's
    own close does — measured: two issues in, `sprint close 1`, both rows
    back to no sprint.

## How this file is maintained

This file is the single owner of the matrix — the READMEs summarize it and
link here. The Jira Server column is re-measured with
`tools/jira-server-lab/seed.sh --reset` and `tools/jira-server-lab/measure.sh`
against the lab instance the runbook describes. A commit that touches `internal/origin/writer.go`,
`internal/origin/linearwriter.go`, `internal/linear/`, `internal/sync/`, or
bumps the issuetap dependency updates this file in the same commit: a
refusal added or removed there is a cell changed here. Structural drift — a
missing file, a malformed row, a footnote marker with no definition, a
README that stopped linking — is caught by `tools/doc-checks.sh` check
**#39**; whether a cell still tells the truth is review's job. Generating
this table from the code instead of maintaining it by hand is GDK-1301.
[^103]: The issue navigator with this view's JQL (`<site>/issues/?jql=…`,
    `web/src/lib/view-link.ts:52`) leads the paste, then `gadak://view?<hash>`
    and the http link (GDK-1343). The JQL comes from the server's `jql/emit/`;
    clauses it cannot carry are named in the toast.
[^104]: Linear has no public URL parameter that carries a filter, so the paste is
    the app lines alone (`web/src/lib/view-link.ts:49`) — the same branch the
    issue copy-link takes on the Built-in origin. No stand-in.
[^105]: The Built-in tracker's page is this app: app lines only
    (`web/src/lib/view-link.ts:49`).

[^106]: `Issue.relations` / `inverseRelations` in the issue query
    (`internal/linear/queries.go:197`) land as outward / inward `links` rows
    named like the Jira ones — Blocks, Duplicate, Relates
    (`internal/sync/linear.go:328`); GDK-1299.

[^107]: The link-type catalog is fetched once per sync run
    (`internal/sync/sync.go:244`) and cached into `link_types`; `open_blockers`
    resolves which types block from that catalog, never a hardcoded name
    (`internal/store/flow.go:81`, `:156`), and `gadak ready` reads the column
    (`cmd/gadak/list.go:119`). "Ready" means no *recorded* open blocker —
    block links are typed far less consistently than epic links.

[^108]: Linear has one blocking relation and no site catalog to fetch; a fixed
    one-row catalog rides every Linear batch (`internal/sync/linear.go:349`),
    so the same column and verb work over the relations GDK-1299 mirrored as
    `links` ([^106]).

[^109]: The origin serves a link-type catalog
    (`issuetap/docs/COMPATIBILITY.md:59`); the column and the verb are the
    Jira path (`internal/store/flow.go:156`).

[^110]: `gadak attach get` resolves the name or id in the mirror, then reads
    `GET /rest/api/3/attachment/content/{id}` through `origin.Client`
    (`cmd/gadak/attach_get.go:159`). Membership is the mirror query, so an id
    belonging to another issue is not found rather than fetched
    (`cmd/gadak/attach_get.go:96`).

[^111]: The Linear branch fetches the stored `uploads.linear.app` URL with the
    workspace's API key (`cmd/gadak/attach_get.go:149` →
    `internal/linear/download.go:50`). A stored URL on any other host is
    refused before the request leaves the process (GDK-560,
    `internal/linear/download.go:26`).

[^112]: The in-process Built-in origin answers the same content route, so the
    verb needs no site: measured on a `gadak init --local` workspace, `attach
    get` wrote the bytes `gadak attach` had uploaded. The branch is on the
    mirrored row's source, never a fallback (`cmd/gadak/attach_get.go:136`).

[^113]: Same multipart route as Jira ([^79]), streamed end to end: the CLI
    builds the multipart body through a pipe (`internal/jira/write.go:448`)
    and the origin writes it straight to a content-addressed file
    (`issuetap` `internal/store/store.go` `AddAttachmentStream`). The cap is
    settings, not a constant — `gadak config set attachmentMaxMB`, default
    1 GiB (GDK-1617) — and over it is a 413 naming the limit, with
    `gadak attach` exiting non-zero on that sentence. Until GDK-1614 an
    oversize upload was truncated instead: stored as its first 8 MiB and
    reported as success.

[^114]: `PUT` / `DELETE /rest/api/3/issue/{key}/comment/{id}`
    (`internal/jira/write.go:282`, `:290`); an edit sends what a post sends,
    so a Jira Server origin's edit carries the wiki-markup string the post
    path already sends (`internal/origin/body.go:28`). The CLI verb is
    `gadak comment edit|rm` (`cmd/gadak/agent.go:2048`), and an edited body
    carries the actor trailer with the same idempotence a post has
    (`internal/origin/trailer.go:258`).

[^115]: `commentUpdate` / `commentDelete` over GraphQL
    (`internal/linear/write.go:246`, `:267`,
    `internal/origin/linearwriter.go:109`); the body is markdown, the same
    dialect `comment` posts in.

[^116]: The built-in tracker answers the same two routes since issuetap
    `17ce397` (`issuetap/internal/api/jira.go:893` `putComment`, `:919`
    `deleteComment`; pinned in `go.mod`). Measured 2026-09-09 on a fresh
    `gadak init --local` workspace: `gadak comment edit STD-2
    standalone-jira:90001 -m …` replaced the body and `gadak comment rm … --yes`
    removed it, both with the mirror's own namespaced id.

[^143]: The sprint field's id is per-site, so its changelog rows arrived under
    that site's own custom field id and nothing could ask for them; sync
    normalises the field to `sprint` using the id it already discovers for the
    issue field (`internal/sync/sync.go:1387`, `internal/sync/sprint.go`).
    `Derive` then counts the distinct sprints an issue has entered
    (`internal/store/derive.go`), reading both origin shapes — Jira's growing
    membership list and the built-in tracker's single-id move — with one rule.
    Existing mirrors are recognised and backfilled at the v48 migration
    (`internal/store/flow.go`).

[^144]: Linear supplies no changelog at all (`Batch.NoHistory`), so
    `carryover_count` is NULL rather than 0 — a cycle an issue was never
    carried out of and a history that cannot be read are different answers
    (`internal/store/derive.go`).

[^145]: The built-in tracker records each sprint move in the changelog under
    the same `customfield_10020` id Cloud uses, stating the single sprint
    moved into rather than the whole membership; the one counting rule reads
    both.

[^146]: The bucket source is the only thing that changes: every metric is
    computed from a `[From, To)` span and none assumes seven days, so
    `SprintBuckets` (`internal/retro/retro.go`) reads each sprint's own
    `start_at`/`end_at` from the `sprints` table and the rest of `Compute` is
    untouched. That table is filled on every origin that has sprints, so this
    works wherever the row above it does — Linear included, where the buckets
    are cycles.
