# Semantic-axis inversion audit — 2026-09-10

The archetype (GDK-1204): the mirror's mapping and the display each inverted
one axis, the two inversions cancelled on the standalone path, and the bug
survived because every reader was on the path where it cancelled. The detector
for that class is not code reading — it is **origin-contract comparison plus
differential observation**: for each axis, at least two of {what Cloud's wire
says, what issuetap serves, what the mirror stores, what a surface renders}
are checked against each other, so an inversion that cancels on one path has
nowhere to hide on the other.

Scope (from the issue): A — mirror ingest/derive directions; B — every
issuetap registry `Supported` row whose field carries meaning
(changelog items, transitions, editmeta, comment author/updateAuthor,
resolution/resolutiondate, search render, plus the link-bearing rows);
C — presentation (CLI link line, web cross-links, board/ordering).
Already checked and excluded (2026-08-31): status-category folding —
single owner in Go (`internal/statuscat`) and web
(`web/src/lib/view-config.ts` `effectiveCategory`).

Verdicts: **일치(match)** / **반전(inverted)** / **미확인(unverified)** /
**분기(divergent)** — same concept, different rule in two owners. Nothing in
this audit was fixed: findings are candidates for the lead to file.
"Measured" means a command in the evidence column ran and its output is
quoted or summarized; "hypothesis" means code reading only.

## A — mirror ingest and derived fields

