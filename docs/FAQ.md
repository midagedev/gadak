# Hard questions, answered first

The questions a skeptical engineer asks before pointing gadak at a company
Jira — answered here rather than discovered in a comment thread. Shorter
answers live in the README; this page is the receipts.

## What load does this put on my Atlassian site?

A full sync fetches issues through the search API at 100 per call with the
changelog riding along (`expand=changelog`), so mirroring N issues costs
roughly **N/100 search calls**, plus one extra call per issue only when an
issue's comments or history are too long to arrive embedded
(`internal/sync/sync.go`, `internal/jira/client.go`). A 10,000-issue project
is on the order of a hundred-plus requests, once. After that the watch loop
runs **incremental** syncs: one watermark-scoped search per source. That is
not "one empty call": on the measured quiet site a tick fetched 16 issues
and 1 page that matched the window — 0 of them changed — and cost 6.7 s of
wall clock; what a tick costs tracks what the watermark window matches, not
what changed ([`docs/BENCHMARKS.md`](BENCHMARKS.md#where-gadak-loses)).

The client respects `Retry-After` on 429/503 with backoff, and counts its own
call volume per UTC day — visible in `gadak status` and the settings runtime
panel. That counter is *our* process's volume, not the site's remaining
shared rate budget: Atlassian does not expose that, so if many colleagues
run heavy tools against the same site, the budget is shared whether those
tools are gadak or dashboards. Two levers keep gadak a good citizen: scope the
mirror with a project/space allowlist (Settings → Sources), and leave the
default sync interval alone.

To your site admin this looks like a normal API-token integration: the calls
are attributed to the user who issued the token, over the official REST API.
gadak does nothing to disguise itself.

Whether bulk-mirroring data you already have read access to complies with
*your company's* policy is your company's question — gadak cannot answer it,
and `SECURITY.md` says what leaves your machine (nothing) so you can ask it
accurately.

On a connected workspace (one pointed at an Atlassian site), offboarding is
`rm -rf ~/.gadak` — that directory is a cache of the site. On a standalone
workspace, `origin/issuetap.yaml` in the profile directory is the original:
moving or deleting that file is deleting the data. The SQLite mirror
(`gadak.db`) is still a cache either way.

## Who makes this, and what happens if they stop?

One person, at the moment. You should weigh that — and here is why it is
less risky than it sounds: the mirror is a **disposable artifact of the
origin**, not a database you migrate into. On a connected workspace, delete
gadak and you have lost nothing but a cache of your Jira. On standalone, the
record is `origin/issuetap.yaml` in the profile directory — plain YAML,
readable in any editor, without gadak. The storage schema is documented, and
the part of it you can build on is promised across versions
(`specs/000-product/data-model.md`); the code is Apache-2.0, and the mirror
is plain SQLite readable by anything. There is no gadak account and no gadak
server. If the project stops tomorrow, a connected workspace's data was
never in it; a standalone workspace's data is that YAML file.

## Several things open the same SQLite file — is that safe?

Yes, by construction: the mirror runs in WAL mode, so one writer (the sync
loop) and any number of readers coexist. The MCP server opens the file with
`store.Open` (read-write; it runs migrations). `gadak sql` opens it with
`store.OpenReadOnly` (SQLite `mode=ro`). Agent SQL is a second connection:
`gadak_query` uses `mode=ro` and rejects anything that is not SELECT or WITH.
The web UI reads through the same store layer. You can run all of them at once
— that is the intended shape.

## Why not the official Atlassian MCP / a Forge app / a browser extension?

The long answer is [How it compares](#how-it-compares) below. The short one:
a network MCP answers one question per round trip and cannot aggregate across
a backlog or work offline; derived history (`reopen_count`, and days-in-status
computed from `status_changed_at`) exists only because the mirror is local. A Forge app runs on
Atlassian's side of the fence — the whole point here is that the data sits
next to your agent.

## If an agent reads the mirror, where does my data go?

To whatever model that agent talks to. gadak sends nothing anywhere, but an
agent reading the mirror will — that is the honest trade of the whole
category, stated plainly in [`SECURITY.md`](../SECURITY.md). Scope the mirror
to what the agent should see (project/space allowlists, or a separate
profile) rather than assuming the pipe is private.

## How do I know "no telemetry" is true?

Run the grep in [`SECURITY.md`](../SECURITY.md#data-flow) — every
outbound request constructor in the tree resolves to your Atlassian site,
the GitHub Releases version check (off by config, never in dev builds), or
gadak talking to itself on loopback.

## How it compares

- **[jira-cli](https://github.com/ankitpokhrel/jira-cli)** talks to Jira's REST
  API per command, so every listing is a network round trip and JQL is the query
  language. gadak queries a local mirror: filters without a network round trip,
  SQL joins over the changelog, offline reads — plus an app and a web UI over
  the same file. If all
  you want is "create an issue from the terminal", jira-cli is lighter.
- **Linear** is a different tracker. If your team can move, move. gadak is for
  the (much larger) group whose org keeps Jira: it gives you Linear-ish speed
  and keyboard flow without asking anyone for permission — it is a mirror, not a
  migration. A standalone workspace (from 0.16) is the other door: no Atlassian
  account, same mirror and same writes, with `origin/issuetap.yaml` as the
  record.
- **Atlassian's Rovo MCP server** gives agents official, hosted access to the
  same data — worth using if it fits, and for "find me the page about X" it
  often does: it searches Jira and Confluence together. The architectural
  difference is what happens after the search. A network MCP cannot aggregate
  or work offline, every call costs tokens and rate budget, and it answers only
  the questions its tools anticipated — there is no `GROUP BY`. A local SQLite
  file has none of those limits, and derived history (reopen counts and
  reasons, honest epic ancestry) exists only in the mirror.
- **Jira's own UI** stays the source of record and the place for boards,
  sprints, and admin. gadak does not replace it; it replaces waiting on it.
