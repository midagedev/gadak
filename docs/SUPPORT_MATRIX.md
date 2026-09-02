# Origin support matrix

One table answers "does this work on my origin?". A gadak workspace has
exactly one origin — **Jira** (Atlassian Cloud), **Linear**, or **Built-in**
(the tracker that travels with the app, from 0.16). The mirror is a cache on
all three; every write passes through the origin and the mirror row is
re-read after it lands.

Every cell carries a footnote pointing at the code that makes it true — a
`path:line` in this repository, or a line in the Built-in origin's
compatibility inventory (module
`github.com/midagedev/issuetap@v0.0.0-20260901141459-03c8c28f05ba`, cited
below as `issuetap/docs/COMPATIBILITY.md`). A Built-in cell is never "same
as Jira": it means the Jira REST verb exists and the Built-in origin
implements the route.

Markers:

- ✅ — works
- ◐ — works, with a limitation the footnote names
- — — not on this origin; gadak refuses with a sentence rather than
  half-applying

| Capability | Jira | Linear | Built-in |
| --- | --- | --- | --- |
| **Read** · issue sync | ✅[^1] | ✅[^2] | ✅[^3] |
| **Read** · full-text search (`gadak search`) | ✅[^4] | ✅[^4] | ✅[^4] |
| **Read** · SQL (`gadak sql`, `issues_full` + RECIPES) | ✅[^5] | ✅[^6] | ✅[^5] |
| **Read** · `--jql` / pasted Jira URL | ✅[^7] | ✅[^8] | ✅[^7] |
| **Read** · comments | ✅[^9] | ✅[^10] | ✅[^9] |
| **Read** · attachment bytes | ✅[^11] | ✅[^12] | ◐[^13] |
| **Read** · history → `status_changed_at`, time-in-status, `reopen_count` | ✅[^14] | —[^15] | ✅[^16] |
| **Read** · issue links | ✅[^17] | —[^18] | ✅[^19] |
| **Read** · remote issue links / cross-workspace refs (`ref`) | —[^20] | —[^20] | ◐[^21] |
| **Read** · development-panel links (`dev`) | ◐[^22] | —[^23] | ✅[^24] |
| **Read** · labels | ✅[^25] | ✅[^26] | ✅[^25] |
| **Read** · components | ✅[^25] | —[^27] | ✅[^28] |
| **Read** · fix versions + `versions` catalog | ✅[^29] | —[^30] | ✅[^31] |
| **Read** · sprints (columns `sprint_id`/`sprint_name`/`sprint_state`) | ✅[^32] | —[^33] | —[^34] |
| **Read** · custom fields (`fields --apply`) | ✅[^35] | —[^36] | ◐[^37] |
| **Read** · issue type | ✅[^38] | —[^39] | ✅[^40] |
| **Read** · hierarchy — `parent_key` / `epic_key` | ✅[^41] | ◐[^42] | ✅[^43] |
| **Read** · wiki pages | ✅[^44] | —[^45] | ✅[^46] |
| **Read** · origin web URL — `gadak open`, web key anchor, copy link | ✅[^47] | ✅[^48] | ◐[^49] |
| **Read** · view link — toolbar / palette "Copy link to this view" | ✅[^103] | ◐[^104] | ◐[^105] |
| **Write** · create issue | ✅[^50] | ◐[^51] | ✅[^52] |
| **Write** · comment — visibility / internal | ✅[^53] | ◐[^54] | ✅[^55] |
| **Write** · transition — screen fields | ✅[^56] | ◐[^57] | ✅[^58] |
| **Write** · assign / unassign | ✅[^59] | ✅[^60] | ✅[^61] |
| **Write** · label edits | ✅[^62] | —[^63] | ✅[^64] |
| **Write** · component edits | ✅[^62] | —[^63] | ✅[^65] |
| **Write** · priority edit | ✅[^62] | ✅[^66] | ✅[^64] |
| **Write** · due date — set | ✅[^67] | ✅[^67] | ✅[^67] |
| **Write** · due date — clear | ✅[^68] | —[^69] | ✅[^68] |
| **Write** · summary / description edit | ✅[^70] | ◐[^71] | ✅[^70] |
| **Write** · custom-field edit | ✅[^35] | —[^72] | ◐[^37] |
| **Write** · issue type edit (`edit --type`) | ✅[^73] | —[^74] | ✅[^75] |
| **Write** · parent set / clear | ✅[^76] | —[^77] | ✅[^78] |
| **Write** · attachment upload | ✅[^79] | ✅[^80] | ✅[^79] |
| **Write** · link / unlink issues | ✅[^81] | —[^18] | ✅[^82] |
| **Write** · wiki write — page create / edit / comment | ✅[^83] | —[^45] | ✅[^84] |
| **Write** · `claim` | ◐[^85] | —[^86] | ✅[^87] |
| **Write** · worklog (`gadak api --write`) | ✅[^88] | —[^89] | —[^90] |
| **Write** · `migrate --from` (source) | ✅[^91] | ◐[^92] | ✅[^93] |
| **Write** · `migrate --to` (destination) | —[^94] | ◐[^94] | ✅[^95] |
| **Surface** · agent surfaces — skill / MCP / SQL | ✅[^96] | ✅[^96] | ✅[^96] |
| **Surface** · board layout (0.19) | ✅[^97] | ✅[^97] | ✅[^97] |
| **Surface** · `views open --keys -` | ✅[^98] | ✅[^98] | ✅[^98] |
| **Surface** · watch feed + OS alerts | ✅[^99] | ◐[^100] | ✅[^99] |
| **Surface** · in-process origin (no network to the tracker) | —[^101] | —[^101] | ✅[^102] |

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
    fix versions, sprint, `epic_key`, custom fields) read empty/NULL
    (`internal/sync/linear.go:218`, `internal/linear/MAPPING.md`).

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
    (`internal/server/attachment.go:279`).

