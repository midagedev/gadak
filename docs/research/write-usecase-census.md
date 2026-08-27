# Write-path census: two surfaces, and a usecase layer

Date: 2026-08-23
Issue: GDK-666
Status: design input. No production code was changed.

The `file.go:NNN` citations below are a **snapshot of the tree on this
document's Date**, above. They are not maintained and they drift with
every insertion above them — `edit.go:251-260` in §9 was already off by
sixteen lines by 2026-08-27 (GDK-962 inserted a flag). Read them as "roughly
here, in that tree"; find the current site by the symbol name, which is
stable, not by the number.

What came out of it, so a reader lands on the follow-through rather than on
the census: GDK-681 (two HTTP write handlers bypass owner routing — filed
Highest after the lead confirmed it in the code, and it also corrects one
overstatement below: `handleAssignee` and `handleLabels` do route through
`keyWriter`) and GDK-682 (the remaining six user-visible drifts from §2).
The verdict in §5 — extract one verb at a time, never a god package, fix the
drifts as plain bugs first — is the part to read if you only read one section.

This round does not extract a layer. It records what the two write
surfaces actually do, which differences are intended, which have already
bitten, and what a usecase layer would have to look like if the next
round starts moving verbs.

Scope is **origin write-through** (Jira / issuetap / Linear / wiki). Out
of census: personal views/watches/favorites, history visits, settings,
credential storage, sync, resync (re-read only), MCP (no write tools),
and `gadak api` (raw origin passthrough).

---

## 0. Premises, corrected

Lead measurements that still hold (line counts re-checked 2026-08-23):

| Path | Lines |
| --- | ---: |
| `internal/server/write.go` | 1472 |
| `cmd/gadak/edit.go` | 812 |
| `cmd/gadak/create.go` | 615 |
| `cmd/gadak/page.go` | 318 |
| `cmd/gadak/agent.go` | 2545 |
| `cmd/gadak/link.go` | 81 |
| `cmd/gadak/attach.go` | 128 |
| `internal/server/link.go` | 137 |

Corrections against the lead brief and against living docs:

1. **`agent.go` is not "mostly reads".** Bytes 1–1458 are issue/search/open
   plus shared helpers; 1459–2397 are the write session
   (`withCreateSession`, `withKeyWriteSession`, `emitAfterWrite`, `mutate`)
   and the verbs `comment` / `transition` / `close` / `assign` / `claim`.
   Comparing `agent.go` 2545 vs `write.go` 1472 as two copies of the write
   path is the wrong contrast. The real second copy is
   `edit.go`+`create.go`+`link.go`+`attach.go`+`page.go` plus those
   `agent.go` write verbs.
2. **HTTP write is not only `write.go`.** `POST {key}/link/` and
   `GET {key}/linktypes/` live in `internal/server/link.go` (GDK-85,
   4e12f21). `handleCreate`, `handleFields`, `handleAssignee`,
   `handleLabels`, and the three wiki handlers are in `write.go` but were
   missing from the brief's handler list.
3. **`docs/decisions/0008-cli-first-parity.md` addendum (2026-08-21) is
   stale.** It says REST has no issue-link endpoint. `server.go:270`
   registers `POST …/{key}/link/`.
4. **`specs/000-product/contracts/api.md` Write-through is stale in three
   places.** It still names the re-read `sync.SyncIssue` (production
   mutate uses `sync.RefreshIssue`; only `handleCreate` still calls
   `SyncIssue` with a reused client). It still says
   `<key>/comment/` `attachment_ids` are ignored; `handleComment` now
   resolves them through `origin.AsMediaRef`. It does not list the link
   route.
5. **A usecase layer already exists, piecemeal.** There is no
   `internal/write/` package. There are per-verb owners:
   `internal/transition`, `internal/create`, `internal/claim`,
   `internal/parenthint`, `internal/origin` (`Writer`, `ResolveLinkType`),
   `internal/sync.RefreshIssue`. The remaining copies are the verbs those
   packages have not taken yet.

### 0.1 Pieces already paid (git log)

| GDK | Commit | What moved |
| --- | --- | --- |
| GDK-218 | `3228e57` | `internal/create` — project/type/priority resolution, both surfaces |
| GDK-341 | `a401b73` | REST transition accepts the CLI identifiers; resolver → `internal/jira.PickTransition` |
| GDK-328 | `4de0ca8` | REST parent create + `PUT {key}/parent/`; `fields.IssueKeyLiteral` |
| GDK-359 | `f9177e1` | `origin.Writer`; server mutate closures take the interface |
| GDK-577/578 | `325b5a9` | `internal/transition.Apply` — one core for CLI (incl. `close`) and REST |
| GDK-591 | `b1dfa2e` | `internal/claim.Apply` — CLI today, package comment says REST later |
| GDK-635 | `4476ad8` | parent-rejection hint → `internal/parenthint` |
| GDK-642 | `7bf39c5` | two `refreshAfterWrite` copies → `sync.RefreshIssue`; lock: `TestRefreshIssueOwnerIsUnique` |
| GDK-85 | `4e12f21` | `origin.ResolveLinkType`; REST link route. The commit message is the mechanism of this issue: **package main cannot be imported, so the HTTP handler copied the CLI resolver until the owner moved** |
| GDK-665 | `be1cb78` | `origin.Writer` speaks origin DTO names; `internal/origin/dto.go` aliases the Jira payload structs so HTTP JSON does not change |

The pattern that has actually closed drift: **one owner outside
`package main`, both surfaces call it, a structural test fails on a third
copy.** Parity tests between two copies (the pre-GDK-85 link resolver)
only notice drift after it exists.

---

## §1. Verb-by-verb contrast

Shared plumbing, cited once so the rows can stay on differences.

| Piece | CLI | HTTP |
| --- | --- | --- |
| Key normalize | `normalizeKey` `cmd/gadak/agent.go:115` (`ToUpper`+trim) | path `{key}` as sent; parent/link bodies `ToUpper` after trim |
| Credential + source routing | `withKeyWriteSession` `agent.go:1509-1537` (`KeySource`, Linear vs Atlassian gate, `origin.WriterFor`) | `keyWriter` `write.go:235-258` (same gate, `key_ambiguous` → 409) |
| Write then re-read | `mutate` `agent.go:1567-1574` → `emitAfterWrite:1543` → `sync.RefreshIssue` | `mutate` `write.go:263-282` → `RefreshIssue` → `respondIssue` |
| Refresh failure | prose `write applied to %s, but the mirror did not refresh (run \`gadak sync\`)` `agent.go:1545` | `502 write_applied_mirror_stale` `write.go:274-279` |
| Origin failure | `error.Error()` (Jira/Linear/pairing folded by `origin.FoldPairedError`) | `failJira` `write.go:87-143` (snake_case codes + `jira_errors` + parent hint in `error`) |
| Success (issue writes) | tab line `summaryLine` or `--json {"issue": IssueLite, …}` | `200 {"issue": IssueLite, …}` from `respondIssue:284` |
| Visits / toast / audit | none on these paths (`warnIfStale` on stderr only) | none on these paths (web toast is the client of the HTTP response; recents are a separate POST) |

