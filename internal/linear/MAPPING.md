# Linear → gadak field mapping decisions (GDK-263)

Status: **proposal by the connector round, for the lead to promote into a
decision document.** Every Linear-side fact below was measured against the
live GraphQL API on 2026-08-18 (personal API key); every gadak-side fact is
cited to the repository. Where a field cannot be mapped truthfully, the
decision is "leave it empty" — a derived field that silently lies is worse
than a column that is honestly null.

Gadak contract fields are the `issues` / `items` columns in
`internal/store/schema.go` (schemaV1) as documented in
`specs/000-product/data-model.md`. The rules that constrain any mapping:

- status/priority/type logic keys on **ids or categories, never display
  names** (CLAUDE.md; the same hazard `internal/store/derive.go:9` records
  for Jira status names).
- `priority_rank` is the 1-based position in the source's most-urgent-first
  priority list; 0 = unset (`internal/store/derive.go:134`).
- `time-in-status` is computed from `status_changed_at`, never stored
  (CLAUDE.md; data-model.md "deliberately absent").
- The mirror holds no gadak-only original data; anything not present in the
  source is left empty, not synthesized.

---

## The table

### `status_category` ← `WorkflowState.type`

Linear `WorkflowState.type` is the stable axis (verified live). The spec's
premise "six types" is **wrong in one value**: this workspace has
`backlog, unstarted, started, completed, canceled, duplicate` — `duplicate`
exists and `triage` only appears on teams with triage enabled. The enum is
open: Linear added `duplicate` after the original six, so any consumer must
tolerate unknown values.

| WorkflowState.type | status_category | what is lost |
| --- | --- | --- |
| `backlog` | `new` | backlog vs triage vs fresh — indistinguishable |
| `unstarted` | `new` | (same) |
| `triage` (if present) | `new` | (same) |
| `started` | `inprogress` | — |
| `completed` | `done` | — |
| `canceled` | `done` | canceled ≠ completed |
| `duplicate` | `done` | duplicate ≠ completed |
| **unknown future type** | **see below — LEAD DECISION** | |

- **Grounds:** collapse is forced by gadak's three-value contract column;
  canceled and duplicate are terminal in Linear (workflow position after
  completed), so `done` is the only defensible bucket. The original survives
  verbatim in `status_id` (the WorkflowState UUID) plus `status` (display
  name), so a query that needs "done but not canceled" re-joins `status_id`
  against `WorkflowStates(teamID)` — that is exactly why this client exposes
  the catalog method.
- **Becomes a false mapping when:** someone filters `status_category='done'`
  and reads it as "shipped" — for Linear it means "closed, including
  canceled/duplicate". And when Linear ships a new type: silently bucketing
  an unknown type would be the display-name trap in a new coat. Recommended
  rule for the sync wiring: a type outside the known set maps to `new` **and
  increments a loudly-reported counter** (never a silent default). The
  alternative — refusing the row — would 0-row silently, which is the exact
  failure mode the constitution bans.

### `priority` / `priority_rank` ← `Issue.priority` (int) + `Issue.priorityLabel`

Linear: `0 = No priority, 1 = Urgent, 2 = High, 3 = Normal/Medium, 4 = Low`
(0/"No priority" verified live; 1–4 from Linear's documented scale).
Gadak: rank 1 = most urgent first, 0 = unset, "unset rank 0 sorts after
Highest when sorting by priority asc" (`e2e/identity-web.spec.ts:51`,
`internal/store/derive.go:134`, `web/src/lib/format.ts:148`).

- **Decision: identity mapping.** Linear's integer already *is* a
  most-urgent-first 1-based rank with 0 = unset: 1→1, 2→2, 3→3, 4→4, 0→0.
  Direction and unset-position match on both ends; nothing flips.
- `priority` (the name column) ← `priorityLabel`, display-only.
- **Becomes a false mapping when:** a mirror sorts Jira and Linear rows as
  one list — rank 2 then means "High" (Linear) and "second in the Jira site's
  list" (Jira), which agree only if that site uses the default five. That is
  a cross-source display question for GDK-262, not a reason to distort this
  connector's ranks. If Linear ever adds priority 5, identity still holds.

### `issue_type` / `issue_type_id` ← (nothing)

