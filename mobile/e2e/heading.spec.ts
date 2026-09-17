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
  search: number
  searchText: string
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
      search: width('.head button.search'),
      searchText: (el('.head button.search')?.textContent ?? '').trim(),
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
          ` (${b.countText}) | .spacer ${b.spacer} | .search ${b.search}` +
          ` | .new ${b.newBtn} | .gear ${b.gear} | .head h=${b.headHeight}` +
          ` scroll=${b.headScroll}`,
      )
      // Capture only when a round asked for it (e2e/capture-guard.unit.ts): the
      // env read sits beside the call, which is the shape that guard reads.
      const shotDir = process.env.GADAK_HEADING_SHOT_DIR ?? ''
      if (shotDir !== '') {
        mkdirSync(shotDir, { recursive: true })
        await page.screenshot({ path: `${shotDir}/heading-${locale}-${view.short}.png` })
      }
      // Every header control is a glyph and only a glyph (GDK-1974, and the
      // search door that took the refresh control's slot in GDK-1990): the
      // words are gone by contract — deleted from the template, not hidden at
      // the squeeze — so text content is a contract reading in every locale,
      // en included. A `syncLabel`-shaped regression shows up as text here
      // long before it shows up as a wrapped row.
      expect(b.searchText, `${locale} · ${view.short} .search carries text`).toBe('')
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
      expect(b.search, `${locale} · ${view.short} .search is visible`).toBeGreaterThan(0)
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
      // GDK-1990: the search door is the third, and it is a full square —
      // the refresh control it replaced was the one narrow box in the row.
      expect(b.search, `${locale} · ${view.short} .search tap target`).toBeGreaterThanOrEqual(44)
    })
  }
}

/*
 * GDK-1989: the header's hierarchy, as numbers.
 *
 * Three rounds landed on one symptom — "the door into search is not visible"
 * (GDK-1974 chevron, GDK-1985 magnifier, this one) — because the mark on the
 * door was the only axis anyone measured. Nothing read the door's *surface*,
 * the ink it shares with passive text, or whether the three right-hand
 * controls read as one set. This is that axis.
 *
 * FAIL-first, measured on HEAD at 402x874 before the fix (2026-09-18):
 *   button.scope  background rgba(0,0,0,0)  border-bottom 0px none
 *   svg.glass     #635A4F (contrast 5.90)  ==  .count, .gear, .fresh
 *   .name         #1C1812 (contrast 15.41) ==  .new
 *   glyph widths  .new 20  .gear 19  .fresh 14
 *   gaps          .scope->.new 79 (a 73px .spacer inside)  .new->.gear 6
 *
 * The cluster was deliberately NOT three 44px boxes while the refresh
 * control was in it: that box was narrow on purpose (GDK-1974 — "every px
 * it spends is a px the name does not get"), and widening it to 44 cost the
 * ja name a whole fit step. GDK-1990 made them equal the other way — the
 * refresh control left and the search door took its slot — so the even
 * rhythm below is now geometry rather than a hand-placed margin. The width
 * that bought it back is in the `.actions` comment in Issues.svelte.
 */