| axis | surface | where | verdict | evidence (command → result) |
|---|---|---|---|---|
| changelog from/to | ingest | `internal/sync/sync.go:1231-1234` | match | identity map `FromString→FromValue, From→FromID, ToString→ToValue, To→ToID`. issuetap writer records `From:` old, `To:` new (`store/store.go:2464-2467`). Measured: `sqlite3 examples/demo.db "SELECT i.key,c.at,c.from_value,c.to_value,i.status FROM changelog c JOIN issues i ON i.item_id=c.item_id WHERE c.field='status' ORDER BY c.at DESC LIMIT 6"` → newest `to_value` equals current status on all rows (NMA-144 In Progress→Done etc.) |
| epic_key (level-1 ancestor direction) | derive | `internal/store/write.go:133-144` | match | two-hop walk picks the `hierarchy_level=1` **ancestor** (child→epic). Measured: `SELECT count(*) FROM issues i WHERE i.epic_key IS NOT NULL AND (SELECT COALESCE(hierarchy_level,-99) FROM issues e WHERE e.key=i.epic_key)<>1` → **0**; epics with an epic_key → **0**; sampled rows all Story/Bug/Task lvl 0 → Epic lvl 1 |
| reopen rule (stored) | derive | `internal/store/derive.go:268-273` | match | `reopenTransition`: done→non-done **and** the inprogress→new sibling. Measured: NMA-1 changelog `Backlog→Done` then `Done→In Progress`, `reopened_at` equals the second row's `at`; NMB-3 `In Progress→Backlog` likewise. `go test ./internal/store -run 'Derive|Durations|Epic|OpenBlockers|Flow|Cycle' -count=1` → ok |
| reopen badge (web) | display | `web/src/lib/view-config.ts:956-961` | **divergent — candidate defect** | web `isReopen` accepts only `from_category==='done' && to_category!=='done'`; the stored rule (derive.go above) and the feed's `reopened` event (`internal/store/feed.go:341`, keyed on `ReopenedAt`) count inprogress→new too. An inprogress→new move is a red-event in the feed and `reopen_count=1` in SQL, with an unpainted dot in the timeline. See findings #1 |
| status_changed_at / durations boundary | derive + query | `internal/store/durations.go:45-88` | match | `Wait` = created → first entry **into** inprogress (keys on `ToID`); `Progress` = latest entry into inprogress → first done after it, else Now. Boundary shared, no gap or double count; reopen restarts the clock from the latest entry. Covered by the same `ok` test run above |
| dev_links PR status | ingest | `internal/jira/devstatus.go:22-60` | match — **hypothesis** (vocabulary) | OPEN/MERGED/DECLINED is the Cloud dev-panel vocabulary; GitHub `CLOSED`→DECLINED, else OPEN; stored lowercase. Measured gap, not a defect: `SELECT count(*) FROM dev_links` on `examples/demo.db` → **0** — the demo fixture never exercises the mapping (see findings #4) |
| sprint_state | ingest | `internal/sync/sprint.go:179-198` | match | lowercases to active/future/closed, picks active>future>closed, then larger id. Measured: `SELECT state,count(*) FROM sprints GROUP BY state` → one each of active/closed/future, lowercase |
| priority_rank sort direction | consumers | `internal/jira/client.go:405-424`; `cmd/gadak/list.go:99`, `cmd/gadak/recipes.go:26`, `internal/mcp/tools.go:96`, `web/src/stores/filters.svelte.ts:1059-1062` | match — with a caveat | rank 1 = most urgent everywhere: Go `order by priority_rank` (ASC implied), web `prioritySortRank` comment "lower = higher priority". Measured: `SELECT priority,priority_rank,count(*) … GROUP BY` → Highest=1 … Lowest=5. issuetap serves "most-urgent first" (`internal/api/registry.go:35`). Caveat: Cloud's `GET /priority` order is de-facto, not a documented promise — the whole scheme rides on it (미확인 edge, findings #5) |
| assignee / reporter / creator | ingest | `internal/sync/sync.go:1095-1098,1141-1146` | match | separate columns; item author = creator with reporter fallback; issuetap renders all three separately (`internal/api/render.go:206-208`); no swap |
| cloned_from | derive | `internal/store/derive.go:305-320` | match | outward+clone-type → target is the origin ("clones X" reads on the clone); fixed by GDK-1214, with the production-mirror 12/12 measurement recorded in the comment |
| open_blockers (link-direction consumer) | derive | `internal/store/flow.go:158-164` | match | counts **inward** blocking links whose target is not done. Measured: NMS-1 `Blocks inward NMB-3` (target new) → open_blockers 1; NMA-3 `Blocks outward NMB-3` (target new) → 0 — outward never blocks self; every inward-to-done row reads 0. Type resolution falls back to `["Blocks"]` when the catalog is empty (`flow.go:125-131`) |

## B — issuetap Jira dialect (registry Supported rows)

| axis | surface | where (issuetap) | verdict | evidence |
|---|---|---|---|---|
| changelog items render | `GET /issue/{key}?expand=changelog`, `GET …/changelog` | `internal/api/render.go:394-426` | match | renders `from/fromString/to/toString` as stored, re-localizing status/priority ids; writer direction verified in axis A. Conformance: `TestGadakSyncMirror` + api contract tests |
| transitions | `GET/POST …/transitions` | `internal/api/jira.go:943-987`, `store.go:2306-2325` | match — documented simplification | `to` is the **destination** status object (direction correct); transition `Name` is the destination status name where Cloud uses the workflow action label — a documented model simplification. Harmless to gadak: `internal/jira/pick.go:118` matches either the transition name or `To.Name`. Covered by `transitions_test.go` |
| editmeta | `GET …/editmeta` (Partial row) | `internal/api/jira.go:1051-1062` | match | passthrough of `st.EditMeta`; gadak consumes AllowedValues value-then-name (`internal/server/write.go:499`) — one consumer, both origins. Covered by `editmeta_test.go` |
| comment author/updateAuthor | `GET/POST …/comment` | `internal/api/render.go:332-350` | **fidelity gap — candidate, no gadak impact** | issuetap never renders `updateAuthor` (Cloud does, diverging from `author` after an edit); gadak's `jira.Comment` has no `UpdateAuthor` field (`internal/jira/types.go:81-89`), so nothing consumes it on either path — mirror-consistent, Cloud-fidelity gap only (findings #3) |
| resolution / resolutiondate | transitions + issue render | `store.go:2450-2460`; registry rows | match — with an unverified edge | issuetap does not serve `resolutiondate`; gadak never requests or reads it (field list `internal/sync/sync.go:50`; `resolved_at` is changelog-derived on both paths — differential-safe). Resolution lifecycle matches Cloud's post-function behavior: entering done without a resolution defaults 10000, leaving done clears it. Edge (미확인): a non-done status that carries a resolution would set Cloud's `resolutiondate` but leave gadak's `resolved_at` empty — unmeasurable without a real Cloud site |
| search render | `POST /search`, `/search/jql` | `internal/api/jira.go:480-483` | match | both routes render through the **same** `issueJSON` as `GET /issue/{key}` — a single owner, so search cannot drift from the issue fetch on links/changelog/authors |
| issuelinks (write projections) | `POST /issueLink`, `DELETE /issueLink/{id}` | `store.go:1550-1554` | match | `{inwardIssue:X, outwardIssue:Y}` stores `OutwardKey:Y` on X and `InwardKey:X` on Y — each side labels the **other end by its role**, Cloud's convention. Measured end-to-end by `TestGadakLinkRoundTrip` (`conformance/gadak_test.go:489-506`): after `gadak link TAP-1 TAP-3 --type blocks`, TAP-1 shows `outwardIssue TAP-3` and TAP-3 shows `inwardIssue TAP-1`, with the crossed shapes refused |
| issuelinks (doc wording) | docs + test comment | `docs/COMPATIBILITY.md:76`; `conformance/gadak_test.go:466-467` | **wording hazard — candidate** | both say "outward sees `outwardIssue`, inward sees `inwardIssue`". Read role-wise ("the outward issue sees…") that is the **opposite** of the code and of the assertions two lines below; it is only true read as "the side whose page shows the outward description sees `outwardIssue`". A reader who "fixes" the code to match the role-wise reading reintroduces GDK-1204. See findings #2 |
| priority list order | `GET /priority` | `internal/api/registry.go:35` | match | serves most-urgent first, the order gadak's `priority_rank` counts from — the two sides state the same contract |

Registry rows not named above carry no direction-bearing semantics
(paging envelopes, CRUD, attachments, agile board/sprint mechanics,
Confluence) — they were walked for this audit and are shape- not
meaning-bearing; the conformance suite and the api `contract_test.go` hold
them.

## C — presentation

| axis | surface | where | verdict | evidence |
|---|---|---|---|---|
| CLI issue-view link line | `gadak issue` | `cmd/gadak/agent.go:782-793` | match | renders the type's own sentence via `origin.LinkPhrase`, falling back to the wire pair (`Blocks outward KEY`) when the mirror's link-type catalog lacks the type — demo.db's `link_types` is empty, so the fallback is what the demo mirror prints. The label is our direction token, not a Jira sentence, and the code says so (GDK-1734 note) |
| LinkPhrase direction selection | origin | `internal/origin/linkresolve.go:90-102` | match | `inward`→type.inward, `outward`→type.outward — matches the ingest convention (element labels the other end by its role) and `link.go`'s write-side comment (`cmd/gadak/link.go:55-60`). Catalog tuple order verified: `LinkTypePhrases` returns `{inward, outward}` and the caller passes `lt[0], lt[1]` positionally (`internal/store/flow.go:79-94`, `agent.go:789`) |
| web cross-links label | LinkedIssues/RelatedIssues | `web/src/lib/link-label.ts:34-47` | match | phrase ladder: backend `phrase` → client catalog row by direction → type name → caller word; no rung renders the direction token (GDK-1215); same direction→description rule as `LinkPhrase` |
| history timeline direction | detail panel | `web/src/components/detail/HistoryTimeline.svelte:40-45` | match (direction) | renders `from → to` chronologically with categories preferred over names; reopen dot rule diverges — see axis A |
| board lanes | board | `web/src/components/board/BoardView.svelte:49,103-123` | match | status-category lanes padded in canonical forward order `new → inprogress → done`; grouping and sort are the list's own `buildGroups`/sort — one owner |
| priority sort | list/board | `web/src/stores/filters.svelte.ts:1059-1062` | match | lower rank sorts first (higher priority), unset ranked explicitly after; same direction as the Go `order by priority_rank` consumers |

## Findings — candidates for the lead (nothing was fixed here)

1. **Reopen rule diverges between the stored/feed rule and the web timeline
   badge** (분기). `internal/store/derive.go:268-273` and the feed event
   (`internal/store/feed.go:341`) count `inprogress→new` as a reopen — the
   derive comment records that 59% of real reopens on one production mirror
   were that shape. `web/src/lib/view-config.ts:956` badges only
   `done→non-done`. A user watching the timeline sees an unpainted dot for an
   event the feed calls `reopened` and SQL counts in `reopen_count`. This is
   the same class as GDK-1204 in miniature: two owners of one concept with
   no shared definition. Measured: both rules read from source with the
   divergence pinned by the files above.
2. **"outward sees outwardIssue" wording reads backwards role-wise**
   (`issuetap docs/COMPATIBILITY.md:76`, `conformance/gadak_test.go:466-467`).
   The code, the render comment (`render.go:180-182`) and the assertions
   (`gadak_test.go:489-506`) all implement and verify the Cloud convention;
   the prose summary of that convention inverts under the natural reading.
   The hazard is a future "fix to match the docs" reintroducing the
   GDK-1204 mirror image. Suggest rewording to "each element labels the
   other end by that end's role" (the render comment's own phrasing) —
   issuetap-side edit, lead's call.
