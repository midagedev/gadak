# HTTP API Contract

This is the contract the web UI in `web/` **already speaks**. It was extracted
from an internal Django backend, so the shapes below are not a proposal — they
are what the client sends and parses today. The Go server implements them so the
UI needs no rewrite.

Base path: `config().apiBase`, default `/api/v1/issues/`. Auth base:
`config().authBase`, default `/api/v1/auth/`. Everything is served from the same
origin as the UI, which is why no CORS handling exists anywhere in this project.

Status column: **R** = required for v0.1, **D** = deferred (UI guards for its
absence), **X** = cut in the extraction and not coming back.

## Response conventions

- Success bodies are the documented object, not wrapped.
- Write failures return `{"error": "<code>", "jira_errors": {...}}` with a 4xx.
  The client surfaces `error` directly and renders `jira_errors` per field.
- `409 credential_required` means no token is stored — open the credential dialog.
- `409 credential_rejected` means a stored token was refused by Jira (expired or
  wrong) — open the same dialog with “replace your token” copy.
- Reads are permitted without authentication when the server is bound to
  loopback; writes require configured credentials.

## Core read

### `GET bootstrap/` — R

Full mirror hydration. Served entirely from SQLite.

Request header: `If-None-Match: "sv-<version>"` (optional).

`304 Not Modified` when `sync_state.version` is unchanged. Otherwise:

```json
{
  "server_time": "2026-08-04T09:15:00Z",
  "sync_version": 412,
  "members": [ { "email": "...", "name": "...", "display_name": null, "profile_image": null, "avatar_url": null, "department": null, "job_role": null, "group": null, "status": null, "jira_account_id": null, "account_type": "agent", "is_bot": true } ],
  "members_version": "sha256:...",
  "issues": [ /* IssueLite, see below */ ],
  "sync_health": {
    "overall": "healthy",
    "checked_at": "2026-08-04T09:15:00Z",
    "sources": [ { "key": "jira", "label": "Jira", "status": "healthy", "synced_at": "...", "message": "ok" } ]
  }
}
```

- `ETag: "sv-<sync_version>"` must be set on 200. The 304 path also accepts the
  client's `"in-<sync_version>"` tag, which its cache-hydration path invents when
  it has a stored version but no stored ETag.
- `server_time` is the cursor the client passes to `delta/`. It must come from
  the same clock that writes `items.synced_at`. That cursor bound is inclusive,
  so a poll in the same millisecond as a write legitimately re-sends the row.
- `members` is keyed by email: the mirror supplies the name and account id of
  everyone who appears as an assignee or reporter with a visible email, and
  `config.members` supplies `group`, `department`, `job_role` and the avatar,
  winning on every conflict. A person whose email Jira hides can still appear
  when configured explicitly. `avatar_url`
  and `profile_image` carry the same value — the client reads the latter.
  Actors of comments, changelog entries and dev-panel links are members too
  (GDK-590): a bot that only commented appears, keyed by account id with an
  empty email, exactly like an email-hidden human. `account_type` is the
  origin's account axis (`agent` for gadak-origin worker accounts, `app` for
  Cloud Connect, `atlassian`/`customer` for humans) and `is_bot` is derived
  from it — the one judgement, never a display-name guess. Both are omitted
  for accounts the catalog has no type for.
- `sync_health.status` is one of `healthy` / `stale` / `failed` / `missing`, and
  `message` is `"ok"` when nothing is wrong (the client suppresses that line).
  It is server text in one language; the client localizes only the status label.
  Staleness is measured from the last run that finished without an error, never
  from the watermark: a quiet project leaves its watermark in the past forever.

### `GET delta/?since=<iso>&mv=<members_version>` — R

```json
{
  "server_time": "2026-08-04T09:20:00Z",
  "upserted": [ /* IssueLite */ ],
  "deleted_keys": ["NMB-9"],
  "members": null,
  "members_version": "sha256:...",
  "sync_health": { }
}
```

- `members` is omitted (`null`) when `mv` matches the current hash.
- `deleted_keys` **must** be correct. The client removes those rows from
  IndexedDB; a missed deletion leaves a tombstone visible forever.
- Polled every 15 s by the client and on tab focus.

### `GET <key>/detail/` — R

