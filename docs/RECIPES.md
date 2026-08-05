# Query recipes — the questions JQL cannot ask

Every recipe runs as-is with `scry sql "…"` (SQLite `mode=ro`) and
returns in milliseconds. `issues_full` is the view with the title included;
add `--json` or `--csv` for machines. Timestamps are ISO-8601 UTC strings, so
`julianday()` and string comparison both work.

None of these are expressible in JQL — that is the point. JQL cannot join, see
the changelog as data, aggregate, or read derived history.

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

**What went straight back after a "Done"?** Transitions as rows:

```sql
select i.key, ch.at, ch.author, ch.from_value, ch.to_value
from changelog ch
join issues i on i.item_id = ch.item_id
where ch.field = 'status' and ch.from_value = 'Done'
order by ch.at desc limit 20
```

**Unassigned and new, oldest first** — the queue nobody owns:

```sql
select key, summary, created_at from issues_full
where status_category = 'new' and assignee is null
order by created_at asc limit 20
```

## Full-text

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

## Releases

**Everything targeted at one fix version** (JQL cannot do ranges or joins over
versions; here they are a JSON array):

```sql
select i.key, i.summary, i.status
from issues_full i, json_each(i.fix_versions) v
where json_valid(i.fix_versions) and v.value = '2026.6.0'
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

---

Copy any of these into an agent prompt and it will adapt them — that is the
other reason they exist. The schema contract is
[`specs/000-product/data-model.md`](../specs/000-product/data-model.md).
