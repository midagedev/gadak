# Security Policy

## Supported Versions

Only the latest published release is supported. Pre-release and development
builds (`0.0.0-dev`, untagged commits) receive no security backports. There is
no long-term support line.

| Version | Supported |
| --- | --- |
| Latest release tag | Yes |
| Older release tags | No |
| Unreleased / `main` | Best effort |

## Reporting a Vulnerability

Use GitHub private vulnerability reporting:

```text
https://github.com/midagedev/scry/security/advisories/new
```

(Repo Settings → Code security → Enable private vulnerability reporting, if the
form is not yet available.)

Do **not** open a public issue for a vulnerability. Do not include real
credentials, real issue data, a database snapshot, or a site URL in a report.

Report privately if the issue involves:

- credential exposure — tokens reaching SQLite, logs, snapshots, or the client
- the attachment media-URL allowlist being bypassable (an XSS vector, see below)
- the loopback bind guard being bypassable
- HTML injection through rendered issue content
- a path that lets a browser page on another origin reach the local API

Public issues are fine for non-sensitive questions about the security model.

## Security Model (summary)

scry is a **single-user local tool with no authentication**. The only network
boundary is the bind address:

| Surface | Guard |
| --- | --- |
| HTTP API / web UI | Binds `127.0.0.1` only; non-loopback requires `--allow-remote` |
| Auth | None — anyone who can reach the port can read the mirror and write to Jira |
| Credentials | API token in `~/.scry/config.json` mode `0600`; never in SQLite, logs, or snapshots |
| Telemetry | None — outbound traffic is only to the configured Jira site |

Therefore:

- `scry serve` refuses a non-loopback bind without an explicit `--allow-remote`.
- `--allow-remote` is **not** a supported multi-user deployment mode. Exposing
  scry on a network publishes every issue the mirror holds to anyone who can
  reach the port.
- There is no multi-user model, no roles, and no audit log.

## Rendered Content Is Untrusted

Issue descriptions and comments are attacker-influenced text — anyone who can
file a ticket can put content in them. The ADF renderer treats them as hostile:

- all text is HTML-escaped
- only a fixed whitelist of tags is emitted; user input never becomes a tag
- `href` values must be `http(s)`; anything else is not rendered as a link
- inline style values must pass a hex-color regex
- unsupported nodes fall back to escaped text, never raw HTML
- media sources must match the exact configured attachment content path shape

Changes to `web/src/lib/adf.ts` are security-relevant. Loosening the media URL
check to a prefix test or a broad regex is an XSS hole, not a simplification.

## Credential Handling

- API tokens live in `~/.scry/config.json` with mode `0600`.
- Tokens are never written to SQLite, a log line, an error message, or a snapshot.
- `GET credential/` returns only a hint, never the token.
- `scry snapshot` refuses to write output containing credential-shaped strings.
  The scan runs over every text column of the finished file while it is still a
  temp file; a hit names the table, row, and pattern — never the value — and
  nothing is published. `--force` overwrites an existing output but cannot skip
  this check.

## Data Boundaries

scry only ever talks to the source you configure. There is no telemetry, no
update check, and no third-party endpoint. Attachment bytes are proxied on
demand (and may be cached locally under the profile directory); credentials never
travel with them in logs or the repository.