On-demand detail. Everything comes from the mirror; no Jira call.

```json
{
  "issue_key": "NMB-142",
  "description_adf": { "type": "doc", "version": 1, "content": [] },
  "attachments": [ { "id": "10021", "filename": "...", "mime_type": "...", "size": 0, "media_id": "", "media_collection": "", "is_image": true, "is_video": false, "cache_status": "ready", "created_at": "...", "content_url": "/api/v1/issues/NMB-142/attachments/10021/content/" } ],
  "comments": [ { "comment_id": "...", "author": "...", "author_email": null, "author_account_id": "...", "author_account_type": "agent", "body": "flattened text", "raw_body": {}, "created_at": "..." } ],
  "history": [ { "at": "...", "by": "...", "author_id": "...", "field": "status", "from": "...", "to": "...", "from_category": "done", "to_category": "inprogress" } ],
  "linked_issues": [ { "key": "NMA-8", "type": "Blocks", "direction": "inward", "summary": "...", "status_category": "done" } ],
  "development_opinion": null,
  "qa_context": null,
  "deploy": null,
  "linked_prs": [ { "number": 9, "title": "fix(NMB-139): retry budget", "url": "https://github.com/acme/api/pull/9", "state": "open", "repo": "acme/api", "author": null, "linked_by": "Claude (build 1)", "linked_by_id": "acc-bot" } ],
  "wait_ms": 407822,
  "progress_ms": 5127000
}
```

The field names above are the client's (`DetailResponse` in
`web/src/lib/types.ts`), which is what actually gets parsed: comments carry
`comment_id` / `raw_body` / `body`, and history carries `by` / `from` / `to`.
`raw_body` is the ADF and `body` is the flattened fallback the renderer uses when
the ADF will not render — the two are never both dropped.

`media_id` is the ADF media node's own id, which the mirror does not carry, so it
is empty and the renderer falls back to matching a media node to an attachment by
filename. `author_email` is resolved by matching the comment author's account id
against `config.members`, and is `null` for anyone not in that directory.
`author_account_type` comes from the cached account catalog (GDK-590) and is
omitted when the catalog has no entry for the author. History's `author_id`
(same addition) is the account id of `by`, so a bot's status move is
attributable without matching localized display names.

`development_opinion`, `qa_context`, `deploy`, and `linked_prs` are merged from
the `enrichments` table, which external plugins write with SQL — the kinds are
`opinion`, `qa`, `deploy` and `prs`, and the payload shapes are in
`docs/PLUGINS.md`. With no plugin writing them they stay `null` / `[]`, which is
what the client's optional-field guards expect. A payload that is not valid JSON
is dropped rather than spliced into the response.

`from_category` and `to_category` on a `status` history entry are optional
(`new` / `inprogress` / `done`). When present the UI marks reopens from them;
otherwise it falls back to matching status names, which is language-dependent and
therefore only a fallback.

`linked_by` / `linked_by_id` on a `linked_prs` entry name who attached the link
(the dev-panel actor, GDK-589) — a different axis from the PR's own `author`:
a bot linking a human's PR keeps both. Only dev-panel links carry them; a
PR-shaped URL attachment has no actor and omits both.

`wait_ms` / `progress_ms` are the lifecycle spans the CLI's durations line
prints (GDK-591): wait = created → first entry into in-progress, progress =
latest entry into in-progress → done, or now while still in progress. Both are
omitted when the changelog cannot answer — the detail chip renders nothing
rather than a zero. Nothing is stored; both are computed per request.

### `GET search/?q=<text>&limit=<n>` — R

Server-side full-text over issue and page titles, bodies, and comments via
FTS5. ⌘K uses this. `keys` / `total` are issue hits; `pages` are document
hits; `matches` maps each returned key to the winning field.

```json
{
  "keys": ["NMB-142", "NMA-8"],
  "pages": [],
  "total": 2,
  "matches": {
    "NMB-142": { "field": "comment", "snippet": "Reproduced on staging with a fresh workspace." },
    "NMA-8": { "field": "body", "snippet": "The idempotency key is dropped when the gateway times out." }
  }
}
```

