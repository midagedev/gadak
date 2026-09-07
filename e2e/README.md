# Browser E2E (Playwright)

```bash
npx playwright install chromium
npx playwright test --config e2e/playwright.config.ts
```

`serve.sh` builds the binary + UI, copies `examples/demo.db` into a temp `GADAK_HOME` (`e2e/.tmp/home-<port>`), injects one deploy enrichment, and serves on `127.0.0.1:${GADAK_E2E_PORT:-7877}` (no Jira credentials). Two worktrees can run at once by giving each a port:

```bash
GADAK_E2E_PORT=7901 npx playwright test --config e2e/playwright.config.ts
```

Each spec is named for the user behaviour it protects, not the round that
wrote it. Captures are never taken as a side effect of running the suite:
capture-only tests skip unless a vision round names a directory
(`TERMINAL_SHOT_DIR=… npx playwright test e2e/terminal.spec.ts`), and inline
captures in behaviour tests write only when that spec's `*_SHOT_DIR` env var
is set — `e2e/capture-guard.unit.ts` keeps both shapes true.

