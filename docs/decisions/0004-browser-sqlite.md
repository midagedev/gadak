# 0004 — SQLite in the browser: demo only, not the data path

Status: accepted
Date: 2026-08-04

## Context

The mirror is SQLite. A browser can run SQLite too: `sqlite-wasm` persisted to
OPFS gives a real relational database in the page, with FTS5, from a Web Worker.
A family of sync engines exists to keep a server database and a client SQLite
copy in step — PowerSync (the most production-tested, Postgres upstream, WASM
SQLite plus OPFS on the web), ElectricSQL (Postgres, last-write-wins only), Zero
(query-driven, React-only), plus CRDT approaches like cr-sqlite and
server-authoritative ones like SQLSync.

The obvious question: should gadak's browser client hold a SQLite replica instead
of its current IndexedDB cache plus in-memory filter engine?

## Decision

**No for the data path. Yes for one narrow use: a zero-install hosted demo.**

The client keeps IndexedDB for durable cache and the in-memory pool for
filtering. A separate, later, optional build target uses `sqlite-wasm` to read a
static `demo.db` over HTTP range requests so the UI can be published on static
hosting with no binary and no Jira account.

## Why not for the data path

1. **The latency those engines exist to hide does not exist here.** They earn
   their complexity against a remote server over the public internet. gadak's
   server is on loopback. The current client already meets the latency target
   (sub-50 ms filtering at 10k issues) with a shipped, in-daily-use engine.

> [2026-08-06] Measured at 10k: ≈61 ms/op — see STATE_OF_PLAY.md for the
> live number; the 50 ms figure was the design target.

2. **Bidirectional sync is architecturally impossible in this product.** Jira
   Cloud sends no CORS headers, so the browser can never write upstream
   directly. Every write must pass through the local process anyway
   (`0003-local-process.md`). Half of what a sync engine provides is unreachable.

3. **The upstream is wrong for all of them.** PowerSync, Electric, and Zero
   assume a Postgres (or similar) system of record plus their own sync service.
   gadak's system of record is Jira and its "server" is a single local binary.
   Adopting one would mean adding a database and a service — a direct
   contradiction of Constitution Article 2.

4. **The cost is real and the benefit is speculative.** A WASM bundle, OPFS and
   worker plumbing, a second copy of the schema to keep in step, and the
   replacement of a battle-tested 12.5k-line filter engine. What is bought is
   "SQL in the browser", which no user asked for.

5. **Two schemas is a correctness risk, not just work.** The schema is a public
   contract (Article 3). Having it exist in two engines with two migration paths
   doubles the surface where they can drift.

## Why yes for the demo

The single biggest adoption lever for a tool like this is the time between
"heard about it" and "saw it working". Today that requires installing Go, Node,
building, and having a Jira token.

With `sqlite-wasm` reading a static snapshot over range requests, the demo
becomes a URL. No install, no account, nothing to trust. The snapshot is already
being produced for tests (`gadak snapshot`), so the marginal work is a build
target and a read-only data adapter behind the existing store interface — not a
rewrite of the filter engine.

Constraints for that target:

- Read-only. No write paths are compiled in.
- Reads the same snapshot artifact the tests use, so it cannot drift from the
  real schema.
- Ships as a separate entry point; the localhost app is unaffected and pays no
  bundle cost.

## Consequences

- `web/src/lib/db.ts` (IndexedDB) stays the client's durable cache.
- A future adapter boundary is needed in the client so the data source can be
  "gadak server" or "static snapshot". That boundary is small today and should be
  introduced when the demo target is built, not speculatively.
- If the client ever needs genuine SQL — cross-issue aggregation in the UI, say —
  this decision gets revisited. Wanting SQL for its own sake is not that trigger.

## Revisit if

- The filter engine stops meeting the latency target at a scale users actually
  have.
- A second connector makes the client's field schema complex enough that
  hand-written filtering becomes the bottleneck.
- Someone wants gadak's mirror synced across their own machines, which is a real
  problem these tools do solve — though libSQL embedded replicas or plain file
  sync would come first.

## Addendum (2026-08) — static JSON + service worker, not sqlite-wasm

**Status:** accepted for the v0.3 hosted demo.

The decision above said "yes for the demo" via `sqlite-wasm` over a static
`demo.db`. When the demo target was built, a smaller path won:

The web client already boots from one `GET bootstrap/` JSON document and loads
`{key}/detail/` on demand, with IndexedDB as a durable cache and all filtering
in memory. It does not speak SQL. Shipping a WASM SQLite engine, OPFS plumbing,
and a second store adapter would only re-implement the same boot path with a
larger bundle and a second schema runtime to keep in step.

Instead the hosted demo:

1. Freezes `examples/demo.db` into static files with `gadak export-static`
   (`bootstrap.json`, `detail/<KEY>.json`, attachment bytes) — same handlers the
   live server uses, so the snapshot cannot drift from the API.
2. Ships a demo-only service worker (`demo-sw.js`, gated by `VITE_HOSTED_DEMO=1`)
   that rewrites `apiBase` fetches onto those files and answers writes with
   `501 demo_read_only`.
3. Relies on the existing in-memory list filter for typing search. Server FTS
   (`search/`) is unavailable offline and surfaces as a toast — an accepted demo
   limit, not a reason to pull WASM SQLite.

`sqlite-wasm` remains a valid answer if the demo ever needs genuine SQL in the
browser (cross-issue aggregation, agent-style queries in the page). Wanting a
URL people can open without installing is not that trigger; static JSON already
provides it.