test('header hierarchy (GDK-1989)', async ({ page }) => {
  await seedScope(page, 'en', 'builtin:all-open')
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('.pane:not(.off) h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  const h = await page.evaluate(() => {
    const el = (sel: string): HTMLElement | null =>
      document.querySelector<HTMLElement>(`.pane:not(.off) ${sel}`)
    const box = (sel: string) => {
      const e = el(sel)
      if (!e) return null
      const b = e.getBoundingClientRect()
      return { left: Math.round(b.left), right: Math.round(b.right), w: Math.round(b.width) }
    }
    const glyphCentre = (sel: string): number => {
      const e = el(sel)
      if (!e) return -1
      const b = e.getBoundingClientRect()
      return Math.round((b.left + b.right) / 2)
    }
    const scope = el('h1 button.scope')
    const cs = scope ? getComputedStyle(scope) : null
    return {
      surface: {
        bottomWidth: Math.round(parseFloat(cs?.borderBottomWidth ?? '0')),
        bottomStyle: cs?.borderBottomStyle ?? 'none',
        bottomColor: cs?.borderBottomColor ?? '',
        rowSeparator: getComputedStyle(
          document.querySelector('.pane:not(.off) button.row')!,
        ).borderBottomColor,
      },
      ink: {
        name: el('h1 .name') ? getComputedStyle(el('h1 .name')!).color : '',
        glass: el('svg.glass') ? getComputedStyle(el('svg.glass')!).color : '',
        count: el('h1 .count') ? getComputedStyle(el('h1 .count')!).color : '',
      },
      glyph: {
        newBtn: Math.round(el('.head button.new svg')?.getBoundingClientRect().width ?? -1),
        gear: Math.round(el('.head button.gear svg')?.getBoundingClientRect().width ?? -1),
        search: Math.round(el('.head button.search svg')?.getBoundingClientRect().width ?? -1),
      },
      centres: {
        newBtn: glyphCentre('.head button.new svg'),
        gear: glyphCentre('.head button.gear svg'),
        search: glyphCentre('.head button.search svg'),
      },
      spacerPresent: el('.head .spacer') !== null,
      boxes: {
        scope: box('h1 button.scope'),
        h1: box('.head h1'),
        actions: box('.head .actions'),
        newBtn: box('.head button.new'),
        gear: box('.head button.gear'),
      },
    }
  })
  console.log(`[hierarchy] ${JSON.stringify(h)}`)

  // 1. The door draws a surface. A rule under the text, not a fill and not a
  //    field shape — the heading stays a heading (DESIGN.md §2, GDK-885).
  expect(h.surface.bottomStyle, 'button.scope draws a bottom rule').not.toBe('none')
  expect(h.surface.bottomWidth, 'button.scope bottom rule width').toBeGreaterThanOrEqual(1)
  //    And it is NOT the ink the list rows below it are ruled with. At
  //    --color-border-subtle the rule was pixel-identical to five stacked
  //    row separators (1.43:1 on the header ground) and read as a divider
  //    that stopped early — the exact way the two earlier rounds failed.
  expect(
    h.surface.bottomColor,
    `button.scope rule is the row separator ink (${h.surface.bottomColor})`,
  ).not.toBe(h.surface.rowSeparator)

  // 2. The door's mark carries the heading's ink, not the count's. On HEAD
  //    the magnifier was the same muted ink as the passive count beside it
  //    and as the least important control in the row.
  expect(h.ink.glass, 'svg.glass wears the heading ink').toBe(h.ink.name)
  expect(h.ink.glass, 'svg.glass is not the count ink').not.toBe(h.ink.count)

  // 3. The three right-hand controls are one set: one glyph size. Their
  //    BOXES stay unequal on purpose (see the header comment).
  expect(h.glyph.gear, '.gear glyph matches .new').toBe(h.glyph.newBtn)
  expect(h.glyph.search, '.search glyph matches .new').toBe(h.glyph.newBtn)

  // 4. The set is separated from the door by more than it is from itself.
  //    Measured from the heading's own slot, not from the rule's right edge:
  //    the heading takes the row's leftover width, so the distance from the
  //    underline to the first glyph is slack, and the gap that says "these
  //    three belong together" is the one between the two slots.
  const doorToSet = h.boxes.actions!.left - h.boxes.h1!.right
  const insideSet = h.boxes.gear!.left - h.boxes.newBtn!.right
  expect(
    doorToSet,
    `the set is grouped: slot gap ${doorToSet} must exceed set-internal ${insideSet}`,
  ).toBeGreaterThan(insideSet)
  expect(insideSet, 'the set is held close').toBeLessThanOrEqual(4)

  // 6. The set has an even rhythm. The three boxes are unequal on purpose,
  //    so the reading that matters is where the GLYPHS sit: at one gap for
  //    all three, the narrow refresh box pulled its glyph 9px closer to the
  //    gear than the gear sits to the create control, which is the stagger
  //    that reads as "the buttons are placed at random".
  const d1 = h.centres.newBtn - h.centres.search
  const d2 = h.centres.gear - h.centres.newBtn
  expect(
    Math.abs(d1 - d2),
    `glyph rhythm: centres ${h.centres.search}/${h.centres.newBtn}/${h.centres.gear}` +
      ` give gaps ${d1} and ${d2}`,
  ).toBeLessThanOrEqual(4)

  // 5. One mechanism for one gap. `.spacer` and `.fresh { margin-left: auto }`
  //    both made the same space on HEAD, and the 73px it held was space the
  //    name could not use because the door was sized by its content.
  expect(h.spacerPresent, '.head .spacer is gone — the door takes the room').toBe(false)
})