`matches` maps each returned issue or page key to the winning FTS column:
`field` is `"title"` | `"body"` | `"comment"` (title wins over body over comment
when more than one column hits). `snippet` is plain text only — no HTML and no
highlight markers; clients highlight against their own query. Always present
(empty object when there are no hits).

### `GET pages/{id}/` — R

Page detail: PageLite plus `body_adf`, `comments[]`, and `body_text` (the ADF body flattened by the same walker FTS indexes). Optional `ref_issue_keys` / `backlink_issue_keys`.

### `GET|POST jql/` — R

Parse a JQL string or a Jira navigator URL into the same filter object the UI
serializes into the hash. The parser is the documented subset in
`internal/jql`: AND of `=`, `IN`, `IS EMPTY`, date comparisons, `text ~`,
`currentUser()`, and `ORDER BY`. Clauses outside the subset are listed in
`unsupported` and are never applied.

GET takes `?q=` and optional `?email=` (for `currentUser()`). POST takes
`{"input","email"}`. Empty input is `400 jql_required`. A saved-filter URL
(`?filter=123` with no `jql=`) answers `200` with `error: "filter_id"` — the
id is not in the mirror.

```json
{
  "input": "project = NMA AND statusCategory = \"In Progress\"",
  "jql": "project = NMA AND statusCategory = \"In Progress\"",
  "filters": { "jira_project": ["NMA"], "status_category": ["inprogress"] },
  "display": { "sort": "updated", "dir": "desc" },
  "applied": ["project", "statusCategory"],
  "unsupported": [],
  "error": "",
  "message": ""
}
```

### `GET ui-focus/` — R

The view hash left by `gadak views open`, plus its write timestamp:
`200 {"hash":"pj=NMA&sc=inprogress","at":"2026-08-26T05:00:00.000000000Z"}`.
**The file is not consumed on read** — every UI polling that profile sees the
same payload while it is fresh (2 min), and the file expires by age. Applying
a given payload once is each client's job; a read that consumed it meant
whichever client polled first stole the focus from the window a human was
looking at (GDK-960). Dedupe on `(at, hash)`, and treat `at` as an opaque
string: it is RFC 3339 and now carries fractional seconds, but a client that
keys on it alone drops the second of two writes that share a second (GDK-981).
Always `200` with a JSON body — `204` is retired, because an empty
body cannot carry `configVersion` (GDK-791). `hash` and `at` are absent when
nothing is fresh. The SPA applies the hash as `#/?<hash>`.

The same poll carries `mirrorVersion`: the disk identity of this profile's
mirror (GDK-1170). Opaque — only ever compared to the previous one. When it
moves, the mirror was written by somebody else (`gadak claim` in a terminal,
another tab, the watch loop) and the client pulls `delta/` on this tick
instead of waiting out its own 15s poll. Empty means "no signal" and never
"nothing changed": an older server omits it, a server with no mirror sends
`""`, and a failed poll has none — in each case the client's own backstop
poll stays in charge. The first value a client sees is a baseline, not a
change.

### `POST jql/emit/` — R

The inverse: a `ViewFilters` (+ optional display and email) becomes JQL.
gadak-only flags (`reopened`, `stale`, …) are listed in `omitted`.

```json
{ "jql": "project = NMA AND statusCategory = \"In Progress\"", "omitted": ["reopened"] }
```

### `GET <key>/attachments/<id>/content/` — R

Streams attachment bytes: metadata lives in SQLite; bytes are cached on disk
(`internal/attachcache`) and fetched from Jira on a miss. A cached image
still renders with no credential.

The client validates that a media URL matches exactly
`<apiBase><key>/attachments/<id>/content/` before using it as an image source,
so this path shape is load-bearing for XSS safety and must not change.

## IssueLite

The row shape used by `bootstrap`, `delta`, and every write response. Stored
verbatim in IndexedDB, so **field names are a contract** — adding is safe,
renaming or removing is not.

Browser cache schema v2 clears v1 `issues` rows and the `sync` cursor once so
the additive `reporter_id` projection reaches unchanged issues through a full
bootstrap. The independent `write` metadata record is preserved, and v2 caches
are reused without another invalidation.

