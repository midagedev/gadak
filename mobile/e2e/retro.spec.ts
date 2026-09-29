// The retro screen (GDK-1827, second half) at 402×874, against `gadak demo`
// — the same fixture every other spec here uses. What the phone answers is
// "how did the last week go", not "compare the weeks": the ISO-week column
// table is the desk's axis and does not fit 402px, so this screen is the
// report's current reading — the opening sentence, the four summary numbers,
// the stalled tail, what closed — plus one line saying where the whole
// table lives.
//
// What this file is for: the derivations (splitTemplate's cut, deltaOf's
// tone rule, agingChart's sort) are pinned in web/src/lib/retro/*.test.ts
// over the same functions, with no browser — those tests moved out of
// components/ precisely so the phone could share them (GDK-2044's class: a
// phone import from components/ skips the Mobile CI job). This file is the
// effect confirmed: the palette row exists, the numbers on screen are the
// route's own numbers (recomputed here with the same formatters the screen
// imports), a tail row opens its issue through the phone's one issue path,
// and an empty report says its reason instead of blank copy.
//
// The words are read from the desk's catalog, not typed here (§3.6, the
// settings.spec rule): a spec that hardcoded "Weekly retro" would stay
// green through a rename of the key.
import { expect, test } from './helpers'
import { openPalette, waitPaired } from './nav'
import { en } from '../../web/src/lib/i18n/catalog'
import { METRIC_SPECS, SUMMARY_KEYS, deltaOf, formatValue } from '../../web/src/lib/retro/metrics'
import { SERVE_ORIGIN } from '../playwright.config'

const TITLE = en['retro.title']
const EMPTY = en['retro.empty']
const PANE = '.pane:not(.off) '

/** One bucket as the route answers it — only the fields this spec reads.
 *  `sessions` included: the empty test is the desk's own (notes aside, a
 *  report is empty when every bucket has no sessions, closures or in-progress
 *  work), and a stub that omits it is not an empty report. */
type Bucket = {
  partial?: boolean
  sessions: number
  closed: number | null
  'cycle p85': number | null
  'in progress': number | null
  'wip age max': number | null
}

/** The doc, same shape discipline: only what this file reads. */
type Doc = {
  buckets: Bucket[]
  notes?: { name: string; text: string }[]
  aging?: { p85_days: number | null; items: { key: string; days: number }[] }
}

/** The screen's own fetch: the same route, the same absence of params. */
async function retroDoc(): Promise<Doc> {
  const res = await fetch(`${SERVE_ORIGIN}/api/v1/issues/retro/`)
  return (await res.json()) as Doc
}

/** The bucket the whole screen speaks for: the running one, else the newest
 *  — the desk's own currentIndex rule (RetroView), which on an ordered report
 *  is simply the last bucket. */
function currentBucket(doc: Doc): Bucket {
  const p = doc.buckets.findIndex((b) => b.partial)
  return doc.buckets[p >= 0 ? p : doc.buckets.length - 1]
}

/** The road in, shared by every case below: list → palette → the retro row. */
async function openRetro(page: import('@playwright/test').Page): Promise<void> {
  await openPalette(page)
  await page.locator('button.palette-row', { hasText: TITLE }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await page.getByRole('heading', { name: TITLE }).waitFor()
}

test('the palette offers the retro with no precondition, and the screen renders the report', async ({
  page,
}) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  // Unlike the sprint row (absent until the snapshot holds sprint rows),
  // this row has no data condition: every workspace has a retro — an empty
  // report is itself an answer with its own copy — and no store-held datum
  // predicts one. The desk's palette offers the retro the same way. The row
  // is clicked in place: the magnifier toggles (GDK-1984), so a second
  // openPalette here would close what the first one opened.
  await openPalette(page)
  const row = page.locator('button.palette-row', { hasText: TITLE })
  await expect(row).toHaveCount(1)
  await row.click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await page.getByRole('heading', { name: TITLE }).waitFor()
  await expect(page.locator(`${PANE}[data-testid="retro-sentence"]`)).toBeVisible()
  // One line says where the complete table lives — the axis this screen
  // deliberately does not draw.
  await expect(page.locator(`${PANE}[data-testid="retro-desktop"]`)).toBeVisible()

  // The fixture's report carries the visits note (a fresh demo home has no
  // local.db reads), and a report that carries a note is never "empty": the
  // note is the answer, printed as the report's own last word.
  const doc = await retroDoc()
  if (doc.notes?.length) {
    await expect(page.locator(`${PANE}[data-testid="retro-note"]`).first()).toHaveText(
      doc.notes[0].text,
    )
  }
})

