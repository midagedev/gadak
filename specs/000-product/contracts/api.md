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
- `409 credential_required` tells the UI to open the credential dialog.
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
  "members": [ { "email": "...", "name": "...", "display_name": "...", "group": null, "department": null, "job_role": null, "avatar_url": null } ],
  "members_version": "sha256:...",
  "issues": [ /* IssueLite, see below */ ],
  "sync_health": { "jira": { "ok": true, "synced_at": "...", "error": null } }
}
```

- `ETag: "sv-<sync_version>"` must be set on 200.
- `server_time` is the cursor the client passes to `delta/`. It must come from
  the same clock that writes `items.synced_at`.
- `members` is derived from assignees and reporters present in the mirror. There
  is no team directory in v0.1, so `group`, `department`, and `job_role` are
  always `null` and the UI's group-based surfaces stay switched off by config.

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
  "attachments": [ { "id": "...", "filename": "...", "mime_type": "...", "size": 0, "content_url": "/api/v1/issues/NMB-142/attachments/10021/content/", "created_at": "..." } ],
  "comments": [ { "id": "...", "author": "...", "author_id": "...", "body_adf": {}, "created_at": "...", "updated_at": "..." } ],
  "history": [ { "at": "...", "author": "...", "field": "status", "from_value": "...", "to_value": "..." } ],
  "linked_issues": [ { "key": "NMA-8", "type": "Blocks", "direction": "inward", "summary": "...", "status_category": "done" } ],
  "development_opinion": null,
  "qa_context": null,
  "deploy": null,
  "linked_prs": []
}
```

`development_opinion`, `qa_context`, `deploy`, and `linked_prs` are **X** — they
were internal integrations. They stay in the response shape as `null`/`[]`
because the client already treats them as optional, and removing the keys buys
nothing.

### `GET search/?q=<text>&limit=<n>` — R

Server-side full-text over description and comment bodies via FTS5. The client
already does substring and chosung matching over the warm pool, so this endpoint
exists only to reach text the client does not hold.

```json
{ "keys": ["NMB-142", "NMA-8"], "total": 2 }
```

### `GET <key>/attachments/<id>/content/` — R

Streams or `303`-redirects to the attachment bytes, fetched from Jira on demand
with the user's credentials. Never mirrored to disk.

The client validates that a media URL matches exactly
`<apiBase><key>/attachments/<id>/content/` before using it as an image source,
so this path shape is load-bearing for XSS safety and must not change.

## IssueLite

The row shape used by `bootstrap`, `delta`, and every write response. Stored
verbatim in IndexedDB, so **field names are a contract** — adding is safe,
renaming or removing is not.

```
key, summary, project_key, issue_type, issue_type_id, status, status_id,
status_category, priority, priority_rank, assignee, assignee_id, assignee_email,
reporter, parent_key, labels[], components[], fix_versions[], duedate,
resolution, created_at, updated_at, status_changed_at, resolved_at,
reopen_count, reopened_at, comment_count
```

Fields the internal version carried that are now always absent: `d1_group`,
`deploy_status`, `qa_impact_*`, `qa_runs`, `qa_suites`,
`working_hours_in_status`, `development_test_result`, `source_project`. The UI
reads them defensively; the config flags that surface them default to off.

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
| `<key>/fields/` | PATCH | `PUT /issue/{key}` | R |
| `<key>/editmeta/` | GET | `GET /issue/{key}/editmeta` | R |
| `create/` | POST | `POST /issue` | R |
| `create-meta/` | GET | `GET /issue/createmeta` | R |
| `users/?q=` | GET | `GET /user/search` | R |
| `meta/write/` | GET | precomputed transition map + create meta | R |

### Credential storage

`PUT credential/` verifies the token against `/myself` and writes it to
`~/.scry/config.json` with mode `0600`. It is never written to SQLite, a log, or
a snapshot. `GET credential/` returns only `{configured, jira_email,
display_name, verified_at, token_hint}` — never the token.

### Field edits

`PATCH <key>/fields/` accepts `{field, value}` and rejects any field not present
in the configured editable-field allowlist. The internal version hardcoded three
custom field ids here; those are now configuration and the allowlist is empty by
default, which hides the inline editor entirely.

## Personal state

| Endpoint | Method | Backing | Status |
| --- | --- | --- | --- |
| `views/` | GET / POST | `saved_views` | R |
| `views/<id>/` | DELETE | `saved_views` | R |
| `watches/` | GET | `watches` | R |
| `watches/<key>/` | PUT / DELETE | `watches` | R |

Stored locally. `403`/`401` is never correct for these on a loopback bind.

## Deferred and cut

| Endpoint | Status | Reason |
| --- | --- | --- |
| `feed/`, `feed/read/` | D | Needs a per-user event stream. Local watch-based feed is a v0.2 design. |
| `notifications/config/`, `notifications/subscription/` | D | Web Push needs VAPID keys; only meaningful once the feed exists. |
| `presence-ticket/`, `ws/issues/` | X | Multi-viewer presence has no meaning in a single-user local tool. |
| `mentions/` | X | The client already had no caller for it. |
| `data-quality/` | X | Internal audit surface. |

Deferred endpoints must return `404` rather than `500`. The client treats a
failure as "feature unavailable" and hides the surface, but only if the failure
is clean.

## Config document

### `GET config.json` — R

Served at the UI's base path, not under the API base. Shape is
`ScryConfig` in `web/src/lib/config.ts`.

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
  "features": { "presence": false, "feed": false, "push": false, "deploy": false, "qa": false, "teamGroups": false }
}
```

The UI fetches this before mount. A missing file is not fatal: the client falls
back to defaults with every optional feature off.

## Auth

| Endpoint | Method | v0.1 behavior |
| --- | --- | --- |
| `me/` | GET | Returns the identity from the verified Jira credential |
| `login/` | POST | `404`. There are no scry accounts |
| `logout/` | POST | `404` |

The UI's login dialog is reachable only when `me/` fails, and its copy now points
at `scry serve` rather than any hosted login.
