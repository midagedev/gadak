# 002 — History: what you looked at, and what you searched for

Status: draft (2026-08-14). Author: lead, from three requests in one session.

## The requests

1. *"최근 본 이슈 LNB를 따로 뷰로도 제공하고 싶어. 히스토리가 좋으니까. SNS에
   자기가 본 글 다시 찾기 위한 기능들이 다 제공되는 것처럼 히스토리가 중요해."*
2. *"혹시 몇 번 봤다든가 그런 이벤트 기록하고 있니. 이것도 중요해, 자가 개선을
   위해서."*
3. *"아예 에이전트를 통해 '내가 최근에 봤던 이슈 있잖아' 같은 명령도 가능해야
   해. 아니면 '검색했었던 거' 그런 느낌."*

They are one feature. (1) is the surface, (2) is the data, (3) is the payoff.

## Where we are

`me.recordRecent()` (`web/src/stores/me.svelte.ts:438`) writes
`{key, viewed_at, kind}` into localStorage and, on a repeat visit, **deletes the
old entry and unshifts a new one**. Cap is 30 (`RECENT_MAX`), scoped per
site+workspace since C5.

So today: last-seen time survives, visit count does not exist, the timeline is
one-deep, and none of it is reachable from SQL, the CLI, or an agent — it lives
in one browser's localStorage.

Request (2) is therefore not "add a counter"; it is **stop throwing the events
away**.

## The shape

**Append-only events in the mirror, aggregates derived.**

Storing one row per visit (rather than a counter) is what makes counts *and*
timelines *and* "yesterday afternoon" all fall out of the same table, and it
matches how `reopen_count` already works here: derive, don't store.

Two tables, both local-authored (never sent to Jira, never fetched from it), in
a database of gadak's own — see "Its own database file" below:

- `visits` — one row per view: kind (`issue` | `page`), key, `viewed_at`,
  and the profile's own identity is implicit (the mirror is per-profile).
- `searches` — one row per executed search: the query text, `searched_at`,
  result count, and — this is what makes request (3) work — the item opened
  from that search, when one was.

`searches` is what turns *"검색했었던 거"* from a vague memory into a row: the
query you typed is a better recall key than the issue key you forgot.

### Not localStorage: SQL, or the agent story dies

Request (3) decides this. In a database, history is reachable by `gadak sql`, by
`gadak_query` over MCP, and by every agent — and then
`gadak views open --keys -` / `gadak_show` puts the answer back on screen.
"SQL answers; show presents" already covers the agent story end to end, so
**this feature needs no new MCP tool**. In localStorage it would be invisible to
all of that, forever.

### Its own database file, not the mirror

`saved_views`, `favorites`, and `feed_reads` live in the mirror today
(`internal/store/schema.go:141,154,196`), so the mirror looks like the obvious
home. It is the wrong one.

"The mirror is a cache you can throw away" is true of Jira-derived rows: delete
the file and a sync rebuilds them. Nothing rebuilds what you looked at. Putting
history in `gadak.db` would make the project's most-repeated promise quietly
destructive for the first time, and would need preservation machinery — an
export/restore that every rebuild path has to remember to call — to undo the
damage of its own placement.

So: **a second SQLite file for data gadak authors itself.** Working name
`~/.gadak/local.db` (per profile, beside the mirror).

This is structural rather than procedural:

- The mirror goes back to being genuinely disposable. `rm gadak.db` is safe
  again, with no caveat and no restore step to forget.
- Its schema versions independently. The mirror is already at v20; personal
  data should not be riding a migration sequence driven by Jira field changes.
- **Snapshot and `export-static` cannot leak it.** They copy named tables out of
  the mirror; a separate file is out of reach by construction, not by a
  maintainer remembering to exclude it. For search-query history — which can
  contain anything the user typed — that is the difference between a policy and
  a guarantee.
- Backup is one small file.

It stays queryable in one breath: the store **ATTACHes `local.db` when it opens
the mirror**, so `local.visits` joins `issues` in a single SELECT. The read-only
guard is unaffected — it rejects non-SELECT statements, and a SELECT across an
already-attached schema is an ordinary SELECT (verified against
`internal/mcp/sqlguard.go:34`). Agents must never need to type `ATTACH`
themselves; if they do, the seam is in the wrong place.

`saved_views`, `favorites` and `feed_reads` belong in `local.db` by the same
argument and carry the same exposure today. Moving them is a data migration with
real risk, so it is **not** in the first cut: this spec establishes the file and
puts new data there. Their move gets its own round, and this spec is the reason
it exists.

### Retention

Append-only needs a ceiling. A visit row is tiny, but "forever" is not a policy.
Default: keep raw events for a bounded window (start at 180 days), prune on the
same pass that prunes tombstones. Counts older than the window can survive as a
rolled-up aggregate if that proves necessary — do not build the rollup until the
raw window is shown to be insufficient.

### Privacy

This is the most personal data gadak holds. It never leaves the machine: no
Jira write, no telemetry, no sync payload. Search queries especially — they can
contain anything the user typed. `SECURITY.md` gains a line, and the settings
dialog gets a way to clear history (SNS analogue: clearing your view history is
table stakes).

## The surface

A **History view**, first class, not a sidebar stub:

- Mixed issues and documents in one timeline, newest first, grouped by day
  (Today / Yesterday / This week / older) — the SNS reference the user named.
- Repeat visits are visible: a count and the last-seen time, so "I keep coming
  back to this one" is legible. That is the self-improvement signal in (2).
- Filterable by kind and searchable, using the same axes the list already has.
- The searches you ran are part of the timeline (or a sibling tab — pick the
  smaller diff): re-running one is a click.
- The existing sidebar recents stay as the quick jump; the view is the deep end.
  The sidebar gets an entry into it.

History should be expressible as the **keys axis** (`ks=`), so
`gadak views open --keys -` and the History view are the same mechanism — which
also means an agent can hand you a history-derived set with no new plumbing.

## Agent recipes (request 3)

These must work and be documented in `skills/gadak/SKILL.md` and
`docs/RECIPES.md`:

- "그때 봤던 이슈" → SQL over `visits` (last N days, kind=issue) →
  `gadak views open --keys -` / `gadak_show`.
- "검색했었던 거" → SQL over `searches` (fuzzy on query text) → the item opened
  from it, or re-run the query.
- "내가 자꾸 돌아가는 이슈" → `count(*) … group by key order by count desc`.

## Open questions (decide during design, record the answer)

- Does a visit require dwell time, or is opening the panel enough? (Start with
  opening; a 1–2s threshold may be needed to stop scroll-by noise from drowning
  the signal — measure before adding it.)
- Do desktop in-app browser tabs count as visits? (Probably yes — it is still
  "I looked at this".)
- Multi-profile: history is per-profile because the mirror is. Confirm that is
  what a user expects when the same person watches two sites.

## Out of scope

Team-wide view analytics (this is one person's local history, not a product
telemetry pipeline). Anything that transmits history off the machine.