```
issue_key, summary, project_key, issue_type, issue_type_id, status, status_id,
status_category, priority, priority_rank, assignee, assignee_id, assignee_email,
reporter, reporter_id, reporter_email, epic_key, parent_key, hierarchy_level,
labels[], components[], fix_versions[],
duedate, resolution, created_at, updated_at, status_changed_at, resolved_at,
reopen_count, reopened_at, comment_count
```

Two groups of fields are added on top of the stored row, both from
configuration:

- `team_group`, when `groupQuery`, `groupRules`, or a member `group` is configured.
  Order: `groupQuery` hit (empty string = unclassified, stop) → first matching
  `groupRules` row → assignee's member group. A `groupQuery` NULL or missing key
  falls through. With no taxonomy configured the key is **omitted**, not null,
  so the client's group surfaces stay empty rather than showing one bogus bucket.
- every `fieldMap` alias present in `issues.custom`, spread as a top-level key
  (`severity`, `solution`, `environment`, …). Aliases are serialized before the
  stored fields, so a stored field of the same name wins in the client's
  `JSON.parse`.

A third group comes from plugin enrichments: `deploy_status` from kind `deploy`,
and `qa_impact_state` / `qa_impact_label` / `qa_runs` / `qa_suites` from kind
`qa` (`docs/PLUGINS.md`). Enrichment keys are serialized before the stored ones
too, so a plugin can add to a row but never shadow what the tracker said.

A fourth group is the actor axis (GDK-590): `actor_ids`, the account ids that
touched the issue — comment, changelog entry, or dev-panel link, from the
`issue_actors` view. Omitted when nobody did, which every pre-v36 mirror row is.
The client's actor filter and actor grouping narrow on these ids and never on
display names.

Fields that stay absent until something populates them:
`development_test_result`, `source_project`. The UI reads them defensively; the
config flags that surface them default to off. `working_hours_in_status` is gone
from the client type entirely — no server ever populated it (see
`data-model.md`), and staleness now comes from `status_changed_at`.

## Write-through

Every endpoint below calls Jira with the user's credentials, re-reads the issue,
updates the mirror, and returns the refreshed `IssueLite` as `{"issue": {...}}`.
No local queue, ever.

| Endpoint | Method | Jira call | Status |
| --- | --- | --- | --- |
| `credential/` | GET / PUT / DELETE | `GET /rest/api/3/myself` to verify | R |
| `<key>/transitions/` | GET | `GET /issue/{key}/transitions` | R |
| `<key>/transition/` | POST | `POST /issue/{key}/transitions` | R |
| `<key>/comment/` | POST | `POST /issue/{key}/comment` (ADF) | R |
| `<key>/attachments/` | POST | `POST /issue/{key}/attachments` | R |
| `<key>/assignee/` | PUT | `PUT /issue/{key}/assignee` | R |
| `<key>/labels/` | PUT | `PUT /issue/{key}` (`fields.labels`) | R |
| `<key>/priority/` | PUT | `PUT /issue/{key}` (`fields.priority`) | R |
| `<key>/summary/` | PUT | `PUT /issue/{key}` (`fields.summary`) | R |
| `<key>/description/` | PUT | `PUT /issue/{key}` (`fields.description`, ADF-wrapped) | R |
| `<key>/duedate/` | PUT | `PUT /issue/{key}` (`fields.duedate`) | R |
| `<key>/parent/` | PUT | `PUT /issue/{key}` (`fields.parent`) | R |
| `priorities/` | GET | `GET /priority` | R |
| `<key>/fields/` | PATCH | `PUT /issue/{key}` | R |
| `<key>/editmeta/` | GET | `GET /issue/{key}/editmeta` | R |
| `create/` | POST | `POST /issue` | R |
| `create-meta/` | GET | `GET /issue/createmeta` | R |
| `users/?q=` | GET | `GET /user/search` | R |
| `meta/write/` | GET | create meta; the transition map is empty (see below) | R |
| `pages/` | POST | wiki page create via `origin.Wiki` (Confluence v1 `POST content`); body `{space, title, adf?\|text?, parent?}` | R |
| `pages/{id}/edit/` | PUT | wiki page update via `origin.Wiki` (`PUT content/{id}`, version = origin HEAD + 1); body `{title?, adf?\|text?}` — `text` replaces the whole body as plain-paragraph ADF | R |
| `pages/{id}/comment/` | POST | wiki page comment via `origin.Wiki` (`POST content`, type=comment); body `{adf?\|text?}` | R |

