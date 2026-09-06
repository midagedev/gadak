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
into the search box), not this file. The subset that path accepts is not
function-free either: `currentUser()` on `assignee`/`reporter`, `sprint in
openSprints()`, and `now()` / `startOfDay()` / `endOfDay()` inside `created` /
`updated` / `due` / `resolved` comparisons all compile. Anything past that
list is skipped with a notice on stderr — never silently re-matched.

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

## Blockers and duplicates

**What blocks this issue?** Every link is stored from both ends, and
`direction` reads from the row's own side: on the blocked issue the row is
`inward` — this issue is blocked by `target_key` — while the blocking issue
carries the same link as its `outward` row. `type` is the origin's link-type
name (`Blocks`, `Duplicate`, `Relates`, …), and `target_key` may point at an
issue the mirror has not synced — a blocker missing from this join is not
proof that none exists:

```sql
select l.target_key as blocked_by, t.status_category, t.summary
from links l
join issues_full i on i.item_id = l.item_id
join issues_full t on t.key = l.target_key
where i.key = 'NMA-24' and l.type = 'Blocks' and l.direction = 'inward'
```

**What does this issue block?** The same join on the row's other side —
these are the issues waiting on it:

```sql
select l.target_key as blocks, t.status_category, t.summary
from links l
join issues_full i on i.item_id = l.item_id
join issues_full t on t.key = l.target_key
where i.key = 'NMA-24' and l.type = 'Blocks' and l.direction = 'outward'
```

**Every open issue an open blocker holds back** — the axis `gadak ready`
walks (it resolves the blocking link type against the origin's catalog
instead of a literal, and says so on stderr when no catalog can answer). The
target join is a left join on purpose: a target outside the mirror is never
known-done:

```sql
select i.key, l.target_key as blocked_by, t.status_category
from links l
join issues_full i on i.item_id = l.item_id
left join issues_full t on t.key = l.target_key
where l.type = 'Blocks' and l.direction = 'inward'
  and i.status_category != 'done'
  and (t.status_category is null or t.status_category != 'done')
order by i.priority_rank, i.key
```

**The duplicates network** — stored like any other link, and the `inward`
row is "this issue duplicates `target_key`". Drop the direction filter to
see both ends of every pair:

```sql
select i.key, i.status_category, l.target_key as duplicate_of
from links l
join issues_full i on i.item_id = l.item_id
where l.type = 'Duplicate' and l.direction = 'inward'
order by i.key
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

**Who was @-mentioned where.** A mention is an ADF node, not a word:
`body_text` LIKE matches the word in prose (on the demo snapshot, comment
text containing "mention" is sentences about mid-sentence mentions) and
cannot resolve to a person. The node in `body_adf` carries the account id,
which display text cannot. The demo snapshot carries no mention nodes — this
fence returns zero rows there and is valid SQL (same contract as the
custom-field fences below):

```sql
with recursive adf(node) as (
  select body_adf from comments where body_adf is not null
  union all
  select child.value from adf, json_each(adf.node, '$.content') child
)
select json_extract(node, '$.attrs.id') as mentioned_id,
       json_extract(node, '$.attrs.text') as mentioned_as
from adf
where json_extract(node, '$.type') = 'mention'
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

**The same question keyed on `author_id`.** The fence above filters on a
display name — right for a skim, wrong as a key: names rename, and a
colliding name is two people. `comments.author_id` is the stable one (the
actor slug on the built-in tracker, local or paired, the origin's account id on
Cloud), the same column the other write surfaces key on:

```sql
-- ids on this mirror: select distinct author_id, author from comments
select i.key, i.kind, substr(c.body_text, 1, 80) as opening, c.created_at
from comments c join items i on i.id = c.item_id
where c.author_id = 'demo-dana'
order by c.created_at desc limit 20
```

**Issues a bot worker touched.** `issue_actors` is one touch per row across
comments, changelog and dev-panel links; `users` caches the origin's
`account_type`, where `agent` (the built-in tracker's worker accounts) and `app` (Cloud
Connect) mean bot. Join on ids — display names localize and rename:

```sql
select distinct a.issue_key
from issue_actors a join users u
  on u.source_id = a.source_id and u.account_id = a.actor_id
where u.account_type in ('agent', 'app')
order by a.issue_key
```

## What a session left

