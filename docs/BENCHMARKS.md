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

## Caveats

- REST medians include TLS + network from one location (KST). Closer regions
  will see lower REST numbers; the ratios shrink, the unaskable rows do not.
- One background process was active on the client during measurement; gadak
  numbers include process start and are conservative.
- The REST GROUP BY row uses the documented paging loop
  (`/rest/api/3/search/jql`, `maxResults=100`, `nextPageToken`), fetching only
  the fields needed for the aggregation. 664 unresolved rows → 7 calls.
