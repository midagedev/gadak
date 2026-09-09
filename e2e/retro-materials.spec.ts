import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { apiURL, attachConsoleErrors, gotoApp } from './helpers'

const here = dirname(fileURLToPath(import.meta.url))

/*
 * The retro's materials (GDK-1721..1726).
 *
 * Two halves, and they use the server differently on purpose.
 *
 * The behaviour half runs against the real e2e serve and skips itself when
 * that server does not send the new fields: the Go side of this round lands
 * separately, and a spec that fails until it does would be red for a reason
 * that is not a defect. When it lands these turn on by themselves — the
 * condition is `aging` in the payload, not a date or a flag.
 *
 * The capture half injects the contract JSON through `page.route`, because a
 * screenshot has to show the sections filled and the demo mirror has no
 * reading history to fill them with. Injection is confined to the captures
 * for exactly that reason: a behaviour assertion standing on a stub is a
 * test of the stub.
 */

const SHOT_DIR = process.env.RETRO_SHOT_DIR ?? join(here, '../scratch')

type Doc = Record<string, unknown>

/*
 * Three titles the layout has to survive (GDK-1737, GDK-1738). The epic one
 * is past sixty characters because the demo mirror's longest is thirty-four:
 * a fixed 8rem column truncated nothing there and everything on a real site.
 */
const LONG_EPIC_TITLE =
  'Search relevance rebuild for the multilingual catalogue and its long tail'
const SURPRISE_TITLE = 'The importer dropped every issue whose sprint field was empty'
const POINT_TITLE = 'Resolved inside the window'

/** The report the real server is sending right now. */
async function fetchDoc(page: Page): Promise<Doc> {
  const res = await page.request.get(apiURL('/api/v1/issues/retro/?since=4w'))
  expect(res.ok()).toBe(true)
  return (await res.json()) as Doc
}

/**
 * The contract from the shared spec, laid over whatever the fixture returns.
 *
 * Real buckets, real dates, invented materials: the geometry under test is
 * "does a bar past p85 read as past p85", and that needs a spread the demo
 * mirror does not happen to have. The dates are derived from each bucket's
 * own window so the density strip and the scatter land inside their boxes.
 */
function withMaterials(body: Doc): Doc {
  const buckets = (body.buckets ?? []) as Record<string, unknown>[]
  const kinds = ['created', 'started', 'resolved', 'comment', 'sprint_in'] as const
  buckets.forEach((b, bi) => {
    const from = Date.parse(String(b.from))
    const day = 86_400_000
    const events: unknown[] = []
    // A rhythm rather than a flat week: quiet Monday, a spike mid-week, and
    // one day with nothing at all — the shape the strip exists to show.
    const perDay = [1, 0, 4, 2, 0, 6, 1]
    perDay.forEach((n, d) => {
      for (let i = 0; i < n; i++) {
        events.push({
          at: new Date(from + d * day + (8 + i) * 3_600_000).toISOString(),
          key: `NMS-${bi * 10 + d * 2 + i}`,
          kind: kinds[(d + i) % kinds.length],
          detail: '',
        })
      }
    })
    b.events = events
    // Titles ride with the keys (GDK-1737). One of them is long on purpose:
    // the row has to truncate rather than push the kind label off the line.
    b.surprises = [
      { kind: 'reopened', key: 'NMS-12', summary: SURPRISE_TITLE, detail: 'the fix did not hold on staging' },
      { kind: 'reversal', key: 'NMS-31', summary: 'Retry the webhook once', detail: '5' },
      { kind: 'added_after_start', key: 'NMS-44', summary: 'Add a status filter to the board', detail: new Date(from + 2 * day).toISOString() },
      { kind: 'carried', key: 'NMS-58', summary: 'Carry the sprint field through import', detail: '2' },
    ]
    b.closed_by_type = [
      { issue_type_id: '10004', issue_type: 'Bug', count: 6, keys: ['NMS-1', 'NMS-2'] },
      { issue_type_id: '10001', issue_type: 'Story', count: 3, keys: ['NMS-3'] },
      { issue_type_id: '10002', issue_type: 'Task', count: 1, keys: ['NMS-4'] },
    ]
    b.closed_by_epic = [
      { epic_key: 'NMS-9', title: LONG_EPIC_TITLE, count: 5, keys: ['NMS-1'] },
      { epic_key: '', title: '', count: 4, keys: ['NMS-4'] },
      { epic_key: 'NMS-17', title: 'Billing migration', count: 1, keys: ['NMS-2'] },
    ]
    b.unplanned = { count: 2, keys: ['NMS-4', 'NMS-5'] }
    // The closed count too, so the injected report is internally consistent:
    // the running week in the demo mirror has closed nothing, and a frame
    // reading "Closed 0 (2 unplanned)" is a capture nobody can judge.
    b.closed = 10
    b.keys = { ...(b.keys as Record<string, unknown>), closed: ['NMS-1', 'NMS-2', 'NMS-3'] }
    b.cycle_points = [0.4, 0.9, 1.2, 1.4, 2.1, 2.3, 3.0, 4.6, 5.2, 11.8].map((days, i) => ({
      key: `NMS-${20 + i}`,
      summary: `${POINT_TITLE} ${i}`,
      resolved_at: new Date(from + (i % 7) * day + 5 * 3_600_000).toISOString(),
      days,
    }))
    b.seen_not_moved = { count: 3, keys: ['NMS-6', 'NMS-7', 'NMS-8'] }
    b.moved_not_seen = { count: 9, keys: ['NMS-9'] }
  })
  body.aging = {
    p85_days: 22.5,
    items: [41.5, 33.2, 26.8, 22.5, 18.1, 14.0, 9.6, 7.2, 5.5, 3.1, 2.4, 1.2].map((days, i) => ({
      key: `NMS-${100 + i}`,
      days,
      summary: `Something that has been open for ${Math.round(days)} days`,
      issue_type_id: '10004',
    })),
  }
  body.actions = [
    {
      key: 'NMS-70',
      summary: 'Split the review step so a change waits on one person, not three',
      status_category: 'done',
      created_at: new Date(Date.now() - 21 * 86_400_000).toISOString(),
      resolved_at: new Date(Date.now() - 3 * 86_400_000).toISOString(),
      metric: 'wip age max',
      then: 12.4,
      now: 9.1,
    },
    {
      key: 'NMS-71',
      summary: 'Stop starting a second thing while the first is in review',
      status_category: 'inprogress',
      created_at: new Date(Date.now() - 14 * 86_400_000).toISOString(),
      resolved_at: null,
      metric: 'in progress',
      then: 11,
      now: 11,
    },
  ]
  return body
}

