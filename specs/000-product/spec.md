# gadak Product Spec

## Problem

Teams that work inside an issue tracker all day pay two taxes.

**Latency.** Every filter, sort, group, and search in Jira Cloud is a network
round trip against a multi-tenant service. At tens of interactions per hour the
tracker stops being a tool you think with and becomes a tool you wait on.

**Illegibility to agents.** A coding agent asked to reason over a team's issue
history has no good interface. The REST API forces pagination, JQL guessing, and
large JSON payloads through a limited context. There is no way to ask a
relational or full-text question and get an answer in one step.

Both are consequences of the data living only on the far side of an API.

## Solution

Mirror the tracker into a local SQLite database, then serve two readers from it:
a browser UI on localhost and any agent with a shell.

Writes are not mirrored. They call the source API directly and refresh the
affected rows, so Jira remains the system of record and the mirror stays
disposable.

## Users

| User | Need | Success looks like |
| --- | --- | --- |
| Engineer / QA in the tracker daily | Find and triage issues without waiting | Typing narrows 10k issues with no visible delay; keyboard-only triage |
| Coding agent | Answer questions about issue history | One SQL query answers "what regressed in billing since June" |
| Team lead | See where work is stuck | Reopened, stale, and unassigned surfaces without building a JQL each time |
| Evaluator (first 60 seconds) | Understand whether this is worth installing | `gadak demo` shows a populated UI with no account setup |

## In Scope (v0.1)

1. **Sync** — full and incremental mirror of configured Jira projects, including
   fields, descriptions, comments, changelog, issue links, and attachment
   metadata.
2. **Derived fields** — computed at sync time from the changelog:
   `status_changed_at`, `resolved_at`, `reopen_count`, `reopened_at`,
   `priority_rank`, `comment_count`, `description_text`.
3. **Search** — SQLite FTS5 over summary, description, and comment bodies, with
   the existing client-side filter engine layered on top.
4. **Serve** — one HTTP server on localhost that hosts the built web UI, the
   JSON API the UI already speaks, and the runtime config document.
5. **Write-through** — status transitions, comments (with mentions and
   attachments), assignee changes, allowed field edits, and issue creation.
6. **Agent access** — a documented, stable SQLite schema. No wrapper required.
7. **Demo mode** — `gadak demo` serves a bundled snapshot with no credentials.

## Out of Scope (v0.1)

- Any hosted component, account, or telemetry.
- Boards, sprints, backlog ranking, reports, and automation.
- Project or workflow administration.
- Offline writes, write queues, and conflict resolution.
- Sources other than Jira Cloud. The schema stays source-neutral, but no second
  connector ships in v0.1.
- Jira Server / Data Center. Cloud only until someone with a DC instance can
  test it.
- Multi-user deployment. One machine, one user, no authorization model beyond
  binding to loopback.

## Functional Requirements

### FR1 Sync
- `gadak sync` performs a full sync when the mirror is empty and an incremental
  sync otherwise, using `updated >= <watermark>` JQL with cursor pagination.
- Issues deleted or moved out of scope in Jira are removed from the mirror.
- A sync failure leaves the previous mirror readable and does not advance the
  watermark.
- `gadak sync --watch` runs continuously on an interval.

### FR2 Serve
- `gadak serve` binds to `127.0.0.1` by default and refuses to bind a
  non-loopback address without an explicit flag.
- It serves the built UI, `config.json`, and the JSON API under one origin, so
  no CORS configuration is ever required.
- Startup with a missing or stale database is not an error: the UI loads and
  reports sync state.

### FR3 Read API
- The API contract in `contracts/api.md` is what the existing web UI already
  speaks. v0.1 implements the subset marked **required** there.
- `bootstrap` returns the full issue set plus a version token; `delta` returns
  changes since a cursor. Both are served from SQLite without contacting Jira.

### FR4 Write-through
- Every write calls Jira with the user's own credentials, then re-reads the
  affected issue and updates the mirror before responding.
- Jira's error body is passed through so the UI can show the real reason.
- No write is ever buffered locally.

### FR5 Agent access
- The schema is documented and migrated forward. The 0.x contract is the
  three promises in `data-model.md`: `issues_full` and the RECIPES queries,
  `gadak sql` stdout, and `gadak views open --keys -`. A column any of those
  names is never repurposed.
- `gadak sql <query>` provides a convenience read-only path, but direct
  `sqlite3` use is fully supported and preferred.

### FR6 Configuration
- One `~/.gadak/config.json` holds site URL, credentials, project list, field
  mappings, and feature flags.
- Nothing installation-specific is compiled into the binary or the UI bundle.

## Non-Functional Requirements

| Property | Target | How it is checked |
| --- | --- | --- |
| Warm filter latency | Under 50 ms at 10k issues | Bench fixture with a synthetic 10k snapshot |
| Cold start to interactive | Under 1 s with a warm cache | Measured in the browser smoke test |
| Full sync throughput | 10k issues in under 10 min on a home connection | Timed against the demo site |
| Incremental sync | Under 5 s for a typical 5-minute window | Timed against the demo site |
| Memory | Server under 100 MB at 10k issues | Observed during the bench run |
| Binary | Single static binary, no CGO | `CGO_ENABLED=0` build in CI |

## Constraints

- **Jira Cloud has no CORS on its REST API**, so a browser-only build cannot
  exist. A local process is required. (`docs/decisions/0003-local-process.md`)
- **Jira does not allow backdating `created`.** Seeded demo data has real
  changelog history but compressed timestamps; realistic time spread is applied
  when generating a snapshot, not in Jira.
- **Jira localizes issue type and status names** in search responses according to
  the account language, ignoring `Accept-Language`. Any logic that must be
  stable across accounts keys on ids or status categories, never names.
- **Custom field ids differ per site.** Field mapping is configuration; the
  repository ships no site's ids.

## Acceptance Criteria

v0.1 is done when, on a machine with no prior setup:

1. `gadak init && gadak sync && gadak serve` produces a working UI against a real
   Jira Cloud site.
2. Filtering, grouping, searching, and opening a detail panel all work with the
   network disconnected from Jira.
3. A status transition, a comment with an attachment, an assignee change, and an
   issue creation all land in Jira and appear in the UI without a manual sync.
4. `sqlite3 ~/.gadak/gadak.db` answers a reopen-history question and an FTS
   question using only the documented schema.
5. `gadak demo` works with no credentials configured.
6. A fresh clone contains no site URL, project key, custom field id, person, or
   company name anywhere outside example values.
