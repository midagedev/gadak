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


## What belongs here, and what does not (GDK-720)

A browser test earns its cost when the browser is what makes the answer
possible: **paint** (a banner sits over the list, a disabled button, a chip
next to submit), **wiring** across components or across a fetch, or
**navigation** — a link pasted out of the app landing somewhere real. Those are
the assertions no cheaper test can make.

A judgement a pure function makes does not belong here, however true it is.
`web/src/lib/integrations.ts` decides which line of an install stream is the
verdict; `web/src/lib/integrations.test.ts` pins that decision in
milliseconds. Re-asserting it through a routed fetch and a Chromium context
buys a slower copy of a test that already exists, and it comes out of the same
CI budget every other spec is sharing (`tools/e2e-partition.sh`).

The test to apply before adding a case: **delete the browser from the sentence
and see whether anything is left.** "A mid-stream `exit=0` is output, not the
verdict" survives — it is about a function, so it is a unit test. "The echoed
line stays in the log panel while the row turns red" does not — it is about
what is on screen, so it stays here.

`e2e/no-duplicated-unit-title.unit.ts` catches the mechanical half of this: an
e2e title that matches a `web/src` unit title, exactly or after normalisation,
because the sentence usually travels with the paste. It cannot catch a re-proof
that was retitled — that judgement is the paragraph above, and it belongs to
review.
