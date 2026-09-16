/*
 * GDK-1964 end to end: every landed write announces itself, and the status
 * move paints the header before any sync lands.
 *
 * src/ui/detail/detail-sheets.test.ts pins the source contracts (one
 * announcer, the field each sheet names) and lib/writes.test.ts the wire
 * verbs; neither can measure the half a person sees — that the toast host
 * carries the sentence in the success kind, and that the header chip moves
 * on the write's own answer rather than waiting for the row a sync brings.
 *
 * Same route handlers every write spec here registers (parentlabels.spec.ts):
 * `gadak demo` is a credential-less serve, so the store's writability probe
 * answers off and every write control ships disabled — answering that one
 * GET configured:true is the smallest change that puts the real controls on
 * screen. The write endpoints themselves ARE routed, with the serve's own
 * response shape ({issue: …} — respondIssue): the transition POST's answer
 * is what the screen must latch, so the test controls it.
 *
 * Key: NMB-105, a standard Bug in the demo fixture carrying status
 * 'In Progress' (inprogress), priority Medium (priority_id 3), assignee
 * Dana Whitfield, updated_at 2026-09-04T12:21:53Z. The stubbed issues below
 * copy that row and differ in exactly the field each test moves, with a
 * newer updated_at — the axis the header's written-latch reads.
 */
import { type Page } from '@playwright/test'
import { expect, test } from './helpers'

const ISSUE = 'NMB-105'

/** NMB-105 as the fixture holds it, in the fields the detail header paints.
 *  updated_at is newer than the fixture row's on purpose: the header
 *  overlays the mirror row with the write's answer only while that answer
 *  is the newer one. */
const LITE = {
  issue_key: ISSUE,
  summary: 'Dashboard chart legend overlaps axis labels on 1280px viewports',
  project_key: 'NMB',
  issue_type: 'Bug',
  issue_type_id: '10029',
  status: 'In Progress',
  status_id: '3',
  status_category: 'inprogress',
  priority: 'Medium',
  priority_id: '3',
  priority_rank: 3,
  assignee: 'Dana Whitfield',
  assignee_id: 'demo-dana',
  reopen_count: 0,
  duedate: null as string | null,
  updated_at: '2026-09-16T00:00:00.000Z',
}

/** WCAG relative-luminance contrast between two computed css colours,
 *  computed here rather than by a dependency — the gate owns the number
 *  (GDK-1969). Throws on transparent/unparseable so a token change that
 *  breaks the reading fails loudly, not green-by-accident. */
function contrast(a: string, b: string): number {
  const lum = (css: string): number => {
    const m = css.match(/rgba?\(([\d.]+),\s*([\d.]+),\s*([\d.]+)(?:,\s*([\d.]+))?\)/)
    if (!m) throw new Error(`contrast: unparseable color "${css}"`)
    if (m[4] !== undefined && Number(m[4]) === 0) throw new Error(`contrast: transparent "${css}"`)
    const ch = [1, 2, 3].map((i) => {
      const c = Number(m[i]) / 255
      return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
    })
    return 0.2126 * ch[0] + 0.7152 * ch[1] + 0.0722 * ch[2]
  }
  const [l1, l2] = [lum(a), lum(b)].sort((x, y) => y - x)
  return (l1 + 0.05) / (l2 + 0.05)
}

async function armWrites(page: Page): Promise<void> {
  await page.route('**/api/v1/credential/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ configured: true }),
    })
  })
}

/** Search → row → detail, the pane's own road to any key (a7-captures). */
async function openIssue(page: Page, key: string): Promise<void> {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('h1 button.scope').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.locator('h1 button.scope').click()
  await page.locator('.pane:not(.off) input').first().fill(key)
  const row = page.locator('.pane:not(.off) button.row', { hasText: key }).first()
  await row.waitFor()
  await row.click()
  await page.locator('button.back').waitFor()
}

