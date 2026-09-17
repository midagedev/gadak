// The list header's own geometry gate (GDK-1936) and its probe.
//
// The review that opened GDK-1936 saw the ja heading cut to `すべて…` with
// the freshness stamp wrapped under it; the fix of that day shed the stamp's
// words only at the squeeze (lib/fit-heading, since deleted). The third
// header control (GDK-1974, 2026-09-17 — the search door) made the shedding
// permanent and the count paid a size, and ja was still cut: a name slot
// that holds ~4.5 CJK glyphs at 26px cannot hold `未割り当ての新規` (8
// glyphs) whole at any size. The lead's decision (2026-09-17): the name
// keeps its words and gives up size first — 26 → 22 → 19 (`--text-title`),
// walked after layout by lib/fit-heading — and only past that last step may
// the CSS ellipsis cut it. This gate walks every built-in issue view in
// every locale and pins that rule: a cut name is legal only at the ladder's
// last step, and a name that fits whole at 19px must be whole.
//
// GDK-1985 (2026-09-17) superseded the search door: the heading is the one
// door and it wears the magnifier, so the name slot took the door's 44px
// back — the readings this gate prints are on that tree.
//
// The probe half prints the header row's box measurements — including the
// `data-fit` step the walk landed on — for every (scope, locale) it runs,
// the same debuggability stance viewport.spec takes with rowH: a moved
// number should explain itself in the run log. One command answers
// "why is this cut off" without writing a scratch spec:
//
//   bash mobile/scripts/heading-boxes.sh            # all three locales
//   GADAK_HEADING_LOCALE=ja bash mobile/scripts/heading-boxes.sh
//
// GADAK_HEADING_SHOT_DIR=<dir> additionally writes one still per case — the
// before/after pair a geometry change owes its reviewer.
import { mkdirSync } from 'node:fs'
import { expect, test } from './helpers'
import { en, ja, ko } from '../../web/src/lib/i18n/catalog'
import { FIT_STEPS } from '../src/lib/fit-heading'

/** The gate runs all three; the probe script may narrow to one. */
const WANTED = (() => {
  const raw = process.env.GADAK_HEADING_LOCALE ?? ''
  if (raw === '') return ['en', 'ko', 'ja'] as const
  if (raw === 'en' || raw === 'ko' || raw === 'ja') return [raw] as const
  throw new Error(`GADAK_HEADING_LOCALE must be en|ko|ja or unset, got ${JSON.stringify(raw)}`)
})()

type Locale = 'en' | 'ko' | 'ja'

/** The desk's own names, resolved from the catalog at load — this gate
 *  never re-types a word the catalog owns. */
const NAMES: Record<Locale, Record<string, string>> = { en, ko, ja }

/*
 * Every built-in issue view (web/src/lib/builtin-views.ts) under the id the
 * phone gives it (domain.ts buildScopes: `all-open` keeps its own id, every
 * other view is `builtin:<id>`). Seeded before the page script runs, the
 * same way the app's own boot restores a stored scope (store.svelte.ts
 * reads it at gadak.issues.scope@local — the dev-proxy host the gate bundle
 * always adopts, see lib/hosts.ts hostIdForEndpoint('')).
 *
 * The two identity views are different: without an identified account the
 * scope list omits them entirely (an anonymous reader has no "mine"), and
 * resolveScope would paint the All-open fallback under their name
 * (domain.ts:694,704). The demo serve answers auth/me with no identity, so
 * each such case route-mocks it with the fixture's own busiest account (the
 * a4-captures pattern — real fixture rows, no writes) and asserts the
 * heading wears the scope's OWN name before measuring anything.
 */
const VIEWS = [
  { short: 'my-work', id: 'builtin:my-work', key: 'view.myWork.name', identity: true },
  { short: 'delegated', id: 'builtin:delegated', key: 'view.delegated.name', identity: true },
  { short: 'all-open', id: 'builtin:all-open', key: 'view.allOpen.name', identity: false },
  { short: 'unassigned-new', id: 'builtin:unassigned-new', key: 'view.unassignedNew.name', identity: false },
  { short: 'reopened', id: 'builtin:reopened', key: 'view.reopened.name', identity: false },
] as const