Those last three rows — failure narrative, success encoding, toast — are
**intended surface differences** everywhere they recur. Rows below do not
repeat them unless a verb diverges from this shape.

`gadak close` is not a separate origin verb. It is `applyTransitionWrite`
with target `"done"` (`agent.go:1987-2009`). REST has no `/close/`.

### How to read a row

Seven axes. A cell that says "동일" means the two implementations agree
on that axis. Every other cell is a difference, then a verdict
(의도 / 드리프트 / 판정 불가).

---

### 1. create

CLI: `cmdCreate` / `createOn` / `createOne` `cmd/gadak/create.go:39,255,362`
HTTP: `handleCreate` `internal/server/write.go:1034-1148`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI: positional summary, `-m`/`-` stdin, `--field alias=value`, `--attach`, `--label` (unsigned), `--priority` name-or-id, `--parent`, `--due`, `--batch -`. HTTP: JSON `project_key`/`issue_type`/`summary`/`description_text`/`assignee_account_id`/`priority_id`/`labels`/`duedate`/`parent`. HTTP has no custom-field or attach keys. CLI has no create-time assignee. | 의도 (UI sends ids; CLI resolves names) **except** the missing HTTP custom-field/attach keys, which are capability gaps — 드리프트, see origin |
| 사전 검증 | Both: `create.Project` / `Type` / `MetaForWithCatalog`, `fields.DateOnlyLiteral`, `IssueKeyLiteral`. HTTP additionally `project_not_mirrored` (`write.go:1067-1071`) if the resolved project is not in `cfg.Projects`. CLI `TestCreateOutsideMirrorPrintsKeyAndExitsZero`: writes, warns, exit 0. HTTP empty summary is `400 project_issue_type_and_summary_required` (name kept for i18n). CLI empty summary is usage. | `project_not_mirrored` vs warn+succeed: **드리프트**. Wire-code name vs usage: 의도 |
| origin 호출 | Both: `CreateIssue` with `project`/`issuetype`/`summary` plus optional description/labels/priority/parent/due. CLI `--field` resolved via `CreateFields`+`resolveCreateAliasFields` (`create.go:438-457`). HTTP cannot send those fields. CLI `--attach` after create. HTTP no attach. HTTP `assignee_account_id` in the create payload; CLI must `assign` after. **Routing:** CLI `withCreateSession`→`resolveCreateSource`→`WriterFor` (`agent.go:1459-1503`), Linear-only proven by `TestCreateLinearOnlyRoutesToLinear`. HTTP `s.client()` (`write.go:1058`) is `origin.Client` — Jira/issuetap only. | custom fields, attach, Linear routing: **드리프트**. assignee-on-create: 의도 (picker vs second verb) |
| 미러 갱신 | CLI `emitAfterWrite` → `RefreshIssue`. HTTP `sync.SyncIssue(…, Options{Client: c})` `write.go:1138` (reuses the Jira client; never Linear). Refresh failure is the same *class* (`write_applied_mirror_stale` / "write applied…"). | Linear-incapable refresh is a consequence of the routing drift. Jira re-read: 동일 in effect |
| 실패 서사 | Shared `Need*` mapped: CLI `formatCreateError` (`create.go:498`, flag names); HTTP `failCreate` / `failCreateOrPairing` (`write.go:989-1029`, `project_required` / `issue_type_required` / `priority_required`). Parent 400: both `parenthint.Wrap`. | 의도 (flag names vs wire codes). Pairing probe: 동일 (GDK-453) |
| 성공 출력 | CLI text: key or `KEY\tsummary`; `--json`: `{issue, created, resolved, attached?}`. HTTP: `{issue, resolved}` 200. CLI not-mirrored: still prints the key. | shape: 의도. not-mirrored success: **드리프트** (same as 사전 검증) |
| 부작용 | CLI `warnIfStale`. HTTP none. Web client toasts + `recordRecent` (not the server). Version bump via the re-read. | 의도 |

### 2. transition (CLI `close` = target `done`)

CLI: `cmdTransition` / `applyTransition` `agent.go:1936,2037`
HTTP: `handleTransition` `write.go:533-563`
Core: `transition.Apply` `internal/transition/apply.go:94` — **already one owner**.

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI: positional target (words joined), `--resolution`, `--field key=JSON`, `-m`/`-`, `--batch -`, `--dry-run` (batch only). HTTP: `{transition_id, fields, comment, resolution}`. Empty HTTP body → `transition_id_required`. `transition_id` is a name; Apply accepts id/status-id/name/category (GDK-341). | extra CLI batch/dry-run/list: 의도. Identifier vocabulary: 동일 |
| 사전 검증 | Same `Apply` (required screen fields, resolution catalog, category no-op GDK-632). CLI with no target lists transitions (`listTransitions` `agent.go:2159`) instead of usage (GDK-466). REST listing is `GET {key}/transitions/` (`handleTransitions:509`), a separate read. | 의도 (CLI UX vs REST GET) |
| origin 호출 | 동일: `transition.Apply` → `Writer.Transition`. | 동일 |
| 미러 갱신 | REST `mutate` **always** `RefreshIssue`, including `Changed=false`. CLI `emitTransitionResult` (`agent.go:2114`): noop + text prints `already %s` and **does not refresh**; noop + `--json` does refresh. | noop refresh: **드리프트** (version bump / delta on REST, not on CLI text) |
| 실패 서사 | `transition.IsRefused` → CLI `formatTransitionError` (appends `--resolution`/`--field` names `agent.go:2146`); REST wraps as `jira.APIError{Status:400}` `write.go:553-556`. | 의도 |
| 성공 출력 | Both include `changed`. CLI text is the summary line (or the noop sentence). | 의도 |
| 부작용 | Noop REST still bumps the mirror version. CLI text does not. | same as 미러; **드리프트** |

### 3. comment