test('a status move announces itself and paints the header before sync', async ({ page }) => {
  await armWrites(page)
  // The transitions catalog the sheet opens on (the real GET 409s on this
  // credential-less fixture — measured in viewport.spec.ts).
  await page.route('**/api/v1/issues/*/transitions/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        transitions: [{ id: '21', name: 'Done', to_status: 'Done', to_id: '5', to_category: 'done' }],
      }),
    })
  })
  await page.route('**/api/v1/issues/*/transition/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        changed: true,
        origin: 'built-in',
        issue: { ...LITE, status: 'Done', status_id: '5', status_category: 'done' },
      }),
    })
  })

  await openIssue(page, ISSUE)
  const chip = page.locator('.chips .chip')
  await expect(chip).toHaveText(/In Progress/)

  // The sheet opens on the routed catalog, and (GDK-1965) its first line
  // says where the work sits now — the status the rows below would move.
  await page.locator('.composer-slab button.status').click()
  const now = page.locator('.sheet .t-current')
  await expect(now).toHaveAttribute('aria-current', 'true')
  await expect(now).toContainText('In Progress')

  // Hold every bootstrap answer until the assertions below are made: the
  // chip must move because the write's own answer latched (`written`), not
  // because a sync brought a row. fallback hands the request to the serve
  // once released, so the real snapshot still lands afterwards.
  let release: (() => void) | null = null
  const held = new Promise<void>((resolve) => (release = resolve))
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    await held
    await route.fallback()
  })

  await page.locator('.sheet button.t-row', { hasText: 'Done' }).click()

  await expect(page.locator('[data-testid="toast"][data-kind="success"]')).toHaveText('Moved to Done')

  // GDK-1969 lane separation: the confirmation lives in the top half of the
  // viewport. Every bottom surface is thumb territory — the sheet this very
  // write just closed falls through `fly y:320` (Sheet.svelte) exactly where
  // the old bottom-anchored toast rose, and users read the confirmation as
  // part of the sheet's exit.
  const box = await page.locator('[data-testid="toast"]').boundingBox()
  const viewportHeight = page.viewportSize()!.height
  expect(box!.y + box!.height, `toast bottom ${box!.y + box!.height} must sit above half of ${viewportHeight}`).toBeLessThan(
    viewportHeight * 0.5,
  )

  // GDK-1969 contrast: the pill is inverted ink on the page ground, and the
  // gate pins the measured number (≥7:1, both themes clear it — light
  // #1c1812 on #f4efe4 ≈ 15:1, dark ≈ 16:1). body paints --color-bg-base
  // (mobile/src/app.css, `body { background: var(--color-bg-base) }`), so
  // body is the ground to read, not a transparent stand-in.
  const { pill, ground } = await page.evaluate(() => ({
    pill: getComputedStyle(document.querySelector('.toast .pill')!).backgroundColor,
    ground: getComputedStyle(document.body).backgroundColor,
  }))
  expect(contrast(pill, ground), `pill ${pill} on ground ${ground}`).toBeGreaterThanOrEqual(7)

  await expect(chip).toHaveText(/Done/)
  release!()
  // The held sync finally lands the fixture's own row — older than the
  // write's answer — and the latch keeps winning on updated_at, so the
  // chip holds what the write said.
  await expect(chip).toHaveText(/Done/)
})

test('a landed comment announces itself', async ({ page }) => {
  await armWrites(page)
  await page.route('**/api/v1/issues/*/comment/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ origin: 'built-in', issue: LITE }),
    })
  })

  await openIssue(page, ISSUE)
  await page.locator('.composer input').fill('shipping this now')
  await page.locator('.composer button.send').click()

  await expect(page.locator('[data-testid="toast"][data-kind="success"]')).toHaveText(
    `Posted comment on ${ISSUE}`,
  )
})

test('a field sheet announces the field it saved', async ({ page }) => {
  await armWrites(page)
  await page.route('**/api/v1/issues/*/priorities/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ priorities: [{ id: '2', name: 'High' }, { id: '3', name: 'Medium' }] }),
    })
  })
  await page.route('**/api/v1/issues/*/priority/', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        origin: 'built-in',
        issue: { ...LITE, priority: 'High', priority_id: '2' },
      }),
    })
  })

  await openIssue(page, ISSUE)
  // The meta line's first control is priority (type · priority · assignee ·
  // due — GDK-1497 A2's order).
  const priority = page.locator('.meta button.m-btn').first()
  await priority.click()
  const sheet = page.locator('.sheet')
  await sheet.waitFor()
  // Medium is what the issue carries (priority_id 3), and the sheet marks
  // it as the current row.
  await expect(sheet.locator('button.t-row', { hasText: 'Medium' })).toHaveAttribute(
    'aria-current',
    'true',
  )

  await sheet.locator('button.t-row', { hasText: 'High' }).click()

  await expect(page.locator('[data-testid="toast"][data-kind="success"]')).toHaveText('Saved Priority')
  // The latch paints the meta line too, before any sync row lands.
  await expect(priority).toContainText('High')
})
