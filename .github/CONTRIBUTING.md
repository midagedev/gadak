# Contributing

gadak is pre-release but working end to end: sync, the read API, write-through,
the web UI and the CLI are implemented and tested. What remains is
release polish. `docs/project/STATE_OF_PLAY.md` and `specs/000-product/tasks.md` are the
honest inventory — read them before assuming something is or is not there.

The most useful contributions right now are a third source connector behind
the neutral storage layer (Confluence was the second — it merged without
reshaping the database, and the next one should ride the same seam; GitHub
Issues and Linear are the obvious candidates), plugin examples in
`examples/plugins/`, and reports from Jira sites configured differently from
the ones we test against (other languages, team-managed projects, unusual
workflows). Smaller starter tasks are listed in
[`docs/project/GOOD_FIRST_ISSUES.md`](../docs/project/GOOD_FIRST_ISSUES.md).

Before contributing, read:

- `README.md` for the product boundary
- `CODE_OF_CONDUCT.md` for community norms
- `.specify/memory/constitution.md` for the rules that override preference
- `AGENTS.md` for the working rules (they apply to humans too)
- `SECURITY.md` for what must be reported privately

## Development Setup

Requirements: Go 1.25+, Node.js 20+.

```bash
npm ci
npm run build              # writes dist/app; required before go build (go:embed)
go build -o bin/gadak ./cmd/gadak
# or: make build
```

## Before Sending Changes

```bash
go build ./... && go vet ./... && go test ./...
npm run typecheck
npm run build
# optional but expected if you touch the UI:
npm run test:e2e           # or: ./node_modules/.bin/playwright test --config e2e/playwright.config.ts
bash scripts/scan-internal.sh   # or: make scan
```

Makefile targets that match the above: `make build`, `make vet`, `make test`,
`make typecheck`, `make scan`, `make plugins-test` (example plugins), `make docker`.

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
  0.x contract is the three promises in `data-model.md`: `issues_full` and
  the RECIPES queries, `gadak sql` stdout, and `gadak views open --keys -`.
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
touches (HTTP, sync, schema, or agent). Include the commands you ran. Prefer the
PR template checkboxes over a free-form dump.

By participating, you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
