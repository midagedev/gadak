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
    /*
     * An uncaught error in the app is reported as itself, not as whatever
     * timed out afterwards (GDK-1992).
     *
     * Measured that day: a keyed-`#each` duplicate made Svelte refuse the
     * whole screen, and nine heading cases spent 90 seconds each waiting for
     * a row that could never render. The error was in the page the whole
     * time — it reached us only because Playwright happens to dump a snapshot
     * on failure. Collected rather than thrown from the listener, because a
     * spec asserting an error path should still be the one to decide; the
     * check runs after the body, so a passing test with a broken page fails
     * with the page's own words.
     */
    const pageErrors: Error[] = []
    page.on('pageerror', (err) => pageErrors.push(err))
    await use(page)
    await page.unrouteAll({ behavior: 'ignoreErrors' })
    if (pageErrors.length > 0) {
      const lines = pageErrors.map((e) => e.message.split('\n')[0]).join('\n  ')
      throw new Error(`the page reported ${pageErrors.length} uncaught error(s):\n  ${lines}`)
    }
  },
})

export { expect }
