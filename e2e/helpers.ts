import { execFileSync } from 'node:child_process'
import { existsSync, readdirSync, readFileSync, unlinkSync } from 'node:fs'
import { basename, dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test as base, type ConsoleMessage, type Locator, type Page } from '@playwright/test'
import { en, ja, ko, type MessageKey } from '../web/src/lib/i18n/catalog'
import { LOCALES, type Locale } from '../web/src/lib/i18n/types'

/*
 * The suite's `test`, extended with one teardown (GDK-1735).
 *
 * A spec that rewrites a response registers `page.route(…)` with a handler
 * that calls `route.fetch()`. The app keeps fetching after a test's last
 * assertion — a negative assertion resolves the instant it is made — so a
 * request can still be in flight when the test ends, reach the handler
 * during teardown, and throw "Test ended". That fails a test that had
 * already passed, and it fails it in another spec's words: the block that
 * reaches the log leads with "Response has been disposed" and "Failed to
 * find context", which reads as a dead browser rather than a race. It cost
 * two CI reds in two days before it was read correctly.
 *
 * Dropping the routes before the page goes is Playwright's own advice — it
 * prints it in the error. Doing it here rather than per spec is what makes
 * it structural: 35 of these files register such a handler today and four
 * cleaned up after themselves, and a spec that grows one tomorrow gets the
 * teardown without knowing this happened.
 *
 * `expect` is re-exported so a spec has one import line, not two.
 */
export const test = base.extend<Record<string, never>>({
  page: async ({ page }, use) => {
    await use(page)
    await page.unrouteAll({ behavior: 'ignoreErrors' })
  },
})

export { expect }

const E2E_DIR = dirname(fileURLToPath(import.meta.url))

/** This suite's directory, for a spec that needs a path beside it. */
export function e2eDir(): string {
  return E2E_DIR
}

/** The worktree root — the cwd a spawned gadak needs for `--static dist/app`. */
export function repoRoot(): string {
  return worktreeRoot()
}
const DEFAULT_E2E_PORT = '7877'
const HARDCODED_E2E_HOST = '127.0.0.1:7877'

/**
 * Issue count in examples/demo.db. Single owner: a fixture regen that
 * changes the count updates this constant, and every e2e assertion that
 * waited on the pool size follows.
 */
export const DEMO_ISSUE_COUNT = 534
export const DEMO_ISSUE_COUNT_EN = `${DEMO_ISSUE_COUNT} issues`
export const DEMO_ISSUE_COUNT_KO = `${DEMO_ISSUE_COUNT}건`
export const DEMO_ISSUE_COUNT_JA = `${DEMO_ISSUE_COUNT}件`
export const DEMO_ISSUE_COUNT_EN_RE = new RegExp(`${DEMO_ISSUE_COUNT} issues`)
export const DEMO_ISSUE_COUNT_RE = new RegExp(String(DEMO_ISSUE_COUNT))

/**
 * The Linear counterpart fixture (GDK-1298): `make demo-linear-fixture`
 * writes it and the playwright config's second serve seeds from it. Same
 * single-owner rule as DEMO_ISSUE_COUNT — a regen that changes the count
 * updates the constant, and linear.spec.ts's boot wait follows.
 */
export const LINEAR_SEED_DB = 'examples/demo-linear.db'
export const LINEAR_ISSUE_COUNT = 100
export const LINEAR_ISSUE_COUNT_RE = new RegExp(String(LINEAR_ISSUE_COUNT))

export type AssertServedArtifactOpts = {
  /** Tests: isolate from the process-global ${TMPDIR}/gadak-e2e-served-<port>.json. */
  stampPath?: string
  /** Tests: override git rev-parse --show-toplevel. */
  root?: string
  /** Which serve's stamp to assert: the Linear port's (see linearServePort). */
  port?: string
  /**
   * The stamp's digest names the fixture that port serves (serve.sh folds
   * `seed=<basename>` in). The linear port's fixture is pinned by the
   * playwright config, not by this process's GADAK_SEED_DB — hence explicit.
   */
  linear?: boolean
}