CLI: `cmdComment` / `postComment` `agent.go:1770,1831`
HTTP: `handleComment` `write.go:567-643`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI: `KEY [text]` or `-m`/`-`, `--visibility role=\|group=`, `--internal`, `--batch -`. Mentions parsed from `@Name` (`resolveCommentMentions` `agent.go:1588`, shortest-first, GDK-510). HTTP: `{text, mentions[{account_id,display_name}], attachment_ids, visibility, internal}`. No `@` parser. | 의도 (UI autocomplete vs CLI @parse) |
| 사전 검증 | Both refuse empty text. HTTP `visibility_needs_role_or_group`. CLI `--visibility` once, same role/group shape. | 동일 in substance |
| origin 호출 | Both `AddComment`. CLI: `jira.Doc(body, mentions)`. HTTP: `jira.DocWithMedia(text, mentions, media)` after `AsMediaRef` per `attachment_ids` (`write.go:615-631`); unresolved media ids are dropped, comment still posts. CLI has no attachment-ids path. | media-in-comment: 의도 (upload-then-comment is the web flow). Drop-on-media-miss: 의도 (comment still lands) |
| 미러 갱신 | 동일 (`mutate` / `RefreshIssue`) | 동일 |
| 실패 서사 | 의도 (prose vs codes). CLI ambiguous `@Name` refuses the write (`TestCommentMentionAmbiguousRefusesWrite`); HTTP never sees that case. | 의도 |
| 성공 출력 | HTTP extra `comment.{comment_id,author,body,created_at}` `write.go:637-640`. CLI extra omits `created_at` (`agent.go:1841-1844`). | `created_at` missing on CLI `--json`: **드리프트** (agent-visible JSON) |
| 부작용 | CLI `warnUnresolvedMentions` on stderr (zero-hit `@` stays plain text). HTTP none. | 의도 |

### 4. attach / upload

CLI: `cmdAttach` `cmd/gadak/attach.go:43`
HTTP: `handleUpload` `write.go:647-685` (does **not** call `mutate`/`respondIssue`)

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI: `KEY file…`. HTTP: multipart field `file`, `maxUpload = 64<<20` (`write.go:37`). | 의도 |
| 사전 검증 | CLI `validateAttachPaths` (exists, regular file, follows symlink) **before** any upload. HTTP `file_required` if no form file; size capped by `MaxBytesReader`. | CLI multi-file preflight: 의도. REST no multi-file: 의도 |
| origin 호출 | Both `Writer.Upload`. CLI loops, stops on first failure (`attachPartialError` names landed vs not). HTTP one file. | 의도 (CLI batch vs REST one-shot) |
| 미러 갱신 | Both `RefreshIssue`. HTTP on failure → `502 write_applied_mirror_stale` (`write.go:669-671`). CLI via `emitAfterWrite`. | 동일 class |
| 실패 서사 | 의도 | 의도 |
| 성공 출력 | CLI: summary line + `attached: [{id,filename}]` (and extra `+ filename` lines unless `--json`). HTTP: `{attachments: [{id,filename,mime_type,size,media_id:"",is_image,is_video,content_url}]}` — **no `issue`**. | 의도 (web needs ids for the next comment; CLI wants the row). Contract table still implies write-through returns IssueLite — document gap, not a silent bug |
| 부작용 | 없음 | 동일 |

### 5. assign

CLI: `cmdAssign` / `assignTo` / `resolveAccount` `agent.go:2209,2243,2339`
HTTP: `handleAssignee` `write.go:893-905`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI: `email\|name\|accountId\|-` (words joined), member directory for Jira, `SearchUsers`, Linear UUID passthrough. HTTP: `{account_id}` (empty/null unassigns via `deref`). | 의도 (picker vs human token) |
| 사전 검증 | CLI resolution can refuse ambiguous/zero hits. HTTP trusts the id. | 의도 |
| origin 호출 | 동일 `SetAssignee` | 동일 |
| 미러 갱신 | 동일 | 동일 |
| 실패 서사 | 의도 | 의도 |
| 성공 출력 | 의도 | 의도 |
| 부작용 | 없음 | 동일 |

### 6. summary

CLI: `gadak edit --summary` → `applyEditChange` `edit.go:224-228` → `EditIssue`
HTTP: `handleSummary` `write.go:807-828` → `UpdateFields`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI flag; HTTP `{summary}`. Both trim. | 의도 |
| 사전 검증 | Both refuse empty. HTTP also `utf8.RuneCount > 255` → `summary_too_long` (`write.go:747,820`). CLI has no 255 gate (origin 400). | local 255: **드리프트** of *where* it fails, same likely origin cap — 판정 불가 whether Jira ever accepts >255 (Cloud documents 255). REST users see a stable code; CLI users see Jira prose |
| origin 호출 | CLI `EditIssue(fields, update)` (one PUT, possibly with other axes). HTTP `UpdateFields({"summary": …})`. Same Jira PUT when summary is the only field (`jira.Client.UpdateFields` / `EditIssue` `internal/jira/write.go:267-283`). | 동일 on the wire when alone; CLI may bundle |
| 미러 갱신 | 동일 class | 동일 |
| 실패 서사 | 의도, plus the 255 code | 의도 + 드리프트 of the 255 path |
| 성공 출력 | 의도 | 의도 |
| 부작용 | 없음 | 동일 |

### 7. description

CLI: `edit -m` / `-` / empty clears `edit.go:231-236`
HTTP: `handleDescription` `write.go:830-855` (`null` or whitespace clears; string wrapped with `jira.Doc`)

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI stdin `-`. HTTP JSON string or `null`. Both plain text → ADF. Neither accepts rich ADF here. | 의도 |
| 사전 검증 | HTTP `invalid_body` if missing. CLI empty `-m` is a clear, not a usage error. | 의도 (`-m` empty = clear; REST `""`/`null` = clear) |
| origin 호출 | CLI `EditIssue` fields.description; HTTP `UpdateFields`. | 동일 when alone |
| 미러 갱신 | 동일 | 동일 |
| 실패 서사 | 의도 | 의도 |
| 성공 출력 | 의도 | 의도 |
| 부작용 | 없음 | 동일 |

### 8. labels

