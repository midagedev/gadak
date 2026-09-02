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

---

## 쓰기 매핑 (GDK-360)

`write.go`의 세 mutation 동사. 읽기 매핑의 규칙이 그대로 적용된다 —
**상태는 `stateId`(WorkflowState UUID), 우선순위는 0-4 정수, 절대 display
name으로 키하지 않는다.**

| 동사 | Linear mutation | 입력 필드 | 비고 |
| --- | --- | --- | --- |
| `CreateIssue` | `issueCreate(input:)` | `teamId`*, `title`*, `description`, `stateId`, `priority`, `assigneeId`, `labelIds`, `dueDate` | \* = 필수(클라이언트가 먼저 검사). 0값 문자열·빈 슬라이스는 입력에서 **생략** — Linear 기본값 적용. 반환 Issue는 읽기 쿼리와 같은 필드 셋이라 미러가 refetch 없이 커밋 가능 |
| `UpdateIssue` | `issueUpdate(id:, input:)` | `title`, `description`, `stateId`, `priority`, `assigneeId`, `labelIds`, `dueDate` | nil = **키 생략** = 미변경. `assigneeId`에 빈 문자열 포인터 = **명시적 null** = 해제. `labelIds`는 슬라이스가 **전체 집합을 교체**(빈 슬라이스 = 라벨 전체 제거) |
| `CreateComment` | `commentCreate(input:)` | `issueId`, `body` | body는 markdown(읽기의 `Comment.body`와 동일; ADF 아님 — 위 "comments" 절) |

- **null 근거**: 문서화된 스키마(Apollo Studio `IssueUpdateInput` 참조,
  2026-08-20 확인) — *"All fields are optional; only provided fields will be
  updated. Setting a field to null (where supported) will clear the field's
  value."* 생략=미변경 / null=해제 / 값=설정의 3분법이 그대로 입력 인코딩이다.
  실측은 불가(캡처 워크스페이스에 mutation 금지) — 테스트는 hand-built
  fixture로 재생한다.
- **priority**는 읽기 매핑의 identity 규칙(0=없음, 1=Urgent … 4=Low)을 그대로
  쓰고, 클라이언트가 0-4 밖 값을 전송 전에 거절한다.
- **dueDate**는 `"YYYY-MM-DD"`만 허용(클라이언트 검증). 이 형태로는 due date
  **해제**(명시적 null)를 표현할 수 없다 — 어댑터(GDK-361)가 필요하면
  `AssigneeID`의 빈-문자열 관례를 확장한다.
- **재시도**: mutation은 쓰기 재시도 정책(`httppolicy.IsRetryableWrite`,
  429·503만; 전송 실패도 재시도 않음)을 탄다 — 500은 "이미 적용됐고 응답만
  유실했다"일 수 있어 재시도하면 이중 생성. jira `write()`와 같은 규율.
- **success=false**는 애플리케이션 수준 거부이며 에러로 변환한다( GraphQL
  errors 배열과 별개 경로).

## 미러 → Linear 이전 매핑 (GDK-1265, `gadak migrate --to linear`)

읽기 매핑을 거꾸로 탄다 — 키는 **계약 축**(`status_category`·
`priority_rank`), display name이 아니다. `internal/migrate/linear.go`.

| 미러 | Linear | 비고 |
| --- | --- | --- |
| `status_category` new / inprogress / done | 팀 workflow state 중 type `backlog\|unstarted\|triage` / `started` / `completed` — 각 type에서 position 최소 | canceled·duplicate 로는 절대 보내지 않는다(done ≠ 취소) |
| `priority_rank` 0,1-4 / ≥5 | `priority` 0,1-4 / 4 | 5 이상은 Low 로 접히고 건수 보고 |
| `issue_type` 이름, `labels` | 팀 라벨(이름 일치 재사용, 없으면 `issueLabelCreate`) + 마커 라벨 `gadak-migrate` | 워크스페이스 라벨도 이름 충돌하므로 `issueLabels` 무필터 조회 |
| `parent_key` | `parentId` (부모 먼저 생성) | Epic 은 라벨 `Epic` 인 부모 이슈 — Project 매핑은 스코프 밖 |
| `links.type` Blocks / Duplicate / 그 외 | `issueRelationCreate` blocks / duplicate / related | outward = 나→대상, 쌍당 1건. 실측 2026-09-02: duplicate 관계를 만들면 Linear 가 중복 쪽 이슈를 `Duplicate` 상태(type duplicate)로 옮긴다 — 카테고리는 done 그대로 |
| 코멘트 | `commentCreate` + `createdAt`, 본문 앞 `**작성자 · 시각**` | 대필 불가 — 생성자는 API 키 사용자 |
| `created_at` | `IssueCreateInput.createdAt` | 서버가 무시하면 1회 경고 |
| 설명 | 평문 + 푸터 `gadak-migrate: <KEY>` | 멱등성 키: 재실행 전 팀 이슈를 훑어 일치 건은 건너뜀 |
| 첨부 | `attachmentCreate(url)` — 소스 URL 또는 Jira `…/attachment/content/<id>` | 바이트 업로드 없음 |
| changelog·위키 페이지·dev links·custom·sprint | **이전 불가** | Linear 에 쓰기 API 없음 — 보고서 "not migrated" |
