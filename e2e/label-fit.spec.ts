import { test, expect, type Page } from './helpers'
import { attachConsoleErrors, catalogFor, forceLocale } from './helpers'
import { LOCALES, type Locale } from '../web/src/lib/i18n/types'

/*
 * GDK-1938 + GDK-1939 — two width caps that were tuned while looking at
 * Latin text, caught as cut labels in the ko/ja release recordings.
 *
 *   GDK-1939: the FilterBar value-chip cap (max-w-[180px] then) read as
 *     `スプリントの状態: 進行中のスプ…` in the ja retro recording — the cap
 *     is a device-pixel constant and Japanese overruns it.
 *   GDK-1938: the breakdown strip's shortest chip rendered `No…  255` in the
 *     group-by recording while its long neighbours kept 15+ characters:
 *     flexbox distributes shrink in proportion to each item's base size, so
 *     every chip loses a similar *fraction* of itself — and the fraction a
 *     short chip loses comes almost entirely out of its label, because the
 *     dot and the count are flex-none. The shortest name is the one most
 *     worth keeping whole.
 *
 * The gate is numeric and locale-symmetric: render the labels the
 * recordings cut — the sprint-state filter chip and the strip's none-group
 * chip — in en, ko and ja, and fail when a label's scrollWidth exceeds its
 * clientWidth, i.e. when the browser had to ellipsize it. Long *data* names
 * (epic summaries) may still ellipsize on a crowded strip; that is the
 * GDK-1057 decision and it stands. What this gate pins is the strip's own
 * chrome label and every label's readable floor.
 *
 * The locale is chosen the way the app chooses it (localStorage
 * gadak_locale, read at module boot), seeded before the page script runs —
 * forceLocale() in helpers.ts is that mechanism.
 */

/** The recording viewports: both defects were shot at 1280×800. */
const RECORDING_VIEWPORT = { width: 1280, height: 800 } as const

/** The epic-axis view the group-by recording squeezed (`No… 255`). */
const EPIC_VIEW = '/#/?sc=new%2Cinprogress&g=epic'

/** The active-sprint filter whose chip the ja retro recording cut. */
const SPRINT_STATE_VIEW = '/#/?sst=active'

type LabelProbe = {
  text: string
  clientWidth: number
  scrollWidth: number
  /** The label's own font size, so an em floor reads in px. */
  fontSize: number
}

/** Measure the truncate spans inside `selector`: text + the two widths. */
function probeLabels(page: Page, selector: string): Promise<LabelProbe[]> {
  return page.evaluate((sel) => {
    return [...document.querySelectorAll<HTMLElement>(sel)].map((el) => ({
      text: (el.textContent ?? '').replace(/\s+/g, ' ').trim(),
      clientWidth: el.clientWidth,
      scrollWidth: el.scrollWidth,
      fontSize: parseFloat(getComputedStyle(el).fontSize),
    }))
  }, selector)
}

/**
 * The strip's chip count follows bind:clientWidth (maxChips is a width
 * budget), so the first paint can carry the pre-measure default. Two rAFs
 * after the strip is visible, layout has flushed and the probe reads the
 * settled flexbox result, not the transition into it.
 */
async function settleLayout(page: Page): Promise<void> {
  await page.evaluate(
    () => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))),
  )
}

function expectNoEllipsis(label: LabelProbe, where: string): void {
  expect(
    label.scrollWidth,
    `${where}: "${label.text}" is ellipsized — scrollWidth ${label.scrollWidth} > clientWidth ${label.clientWidth}. A width rule tuned on Latin text cut this label; the rule must be sized in the text's own terms.`,
  ).toBeLessThanOrEqual(label.clientWidth)
}

/** Print the probe rows, so a red AND a green run both carry the measurement. */
function logProbe(where: string, labels: LabelProbe[]): void {
  console.log(
    `[label-fit] ${where}: ` +
      labels.map((l) => `"${l.text}" ${l.clientWidth}<${l.scrollWidth}`).join('  '),
  )
}