**Everything one actor left, across all four write surfaces** — on the
built-in tracker, local or paired, every write records its actor
(`GADAK_ACTOR`, or `claude:<session prefix>` auto-detected for Claude
Code), so "what did the last session do" is a query, not archaeology.
Issues and pages it created, comments it wrote, fields it moved, page
versions it saved — one timeline, newest first. Swap in the actor id you
are asking about (`gadak status --json` prints the current one under
`actor.slug`); `issue_actors` above is the coarse first pass (which
issues), this is the full answer (what, when):

```sql
select at, kind, ref, what from (
  select it.created_at as at, it.kind, coalesce(i.key, it.key) as ref,
         'created: ' || it.title as what
  from items it left join issues i on i.item_id = it.id
  where it.author_id = 'claude:354bff2b'
  union all
  select c.created_at, it.kind, coalesce(i.key, it.key),
         'comment: ' || substr(c.body_text, 1, 80)
  from comments c join items it on it.id = c.item_id
  left join issues i on i.item_id = it.id
  where c.author_id = 'claude:354bff2b'
  union all
  select g.at, it.kind, coalesce(i.key, it.key),
         'changed ' || g.field || ': ' || coalesce(g.from_value, '')
           || ' -> ' || coalesce(g.to_value, '')
  from changelog g join items it on it.id = g.item_id
  left join issues i on i.item_id = it.id
  where g.author_id = 'claude:354bff2b'
  union all
  select v.created_at, 'page', it.key,
         'edited v' || v.number || coalesce(': ' || v.message, '')
  from page_versions v join items it on it.id = v.item_id
  where v.author_id = 'claude:354bff2b' and v.number > 1
) order by at desc limit 50
```

For `kind = 'page'` the ref is the origin page id (`items.key` — what
`gadak page edit` takes). Page version 1 is excluded — it is the same
event as the `created:` row. On a Jira Cloud workspace the same
query works with the Atlassian `accountId` as the author id.

## Flow

The steward's questions — what is aging, what is neglected, what a person
handed to others and hears nothing about, how priority is distributed, who
waits on whom (`docs/project/THEORY.md`, tenet T4: age is the risk). The
row is always an issue; people appear as columns, never as rankings. Weekly
throughput and closed-count live in **Resolution throughput by week** (Team
shape) and the Retro section — this pack is the live tail, not the report.
One clock caveat covers every fence here, the one `docs/MIRROR.md` states
for its stuck query: *the built-in tracker's clock can sit far behind wall
time (GDK-369); `julianday('now')` then mis-ages rows.* The demo snapshot's
own clock is frozen at its regeneration date, so its ages drift up a day
per day — read them as a shape, not a number.

**Aging in progress, oldest first, with the p85 line.** The age that matters
is time in the current status — the same axis retro's `wip age p85` reads —
so this is the *What has been stuck in progress the longest?* query with the
whole tail kept, plus the percentile that turns it into a judgement: an
in-progress issue past the set's own 85th percentile is not going to be
late, it already is. Two ages ride as columns now (v43): `status_changed_at`
is *stuck where it is*, `started_at` is *underway at all* — the first
transition into an in-progress category, so a row that hopped statuses
yesterday after six weeks of work still shows its six weeks. The p85 is
nearest-rank in the same integer `(85n+99)/100` form the Retro section
derives:

```sql
with ages as (
  select key, summary, assignee,
         julianday('now') - julianday(status_changed_at) as days,
         julianday('now') - julianday(started_at) as underway_days
  from issues_full
  where status_category = 'inprogress'
),
ranked as (
  select ages.*, row_number() over (order by days) as rn,
         count(*) over () as n
  from ages
)
select key, round(days, 1) as age_days,
       round(underway_days, 1) as underway_days, assignee,
       round(max(case when rn = (85 * n + 99) / 100 then days end) over (), 1)
         as wip_age_p85
from ranked
order by days desc
```

A row with an empty `underway_days` never logged a transition into progress —
created straight into the state, its history starts at `created_at`.

