# Plugins

gadak mirrors an issue tracker. It does not know what a deployment is, what a test
run is, or who reviewed what — and it never will, because every one of those
integrations was the reason the tool it was extracted from could not be shared.

So the boundary is a table, not an API. There are **zero lines of external
integration code in this repository** (verified 2026-08-06:
`grep -R --include='*.go' -E 'go-github|slack-go|andygrunwald|xanzy/go-gitlab'
internal/ cmd/` → empty; plugin reference code lives only under
`examples/plugins/`): an integration runs as its own process, in
any language, and writes rows into `enrichments` with SQL. The server merges what
it finds into its responses. It never calls a plugin, never loads one, and has no
plugin registry, lifecycle, or manifest.

If your plugin can open a SQLite file, it is compatible.

PR URLs on an issue are first-class `dev_links` (`gadak issue KEY --json`;
SQL joins `dev_links` on `item_id`). On standalone, `gadak dev link` /
`gadak dev scan` write that table through the origin; on connected Cloud,
`gadak config set devStatus true` mirrors Jira's development panel into it
(read). The `kind=prs` plugin is extra metadata on top of that, not the
way to attach a PR.

**Runnable examples** (Python 3 stdlib, copy-pasteable):
[`examples/plugins/`](../examples/plugins/) — GitHub PRs, git-tag deploy status,
and CSV import. How to choose config vs plugins vs SQL:
[`EXTENDING.md`](EXTENDING.md).

## The table

```sql
CREATE TABLE enrichments (
  key        TEXT NOT NULL,       -- issue key, e.g. NMB-42
  kind       TEXT NOT NULL,       -- deploy | qa | prs | opinion
  payload    TEXT NOT NULL,       -- JSON, shape per kind below
  source     TEXT NOT NULL DEFAULT '',  -- your plugin's name, informational
  updated_at TEXT NOT NULL,
  PRIMARY KEY (key, kind)
);
```

There is no foreign key on `key`: a plugin may write before the issue is
mirrored, or for an issue that has left the mirror's scope. Rows for unknown keys
are simply never read.

The mirror lives at `~/.gadak/gadak.db` (or `$GADAK_HOME/gadak.db`, or a profile under
`~/.gadak/profiles/<name>/gadak.db`). It is opened in WAL mode, so a writer and the
running server coexist without either blocking the other. Plugins should set
`PRAGMA busy_timeout=5000` and keep transactions short.

## Writing a row

```sql
INSERT INTO enrichments (key, kind, payload, source, updated_at)
VALUES ('NMB-42', 'deploy', json('{"status":{"state":"prod","merged_prs":2,"total_prs":2}}'),
        'my-deploy-plugin', strftime('%Y-%m-%dT%H:%M:%fZ','now'))
ON CONFLICT(key, kind) DO UPDATE SET
  payload = excluded.payload, source = excluded.source, updated_at = excluded.updated_at;

-- Make the change visible: the ETag and the server's cache both key on this.
UPDATE sync_state SET version = version + 1;
```

That version bump is not optional. The client holds the whole mirror in
IndexedDB and only refetches when `sync_state.version` moves; the server caches
the merged view against the same number. Without the bump your row is on disk and
nowhere else. One bump after a batch of rows is enough.

Wrap the payload in `json()` as above (or pass a string that is already valid
JSON from your language’s encoder). Invalid JSON is dropped at read time rather
than spliced into a response — a plugin cannot corrupt the API by writing
garbage, but it also gets no error, so validate on your side.

End to end, with the CLI:

```sh
sqlite3 ~/.gadak/gadak.db <<'SQL'
INSERT INTO enrichments (key, kind, payload, source, updated_at)
VALUES ('NMB-42', 'deploy',
        json('{"status":{"state":"prod","merged_prs":2,"total_prs":2,"dev":null,"qa_release":null,"qa_swapped_at":null,"prod_at":"2026-08-04T02:00:00Z"},
               "detail":{"state":"prod","releases":[{"tag":"v1.2.3","at":"2026-08-04T02:00:00Z","channel":"prod"}]}}'),
        'my-deploy-plugin', strftime('%Y-%m-%dT%H:%M:%fZ','now'))
ON CONFLICT(key, kind) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at;
UPDATE sync_state SET version = version + 1;
SQL

curl -s localhost:7777/api/v1/issues/NMB-42/detail/ | jq .deploy
```

