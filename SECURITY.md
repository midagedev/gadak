# Security Policy

## Supported Versions

scry is pre-release. There is no supported version line yet.

## Reporting a Vulnerability

Open a private advisory:

```text
https://github.com/midagedev/scry/security/advisories/new
```

Do not include real credentials, real issue data, or a database snapshot in a
report.

Report privately if the issue involves:

- credential exposure — tokens reaching SQLite, logs, snapshots, or the client
- the attachment media-URL allowlist being bypassable (an XSS vector, see below)
- the loopback bind guard being bypassable
- HTML injection through rendered issue content
- a path that lets a browser page on another origin reach the local API

Public issues are fine for non-sensitive questions about the security model.

## Security Model

scry has **no authentication**. It is a single-user local tool, and the only
thing separating the mirror from the network is that it binds `127.0.0.1`.
Therefore:

- `scry serve` refuses a non-loopback bind without an explicit `--allow-remote`.
- `--allow-remote` is not a supported deployment mode. Exposing scry to a network
  publishes every issue the mirror holds to anyone who can reach the port.
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

- API tokens live in `~/.scry/config.json` with mode `0600`, or in the OS keychain.
- Tokens are never written to SQLite, a log line, an error message, or a snapshot.
- `GET credential/` returns only a hint, never the token.
- `scry snapshot` refuses to write output containing credential-shaped strings.

## Data Boundaries

scry only ever talks to the source you configure. There is no telemetry, no
update check, and no third-party endpoint. Attachment bytes are proxied on
demand and never written to disk.