Implementation notes that are part of the contract:

- The re-read is `sync.SyncIssue`, the same mapping and derived-field code a
  scheduled sync runs, forced so the rewrite moves `synced_at` and
  `sync_state.version`. That is what makes the client's next `delta` carry the row
  and its ETag stop matching.
- A write that Jira accepted but whose re-read failed answers
  `502 write_applied_mirror_stale`. It is not a failure the user should retry: the
  change is in Jira, only the mirror is behind.
- A retry on a state-changing request would risk a duplicate, so only `429` and
  `503` — where Jira states it did not act — are retried, and a dropped connection
  never is.
- `create-meta/` (and `meta/write/` `create_meta`) lists creatable projects and
  issue types. Each type has `id` and `name`. `subtask` is present when the
  type requires a parent; `hierarchyLevel` is the source tree rank (epic 1,
  standard 0, sub-task −1). Both omit when false/0 so older clients see the
  previous shape.
- `meta/write/` fills `create_meta` but leaves `transitions` empty: precomputing it
  costs a Jira call per project and status, and the client already fetches an
  issue's transitions when the menu opens. Without a credential it answers `200`
  with empty collections rather than an error, because it runs on every boot.
- `<key>/comment/` accepts `attachment_ids` and ignores them. The files are
  already attached by the upload endpoint; embedding them in the body needs the
  ADF media id, which Jira only exposes through the attachment redirect.
- `<key>/labels/` is a full replace. Send `{"labels":[]}` to clear. The server
  trims and de-duplicates. This is not the custom-field allowlist
  (`PATCH <key>/fields/`).
- `<key>/priority/` takes `{"priority_id":"<id>"}` from `GET priorities/`
  (site order, most urgent first). `null` or `""` clears. Names are not
  accepted: Jira localizes them per account.
- `<key>/summary/` takes `{"summary":"…"}`. Trim; empty is `400 summary_required`.
  Longer than 255 runes is `400 summary_too_long`.
- `<key>/description/` takes `{"description": "plain text" | null}`. The server
  wraps plain text into a one-paragraph ADF doc (the same shape `create/` sends);
  `null` or an empty/whitespace string clears the field. Rich ADF is not accepted
  here — this endpoint is the plain-text slice.
- `<key>/duedate/` takes `{"duedate":"YYYY-MM-DD" | null}`. Anything that is not
  a bare date literal is `400`; `null` or `""` clears. Timestamps are rejected on
  purpose: due date is a calendar day, not an instant.
- `<key>/parent/` takes `{"parent":"ABC-123" | null}`. A value that is not a
  Jira key is `400`; `null` or `""` clears. Setting the issue as its own parent
  is `400`.

### Credential storage

`PUT credential/` verifies the token against `/myself` and writes it to
`~/.gadak/config.json` with mode `0600`. It is never written to SQLite, a log, or
a snapshot. `GET credential/` returns only `{configured, jira_email,
display_name, verified_at, token_hint}` — never the token.

### First-run onboarding

Three endpoints exist so a browser can finish setup with no CLI step: `gadak serve`
plus the wizard is the whole path.

| Endpoint | Method | Jira call | Status |
| --- | --- | --- | --- |
| `onboarding/connect/` | PUT | `GET /rest/api/3/myself` to verify | R |
| `projects/available/` | GET | `GET /rest/api/3/project/search` | R |
| `sync/` | POST | full or incremental sync (background) | R |
| `sync/progress/` | GET | none | R |

`PUT onboarding/connect/` takes `{site, jira_email, api_token}` and is the only
endpoint that writes `site`: on a first run there is no stored site, and
`PUT credential/` refuses to verify without one. `site` is normalised (a missing
scheme becomes `https://`, a trailing path is dropped) and rejected as
`400 site_required` when it is not a host URL. Verification and storage are
otherwise identical to `PUT credential/`, response included — a rejected token is
`401 credential_rejected`, and the token never appears in a response.

`GET projects/available/` proxies the site's project list as
`{projects: [{key, name, projectTypeKey}], truncated}`, permission-filtered by the
credential. It answers `409 credential_required` without one. It stops at 500
projects and sets `truncated: true`, because a bigger site is one where typing keys
into settings beats scrolling a picker.

