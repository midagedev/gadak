// Viewport geometry gate (GDK-868) at the iPhone 17 Pro layout size the
// lead measured (402×874). Selectors and the sheet-exit trap come from
// scratch/mobile-viewport-probe.mjs. The demo tour is disarmed by omitting
// `?demo-tour` (GDK-869) — do not abort `/__demo-tour__`; that workaround
// existed only while HEAD 200 meant "armed".
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'
import { SHEET_INSET_FLOOR_PX, sheetBottomInset } from '../src/lib/inset'
// GDK-902 2026-09-15: the road to Settings is defined once in nav.ts.
// The local waitPaired/openPalette below stay as they are — they carry
// this file's own measurement waits.
import { openSettings } from './nav'

type Measure = {
  label: string
  hOverflow: number
  navBottomFlush: number | null
  rowCount: number
  rowH: number | null
  /** GDK-1550: min/max of the row heights the median was taken over. */
  rowHSpread: { min: number; max: number } | null
  /** GDK-1550: the main's client height — the dividend of the density print. */
  mainH: number | null
  rowsPerScreen: number | null
  inputsUnder16: { tag: string; fs: string }[]
  buttonsUnder44pt: number
  under44: { h: number; cls: string }[]
  hasEscape: boolean
  /** Which controls answered — the failure message's whole content. */
  escapes: string[]
  /**
   * The open sheet's computed bottom padding and the --safe-bottom the
   * page reports — the GDK-907/GDK-911 axis. Null padding = no sheet open
   * at this step. The expected value is derived from sheetBottomInset, the
   * same pure function the unit beside it pins app.css to, so this rig
   * (inset 0 → the floor) and a notched one are both asserted against the
   * one formula.
   */
  sheetInsetPx: number | null
  safeBottomPx: number
}

async function measure(page: Page, label: string): Promise<Measure> {
  return page.evaluate((label) => {
    const isShown = (el: Element | null): el is Element => {
      if (!el) return false
      const r = el.getBoundingClientRect()
      if (r.width === 0 || r.height === 0) return false
      const s = getComputedStyle(el)
      return s.display !== 'none' && s.visibility !== 'hidden'
    }
    const pane = [...document.querySelectorAll('.pane')].find((p) => p.getBoundingClientRect().height > 0)
    const rows = pane ? [...pane.querySelectorAll('button.row')] : []
    const inputs = [...document.querySelectorAll('input, textarea')]
      .filter((el) => el.getBoundingClientRect().height > 0)
      .map((el) => ({ tag: el.tagName.toLowerCase(), fs: getComputedStyle(el).fontSize }))
    // GDK-902 2026-09-15: there is no tab bar to measure flush against the
    // home indicator. The column itself is what must reach the bottom edge
    // now, so the same axis — "nothing is left floating above the
    // indicator" — is read off the owner pane's own box.
    const paneBox = pane ? pane.getBoundingClientRect() : null
    const main = pane?.querySelector('main')
    // GDK-1550: the row height is the MEDIAN of the painted rows, not
    // rows[0]. Sampling the first row tied the density reading to whichever
    // row the fixture happened to sort first — a one-line rows[0] (59.58px)
    // among a two-line-majority list would report 12/screen where the list
    // the reader scrolls is 9 (baseline 2026-09-11: issues median of 368 =
    // 83.77px, spread 60.58–83.77). The median is the typical row; the
    // spread and the main's height ride along so a moved number can
    // explain itself in the run log.
    const heights = rows
      .map((r) => r.getBoundingClientRect().height)
      .filter((h) => h > 0)
      .sort((a, b) => a - b)
    const rowH = heights.length ? heights[Math.floor(heights.length / 2)] : null
    const rowHSpread =
      heights.length > 0 ? { min: heights[0], max: heights[heights.length - 1] } : null
    const mainH = main ? main.clientHeight : null
    const rowsPerScreen =
      rowH && main && rowH > 0 ? Math.floor(main.clientHeight / rowH) : null
    const under44 = [...document.querySelectorAll('button')]
      .map((b) => {
        const r = b.getBoundingClientRect()
        return { h: r.height, cls: String(b.className).split(' ')[0] }
      })
      .filter((x) => x.h > 0 && x.h < 44)
    // Named, not just counted (GDK-902 debuggability): when this axis
    // fails, "which control is the way out" is the whole question, and a
    // boolean makes it a hunt through a screenshot.
    const escapes = ['button.back', 'button.cancel', 'button.palette-cancel'].flatMap((sel) =>
      [...document.querySelectorAll(sel)]
        .filter(isShown)
        .map((el) => `${sel}@${el.closest('[class]')?.parentElement?.className ?? '?'}`),
    )
    const sheet = document.querySelector('.sheet')
    const sheetInsetPx = sheet ? parseFloat(getComputedStyle(sheet).paddingBottom) : null
    const safeBottomPx =
      parseFloat(
        getComputedStyle(document.documentElement).getPropertyValue('--safe-bottom'),
      ) || 0
    return {
      label,
      hOverflow: document.documentElement.scrollWidth - window.innerWidth,
      navBottomFlush: paneBox ? Math.round(window.innerHeight - (paneBox.y + paneBox.height)) : null,
      rowCount: rows.length,
      rowH,
      rowHSpread,
      mainH,
      rowsPerScreen,
      inputsUnder16: inputs.filter((i) => parseFloat(i.fs) < 16),
      buttonsUnder44pt: under44.length,
      under44,
      // GDK-902 2026-09-15: the tab bar used to count as every screen's
      // visible escape, which made the assertion vacuous on the root. The
      // root has no exit by contract (DESIGN.md §2 entry/exit table) and
      // the labels that measure it are exempted below; every other surface
      // must still show one of its own.
      hasEscape: escapes.length > 0,
      escapes,
      sheetInsetPx,
      safeBottomPx,
    }
  }, label)
}

