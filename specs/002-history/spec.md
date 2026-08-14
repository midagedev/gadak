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

Two tables, both local-authored (never sent to Jira, never fetched from it):

- `visits` — one row per view: kind (`issue` | `page`), key, `viewed_at`,
  and the profile's own identity is implicit (the mirror is per-profile).
- `searches` — one row per executed search: the query text, `searched_at`,
  result count, and — this is what makes request (3) work — the item opened
  from that search, when one was.

`searches` is what turns *"검색했었던 거"* from a vague memory into a row: the
query you typed is a better recall key than the issue key you forgot.

### Why the mirror and not localStorage

Precedent exists: `saved_views`, `favorites`, and `feed_reads` are already
local-authored tables in the mirror (`internal/store/schema.go:141,154,196`).
History joins them.

The decisive reason is request (3). In the mirror, history is reachable by
`gadak sql`, by `gadak_query` over MCP, and by every agent — and then
`gadak views open --keys -` / `gadak_show` puts the answer back on screen.
"SQL answers; show presents" already covers the agent story end to end, so
**this feature needs no new MCP tool**. In localStorage it would be invisible
to all of that, forever.

### The contract this strains, and how it is settled

"The mirror is a cache you can throw away" is true of Jira-derived rows: delete
the file and a sync rebuilds them. It is **not** true of history — nothing can
reconstruct what you looked at. `favorites` and `saved_views` already carry this
exposure with no preservation path (verified: no export/restore path exists for
them today).

So this spec owns a gap that predates it:

> Local-authored tables (`saved_views`, `favorites`, `feed_reads`, `visits`,
> `searches`) must survive a mirror rebuild.

Either the rebuild path preserves them, or there is an export/import that the
rebuild path uses. Deciding which is part of the work; shipping history without
it is not acceptable, because it would make "throw the mirror away" quietly
destructive for the first time.

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