CLI: `edit --label +x/-x` → Jira `update.labels` add/remove `edit.go:238-239,447-453`
HTTP: `handleLabels` `write.go:859-872` full replace (`normalizeLabels`, documented in `api.md:333`)

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI signed tokens; bare value refused (`TestEditLabelBareValueRejected`). HTTP array replace; `[]` clears. | 의도 (complementary APIs, documented). An agent that POSTs REST thinking `+x` works will not — that is a skill/docs problem, not a silent second implementation of the same op |
| 사전 검증 | CLI `+`/`-` required. HTTP `labels` key required. | 의도 |
| origin 호출 | CLI `EditIssue` `update`. HTTP `UpdateFields` `fields.labels`. Linear: CLI `EditIssue` with update ops → `linear: label add/remove operations are not supported`; HTTP `labels` is not in Linear `UpdateFields` switch → `linear: field "labels" is not editable`. Both refuse Linear labels, different sentences. | Linear sentence: **드리프트** of copy, same outcome. Jira: 의도 (delta vs replace) |
| 미러 갱신 | 동일 | 동일 |
| 실패 서사 | 의도 | 의도 |
| 성공 출력 | 의도 | 의도 |
| 부작용 | 없음 | 동일 |

### 9. priority

CLI: `edit --priority NAME-or-id` → `create.Priority` then `fields.priority {id}` `edit.go:267-276`
HTTP: `handlePriority` `write.go:726-742` — `priority_id` only; `null`/`""` sends `priority: nil`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI name or id (localized names go through the catalog). HTTP id only (`api.md:336`: names not accepted). | 의도 |
| 사전 검증 | CLI empty is `NeedPriorityError`. HTTP empty **clears**. CLI has no `none` sentinel (unlike `--due`/`--parent`). | clear: **드리프트**. On Linear, REST nil → priority 0 (`linearwriter.go:159-167`); CLI cannot issue that write |
| origin 호출 | Both `priority: {id}` or CLI never nil. HTTP `UpdateFields`; CLI `EditIssue` fields. | 동일 when setting |
| 미러 갱신 | 동일 | 동일 |
| 실패 서사 | 의도 | 의도 |
| 성공 출력 | 의도 | 의도 |
| 부작용 | 없음 | 동일 |

### 10. duedate

CLI: `edit --due YYYY-MM-DD\|none` `edit.go:158-170,269-274`
HTTP: `handleDuedate` `write.go:749-771`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI `none` vs HTTP `null`/`""`. Both `fields.DateOnlyLiteral`. | 의도 (sentinel spelling) |
| 사전 검증 | 동일 (invalid date never reaches origin). Linear clear: origin refuses (`linear: clearing a due date is not supported yet`) on both. | 동일 |
| origin 호출 | 동일 family (`duedate: raw \| nil`) | 동일 |
| 미러 갱신 | 동일 | 동일 |
| 실패 서사 | 의도 | 의도 |
| 성공 출력 | 의도 | 의도 |
| 부작용 | 없음 | 동일 |

### 11. parent

CLI: `edit --parent KEY\|none` `edit.go:172-184,262-267` + `parenthint` via `withParentHint`
HTTP: `handleParent` `write.go:773-805` + `parenthint.Wrap`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | Both `IssueKeyLiteral` + upper. CLI `none`; HTTP `null`/`""`. | 의도 |
| 사전 검증 | HTTP self-parent 400 before origin (`write.go:790-794`, `TestEditParentRejectsSelfBeforeJira`). CLI has **no** self-check; origin 400 (localized). | self-parent: **드리프트** of *when* it fails and of the message. Outcome (refuse) is the same |
| origin 호출 | Both `parent: {key} \| nil`. Hint owner is `internal/parenthint` (GDK-635). | 동일 |
| 미러 갱신 | 동일 | 동일 |
| 실패 서사 | 의도, except self-parent (REST stable sentence vs Jira localized) | self-parent copy: **드리프트** |
| 성공 출력 | 의도 | 의도 |
| 부작용 | 없음 | 동일 |

### 12. components

CLI: `edit --component +Name/-Name` `edit.go:241-242,456-461` + `withComponentHint` on origin 400
HTTP: no dedicated route. `PATCH {key}/fields/` with alias `components` (`fields.builtinEditable` `internal/fields/editable.go:24-26`, `TestComponentsBuiltinEditMetaAndWrite`) is a **full replace** of the array by id, not +/-.

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI signed names. REST alias + id array. | 의도 (two APIs) if both exist; they are not the same op |
| 사전 검증 | CLI +/- required. REST allowlist + editmeta. | 의도 |
| origin 호출 | CLI `update.components`. REST `UpdateFields` via `fields.FieldValue("component_array")`. | not the same verb; do not unify blindly |
| 미러 갱신 | 동일 class when they write | 동일 |
| 실패 서사 | CLI appends editmeta allowed names (`edit.go:584-597`). REST `403 field_not_editable` / origin 400. | 의도 |
| 성공 출력 | 의도 | 의도 |
| 부작용 | 없음 | 동일 |

### 13. fixVersions

CLI: `edit --fix-version +id-or-name/-…` `edit.go:244-249,473-513` (catalog, GDK-678 mint-by-name)
HTTP: no dedicated route. Only if a configured field spec has `kind=version_array` on `PATCH fields/`.

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력…부작용 | REST has no first-class fix-version write. CLI-only as a signed update op. | **CLI-only capability**, not a drifted copy. Web can only do it if a version_array alias is configured |

### 14. fields (custom / alias)