function requirePort(envName: string, raw: string): string {
  if (!/^[1-9][0-9]*$/.test(raw)) {
    throw new Error(`${envName} must be an integer 1-65535, got ${JSON.stringify(raw)}`)
  }
  if (Number(raw) > 65535) {
    throw new Error(`${envName} out of range: ${raw}`)
  }
  return raw
}

/**
 * Single owner for the e2e listen port. Playwright config, serve.sh, the
 * served-artifact stamp, and apiURL() all read GADAK_E2E_PORT (default 7877).
 */
export function e2eServePort(): string {
  const raw = process.env.GADAK_E2E_PORT
  if (raw === undefined || raw === '') return DEFAULT_E2E_PORT
  return requirePort('GADAK_E2E_PORT', raw)
}

/**
 * The suite's second serve: the Linear fixture's port (GDK-1298,
 * e2e/linear.spec.ts). Defaults to two past e2eServePort() so a parallel
 * round that moves the base port moves this one with it. One past is taken:
 * built-in-attachments.spec.ts spawns its own serve on base+1, and this
 * default sitting there meant its healthz poll adopted the Linear serve and
 * every upload was refused by the linear origin — measured when linear.spec
 * landed. GADAK_E2E_LINEAR_PORT overrides it (same integer rules, and never
 * the base port — one listener per port, same contract as the base).
 */
export function linearServePort(): string {
  const raw = process.env.GADAK_E2E_LINEAR_PORT
  if (raw === undefined || raw === '') return String(Number(e2eServePort()) + 2)
  const port = requirePort('GADAK_E2E_LINEAR_PORT', raw)
  if (port === e2eServePort()) {
    throw new Error(`GADAK_E2E_LINEAR_PORT must differ from GADAK_E2E_PORT (${port})`)
  }
  return port
}

/** Absolute URL on the e2e server. Empty path is origin with no trailing slash. */
export function apiURL(path = ''): string {
  const p = !path ? '' : path.startsWith('/') ? path : `/${path}`
  return `http://127.0.0.1:${e2eServePort()}${p}`
}

/** Absolute URL on the Linear-fixture serve. Same shape as apiURL(). */
export function linearApiURL(path = ''): string {
  const p = !path ? '' : path.startsWith('/') ? path : `/${path}`
  return `http://127.0.0.1:${linearServePort()}${p}`
}

/** GADAK_HOME for this suite: e2e/.tmp/home-<port>, so two ports do not share a db. */
export function e2eHomeDir(): string {
  return join(E2E_DIR, '.tmp', `home-${e2eServePort()}`)
}

/**
 * GDK-960: GET ui-focus/ no longer consumes the file. A leftover from
 * `views open` / a spec writeFocus would otherwise yank the next test's URL.
 */
export function clearUIFocus(): void {
  try {
    unlinkSync(join(e2eHomeDir(), 'ui-focus.json'))
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code !== 'ENOENT') throw err
  }
}

/**
 * GDK-672: a literal 127.0.0.1:7877 in e2e/*.spec.ts pins the suite to one
 * port and two worktrees cannot run at once. Use apiURL() from this file.
 * GDK-1559: e2e/demo/*.config.ts joins the scan — a demo suite hardcoding
 * the port collides with a parallel round exactly the same way. (The one
 * deliberate off-port demo config, terminal.config.ts on 7793, documents
 * its reason in the file and never spells this literal.)
 */
export function hardcodedE2EHosts(root = E2E_DIR): string[] {
  const hits: string[] = []
  const scan = (dir: string, label: string, keep: (name: string) => boolean) => {
    for (const name of readdirSync(dir)) {
      if (!keep(name)) continue
      const lines = readFileSync(join(dir, name), 'utf8').split('\n')
      for (let i = 0; i < lines.length; i++) {
        if (!lines[i].includes(HARDCODED_E2E_HOST)) continue
        hits.push(`${label}${name}:${i + 1}: ${lines[i].trim()}`)
      }
    }
  }
  scan(root, '', (name) => name.endsWith('.spec.ts'))
  scan(join(root, 'demo'), 'demo/', (name) => name.endsWith('.config.ts'))
  return hits
}

