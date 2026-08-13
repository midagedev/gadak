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
runs **incremental** syncs: one search scoped to the updated-since watermark,
which on a quiet project is a single call that returns nothing.

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
accurately. Offboarding is `rm -rf ~/.gadak`.

## Who makes this, and what happens if they stop?

One person, at the moment. You should weigh that — and here is why it is
less risky than it sounds: the mirror is a **disposable artifact of your own
Jira**, not a database you migrate into. Delete gadak and you have lost
nothing but a cache. The storage schema is documented, and the part of it you
can build on is promised across versions
(`specs/000-product/data-model.md`); the code is Apache-2.0, and the file is
plain SQLite readable by anything. There is no account, no server, and no
format to be stranded in. If the project stops tomorrow, your data was never
in it.

## Several things open the same SQLite file — is that safe?

Yes, by construction: the mirror runs in WAL mode, so one writer (the sync
loop) and any number of readers coexist; `gadak sql` and the MCP server open
the file **read-only** (`mode=ro`). The web UI reads through the same
store layer. You can run all of them at once — that is the intended shape.

## Why not the official Atlassian MCP / a Forge app / a browser extension?

The README's [comparison table](../README.md#how-it-compares) is the long
answer. The short one: a network MCP answers one question per round trip and
cannot join issues to wiki pages, aggregate across a backlog, or work
offline; the derived history columns (`reopen_count`, time-in-status) exist
only because the mirror is local. A Forge app runs on Atlassian's side of
the fence — the whole point here is that the data sits next to your agent.

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