test.describe('GDK-1939: the sprint-state filter chip fits its whole label', () => {
  test.use({ viewport: { width: RECORDING_VIEWPORT.width, height: RECORDING_VIEWPORT.height } })

  for (const locale of LOCALES) {
    test(`sprint-state chip label is not cut (${locale})`, async ({ page }) => {
      const errors = attachConsoleErrors(page)
      await forceLocale(page, locale)
      await page.goto(SPRINT_STATE_VIEW)
      await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
      await expect(
        page.locator('[data-testid="filter-chip"][data-filter-field="sprint_state"]'),
      ).toBeVisible({ timeout: 30_000 })
      await settleLayout(page)

      const chips = await probeLabels(
        page,
        '[data-testid="filter-chip"][data-filter-field="sprint_state"] > span.truncate',
      )
      expect(chips.length, 'the sprint-state chip must paint exactly one label').toBe(1)
      const cat = catalogFor(locale)
      const expected = (cat['filter.chipFieldValue'] ?? '{field}: {value}')
        .replace('{field}', cat['field.sprint_state'])
        .replace('{value}', cat['sprint.active'])
      // The catalog, not a hand copy: if the chip shows some other string the
      // width assertion below would be about the wrong label.
      expect(chips[0].text, `${locale}: chip text must be the catalog label`).toBe(expected)
      logProbe(`${locale} sprint-state chip`, chips)
      expectNoEllipsis(chips[0], `${locale} sprint-state chip`)
      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  }
})

test.describe('GDK-1938: a crowded breakdown strip keeps its shortest name whole', () => {
  test.use({ viewport: { width: RECORDING_VIEWPORT.width, height: RECORDING_VIEWPORT.height } })

  for (const locale of LOCALES) {
    test(`the none-group chip keeps its full label and every label keeps a floor (${locale})`, async ({
      page,
    }) => {
      const errors = attachConsoleErrors(page)
      await forceLocale(page, locale)
      await page.goto(EPIC_VIEW)
      await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
      await expect(page.getByTestId('breakdown-strip')).toBeVisible({ timeout: 30_000 })
      await expect(
        page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first(),
      ).toBeVisible({ timeout: 30_000 })
      await settleLayout(page)

      const strip = page.getByTestId('breakdown-strip')
      const probe = await strip.evaluate((el) => ({
        sw: el.scrollWidth,
        cw: el.clientWidth,
      }))
      // The floor must not buy readability by scrolling the strip past its
      // edge — that would undo GDK-1057, which this gate keeps honest.
      expect(
        probe.sw,
        `${locale}: strip scrollWidth ${probe.sw} > clientWidth ${probe.cw} — the floor made the legend scroll`,
      ).toBeLessThanOrEqual(probe.cw)

      const labels = await probeLabels(page, '[data-testid="breakdown-strip"] button span.truncate')
      expect(labels.length, 'the fixture must paint breakdown chips on the epic axis').toBeGreaterThan(0)
      const noneExpected = catalogFor(locale)['group.noEpic']
      const noneChip = labels.find((l) => l.text === noneExpected)
      expect(
        noneChip,
        `${locale}: the none-group chip must be on the strip (expected "${noneExpected}"; got ${labels.map((l) => l.text).join(' | ')})`,
      ).toBeTruthy()
      expectNoEllipsis(noneChip!, `${locale} breakdown none-group chip`)
      logProbe(`${locale} breakdown strip`, labels)

      // Every label keeps a readable floor: 7em of the label's own font —
      // the min-w-[7em] rule BreakdownBar's label span carries (7em covers
      // ja エピックなし, the widest none-word at 6em, with font-stack
      // margin). A chip squeezed below that is a sliver with a count, not
      // a summary.
      for (const label of labels) {
        expect(
          label.clientWidth,
          `${locale} "${label.text}": label squeezed to ${label.clientWidth}px — below the 7em (${Math.round(label.fontSize * 7)}px) readable floor`,
        ).toBeGreaterThanOrEqual(7 * label.fontSize)
      }
      expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
    })
  }
})

/*
 * The audit (GDK-1938/1939 debuggability): "is this cut off in Japanese"
 * as one command. Prints every element on the page whose content overflows
 * its box, with clientWidth/scrollWidth, at a chosen locale and viewport:
 *
 *   GADAK_ELLIPSIS_AUDIT=1 GADAK_AUDIT_LOCALE=ja \
 *   GADAK_AUDIT_VIEWPORT=1280x800 GADAK_AUDIT_PATH='/#/?sc=new%2Cinprogress&g=epic' \
 *   GADAK_E2E_PORT=8061 npx playwright test e2e/label-fit.spec.ts -g audit
 *
 * GADAK_AUDIT_LOCALE defaults to en and is validated against LOCALES; the
 * viewport is WIDTHxHEIGHT; the path is any hash view the app understands.
 * CI never sets GADAK_ELLIPSIS_AUDIT, so the describe below does not even
 * register there — the suite's pass count stays honest.
 */
function auditLocale(): Locale {
  const raw = process.env.GADAK_AUDIT_LOCALE
  if (raw === undefined || raw === '') return 'en'
  const hit = LOCALES.find((l) => l === raw)
  if (!hit) {
    throw new Error(`GADAK_AUDIT_LOCALE must be one of ${LOCALES.join(' | ')}, got ${JSON.stringify(raw)}`)
  }
  return hit
}

function auditViewport(): { width: number; height: number } {
  const raw = process.env.GADAK_AUDIT_VIEWPORT ?? '1280x800'
  const m = /^(\d{3,5})x(\d{3,5})$/.exec(raw)
  if (!m) {
    throw new Error(`GADAK_AUDIT_VIEWPORT must look like 1280x800, got ${JSON.stringify(raw)}`)
  }
  return { width: Number(m[1]), height: Number(m[2]) }
}

if (process.env.GADAK_ELLIPSIS_AUDIT) {
  test.describe('ellipsis audit', () => {
    test.use({ viewport: auditViewport() })

    test('prints every element whose content overflows its box', async ({ page }) => {
      const locale = auditLocale()
      const path = process.env.GADAK_AUDIT_PATH ?? EPIC_VIEW
      await forceLocale(page, locale)
      await page.goto(path)
      await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
      // Any known content root, so a board or docs path audits as happily as
      // the list; a path with none of them still audits what painted.
      const contentRoot = page.locator(
        '[data-testid="issue-list-scroller"], [data-testid="board"], [data-testid="docs-view"]',
      )
      try {
        await contentRoot.first().waitFor({ state: 'visible', timeout: 10_000 })
      } catch {
        /* no known root — fall through and audit whatever is on the page */
      }
      await settleLayout(page)

      const rows = await page.evaluate(() => {
        const out: { where: string; text: string; axis: string; clientWidth: number; scrollWidth: number }[] = []
        for (const el of document.body.querySelectorAll<HTMLElement>('*')) {
          const sw = el.scrollWidth
          const cw = el.clientWidth
          if (!(sw > cw) || cw === 0) continue
          // Leaf text carriers only: a container that overflows because its
          // child does would double-report the same cut.
          if (el.children.length > 0 && [...el.children].some((c) => c.scrollWidth > c.clientWidth)) continue
          const text = (el.textContent ?? '').replace(/\s+/g, ' ').trim().slice(0, 60)
          if (!text) continue
          const tid = el.closest('[data-testid]')
          out.push({
            where: tid ? `${tid.getAttribute('data-testid')} ${el.tagName.toLowerCase()}` : el.tagName.toLowerCase(),
            text,
            axis: `w ${cw}<${sw}`,
            clientWidth: cw,
            scrollWidth: sw,
          })
        }
        return out
      })
      console.log(`[ellipsis-audit] locale=${locale} viewport=${JSON.stringify(auditViewport())} path=${path}`)
      console.table(rows)
    })
  })
}