/** The fixture's own busiest account — measured in a4-captures.spec.ts. */
const IDENTITY = { email: 'demo@example.com', account_id: 'demo-alex', name: 'Alex Kim' }

async function seedScope(
  page: import('@playwright/test').Page,
  locale: string,
  scopeId: string,
): Promise<void> {
  await page.addInitScript(
    (args) => {
      localStorage.setItem('gadak_locale', args.locale)
      localStorage.setItem('gadak.issues.scope@local', JSON.stringify(args.scopeId))
    },
    { locale, scopeId },
  )
}

type Boxes = {
  head: number
  scope: number
  name: number
  nameScroll: number
  nameText: string
  count: number
  countText: string
  spacer: number
  fresh: number
  freshText: string
  newBtn: number
  gear: number
  headHeight: number
  headScroll: number
  fit: string
}

async function measure(page: import('@playwright/test').Page): Promise<Boxes> {
  return page.evaluate(() => {
    const el = (sel: string): HTMLElement | null =>
      document.querySelector<HTMLElement>(`.pane:not(.off) ${sel}`)
    const width = (sel: string): number => Math.round(el(sel)?.getBoundingClientRect().width ?? -1)
    const name = el('h1 .name')
    return {
      head: width('.head'),
      scope: width('h1 button.scope'),
      name: width('h1 .name'),
      nameScroll: name ? name.scrollWidth : -1,
      nameText: (name?.textContent ?? '').trim(),
      count: width('h1 .count'),
      countText: (el('h1 .count')?.textContent ?? '').trim(),
      spacer: width('.head .spacer'),
      fresh: width('button.fresh'),
      freshText: (el('button.fresh')?.textContent ?? '').trim(),
      newBtn: width('.head button.new'),
      gear: width('.head button.gear'),
      headHeight: Math.round(el('.head')?.getBoundingClientRect().height ?? -1),
      headScroll: el('.head')?.scrollWidth ?? -1,
      fit: el('.head')?.getAttribute('data-fit') ?? '',
    }
  })
}

