# Good first issues

Concrete starter work with code evidence. Prefer these over vague "docs polish"
unless you have a specific gap in mind. Label candidates `good first issue` when
filing.

## 1. `scry export` for personal state (T1.4)

**Why:** `saved_views`, `watches`, and `favorites` are the only rows a user
loses if the mirror is deleted. The schema documents that `scry export` must dump
them; the command does not exist yet.

**Where:**

- Spec: `specs/000-product/tasks.md` T1.4 (tables and CRUD done; export not written)
- Contract: `specs/000-product/data-model.md` — section `saved_views`, `watches`,
  `favorites`
- Store CRUD: `internal/store` personal helpers; HTTP already in
  `internal/server/personal.go`
- CLI wiring: `cmd/scry/main.go` (new subcommand next to `status` / `sql`)

**Done when:** `scry export` writes JSON (or a small archive) of the three
tables for the active profile; a round-trip test restores them into an empty DB;
no credentials appear in the output.

## 2. Resolve `@Name` mentions in `scry comment`

**Why:** The web UI turns mentions into ADF mention nodes so people get notified.
The CLI leaves `@Name` as plain text, so nobody is notified.

**Where:**

- `cmd/scry/agent.go` — comment path, explicit deferral:
  `// No mention resolution: … ponytail: add it when someone asks, via the users endpoint the UI uses.`
- Existing user search: `internal/jira` / server `handleUsers` (same lookup the UI uses)
- ADF builder: `internal/jira` `Doc(…, mentions)` used by the server write path

**Done when:** `scry comment KEY -m "thanks @Display Name"` resolves unambiguous
names the same way the UI does, refuses ambiguous matches with candidates, and a
unit test covers longest-name-wins (already tested for the server path).

## 3. English console diagnostics in the web client

**Why:** Several `console.warn` paths still emit Korean strings. The UI catalogs
are English/Korean via i18n, but operator-facing browser console noise should be
English (or locale-aware) for an English-primary open-source audience.

**Where (examples):**

- `web/src/stores/views.svelte.ts` — `'[views] 개인 뷰 저장 실패'`, etc.
- `web/src/stores/me.svelte.ts` — `'워치 로드 실패'`, `'피드 로드 실패'`, …

Source comments are English throughout; this item is the `console.*` text that
was left behind. Korean strings that are *data* — the `i18n/ko.ts` catalog, and
the localized status and person names in test fixtures — are deliberate and must
stay: those fixtures are what keep the display-name trap from coming back.

**Done when:** all `console.warn` / `console.error` strings under `web/src` are
English (or routed through `t()`), and a quick grep shows no Hangul in those
calls.

## 4. Shell completion for the CLI

**Why:** The CLI is hand-rolled (`flag` + a switch in `cmd/scry/main.go`) with no
completion scripts. Completing subcommands (`issue`, `search`, `sql`, `sync`, …)
and common flags is a small, self-contained UX win.

**Where:**

- Command list / usage: `cmd/scry/main.go` (`usage` string and the `switch` on
  `args[0]`)
- Agent subcommands: `cmd/scry/agent.go`
- No completion package exists today — add something like `scry completion bash`
  (and zsh/fish) that prints a script to stdout, documented in `README` or
  `CONTRIBUTING`

**Done when:** installing the script completes subcommand names; tests or a
smoke snippet assert the script contains every top-level command from `usage`.

## 5. Non-Python example plugin

**Why:** The enrichment boundary is language-agnostic, but every example under
`examples/plugins/` is Python 3 stdlib only. A second language proves the
contract and gives non-Python teams a starting point.

**Where:**

- Contract: `docs/PLUGINS.md` (payload shapes, `enrichments` upsert +
  `sync_state.version` bump)
- Working Python references: `examples/plugins/{github-prs,deploy-status,csv-import}/`
- Index: `examples/plugins/README.md`, `make plugins-test`

**Done when:** a small Go or POSIX-sh example (e.g. CSV → `enrichments` upsert)
runs offline against a copy of `examples/demo.db`, is listed in the plugins
README, and is wired into `make plugins-test` (or a sibling target).

---

Not good first issues (need more context or are large): a second source
connector, feed/push redesign, bootstrap payload streaming at 10k issues.
