# Contributing

scry is pre-release but working end to end: sync, the read API, write-through,
the web UI, the TUI, and the CLI are implemented and tested. What remains is
release polish. `docs/STATE_OF_PLAY.md` and `specs/000-product/tasks.md` are the
honest inventory — read them before assuming something is or is not there.

The most useful contributions right now are a second source connector behind the
neutral storage layer, plugin examples in `examples/plugins/`, and reports from
Jira sites configured differently from the ones we test against (other
languages, team-managed projects, unusual workflows).

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
go build ./... && go vet ./... && go test ./...
npm run typecheck
npm run build
./node_modules/.bin/playwright test --config e2e/playwright.config.ts
bash scripts/scan-internal.sh
```

`scan-internal.sh` is the one that keeps this repository publishable — it fails on
token shapes, company strings, and non-allowlisted tenant hosts, including inside
the committed demo snapshot. CI runs it too.

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
