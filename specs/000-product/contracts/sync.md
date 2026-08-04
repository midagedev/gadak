# Sync Contract

How the mirror is filled and kept current. The rules here exist because a mirror
that is subtly stale is worse than no mirror: the UI looks authoritative either
way.

## Invariants

1. **Jira is the record.** Sync only ever writes to the mirror. It never writes
   to Jira.
2. **A failed sync leaves the last good mirror intact** and does not advance the
   watermark. Partial progress is committed per page, so a failure halfway
   through keeps the pages already written.
3. **The watermark only moves forward** and only after the page that justified it
   is committed.
4. **Deletions propagate.** An issue that leaves scope must leave the mirror.
5. **Derived fields are recomputed from the changelog**, never carried over from
   a previous row.

## Full sync

Triggered when the mirror has no rows for a source, when `--full` is passed, or
when the schema version changed.

```
for each configured project:
  JQL: project = <KEY> ORDER BY created ASC
  GET /rest/api/3/search/jql
    fields: <field list>          # explicit, never *all
    expand: changelog
    maxResults: 100
    nextPageToken: <cursor>       # token pagination, not startAt
  for each page:
    upsert items + issues + comments + attachments + changelog + links
    recompute derived fields
    commit
  record watermark = max(updated) seen
```

Notes:

- `POST /rest/api/3/search/jql` is used with token pagination. The legacy
  `startAt` search API is deprecated and drifts under concurrent writes.
- `expand=changelog` returns at most the most recent entries for very long
  histories. When an issue reports a truncated changelog, sync fetches
  `GET /issue/{key}/changelog` separately and pages it.
- Comments are paged separately for issues with more than the inline limit.
- The field list comes from configuration. Requesting `*all` on a large site is
  slow and pulls custom fields nobody mapped.

## Incremental sync

```
JQL: project in (<KEYS>) AND updated >= "<watermark - overlap>" ORDER BY updated ASC
```

- `overlap` defaults to 2 minutes. Jira's `updated` has minute granularity in
  JQL, so an exact `>=` boundary drops issues updated within the same minute as
  the watermark. The overlap makes re-processing harmless and loss impossible.
- Re-processing is idempotent: every write is an upsert keyed on the source id.
- Bumps `sync_state.version` once per completed run, which is what invalidates
  the UI's `bootstrap` ETag.

## Deletion detection

Jira does not report deletions in a search result, so absence has to be proven.

- **Cheap pass, every incremental run:** none. Absence cannot be inferred from an
  `updated >=` window.
- **Reconcile pass, on a schedule (default hourly) and on `--full`:** fetch all
  keys in scope with a key-only field list, diff against the mirror's key set,
  and delete the difference.
- A key that 404s on a detail fetch is deleted immediately.

Reconcile is a separate pass because it is the only operation whose cost scales
with total issue count rather than change volume.

## Derived field computation

Computed after each issue's changelog is written, in one pass over that issue's
entries ordered by time. Rules are in `../data-model.md`.

The reopen rule is worth restating because it replaces a site-specific one: a
reopen is any transition **from** a status whose category is `done` **to** one
whose category is not. The internal version matched on status names
(`"Reopened"`, `"다시 열림"`), which breaks on every other site and on every
account language. Categories are stable across both.

## Field mapping

Custom fields are configuration, never code:

```json
{
  "fieldMap": {
    "severity": "customfield_10050",
    "solution": "customfield_10092"
  },
  "bodyFields": ["customfield_10101"]
}
```

- `fieldMap` entries land in `issues.custom` keyed by the alias.
- `bodyFields` are ADF fields flattened into `description_text` so they are
  searchable, which is how a site that keeps its repro steps in a custom field
  still gets useful full-text search.
- The repository ships an empty map. No site's ids are committed.

## Localization hazard

Jira returns `issuetype.name` and `status.name` translated into the **account's**
display language and ignores `Accept-Language`. Two consequences:

1. Sync stores both the display name and the id, and all logic keys on the id or
   on `statusCategory`.
2. Anything that needs canonical English names (tooling, fixtures, the seed
   script) reads `GET /rest/api/3/project/{key}/statuses`, which is not
   localized.

## Rate limits and backoff

- Retry on `429`, `500`, `502`, `503`, `504` with exponential backoff capped at
  30 s; respect `Retry-After` when present.
- At most 4 concurrent requests per source by default.
- A `401` or `403` aborts the run immediately and records `last_error`; retrying
  a bad credential just burns the rate budget.

## Watch mode

`scry sync --watch` runs incremental sync on an interval (default 60 s) and a
reconcile pass on a longer one (default 3600 s). `scry serve` runs the
same loop inside the server process so a single command is enough for normal use.

## Snapshot generation

`scry snapshot <out.db>` produces a shareable database for demos and tests:

- Copies `items`, `issues`, `comments`, `attachments` (metadata only),
  `changelog`, and `links`.
- Drops `saved_views`, `watches`, `favorites`, and every credential-bearing row.
- Optionally rewrites timestamps to spread them over a requested window, because
  Jira assigns `created` at insert time and seeded demo data is otherwise all
  created within minutes of itself.
- Optionally scales volume by cloning issues with new keys, for benchmarking the
  10k-issue latency target without needing a 10k-issue Jira site.
- Refuses to write if any credential-shaped string is found in the output.
