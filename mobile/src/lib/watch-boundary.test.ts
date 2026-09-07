// GDK-1526 — a *config contract*, not a behaviour test.
//
// It asserts that the globs authored in mobile/vite.config.ts cover every
// path the Playwright harness writes or holds, and cover none of the app
// sources that must keep hot-reloading. It does not start a dev server, so
// it cannot prove a page did or did not reload — mobile/e2e/reload-
// isolation.spec.ts does that against a live server.
//
// The matcher is picomatch called the way vite's watcher calls it. Vite
// hands `server.watch.ignored` to chokidar, which tests it through anymatch
// with `{dot: true}` (node_modules/vite/dist/node/chunks/dep-Dm0c1Wj2.js:
// 22066 ANYMATCH_OPTS, applied at :24025 `_userIgnored`). Restating glob
// semantics by hand here would test the restatement, so this borrows the
// real engine; it comes in through createRequire because picomatch 4.0.5
// ships no type declarations and is vite's own transitive dependency, not
// one this package declares.
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import viteConfig, { watchIgnored } from '../../vite.config'

type Picomatch = (
  glob: string | string[],
  options?: { dot?: boolean },
) => (path: string) => boolean

const picomatch = createRequire(import.meta.url)('picomatch') as Picomatch

const MOBILE = fileURLToPath(new URL('../../', import.meta.url)).replace(/\/$/, '')
const REPO = fileURLToPath(new URL('../../../', import.meta.url)).replace(/\/$/, '')

function ignoredGlobs(): string[] {
  const watch = viteConfig.server?.watch
  expect(watch, 'server.watch must stay authored in mobile/vite.config.ts').toBeTruthy()
  const ignored = watch?.ignored
  expect(Array.isArray(ignored), 'server.watch.ignored must be an array of globs').toBe(true)
  return ignored as string[]
}

const isIgnored = (path: string): boolean => picomatch(ignoredGlobs(), { dot: true })(path)

/**
 * Every path the phone's test harness writes or holds. A write to any of
 * these while the viewport gate runs must not reach the app under test.
 * The trace/screenshot entries are the ticket's original suspects; the
 * spec-file entries are the ones that were measured to reload the page.
 */
const HARNESS_PATHS: Array<[string, string]> = [
  ['Playwright trace of a failed test', `${MOBILE}/test-results/a2-captures-Detail/trace.zip`],
  ['Playwright failure screenshot', `${MOBILE}/test-results/a2-captures-Detail/test-failed-1.png`],
  ['Playwright run stamp (dotfile)', `${MOBILE}/test-results/.last-run.json`],
  ['gate spec source', `${MOBILE}/e2e/a2-captures.spec.ts`],
  ['capture written by a spec (dotdir)', `${MOBILE}/e2e/.shots/a2-detail-header.png`],
  ['capture-harness spec source', `${MOBILE}/shots/walk.spec.ts`],
  ['gate config', `${MOBILE}/playwright.config.ts`],
  ['capture-harness config', `${MOBILE}/shots.config.ts`],
  ['tauri shell', `${MOBILE}/src-tauri/tauri.conf.json`],
]

/**
 * App sources. These must keep reaching the dev server: a developer saving
 * a screen has to see it, and an over-broad ignore glob would take that
 * away silently — the failure mode is "my edits do nothing", which is far
 * more expensive to diagnose than a stray reload. web/src/app.css is here
 * because the design tokens live outside mobile/ and are watched too.
 */
const APP_SOURCES: Array<[string, string]> = [
  ['plain module', `${MOBILE}/src/lib/types.ts`],
  ['store', `${MOBILE}/src/lib/store.svelte.ts`],
  ['screen', `${MOBILE}/src/screens/Detail.svelte`],
  ['phone stylesheet', `${MOBILE}/src/app.css`],
  ['html entry', `${MOBILE}/index.html`],
  ['shared design tokens outside mobile/', `${REPO}/web/src/app.css`],
]

describe('vite dev-server watch boundary (GDK-1526)', () => {
  it.each(HARNESS_PATHS)('ignores the %s', (_label, path) => {
    expect(isIgnored(path)).toBe(true)
  })

  it.each(APP_SOURCES)('still watches the %s', (_label, path) => {
    expect(isIgnored(path)).toBe(false)
  })

  // The vitest units are the one entry that has to depend on who is
  // watching. The app's dev server must ignore them — an agent editing a
  // unit test mid-gate otherwise reloads the app under a running spec, which
  // was observed live on 2026-09-07 during this very round. Vitest must not
  // ignore them, because it builds its own watcher from this option and the
  // glob leaves `vitest --watch` deaf to an edited test (measured: with the
  // glob the edit produced no output, without it it printed "RERUN").
  const UNIT = `${MOBILE}/src/lib/store.test.ts`

  it('hides the vitest units from the app dev server', () => {
    expect(picomatch(watchIgnored({}), { dot: true })(UNIT)).toBe(true)
  })

  it('leaves the vitest units visible to vitest itself', () => {
    expect(picomatch(watchIgnored({ VITEST: 'true' }), { dot: true })(UNIT)).toBe(false)
  })

  it('is running under vitest, so the branch above is the live one', () => {
    // If vitest ever stops setting VITEST, the branch would silently pick
    // the dev-server list and `vitest --watch` would go deaf with no test
    // to say so.
    expect(process.env.VITEST).toBeTruthy()
    expect(isIgnored(UNIT)).toBe(false)
  })

  it('keeps the ignore list explicit — no catch-all', () => {
    for (const glob of ignoredGlobs()) {
      expect(glob).not.toBe('**')
      expect(glob).not.toBe('**/*')
      expect(glob.startsWith('**/src/')).toBe(false)
    }
  })
})