## Kinds

Every kind feeds one or two fields the UI already renders. Authority on field
names is the **server merge** in `internal/server/read.go` (`enrichRow` for the
list, `handleDetail` for detail) and the client types in `web/src/lib/types.ts`.

| `kind` | Payload root | List field(s) | Detail field |
| --- | --- | --- | --- |
| `deploy` | `{ "status": DeployStatus, "detail": DeployDetail }` (or bare either) | `deploy_status` ← `status` (or whole payload if unwrapped) | `deploy` ← `detail` (or whole) |
| `qa` | `{ "impact": {…}, "context": QaIssueContext }` (or bare either) | keys inside `impact` spread onto the row | `qa_context` ← `context` (or whole) |
| `prs` | `LinkedPr[]` (JSON array) | — | `linked_prs` |
| `opinion` | JSON string (a quoted string value) | — | `development_opinion` |

A payload that is not wrapped is passed through whole (`pick` falls back to `p`),
so writing a bare `DeployStatus` also works — it feeds both the list and detail
paths with the same object. Wrap when the list row and the detail panel want
different amounts of detail, which is the usual case for `deploy` and `qa`.

Keys in `qa.impact` are spread into the list row, but they are serialized
**before** the mirrored fields, so a plugin cannot shadow `status`, `summary`, or
anything else that came from the tracker: the mirror’s value is later in the
object and wins in `JSON.parse`. Enrichments add to a row; they never overwrite
it.

---

### `deploy` — payload fields

Confirmed against `DeployStatus` / `DeployDetail` in `web/src/lib/types.ts` and
server tests in `internal/server/server_test.go`.

**`status` (list badge → `deploy_status`)**

| Field | Type | Notes |
| --- | --- | --- |
| `state` | string | `none` \| `merged` \| `dev` \| `qa_preview` \| `qa` \| `prod` |
| `merged_prs` | number | How many linked PRs are merged |
| `total_prs` | number | How many linked PRs were considered |
| `dev` | `{tag, at}` \| null | First dev release containing the fix |
| `qa_release` | `{tag, at}` \| null | QA channel release |
| `qa_swapped_at` | string \| null | ISO time QA became verifiable |
| `prod_at` | string \| null | ISO time of production |

**`detail` (detail panel → `deploy`)** — all of the above, plus:

| Field | Type | Notes |
| --- | --- | --- |
| `releases` | `{tag, html_url?, at?, channel?}[]` | Evidence list; `channel` is `dev`/`qa`/`prod` |
| `prs` | `{number, title?, url?, repo?, merged?, included_in?}[]` | Per-PR inclusion evidence |

**Minimal write + check**

```sql
INSERT INTO enrichments (key, kind, payload, source, updated_at)
VALUES (
  'NMB-42', 'deploy',
  json('{"status":{"state":"prod","merged_prs":1,"total_prs":1,"dev":null,"qa_release":null,"qa_swapped_at":null,"prod_at":"2026-08-04T02:00:00Z"},"detail":{"state":"prod","releases":[{"tag":"v1.2.3","channel":"prod","at":"2026-08-04T02:00:00Z"}]}}'),
  'example', strftime('%Y-%m-%dT%H:%M:%fZ','now')
)
ON CONFLICT(key, kind) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at;
UPDATE sync_state SET version = version + 1;

SELECT key, json_extract(payload, '$.status.state') AS state
FROM enrichments WHERE kind = 'deploy' AND key = 'NMB-42';
```

Example implementation: [`examples/plugins/deploy-status/`](../examples/plugins/deploy-status/).

---

### `qa` — payload fields

**`impact` (spread onto list row)** — use the client’s own field names:

| Field | Type | Notes |
| --- | --- | --- |
| `qa_impact_state` | string | `blocking` \| `retest` \| `verified` \| `linked` \| `""` |
| `qa_impact_label` | string | Display label (any locale) |
| `qa_runs` | `{key, label}[]` | Run chips on the list |
| `qa_suites` | `{key, label, path}[]` | Suite chips on the list |