[^12]: Serve fetches `uploads.linear.app` with the workspace's Linear API key
    (`internal/server/attachment.go:268`).

[^13]: The origin serves the bytes (`issuetap/docs/COMPATIBILITY.md:76`), but
    serve's proxy builds a site URL a Built-in workspace does not have — on a
    cold cache the proxy fails (`internal/server/attachment.go:279` with an
    empty `cfg.Site`; measured 502). `gadak api GET
    /rest/api/3/attachment/content/{id}` still returns the bytes — the same
    route the migrate export uses (`cmd/gadak/migrate.go:105`).

[^14]: Changelog events (`internal/jira/client.go:206`) feed
    `status_changed_at` and `reopen_count`; time-in-status is computed from
    `status_changed_at`, never stored as a column
    (`specs/000-product/data-model.md`).

[^15]: Linear's read path carries no state history — `status_changed_at` and
    `reopen_count` stay NULL (`internal/sync/linear.go:46`, `:223`).

[^16]: The origin keeps a changelog and serves it
    (`issuetap/docs/COMPATIBILITY.md:71`); the same columns derive from it.

[^17]: `issuelinks` in the issue payload plus the link-type catalog
    (`internal/jira/write.go:234`, `:212`).

[^18]: Linear relations are not mirrored, and the write half refuses with
    `ErrNoIssueLinks` (`internal/origin/writer.go:101`); syncing relations is
    GDK-1299 (in flight).

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
    per site (`internal/sync/sprint.go:18`, `:58`); a site without it syncs
    empty. No origin gets sprint *verbs* — moving an issue between sprints or
    editing a sprint happens in Jira.

[^33]: Linear has no sprint concept in gadak's mapping
    (`internal/linear/MAPPING.md`).

[^34]: The origin's issue model has no sprint field — the editable set
    carries none (`issuetap/docs/COMPATIBILITY.md:72`).

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

[^50]: `POST /issue` (`internal/jira/write.go:331`).

[^51]: Create works; assignee, labels, parent, and issue type are refused on
    create (`internal/origin/linearwriter.go:297`, `cmd/gadak/create.go:377`).

[^52]: `issuetap/docs/COMPATIBILITY.md:75`, with the same parent-hierarchy
    rule.

[^53]: ADF body with optional visibility and internal flag
    (`internal/jira/write.go:256`).

[^54]: Neither visibility nor internal (`internal/origin/linearwriter.go:84`);
    the body flattens ADF to plain text (`:91`).

[^55]: `visibility` plus the `sd.public.comment` internal mapping
    (`issuetap/docs/COMPATIBILITY.md:75`).

[^56]: `POST /issue/{key}/transitions` with fields and comment
    (`internal/jira/write.go:116`).

[^57]: Linear transitions carry no screen fields
    (`internal/origin/linearwriter.go:73`).

[^58]: `fields.resolution` and `update.comment` are honored, screen-checked
    (`issuetap/docs/COMPATIBILITY.md:70`).

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

[^67]: `edit --due YYYY-MM-DD` → `fields.duedate` (`cmd/gadak/edit.go:314`);
    Linear takes the date as-is (`internal/origin/linearwriter.go:169`).

[^68]: `edit --due none` → `fields.duedate = nil` (`cmd/gadak/edit.go:312`).

[^69]: "clearing a due date is not supported yet"
    (`internal/origin/linearwriter.go:171`).

[^70]: `fields.summary` / `fields.description` (`cmd/gadak/edit.go:305`,
    `internal/jira/write.go:286`).

[^71]: A formatted description is flattened to plain text on write
    (`internal/origin/linearwriter.go:155`). On every origin, `edit -m`
    refuses to destroy a formatting-carrying description without
    `--force-plain`.

[^72]: Any field outside Linear's editable set is refused
    (`internal/origin/linearwriter.go:190`).

[^73]: `edit --type` → `fields.issuetype` (`cmd/gadak/edit.go:301`).

[^74]: `ErrNoIssueTypes` — Linear has no issue types
    (`internal/origin/writer.go:133`).

[^75]: `issuetype` with allowed values (`issuetap/docs/COMPATIBILITY.md:72`).

[^76]: `create --parent` / `edit --parent KEY|none` → `fields.parent`
    (`cmd/gadak/create.go:528`, `cmd/gadak/edit.go:305`). Jira has no
    dedicated REST parent route — the edit fields path is the only road.

[^77]: Refused on create (`cmd/gadak/create.go:377`) and on edit
    (`internal/origin/linearwriter.go:190`).

[^78]: Same `fields.parent`, with hierarchy validation and honest 400s
    (`issuetap/docs/COMPATIBILITY.md:75`).

[^79]: `POST /issue/{key}/attachments` multipart (`internal/jira/write.go:449`).

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
    (`cmd/gadak/migrate.go:105`).

[^94]: `--to linear --team KEY` sends a mirror's issues into a Linear team
    through the Linear workspace the command runs in (`cmd/gadak/migrate.go:56`,
    `internal/migrate/linear.go`; GDK-1265): issues, comments, parents and
    relations land, idempotent on re-run via a `gadak-migrate: KEY` footer;
    change history, wiki pages, dev links, custom fields and sprints stay
    behind and the report says so. Jira is not a destination.

[^95]: The migrate command creates a fresh Built-in workspace as its target,
    which must not exist yet (`cmd/gadak/migrate.go:23`).

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

## How this file is maintained

This file is the single owner of the matrix — the READMEs summarize it and
link here. A commit that touches `internal/origin/writer.go`,
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
