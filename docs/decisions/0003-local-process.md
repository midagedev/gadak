# 0003 — A local process is required; browser-only is impossible

Status: accepted
Date: 2026-08-04

## Context

The most frictionless shape would be a static page: no install, no binary. The
question is whether a browser can talk to Jira Cloud directly.

## Finding

It cannot. Jira Cloud deliberately sends no CORS headers on its REST API, and
there is no origin allowlist on Cloud. Verified against a live site:

| Attempt | Result |
| --- | --- |
| Basic auth, `https://<site>.atlassian.net/rest/api/3/myself` from a page origin | blocked, no `Access-Control-Allow-Origin` |
| Bearer, `https://api.atlassian.com/ex/jira/{cloudId}/rest/api/3/myself` | also not CORS-enabled |

Atlassian's own guidance is to proxy through your own backend. The long-standing
requests for CORS support (JRACLOUD-30371, JRACLOUD-65573) remain open.

## Alternatives considered

- **Browser extension.** Host permissions bypass CORS and the user's existing
  session is reused, so setup is genuinely lighter. Rejected as the primary form
  because an extension cannot hand a coding agent a queryable local database,
  which is half the product. Still the best option for a read-only "make Jira
  fast" tool, and worth revisiting as a companion.
- **Atlassian Forge app.** Runs inside Jira with a request bridge, and is
  distributable through Marketplace, which is the easiest enterprise approval
  path. Rejected for the same reason, plus it puts the UI inside an Atlassian
  iframe and Atlassian's storage limits.
- **Tauri desktop app.** Native fetch has no CORS. A reasonable future packaging
  of the same binary, not a different architecture.

## Decision

Ship a local process. It solves CORS as a side effect (UI and API share one
localhost origin, so no CORS code exists anywhere), and it is the only shape that
also produces the SQLite file agents read.
