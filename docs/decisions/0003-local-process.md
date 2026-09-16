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

## Addendum (2026-09-16) — the tailnet is the LAN; the phone is a browser (GDK-1966)

The single-user loopback model above assumed one machine. A serve bound to a
Tailscale address (`--addr <tailnet-ip>:7777 --allow-remote`) extends the same
trust boundary to the tailnet: every device that can reach the address was
admitted by the tailnet's admin console, so reaching it is the credential.
Consequences:

- The host guard's DNS-name refusal (anti-rebinding) gains an allowlist —
  `--public-url` / `serve.publicUrls`, plus the node's own MagicDNS name when
  `tailscale status` can supply it. Any other DNS name is still 403.
- Identity comes from `tailscale serve` (`Tailscale-User-Login`, `-Name`), read
  only when the request arrives from this machine (loopback or the host's
  own interface address) — the only peers a `tailscale serve` proxy dials
  from, and the only path a trustworthy copy can take. A direct tailnet bind has no identity and
  attributes writes as it did before.
- The phone opens `…/m/` in its browser: the same bundle the native shell
  wrapped, now embedded in the binary. Pairing stays for other machines'
  `gadak` (a CLI has no tailnet identity headers to read) and is gone for the
  phone. The App Store path (GDK-796, GDK-958) is parked, not deleted: the
  native shell returns when a widget or push needs it.

Not decided here: what a shared tailnet means. Today every admitted device
has the owner's write rights on the built-in tracker; per-viewer actors are
the seam the first round measures.
