# Derived fields

`gadak` mirrors Jira. A handful of columns do not come from the source at all —
gadak computes them during sync (reopens, resolution dates, priority rank, epic
keys, cross-references). This file is the single place that explains them: what
each column means, why the rule looks like that, and the queries an agent
actually runs. The schema — which table carries which column — stays in
[`specs/000-product/data-model.md`](../specs/000-product/data-model.md); the
rule table and the example queries moved out of it live here ([GDK-88]/[GDK-89]).

The rules are documentation, not a 0.x promise. While the version is 0.x the
promises are still only the three `data-model.md` lists at its top:
`issues_full` and the `docs/RECIPES.md` queries, `gadak sql` stdout, and
`views open --keys -`. What *is* locked here is execution: every runnable
`sql` fence below is run verbatim by a test (see [Queries](#queries)), so the
file cannot rot the way a second, hand-copied list in Go would.

## One rule that outranks every query below

Status, priority, and issue-type **names localize per account**. Jira
translates `status.name` per site and ignores `Accept-Language`, so keying on
an English or Korean default name is a silent no-op everywhere else. The
internal tool this code was extracted from broke exactly there
(`specs/000-product/contracts/sync.md`, "Localization hazard"): it matched
"Reopened" / "다시 열림" and worked only on the sites that spelled it that way.
Every derived rule below therefore keys on `status_category`
(`new` | `inprogress` | `done`) or on ids, and so should your queries:

```sql ignore
-- WRONG: display names localize per account — this is 0 rows on a Korean site
WHERE status = 'In Progress'
-- RIGHT: stable on every site
WHERE status_category = 'inprogress'   -- or: status_id = '<stable id>'
```

`priority` and `issue_type` are display names the same way: filter on
`priority_rank` (or `priority_id`) and `issue_type_id`, never on the name.

The second shape to refuse is querying `issues.raw`. It is the full source
payload, kept as an escape hatch: it is shaped by Jira's API (field ids like
`customfield_10200` that differ per site), not by gadak's contract, so anything
built on it breaks silently on the next site or API revision.

```sql ignore
-- WRONG: raw is an escape hatch shaped by Jira's per-site field ids
SELECT key, json_extract(raw, '$.fields.customfield_10200') FROM issues;
-- RIGHT: mapped custom fields live in issues.custom, keyed by the configured alias
SELECT key, json_extract(custom, '$.severity') FROM issues;
```

## Derived field rules

Each rule is computed during sync from the changelog, and every rule that would
otherwise depend on site-specific naming keys on `statusCategory` instead.

| Field | Rule |
| --- | --- |
| `status_changed_at` | Timestamp of the newest changelog entry whose field is `status` |
| `resolved_at` | Timestamp of the newest transition whose target category is `done`; NULL if the current category is not `done` |
| `reopen_count` | Count of changelog transitions from a `done`-category status to a non-`done` one |
| `reopened_at` | Timestamp of the newest such transition |
| `assignee_changed_at` | Timestamp of the newest changelog entry whose field is `assignee` |
| `priority_rank` | Position in the site's priority list, 1-based; 0 when unset or unknown |
| `items.body_text` | ADF flattened to plain text, plus any custom fields configured as body text. It lives on the spine, not on `issues`, because it is what FTS indexes |
| `comment_count` | Number of rows in `comments` for the issue |
| `reopen_reason` | Body text of the earliest comment created at or after `reopened_at`, capped at 1000 bytes on a rune boundary. A heuristic: it surfaces the explanation on teams where whoever reopens says why in a comment. Empty when never reopened or no comment followed |
| `cloned_from` | Target of an outward link whose type name contains "clone" (Jira's default "Cloners" type) — the clone is the issue that displays "clones \<origin\>", so the origin sits behind the outward direction. Caveat: link type names are site configuration created in the site's language — a non-English clone type derives nothing, and there is no language-stable id to key on. The read API also exposes `source_project`, the key's project prefix |
| `epic_key` | Key of the nearest `hierarchy_level = 1` ancestor reachable via `parent_key` within two hops (the parent, else the grandparent); NULL when there is none |
| `started_at` | Timestamp of the oldest changelog transition whose target category is `inprogress`; NULL when the issue never entered progress |
| `cycle_hours` | `resolved_at` minus `started_at` in hours, stored only while the issue is `done` now and the span is positive; NULL otherwise |
| `last_activity_at` | Newest of the item's `updated_at`, the newest changelog entry and the newest comment — a string max, because ISO-8601 UTC stamps compare lexicographically; NULL when all three are absent |
| `open_blockers` | Count of inward links of a blocking type whose target issue is in the mirror and not `done`. Which types block resolves through the cached `link_types` catalog (`lower(name) = 'blocks'` or `lower(outward) LIKE 'block%'`), never a hardcoded display name; a target outside the mirror is not blocking — unknown must not hold work back. Recomputed on every batch for the batch's keys plus every issue holding an inward link at one of them, and swept whole-table after a full sync |

A status id the site's status list does not cover counts as **not** `done`.
That direction is deliberate: it can only miss a reopen, never invent one.

## Why the rules look like that

**The changelog carries ids, not names.** A changelog entry records
`from_id`/`to_id`; the id → category mapping comes from the site's status list,
which the connector supplies per batch. That is why every transition rule is
stated in categories: it is the only site-stable vocabulary the sync ever sees.

**Unknown ids count as not `done`.** When the category map has no entry for a
status id, no transition into it is a resolution and no transition out of it is
a reopen. The other direction — treating unknown as `done` — would invent
reopens whenever a site renames or adds statuses. A missed reopen is recoverable
(fix the status list, re-sync); an invented one is a lie in the metrics.

**`resolved_at` is nulled when the issue is not done now.** The pass keeps the
newest transition into `done`, then discards it unless the issue currently sits
in a `done` category: a resolution that was undone is not a resolution date. An
issue resolved and reopened three times shows `resolved_at` only after its final
close.

**`reopen_reason` is a heuristic and says so.** It is the body of the *earliest*
comment created at or after `reopened_at` — on teams where whoever reopens
explains why in a comment, that comment is the reason. Timestamps are ISO-8601
UTC and sort lexicographically, so "earliest" is a string comparison. The cap
is 1000 bytes, and truncation backs up to a UTF-8 rune boundary rather than
splitting a sequence mid-rune — without that, Korean text would end in a broken
final character.

**`cloned_from` matches a link-type *name*.** Jira's default clone link type is
"Cloners", and the match is `contains "clone"`, case-insensitive, on outward
links — the clone is the issue that displays "clones <origin>", so the origin
sits behind the outward direction (GDK-1214; the inward side marks the origin
and derives nothing). Link type names are site configuration created in the
site's language, so a site whose clone type carries a non-English name derives
`''` here — there is no language-stable id to key on. `cloned_from != ''` is
the is-a-clone test.

**`priority_rank` is 1-based and 0 means unknown.** The site's priority list
arrives most-urgent-first; rank 1 is the most urgent. 0 covers both "no
priority set" and "priority not in the site's list" — when you sort by it,
handle 0 explicitly (`priority_rank = 0` is not "most urgent", it is "no
signal").

**`epic_key` walks `parent_key` at most two hops** — the parent, else the
grandparent — and takes the nearest ancestor with `hierarchy_level = 1` (Jira:
Epic). Deeper chains stay NULL; standard Jira hierarchies (epic → story →
sub-task) never need a third hop. It is recomputed for the whole table after
every upsert batch, on purpose: parent chains can resolve only after later rows
arrive, so both reverse-arriving batches and children of unchanged parents need
a second look.

**The flow columns are stamps and counts, not live durations.**
`started_at`, `cycle_hours`, `last_activity_at` and `open_blockers` exist so the
questions THEORY.md calls aging, cycle time, silence and readiness are filters
(`WHERE started_at IS NOT NULL`, `ORDER BY last_activity_at`) instead of
changelog walks. They are values the last sync observed — none of them ticks.
`last_activity_at` is a stamp, and "how long silent" is the reader's query
against now; `cycle_hours` is frozen at close because the issue is done;
`open_blockers` recomputes when links or target statuses move, which is a sync
event, not a clock. See also *time-in-status is not a column* below.

**`item_refs` rows outlive their targets.** `item_refs` (a table, not a column)
holds cross-references extracted from text at upsert time: page bodies → issue
keys, issue bodies and comments → wiki page ids. Four patterns are recognized —
`/browse/KEY` links (via `url`), bare issue keys in plain text (via `text`,
accepted only when the project prefix exists in the mirror), and the two
Confluence page-URL shapes (via `url`). The same target found by both a URL and
bare text keeps one row, `via = 'url'`. Self-references are dropped. A row is
written even when the target is not in the mirror — **readers join `items` on
`key` + `kind` and surface live rows only**; `via` tells you how strong the
signal is (`url` is an actual link, `text` is a bare mention). Refs are
recomputed (delete + insert) in the same transaction as every item upsert, so a
ref never outlives the edit that removed the mention.

## time-in-status is not a column

Deliberately absent. The internal system this was extracted from carried a
`working_hours_in_status` column that no code ever populated, and the UI's
"stale" view read it as always zero. Staleness is computed from
`status_changed_at` instead, with the threshold in configuration.

There is no `time_in_status` column to query, and adding one would repeat the
mistake. Two shapes cover what people actually ask:

- How long has the **current** status been open? — `status_changed_at` against
  now (the "sitting" query below).
- How long was spent **per** status, over the issue's life? — walk the
  changelog: each `field = 'status'` entry runs until the next one (or now).

The flow columns (v43) do not change that verdict. `started_at` is a stamp, not
an elapsed-time column; `cycle_hours` is a span frozen at close, not a running
clock; `last_activity_at` is a stamp the reader ages against now. None of them
is `time_in_status`, and none accrues between syncs.

The flow canon agrees rather than merely permitting it: the Kanban Guide's
cycle time is *elapsed* time between start and finish, one running clock, and
per-status durations are an analysis performed on the changelog, not a stored
fact.

**Which cycle time `cycle_hours` is.** Three definitions are in circulation and
gadak measures the first. gadak: elapsed wall clock from the first entry into
an in-progress category to the resolution — the Kanban Guide's definition, and
the only one derivable without deciding which statuses count as active. Jira's
control chart: the *sum* of time in the statuses you select, so a reopened
issue's second pass is added to the first. Swarmia: time in In Progress only,
so a three-month postponement between two weeks of work reports two weeks. A
reader comparing gadak's number to either of those is looking at a definitional
gap, not a defect.

## Queries

Every runnable `sql` fence below is executed verbatim by
`TestDocumentedExampleQueries` in `internal/store`: the test parses this file,
runs each fence against a throwaway copy of `examples/demo.db`, and fails the
build when a query stops parsing or returns fewer rows than its `min=N` marker
demands (`min` defaults to 1). One query per fence. Fences marked `sql ignore`
are never executed — they document shapes that must not run. Editing a query
here means editing the one copy; there is no second list in Go.

`issues_full` is the agent convenience view — every `issues` column plus
`summary` from `items.title`.

```sql min=1
-- Everything that was ever reopened, worst first
SELECT key, summary, reopen_count, reopened_at
FROM issues_full
WHERE reopen_count > 0
ORDER BY reopen_count DESC, reopened_at DESC;
```

Narrow it to a window by adding `AND reopened_at > datetime('now', '-1 month')`.
That clause is deliberately not in the fence above: the fixture this file is
tested against is a frozen snapshot, so a relative window would make the gate
go red on a date rather than on a change — a test that fails because a month
passed teaches everyone to ignore it.

```sql min=1
-- Reopens per epic: which epics' work keeps coming back
SELECT epic_key, COUNT(*) AS reopened_issues, TOTAL(reopen_count) AS reopen_events
FROM issues
WHERE epic_key IS NOT NULL AND reopen_count > 0
GROUP BY epic_key
ORDER BY reopen_events DESC;
```

```sql min=1
-- Why it came back: the earliest comment after the last reopen
SELECT key, reopen_count, reopened_at, reopen_reason
FROM issues_full
WHERE reopen_reason != ''
ORDER BY reopen_count DESC, reopened_at DESC
LIMIT 20;
```

```sql min=1
-- Resolved before, open again now — resolutions that did not hold
SELECT key, status, reopen_count, reopened_at
FROM issues_full
WHERE reopen_count > 0 AND status_category != 'done'
ORDER BY reopened_at DESC
LIMIT 20;
```

```sql min=2
-- Open work per assignee in one project
SELECT COALESCE(assignee, '(unassigned)') AS who, COUNT(*) AS open
FROM issues
WHERE project_key = 'NMB' AND status_category != 'done'
GROUP BY who ORDER BY open DESC;
```

```sql min=1
-- How long has each in-progress issue been sitting?
SELECT key, status, ROUND(julianday('now') - julianday(status_changed_at), 1) AS days
FROM issues
WHERE status_category = 'inprogress'
ORDER BY days DESC LIMIT 20;
```

The four fences below are the flow layer (v43): aging, cycle time, silence and
readiness as column reads. Each one replaces a changelog walk of the same
question — `started_at` for aging, `cycle_hours` for cycle time,
`last_activity_at` for silence, `open_blockers` for readiness.

```sql min=1
-- Aging: how long since work started, for in-progress issues
SELECT key, status, ROUND(julianday('now') - julianday(started_at), 1) AS days_started
FROM issues
WHERE status_category = 'inprogress' AND started_at IS NOT NULL
ORDER BY days_started DESC LIMIT 20;
```

`started_at` is the first transition into an in-progress category, so this is
"how long has this been underway" — a different question than the sitting query
above, which is "how long since the last status change".

```sql min=1
-- Cycle time p85, nearest-rank, over issues resolved in the last 90 days
WITH done AS (
  SELECT cycle_hours FROM issues
  WHERE status_category = 'done' AND cycle_hours IS NOT NULL
    AND resolved_at >= date('now', '-90 day')
)
SELECT ROUND(cycle_hours, 1) AS p85_cycle_hours, (SELECT COUNT(*) FROM done) AS samples
FROM done ORDER BY cycle_hours
LIMIT 1 OFFSET (SELECT (85 * COUNT(*) + 99) / 100 - 1 FROM done);
```

The rank arithmetic `(85·n+99)/100` is nearest-rank in integers — the same
idiom the Retro section of RECIPES.md uses and `internal/retro` computes, kept
in lockstep so a SQL answer and a Go answer cannot drift apart on rounding.
`cycle_hours` is stored only for issues done now with a positive span, so the
pool is exactly the population the old changelog walk built.

```sql min=1
-- Neglected: open issues silent for 7 days or more
SELECT key, status, ROUND(julianday('now') - julianday(last_activity_at), 1) AS days_silent
FROM issues
WHERE status_category != 'done' AND last_activity_at IS NOT NULL
  AND julianday('now') - julianday(last_activity_at) >= 7
ORDER BY days_silent DESC LIMIT 20;
```

The threshold here is 7 days. Pick your own by changing the `7` — the column
is a stamp, so the definition of neglected stays a query-time decision.
`last_activity_at` is the newest of the item's updated stamp, the newest
changelog entry and the newest comment, so a priority edit or a status move
counts as activity even when no comment follows.

```sql min=1
-- Ready: open issues no open blocker holds back
SELECT key, status, priority_rank, open_blockers
FROM issues
WHERE status_category != 'done' AND open_blockers = 0
ORDER BY priority_rank, key LIMIT 20;
```

`open_blockers` counts inward links of a blocking type whose target is in the
mirror and not done. This is the filter behind `gadak ready` — same column,
same rule, one owner (`internal/store/flow.go`).

```sql min=1
-- Time in status per status, from the changelog — no stored column exists.
-- Groups by to_id (stable); MAX(to_value) only picks a display label.
-- Intervals start at each issue's first recorded status transition; the last
-- one stays open and accrues to now.
WITH moves AS (
  SELECT item_id, at, to_id, to_value,
         LEAD(at) OVER (PARTITION BY item_id ORDER BY at) AS next_at
  FROM changelog
  WHERE field = 'status'
)
SELECT i.key, m.to_id AS status_id, MAX(m.to_value) AS status_name,
       ROUND(TOTAL(COALESCE(julianday(m.next_at), julianday('now')) - julianday(m.at)), 1) AS days
FROM moves m
JOIN issues i ON i.item_id = m.item_id
GROUP BY i.key, m.to_id
ORDER BY days DESC
LIMIT 20;
```

```sql min=1
-- Full-text across bodies and comments
SELECT i.key, it.title
FROM items_fts f
JOIN items it ON it.rowid = f.rowid
JOIN issues i ON i.item_id = it.id
WHERE items_fts MATCH 'idempotency AND retry'
LIMIT 20;
```

The join in these queries is spelled out rather than `USING (item_id)` because
the spine's primary key is `items.id`, and `key` exists on both tables.

## Where the rules live in code

| Rule | Code |
| --- | --- |
| every changelog-derived field above | `Derive` in `internal/store/derive.go` |
| `epic_key` | `recomputeEpicKeys` in `internal/store/write.go` |
| `open_blockers` | `recomputeOpenBlockers` in `internal/store/flow.go` |
| `link_types` catalog cache | `cacheLinkTypeCatalog` in `internal/store/flow.go` |
| `item_refs` extraction and rewrite | `internal/store/refs.go` |
| view wiring (`issues_full`, migrations) | `internal/store/schema.go` |

[GDK-88]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-88
[GDK-89]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-89
