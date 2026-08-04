# scry Constitution

## Article 1: The Mirror Is Disposable, Jira Is the Record

scry never becomes the system of record. Deleting the database and re-syncing
must always be a valid recovery. No state that a user would mourn may live only
in scry, with one exception: local personal data (saved views, watches,
favorites) which must be explicitly exportable.

## Article 2: Local First, No Service

scry runs entirely on the user's machine. There is no scry backend, no account,
no telemetry, and no outbound traffic except to the user's own configured
sources. Any feature that would require a hosted component is out of scope.

## Article 3: The Database Is a Public Interface

The SQLite schema is a contract, not an implementation detail. Agents and
scripts query it directly. Schema changes follow the same review bar as HTTP API
changes: documented in `specs/000-product/data-model.md`, migrated forward, and
noted in `CHANGELOG.md`.

## Article 4: Speed Is the Feature

Interaction latency is a correctness property. Filtering, grouping, sorting, and
search over a warm mirror are memory operations and must stay that way. Any
change that puts a network call on a keystroke path is a regression regardless
of what it adds.

## Article 5: Writes Go Straight Through

Write actions call the source API directly and then refresh the affected rows.
scry does not queue, batch, or reconcile writes, and it does not invent an
offline write model. A failed write surfaces the source's own error.

## Article 6: The Store Is Source-Neutral

Storage, search, and the list/filter engine must not assume Jira. Source-specific
behavior lives behind a connector boundary. A change that spreads Jira concepts
into the neutral layer is rejected even when it is the smaller diff.

## Article 7: Tenant-Neutral Artifacts

No site URL, project key, custom field id, status name, team label, or person
appears in code or in a built artifact. Anything installation-specific is
configuration. This is a hard rule: the repository was extracted from an internal
deployment and must never leak it.

## Article 8: Credentials Stay Out of the Database

API tokens live in a `0600` config file or the OS keychain, never in SQLite,
never in a log, never in a debug bundle, never in a snapshot. Demo snapshots are
verified free of credentials before they are committed.

## Article 9: Derived Fields Are Computed, Documented, and Configurable

Fields Jira does not provide (reopen counts, time in status, resolution
timestamps, priority ranks) are computed during sync from the changelog. Each
one documents its rule, and any rule that depends on an installation's naming is
configurable rather than hardcoded.

## Article 10: Decisions Are Written Down

Architecture, storage, schema, connector, security, and scope decisions are
recorded under `docs/decisions/`. A reversal updates the original record rather
than silently contradicting it.
