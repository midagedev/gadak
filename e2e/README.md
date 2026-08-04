# Browser E2E (Playwright)

```bash
npx playwright install chromium
npx playwright test --config e2e/playwright.config.ts
```

`serve.sh` builds the binary + UI, copies `examples/demo.db` into a temp `SCRY_HOME`, injects one deploy enrichment, and serves on `127.0.0.1:7877` (no Jira credentials).
