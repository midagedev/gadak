# Extraction

Where this code came from, what was cut, and what still needs cutting. Written
down because a reader deserves to know that the web application is battle-tested
and the server is not.

## Origin

The web application in `web/` was built as an internal tool inside a company
monorepo: a Svelte 5 SPA sitting on a Django backend that already mirrored Jira
into PostgreSQL for other purposes. It has been in daily use by a product team
against a real backlog of roughly ten thousand issues.

That is why the UI is mature — virtualized list, saved views, keyboard triage,
ADF rendering, inline write-through — while the server in this repository is a
skeleton. The extraction kept the client and threw away the backend, because the
backend was inseparable from the company's other systems.

## What the internal backend provided

| Capability | Fate in scry |
| --- | --- |
| Jira mirror in PostgreSQL, synced by cron | **Reimplemented** as a local SQLite mirror synced by `scry sync` |
| Derived fields (reopen counts, status timestamps, priority rank) | **Reimplemented**, with site-specific naming rules replaced by status categories |
| Full-text search over descriptions and comments | **Reimplemented** on FTS5 |
| Write proxy to Jira with per-user credentials | **Reimplemented** with credentials in a local config file |
| Attachment caching in S3 with presigned URLs | **Replaced** by a local on-disk cache (`internal/attachcache`) filled on demand |
| Team directory (people, parts, aliases, avatars) | **Cut.** Members are now derived from assignees and reporters in the mirror |
| Team/part taxonomy grouping | **Cut.** The config keys remain so an organization can supply its own labels |
| Deployment state per issue, from a CI/CD index | **Cut** |
| Pull-request links per issue | **Cut** |
| Test-management (Qase) context per issue | **Cut** |
| Personal activity feed and Web Push | **Deferred.** A local watch-based feed is a v0.2 design |
| Multi-viewer presence over WebSocket | **Cut.** Meaningless in a single-user local tool |
| Company SSO and session auth | **Cut.** There are no scry accounts; identity is the stored Jira credential only |
| Email/password login dialog, `scry_token` localStorage, `Authorization: Token` | **Cut.** Frontend leftovers from the internal SSO; writes gate on credential settings alone |
| Data-quality audit endpoint | **Cut** |

## What was scrubbed

The extraction had to remove every trace of the originating installation. Verified
absent from `web/`, `tools/`, and the docs:

- Company and product names, internal domains, and the internal Jira site URL
- Internal email addresses (placeholders are now `you@example.com`)
- Jira project keys that were hardcoded in the built-in view presets
- Team and part identifiers used as grouping keys and avatar colors
- Internal workflow status names used to detect resolution and reopening
- Internal deployment-pipeline vocabulary
- The internal deployment's base path, baked into the bundle, the manifest, and
  the service worker scope
- An internal runbook, including an S3 backup bucket and an AWS profile name

Everything installation-specific now arrives at runtime through `config.json`.
No custom field ids are committed: the internal version hardcoded three of them
in its field-edit allowlist, and that allowlist is now empty by default, which
simply hides the inline editor until an operator configures it.

Constitution Article 7 exists to keep this true. A leak here is not a style
issue; it is the failure mode that makes a public repository unpublishable.

## Behavior changes forced by generalization

These are places where the internal rule was wrong outside its own installation,
so scry uses a different one:

1. **Reopen detection.** The internal version matched status names
   (`"Reopened"`, and its Korean translation). scry counts transitions from a
   `done`-category status to a non-`done` one, which is stable across every site
   and every account language.
2. **Resolution detection.** Same problem, same fix: category, not name.
3. **Staleness.** The internal version read a `working_hours_in_status` column
   that, on inspection, no code ever populated — the "stale" view was reading a
   permanent zero. scry computes staleness from `status_changed_at` with a
   configurable threshold, and the dead column is not carried over.
4. **Built-in views.** Presets that filtered on internal project keys, status
   names, and part groupings are replaced by six presets built only on axes that
   mean the same thing everywhere.
5. **Attachment URLs.** The client validates media URLs against an exact path
   shape before using them as image sources. That check now derives its prefix
   from the configured API base while remaining an exact-shape allowlist, because
   loosening it would be an XSS hole.

## Still to do

- **`PrList`, `DeployTimeline`, and `QaImpact` are still in the tree.** They
  render nothing without data and their config-driven links are inert, but they
  should either move behind a plugin boundary or be deleted. Leaving them is a
  deliberate, temporary choice: deleting them touches the detail panel and the
  type surface, and that refactor is not worth doing in the same change as the
  extraction.
- ~~**The UI is Korean-only.**~~ Done: the copy is English-first with Korean
  kept as a locale (`web/src/lib/i18n/`). Source comments are still partly
  Korean; translating them is cosmetic, not blocking.
- **`d1_group` is still the field name** for the optional group taxonomy in the
  client's types and view config. It should be renamed to something neutral like
  `team_group` in the same pass as the API contract's first stable release.

## Provenance of the demo data

The public demo data is not derived from the originating installation. It is
generated from scratch against a personal Jira Cloud site with three fictional
products, using `go run ./tools/seed-demo`. No real issue text, customer, or
person appears in it. The snapshot that ships in `examples/` is scanned for
credential-shaped strings before it is committed.