/** Port-keyed stamp outside any worktree. Matches e2e/serve.sh. */
export function servedStampPathFor(port: string): string {
  const tmp = process.env.TMPDIR || '/tmp'
  return join(tmp, `gadak-e2e-served-${port}.json`)
}

export function servedStampPath(): string {
  return servedStampPathFor(e2eServePort())
}

function worktreeRoot(): string {
  return execFileSync('git', ['rev-parse', '--show-toplevel'], {
    cwd: join(E2E_DIR, '..'),
    encoding: 'utf8',
  }).trim()
}

function servedSourceDigest(root: string, seed: string): string {
  const git = execFileSync('bash', [join(E2E_DIR, 'served-digest.sh')], {
    cwd: root,
    encoding: 'utf8',
  }).trim()
  // Mirrors e2e/serve.sh: GADAK_E2E_SHELL changes what the suite measures but
  // is not a git fact, so served-digest.sh cannot see it. Without this a
  // wide-prompt run would silently reuse the server a plain run left behind.
  let digest = git
  const shell = process.env.GADAK_E2E_SHELL
  if (shell) digest = `${digest} shell=${shell}`
  // Also mirrors serve.sh: the stamp names the fixture being served
  // (`seed=<basename>`), so a live server left by a run that served a
  // different fixture is refused instead of reused.
  return `${digest} seed=${seed}`
}

function parseServedStamp(raw: string, stampPath: string): { worktree: string; digest: string } {
  let value: unknown
  try {
    value = JSON.parse(raw) as unknown
  } catch {
    throw new Error(
      `stale e2e server: stamp ${stampPath} is not JSON. reuseExistingServer picked up a process that was not started by this e2e/serve.sh. Stop it (pkill -f 'e2e/.tmp/gadak') and re-run.`,
    )
  }
  if (
    !value ||
    typeof value !== 'object' ||
    typeof (value as { worktree?: unknown }).worktree !== 'string' ||
    typeof (value as { digest?: unknown }).digest !== 'string' ||
    (value as { worktree: string }).worktree === '' ||
    (value as { digest: string }).digest === ''
  ) {
    throw new Error(
      `stale e2e server: stamp ${stampPath} is not a {worktree, digest} object. reuseExistingServer picked up a process that was not started by this e2e/serve.sh. Stop it (pkill -f 'e2e/.tmp/gadak') and re-run.`,
    )
  }
  return { worktree: (value as { worktree: string }).worktree, digest: (value as { digest: string }).digest }
}

/**
 * Fail when reuseExistingServer attached to a binary that is not this
 * worktree's current served artifact (absolute worktree + source digest).
 */
export function assertServedArtifact(opts: AssertServedArtifactOpts = {}): void {
  const root = opts.root ?? worktreeRoot()
  const port = opts.port ?? e2eServePort()
  const stampPath = opts.stampPath ?? servedStampPathFor(port)
  const seed = opts.linear
    ? basename(LINEAR_SEED_DB)
    : basename(process.env.GADAK_SEED_DB ?? 'examples/demo.db')
  const digest = servedSourceDigest(root, seed)

  if (!existsSync(stampPath)) {
    throw new Error(
      `stale e2e server: missing ${stampPath}. reuseExistingServer picked up a process that was not started by e2e/serve.sh. This worktree is ${root} digest ${digest}. Stop it (pkill -f 'e2e/.tmp/gadak') and re-run.`,
    )
  }

  const stamp = parseServedStamp(readFileSync(stampPath, 'utf8'), stampPath)
  if (stamp.worktree !== root || stamp.digest !== digest) {
    throw new Error(
      `stale e2e server: stamp worktree ${stamp.worktree} digest ${stamp.digest}; this worktree ${root} digest ${digest}. reuseExistingServer reused that process. Stop it (pkill -f '${stamp.worktree}/e2e/.tmp/gadak') and re-run.`,
    )
  }
}

export default function globalSetup(): void {
  const hits = hardcodedE2EHosts()
  if (hits.length) {
    throw new Error(
      `hardcoded ${HARDCODED_E2E_HOST} in e2e/*.spec.ts or e2e/demo/*.config.ts — use apiURL()/e2eServePort() from e2e/helpers.ts:\n${hits.join('\n')}`,
    )
  }
  assertServedArtifact()
  // The second serve (the Linear fixture, GDK-1298) gets the same honesty
  // check: its stamp is port-keyed and its digest names its own fixture.
  assertServedArtifact({ port: linearServePort(), linear: true })
  clearUIFocus()
}

