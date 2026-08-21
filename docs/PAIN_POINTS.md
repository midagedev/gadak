# The pain points, with receipts

gadak's pitch rests on claims about what hurts in Jira. This page is the
evidence: recurring, sourced complaints from real users, and what gadak does
about each. Collected 2026-08; quality note — HN, Atlassian Community, and
issue trackers below are first-hand user reports; vendor comparison blogs were
excluded.

## What gadak addresses

**1. Opening a ticket is slow.** The single most repeated complaint, across HN
threads spanning 2018–2024. Root cause is architectural: one ticket render fans
out into many sequential requests. A local mirror answers with zero network
round trips — this is the core of the product.
[HN 36381111](https://news.ycombinator.com/item?id=36381111),
[HN 25594451](https://news.ycombinator.com/item?id=25594451),
[HN 39376054](https://news.ycombinator.com/item?id=39376054),
[radial.build analysis](https://www.radial.build/blog/why-is-jira-so-slow)

**2. Search cannot find text you know is there.** Long-running threads on
Atlassian's own forum about tokenization and ranking. The demand is proven by
tools like [`jirafts`](https://pypi.org/project/jirafts) that download issues
just to index them locally — the same move gadak makes.
[Atlassian Community qaq-p/91620](https://community.atlassian.com/forums/Jira-questions/Is-there-a-plan-to-make-JIRA-search-better/qaq-p/91620)

**3. Notification floods get ignored wholesale.** ~60 Jira emails a day per
developer, 3–4 relevant, per Atlassian's own community articles. The fix is
relevance, not volume — the local watch feed computes "changes that concern
me" (assignee, reporter, mention, watched) as a query over the mirror.
[ba-p/3110559](https://community.atlassian.com/forums/Jira-Cloud-Admins-articles/The-Jira-Cloud-Struggle-Nobody-Talks-About-Too-Many/ba-p/3110559)

**4. JQL cannot express real questions.** Comment history, version ranges, and
body+author+date combinations are documented gaps that paid marketplace apps
fill. SQL over a mirror makes them ordinary `WHERE`/`JOIN` clauses; derived
fields (reopen history) answer questions JQL cannot ask at all.
[td-p/730101](https://community.atlassian.com/forums/New-to-Jira-discussions/JQL-is-a-nightmare/td-p/730101)

**5. Reading Jira programmatically is fragile.** The 2025 point-based rate
limits share one invisible site-wide pool across all apps, and the search API
was replaced under running clients. A mirror concentrates API exposure into one
sync path — the client honors `Retry-After` with exponential backoff, retries
only 429/503 (never non-idempotent writes), and everything downstream reads
SQLite.
[ba-p/3199366](https://community.atlassian.com/forums/App-Central-articles/Heads-up-Jira-s-new-API-rate-limits-kick-in-tomorrow-Here-s-how/ba-p/3199366),
[qaq-p/3101716](https://community.atlassian.com/forums/Jira-questions/REST-The-new-rest-api-3-search-jql-endpoint-is-a-complete/qaq-p/3101716)

**6. No offline.** Requested since [JRA-7490](https://jira.atlassian.com/browse/JRA-7490)
(two decades open). The mirror satisfies the read half: everything you have
synced — including cached attachments — renders with no network at all.

## What gadak deliberately does not address

- Boards, sprint planning UI, and drag-and-drop accidents — board UI territory;
  use Jira. Sprint fields (`sprint_id` / `sprint_state`) are in the mirror.
- Notification *scheme* design, permission schemes, workflow admin — server-side
  administration.
- Velocity dashboards — the complaint behind them ("optimizing for Jira") is
  cultural, and a dashboard feeds it. gadak stays a triage tool on purpose.
- Jira Server / Data Center — untested, therefore unclaimed.

## Competitive note

`jira-cli` and the TUI field (`jirust`, `jiratui`, …) all call the live API per
command. Their draw is "never leave the terminal"; gadak's is different — the
local mirror and SQL. The terminal surfaces are consequences, not the point.