144 rows on the demo snapshot (all 144 with a `started_at`, underway
anywhere from 3 to ~90 days), p85 ≈ 78 days as of its regeneration — it
drifts with the frozen clock. The better threshold is the
*cycle-time* p85, how long completed work actually took: first entry into an
in-progress status to resolution. Since v43 that is a stored column —
`cycle_hours` (`resolved_at` − `started_at`, DERIVE.md) is written by every
sync and kept only while the issue is done now with a positive span — so the
walk is a filter. No `status_catalog` join, no changelog read, and the
shipped demo fixture answers too (the v43 migration backfilled the column
from the mirror's own rows):

```sql
with cycle as (
  select cycle_hours / 24.0 as days
  from issues_full
  where cycle_hours is not null
)
select round(days, 1) as cycle_time_p85
from cycle
order by days
limit 1 offset ((85 * (select count(*) from cycle) + 99) / 100 - 1)
```

56 finished samples on the demo snapshot, every one under two seconds — its
synthetic issues live out whole lifecycles in moments, so the number is 0.0
days and a live mirror answers in the days-to-weeks range. The shape of the
query is what carries over.

**Neglected: in progress, silent for a week.** DeGrandis's fifth time thief
is *neglected work* — started, then touched by no field change and no
comment for 7 days. Silence used to be a changelog-plus-comments CTE; since
v43 it is the `last_activity_at` column (the newest of the item's `updated`
stamp, the newest changelog entry and the newest comment — so a priority
edit counts as activity even when no comment follows), falling back to
creation when nothing was ever observed:

```sql
select key, summary, assignee,
       round(julianday('now')
             - julianday(coalesce(last_activity_at, created_at)), 1) as days_silent
from issues_full
where status_category = 'inprogress'
  and julianday(coalesce(last_activity_at, created_at)) < julianday('now', '-7 day')
order by coalesce(last_activity_at, created_at) asc
```

136 of the snapshot's 144 in-progress issues qualify at regeneration, the
quietest ~90 days — the tail of a 90-day spread is mostly older than a week,
which is itself the demonstration of why this list is worth having.

**The delegation ledger: what one person reported and others hold, by
silence.** The reporter's side of the queue — open issues `:me` filed that
someone else, or no one, is carrying — the quietest first. The row is the
issue and the assignee is a column: a ledger, not a leaderboard. Silence is
the same `last_activity_at` column the neglected recipe reads. Swap in your
own account id — `gadak status --json` prints the current one under
`actor.slug`, and `select account_id, name from users` lists the mirror's
(empty on the shipped demo fixture, like `status_catalog`):

```sql
select key, summary, assignee,
       round(julianday('now')
             - julianday(coalesce(last_activity_at, created_at)), 1)
         as days_since_activity
from issues_full
where status_category != 'done'
  and reporter_id = 'demo-dana'
  and (assignee_id is null or assignee_id != reporter_id)
order by days_since_activity desc
```

With `demo-dana` (Dana Whitfield) this is 60 rows on the snapshot, the
quietest at ~90 days.

**Priority distribution — is priority carrying information?** Open issues
by `priority_rank` (1 = most urgent, 0 = unset), the display `priority`
joined for reading only. No threshold in the SQL — the reader judges: when
one rank holds most rows, priority is a label, not an order, and every
sort-by-priority is sorting by noise:

```sql
select priority_rank, priority, count(*) as issues,
       round(100.0 * count(*) / sum(count(*)) over (), 1) as pct
from issues_full
where status_category != 'done'
group by priority_rank
order by priority_rank
```

On the snapshot Medium holds 53.5% of the 368 open issues.

**Who waits on whom.** The Blockers section's *every open issue an open
blocker holds back* query answers what is blocked; this one adds the column
a steward escalates with — who holds the blocker, and how long it has been
sitting. Link direction and the type caveat are that section's: on the
blocked issue the row is `inward`, and `type` is the origin's link-type
name (`Blocks`, `Duplicate`, `Relates`, …) — `gadak ready` filters on the
stored `open_blockers` column, which resolves the blocking type through the
cached `link_types` catalog (a sync fetches it; the `'Blocks'` literal is
only the pre-sync fallback) instead of a live origin read. The target
join stays a left join, so a blocker outside the mirror is never known-done:

```sql
select i.key as waiting, i.priority_rank,
       l.target_key as blocker, t.assignee as held_by,
       round(julianday('now') - julianday(t.status_changed_at), 1)
         as blocker_age_days
from links l
join issues_full i on i.item_id = l.item_id
left join issues_full t on t.key = l.target_key
where l.type = 'Blocks' and l.direction = 'inward'
  and i.status_category != 'done'
  and (t.status_category is null or t.status_category != 'done')
order by i.priority_rank, i.key
```

Five pairs on the snapshot; NMA-26 has sat in progress unassigned for ~68
days while NMS-7 waits on it. An empty `blocker_age_days` means the target
records no status transition — no evidence, not fresh. This query's per-row
count is also a column: `open_blockers` on the blocked row, so "how many"
needs no join at all and `= 0` is exactly the `gadak ready` filter.

## Retro

**The three `gadak retro` rows with the most machinery, by hand.** `gadak
retro` prints the weekly table (sessions, resume, wip age p85, wip age max,
in progress, closed, cycle p50, cycle p85, mismatch) with a definition under
every row; these three are the ones worth re-deriving when a number looks
wrong. They key on status ids through `status_catalog`, on `status_category`,
or on frozen flow columns (`resolved_at`, `cycle_hours`) — never on the
display name beside them, which is zero rows on an account that names
statuses in another language. The mirror stores UTC timestamps, while
retro's week edges are local midnight, so a hand query states the bound in
UTC: for the ISO week starting Monday 2026-08-24 in a UTC+9 workspace, the
bound is `2026-08-23T15:00:00.000Z` (`gadak retro --json` prints the same
edges in the local zone as RFC 3339).

`closed` — issues that entered a done status during the week (the row they
came from must not already be done, so a reopen-and-close week counts once,
not twice):

```sql
select count(distinct c.item_id) as closed
from changelog c
join items it on it.id = c.item_id
join status_catalog done
  on done.source_id = it.source_id and done.status_id = c.to_id
 and done.category = 'done'
left join status_catalog prev
  on prev.source_id = it.source_id and prev.status_id = c.from_id
 and prev.category = 'done'
where c.field = 'status'
  and c.at >= '2026-08-23T15:00:00.000Z'
  and c.at <  '2026-08-30T15:00:00.000Z'
  and prev.status_id is null
```

`wip age p85` — the 85th percentile of how long the issues now in progress
have been in progress (the current-week column; a finished week needs the
changelog walk retro does — the last status row at or before week end,
mapped through `status_catalog`). Nearest-rank with integer arithmetic,
the same `(85n+99)/100` retro uses — float percentiles land a hair high on
round counts:

```sql
with ages as (
  select julianday('now') - julianday(status_changed_at) as days
  from issues_full
  where status_category = 'inprogress'
)
select round(days, 1) as wip_age_p85
from ages
order by days
limit 1 offset ((85 * (select count(*) from ages) + 99) / 100 - 1)
```

`cycle p85` — how long the week's closures took from first entry into
progress to the latest done entry (`cycle_hours`, frozen at close by
DERIVE.md), in days. The `reopen_count = 0` clause is the sample rule, the
same one the row's definition states: a reopened-and-refinished issue parks
its idle interval inside `cycle_hours`, and the sample this row wants is
normal-path work. The week is the ISO week starting Monday 2026-08-31 in a
UTC+9 workspace; the sample count rides along so a zero-sample week answers
`0`, not an empty result:

```sql
with cycles as (
  select cycle_hours / 24.0 as days
  from issues
  where resolved_at >= '2026-08-30T15:00:00.000Z'
    and resolved_at <  '2026-09-06T15:00:00.000Z'
    and cycle_hours is not null
    and reopen_count = 0
)
select round(days, 1) as cycle_p85_days, (select count(*) from cycles) as samples
from cycles
order by days
limit 1 offset ((85 * (select count(*) from cycles) + 99) / 100 - 1)
```

On the shipped snapshot the samples are all sub-minute synthetic spans, so
the percentile rounds to 0.0d here; the sample count moves whenever the
fixture is regenerated, and the sample rule is the part worth checking.

All three equal the `gadak retro --json` numbers on `examples/demo.db` —
the first two once `status_catalog` is seeded (the shipped fixture carries
none — a sync fills it), the cycle row without any seeding because it reads
the frozen columns directly; `retro_test.go` asserts the equality on the
seeded copy.

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

## Epics, components, and due dates

**Everything under one epic.** `epic_key` is derived at sync time — the
nearest `hierarchy_level = 1` ancestor reached through `parent_key` — and
recomputed on every sync, a column the origin does not expose as data at
all. `parent_key` stays the *immediate* parent (a sub-task points at its
story); `epic_key` is the epic either way:

```sql
select key, status_category, priority_rank, summary
from issues_full
where epic_key = 'NMB-194'
order by status_category, priority_rank, key
```

**Open work per epic** — the roll-up of that drill-down, biggest first:

```sql
select epic_key, count(*) from issues_full
where resolved_at is null and epic_key <> ''
group by epic_key order by 2 desc
```

**Open work per component.** `components` is a JSON array the way
`fix_versions` is — one row per issue per component:

```sql
select c.value as component, count(*) as open_issues
from issues_full i, json_each(i.components) c
where json_valid(i.components) and i.status_category != 'done'
group by c.value order by open_issues desc
```

**Overdue and not done.** `duedate` is the origin's date-only string, so
the string comparison against `date('now')` is exact — no `julianday`
needed:

```sql
select key, summary, duedate
from issues_full
where duedate is not null and duedate < date('now')
  and status_category != 'done'
order by duedate
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
