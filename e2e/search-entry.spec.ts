import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { attachConsoleErrors, gotoApp, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

/*
 * Ways in to the one search.
 *
 * The mirror has held issues and pages together for a while, and the server has
 * searched both since the unified index landed — but the client only ever
 * offered that on the issue list. The palette matched issues alone, `/` was
 * bound inside the list's search box (so it died on a document screen), and the
 * document screens had no narrowing field at all. These pin the three entry
 * points, and that none of them costs a request on a keystroke.
 *
 * GDK-1790 — how "no request on a keystroke" is measured. The palette's server
 * search is debounced (web/src/lib/unified-search.ts, UNIFIED_DEBOUNCE_MS =
 * 250): seven keystrokes re-arm one timer, and ONE search fires 250ms after
 * the last key — by design. So the claim is about the typing window, not about
 * wall time after it: an assertion that reads the request log late (a loaded
 * machine stretched the assertion chain past 250ms — reproduced 6/80 under
 * 2.4x CPU oversubscription, one request, never a burst) sees the legitimate
 * debounced request and calls it a leak. The clock is installed before boot
 * and paused for the typing window, advancing 20ms of fake time per keystroke,
 * so "nothing fired while typing" is a state, not a stopwatch; the same burst
 * is then fast-forwarded past the window to pin the other half — it collapses
 * into exactly one request that carries the final query.
 */

/** Open the tabbed Documents view from the sidebar. */
async function openDocuments(page: Page): Promise<void> {
  await page.getByTestId('docs-documents').click()
  await expect(page.getByTestId('docs-view')).toBeVisible()
}

/** Let the boot chatter (bootstrap / write-meta / pages / focus-time pull)
 *  finish before a spec starts counting requests. Deliberately not networkidle:
 *  the app polls for a delta every 15s, so "quiet for 500ms" only becomes true
 *  after that fires. */
async function settled(page: Page): Promise<void> {
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  // Same observable ux-p1.spec.ts uses. The chip lives on ListView; documents
  // unmount that column, so the sidebar row (same mirrorBusy sentence) is the
  // stand-in — it stays mounted on every main-column screen.
  const chip = page.getByTestId('freshness-chip')
  if ((await chip.count()) > 0) {
    await expect(chip).not.toHaveAttribute('data-state', 'syncing', { timeout: 30_000 })
    return
  }
  // GDK-460: the sidebar row no longer repeats the chip's busy sentence, so
  // "does not contain Syncing" would pass while a pull is still running.
  // data-state is the same flag the chip uses.
  await expect(page.getByTestId('sidebar-sync-now')).not.toHaveAttribute('data-state', 'syncing', {
    timeout: 30_000,
  })
}

/** One /api/ hit, with when it fired (Node wall time, ms since the sink was
 *  attached). GDK-1790: a future failure should read as "fired at +812ms" —
 *  after the typing window, i.e. the debounce — or "at +96ms", mid-typing,
 *  i.e. a real leak — without anyone re-deriving the timeline. */
interface ApiHit {
  line: string
  atMs: number
}

function recordApiDuringType(page: Page, sink: ApiHit[]): void {
  const t0 = Date.now()
  page.on('request', (req) => {
    const url = req.url()
    if (!url.includes('/api/') || url.includes('/ui-focus/')) return
    let path = url
    try {
      path = new URL(url).pathname
    } catch {
      /* keep raw */
    }
    sink.push({ line: `${req.method()} ${path}`, atMs: Date.now() - t0 })
  })
}

function hitLines(sink: ApiHit[]): string[] {
  return sink.map((h) => h.line)
}

function hitReport(sink: ApiHit[]): string {
  return sink.map((h) => `${h.line} @+${h.atMs}ms`).join(', ') || '(none)'
}

test.describe('search entry points', () => {
  test('the palette matches documents, above the issues, and says how many it hid', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    // Before boot, so the app's timers are fake from the moment they are armed
    // (keys-focus.spec.ts's lesson: a mid-test install leaves boot-era timers
    // on real native handles). The delta poll lives on the same fake clock.
    await page.clock.install()
    await gotoApp(page)
    await settled(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    // pauseAt takes an ABSOLUTE time, and the page's fake clock only loosely
    // tracks Node's real clock — under load, CDP latency can leave a Node-side
    // `new Date()` behind fake-now, and pauseAt refuses to move to the past
    // (reproduced 4/80 under 2.4x oversubscription). Read fake now from the
    // page and pause shortly after it; the small jump fires whatever was
    // about to fire anyway, which is why the request sink attaches only after
    // the pause, at the exact moment the typing window opens.
    const fakeNow = await page.evaluate(() => Date.now())
    await page.clock.pauseAt(fakeNow + 1000)

    const apiDuringType: ApiHit[] = []
    recordApiDuringType(page, apiDuringType)

    // Deterministic typing (GDK-1790): the clock is paused and advances ONLY
    // with the keystrokes — 20ms of fake time per key, the inter-key delay the
    // old wall-clock test used. No timer the app arms can outpace the typing,
    // because none of this depends on the machine's load; and a timer armed
    // *per keystroke* (a debounce regression — setTimeout(..., 0)) fires inside
    // its own 20ms step, ahead of the next key's cancel, exactly as it would
    // for a real typist. Pure freezing would hide that class.
    // Seven pages in the mirror are runbooks; four of them fit the section.
    for (const ch of 'runbook') {
      await page.keyboard.type(ch)
      await page.clock.fastForward(20)
    }

    const docRows = palette.getByTestId('palette-doc-row')
    await expect(docRows).toHaveCount(4)
    await expect(docRows.first()).toContainText('Runbook')
    // The space rides along, because two pages can share a title and the space
    // is what tells them apart. (This mirror has no space names yet, so the row
    // shows the key — the same fallback the document rows use.)
    await expect(docRows.first()).toContainText('ENG')

    // Capped, and it says so — four rows with no total read as the whole answer.
    // Spelled out, not "4 / 7": the slash fraction belongs to the document
    // screens' filter (shown / total), and the palette's truncation must not
    // wear the same glyph with a different meaning.
    await expect(palette.getByTestId('palette-doc-count')).toHaveText('4 of 7')

    // A row shows the part of the title the query found, so it never asks to be
    // taken on trust.
    await expect(docRows.first().locator('mark', { hasText: /runbook/i })).toBeVisible()

    // Documents lead: when someone types words rather than a key, the page is
    // often what they came for, and the issues section would bury it.
    const sections = await palette
      .getByTestId('palette-section')
      .evaluateAll((els) => els.map((el) => el.getAttribute('data-section')))
    expect(sections).toContain('doc')
    expect(sections).toContain('issue')
    expect(sections.indexOf('doc')).toBeLessThan(sections.indexOf('issue'))

    expect(
      hitLines(apiDuringType),
      `in-flight /api/ while typing (must be none): ${hitReport(apiDuringType)}`,
    ).toEqual([])

    // The other half of the contract, made positive: the same seven keystrokes
    // collapse into exactly ONE server search, and only the debounce window
    // elapsing can arm it (unified-search.ts fires 250ms after the last
    // request()). fastForward owns the elapse, so the machine's load owns
    // nothing. Scoped to /search/ on purpose: the 15s delta poll rides the
    // same fake clock, and if its deadline happens to fall inside this 300ms
    // that is product behavior orthogonal to the debounce under test.
    await page.clock.fastForward(300)
    await expect
      .poll(() => hitLines(apiDuringType).filter((l) => l.includes('/search/')).length, {
        timeout: 5_000,
        message: 'the debounced palette search never fired',
      })
      .toBe(1)
    expect(hitLines(apiDuringType).filter((l) => l.includes('/search/'))).toEqual([
      'GET /api/v1/issues/search/',
    ])
    await page.clock.resume()

    // Choosing one opens the page, the same as it always did from the recent list.
    await docRows.first().click()
    await expect(palette).toBeHidden()
    await expect(page.getByTestId('doc-panel')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('/ reaches the narrowing field on a document screen, not only on the list', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // The list is where it always worked; assert it still does after the binding
    // moved out of the search box and into the shell.
    await page.keyboard.press('/')
    await expect(page.getByTestId('search-input')).toBeFocused()
    await page.keyboard.press('Escape')

    await openDocuments(page)
    await page.keyboard.press('/')
    await expect(page.getByTestId('docs-filter-input')).toBeFocused()

    // Esc clears what is typed before it gives the keyboard back — SearchBox's
    // contract, and the reason the field is safe to leave text in.
    await page.keyboard.type('runbook')
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('docs-filter-input')).toHaveValue('')
    await expect(page.getByTestId('docs-filter-input')).toBeFocused()

    // A space screen has the same field, reached the same way.
    await page.getByTestId('docs-spaces').click()
    await page.getByTestId('docs-section').getByTestId('docs-space').filter({ hasText: 'ENG' }).click()
    await expect(page.getByTestId('space-docs-view')).toBeVisible()
    await page.keyboard.press('/')
    await expect(page.getByTestId('docs-filter-input')).toBeFocused()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('typing in the document filter narrows locally, with zero /api/ traffic', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    // Same measurement discipline as the palette test above (GDK-1790): the
    // filter itself never fetches (DocsFilter — local narrowing, Enter is the
    // only way out to the server), so the one thing that could pollute the
    // read is ambient traffic — the 15s delta poll — landing between typing
    // and the log read on a slow machine. Frozen time closes that window.
    await page.clock.install()
    await gotoApp(page)
    await openDocuments(page)
    await page.getByTestId('docs-view').getByTestId('docs-tab').filter({ hasText: 'Updated' }).click()
    await settled(page)

    const view = page.getByTestId('docs-view')
    await expect(view.getByTestId('docs-count')).toHaveText('71')
    const before = await view.getByTestId('doc-row').count()
    expect(before).toBeGreaterThan(0)

    const filter = page.getByTestId('docs-filter-input')
    await filter.click()
    // Same fake-now discipline as the palette test above: pause shortly after
    // the page's own clock reading (a Node-side `new Date()` can be behind
    // fake-now under load → "Cannot fast-forward to the past"), and count
    // requests only from the moment time froze.
    const fakeNow = await page.evaluate(() => Date.now())
    await page.clock.pauseAt(fakeNow + 1000)

    const apiDuringType: ApiHit[] = []
    recordApiDuringType(page, apiDuringType)

    await filter.pressSequentially('runbook', { delay: 20 })

    // The count becomes a fraction: what is left, out of what the tab holds.
    await expect(view.getByTestId('docs-count')).toHaveText('7 / 71')
    await expect(view.getByTestId('doc-row')).toHaveCount(7)
    expect(await view.getByTestId('doc-row').count()).toBeLessThan(before)

    expect(
      hitLines(apiDuringType),
      `in-flight /api/ while filtering (must be none): ${hitReport(apiDuringType)}`,
    ).toEqual([])
    await page.clock.resume()

    // Nothing matched is a state with a way out of it, not a dead end.
    await filter.fill('zzzznotathing')
    await expect(view.getByTestId('doc-row')).toHaveCount(0)
    await expect(view).toContainText('No documents match')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Enter in the document filter leaves for the whole mirror', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openDocuments(page)

    const filter = page.getByTestId('docs-filter-input')
    await filter.click()
    await filter.fill('runbook')
    await filter.press('Enter')

    // Unified results have one home — the issue list's search section, where the
    // page hits sit above the issues. The document screen hands over to it.
    await expect(page.getByTestId('docs-view')).toHaveCount(0)
    await expect(page.getByTestId('search-docs')).toBeVisible()
    // The server ranks these (a page whose body says "runbooks" can outrank a
    // title), so the claim is that the runbooks are in the group, not first.
    await expect(
      page.getByTestId('search-doc-row').filter({ hasText: 'Runbook —' }).first(),
    ).toBeVisible()
    // The query it left with is the query the list is now showing.
    await expect(page.getByTestId('search-input')).toHaveValue('runbook')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