CLI: `edit --field alias=value` `edit.go:209-218,276-287,666-733` (`EditMeta` on the **key's Writer**, name→id for allowedValues, `resolveAccount` for `kind=user`)
HTTP: `handleFields` `write.go:947-987`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI JSON-or-string, names resolved via editmeta (`matchAllowedID`). HTTP `{field, value}` already id-shaped (`fields.FieldValue`). | 의도 for web. REST agents sending display names: 판정 불가 whether that is a supported REST skill |
| 사전 검증 | Both `fields.EditableAliases`. Missing → CLI lists configured aliases; HTTP `403 field_not_editable`. | 의도 |
| origin 호출 | CLI `EditIssue` fields map. HTTP `UpdateFields`. | 동일 family |
| 미러 갱신 | 동일 | 동일 |
| 실패 서사 | 의도 | 의도 |
| 성공 출력 | 의도 | 의도 |
| 부작용 / routing | HTTP fetches `EditMeta` via `s.client()` (`write.go:956`) — **Jira client, not `keyWriter`**. `mutate` afterwards uses `keyWriter`. A Linear key therefore reads Jira editmeta (or fails Linear-only) and writes through the Linear writer. CLI `withKeyWriteSession` uses one Writer for meta and write. | **드리프트**. User-visible on a Linear row |

### 15. link

CLI: `cmdLink` `cmd/gadak/link.go:17-81`
HTTP: `handleLink` `internal/server/link.go:45-137`
Resolver: `origin.ResolveLinkType` — **already one owner** (GDK-85).

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI `gadak link A B --type token`. HTTP `{type, key}` on `POST {key}/link/` (A from path, B from body). Both upper, trim. | 의도 |
| 사전 검증 | Both self-ref refuse before catalog GET. HTTP also `type_required` / `key_required` / `IssueKeyLiteral` on B. CLI empty A/B is usage. HTTP `AsIssueLinker` miss → 400 with `ErrNoIssueLinks` text. CLI returns that error as prose. | 동일 in substance |
| origin 호출 | 동일: catalog → `ResolveLinkType` → maybe swap → `LinkIssues`. | 동일 |
| 미러 갱신 | Both refresh **B then A** (comments in both files). HTTP maps A or B refresh failure to `write_applied_mirror_stale`. CLI wraps B's failure with the "write applied… B" sentence, then `emitAfterWrite` for A. After a landed write, HTTP `KeySource(B)` error is 409 `key_ambiguous`; CLI returns the store error as a write failure. | B-then-A order: 동일. Ambiguous-B after success: **드리프트** of the reported failure (write already landed on both) |
| 실패 서사 | 의도 | 의도 |
| 성공 출력 | Extra `{keys, type:{id,name,outward,inward}}` is the same map on both (`link.go:70-77` / `link.go REST:128-136`). | 동일 extra; envelope 의도 |
| 부작용 | 없음 | 동일 |

### 16. claim

CLI: `cmdClaim` `agent.go:2296-2330` → `claim.Apply`
HTTP: **no route** (`grep handleClaim` in `internal/server`: none). Package comment: "CLI today, REST later".

| Axis | Difference | Verdict |
| --- | --- | --- |
| all | CLI-only. Conflict exit 75 (`exitClaimConflict`). Non-atomic Cloud fallback warns on stderr. | **CLI-only**. Not a drifted copy. `internal/claim` is already the usecase layer |

### 17. project create

CLI: `cmdProjectCreate` `cmd/gadak/project.go:31` — standalone only, `POST /rest/api/3/project` via `origin.Client.Raw`
HTTP: none

| Axis | Difference | Verdict |
| --- | --- | --- |
| all | CLI-only, standalone-only. Not a UI verb. | **CLI-only** (justified: connected projects are Jira admin) |

### 18. page create

CLI: `cmdPageCreate` `cmd/gadak/page.go:38-115`
HTTP: `handlePageCreate` `write.go:1385-1428`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI `--space --title -m\|--adf-file --parent`. HTTP `{space,title,adf?,text?,parent?}`. | 의도 |
| 사전 검증 | Both require space+title. HTTP `validADF` requires JSON whose top-level `type` is `"doc"` (`write.go:162-176`). CLI `--adf-file` is `json.Valid` only (`page.go:75-78`). | ADF shape: **드리프트** |
| origin 호출 | 동일 `origin.Wiki` → `CreatePage`. CLI unknown-space 400 rewritten to catalog sentence (`formatPageSpaceError` `page.go:120-145`, GDK-467, `TestPageCreateUnknownSpaceListsAvailable`). HTTP `failJira` of the origin body. | catalog sentence: **드리프트** (same class as GDK-635 before the hint moved) |
| 미러 갱신 | Both `sync.SyncPage`. Failure: CLI "page %s created, but the mirror did not refresh"; HTTP `write_applied_mirror_stale`. | 동일 class |
| 실패 서사 | 의도, except the space catalog | 의도 + 드리프트 |
| 성공 출력 | CLI `ID\ttitle` or `--json {page}`. HTTP `201 {page: PageDetail}`. | 의도 (201 vs 200 is REST-only) |
| 부작용 | 없음 | 동일 |

### 19. page edit

CLI: `cmdPageEdit` `page.go:220-318`
HTTP: `handlePageEdit` `write.go:1313-1380`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI `--title -m\|--adf-file --version`. HTTP `{title,adf,text,version,force}`. CLI `-m` empty-and-no-adf-no-title is "nothing to change". HTTP `text: ""` is a replace. | 의도, except `force` (REST only) |
| 사전 검증 | HTTP `isSimpleADF` / `format_loss` 409 unless `force` (`write.go:1355-1357`, `TestPageEditTextFormatLoss`). CLI help text says rich pages lose formatting; **no gate, no `--force`**. HTTP `validADF`; CLI `json.Valid`. | format_loss: **드리프트** (REST refuses, CLI overwrites). ADF type=doc: **드리프트** |
| origin 호출 | 동일 `Page` then `UpdatePage` with HEAD+1 or caller version+1. | 동일 |
| 미러 갱신 | Both `SyncPage`. | 동일 class |
| 실패 서사 | 의도, plus `format_loss` code the CLI cannot emit | 드리프트 of the gate |
| 성공 출력 | CLI text `ID\ttitle`; `--json {page}`. HTTP `{page}`. CLI if the page is outside mirrored spaces: a sentence, exit 0 (`page.go:315`). HTTP `write_applied_mirror_stale` if `PageDetail` misses. | outside-mirror: **드리프트** (same family as create-not-mirrored) |
| 부작용 | CLI silent format loss on rich pages | **드리프트** (data loss vs 409) |

### 20. page comment

CLI: `cmdPageComment` `page.go:147-218`
HTTP: `handlePageComment` `write.go:1432-1472`

| Axis | Difference | Verdict |
| --- | --- | --- |
| 입력 파싱·정규화 | CLI `-m\|--adf-file`. HTTP `{adf,text}`. | 의도 |
| 사전 검증 | HTTP empty → `empty_comment`; ADF → `validADF`. CLI empty → prose; ADF → `json.Valid`. | ADF: **드리프트** |
| origin 호출 | 동일 `AddPageComment` | 동일 |
| 미러 갱신 | Both `SyncPage`. | 동일 |
| 실패 서사 | 의도 | 의도 |
| 성공 출력 | CLI `--json` encodes the **origin comment object**; text prints `ID\tcomment N added`. HTTP returns `{page: PageDetail}` (no comment id). | **드리프트** for JSON consumers; text vs page envelope is otherwise 의도 |
| 부작용 | 없음 | 동일 |

### §1 totals

- **Rows (verbs): 20** — create, transition, comment, attach, assign, summary, description, labels, priority, duedate, parent, components, fixVersions, fields, link, claim, project create, page create, page edit, page comment. (`gadak close` is not a 21st origin verb; it is transition with target `done`.)
- **Difference-axis cells** (verb × axis that is not "동일"): **60**.

| Verb | non-동일 axes | Notes |
| --- | ---: | --- |
| create | 7 | routing, custom fields, attach, `project_not_mirrored` |
| transition | 5 | batch/dry-run, noop refresh |
| comment | 4 | @mentions vs ids, `created_at` |
| attach | 5 | multi-file vs one, no `issue` in REST body |
| assign | 2 | name vs account id |
| summary | 3 | 255 gate |
| description | 2 | stdin vs JSON |
| labels | 2 | +/- vs replace |
| priority | 2 | name vs id; REST can clear |
| duedate | 1 | `none` vs `null` (의도) |
| parent | 2 | self-check |
| components | 4 | dedicated CLI vs PATCH alias |
| fixVersions | 1 | CLI-only signed op |
| fields | 3 | Linear `EditMeta` routing |
| link | 2 | post-success `KeySource(B)` |
| claim | 1 | CLI-only |
| project create | 1 | CLI-only, standalone |
| page create | 4 | ADF, space catalog |
| page edit | 5 | `format_loss` |
| page comment | 4 | ADF, JSON envelope |
| **sum** | **60** | |

Of the 60, most are intended surface differences (prose vs codes, picker vs token, one-file vs many). The ones that change an outcome are listed in §2 (8). Claim, fixVersions, and project create are capability gaps, not drifted copies.

---

## §2. User-visible drift (registerable without extracting a layer)

Only rows where **the same human/agent action succeeds on one surface and fails on the other, writes a different origin, or destroys data**. Error-code vs prose is excluded (의도). Self-parent and summary-255 fail on both; they are message-quality, not this list.

| # | Drift | CLI | HTTP | Repro |
| --- | --- | --- | --- | --- |
| 1 | Page text-edit of a rich ADF body | succeeds; body replaced with one paragraph | `409 format_loss` unless `force` | `gadak page edit <ID> -m "hello"` vs `curl -X PUT …/pages/<ID>/edit/ -d '{"text":"hello"}'` |
| 2 | Create on a Linear project / Linear-only workspace | `WriterFor("linear")`; `TestCreateLinearOnlyRoutesToLinear` | `origin.Client` (Jira); Linear teams never enter `handleCreate` | `gadak --profile <linear> create --project TEAM "s"` vs `POST /api/v1/issues/create/` `{"project_key":"TEAM","summary":"s"}` |
| 3 | Create with a required custom field (`--field`) | sent on `CreateIssue` | `handleCreate` has no field map; origin 400 or the web dialog only *warns* (`web/src/lib/create-fields.ts` extra-required is a warning, not a payload) | `gadak create "s" --field alias=value` vs `POST create/` without that field |
| 4 | Create in a project not in `cfg.Projects` | write + stderr warning + exit 0 (`TestCreateOutsideMirrorPrintsKeyAndExitsZero`) | `400 project_not_mirrored` | `gadak create --project OTHER "s"` vs `POST create/` `{"project_key":"OTHER","summary":"s"}` |
| 5 | Page create with an unknown space | catalog sentence (`TestPageCreateUnknownSpaceListsAvailable`) | origin 400 body via `failJira` | `gadak page create --space NOPE --title T -m x` vs `POST /api/v1/issues/pages/` `{"space":"NOPE","title":"T","text":"x"}` |
| 6 | Clear priority | no CLI flag (`--priority none` is not a sentinel) | `PUT {key}/priority/` `{"priority_id": null}` | (CLI cannot). `curl -X PUT …/NMA-1/priority/ -d '{"priority_id":null}'` |
| 7 | `PATCH {key}/fields/` on a Linear row | `gadak edit KEY --field …` uses the Linear writer's `EditMeta` | `handleFields` `EditMeta` via `s.client()` (Jira) | `gadak edit LIN-1 --field alias=v` vs `PATCH …/LIN-1/fields/` |
| 8 | Page ADF that is JSON but not `{"type":"doc",…}` | `--adf-file` accepted if `json.Valid` | `400 invalid_adf` | `gadak page edit ID --adf-file not-a-doc.json` vs `PUT …/edit/` `{"adf":"{\"type\":\"paragraph\"}"}` |

Not in this table, still agent-visible JSON:

- `gadak comment --json` omits `created_at` that REST includes.
- `gadak page comment --json` returns the origin comment; REST returns `{page}`.
- Transition noop: REST refreshes (version++); CLI text does not.

Claim and fixVersions and project-create are **gaps** (one surface), not drifts of two implementations. They belong on a backlog as new REST (or not), not as "the copy drifted".

---

## §3. Usecase-layer signatures

### 3.1 Where it lives

Do **not** dump every verb into a new `internal/write/` god package.

The repo already picked the shape that closed GDK-341/577/591/635/642/85:
**one verb package (or origin face) outside `package main`, both call
sites import it, refresh stays outside.** `internal/create` is
resolution only, not the POST. `internal/transition` is the model to
copy. `internal/claim` even says "CLI today, REST later" — the REST
adapter is a later thin mapper, not a second Apply.

`internal/write` is a reasonable home **only** for the session (the
thing `withKeyWriteSession` and `keyWriter` both restate). Verbs stay
next to their existing owners.

GDK-665: Writer DTOs are **aliases** of Jira payload structs
(`internal/origin/dto.go`). A usecase that invents a parallel
`write.Transition` struct would force every JSON contract and every
stub to rename, which that commit explicitly refused. Speak
`origin.Writer` (and the optional faces). Field payloads stay
`map[string]any` because that is what `CreateIssue` / `UpdateFields` /
`EditIssue` take.

### 3.2 Errors

Keep the split that `transition` and `create` already enforce:

- Usecase returns **classified errors** (`*transition.Refused`,
  `*create.NeedProjectError`, `*claim.TakenError`, plus a small
  `write.MirrorStaleError` if the session owns the tail).
- Origin errors pass through unchanged (`error` from the Writer).
- CLI composes flag names (`formatTransitionError`, `formatCreateError`).
- REST maps classified errors to wire codes (`failCreate`,
  `IsRefused` → 400 `APIError`) and origin errors to `failJira`.

Do not put English/Korean sentences in the usecase, and do not put
`--flag` names or `snake_case` codes there either. That is the whole
point of `Need*` carrying catalogue data only (`internal/create/resolve.go:5-6`,
`internal/transition/apply.go:7-8`).

### 3.3 RefreshIssue: outside

`internal/transition/apply.go:10-11` already states it: mirror refresh
is the write-through tail, not a property of the verb. If Apply takes
`*store.DB`, every verb package grows a store dependency, and
`internal/store` must still not import Jira-shaped code
(`docs/ARCHITECTURE.md`, constitution). `parenthint` is the exception
that reads the mirror, and it takes a `Querier` interface, not
`*store.DB`.

Create's HTTP `SyncIssue(…, Options{Client: c})` is a client-reuse
optimization, not a reason to put refresh inside create. A session
helper can offer `After(ctx, key, src)` and `AfterWithClient` without
the verb knowing.

### 3.4 Actual signatures (derived from §1, not invented)

```go
// internal/write — session only (proposed).
package write

type Session struct {
    Cfg *config.Config
    DB  *store.DB
}

// WriterForKey is keyWriter / withKeyWriteSession without HTTP/CLI.
// ErrKeyAmbiguous and credential misses are classified so REST can
// emit key_ambiguous / credential_required and CLI can print prose.
func (s Session) WriterForKey(ctx context.Context, key string) (w origin.Writer, src string, err error)

func (s Session) WriterForCreate(ctx context.Context, project string) (w origin.Writer, src string, err error)

// After is RefreshIssue. Errors are unwrapped; callers wrap.
func (s Session) After(ctx context.Context, key, src string) error
```

```go
// internal/issuelink (proposed). Owner of the sequence both files
// still copy: self-ref → AsIssueLinker → catalog → ResolveLinkType →
// LinkIssues. Refresh B then A stays in the session / surfaces
// (two keys).
package issuelink

type Request struct {
    A, B, Token string // already normalized or not? surfaces normalize.
}

type Result struct {
    Outward, Inward string
    Type            origin.IssueLinkType // today jira.IssueLinkType via alias
    Reverse         bool
}

func Apply(ctx context.Context, w origin.Writer, req Request) (Result, error)
```

`ResolveLinkType` stays on `origin` (vocabulary of the face). `Apply`
is the orchestration `package main` still owns.

```go
// internal/create — extend, do not replace. Project/Type/Priority
// already live here. Missing is payload assembly + CreateIssue.
type IssueRequest struct {
    Project, Type, Summary, Description, Priority, Parent, Due string
    Labels   []string
    Fields   map[string]json.RawMessage // alias → raw; empty on REST today
    Assignee string                     // account id; empty on CLI today
}

type IssueResult struct {
    Key      string
    Project  Resolved
    Type     Resolved
}

func Issue(ctx context.Context, w origin.Writer, cfg *config.Config, req IssueRequest) (IssueResult, error)
```

Attach after create stays a second call (`Writer.Upload`) so a failed
attach does not look like a failed create (CLI already returns the key
plus a partial error). Linear refusals (`labels`/`parent`/`fields` not
supported) stay in `createLinearOne` until `Issue` branches on the
writer's faces, not on a `src == "linear"` string in `package main`.

```go
// comment — not worth a package until mentions have one owner.
// The origin call is one line (AddComment). Mention resolution is
// CLI-only; media refs are REST-only. A shared helper is:
func Mentions(ctx context.Context, w origin.Writer, body string) (map[string]string, []string, error)
// that is today's resolveCommentMentions, moved out of package main
// so REST *could* reuse it; REST currently should not (UI sends ids).
```

```go
// edit — do not unify REST per-field UpdateFields with CLI one-PUT
// EditIssue. That changes origin call count and partial-failure
// behaviour (TestEditCombinedFlagsOnePUT). Share validators only:

func ParseDue(raw, clearSentinel string) (value string, clear bool, err error) // DateOnlyLiteral
func ParseParent(raw, clearSentinel string) (key string, clear bool, err error) // IssueKeyLiteral + self-check
func ParsePriority(ctx, w, raw string) (id string, clear bool, err error)      // create.Priority; allow clear
```

Self-parent belongs in `ParseParent`, not in one handler.

Wiki ADF helpers belong next to the existing port, not in `internal/write`:

```go
// already in write.go as validADF / isSimpleADF (ported from web/src/lib/adf.ts).
// Move to internal/adf (plain JSON, no Jira types — ARCHITECTURE.md) so
// cmd/gadak/page.go can call them. That is a drift fix (§2.1, §2.8), not a
// usecase layer.
func ValidDoc(raw []byte) bool
func IsSimple(raw string) bool
```

`claim.Apply` and `transition.Apply` stay as they are. REST claim is a
new handler that maps `TakenError` → 409, not a new package.

### 3.5 Three decisions

1. **Types: `origin.Writer` + existing aliases, not new DTOs.** GDK-665
   already paid the cost of renaming the interface; a second vocabulary
   would re-open the HTTP JSON contract the aliases exist to protect.
2. **Classified errors in the usecase; sentences and wire codes in the
   surface.** Proven by `transition.Refused` / `create.Need*` /
   `claim.TakenError`. CLI flag names and REST snake_case must fork
   *after* the same error value.
3. **`RefreshIssue` stays outside the verb.** Proven by
   `transition.Apply` and by `TestRefreshIssueOwnerIsUnique`. Putting
   `*store.DB` on every Apply pulls the store into the write layer and
   fights the store↔jira import rule. Create's `SyncIssue(Client:)` is
   a session concern.

---

## §4. Migration order and blast radius

First verb has to be the smallest one that still has **two copies of
the same origin sequence**, so the layer signature is proven without
changing HTTP JSON.

### Step 0 — drift fixes that do not need a layer (cheapest)

| Item | Why first | Tests that catch a regression | Empty gate today |
| --- | --- | --- | --- |
| Port `validADF` / `isSimpleADF` to `internal/adf`; CLI page edit gains `format_loss` (or `--force` matching REST) | §2.1 is data loss | `TestPageEditTextFormatLoss`; **no CLI test** | CLI format_loss |
| `handleFields` `EditMeta` through `keyWriter`, not `s.client()` | §2.7 Linear | `TestFieldEditAllowlistAndShapes` (Jira); **no Linear field-edit REST test** | Linear PATCH |
| `handleCreate` through `WriterFor` / `resolveCreateSource` | §2.2 | `TestCreateLinearOnlyRoutesToLinear` (CLI only); `TestCreateIssue*` (Jira REST) | REST Linear create |
| CLI `--priority none` or document that REST-only is the clear | §2.6 | `TestPrioritySetAndClear` (REST); CLI has no clear test | CLI clear |
| Self-parent check in CLI `parseParentKey` | §2. not outcome, but cheap | `TestEditParentRejectsSelfBeforeJira` | CLI self-parent |
| Space-catalog rewrite on REST page create (call the same helper as CLI) | §2.5, GDK-467 leftover | `TestPageCreateUnknownSpaceListsAvailable` | REST space catalog |

HTTP JSON for these is unchanged except new 409 `format_loss` on a path
that currently 200s in CLI only (REST already 409s).

### Step 1 — first extraction: `issuelink.Apply`

Why this one: GDK-85 already moved the resolver; the remaining
sequence (self-ref, face, catalog, reverse, `LinkIssues`) is ~40 lines
copied. Extra JSON `{keys, type}` already matches. No EditIssue-vs-
UpdateFields knot.

Blast radius:

- HTTP contract: `POST {key}/link/` body `{type,key}` and 200
  `{issue, keys, type}` — `specs/000-product/contracts/api.md` does not
  yet list the route (stale). Do not change the body.
- Tests: `TestLinkRESTBlocksOutward`, `TestLinkRESTInwardReversesDirection`,
  `TestLinkRESTUnknownTypeDoesNotPOST`, `TestLinkRESTSelfRefusedNoOrigin`,
  `TestLinkRESTOriginFailureLeavesMirrorUnchanged`, `TestLinkRESTRequiresCredential`,
  `TestResolveLinkTypeMatchesCLI`, `TestLinkBlocksOutward`,
  `TestLinkIsBlockedByReversesDirection`, `TestLinkUnknownTokenListsCatalogAndDoesNotPOST`,
  `TestLinkSelfRefusedWithoutCatalogGET`, `TestLinkJSONIncludesBothKeysAndType`,
  `TestLinkSymmetricTypeIsNotAmbiguous`.
- Empty gate: post-success `KeySource(B)` ambiguous (both surfaces lie
  about whether the write landed).

Signature proven: `Apply(ctx, Writer, Request) (Result, error)` with
classified self/unknown/ambiguous errors, refresh outside.

### Step 2 — session (`internal/write.Session`)

Replace `keyWriter` + `withKeyWriteSession` + `withCreateSession` with
one constructor. Surfaces keep wrapping (`failJira` vs prose,
`respondIssue` vs `summaryLine`).

Blast radius: every write verb's credential and Linear routing tests
(`TestWritesRequireACredential`, `TestWritesRefuseToRunWithoutACredential`,
`TestLinearKeyPrioritiesWithoutJiraCredential`,
`TestCreateLinearOnlyRoutesToLinear`, `TestKeyPrioritiesRoutesBySource`).
Empty gate: HTTP create still not on `WriterFor` until step 0/3.

Do not put `RefreshIssue` inside Session.After's *callers* by accident
twice (`TestRefreshIssueOwnerIsUnique`).

### Step 3 — `create.Issue` assembly

Move `createOne` / `handleCreate` payload assembly next to
`create.Project`/`Type`. Attach stays a caller loop.

Blast radius: `TestCreateIssue*` (REST, ~15 functions),
`TestCreateHappyPathSendsFieldsAndPrintsReread` and the rest of
`cmd/gadak/create_test.go` (~40 functions),
`TestCreateRESTErrorBodiesHaveNoCLIFlagTokens` (wire codes must stay
flag-free), `project_not_mirrored` (decide: keep REST refusal or adopt
CLI warn — that is a product call, not a silent unify).

Empty gate: REST custom fields on create (§2.3); REST Linear create
unless step 0 landed.

### Step 4 — comment JSON parity, then maybe mentions

Add `created_at` to CLI `--json` (additive). Moving
`resolveCommentMentions` out of `package main` is optional; REST should
keep taking ids.

Tests: `TestCommentSendsADFAndRefusesAnEmptyBody`,
`TestCommentSendsMentionsAsADF`, `TestCommentPassesVisibilityAndInternal`,
`TestCommentMention*`.

### Step 5 — last, maybe never: unify `edit` PUT shape

CLI one `EditIssue` PUT (`TestEditCombinedFlagsOnePUT`) vs REST one
endpoint per field is **not drift of the same op**; it is two APIs.
Share `ParseDue` / `ParseParent` / self-check / 255. Do not turn REST
into a batch editor or CLI into N round-trips without a separate
decision. Linear `EditIssue` rejecting `update` ops is a reason to keep
the split: REST `UpdateFields` is the path Linear implements.

### Step 6 — REST `claim` (new surface)

`claim.Apply` already exists. A handler is an adapter:
`TakenError` → 409, `Atomic=false` → extra field, then `mutate`.
Contract addition to `api.md`. Tests: `TestApplyAtomic`,
`TestClaimStandaloneTwoActors`, `TestClaimConnectedFallback` plus a new
REST httptest. Empty gate: web UI.

Wiki page comment as an early extraction is tempting (small) but it
talks `*confluence.Client` / `origin.Wiki`, not `origin.Writer`. Do not
use it to prove the Writer-layer signature.

---

## §5. Should we not integrate?

No. Dismissal would need the remaining copies to be cheap to keep.
They are not:

| Number | What |
| ---: | --- |
| ~400–600 | Remaining duplicated *orchestration* lines (session + create assembly + link sequence + wiki ADF gates + comment extras). Not the 3k lines of CLI flag/batch chrome |
| 80+ | `Test*` in `internal/server/write_test.go` locking today's REST JSON |
| 55 | `Test*` in `cmd/gadak/edit_test.go` locking the one-PUT edit |
| 40+ | `cmd/gadak/create_test.go` |
| 20 | `internal/transition/apply_test.go` |
| 11 | `internal/create/resolve_test.go` |
| 8 | `internal/server/link_test.go` |
| 5 | Historical copy-drift bugs already filed and paid (GDK-341, 328, 635, 85, 642) |
| 8 | Current user-visible drifts in §2 |

A god-package `internal/write` that also swallows CLI batch, Linear
create, and REST per-field editors would touch the HTTP JSON contract
and the one-PUT edit contract at once. That blast radius is the
argument against a **single** extraction, not against **any**
extraction.

What we should refuse:

- Unifying CLI `EditIssue` (one PUT, `update` ops) with REST
  `UpdateFields` (one field, replace) in the same round as the session.
- Inventing origin DTOs distinct from `origin.dto.go` aliases.
- Putting `RefreshIssue` inside each verb.

What we should do: keep paying the same way GDK-85 / 642 / 577 paid —
one owner outside `package main`, structural test against a third copy,
surfaces stay thin mappers. First owner: `issuelink.Apply`. Parallel
cheap: §2 items 1, 2, 7 (format_loss, Linear create, `handleFields`
routing) as bug fixes with no layer at all.