async function stubMaterials(page: Page): Promise<void> {
  await page.route('**/api/v1/issues/retro/**', async (route) => {
    // A reload mid-flight disposes the fetched response before it is read
    // (measured once on 2026-09-09, "apiResponse.json: Response has been
    // disposed", in the explanations test which reloads twice). The request
    // that died with its page has no reader; let it go rather than fail
    // the test on the handler's own error.
    try {
      const res = await route.fetch()
      await route.fulfill({ response: res, json: withMaterials((await res.json()) as Doc) })
    } catch {
      await route.abort().catch(() => {})
    }
  })
}

/** Locale and theme before the app boots — both are read out of storage. */
async function prepare(page: Page, locale: string, theme: string): Promise<void> {
  await page.addInitScript(
    ([loc, th]) => {
      try {
        localStorage.setItem('gadak_locale', loc)
        localStorage.setItem('gadak:theme', th)
      } catch {
        /* private mode */
      }
    },
    [locale, theme],
  )
}

test.describe('retro materials', () => {
  test('the sections stand on the real report, or stand down when the server has none', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const doc = await fetchDoc(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-view')).toBeVisible()

    // The contract that holds on every server: the report still renders, and
    // its complete table is one click away rather than gone.
    await expect(page.getByTestId('retro-summary')).toBeVisible()
    await expect(page.getByTestId('retro-table')).toBeHidden()
    await page.getByTestId('retro-table-toggle').click()
    await expect(page.getByTestId('retro-table')).toBeVisible()

    test.info().skip(
      !doc.aging,
      'this server sends no aging block — the Go half of GDK-1721 has not landed',
    )

    // From here on the server fills the materials, so the sections must be
    // on the screen and must be doors.
    await expect(page.getByTestId('retro-sentence')).toBeVisible()
    await expect(page.locator('[data-testid="retro-section"][data-section="aging"]')).toBeVisible()
    await expect(page.getByTestId('retro-aging-row').first()).toBeVisible()

    const value = page.locator('[data-testid="retro-sentence-value"][data-slot="closed"]')
    await expect(value).toBeVisible()
    await value.click()
    await expect(page.getByTestId('retro-view')).toBeHidden()
    await expect.poll(() => page.url()).toContain('ks=')

    expect(errors.filter((e) => !e.includes('409') && !e.includes('502'))).toEqual([])
  })

  test('the sections read the injected contract, and every count is a door', async ({ page }) => {
    await stubMaterials(page)
    await gotoApp(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-view')).toBeVisible()

    // The sentence, with its four values as their own elements.
    const sentence = page.getByTestId('retro-sentence')
    await expect(sentence).toBeVisible()
    await expect(sentence.getByTestId('retro-sentence-value')).toHaveCount(4)
    await expect(
      sentence.locator('[data-slot="unplanned"]'),
    ).toHaveText('2')
    // One reopened surprise in the stub, and the sentence counts it.
    await expect(sentence.locator('[data-slot="reopened"]')).toHaveText('1')

    // Aging: twelve items, the four past p85 marked and nothing else.
    await expect(page.getByTestId('retro-aging-row')).toHaveCount(12)
    await expect(page.locator('[data-testid="retro-aging-row"][data-over="1"]')).toHaveCount(3)
    // Counted, not "visible": a vertical <line> has a zero-width box and
    // Playwright reads that as hidden.
    await expect(page.getByTestId('retro-aging-p85')).toHaveCount(1)

    // The strip is the running week, so it is as many columns as that week
    // has had days — one to seven, never zero and never the whole future
    // week. The day grid itself is pinned in materials.test.ts, where the
    // clock is an argument rather than the wall.
    const dayCount = await page.getByTestId('retro-density-day').count()
    expect(dayCount).toBeGreaterThan(0)
    expect(dayCount).toBeLessThanOrEqual(7)
    await expect(page.getByTestId('retro-surprise')).toHaveCount(4)

    // GDK-1737: a surprise names the work, not just the key, and the whole
    // title is on the element even when the row truncates it.
    const surpriseTitle = page.getByTestId('retro-surprise-title').first()
    await expect(surpriseTitle).toHaveText(SURPRISE_TITLE)
    await expect(surpriseTitle).toHaveAttribute('title', SURPRISE_TITLE)

    // …and so does an aging row: the title is beside the key now rather than
    // only inside the row's tooltip.
    const agingTitle = page.getByTestId('retro-aging-title').first()
    await expect(agingTitle).toHaveText(/^Something that has been open for /)

    // A scatter dot carries key, title and days in its tooltip.
    await expect(page.getByTestId('retro-cycle-point').first()).toHaveAttribute(
      'title',
      /^NMS-20 · Resolved inside the window 0 · /,
    )

    // Closed: two cuts and a scatter with both percentile lines.
    await expect(page.locator('[data-testid="retro-closed-group"][data-group="type"]')).toBeVisible()
    await expect(page.locator('[data-testid="retro-closed-group"][data-group="epic"]')).toBeVisible()

    // GDK-1738: the epic label is no longer a fixed 8rem box. The full title
    // is the tooltip, the epic key rides behind it so a truncated row is
    // still identifiable, and the rendered box is wider than the old 128px.
    const epicRow = page
      .locator('[data-testid="retro-closed-group"][data-group="epic"] [data-testid="retro-closed-label"]')
      .first()
    await expect(epicRow).toHaveAttribute('title', `${LONG_EPIC_TITLE} · NMS-9`)
    await expect(epicRow.getByTestId('retro-closed-epic-key')).toHaveText('NMS-9')
    const labelBox = await epicRow.boundingBox()
    expect(labelBox!.width).toBeGreaterThan(128)
    await expect(page.getByTestId('retro-cycle-point')).toHaveCount(10)
    await expect(page.getByTestId('retro-cycle-line')).toHaveCount(2)

    // Decided last time, with the number it named then and now.
    await expect(page.getByTestId('retro-action')).toHaveCount(2)
    await expect(page.getByTestId('retro-action-step').first()).toContainText('9.1d')

    // Seen and moved: two counts, both doors.
    await expect(page.locator('[data-testid="retro-seen-count"][data-kind="seen-not-moved"]')).toContainText('3')

    // A door is a door: the unplanned count puts its issues on the list.
    await page.getByTestId('retro-unplanned').click()
    await expect(page.getByTestId('retro-view')).toBeHidden()
    await expect.poll(() => page.url()).toContain('ks=')
  })

  test('the explanations are folded until asked for, except the first time', async ({ page }) => {
    await stubMaterials(page)
    await gotoApp(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-view')).toBeVisible()

    // First visit: one paragraph, the aging one, and nothing else
    // (THEORY.md G3 — speak at the boundary, once).
    await expect(page.getByTestId('retro-explain')).toHaveCount(1)
    await expect(page.locator('[data-testid="retro-explain"][data-section="aging"]')).toBeVisible()

    // The definitions toggle unfolds all of them, table included.
    await page.getByTestId('retro-defs-toggle').click()
    expect(await page.getByTestId('retro-explain').count()).toBeGreaterThanOrEqual(5)

    // And the fold survives a reload, while the first-visit paragraph does not.
    await page.getByTestId('retro-defs-toggle').click()
    await page.reload()
    await expect(page.getByTestId('retro-view')).toBeVisible()
    await expect(page.getByTestId('retro-explain')).toHaveCount(0)
  })

  test('the table fold is remembered', async ({ page }) => {
    await gotoApp(page)
    await page.goto('/#/?retro=1')
    await expect(page.getByTestId('retro-table')).toBeHidden()
    await page.getByTestId('retro-table-toggle').click()
    await expect(page.getByTestId('retro-table')).toBeVisible()
    await page.reload()
    await expect(page.getByTestId('retro-view')).toBeVisible()
    await expect(page.getByTestId('retro-table')).toBeVisible()
  })
})

/*
 * Capture-only. Six frames — three locales against two themes — plus the
 * explanations unfolded and a close-up of the aging chart, which is the one
 * section whose verdict is about a threshold rather than a layout.
 */
test.describe('retro materials captures', () => {
  test('capture', async ({ page }) => {
    test.skip(!process.env.RETRO_SHOT_DIR, 'capture-only; set RETRO_SHOT_DIR to run')
    mkdirSync(SHOT_DIR, { recursive: true })
    // Tall on purpose: the report scrolls inside a fixed app layout, so
    // `fullPage` cannot reach below the window and the lower sections would
    // never appear in a frame.
    const VIEW = { width: 1280, height: 1800 }
    await page.setViewportSize(VIEW)

    for (const theme of ['light', 'dark']) {
      for (const locale of ['en', 'ko', 'ja']) {
        const ctx = await page.context().browser()!.newContext({ viewport: VIEW })
        const p = await ctx.newPage()
        await p.setViewportSize(VIEW)
        await prepare(p, locale, theme)
        await stubMaterials(p)
        // Straight to the view rather than through gotoApp: that helper
        // waits on the English pool label, which a Korean or Japanese boot
        // never prints. Waiting on the sentence is the same guarantee for
        // this screen — it is the last thing the report renders.
        await p.goto('/#/?retro=1')
        await expect(p.getByTestId('retro-sentence')).toBeVisible()
        await expect(p.getByTestId('retro-aging-chart')).toBeVisible()
        // The theme, stamped after boot. Storage alone does not hold it: the
        // app hydrates appearance from the server's settings and this serve
        // is light, so a dark frame asked for through localStorage came back
        // light (measured on the first capture round). `data-theme` is the
        // override the picker itself writes.
        await p.evaluate((th) => {
          if (th === 'light') document.documentElement.removeAttribute('data-theme')
          else document.documentElement.setAttribute('data-theme', th)
        }, theme)
        await p.screenshot({
          path: join(SHOT_DIR, `retro-${locale}-${theme}.png`),
          fullPage: true,
          animations: 'disabled',
        })
        if (locale === 'en' && theme === 'light') {
          // Everything unfolded — the weight test for GDK-1726.
          await p.getByTestId('retro-defs-toggle').click()
          await p.getByTestId('retro-table-toggle').click()
          await expect(p.getByTestId('retro-table')).toBeVisible()
          await p.screenshot({
            path: join(SHOT_DIR, 'retro-explained.png'),
            fullPage: true,
            animations: 'disabled',
          })
          await p.getByTestId('retro-defs-toggle').click()
          await p.getByTestId('retro-table-toggle').click()
          await p
            .locator('[data-testid="retro-section"][data-section="aging"]')
            .screenshot({ path: join(SHOT_DIR, 'retro-aging-closeup.png'), animations: 'disabled' })
        }
        await ctx.close()
      }
    }
  })
})

/*
 * Capture-only, for the titles round (GDK-1737, GDK-1738). Four frames at the
 * width the verdict is about — 1440 — because the three title columns added
 * here are all "does it fit", and that question has a different answer at
 * 1280. Korean is one of the two locales for the same reason: the same rem
 * holds roughly half the characters.
 */
test.describe('retro titles captures', () => {
  test('capture', async ({ page }) => {
    test.skip(!process.env.RETRO_TITLES_SHOT_DIR, 'capture-only; set RETRO_TITLES_SHOT_DIR to run')
    const dir = process.env.RETRO_TITLES_SHOT_DIR as string
    mkdirSync(dir, { recursive: true })
    const VIEW = { width: 1440, height: 900 }
    for (const theme of ['light', 'dark']) {
      for (const locale of ['en', 'ko']) {
        const ctx = await page.context().browser()!.newContext({ viewport: VIEW })
        const p = await ctx.newPage()
        await prepare(p, locale, theme)
        await stubMaterials(p)
        await p.goto('/#/?retro=1')
        await expect(p.getByTestId('retro-sentence')).toBeVisible()
        await expect(p.getByTestId('retro-aging-chart')).toBeVisible()
        await p.evaluate((th) => {
          if (th === 'light') document.documentElement.removeAttribute('data-theme')
          else document.documentElement.setAttribute('data-theme', th)
        }, theme)
        await p.screenshot({
          path: join(dir, `retro-titles-${locale}-${theme}.png`),
          fullPage: true,
          animations: 'disabled',
        })
        await ctx.close()
      }
    }
  })
})