3. **issuetap does not render `updateAuthor` on comments** — Cloud fidelity
   gap, no gadak consumer (`jira.Comment` has no such field), so it is
   invisible end-to-end today. Noting it so the fidelity ledger knows; not a
   gadak defect.
4. **The demo fixture has zero `dev_links` rows** (`SELECT count(*) FROM
   dev_links` → 0), so the PR-status mapping (axis A) is exercised by tests
   but never by the fixture the dogfooding mirror reads. If dev-panel
   status ever matters in demos/e2e, the fixture needs a PR link — a
   fixture-gap note, not a defect.
5. **Cloud's `GET /priority` ordering is de-facto, not a documented
   promise** — `priority_rank`'s entire direction hangs on "most urgent
   first". issuetap serves that order deliberately; Cloud observed to. If a
   site ever returns a different order, every rank silently flips. A
   defensive note for the ledger; no action proposed (미확인 — would need a
   real Cloud site to probe).
6. **`resolved_at` vs Cloud `resolutiondate` edge** — gadak derives
   resolution time from entry into the done category; Cloud stamps when a
   resolution is set. Identical on standard workflows; a custom workflow
   whose non-done status carries a resolution would disagree. 미확인
   (unmeasurable without a Cloud site; issuetap cannot express it either).

## What this audit did not do

- No code was changed; no gates were run for this document ("no gates for
  this round" — the evidence commands are read-only queries and test runs).
- The issuetap checkout (the standalone origin's source tree, adjacent to
  this repo) was read, never executed or modified; its conformance
  assertions are cited as the standalone-side ground truth. File references
  in axis B are relative to that checkout.
- The connected-Cloud half of each differential rests on gadak's client
  code, the issuetap conformance suite, and Atlassian's documented wire —
  no live Cloud site was probed (that is what 미확인 marks).
