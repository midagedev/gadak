# Query recipes — the questions JQL cannot ask

Every recipe runs as-is with `gadak sql "…"` (SQLite `mode=ro`) and returns in
single-digit milliseconds on the demo snapshot (measured: `1–2 ms`,
`sqlite3 examples/demo.db` with `.timer on` on the reopen recipe below; full
`gadak sql` process wall-clock ≈16 ms including startup). `issues_full` is the
view with the title included; add `--json` or `--csv` for machines. Timestamps
are ISO-8601 UTC strings, so `julianday()` and string comparison both work.

None of these are expressible in JQL — that is the point. JQL cannot join, see
the changelog as data, aggregate, or read derived history. The inverse — a
Jira filter you already have — is `gadak search --jql '…'` (or paste the URL
into the search box), not this file.

Every recipe also runs against the demo snapshot in a plain browser tab, no
install, no account: [open the epic `GROUP BY` in Datasette
Lite](<https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc>)
and edit the SQL in place — the URL carries the query, so the link you share
is the answer.

## Triage

**What keeps coming back?** Reopen counts are derived at sync time from the
changelog — Jira has no field for this at all.

```sql
select key, summary, reopen_count, reopened_at, reopen_reason
from issues_full where reopen_count > 0
order by reopened_at desc limit 20
```

**What has been stuck in progress the longest?**

```sql
select key, summary, assignee,
       round(julianday('now') - julianday(status_changed_at), 1) as days_stuck
from issues_full
where status_category = 'inprogress'
order by status_changed_at asc limit 20
```

**What went straight back after a "Done"?**

```sql
-- from_value/to_value hold display names and localize per site; reopen_count is derived from status *categories* and is stable.
select key, summary, reopen_count, reopened_at, reopen_reason
from issues_full
where reopen_count > 0
order by reopen_count desc, reopened_at desc
limit 20
```

**Why did these close?** Resolution names localize per account (`완료` vs
`Done`); key on the stable id, never the display name. `issues.resolution_id`
is that id as of schema v27 (empty until a sync rewrites the row — same
contract as `priority_id`). The fence below uses `changelog.to_id`, which
already carries the same id on the committed demo snapshot.

```sql
select i.key, i.summary, c.to_id as resolution_id, i.resolved_at
from issues_full i
join changelog c on c.item_id = i.item_id
where c.field = 'resolution' and c.to_id != ''
order by i.resolved_at desc
limit 20
```

**Unassigned and new, oldest first** — the queue nobody owns:

```sql
select key, summary, created_at from issues_full
where status_category = 'new' and assignee is null
order by created_at asc limit 20
```

## Full-text

`gadak search` (and the REST/MCP search path) rewrites bare terms as FTS5 prefix
matches, so Korean particles and verb endings are found (`로그인` → `로그인이`,
`실패` → `실패합니다`) and English stems work too (`retri` → `retries`).
CJK *middles* hit — `결제` finds `간편결제`, because a `cjk_bigram` column
indexes overlapping 2-grams of CJK runs and the rewrite turns a CJK term into
the AND of its bigrams (`docs/decisions/0009`). English middles still miss on
purpose — `ency` will not hit `idempotency` (mid-token English matches were
measured at 0.30–0.34 precision and rejected).

**Search descriptions AND comments together** (FTS5, with AND/OR/NEAR):

```sql
select i.key, it.title
from items_fts f
join items it on it.rowid = f.rowid
join issues i on i.item_id = it.id
where items_fts match 'idempotency AND webhook' limit 20
```

**Who said what, when** — comments as data:

```sql
select i.key, c.author, c.created_at, substr(c.body_text, 1, 120) as excerpt
from comments c
join issues i on i.item_id = c.item_id
where c.body_text like '%rollback%'
order by c.created_at desc limit 20
```

## Team shape

**Open load per person:**

```sql
select assignee, count(*) as open_issues
from issues_full
where status_category != 'done' and assignee is not null
group by assignee order by open_issues desc
```

**Resolution throughput by week:**

```sql
select strftime('%Y-%W', resolved_at) as week, count(*) as resolved
from issues_full where resolved_at is not null
group by week order by week desc limit 12
```

**Who reassigns the most** — every assignee change is a changelog row:

```sql
select ch.author, count(*) as reassignments
from changelog ch where ch.field = 'assignee'
group by ch.author order by reassignments desc limit 10
```

**Everything one person said lately** — comments cross issues and wiki pages,
so this is one query here and two products upstream (the web UI's person
panel runs on the same axis):

```sql
select i.key, i.kind, substr(c.body_text, 1, 80) as opening, c.created_at
from comments c join items i on i.id = c.item_id
where c.author = 'Dana Whitfield'
order by c.created_at desc limit 20
```