`POST sync/` starts one background sync and answers `202` with the progress
document. Body is optional: empty (or `{"mode":"full"}`) runs a full sync
(onboarding first run); `{"mode":"incremental"}` runs an incremental pass for
daily “Sync now”. Unknown `mode` is `400 invalid_mode`. It is single-flight: a
second call while one is running is `409 sync_in_progress`. An incomplete setup
is `400 credential_required`; an empty project list is a scope, not an
incomplete setup — it means every project the account can see. `GET sync/progress/`
polls `{running, phase, fetched, changed, deleted, done, error, error_code,
started_at, finished_at}` where `phase` is `idle | syncing | done | error`.
`error_code` is present only when the failure means the stored credential was
rejected (`credential_rejected`, the same code the write path answers with) —
one rule (the sync package's `IsRejectedCredential`) classifies, and clients
key recovery affordances on the code, never on `error` prose. A transport
failure (500, timeout, DNS) leaves it absent, and so does a wiki-only failure
after the Jira pass succeeded (`phase` `done` with `error`): the same token
authenticated against Jira moments earlier, so that is a product permission
gap, not a dead token. The document
carries counters only — no site, no email, nothing derived from the token — and
`fetched`/`changed` advance per committed page, so a client polling every second
can show issues arriving live. The job is process-wide state, matching the one
server `gadak serve` runs.

### Field edits

`PATCH <key>/fields/` accepts `{field, value}` and rejects any field not present
in the configured editable-field allowlist. The internal version hardcoded three
custom field ids here; those are now configuration and the allowlist is empty by
default, which hides the inline editor entirely.

`field` is the configured **alias**, never a field id, and rejection is
`403 field_not_editable` — enforced whether or not the UI offered the field, and
again when Jira's editmeta says the field is not editable on that issue.

`value` is the client's `string | string[] | null`, and how it reaches Jira comes
from the field's editmeta schema rather than from configuration:

| Schema | Kind | Sent as |
| --- | --- | --- |
| `option` | `option` | `{"id": value}` |
| `user` | `user` | `{"accountId": value}` |
| `array` of `version` | `version_array` | `[{"id": …}, …]` |

A `null` clears the field (an empty array for `version_array`). Any other schema
has no editor, so `editmeta/` leaves it out and an edit to it is refused.

## Personal state

| Endpoint | Method | Backing | Status |
| --- | --- | --- | --- |
| `views/` | GET / POST | `saved_views`; GET also carries `source` (mirrored Jira filters) | R |
| `views/<id>/` | DELETE | `saved_views` | R |
| `watches/` | GET | `watches` | R |
| `watches/<key>/` | PUT / DELETE | `watches` | R |
| `favorites/` | GET | `favorites` | R |
| `favorites/<key>/` | PUT / DELETE | `favorites` | R |
| `feed/` | GET | computed from the mirror; receipts in `feed_reads` | R |
| `feed/read/` | POST | `feed_reads` | R |

Stored locally (feed events are computed at query time). `403`/`401` is never
correct for these on a loopback bind.

### `GET feed/?focus=&limit=` — R

Personal watch feed (`internal/server/feed.go`). `focus` is
`all` | `assignee` | `reporter` | `mention`. Default `limit` is 80, hard max
200, window 30 days. `features.feed` defaults on.

```json
{ "items": [], "unread_counts": { "all": 0, "assignee": 0, "reporter": 0, "mention": 0 } }
```

### `POST feed/read/` — R

Body: `{event_ids?: string[], issue_keys?: string[], all?: boolean}`. At least
one of those must be set (`400 nothing_to_mark` otherwise). Answers
`{updated, unread_counts}`.

## People axis

### `GET people/{author_id}/comments/?limit=<n>` — R

Comments written by one person, newest first. Exact match on
`comments.author_id` (source account id). Joins `items` for the parent issue or
page key/title. Served entirely from the mirror.

`limit` defaults to 50 and is capped at 200. An unknown `author_id` is **not**
`404`: the response is `200` with `total: 0` and an empty `comments` array so the
UI can render an empty person header.

```json
{
  "author": "Alex Kim",
  "total": 634,
  "comments": [
    {
      "key": "622723",
      "kind": "page",
      "title": "Research — Integrator Pain Points",
      "snippet": "Share with ENG architecture owners.",
      "created_at": "2026-08-05T11:31:43.827Z"
    },
    {
      "key": "NMB-140",
      "kind": "issue",
      "title": "batch worker drops the last page",
      "snippet": "Reproduced on staging.",
      "created_at": "2026-08-04T09:15:00.000Z"
    }
  ]
}
```

- `author` is the display name on the newest matching comment (`""` when none).
- `kind` is the parent item kind: `"issue"` or `"page"`.
- `snippet` is `body_text` (or ADF plain text when `body_text` is empty),
  whitespace-normalized, hard-cut at 160 runes (UTF-8 safe).
- `total` is the full count for that author; `comments` is the limited page.

## Deferred and cut

| Endpoint | Status | Reason |
| --- | --- | --- |
| `notifications/config/`, `notifications/subscription/` | D | Web Push needs VAPID keys. In-tab Notification is what shipped with the feed. |
| `presence-ticket/`, `ws/issues/` | X | Multi-viewer presence has no meaning in a single-user local tool. |
| `mentions/` | X | The client already had no caller for it. |
| `data-quality/` | X | Internal audit surface. |

Deferred endpoints must return `404` rather than `500`. The client treats a
failure as "feature unavailable" and hides the surface, but only if the failure
is clean.

## Config document

### `GET config.json` — R

Served at the UI's base path, not under the API base. Shape is
`GadakConfig` in `web/src/lib/config.ts`.

```json
{
  "apiBase": "/api/v1/issues/",
  "authBase": "/api/v1/auth/",
  "jiraBaseUrl": "https://your-site.atlassian.net",
  "qaDashboardUrl": "",
  "projects": ["NMB", "NMA"],
  "groupLabels": {},
  "groupColors": {},
  "productByGroup": {},
  "staleThresholdHours": 72,
  "features": { "feed": false, "push": false, "deploy": false, "qa": false, "teamGroups": false }
}
```

The UI fetches this before mount. A missing file is not fatal: the client falls
back to defaults with every optional feature off.

The document always carries a real `staleThresholdHours`: the client merges it
over its own defaults, so a literal `0` would override the 72-hour default and
mark every issue stale.

### `GET settings/` · `PUT settings/` — R

The settings UI reads and writes the configuration through these. The document is
the credential-free part of `~/.gadak/config.json` — `projects`, `fieldMap`,
`bodyFields`, `editableFields`, `members`, `groupRules`, `groupLabels`,
`groupColors`, `productByGroup`, `features`, `qaDashboardUrl`,
`staleThresholdHours`, `syncIntervalSec`, `reconcileIntervalSec` — plus
read-only context for the UI:

```json
{
  "site": "https://your-site.atlassian.net",
  "hasCredential": true,
  "projects": ["NMB"],
  "syncIntervalSec": 0,
  "reconcileIntervalSec": 0,
  "staleThresholdHours": 72,
  "runtime": {
    "profile": "default",
    "dbPath": "/Users/you/.gadak/gadak.db",
    "dbSizeBytes": 4823040,
    "dbSizeHuman": "4.6 MB",
    "dbModifiedAt": "2026-08-04T09:15:00Z",
    "configPath": "/Users/you/.gadak/config.json",
    "issueCount": 534,
    "commentCount": 339,
    "schemaVersion": 3,
    "watermark": "2026-08-04T09:15:00.000Z",
    "syncVersion": 41,
    "lastFullSyncAt": "2026-08-01T12:00:00.000Z",
    "lastError": null,
    "gadakVersion": "0.0.0-dev",
    "defaultSyncIntervalSec": 60,
    "defaultReconcileIntervalSec": 3600,
    "osNotifySupported": true,
    "apiUsage": {
      "today": {
        "day": "2026-08-05",
        "requests": 1204,
        "throttled": 2,
        "server_errors": 0,
        "retries": 2,
        "wait_ms": 4500,
        "last_throttled_at": "2026-08-05T14:03:00.000Z"
      },
      "last_7_days": {
        "requests": 8400,
        "throttled": 5,
        "server_errors": 1,
        "retries": 6,
        "wait_ms": 12000,
        "last_throttled_at": "2026-08-05T14:03:00.000Z"
      }
    }
  },
  "…": "…"
}
```

`runtime.apiUsage` is this process's outbound Jira call volume flushed into the
mirror after each sync cycle (today plus a 7-day rollup). It is **not** Jira's
remaining rate-limit point budget — the site does not expose that. Zeros mean
no traffic was flushed yet, not "unlimited headroom".

`PUT` replaces exactly the writable fields and preserves everything else in the
file, credentials included; `site` and the token stay the credential endpoint's
business. `runtime`, `site`, and `hasCredential` on the request body are
ignored. Unknown `features` keys are dropped, a negative threshold is clamped to
0 (meaning "use the client default"), and the response is the stored document
plus a freshly assembled `runtime`. There is no authentication here either —
the server is loopback-bound, and a config write is no more sensitive than the
file itself.

#### Sync intervals

| Field | Unit | `0` / omitted | Minimum when set | Default |
| --- | --- | --- | --- | --- |
| `syncIntervalSec` | seconds | use default | **15** | `60` |
| `reconcileIntervalSec` | seconds | use default | **300** (5 minutes) | `3600` |

A value below the floor answers `400` with a clear `error` string, e.g.
`syncIntervalSec must be 0 (default) or >= 15 (got 5)`. Rejected writes do not
touch the file.

**Apply timing:** `gadak serve` passes `Reload: config.Load` into `Watch`.
A successful `PUT` updates `config.json` and the in-process settings atomic
immediately (group rules, features, etc.). Interval changes are picked up on
the next watch tick (`tick.Reset`); no process restart.

`staleThresholdHours` is how long an unresolved issue may sit in its current
status — measured from `status_changed_at`, falling back to `updated_at` — before
the `stale` filter and the row badge pick it up.

#### `runtime` (GET only)

Read-only instance facts. **Never** includes credentials (`token`, `email`, or
anything derived from them beyond the top-level `site` / `hasCredential` already
documented). Paths are absolute. `profile` is `"default"` when no
`--profile` / `GADAK_PROFILE` is set. `gadakVersion` comes from `server.Version`
(package var; `cmd/gadak` should assign the ldflags version at startup — until
wired, the default is `0.0.0-dev`).

`apiUsage` (optional) carries `{today, last_7_days}` counters for outbound Jira
HTTP volume recorded by this installation. See the sample document above; field
semantics match the `api_usage` table in `data-model.md`.

`osNotifySupported` is whether this process can fire a real OS desktop
notification (`sync.OSNotifier.Supported`: macOS and Linux true, Windows and
others false). Always sent — `false` is the Windows no-op, not an omission.
The settings Features tab hides the in-tab browser Notification toggle only
when this is true *and* the surface is the desktop app.

Each `features` flag gates a surface that needs a capability the server may not
have: `feed` the personal feed and its polling, `push` Web Push and the service
worker registration, `deploy` the deploy column/filter plus the deploy and PR
detail sections, `qa` the QA column/filters, QA section, and inline field
editing, `teamGroups` the group filter, column, and the group/product grouping
modes. A flag that is off removes its surface from the catalog, so a shared URL
cannot bring the filter back.

## Auth

gadak is a single-user local tool. There are no gadak accounts and no session
tokens. **Identity is the stored Jira credential** (`site` / email / API token in
`~/.gadak/config.json`, managed via `credential/`). `GET me/` projects that
credential into `{email, account_id, name, department}` for the UI; when nothing is
configured it answers `200 {"email": null}` so the boot probe never 4xxes.
Writes that need Jira call out with that same credential. The UI has no login
dialog — if identity is missing it opens the credential settings dialog instead.

| Endpoint | Method | v0.1 behavior |
| --- | --- | --- |
| `me/` | GET | `{email, account_id, name, department}` from the stored credential and the configured member directory, with no call to Jira. `account_id` is null for credentials verified by an older build; `200 {"email": null}` when nothing is configured |
| `login/` | POST | `404`. There are no gadak accounts; use `PUT credential/` |
| `logout/` | POST | `404`. Clear credentials with `DELETE credential/` instead |
