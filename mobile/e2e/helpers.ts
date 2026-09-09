/*
 * The phone suite's `test` (GDK-1735).
 *
 * Same teardown as the web suite's e2e/helpers.ts, for the same reason: a
 * spec that rewrites a response registers a route whose handler calls
 * `route.fetch()`, the app keeps fetching after the last assertion, and a
 * request still in flight reaches the handler during teardown and throws
 * "Test ended" — failing a test that had already passed. Three of the specs
 * here register such a handler.
 *
 * A file of its own rather than an export from serve.ts: that module is the
 * single owner of the two ports and the bundle stamp, and it is imported by
 * both Playwright configs, which must not pull a Playwright fixture in.
 *
 * `expect` is re-exported so a spec has one import line, not two.
 */
import { expect, test as base } from '@playwright/test'

export const test = base.extend<Record<string, never>>({
  page: async ({ page }, use) => {
    await use(page)
    await page.unrouteAll({ behavior: 'ignoreErrors' })
  },
})

export { expect }
