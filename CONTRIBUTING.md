# Contributing

scry is pre-release. The web application is mature; the server is a skeleton.
`specs/000-product/tasks.md` is the honest inventory — read it before assuming
something works.

Before contributing, read:

- `README.md` for the product boundary
- `.specify/memory/constitution.md` for the rules that override preference
- `AGENTS.md` for the working rules (they apply to humans too)
- `SECURITY.md` for what must be reported privately

## Development Setup

```bash
npm ci
npm run build
go build ./... && go vet ./...
```

## Before Sending Changes

```bash
go build ./... && go vet ./...
npm run typecheck
npm run build
```

Add a test for anything non-trivial: parsing, derived fields, sync cursors,
schema access, or security checks.

## What Gets a Change Rejected

- **Anything installation-specific in code or a built artifact.** No site URL,
  project key, custom field id, status name, team label, or person. This is the
  rule that keeps the repository publishable.
- **Logic keyed on localized display names.** Jira translates issue type and
  status names per account and ignores `Accept-Language`. Use ids or
  `statusCategory`.
- **A network call on a keystroke path.** Filtering stays in memory.
- **Jira concepts leaking into `internal/store`, or SQL leaking into the
  connector.** The boundary is what makes a second source possible.
- **A schema change without a migration and a `data-model.md` update.** The
  schema is a public contract; agents query it directly.
- **Credentials anywhere near SQLite, logs, or snapshots.**

## Public Repo Hygiene

Do not commit:

- credentials, API tokens, or organization API keys
- real issue text, customer data, or personal names
- internal hostnames, private IPs, or company domains
- local absolute paths
- a database snapshot that has not been credential-scanned

## Pull Requests

Keep the change scoped, describe the behavior change, and say which contract it
touches (HTTP, sync, schema, or agent). Include the commands you ran.