/**
 * Wait for a sheet's rise to finish before measuring it. Svelte's `fly` runs
 * as a CSS animation on a composited layer, and a rect read mid-transform at
 * deviceScaleFactor 3 comes back a hair under the laid-out size — a 44px
 * control measured 43.99993896484375 and failed the floor for a reason that
 * was never on screen. Measuring a settled sheet is the honest reading; the
 * 44pt assertion itself is untouched.
 */
async function settleSheet(page: Page): Promise<void> {
  await page.locator('.sheet').first().evaluate(async (el) => {
    await Promise.all(el.getAnimations().map((a) => a.finished.catch(() => {})))
  })
}

async function waitPaired(page: Page): Promise<void> {
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

/**
 * GDK-902 2026-09-15: the palette is not a sheet, so `settleSheet` has
 * nothing to wait on. It rises with the body it replaces; what a
 * measurement must not catch is the field mid-layout.
 */
async function openPalette(page: Page): Promise<void> {
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
}

async function walkAll(page: Page): Promise<Measure[]> {
  // No ?demo-tour — that is the real disarm (GDK-869).
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  const report: Measure[] = []
  report.push(await measure(page, 'issues'))

  // GDK-885/GDK-902: the heading is the owner control. Open it, measure the
  // owner list, and leave by Cancel — the picker is not a stack and must
  // not be a dead end (DESIGN.md §2). The label stays `scope-sheet`: it is
  // the same composition, measured at the same point in the walk, and the
  // gate compares labels.
  await openPalette(page)
  report.push(await measure(page, 'scope-sheet'))
  await page.locator('button.palette-cancel').click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })

  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('button.back').waitFor()
  report.push(await measure(page, 'detail'))

  // GDK-911 — this walk measures a sheet inside .detail-layer again. It
  // used to open the transition sheet for that; `gadak demo` is a serve
  // with no origin credential, so the transitions GET answers 409
  // credential_required (measured 2026-08-26 — PROBE HTTP 409
  // /api/v1/issues/NMS-134/transitions/) and, since GDK-906, that sheet
  // cannot open on this fixture. But the layer stopped being a one-sheet
  // layer when the A2 write controls landed (GDK-1497): three more sheets
  // live in it, and the assignee picker opens with no server round-trip —
  // no fetch guards its open, so no 409 can close it. It is the inset
  // owner GDK-907 added, measured on the bytes that ship; the inset
  // assertion itself is in the geometry test below. The priority picker
  // (the first m-btn) cannot be the vehicle: its open fires a priorities
  // GET that 409s on this fixture and refuseWrite() closes the sheet it
  // had just opened.
  await page.locator('.detail-layer button.m-btn').nth(1).click()
  await page.locator('button.cancel').waitFor()
  await settleSheet(page)
  report.push(await measure(page, 'detail-sheet'))
  await page.locator('button.cancel').click()
  await page.locator('button.cancel').waitFor({ state: 'hidden' })

  // The state the 409 leaves behind is still measured: the control refuses
  // at the control — it disables and carries the sentence.
  const chip = page.locator('button.status').first()
  if ((await chip.count()) > 0) {
    await chip.click()
    await chip.and(page.locator(':disabled')).waitFor()
    report.push(await measure(page, 'detail-writes-off'))
  }

  await page.locator('button.back').first().click()
  // Wait for the layer to LEAVE, not just for the list to be there — the
  // list pane is always mounted, so its rows answer instantly while the
  // detail is still flying out (200ms). Measuring in that window read the
  // detail's own back control as the docs plate's escape (GDK-902
  // 2026-09-15; the tab bar used to answer hasEscape for every screen, so
  // this race could not surface before).
  await page.locator('.detail-layer').waitFor({ state: 'detached' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  // GDK-887: Updated (whole-mirror) documents plate, then one page detail.
  await openPalette(page)
  await page.locator('.palette-section', { hasText: 'Documents' }).waitFor()
  await page.locator('button.palette-row', { hasText: 'Updated' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().waitFor()
  report.push(await measure(page, 'docs'))

  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().click()
  await page.locator('.page-detail button.back').waitFor()
  report.push(await measure(page, 'page-detail'))
  await page.locator('.page-detail button.back').first().click()
  await page.locator('.detail-layer').waitFor({ state: 'detached' })
  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().waitFor()

  // GDK-902 2026-09-15: every label below survives — the gate compares
  // them — but the road to each one changed. Search is the palette with a
  // query, and the `pairing` frame is the Settings push layer behind the
  // gear (the label is the gate's, the screen's name is Settings).
  await openPalette(page)
  report.push(await measure(page, 'search-empty'))

  await page.locator('.palette-field input').fill('tenant')
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  report.push(await measure(page, 'search-results'))

  await page.locator('button.palette-cancel').click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await openSettings(page)
  report.push(await measure(page, 'pairing'))

  await page.locator('.settings-layer button.back').click()
  await page.locator('.settings-layer').waitFor({ state: 'detached' })
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.emulateMedia({ colorScheme: 'dark' })
  report.push(await measure(page, 'issues-dark'))

  return report
}

test('disarmed boot does not change owner, palette, layer or detail', async ({ page }) => {
  // GDK-902 2026-09-15: was "does not change tab or detail", read off the
  // tab bar's aria-current. There are four pieces of navigation state now
  // and all four must stay put — including the palette, whose whole
  // contract is that it is dormant until the heading is tapped.
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  const state = async () => ({
    // The list owns the column: its pane is the visible one and it draws
    // rows, which only the list does.
    listRows: await page.locator('.pane:not(.off) button.row').count(),
    palette: await page.locator('.palette-field input').count(),
    layers: await page.locator('.detail-layer, .settings-layer, button.back').count(),
  })
  const before = await state()
  expect(before.listRows, 'the list is the owner and is drawing').toBeGreaterThan(0)
  expect(before.palette, 'the palette is dormant on boot').toBe(0)
  expect(before.layers, 'nothing is pushed over the list').toBe(0)
  const scroller = page.locator('.pane:not(.off) main')
  const y0 = await scroller.evaluate((el) => el.scrollTop)
  // Tour's first move is await wait(2200) then a scroll. Stay past that.
  await page.waitForTimeout(3500)
  expect(await state()).toEqual(before)
  expect(await scroller.evaluate((el) => el.scrollTop)).toBe(y0)
})

test('viewport geometry at 402×874', async ({ page }) => {
  const report = await walkAll(page)
  expect(report.map((r) => r.label)).toEqual([
    'issues',
    'scope-sheet',
    'detail',
    'detail-sheet',
    'detail-writes-off',
    'docs',
    'page-detail',
    'search-empty',
    'search-results',
    'pairing',
    'issues-dark',
  ])
  /*
   * GDK-902 2026-09-15 — the root has no exit, and now it says so.
   *
   * Every screen used to pass `hasEscape` because the tab bar was on it,
   * which made the assertion vacuous exactly where it mattered. DESIGN.md
   * §2's entry/exit table has always ended "the root has no exit": the
   * list, under whichever scope, IS the root. So the three root readings
   * are named here and the assertion becomes real for the other eight —
   * a palette, a layer or a detail that shipped without a visible way out
   * now fails, where before nothing could.
   */
  const ROOT_LABELS = new Set(['issues', 'docs', 'issues-dark'])
  for (const row of report) {
    expect(row.hOverflow, `${row.label} horizontal overflow`).toBe(0)
    expect(row.navBottomFlush, `${row.label} column flush to the bottom edge`).toBe(0)
    expect(row.inputsUnder16, `${row.label} inputs under 16px`).toEqual([])
    expect(
      row.hasEscape,
      `${row.label} visible escape: ${JSON.stringify(row.escapes)}`,
    ).toBe(!ROOT_LABELS.has(row.label))
  }
  const issues = report.find((r) => r.label === 'issues')
  expect(issues, 'issues measurement').toBeTruthy()
  // Why the density number moved is invisible from the assertion alone, so
  // print the geometry it is derived from every run (GDK-1543 debuggability).
  for (const r of report) {
    if (r.rowH !== null) {
      const spread = r.rowHSpread ? ` (median of ${r.rowCount}, ${r.rowHSpread.min}–${r.rowHSpread.max})` : ''
      console.log(`[viewport] ${r.label}: rowH ${r.rowH}px${spread} → ${r.rowsPerScreen}/screen (main ${r.mainH}px)`)
    }
  }
  // GDK-1543, 2026-09-07 — re-derived, not loosened.
  //
  // The floor of 12 was set when every issue row was one line of title over
  // one meta line — 59.58px measured, which is what a page row still is (see
  // the `docs` reading printed above). GDK-1543 spends part of that height
  // on legibility: on the demo fixture 11 of the 12 first-screen summaries
  // were cut, and moving the date off the title line recovered only one of
  // them (11/12 → 10/12 at 370px of title, measured by a4-captures). So the
  // summary clamps to two lines, and a row whose title uses both measures
  // 83.77px → 9 per screen.
  //
  // This is a floor on the WORST case, not a number chosen to pass: the
  // clamp is at 2, so no issue row can be taller than that 83.77px, and a
  // row whose title fits one line still measures 59.58px → 12 (the
  // `issues-dark` reading above is exactly that). Pages are untouched —
  // DocRow did not change and keeps the old floor of 12.
  //
  // FAIL-first (this file, unmodified, against the GDK-1543 Row.svelte):
  //   Error: issues rows per screen … Expected: >= 12, Received: 9
  const ISSUE_ROWS_PER_SCREEN = 9
  expect(issues!.rowsPerScreen, 'issues rows per screen').toBeGreaterThanOrEqual(
    ISSUE_ROWS_PER_SCREEN,
  )
  const docs = report.find((r) => r.label === 'docs')
  expect(docs, 'docs measurement').toBeTruthy()
  expect(docs!.rowsPerScreen, 'docs rows per screen').toBeGreaterThanOrEqual(12)
  const search = report.find((r) => r.label === 'search-results')
  expect(search, 'search-results measurement').toBeTruthy()
  // GDK-1550, 2026-09-11 — the one plate this gate never floored.
  //
  // The floor is the worst case of the SAME row grammar the issues floor
  // pins (GDK-1543): a Row clamps its summary at two lines — 83.77px, the
  // median of this run's own rows — but the search pane pays the query
  // field block out of the same main. Measured: the issues main is 757px
  // and 9 two-line rows fill it with 3px slack; the search main is 700px
  // (the field block's 57px), and floor(700/83.77) = 8 — so the search
  // bound is one row short of the issues floor, 8, with 32px of slack
  // before it would fall to 7. A one-line answer still reads 12; 8 is the
  // all-two-line bound, not a number chosen to pass. If the field grows
  // chrome, or the row grammar gets taller, this trips here first.
  //
  // GDK-902 2026-09-15 — the arithmetic moved, the floor did not. The two
  // mains are one main now (the palette is the list's own body), and it
  // grew to 822px when the tab bar left, so this run reads 9 where it read
  // 8. The floor stays at 8: it is the bound this plate is allowed to
  // reach, not a record of what it currently measures.
  // FAIL-first (asserting the issues floor's 9 here, same source):
  //   Error: search-results rows per screen … Expected: >= 9, Received: 8
  expect(search!.rowsPerScreen, 'search-results rows per screen').toBeGreaterThanOrEqual(8)
  // GDK-911: a sheet inside .detail-layer is the bottom-most painted
  // surface and clears the home indicator by the same formula .safe-bottom
  // gives the tab bar — max(reported inset, floor), the number owned by
  // sheetBottomInset and pinned to app.css by inset.test.ts. The walk's
  // detail-sheet row is the assignee picker, a sheet actually open in that
  // layer. (GDK-902 2026-09-15: the scope-sheet row used to be the same
  // Sheet component inside .tabs, exempt because the tab bar paid its
  // inset. It is not a sheet at all now — see the null assertion below —
  // and app.css gives every remaining sheet this same formula.)
  const detailSheet = report.find((r) => r.label === 'detail-sheet')
  expect(detailSheet, 'detail-sheet measurement').toBeTruthy()
  expect(detailSheet!.sheetInsetPx, 'detail-layer sheet bottom inset').toBe(
    sheetBottomInset(detailSheet!.safeBottomPx),
  )
  // GDK-902 2026-09-15: the owner list is not a sheet any more — it is the
  // list's own body — so there is no panel here to owe an inset. The claim
  // that reading made ("a picker inside the column pays nothing of its
  // own") is now structural: the column has no second surface to pay for.
  // What is asserted instead is that no sheet is open at this step at all.
  const scopeSheet = report.find((r) => r.label === 'scope-sheet')
  expect(scopeSheet!.sheetInsetPx, 'the owner list is the body, not a sheet').toBeNull()
  // Same debuggability stance as the rowH print above: the axis this gate
  // owns, printed every run so a moved number explains itself.
  console.log(
    `[viewport] detail-sheet inset ${detailSheet!.sheetInsetPx}px (safe-bottom ${detailSheet!.safeBottomPx}px, floor ${SHEET_INSET_FLOOR_PX}px)`,
  )
  // GDK-885: opening the picker must not cost the list its density.
  // GDK-902 2026-09-15: and it is the same `main` now, so this also floors
  // the restore — a palette that left the body a row shorter trips here.
  const afterSheet = report.find((r) => r.label === 'issues-dark')
  expect(afterSheet!.rowsPerScreen, 'rows per screen after the picker closed').toBeGreaterThanOrEqual(
    ISSUE_ROWS_PER_SCREEN,
  )
  // Recurrence: a page row has no status spine (DESIGN.md §3.4).
  expect(await page.locator('[data-testid="doc-row"] .spine').count()).toBe(0)
})

test('the keyboard band (?kb=300) is one number the scroller and the sheet both honor', async ({ page }) => {
  // GDK-1971 — the covered band gets one owner, and this gate pins the two
  // surfaces that must read it. The band cannot be raised in Playwright
  // (Chromium has no software keyboard), so the rig boots with the ?kb=300
  // probe — the same URL-param idiom ?hosted uses — and lib/keyboard.ts
  // answers 300px from it exactly as it would from the VisualViewport.
  //
  // All three measurements are taken and printed BEFORE any assertion, so a
  // red run names every number: the FAIL-first run of this test on the
  // unmodified tree printed var "" and both edges at ~874 against a band top
  // of 574.
  await page.goto('/?kb=300', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  // (a) the root variable: bindKeyboardBand wrote the band on #app, where
  // every scroller and the sheet panel read it back as --keyboard-inset.
  const kbVar = await page.evaluate(() =>
    getComputedStyle(document.getElementById('app')!).getPropertyValue('--keyboard-inset').trim(),
  )

  // (b) the palette's LAST row must be scrollable to above the band: Screen's
  // main pays the band as padding-bottom, so a fully-scrolled list leaves the
  // last row's bottom edge at the band's top rather than at the screen edge.
  await openPalette(page)
  const rowBottom = await page.evaluate(() => {
    const main = document.querySelector('.pane:not(.off) main') as HTMLElement
    main.scrollTop = main.scrollHeight
    const rows = [...main.querySelectorAll('.palette-row, .desk-row, .recent, .more')]
    const last = rows[rows.length - 1]
    const r = last.getBoundingClientRect()
    return Math.round((r.y + r.height) * 10) / 10
  })

  // (c) the create sheet's panel must clear the band: the create control is
  // the list header's button.new (Issues.svelte), the panel is Sheet's
  // .sheet, and keyboardInset translates it up by the same number.
  await page.locator('button.palette-cancel').click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await page.locator('button.new').click()
  await page.locator('.sheet').first().waitFor()
  await settleSheet(page)
  const sheetBottom = await page.evaluate(() => {
    const r = document.querySelector('.sheet')!.getBoundingClientRect()
    return Math.round(r.bottom * 10) / 10
  })

  // The band's top edge on this rig: innerHeight − 300. The +1 is DSF-3
  // fraction hygiene (settleSheet's note: a settled 44px control measures
  // 43.9999…), not slack — the defect this gate exists for is 300px wide.
  const bandTop = await page.evaluate(() => window.innerHeight - 300)
  console.log(
    `[kb] var ${JSON.stringify(kbVar)} | last palette row bottom ${rowBottom} | create panel bottom ${sheetBottom} | band top ${bandTop}`,
  )
  expect(kbVar, 'the probe wrote --keyboard-inset on #app').toBe('300px')
  expect(rowBottom, 'the last palette row scrolls above the band').toBeLessThanOrEqual(bandTop + 1)
  expect(sheetBottom, 'the create sheet panel clears the band').toBeLessThanOrEqual(bandTop + 1)
})

test('no visible button is under 44pt', async ({ page }) => {
  // GDK-867: 44pt floor on every visible button. FAIL-first on unmodified
  // source (2026-08-25): failed at the list screen with buttonsUnder44pt=1
  // [{h:32, cls:"fresh"}] (first screen; the rest were not reached). The
  // four 32pt chips shared --spacing-control-sm as a tap size; the owner
  // is now button { min-height: var(--spacing-control) } in app.css.
  const report = await walkAll(page)
  for (const row of report) {
    expect(row.buttonsUnder44pt, `${row.label} buttons under 44pt: ${JSON.stringify(row.under44)}`).toBe(0)
  }
})