test('the four summary numbers are the last bucket the route answered', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  await openRetro(page)

  const doc = await retroDoc()
  const cur = currentBucket(doc)
  const prevIdx = doc.buckets.indexOf(cur) - 1
  const prev = prevIdx >= 0 ? doc.buckets[prevIdx] : undefined

  // Four cells, in SUMMARY_KEYS' own order, each printing the value the
  // desk's ladder prints for the same bucket — the two surfaces cannot
  // disagree about a number.
  const metrics = await page.locator(`${PANE}[data-testid="retro-cell"]`).evaluateAll((els) =>
    els.map((e) => e.getAttribute('data-metric')),
  )
  expect(metrics).toEqual([...SUMMARY_KEYS])
  await expect(page.locator(`${PANE}[data-testid="retro-value"]`)).toHaveText(
    SUMMARY_KEYS.map((key) => {
      const spec = METRIC_SPECS.find((m) => m.key === key)!
      return formatValue(cur[spec.key], spec.unit)
    }),
  )

  // The step from the bucket before, when there is one to read: the same
  // deltaOf the desk's summary strip uses — running buckets included, whose
  // figure is real but never scored.
  const chips = page.locator(`${PANE}[data-testid="retro-delta"]`)
  const expected = SUMMARY_KEYS.map((key) => {
    const spec = METRIC_SPECS.find((m) => m.key === key)!
    return deltaOf(cur[spec.key], prev ? prev[spec.key] : null, spec.unit, spec.direction, Boolean(cur.partial))
  }).filter((d): d is NonNullable<typeof d> => d !== null)
  if (expected.length) {
    await expect(chips).toHaveText(expected.map((d) => `${d.glyph}${d.text}`))
  } else {
    await expect(chips).toHaveCount(0)
  }
})

test('a stalled tail row opens its issue through the phone’s own path', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  await openRetro(page)

  // The tail reads oldest first — agingChart's own sort, which is also its
  // cap (30 + "N more"), so the first row on screen is the item the whole
  // section is about.
  const doc = await retroDoc()
  const oldest = [...(doc.aging?.items ?? [])].sort((a, b) => b.days - a.days)[0]
  test.skip(!oldest, 'the fixture answers no aging items — nothing to open')
  const row = page.locator(`${PANE}[data-testid="retro-aging-row"]`).first()
  await expect(row).toHaveAttribute('data-key', oldest.key)
  await row.click()

  // The same landing a list row's tap makes: the detail layer over the
  // column, wearing the issue's own key.
  await page.locator('.detail-layer button.back').waitFor()
  await expect(page.locator('.detail-layer .bar-key').first()).toHaveText(oldest.key)
})

test('an empty report says its reason, and a bare one says the empty sentence', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const zeroBucket: Bucket = {
    partial: true,
    sessions: 0,
    closed: 0,
    'cycle p85': null,
    'in progress': 0,
    'wip age max': null,
  }
  const emptyDoc = (notes?: { name: string; text: string }[]) => ({
    buckets: [zeroBucket],
    definitions: {},
    ...(notes ? { notes } : {}),
  })
  // The handler stays registered; `doc` is what it answers today.
  let doc = emptyDoc([
    { name: 'visits', text: 'no issue reads recorded in this window — the stub note' },
  ])
  await page.route('**/api/v1/issues/retro/', (route) =>
    route.fulfill({ contentType: 'application/json', body: JSON.stringify(doc) }),
  )

  await openRetro(page)
  // A report that carries a note is never "empty" (GDK-1679): the note is
  // the answer, so the numbers render above it and the empty plate does not.
  await expect(page.locator(`${PANE}[data-testid="retro-note"]`)).toHaveText(
    'no issue reads recorded in this window — the stub note',
  )
  await expect(page.locator(`${PANE}[data-testid="retro-value"]`)).toHaveCount(4)
  await expect(page.locator(`${PANE}.empty`)).toHaveCount(0)

  // The same zeros without a note: the report's own empty sentence — said,
  // not blanked — with no numbers pretending to be a reading.
  doc = emptyDoc()
  await page.locator(`${PANE}button.back`).click()
  await openRetro(page)
  await expect(page.locator(`${PANE}.empty .title`)).toHaveText(EMPTY)
  await expect(page.locator(`${PANE}[data-testid="retro-value"]`)).toHaveCount(0)
  expect(EMPTY).not.toBe('')
})
