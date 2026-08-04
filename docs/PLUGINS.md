# Plugins

scry mirrors an issue tracker. It does not know what a deployment is, what a test
run is, or who reviewed what — and it never will, because every one of those
integrations was the reason the tool it was extracted from could not be shared.

So the boundary is a table, not an API. There are **zero lines of external
integration code in this repository**: an integration runs as its own process, in
any language, and writes rows into `enrichments` with SQL. The server merges what
it finds into its responses. It never calls a plugin, never loads one, and has no
plugin registry, lifecycle, or manifest.

If your plugin can open a SQLite file, it is compatible.

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

The mirror lives at `~/.scry/mirror.db` (or wherever `--db` points). It is opened
in WAL mode, so a writer and the running server coexist without either blocking
the other.

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

Wrap the payload in `json()` as above. Invalid JSON is dropped at read time
rather than spliced into a response — a plugin cannot corrupt the API by writing
garbage, but it also gets no error, so validate on your side.

End to end, with the CLI:

```sh
sqlite3 ~/.scry/mirror.db <<'SQL'
INSERT INTO enrichments (key, kind, payload, source, updated_at)
VALUES ('NMB-42', 'deploy',
        json('{"status":{"state":"prod","merged_prs":2,"total_prs":2,"dev":null,"qa_release":null,"qa_swapped_at":null,"prod_at":"2026-08-04T02:00:00Z"},
               "detail":{"state":"prod","releases":[{"tag":"v1.2.3","at":"2026-08-04T02:00:00Z"}]}}'),
        'my-deploy-plugin', strftime('%Y-%m-%dT%H:%M:%fZ','now'))
ON CONFLICT(key, kind) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at;
UPDATE sync_state SET version = version + 1;
SQL

curl -s localhost:7777/api/v1/issues/NMB-42/detail/ | jq .deploy
```

## Kinds

Every kind feeds one or two fields the UI already renders. The authority on each
shape is `web/src/lib/types.ts` — the client parses these, so its type wins over
any prose here.

| `kind` | Payload | Fills |
| --- | --- | --- |
| `deploy` | `{"status": DeployStatus, "detail": DeployDetail}` | `deploy_status` on every list row, `deploy` in detail |
| `qa` | `{"impact": {…list fields}, "context": QaIssueContext}` | `qa_impact_state` / `qa_impact_label` / `qa_runs` / `qa_suites` on list rows, `qa_context` in detail |
| `prs` | `LinkedPr[]` | `linked_prs` in detail |
| `opinion` | a JSON string | `development_opinion` in detail |

A payload that is not wrapped is passed through whole, so writing a bare
`DeployStatus` (or a bare `QaIssueContext`) also works — it just feeds only the
field whose shape it matches. Wrap when the list row and the detail panel want
different amounts of detail, which is the usual case for `deploy`.

The `qa` payload's `impact` object is **keyed by the client's own field names**,
so the list half needs no translation:

```json
{
  "impact": {
    "qa_impact_state": "blocking",
    "qa_impact_label": "차단 중",
    "qa_runs":   [{ "key": "R-12", "label": "Regression 12" }],
    "qa_suites": [{ "key": "S-3", "label": "Checkout", "path": "web/checkout" }]
  },
  "context": {
    "state": "blocking", "state_label": "차단 중", "runs": [], "suites": []
  }
}
```

Keys in `impact` are spread into the list row, but they are serialized **before**
the mirrored fields, so a plugin cannot shadow `status`, `summary`, or anything
else that came from the tracker: the mirror's value is later in the object and
wins in `JSON.parse`. Enrichments add to a row; they never overwrite it.

## Turning the surfaces on

Merged data alone renders nothing. The UI gates each surface behind a feature
flag in `~/.scry/config.json`, and every flag is off by default:

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
