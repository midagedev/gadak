# Benchmarks: the live REST API vs the local mirror

Measured 2026-08-15 against a **live Atlassian Cloud site** — a real work
project with **2,853 issues** in the mirror — not a synthetic fixture. Client:
Apple M1 Pro, macOS 26.5, network in KST (your REST latency will vary with
distance to Atlassian's region; the local numbers will not). Method: 5 runs
per scenario (3 for the paged ones), medians reported; REST timed with
`urllib` over HTTPS with a Basic API token, gadak timed as a full process
invocation — **CLI startup is included in every gadak number**. Harness:
[`tools/bench-live.py`](../tools/bench-live.py) — point it at your own
profile and publish your numbers.

| Question | Jira REST API | `gadak` | |
| --- | ---: | ---: | ---: |
| Simple filter, 100 issues | 706 ms | 17 ms | 42× |
| One issue + full changelog | 1,055 ms | 54 ms | 20× |
| Free-text search | 520 ms | 78 ms | 7× |
| **Open issues per epic (GROUP BY)** | 3,924 ms — 7 API pages, aggregated client-side | 24 ms — one query | **162×** |
| **A count over the change history** (measured: issues that ever left Done) | not expressible — walking every changelog measured ≈ 414 ms/issue, ≈ 20 min for this corpus | 14.5 ms | — |

The first three rows are "faster". The last two are the point: past a page
size, JQL answers stop being slow and start being **unaskable** — the API can
hand you rows but not the aggregate, so every GROUP BY becomes a paging loop
in your code, and anything derived from the changelog — time spent per
status, what bounced back from Done, who touched what — becomes a crawl.

## Re-measured 2026-08-23

Same harness, same site, current binary and a grown corpus (7,166 issues,
up from 2,853). Re-run for the gadak.dev landing page; it also caught a
regression — the CLI `search`/`issue` verbs had been paying a whole-mirror
load per invocation (`lookup()` filtered `IssueLites()` in memory, ~240 ms
fixed). Fixed the same day (`IssueLitesByKeys`, GDK-747); the gadak column
below is post-fix. The `--jql` verb still carries a people-resolution
full-list cost (GDK-748) and is not shown.

| Question | Jira REST API | `gadak` | |
| --- | ---: | ---: | ---: |
| Simple filter, 100 issues | 374 ms | 17 ms | 23× |
| One issue + full changelog | 687 ms | 29 ms | 24× |
| Free-text search | 504 ms | 22 ms | 23× |
| A count over the change history (issues that ever left Done) | not expressible — ≈911 ms/issue measured, ≈109 min for this corpus | 14 ms | — |

The GROUP BY row is absent this run: the paged REST aggregation matched
zero rows on the current corpus (the project's epic shape changed since
August 15); the gadak side of that row measured 25 ms either way. The
honesty table below is from 2026-08-15; the incremental-sync row was
re-confirmed this run at 4.7 s.

## Re-measured 2026-08-26

Same harness and site, current binary, **3,296 issues** in the mirror. The
corpus is not the one above: the measured project was re-scoped between runs
(7,166 → 3,296), so **these numbers are not comparable to the 08-23 table**
row by row — read each run against itself. The machine was quiet for this
one; nothing else was running.

| Question | Jira REST API | `gadak` | |
| --- | ---: | ---: | ---: |
| Simple filter, 100 issues | 583 ms | 19 ms | 31× |
| One issue + full changelog | 710 ms | 28 ms | 25× |
| Free-text search | 543 ms | 41 ms | 13× |
| **Open issues per epic (GROUP BY)** | 4,761 ms — 8 API pages, 781 rows aggregated client-side | 22 ms — one query | **214×** |
| A count over the change history (issues that ever left Done) | not expressible — ≈503 ms/issue measured, ≈28 min for this corpus | 14 ms | — |

The GROUP BY row is back — it was absent on 08-23 only because the REST
aggregation matched zero rows on that corpus, not because the shape changed.

The `--jql` verb measured 72 ms against the same filter's 19 ms of `gadak
sql`. The people-resolution cost that row used to carry is gone (the narrow
projection landed); bisecting with `search --jql --emit`, which returns right
after people resolution, puts parse + resolve at 20 ms and the remaining
~45 ms in the matching path, which loads every issue and filters in Go
rather than pushing WHERE/ORDER BY/LIMIT into SQL. That cost is linear in
mirror size (≈10 ms at 978 issues, ≈45 ms at 3,323) and `--limit` does not
reduce it. Tracked as GDK-748.

If you re-run the harness, expect maxima 10–20× the medians in the first
sample of each row. That is a cold **access path**, not process startup: the
same query twelve times runs 320, then 20 ms flat, and a *different* query
then pays 240 ms of its own before settling. Each column set is paged in
once per boot from the 145 MB mirror. Medians are the honest figure; the
first sample is measuring the page cache.

## Where gadak loses

Honesty rows, measured 2026-08-15 (same evening re-run, corpus now 2,865
issues; the table above was measured at 2,853). Wall clock via the same
harness's `--source jira` / `--source confluence` split and one `--full` run
per profile:

| Cost | Measured |
| --- | ---: |
| First full sync, 534 issues + 71 pages | 26.4 s (re-fetch of identical data) |
| First full sync, 2,865 issues + 438 pages | 7.2 min (re-fetch of identical data) |
| Incremental sync (each watch tick, quiet site) | 6.7 s — jira 2.2 s (16 issues fetched, 0 changed), confluence 2.2 s (1 page, 0 changed); the combined tick runs longer than its two halves |
| CLI process startup, every invocation | 15 ms |
| Freshness | the mirror trails Jira by up to one sync interval |

Two shapes to read out of that: the tick's cost tracks what the watermark
window *matches*, not what *changed* — the demo site, whose Confluence
watermark is ten days old, spends 19.4 s of a 21.4 s tick re-reading 71
unchanged pages — and a tick costs real seconds even when nothing changed,
so the honest unit for a watch loop is "one tick ≈ 7 s of work every
interval", not "a single call that returns nothing".

If you need this minute's Jira state, ask Jira. gadak trades a sync interval
of staleness for reads that cost nothing.

Re-measured 2026-08-26 (3,296 issues): a first full sync of 3,323 issues +
457 pages took **10.6 min**, and CLI startup re-confirmed at 15 ms. The
incremental rows are *not* restated, because that run cannot carry them:
the harness takes n=2 for sync, and the two samples spread 2.6–8.3 s (jira)
and 2.0–15.7 s (confluence). At that spread a median means nothing, and the
combined `--source all` tick came out **faster** (4.3 s) than either half —
which contradicts the 08-15 row above rather than confirming it. Two samples
are not enough to overturn a measured claim, so the 08-15 numbers stand and
this is recorded as an open question about the harness, not a new result.
An incremental-sync row worth publishing needs more runs than the harness
currently takes.

## Caveats

- REST medians include TLS + network from one location (KST). Closer regions
  will see lower REST numbers; the ratios shrink, the unaskable rows do not.
- One background process was active on the client during measurement; gadak
  numbers include process start and are conservative.
- The REST GROUP BY row uses the documented paging loop
  (`/rest/api/3/search/jql`, `maxResults=100`, `nextPageToken`), fetching only
  the fields needed for the aggregation. 664 unresolved rows → 7 calls.