**Issues a bot worker touched.** `issue_actors` is one touch per row across
comments, changelog and dev-panel links; `users` caches the origin's
`account_type`, where `agent` (standalone worker accounts) and `app` (Cloud
Connect) mean bot. Join on ids — display names localize and rename:

```sql
select distinct a.issue_key
from issue_actors a join users u
  on u.source_id = a.source_id and u.account_id = a.actor_id
where u.account_type in ('agent', 'app')
order by a.issue_key
```

## This sprint

**My open work in the active sprint.** Key on `sprint_state` (or `sprint_id`),
never `sprint_name` — names rename and localize. `active` is what
`sprint in openSprints()` maps to. The three columns are a projection of one
sprint per issue (active > future > closed, then larger id); they stay NULL
until a sync has rewritten the row on a site that has a sprint field.

```sql
select key, status, priority, summary
from issues_full
where sprint_state = 'active'
  and assignee_id = '<id from the person lookup>'
  and status_category != 'done'
order by priority_rank, updated_at desc
```

## Releases

**Everything targeted at one fix version** (JQL cannot do ranges or joins over
versions; here they are a JSON array):

```sql
select i.key, i.summary, i.status
from issues_full i, json_each(i.fix_versions) v
where json_valid(i.fix_versions) and v.value = '2026.6.0'
```

**What shipped this week.** Join on `versions.id` via `fix_version_ids`, never
on the name array — names rename. `released` is 0/1; `release_date` is the
origin's date-only string. Rows stay empty until a full or reconcile sync
has filled the catalog.

```sql
select i.key, i.summary, v.name, v.release_date
from issues_full i, json_each(i.fix_version_ids) j
join versions v on v.id = j.value
where json_valid(i.fix_version_ids)
  and v.released = 1
  and v.release_date >= date('now', '-7 day')
order by v.release_date desc, i.key
```

**What is on an unreleased train** (not released, not archived). Left join so
a train with no issues still appears:

```sql
select v.project_key, v.name, i.key, i.summary
from versions v
left join issues_full i on exists (
  select 1 from json_each(i.fix_version_ids) j where j.value = v.id
)
where v.released = 0 and v.archived = 0
order by v.project_key, v.name, i.key
```

**Issues carrying attachments over 1 MB** — worth a look before archiving:

```sql
select i.key, a.filename, a.size
from attachments a
join issues i on i.item_id = a.item_id
where a.size > 1048576 order by a.size desc limit 20
```

## Instance hygiene

**Field bloat** — which mapped custom fields are actually used (each mapped
field lands in the `custom` JSON column):

```sql
select j.key as field, count(*) as non_empty
from issues_full i, json_each(i.custom) j
where json_valid(i.custom) and j.value is not null and j.value != ''
group by j.key order by non_empty desc
```

**Mirror health** — is the data fresh enough to trust:

```sql
select watermark, last_full_sync_at, sync_count, first_sync_at, last_error
from sync_state where source_id = 'jira'
```

## Custom fields

`issues.custom` is filled only after field mapping (`gadak fields --apply`).
Until then every row is `{}` — an empty `json_extract` is "not mapped", not
"the field is blank". `gadak status --json` reports `custom_fields.mapped`;
`gadak doctor` names the case where raw still carries unmapped
`customfield_` keys. demo.db is all `{}` (measured), so these fences return
zero rows there; they are valid SQL.

**A scalar alias** (story points, a select):

```sql
select key, json_extract(custom, '$.story_points') as sp
from issues_full
where json_extract(custom, '$.story_points') is not null
```

**An array alias** (multi-select, a labels-like axis) needs `json_each`:

```sql
select i.key, je.value
from issues_full i, json_each(i.custom, '$.labels_axis') je
```

## Show on the app

SQL answers; `gadak views open` presents. When the ask is to *see* a set, pipe
the keys into the running app instead of pasting a table. `--keys` keeps
first-seen order, so the `ORDER BY` is what the list shows. `gadak sql` prints
a header row first — skip it (`tail -n +2`), or that word becomes a key. Select only `key`:
`--keys` splits on commas and whitespace.
`-- or: gadak sql --no-header "…" (same rows, no header line)`.

**Put this label's unresolved issues on screen:**

```bash
gadak sql --no-header "select i.key
from issues_full i, json_each(i.labels) l
where json_valid(i.labels) and l.value = 'batch' and i.status_category != 'done'
order by i.priority_rank, i.updated_at desc" | gadak views open --keys -
```

---

Copy any of these into an agent prompt and it will adapt them — that is the
other reason they exist. The schema contract is
[`specs/000-product/data-model.md`](../specs/000-product/data-model.md).