/**
 * The UI locale a media take records in: GADAK_MEDIA_LOCALE, default en.
 *
 * Single owner for the demo rig's language handle. The specs read it to pin
 * the UI and to key their assertions; e2e/demo/export-*.sh read the same
 * variable to name the files they write, so one export cannot land under a
 * name the other half of the pipeline never chose. An unknown value is a
 * typo that would otherwise record a silently English clip, so it throws.
 */
/**
 * Filename the demo specs drop beside a take to record the language it was
 * shot in, and the export scripts read back before naming anything. Same
 * shape as the served-artifact stamp: the pipeline's two halves run minutes
 * apart from different shells, so the take has to carry its own identity.
 */
export const MEDIA_LOCALE_STAMP = '.gadak-media-locale'

export function mediaLocale(): Locale {
  const raw = process.env.GADAK_MEDIA_LOCALE
  if (raw === undefined || raw === '') return 'en'
  const hit = LOCALES.find((l) => l === raw)
  if (!hit) {
    throw new Error(
      `GADAK_MEDIA_LOCALE must be one of ${LOCALES.join(' | ')}, got ${JSON.stringify(raw)}`,
    )
  }
  return hit
}

/**
 * That locale's message table. A demo spec asserting on chrome text reads
 * the string through this rather than restating an English translation:
 * the catalog is the thing the UI renders from, so a spec keyed to it
 * cannot go stale against a copy edit, and it holds in every locale.
 */
export function catalogFor(locale: Locale): Record<MessageKey, string> {
  return { en, ko, ja }[locale]
}

/**
 * The translation file a media take's fixture was built from (GDK-1556).
 *
 * `examples/demo-i18n/<locale>.json` is the committed one; the Makefile's
 * `GADAK_DEMO_I18N_STRINGS` override points both halves at the same other
 * file, so a spec always reads the strings that were actually applied to the
 * mirror it is looking at. `en` is the source list itself — the fixture is
 * English, so `en.json` *is* what the frame shows.
 */
export function fixtureStringsPath(locale: Locale = mediaLocale()): string {
  const root = join(E2E_DIR, '..')
  if (locale === 'en') return join(root, 'examples/demo-i18n/en.json')
  const override = process.env.GADAK_DEMO_I18N_STRINGS
  if (override) return override
  return join(root, `examples/demo-i18n/${locale}.json`)
}

const fixtureStringCache = new Map<string, Record<string, string>>()

function fixtureStrings(path: string): Record<string, string> {
  const hit = fixtureStringCache.get(path)
  if (hit) return hit
  const parsed = JSON.parse(readFileSync(path, 'utf8')) as { strings?: Record<string, string> }
  if (!parsed.strings) throw new Error(`${path}: no "strings" object — not a demo-i18n file`)
  fixtureStringCache.set(path, parsed.strings)
  return parsed.strings
}

/**
 * What the *fixture* reads on camera, in the locale this take records in.
 *
 * The two landing clips record a translated mirror (GDK-1556): the Makefile
 * copies examples/demo.db, runs tools/demo-i18n/apply.py over the copy, and
 * seeds the take from that. So a recording assertion about a priority name or
 * a status name cannot be an English literal — it reads the same id the
 * translation is keyed by (`catalog:priority:High`, `catalog:status:3`;
 * tools/demo-i18n/extract.py documents the id families).
 *
 * A locale file that does not carry the id falls back to `en.json`, which is
 * exactly what apply.py does to the db: an id with no translation stays
 * English there too. That is what makes a partial translation file usable.
 *
 * Only *recordings* may key on a display name, and only to assert what the
 * frame shows. Product logic keys on status_category / status_id /
 * priority_rank / issue_type_id (CLAUDE.md), and so do the locators here —
 * `data-filter-value` for the status axis is the status id.
 */