for (const locale of WANTED) {
  for (const view of VIEWS) {
    // FAIL-first on the unfixed source (2026-09-17, this round, before
    // lib/fit-heading was wired back into Issues.svelte — the walk never
    // ran, data-fit never appeared, and five cases were cut at 26px):
    //   en · delegated      .name is cut at step "": scrollWidth 122 > 119
    //   en · unassigned-new .name is cut at step "": scrollWidth 178 > 119
    //   ja · delegated      .name is cut at step "": scrollWidth 129 > 119
    //   ja · all-open       .name is cut at step "": scrollWidth 152 > 119
    //   ja · unassigned-new .name is cut at step "": scrollWidth 205 > 119
    test(`heading boxes (${locale} · ${view.short})`, async ({ page }) => {
      if (view.identity) {
        await page.route('**/api/v1/auth/me/', async (route) => {
          console.log(
            `[heading] ${locale} · ${view.short}: auth/me route-mocked → ${IDENTITY.account_id}` +
              ' (fixture answers email:null)',
          )
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify(IDENTITY),
          })
        })
      }
      await seedScope(page, locale, view.id)
      await page.goto('/', { waitUntil: 'domcontentloaded' })
      await page.locator('.pane:not(.off) h1 button.scope').waitFor()
      await page.locator('.pane:not(.off) button.row').first().waitFor()
      const b = await measure(page)
      const ownName = NAMES[locale][view.key]
      // An identity view the fixture could not identify paints the All-open
      // fallback (resolveScope), and measuring that would gate the wrong
      // row under the right test name — skipped, loudly, never measured.
      if (view.identity && b.nameText === NAMES[locale]['view.allOpen.name']) {
        test.skip(true, `fixture gave no identity: ${view.short} fell back to All open`)
      }
      // Before any geometry: the heading must wear this scope's own name.
      expect(b.nameText, `${locale} · ${view.short} heading text`).toBe(ownName)
      console.log(
        `[heading] ${locale} · ${view.short}: .head ${b.head} | .scope ${b.scope}` +
          ` (.name ${b.name}/${b.nameScroll}${b.nameScroll > b.name ? ' ✂' : ''}` +
          ` "${b.nameText}") | data-fit ${JSON.stringify(b.fit)} | .count ${b.count}` +
          ` (${b.countText}) | .spacer ${b.spacer} | .new ${b.newBtn}` +
          ` | .gear ${b.gear} | .fresh ${b.fresh} | .head h=${b.headHeight} scroll=${b.headScroll}`,
      )
      // Capture only when a round asked for it (e2e/capture-guard.unit.ts): the
      // env read sits beside the call, which is the shape that guard reads.
      const shotDir = process.env.GADAK_HEADING_SHOT_DIR ?? ''
      if (shotDir !== '') {
        mkdirSync(shotDir, { recursive: true })
        await page.screenshot({ path: `${shotDir}/heading-${locale}-${view.short}.png` })
      }
      // The refresh control is a glyph and only a glyph (GDK-1974, 2026-09-17):
      // the words are gone by contract — deleted from the template, not hidden
      // at the squeeze — so text content is a contract reading in every
      // locale, en included. A `syncLabel`-shaped regression shows up as text
      // here long before it shows up as a wrapped row.
      expect(b.freshText, `${locale} · ${view.short} .fresh carries text`).toBe('')
      expect(b.name, `${locale} · ${view.short} .name clientWidth`).toBeGreaterThan(0)
      const whole = b.nameScroll <= b.name
      // The rule this file exists for (GDK-1974): the name gives up size
      // before it gives up words, and the count goes before the words do,
      // so a cut name is legal only at the ladder's last step — never
      // while a step remains.
      expect(
        whole || b.fit === FIT_STEPS[3],
        `${locale} · ${view.short} .name is cut at step ${JSON.stringify(b.fit)}` +
          `: scrollWidth ${b.nameScroll} > clientWidth ${b.name}`,
      ).toBe(true)
      // Every name this fixture seeds is whole somewhere on the ladder.
      // Re-measured 2026-09-17 after GDK-1985 returned the search door's
      // 44px to the name slot: the two longest names no longer reach the
      // last step, so no seeded case hides its count any more — the count
      // assertion below is unconditional. Pinned per locale because the
      // two languages land on different steps: en `Unassigned new` is
      // whole at 22px (step '1'), ja `未割り当ての新規` whole at 19px
      // (step '2') with its count. "English never steps" is true of
      // all-open (pinned below), not of every name.
      expect(
        b.nameScroll,
        `${locale} · ${view.short} .name is ellipsized: scrollWidth ${b.nameScroll}` +
          ` > clientWidth ${b.name}`,
      ).toBeLessThanOrEqual(b.name)
      if (view.short === 'unassigned-new' && locale === 'en') {
        expect(b.fit, 'en · unassigned-new whole at the 22px step (longest name)').toBe(
          FIT_STEPS[1],
        )
      }
      if (view.short === 'unassigned-new' && locale === 'ja') {
        expect(b.fit, 'ja · unassigned-new whole at the 19px step (longest name)').toBe(
          FIT_STEPS[2],
        )
      }
      expect(b.count, `${locale} · ${view.short} keeps its count`).toBeGreaterThan(0)
      // English never steps on all-open — the ladder's floor, and the
      // before/after baseline for the round that built it.
      if (locale === 'en' && view.short === 'all-open') {
        expect(b.fit, 'en · all-open never steps').toBe(FIT_STEPS[0])
      }
      // One line in every language (GDK-1936, revised 2026-09-16): the row's
      // height is asserted, not any one child's — a wrap shows up here as
      // scroll the row cannot hold.
      expect(b.fresh, `${locale} · ${view.short} .fresh is visible`).toBeGreaterThan(0)
      expect(
        b.headScroll,
        `${locale} · ${view.short} the header row overflows its line: scrollWidth` +
          ` ${b.headScroll} > ${b.head}`,
      ).toBeLessThanOrEqual(b.head + 1)
      // The touch-target contract the fix must not pay its width from —
      // the create control and the gear owe the same 44 as each other. The
      // search door is gone (GDK-1985: the heading is the only door and it
      // wears the magnifier), which is where the name slot's 43px came
      // from; the heading button itself carries the 44pt floor every
      // button has (app.css, GDK-867).
      expect(b.newBtn, `${locale} · ${view.short} .new tap target`).toBeGreaterThanOrEqual(44)
      expect(b.gear, `${locale} · ${view.short} .gear tap target`).toBeGreaterThanOrEqual(44)
    })
  }
}