Linear has no issue-type concept (the 85-field `Issue` introspection carries
none). **Leave both empty.** A synthetic constant (`"linear-issue"`) would be
gadak-only original data and would make `issue_type_id = X` queries return
phantom truth for a field the source does not have. Consequence, accepted:
type filters and type facets return no Linear rows — that is correct.

### `key` ← `Issue.identifier` (`MID-1`-shaped)

Same shape as a Jira key (`TEAMKEY-number`), so a Jira `ENG-123` and a
Linear `ENG-123` can meet in one mirror. Storage does not collide —
`items_source_key` is UNIQUE on `(source_id, key)` (`schema.go`, schemaV1) —
but every **key-only** surface does: `watches.key` and `favorites.key` are
single-column PRIMARY KEYs, saved views and `gadak views open --keys -`
address issues by bare key, and deep links key on it. This is the same trap
GDK-241 records for id collisions (that issue could not be read this round —
see the report; the schema reading above is the verification).

- **Decision for this round: none to make** — the client returns
  `identifier` verbatim, as the constitution demands ("identifiers from the
  source are stored verbatim, never re-keyed"). Qualifying keys for display
  (`linear:ENG-123`) or collision-detecting at sync time is GDK-262's
  contract with the surfaces; both options need the surfaces' buy-in.

### `status_changed_at` ← derived from `Issue.stateHistory` (not fetched by `Issues()` today)

There is **no** status-changed timestamp on `Issue` itself (verified against
the full field list). Two derivable sources, both verified live:

- `Issue.stateHistory` → `IssueStateSpan { stateId, startedAt, endedAt }`:
  the span list is the issue's state timeline; the span with
  `endedAt == null` is the current state, so its `startedAt` is
  `status_changed_at`. Cleanest source, one nested paginated connection.
- `Issue.history` → `IssueHistory { createdAt, actor, fromState, toState,
  fromPriority, toPriority, … }`: maps 1:1 onto gadak's `changelog` rows
  (`field='status'`, `from_id`/`to_id` = state ids, `at` = createdAt), and
  `store.Derive` (`internal/store/derive.go:64`) then computes
  `status_changed_at`, `resolved_at`, `reopen_count` the same way it does for
  Jira — no Linear-specific derivation code needed.

Both connections are unbounded, which is why `Issues()` does not inline
them (a bounded inline fetch would truncate silently; the comments inline
page avoids this by exposing `HasNextPage`). **What Linear sync must do**
(GDK-262+ design): fetch `history` per issue (or `stateHistory`) with cursor
follow-up, feed the existing `Derive` path. Until then: leave
`status_changed_at` NULL — **time-in-status will not exist for Linear
issues**, and that is the honest state.

- Related verified fields that could shortcut some of this later:
  `Issue.startedAt` / `completedAt` / `canceledAt` (work-start / completion /
  cancellation instants, set per state type). `completedAt`+`canceledAt` can
  fill `resolved_at` without history — but not `status_changed_at`, and not
  reopens.

### `parent_key` / `epic_key` ← `Issue.parent`; Project/Cycle ← **unmapped**

- `Issue.parent { id, identifier }` → `parent_key`, same semantic (direct
  parent). True mapping, no caveat.
- `epic_key` stays NULL: gadak defines it as the
  hierarchy-level-1 ancestor, which is a Jira-ism. Treating a Linear parent
  as an epic is a false mapping whenever a team nests sub-tasks.
- `Project` and `Cycle` have **no Jira counterpart and no gadak column.**
  Forcing them into epic would be the classic false mapping. They survive
  only in `raw` if the sync stores it; exposing them is a product decision
  with a schema change attached (lead's), not a connector decision.

### Comments ← `Issue.comments` (nested connection, markdown bodies)

Comments come **nested on the issue** (`comments(first: N) { pageInfo nodes
}`) with their own cursor — verified live; a follow-up fetch pattern exists
the day an issue exceeds the inline page (`CommentsPageSize`, truncation
exposed via `HasNextPage`, never silent).

- `Comment.body` is **markdown** (verified; Linear also exposes `bodyData`
  structured payloads and `documentContent`, both out of scope). gadak's
  Jira path stores `body_adf` (ADF JSON via `internal/adf`) — **stuffing
  markdown into an ADF column would be a false mapping.** Linear comments
  belong in `body_text` (+ raw), and `body_adf` stays NULL until a
  `body_markdown`-style column or a renderer decision exists (GDK-262+).
- `attachments`: `Issue.attachments { nodes { id title url subtitle } }`
  exists (shape verified; `size`/`mimeType` are **not** fields there, unlike
  Jira). The workspace had no attachments, so the auth-requirements of
  attachment URLs could not be tested — unverified, flagged.

### The straightforward rows

| gadak | Linear | notes |
| --- | --- | --- |
| `items.title` | `Issue.title` | verbatim |
| `items.body_text` | `Issue.description` (markdown) | flatten markdown to text for FTS; do not store it as ADF |
| `items.url` | `Issue.url` | contains the org slug; deep link works in a browser session |
| `items.created_at` / `updated_at` | `createdAt` / `updatedAt` | verbatim ISO-8601 UTC with ms — the format the mirror already stores unmodified |
| `items.external_id` | `Issue.id` | UUID |
| `items.author` / `author_id` | `creator.name` / `creator.id` | reporter ← creator |
| `assignee` / `assignee_id` / `assignee_email` | `Issue.assignee` | NULL assignee is common |
| `labels` | `labels.nodes[].name` | Jira labels are name-keyed too; a renamed Linear label re-keys, same as Jira |
| `duedate` | `Issue.dueDate` | field exists; no value in the capture workspace, so the exact wire format (date-only vs datetime) is **unverified** — check before wiring |
| `project_key` | `team.key` | **proposal**: team is Linear's scope unit (the connector's `Teams` are what a Jira project maps to); `identifier`'s prefix is the team key, matching how `build()` falls back to the key prefix (`internal/sync/sync.go:623`). False-mapping case: a workspace using Linear *Projects* as the project axis — then team-key-as-project lies. Revisit when Project mapping is decided. |
| `resolution` | (nothing) | Linear has no resolution field; `completedAt`/`canceledAt` are instants, not resolutions. Leave empty. |
| `changelog` | `Issue.history` | see status_changed_at above — the rows land in the existing table unchanged |

## Deletion / archive semantics (reconcile input)

- Archived: `Issue.archivedAt` set; **rows are visible again when
  `includeArchived: true`** (the arg exists on `Query.issues`), so reconcile
  can distinguish archived from deleted. The client passes the flag through
  (`IssueOpts.IncludeArchived`).
- Deleted: Linear moves issues to trash (`Issue.trashed` field exists) and
  they disappear from the default listing. Trashing could not be exercised
  live (no mutations allowed), so **whether trashed rows ever appear in any
  listing mode is unverified**; the safe reconcile rule is the Jira one —
  absence from a full listing is deletion (`internal/sync/sync.go:527`
  documents the same proof-by-absence).

## What GDK-262 must provide before any of this syncs

1. **Credential storage**: one personal API key string per Linear source
   (`Authorization: <key>` bare — a "Bearer" prefix fails with an explicit
   400; this client's header shape is pinned by test). No site/email pair
   like Atlassian.
2. **A `sources` row**: `kind='linear'`, `base_url='https://api.linear.app'`
   (or empty — the endpoint is a constant), `id` a stable slug (`linear`).
   The workspace-is-bound-to-one-origin constitution article applies
   unchanged.
3. **Key-space policy**: decide how bare keys on key-only surfaces
   (`watches`, `favorites`, `views open --keys -`, deep links) disambiguate
   a Jira/Linear collision (GDK-241 class). Storage already tolerates it;
   the surfaces do not.
4. **Watermark format**: `sync_state.watermark` = Linear `updatedAt`
   string (ISO-8601 UTC ms, verbatim); incremental pass = `Issues(
   UpdatedAfter: watermark)` with the same minute-buffer the Jira path
   applies (`internal/sync/sync.go:35` drops same-minute updates; Linear's
   ms precision narrows but does not remove that hazard — `gte` re-reads the
   boundary row, which is cheap and idempotent).
5. **History fetch design** for `status_changed_at` / reopen fields (see
   above), or an explicit decision to ship Linear without time-in-status.
6. **A display story for markdown** bodies (comments, descriptions) —
   `body_adf` must not become a markdown container.
