# Roadmap

`../specs/000-product/tasks.md` holds the task-level state. This is the ordering
and the reasoning.

## v0.1 — the mirror works

Goal: someone can install this, point it at their Jira, and stop waiting on
search.

1. Store: schema, migrations, WAL, FTS5.
2. Jira connector: full sync, incremental sync, changelog and comment paging,
   derived fields.
3. Read API from SQLite: `bootstrap`, `delta`, `detail`, `search`, attachment
   proxy.
4. Write-through: credentials, transitions, comments, attachments, assignee,
   field edits, creation.
5. `scry sql` and `scry status --json` for agents.
6. Demo snapshot and `scry demo`.

**Release blocker, independent of all of the above: the UI is Korean-only.** The
copy has to become English with the current strings kept as a locale. A tool
whose interface the audience cannot read does not spread, however good it is.

## v0.2 — reach and polish

- **Zero-install hosted demo.** `sqlite-wasm` reading a static snapshot over HTTP
  range requests, published on static hosting. No binary, no account, no trust
  decision — the strongest adoption lever available, and much larger than any
  further latency work (`decisions/0004-browser-sqlite.md`).
- **MCP server** for agents without shell access.
- **Local watch feed.** Notify on changes to watched issues, computed from sync
  diffs rather than a server-side event stream. This is what replaces the cut
  internal feed.
- **Plugin boundary or removal** for the leftover internal surfaces (`PrList`,
  `DeployTimeline`, `QaImpact`).
- **Packaging**: signed binaries, Homebrew, container image, and `embed.FS` so the
  binary carries its own UI.

## v0.3 — the second source

Confluence, as the proof that the neutral layer is actually neutral.

- Confluence connector against the same `items` spine and the same FTS path.
- Unified search across issues and pages, since the questions people ask ("what
  do we know about X") do not respect the boundary.
- A generic detail view for document-shaped items.

This is deliberately last. Shipping it earlier would mean designing the neutral
layer against one imagined consumer instead of one real one.

## Considered and not planned

- **Bidirectional sync engines** (PowerSync, Electric, Zero). Wrong shape: they
  assume a remote server with a Postgres upstream, and writes here cannot leave
  the browser directly anyway. See `decisions/0004-browser-sqlite.md`.
- **Jira Server / Data Center.** Not until someone with an instance can test it.
  Guessing at DC behavior is worse than declining to support it.
- **Multi-user or hosted deployment.** Contradicts the security model, which is
  "one user, loopback only, no auth".
- **Boards, sprints, reports.** Jira's own UI does these, and a partial
  reimplementation is worse than a link.
- **Writing to the mirror.** Jira is the record. Any local write model would need
  conflict resolution, which is a different product.
