// The scope palette's row-inset gate (GDK-1948) and its probe.
//
// The palette renders DeskRow as a sibling of its own rows inside one list
// (Palette.svelte), and a row lifted into a list needs that list's inset:
// `.palette-row` carries 16px of side padding where DeskRow's own sheet
// values carry 8 (DeskRow.svelte, GDK-1704 — right in a sheet, short by 8
// on each side in this list). Measured on the unfixed source, 2026-09-16 at
// 402×874 / DSF 3: ordinary rows left 49 right 1155; `View settings` left
// 26 right 1179 — the two desk rows overhang the list 24px (8 CSS px a
// side). The fix wraps each Palette DeskRow in SprintLine's `.desk` wrapper
// (`padding: 0 8px`); DeskRow's own values stay where they are already
// right (board row, page-edit row, the detail fields row).
//
// The gate is a measurement, not a screenshot: every row in the open sheet —
// palette rows, section headers, the more/recent buttons, and the desk rows —
// shares one content-box left edge and one right edge.
//
// The probe half prints each row's edges, so a moved number explains itself
// in the run log. One command answers "what are these rows' edges":
//
//   bash mobile/scripts/palette-boxes.sh
import { expect, test } from './helpers'

/** A row's content-box edges: the box the row's content sits in, with the
 *  row's own side padding folded back out. Named by testid when it has one,
 *  by its label text otherwise. */
type RowBox = { name: string; kind: string; left: number; right: number }

async function settle(page: import('@playwright/test').Page): Promise<void> {
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
}

async function bootIssues(page: import('@playwright/test').Page): Promise<void> {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

async function openPicker(page: import('@playwright/test').Page): Promise<void> {
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await settle(page)
}

/** Every row in the open scope sheet, with content-box edges rounded to CSS
 *  pixels (DSF 3 measures in thirds; an inset regression is 8px, not 0.3). */
async function measureSheetRows(page: import('@playwright/test').Page): Promise<RowBox[]> {
  return page.evaluate(() => {
    const rows: { name: string; kind: string; left: number; right: number }[] = []
    const sel = [
      'p.palette-section',
      'button.palette-row',
      'button.desk-row',
      'button.more',
      'button.recent',
    ]
      .map((s) => `.pane:not(.off) ${s}`)
      .join(', ')
    for (const el of document.querySelectorAll(sel)) {
      const name =
        (el as HTMLElement).dataset.testid ??
        el.querySelector('.name, .r-q')?.textContent?.trim() ??
        el.textContent?.trim() ??
        el.className
      const r = el.getBoundingClientRect()
      const cs = getComputedStyle(el)
      rows.push({
        name,
        kind: el.classList.contains('desk-row') ? 'desk-row' : el.tagName.toLowerCase(),
        left: Math.round(r.left + parseFloat(cs.paddingLeft)),
        right: Math.round(r.right - parseFloat(cs.paddingRight)),
      })
    }
    return rows
  })
}

/** The one-edge assertion, with a failure that names the rows that moved. */
function expectOneEdge(rows: RowBox[], side: 'left' | 'right'): void {
  const byEdge = new Map<number, string[]>()
  for (const row of rows) {
    const names = byEdge.get(row[side]) ?? []
    names.push(row.name)
    byEdge.set(row[side], names)
  }
  const listing = [...byEdge.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([edge, names]) => `${edge}px: ${names.join(', ')}`)
    .join('\n')
  expect(
    byEdge.size,
    `${side} edges differ across the sheet's rows:\n${listing}`,
  ).toBe(1)
}

test('every row in the scope sheet shares one left edge and one right edge', async ({ page }) => {
  await bootIssues(page)
  await openPicker(page)

  const rows = await measureSheetRows(page)
  // The sheet always holds ordinary rows, headers, and the two desk rows —
  // a shorter list means the selector drifted, not the inset.
  expect(rows.length, `rows measured: ${rows.map((r) => r.name).join(', ')}`).toBeGreaterThanOrEqual(6)
  expect(rows.some((r) => r.kind === 'desk-row'), 'desk rows on screen').toBe(true)
  console.log(
    `[palette] ${rows.length} rows | ` +
      rows.map((r) => `${r.name} ${r.kind === 'desk-row' ? '(desk) ' : ''}${r.left}..${r.right}`).join(' | '),
  )

  expectOneEdge(rows, 'left')
  expectOneEdge(rows, 'right')
})

test('the other desk-row placements measure where they already were', async ({ page }) => {
  // The fix must not move DeskRow where it is already right (the spec's
  // constraint). These two are the reachable placements outside the palette
  // on this fixture; their alignment is deskrows.spec.ts's contract — this
  // test only reads the numbers, as the before/after baseline a change to
  // DeskRow itself would owe its reviewer. The detail fields row
  // (desk-row-fields) is unreachable here: the fixture configures no custom
  // fields (deskrows.spec.ts pins that premise), and Detail.svelte zeroes
  // the row's own padding through the same wrapper either way.
  await bootIssues(page)
  await page.locator('.pane:not(.off) [data-testid="sprint-line"]').waitFor()
  await settle(page)
  const edges = async (sel: string): Promise<{ left: number; right: number }> => {
    const box = await page.locator(sel).evaluate((el) => {
      const r = el.getBoundingClientRect()
      const cs = getComputedStyle(el)
      return { left: r.left + parseFloat(cs.paddingLeft), right: r.right - parseFloat(cs.paddingRight) }
    })
    return { left: Math.round(box.left), right: Math.round(box.right) }
  }
  const board = await edges('.pane:not(.off) [data-testid="desk-row-board"]')
  console.log(`[palette] board row (sprint line): ${board.left}..${board.right}`)

  await openPicker(page)
  await page.locator('button.palette-row', { hasText: 'Updated' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  const row = page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first()
  await row.waitFor()
  await row.click()
  await page.locator('.page-detail button.back').waitFor()
  await settle(page)
  // PageDetail's screen is not a `.pane` child (deskrows.spec.ts addresses
  // it the same way) — measure the row where it renders.
  const pageEdit = await edges('[data-testid="desk-row-page-edit"]')
  console.log(`[palette] page-edit row (page detail): ${pageEdit.left}..${pageEdit.right}`)

  // Both rows sit inside their pane — a negative reading is a selector
  // drift, not a measurement.
  expect(board.left).toBeGreaterThan(0)
  expect(pageEdit.left).toBeGreaterThan(0)
})