**`context` (detail → `qa_context`)**

| Field | Type | Notes |
| --- | --- | --- |
| `state` | string | Same enum as `qa_impact_state` (non-empty) |
| `state_label` | string | Display label |
| `runs` | `QaRunContext[]` | Rich run objects (see `types.ts`) |
| `suites` | `{key, label, path}[]` | Suites linked to the issue |

**Minimal write + check**

```sql
INSERT INTO enrichments (key, kind, payload, source, updated_at)
VALUES (
  'NMB-42', 'qa',
  json('{"impact":{"qa_impact_state":"blocking","qa_impact_label":"Blocking","qa_runs":[{"key":"R-1","label":"Regression"}],"qa_suites":[]},"context":{"state":"blocking","state_label":"Blocking","runs":[],"suites":[]}}'),
  'example', strftime('%Y-%m-%dT%H:%M:%fZ','now')
)
ON CONFLICT(key, kind) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at;
UPDATE sync_state SET version = version + 1;

SELECT key, json_extract(payload, '$.impact.qa_impact_state') AS state
FROM enrichments WHERE kind = 'qa' AND key = 'NMB-42';
```

---

### `prs` — payload fields

Payload is a **JSON array** (not an object). Each element:

| Field | Type | Notes |
| --- | --- | --- |
| `number` | number | PR number |
| `title` | string | Title |
| `url` | string | Browser URL |
| `state` | string | UI chips: `open`, `merged`, `closed` (case-insensitive) |
| `repo` | string \| null | `owner/name` preferred |
| `author` | string \| null | Login or display name |

**Minimal write + check**

```sql
INSERT INTO enrichments (key, kind, payload, source, updated_at)
VALUES (
  'NMB-42', 'prs',
  json('[{"number":7,"title":"fix the drop","url":"https://github.com/example/app/pull/7","state":"merged","repo":"example/app","author":"alice"}]'),
  'github-prs', strftime('%Y-%m-%dT%H:%M:%fZ','now')
)
ON CONFLICT(key, kind) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at;
UPDATE sync_state SET version = version + 1;

SELECT key, json_array_length(payload) AS n
FROM enrichments WHERE kind = 'prs' AND key = 'NMB-42';
```

Example implementation: [`examples/plugins/github-prs/`](../examples/plugins/github-prs/).

---

### `opinion` — payload fields

Payload is a **JSON string** (the value itself is quoted JSON text), shown as
`development_opinion` in detail.

| Field | Type | Notes |
| --- | --- | --- |
| *(root)* | string | Free-text note; store as `json('"…text…"')` |

**Minimal write + check**

```sql
INSERT INTO enrichments (key, kind, payload, source, updated_at)
VALUES (
  'NMB-42', 'opinion',
  json('"Repro is narrow; safe to ship behind the flag."'),
  'review-bot', strftime('%Y-%m-%dT%H:%M:%fZ','now')
)
ON CONFLICT(key, kind) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at;
UPDATE sync_state SET version = version + 1;

SELECT key, payload FROM enrichments WHERE kind = 'opinion' AND key = 'NMB-42';
```

Generic bulk path for any kind: [`examples/plugins/csv-import/`](../examples/plugins/csv-import/).

---

## Turning the surfaces on

Merged data alone renders nothing. The UI gates each surface behind a feature
flag in `~/.gadak/config.json`, and every flag is off by default:

```json
{ "features": { "deploy": true, "qa": true } }
```

`deploy` switches on the deploy column, its filter, and the deploy and PR
sections in the detail panel. `qa` switches on the QA column, the QA filters and
the QA section. A flag that is off removes the surface from the catalog
entirely, so nothing half-populated ever shows up.

## What not to put here

Enrichments are lost when the mirror is deleted and re-synced, and that is
correct: they are derived from a system that still has them. Nothing a user would
mourn may live only in this table — if your plugin is the only copy of something,
your plugin needs its own storage.

Do not put credentials in `payload`. Do not invent a new `kind` expecting the UI
to render it without a core change. See [`EXTENDING.md`](EXTENDING.md) §5.
