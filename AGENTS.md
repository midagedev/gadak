# scry Agent Instructions

## Required Reading Order

1. `.specify/memory/constitution.md`
2. `specs/000-product/spec.md`
3. `specs/000-product/tasks.md` — the honest state of every piece
4. `specs/000-product/data-model.md` — the schema is a public contract
5. `specs/000-product/contracts/` — HTTP, sync, and agent contracts
6. `docs/ARCHITECTURE.md` and `docs/EXTRACTION.md`

## Development Rules

- The mirror is disposable and Jira is the record. Never add state that only
  lives in scry, except local personal data, which must stay exportable.
- Nothing installation-specific goes in code or in a built artifact. No site URL,
  project key, custom field id, status name, team label, or person.
- Logic keys on ids and `statusCategory`, never on localized display names. Jira
  translates type and status names per account and ignores `Accept-Language`.
- `internal/store` must not import Jira-shaped code; `internal/connector/jira`
  must not write SQL.
- No network call on a keystroke path. Filtering stays in memory.
- Schema changes are contract changes: update `data-model.md`, add a migration,
  note it in `CHANGELOG.md`, and keep the documented example queries working.
- Credentials never reach SQLite, a log, or a snapshot.
- Derived fields are recomputed from the changelog, never carried forward.

## Before Sending Changes

```bash
go build ./... && go vet ./...
npm run typecheck
npm run build
```

Add a test with any non-trivial logic — parsing, derived fields, sync cursors,
or anything touching the schema.

## Handoff Format

```
Summary
- What changed

Files changed
- path

Verification
- command or evidence

Open risks
- risk and next step
```