export function fixtureString(id: string, locale: Locale = mediaLocale()): string {
  const strings = fixtureStrings(fixtureStringsPath(locale))
  const hit = strings[id]
  if (hit !== undefined) return hit
  const source = fixtureStrings(fixtureStringsPath('en'))
  const fallback = source[id]
  if (fallback === undefined) {
    throw new Error(`unknown fixture string id ${JSON.stringify(id)} — see tools/demo-i18n/extract.py`)
  }
  return fallback
}

/**
 * The token the scale flagship types into the palette, per locale.
 *
 * It has to exist across titles, bodies and comments of the cloned rows, so
 * it is a word the fixture's own prose uses — English `retry`, and whatever
 * the translators settled on for it (the shard brief fixes 리트라이→재시도 for
 * ko and リトライ for ja, so the same word recurs across the mirror rather
 * than drifting per document).
 *
 * Verified 2026-09-07 through the app's own search, not a raw FTS MATCH:
 * unicode61 does not segment Japanese, so `items_fts MATCH 'リトライ'` is 0
 * rows while `GET issues/search/?q=リトライ` is 8 issues + 2 pages — the CJK
 * path goes through the cjk_bigram column (internal/store/cjk.go). ko 재시도
 * is 8 issues; en retry, 8.
 */
export const MEDIA_SEARCH_TOKEN: Record<Locale, string> = {
  en: 'retry',
  ko: '재시도',
  ja: 'リトライ',
}

/**
 * The token the search take types, in two halves.
 *
 * That clip exists to show All search reaching past local title matching, so
 * the token has to satisfy three measured properties on the mirror this take
 * records over: no issue or page *title* carries it (local matching cannot see
 * it), no wiki *body* carries it (a doc row would outrank the issues and Enter
 * would land on a page, not the NMA issue the clip ends on), and enough
 * comments carry it that the unified section fills. `prefix` is typed first
 * and still has local hits; the rest of the word clears that section.
 *
 * Measured 2026-09-07 on the applied fixtures — first unified row / its match
 * field, via the palette itself:
 *   en `workaround`   0 titles, 2 issue bodies, 31 comments, 0 pages → NMA-36, comment
 *   ko `임시 방편`     0 titles, 0 issue bodies, 15 comments, 0 pages → NMA-36, comment
 *   ja `回避策`        0 titles, 2 issue bodies, 31 comments, 0 pages → NMA-36, comment
 * — the same issue the English take lands on, in all three.
 *
 * `우회` was the first ko candidate and is wrong: a PROD brief's *body* uses
 * it ("초대가 회사 IdP를 우회하면"), so the selected row came up
 * `palette-unified-doc` and Enter opened a wiki page instead of NMA-36.
 * Prefix title hits (the local section the second half clears): en 103,
 * ko 20, ja 44.
 */
export const MEDIA_COMMENT_TOKEN: Record<Locale, { token: string; prefix: string }> = {
  en: { token: 'workaround', prefix: 'work' },
  ko: { token: '임시 방편', prefix: '임시' },
  ja: { token: '回避策', prefix: '回' },
}

/**
 * Seed locale only when unset so catalog assertions match en.ts by default,
 * without clobbering a user-driven setLocale() across reloads (locale.spec).
 */
export async function forceLocale(page: Page, locale: Locale = 'en'): Promise<void> {
  clearUIFocus()
  await page.addInitScript((loc) => {
    try {
      if (!localStorage.getItem('gadak_locale')) {
        localStorage.setItem('gadak_locale', loc)
      }
    } catch {
      /* ignore */
    }
  }, locale)
}

