// The list header's own geometry gate (GDK-1936) and its probe.
//
// The review that opened GDK-1936 saw the ja heading cut to `すべて…` with
// the freshness stamp wrapped under it, while en and ko fit. The gate below
// pins the two readings that defect broke — the view name is not ellipsized,
// the stamp is one line — for ko and ja at the gate viewport, on the
// all-open scope (the longest catalog view name the heading can wear).
//
// The probe half prints the header row's box measurements for every locale
// it runs, the same debuggability stance viewport.spec takes with rowH: a
// moved number should explain itself in the run log. One command answers
// "why is this cut off" without writing a scratch spec:
//
//   bash mobile/scripts/heading-boxes.sh            # all three locales
//   GADAK_HEADING_LOCALE=ja bash mobile/scripts/heading-boxes.sh
//
// GADAK_HEADING_SHOT_DIR=<dir> additionally writes one still per locale — the
// before/after pair a geometry change owes its reviewer.
import { mkdirSync } from 'node:fs'
import { expect, test } from './helpers'

/** The gate runs all three; the probe script may narrow to one. */
const WANTED = (() => {
  const raw = process.env.GADAK_HEADING_LOCALE ?? ''
  if (raw === '') return ['en', 'ko', 'ja'] as const
  if (raw === 'en' || raw === 'ko' || raw === 'ja') return [raw] as const
  throw new Error(`GADAK_HEADING_LOCALE must be en|ko|ja or unset, got ${JSON.stringify(raw)}`)
})()

/** All-open wears the longest catalog name of the built-ins (7 CJK glyphs
 *  in ja) — the scope the review's cut came from, and the worst case this
 *  axis owns. Seeded before the page script runs, the same way the app's
 *  own boot restores a stored scope (store.svelte.ts reads it at
 *  gadak.issues.scope@local — the dev-proxy host the gate bundle always
 *  adopts, see lib/hosts.ts hostIdForEndpoint('')). */
async function seedAllOpen(page: import('@playwright/test').Page, locale: string): Promise<void> {
  await page.addInitScript(
    (code) => {
      localStorage.setItem('gadak_locale', code)
      localStorage.setItem('gadak.issues.scope@local', JSON.stringify('builtin:all-open'))
    },
    locale,
  )
}

type Boxes = {
  head: number
  scope: number
  name: number
  nameScroll: number
  count: number
  spacer: number
  fresh: number
  freshLines: number
  newBtn: number
  gear: number
  headHeight: number
  headScroll: number
  stampShown: boolean
}

async function measure(page: import('@playwright/test').Page): Promise<Boxes> {
  return page.evaluate(() => {
    const el = (sel: string): HTMLElement | null =>
      document.querySelector<HTMLElement>(`.pane:not(.off) ${sel}`)
    const width = (sel: string): number => Math.round(el(sel)?.getBoundingClientRect().width ?? -1)
    const name = el('h1 .name')
    const fresh = el('button.fresh')
    // How many line boxes the stamp's text occupies: a Range over the text
    // returns one rect per visual line, so a wrap reads 2 without guessing
    // what "one line tall" is in px.
    const freshLines = (() => {
      const span = fresh?.querySelector('span.stamp')
      if (!span || getComputedStyle(span).display === 'none') return 1
      const range = document.createRange()
      range.selectNodeContents(span)
      return range.getClientRects().length
    })()
    return {
      head: width('.head'),
      scope: width('h1 button.scope'),
      name: width('h1 .name'),
      nameScroll: name ? name.scrollWidth : -1,
      count: width('h1 .count'),
      spacer: width('.head .spacer'),
      fresh: width('button.fresh'),
      freshLines,
      newBtn: width('.head button.new'),
      gear: width('.head button.gear'),
      headHeight: Math.round(el('.head')?.getBoundingClientRect().height ?? -1),
      headScroll: el('.head')?.scrollWidth ?? -1,
      stampShown: (() => {
        const span = fresh?.querySelector('span.stamp')
        return !!span && getComputedStyle(span).display !== 'none'
      })(),
    }
  })
}

for (const locale of WANTED) {
  // FAIL-first on the unfixed source (2026-09-16, this round):
  //   Error: ja .name is ellipsized: scrollWidth 177 > clientWidth 124
  //   [heading] ja: … .fresh 60 (2 lines) — the run stops at the first
  //   assertion, so the fold reads from the probe line.
  test(`heading boxes (${locale})`, async ({ page }) => {
    await seedAllOpen(page, locale)
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await page.locator('.pane:not(.off) h1 button.scope').waitFor()
    await page.locator('.pane:not(.off) button.row').first().waitFor()
    const b = await measure(page)
    console.log(
      `[heading] ${locale}: .head ${b.head} | .scope ${b.scope} (.name ${b.name}/${b.nameScroll}` +
        `${b.nameScroll > b.name ? ' ✂' : ''}) | .count ${b.count} | .spacer ${b.spacer}` +
        ` | .new ${b.newBtn} | .gear ${b.gear} | .fresh ${b.fresh}` +
        ` (stamp ${b.stampShown ? 'shown' : 'shed'}) | .head h=${b.headHeight} scroll=${b.headScroll}`,
    )
    // Capture only when a round asked for it (e2e/capture-guard.unit.ts): the
    // env read sits beside the call, which is the shape that guard reads.
    const shotDir = process.env.GADAK_HEADING_SHOT_DIR ?? ''
    if (shotDir !== '') {
      mkdirSync(shotDir, { recursive: true })
      await page.screenshot({ path: `${shotDir}/heading-${locale}.png` })
    }
    if (locale === 'en') return // en fits today; it is the before/after baseline, not an assertion
    expect(b.name, `${locale} .name clientWidth`).toBeGreaterThan(0)
    expect(
      b.nameScroll,
      `${locale} .name is ellipsized: scrollWidth ${b.nameScroll} > clientWidth ${b.name}`,
    ).toBeLessThanOrEqual(b.name)
    expect(b.fresh, `${locale} .fresh is visible`).toBeGreaterThan(0)
    expect(b.freshLines, `${locale} .fresh wraps`).toBe(1)
    // One line in every language (GDK-1936, revised 2026-09-16). The first
    // fix let the row wrap, which kept the name whole and moved the stamp to
    // a second line in ja only — the exhibit then showed one locale with a
    // header a band taller and a chip floating in it. The row is one line
    // now and the stamp sheds its words instead, so the assertion is on the
    // row's height, not on any one child.
    expect(
      b.headScroll,
      `${locale} the header row overflows its line: scrollWidth ${b.headScroll} > ${b.head}`,
    ).toBeLessThanOrEqual(b.head + 1)
    // The touch-target contract the fix must not pay its width from.
    expect(b.newBtn, `${locale} .new tap target`).toBeGreaterThanOrEqual(44)
    expect(b.gear, `${locale} .gear tap target`).toBeGreaterThanOrEqual(44)
  })
}
