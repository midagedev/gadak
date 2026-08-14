# 0007 — JQL is a filter interchange, not a query engine

Status: accepted
Date: 2026-08-14

## Context

People already live in Jira dashboards and saved filters. Those are JQL
strings (often sitting in a `jql=` URL parameter). Asking them to rebuild
the same view as gadak chips, or asking an agent to guess JQL against the
live API, is the migration tax the product is supposed to remove.

A full JQL → SQL compiler was deferred on the roadmap because a
half-working translator fails in trust-destroying ways: `labels = a AND
labels = b` is not `labels in (a, b)`, and `status = A OR assignee = B`
cannot be a ViewFilters row.

## Decision

**Map a documented subset onto the existing in-memory filter, both
directions, and list everything else.**

- One parser (`internal/jql`), used by `gadak search --jql`,
  `POST /api/v1/issues/jql/`, the search-box paste, and sync's import of
  owned/starred Jira filters (`source_queries`). Dashboards are not imported
  as layouts — only the JQL filters behind them, when owned or starred.
- Supported: AND of `=`, `IN`, `IS EMPTY`, date comparisons (`-7d`,
  `startOfDay()`), `text ~`, `currentUser()`, `ORDER BY` on
  updated/created/priority, and the field set ViewFilters already has
  (project, status, statusCategory, assignee, reporter, labels, priority,
  type, component, fixVersion, created, updated, key, resolution-as-open).
- Refused out loud: WAS/CHANGED, sprint, `!=`, cross-field OR, AND of two
  equals on a multi-valued field, saved `filter=` ids, custom fields.
- Emit is the inverse. gadak-only flags (`reopened`, `stale`) are
  `omitted`, not faked as JQL.
- The store does not import this package. Filtering stays a memory
  operation (Constitution Article 4 and 6).

A JQL → SQL compiler for questions the filter cannot express is a
separate decision, and still needs a real corpus.

## Consequences

- A paste from Jira applies chips in one shot; **Copy JQL** is the way
  back into a dashboard.
- The hosted static demo has no parse endpoint — the toast says so.
- Display names in pasted JQL match whatever the syncing account stored.
  `statusCategory` is the stable axis (`To Do` / `In Progress` / `Done`).
