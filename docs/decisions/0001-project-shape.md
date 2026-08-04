# 0001 — Project shape: extracted client, new local server

Status: accepted
Date: 2026-08-04

## Context

The web application already existed as an internal tool with a Django backend
entangled with a company's other systems. Publishing it required choosing what
to keep.

## Decision

Keep the client verbatim and rewrite the backend as a single local Go binary
with a SQLite mirror. Cut every internal integration rather than porting it.

## Why

The client is the part with accumulated value: virtualized list, filter engine,
ADF rendering, keyboard triage, write-through UX, all proven against a real
10k-issue backlog. Rewriting it would discard that for nothing.

The backend's value was in a pipeline that already existed for other reasons.
Reimplementing just the mirror, locally, is smaller than untangling it — and it
is what turns the tool from "needs a company's infrastructure" into "one binary".

## Consequences

- The HTTP contract is fixed by what the client already speaks
  (`specs/000-product/contracts/api.md`), not designed fresh.
- Cut surfaces (deployment state, PR links, QA context, presence, feed) remain as
  optional, config-gated holes in the response shape.
- The repository ships a mature client and an immature server, and must say so.
