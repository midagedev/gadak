---
title: "gadak 0.18: the Jira mirror that no longer needs Jira"
date: 2026-08-28
description: "An annotated release: a terminal next to the tracker, a phone that pairs to your desk, and the measurement behind the one-file mirror — including the rows where it loses."
lang: en
---

gadak mirrors Jira and Confluence into one local SQLite file, so "which epic
is actually stuck?" is a `GROUP BY`, not seven pages of REST. You can check
that claim before installing anything: [this Datasette Lite
URL](https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc)
loads the demo snapshot — a plain SQLite file served from this repository —
and runs the epic query client-side in your browser. No install, no account,
no server of ours.

And since 0.16, gadak also runs with no Jira at all. `gadak init
--standalone` starts a deliberately minimal in-process tracker (`issuetap`)
as the origin; the app, the CLI, `gadak sql`, and the MCP server work
unchanged on top of it. The design rule survives: the origin owns the data,
the mirror is a cache you can throw away, and the one file you back up is
the origin's persist file.

This is the annotated announcement for 0.18 — what shipped, and the
measurement that explains why the whole thing is shaped like a file. The
numbers section reports where the mirror loses, too; that part is not
optional reading.

## What shipped in 0.18

**A terminal, inside gadak.** ⌘K → Terminal, or `` Ctrl+` ``. It is a real
shell in the same window as your issues — so you can run a coding agent
there and watch it move the board next to it. The same terminal shows up in
the web tab, in the macOS app, and on a paired phone. Korean composition
lands on the cursor, where a terminal canvas does not make that automatic.
It ships **Beta** — useful, and we would rather name the rough edges than
hide them.

**A token that opens a shell and nothing else.** Pairing tokens are scoped:
`gadak pairing mint --scope terminal` is the only kind that opens a shell; a
`serve` or `origin` token opens none. Revoke a terminal token and the shells
it opened close within seconds, and are told why. Loopback still needs no
token at all.

**The phone app is something you can use.** Issues in your own saved views,
wiki pages beside them, search that shows page hits, and notifications only
for what is actually yours — assigned, mentioned, reopened. It reaches the
terminal too, over a second token, and unpairing forgets that shell along
with everything else.

**And the patch, the same day.** 0.18.1 taught the terminal to clean up
after itself: closing a session now closes every process it started — the
forgotten `sleep 999 &`, the agent you walked away from. The close walks the
session's terminal once, while the walk can still be trusted; a measured
Linux failure where a second walk found *another* session's shell is why it
is once.

The changelog you would normally read for the rest is [on the
site](/changelog/) now, rendered from the repository rather than
copied. What follows is the part a changelog cannot carry: the measurement.

## JQL has no GROUP BY

Which epic is stuck? That is the question a standup turns into a search.
Not a board. A count: open issues per epic, largest first.

Jira will not give you that count. JQL is a filter language. It selects
issues; it does not join, it does not aggregate, and it does not treat the
changelog as a table. The REST search endpoint matches that shape: you ask
for issues, you get a page — `maxResults=100`, then a `nextPageToken`. If
the answer you wanted was ten numbers, you still paid for every issue those
numbers were computed from, and you computed them after the download.

The changelog is a second, worse version of the same limit. History is not
searchable as data; it arrives as an expansion on a single issue. Anything
you want from it — time in status, what bounced back from Done, who touched
what — means walking issues one at a time. Jira does not even have a field
for how many times a ticket came back. A reopen count is something you
derive, or you do not have.

**But the dashboard does that.** It does, for that one question. Put a saved
filter behind a two-dimensional statistics gadget and Jira will count its
issues by epic for you. The gadget is a fixed menu over a saved filter: it
counts one field by another, it does not join, it does not compose, and it
does not hand the result to anything else. Change the question and you are
building a new filter and a new gadget — and history stays out of reach
either way, because "ever left Done" is not a field to count; it is a fold
over the changelog. Atlassian does sell the real version of this: their
analytics product puts Jira data in a warehouse and lets you write actual
SQL. It is an upper-tier Cloud product and a hosted warehouse — the right
answer for a company that has it, and the wrong answer for a file on my
laptop or the agent in my terminal.

### What I measured

Measured 2026-08-15 against a live Atlassian Cloud site — a real work
project, 2,853 issues in the mirror, not a synthetic fixture. Client: Apple
M1 Pro, macOS, network in KST. REST timed with `urllib` over HTTPS and a
Basic API token; gadak timed as a full process invocation, so every local
number includes CLI startup. Five runs per scenario, three for the paged
ones; medians below.

| Question | Jira REST API | gadak | |
| --- | ---: | ---: | ---: |
| Simple filter, 100 issues | 706 ms | 17 ms | 42× |
| One issue + full changelog | 1,055 ms | 54 ms | 20× |
| Free-text search | 520 ms | 78 ms | 7× |
| **Open issues per epic (GROUP BY)** | 3,924 ms — 7 API pages, aggregated client-side | 24 ms — one query | **162×** |
| **A count over the change history** (issues that ever left Done) | not expressible — walking every changelog measured ≈ 414 ms/issue, ≈ 20 min for this corpus | 14.5 ms | — |

The harness is
[`tools/bench-live.py`](https://github.com/midagedev/gadak/blob/main/tools/bench-live.py),
and [BENCHMARKS.md](https://github.com/midagedev/gadak/blob/main/docs/BENCHMARKS.md)
carries two later re-runs on changed corpora — one of which caught a gadak
CLI regression that the same table then fixed. Point the harness at your own
profile and publish your numbers.

The first three rows are "faster." I will not pretend they are the reason to
keep a mirror — a closer region than KST shrinks those ratios. The last two
rows do not shrink, because they are not a latency contest. They are
questions the API cannot ask.

The GROUP BY row is the standup question. The documented paging loop
(`/rest/api/3/search/jql`) returned 664 unresolved issues across 7 calls;
median wall clock to page them and count by epic in the client: 3,924 ms.
The same count against the mirror is one SQL statement, 24 ms, process start
included. More honestly than "162×": one side is a download-and-fold, the
other is a query.

The last row has no REST median to sit next to, because there is nothing to
time but a crawl. I did not sit through a 20-minute run: I timed a sample of
five issue+changelog fetches (about 414 ms each) and multiplied by the 2,853
issues in the corpus — arithmetic, not a measured full crawl. It is also the
only way the API offers. There is no "give me every status transition in
this project" endpoint to point a faster client at. You fetch issues. You
expand history. You walk.

### Where it loses

Same corpus, same machine.

| Cost | Measured |
| --- | ---: |
| First full sync (one-time, ~N/100 API calls) | minutes, size-dependent |
| Incremental sync (each watch tick) | 6.6 s |
| CLI process startup, every invocation | 15 ms |
| Freshness | the mirror trails Jira by up to one sync interval |

The mirror is as fresh as the last successful tick. If you need this
minute's Jira state, ask Jira. The trade is a sync interval of staleness for
reads that cost nothing. That is the whole bargain, stated plainly.

## What this does to an agent

This is half of why the file exists, and it is not the frame for the numbers
above — those stand if no model is involved.

An agent over Jira's REST API does what the GROUP BY row does: it pages.
Each page is tokens. A question that needs the set — open per epic, ever
left Done, who reassigned what — turns into a loop that fills the context
window with rows the model then has to add up. The model spends its working
memory on a job the database would have finished in one statement.

An agent that can see the mirror writes the statement instead. The same pipe
a human uses is the handoff back to the window:

```bash
gadak sql --no-header "select key from issues_full where status_category = 'inprogress'
                       order by status_changed_at asc limit 5" | gadak views open --keys -
```

SQL answers; the running app presents that set, in that order, on your
screen. The agent does not paste a markdown table of tickets it paged from
REST, and it does not keep those rows in context after the query returns.
0.18's terminal closes the loop in the other direction: the agent works in a
pane next to the board it is moving.

I will not claim a token count I did not measure. The shape is enough: one
call against a file, or a crawl against an API that only returns rows.

## Try it

In order of commitment:

1. **No install:** [run the epic query in your
   browser](https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc)
   — Datasette Lite over the raw demo snapshot. The file it loads is the
   product's actual format.
2. **The window:** the [live demo](/demo/) — the
   same UI, 534 issues, in the browser, no account.
3. **The real thing:**

   ```bash
   brew install midagedev/tap/gadak        # the macOS app; the bundled CLI lands on PATH
   brew install midagedev/tap/gadak-cli    # CLI only — the Linux path too
   ```

   No Jira? `gadak init --standalone` and the tracker is yours, one file on
   your machine.

If gadak stopped shipping tomorrow, you would delete a directory and lose
nothing: your Jira was the source of truth the whole time, and a standalone
workspace's record is one SQLite file you already know how to read. That
property is the product.
