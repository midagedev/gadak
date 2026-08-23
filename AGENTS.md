# AGENTS.md

Two audiences. Pick your half:

- **[Using the mirror](docs/MIRROR.md#using-the-mirror)** — you want answers about issues, or
  you want to comment, transition, or assign one. Most agents stop here.
- **[Developing gadak](#developing-gadak)** — you are changing this repository.

## Developing gadak

### Required reading order

0. **`docs/STATE_OF_PLAY.md`** — what actually exists right now, the next task,
   and the Jira behaviors that already cost debugging time. Start here.
1. `.specify/memory/constitution.md`
2. `specs/000-product/spec.md`
3. `specs/000-product/tasks.md` — the honest state of every piece
4. `specs/000-product/data-model.md` — the schema, and how much of it is promised
5. `specs/000-product/contracts/` — HTTP, sync, and agent contracts
6. `docs/ARCHITECTURE.md` and `docs/EXTRACTION.md`

### Development rules

- The mirror is disposable and Jira is the record. Never add state that only
  lives in gadak, except local personal data, which must stay exportable.
- Nothing installation-specific goes in code or in a built artifact. No site URL,
  project key, custom field id, status name, team label, or person.
- Logic keys on ids and `statusCategory`, never on localized display names. Jira
  translates type and status names per account and ignores `Accept-Language`.
- `internal/store` must not import Jira-shaped code; `internal/jira`
  must not write SQL.
- No network call on a keystroke path. Filtering stays in memory.
- Schema changes are contract changes: update `data-model.md`, add a migration,
  note it in `CHANGELOG.md`, and keep the documented example queries working.
- Credentials never reach SQLite, a log, or a snapshot.
- Derived fields are recomputed from the changelog, never carried forward.

### Before sending changes

```bash
go build ./... && go vet ./... && gofmt -l .
go test ./...
npm run typecheck
npm run build
```

Add a test with any non-trivial logic — parsing, derived fields, sync cursors,
or anything touching the schema.

The E2E fixture server (`e2e/serve.sh`) serves the **built** UI from `dist/app`,
not your source tree. After editing anything under `web/`, rebuild before you
run the suite or screenshot it — otherwise you are testing the last build and
will draw conclusions from a screen the code no longer produces.

`npx playwright test` only rebuilds when it has to start the server itself. The
config sets `reuseExistingServer`, so a server left running from an earlier run
is reused as-is, `serve.sh` never re-runs, and your edits are simply absent.
Stop it first (`pkill -f 'e2e/.tmp/gadak'`) or run `npm run build` by hand.

### Handoff format

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