/** Boot the SPA and wait until the issue list is hydrated. */
export async function gotoApp(page: Page, opts: { startup?: 'epics' | 'product' } = {}): Promise<void> {
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  // Sidebar pool size — visible as soon as bootstrap lands, *before* the
  // startup view. A DEMO_ISSUE_COUNT match is not "the list is ready for keys".
  // The bare count, not the English label: a spec that pre-seeds another
  // locale (locale-goto.spec.ts, GDK-1727) renders "534건" here, and digits
  // are the part every locale shares.
  await expect(page.getByText(DEMO_ISSUE_COUNT_RE).first()).toBeVisible({ timeout: 30_000 })
  // applyStartupView waits for me.authChecked (favorites + GET auth/me/ +
  // personal loads) and then writes the default all-open filter into the
  // hash. IssueList's viewKey effect resetCursor()s on that write, so a
  // j/k/x that landed on the unfiltered pool is wiped. Wait on the
  // hash *and* the refiltered count — Playwright auto-wait, not a sleep —
  // so keyboard tests start after that commit. (GDK-39)
  await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
  // The regex, not the English string: not.toHaveText(string) resolves the
  // instant the text differs from "534 issues", which every non-en locale
  // does before any filtering lands — the wait GDK-39 exists for would be
  // a vacuous pass there (jql.spec.ts:91 and search-demo.spec.ts:86 use the
  // same shape for the same reason).
  await expect(page.getByTestId('list-count')).not.toHaveText(DEMO_ISSUE_COUNT_RE)
  // The fixture's account (dana@example.com) has open work, so the product's
  // first-run rule (startup-view.ts: identified + assigned work → My issues)
  // lands every fresh context on Dana's 46 issues. The specs that call this
  // helper were written against the Epics breakdown — the first-run default
  // before that rule (GDK-100) — and search or click issues outside Dana's
  // set. Restore that view by its hash, so "the list" still means the whole
  // open pool (it was a sidebar click until the row was cut). A spec whose
  // subject is the startup decision itself asks for `startup: 'product'`
  // and sees the rule undisturbed (my-work.spec.ts).
  if (opts.startup !== 'product' && /[#?&]fl=mine(&|$)/.test(page.url())) {
    const mineCount = await page.getByTestId('list-count').textContent()
    // The Epics built-in left the sidebar on 2026-09-07 (GDK-1493), so the
    // same view is reached by its address: the open pool grouped by epic.
    await page.goto('/#/?sc=new%2Cinprogress&g=epic')
    await expect(page).toHaveURL(/[#?&]g=epic/, { timeout: 30_000 })
    await expect(page).not.toHaveURL(/[#?&]fl=mine(&|$)/)
    // The view change is a second boot commit for the list (resetCursor on
    // viewKey): wait for the refiltered count, not just the hash, so a key
    // pressed right after this helper lands on the Epics list, not on the
    // one being torn down.
    await expect(page.getByTestId('list-count')).not.toHaveText(mineCount ?? '')
    // Boot leaves nothing focused; keep it that way after the steer.
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  }
}

/**
 * Boot until the unfiltered list paints — and stop there.
 *
 * gotoApp waits for applyStartupView's hash write (GDK-39) so ordinary
 * keyboard specs never see this window. GDK-46 is about the window itself,
 * so those specs must bypass that wait on purpose.
 */
export async function gotoAppBeforeStartup(page: Page): Promise<void> {
  await forceLocale(page, 'en')
  await page.goto('/')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  // Bare count for the same reason as gotoApp above (GDK-1727).
  await expect(page.getByText(DEMO_ISSUE_COUNT_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
}

/**
 * Hold GET auth/me/ until both `delayMs` and `released` have settled.
 *
 * applyStartupView waits on me.authChecked, which flips in the `finally`
 * after this request. A delay alone is racy on a slow machine (keys land
 * after the commit). Gating continue() on the test's own release is what
 * keeps the keystrokes inside the window.
 */
export async function holdAuthMe(
  page: Page,
  opts: { delayMs: number; released: Promise<void> },
): Promise<void> {
  await page.route('**/api/v1/auth/me/**', async (route) => {
    await Promise.all([
      new Promise<void>((r) => setTimeout(r, opts.delayMs)),
      opts.released,
    ])
    await route.continue()
  })
}

/** Console noise the fixture credential produces (writes 409). Not an app bug. */
export function appConsoleErrors(errors: string[]): string[] {
  return errors.filter((e) => !e.includes('409'))
}

export function attachConsoleErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (msg: ConsoleMessage) => {
    if (msg.type() === 'error') {
      const text = msg.text()
      // Fake e2e token → write endpoints 409; Chromium logs that as a console error.
      if (text.includes('409')) return
      errors.push(text)
    }
  })
  page.on('pageerror', (err) => {
    errors.push(String(err))
  })
  return errors
}

export async function openServerSettings(page: Page): Promise<void> {
  await page.getByRole('button', { name: 'Settings', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible()
}

/** Local client-side search box (`data-testid` — placeholder copy is not the contract). */
export function searchInput(page: Page) {
  return page.getByTestId('search-input')
}

/**
 * Every row of a windowed list, in list order.
 *
 * The document lists render only the rows in view, so counting the DOM answers
 * "how tall is the window", not "what is in the list" — and "grouping regroups,
 * it does not filter" is a claim about the list. This walks the scroller to the
 * bottom and accumulates rows by their key, which asks the original question of
 * a list that no longer renders itself all at once.
 */
export async function walkRows(
  scroller: Locator,
  opts: { rowTestId?: string; keyAttr?: string } = {},
): Promise<{ key: string; title: string }[]> {
  const rowTestId = opts.rowTestId ?? 'doc-row'
  const keyAttr = opts.keyAttr ?? 'data-doc-key'
  const seen = new Map<string, string>()
  await scroller.evaluate((el) => {
    el.scrollTop = 0
  })
  // Bounded so a list that never reports its end fails loudly instead of hanging.
  for (let step = 0; step < 500; step++) {
    const batch = await scroller.evaluate(
      (el, sel) => ({
        rows: [...el.querySelectorAll(`[data-testid="${sel.t}"]`)].map((r) => ({
          key: r.getAttribute(sel.k) ?? '',
          title: r.querySelector('span')?.textContent?.trim() ?? '',
        })),
        atEnd: el.scrollTop + el.clientHeight >= el.scrollHeight - 1,
      }),
      { t: rowTestId, k: keyAttr },
    )
    for (const r of batch.rows) if (!seen.has(r.key)) seen.set(r.key, r.title)
    if (batch.atEnd) break
    await scroller.evaluate((el) => {
      el.scrollTop += el.clientHeight * 0.8
    })
    // A render settle, not a boot wait: the virtualised list rebuilds its
    // window on the frame after scrollTop moves. There is no state to observe
    // here — the next batch is precisely what this loop is about to read, and
    // the steps overlap by 20%, so "the rows changed" is not a safe signal
    // either. One frame plus slack, and `atEnd` above is what ends the loop.
    await scroller.page().waitForTimeout(30)
  }
  await scroller.evaluate((el) => {
    el.scrollTop = 0
  })
  return [...seen].map(([key, title]) => ({ key, title }))
}

/*
 * The terminal reader moved to e2e/term-read.ts (GDK-1567): that module is
 * import-free so mobile/e2e — which compiles against its own @playwright/test
 * install — can import the owner directly instead of carrying a forked copy
 * of the buffer walk. Re-exported here so every existing
 * `import { readTerm } from './helpers'` keeps working (term-rows.unit.ts
 * among them). `translateToString(` now appears only in the owner;
 * e2e/term-read-owner.unit.ts holds that.
 */
export { readTerm, readTermLines, stitchTermRows } from './term-read'
export type { TermRow } from './term-read'

/**
 * Delete every terminal session on the serve, and report how many there were.
 *
 * One owner because the leak is cross-suite (GDK-1127): the whole set shares
 * one serve, so a suite that opens a pane and walks away leaves a session
 * that the 60s grace will not reap — the shell inside is alive, so the grace
 * re-arms (GDK-994) and the row survives with `grace_extensions` counting up.
 * The suite that then asserts a session count fails, and it is never the
 * suite that leaked. Any spec that opens the terminal pane owes an
 * `afterEach(drainTerminalSessions)`; the counting suites also call it in
 * `beforeEach`, so test ordering cannot re-create this.
 */
export async function drainTerminalSessions(page: Page): Promise<number> {
  const res = await page.request.get(apiURL('/api/v1/terminal/sessions/'))
  if (!res.ok()) return 0
  const body = (await res.json()) as { sessions?: { id: string }[] }
  const rows = body.sessions ?? []
  for (const s of rows) {
    await page.request.delete(apiURL(`/api/v1/terminal/sessions/${s.id}/`))
  }
  return rows.length
}
