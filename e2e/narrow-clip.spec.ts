import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect, type Page } from '@playwright/test'
import {
  attachConsoleErrors,
  forceLocale,
  gotoApp,
  searchInput,
  DEMO_ISSUE_COUNT_KO,
} from './helpers'
import { VIEWPORT_DOCKED_MIN_PX } from '../web/src/lib/viewport-regime'

/*
 * GDK-766: narrow-width clipping probe.
 *
 * Three defects from the 2026-08-24 hosted-demo audit (gadak.dev/demo):
 *   1. Stale chip: @container issuerow (max-width: 1100px) sets
 *      .stale-glyph { display: none } in the components layer, but
 *      IssueRow's `class="stale-glyph flex"` is a utilities-layer display
 *      and wins — the hourglass stays, and a 44px slot clips "13일"/"35일".
 *   2. Row trailing: flex-none slots + .issue-main-column overflow:hidden
 *      silently cut avatar / updated at 740 and at the 1100 docked seam.
 *   3. Demo banner: HostedLinks was position:absolute, so GitHub/About
 *      painted over "Run it on your own Jira →" at en 800.
 *
 * Geometry only — no screenshot comparison. Container ≤1100 is produced
 * by an 800px viewport (list ~528px, audit JSON); 740 is the trailing
 * case; 1100 with the panel open is the docked seam.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const REPO = join(HERE, '..')

type TrailOverflow = {
  key: string
  col: string
  right: number
  scrollerRight: number
  viewportW: number
}

type StaleProbe = {
  text: string
  slotSW: number
  slotCW: number
  glyphDisplay: string
}

async function waitListRows(page: Page): Promise<void> {
  await expect(
    page.getByTestId('issue-list-scroller').locator('[data-issue-key]').first(),
  ).toBeVisible({ timeout: 30_000 })
}

async function probeStale(page: Page): Promise<StaleProbe[]> {
  return page.evaluate(() => {
    const scroller = document.querySelector('[data-testid="issue-list-scroller"]')
    if (!scroller) return []
    const out: StaleProbe[] = []
    for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
      const r = row.getBoundingClientRect()
      if (r.bottom < 0 || r.top > innerHeight) continue
      const slot = row.querySelector<HTMLElement>('[data-col="stale"]')
      if (!slot) continue
      const style = getComputedStyle(slot)
      if (style.display === 'none' || style.visibility === 'hidden') continue
      const glyph = slot.querySelector<HTMLElement>('.stale-glyph')
      const text = (slot.textContent ?? '').replace(/\s+/g, ' ').trim()
      if (!text) continue
      out.push({
        text,
        slotSW: slot.scrollWidth,
        slotCW: slot.clientWidth,
        glyphDisplay: glyph ? getComputedStyle(glyph).display : 'missing',
      })
    }
    return out
  })
}

async function trailOverflows(page: Page): Promise<TrailOverflow[]> {
  return page.evaluate(() => {
    const scroller = document.querySelector<HTMLElement>('[data-testid="issue-list-scroller"]')
    if (!scroller) return []
    const sbox = scroller.getBoundingClientRect()
    const hits: TrailOverflow[] = []
    for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
      const r = row.getBoundingClientRect()
      if (r.bottom < 0 || r.top > innerHeight) continue
      const trail = row.querySelector<HTMLElement>('[data-testid="issue-row-trail"]')
      if (!trail) continue
      for (const child of [...trail.children] as HTMLElement[]) {
        const s = getComputedStyle(child)
        if (s.display === 'none' || s.visibility === 'hidden') continue
        const b = child.getBoundingClientRect()
        if (b.width < 1) continue
        // 0.5px: subpixel from deviceScaleFactor. Past the scroller or the
        // viewport is the silent-cut the audit photographed.
        if (b.right > sbox.right + 0.5 || b.right > innerWidth + 0.5) {
          hits.push({
            key: row.dataset.issueKey ?? '',
            col: child.dataset.col ?? child.className.toString().slice(0, 40),
            right: Math.round(b.right * 10) / 10,
            scrollerRight: Math.round(sbox.right * 10) / 10,
            viewportW: innerWidth,
          })
        }
      }
    }
    return hits
  })
}

function boxesOverlap(
  a: { x: number; y: number; width: number; height: number },
  b: { x: number; y: number; width: number; height: number },
): boolean {
  return !(
    a.x + a.width <= b.x ||
    b.x + b.width <= a.x ||
    a.y + a.height <= b.y ||
    b.y + b.height <= a.y
  )
}

test.describe('GDK-766 narrow clip', () => {
  test('HostedLinks is in-flow (source: not absolute-stacked on the banner)', () => {
    /*
     * Geometric overlap needs VITE_HOSTED_DEMO=1 (hosted suite). The CI set
     * is gadak serve, so this assertion is the FAIL-first that can run here:
     * `absolute right-3 top-0` is exactly the stacking the 800-banner crop
     * photographed. After the fix the root is relative/in-flow.
     */
    const src = readFileSync(join(REPO, 'web/src/components/shell/HostedLinks.svelte'), 'utf8')
    expect(
      src,
      'HostedLinks root must not be position:absolute at the banner corner (GDK-766 banner overlap)',
    ).not.toMatch(/class="absolute right-3 top-0/)
    const app = readFileSync(join(REPO, 'web/src/App.svelte'), 'utf8')
    const bannerIdx = app.indexOf('data-testid="demo-banner"')
    const linksIdx = app.indexOf('<HostedLinks')
    expect(bannerIdx, 'demo-banner must exist').toBeGreaterThan(-1)
    expect(linksIdx, 'HostedLinks must be mounted').toBeGreaterThan(-1)
    expect(
      linksIdx > bannerIdx,
      'HostedLinks must sit inside the banner markup, not as an absolute sibling in front of it',
    ).toBe(true)
  })

  test('docked track mins sum to VIEWPORT_DOCKED_MIN_PX (±0)', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    const tokens = await page.locator('[data-testid="issue-layout"]').evaluate((el) => {
      const s = getComputedStyle(el)
      const sidebar = parseFloat(s.getPropertyValue('--layout-sidebar'))
      const list = parseFloat(s.getPropertyValue('--layout-list-min'))
      const detail = parseFloat(s.getPropertyValue('--layout-detail-min'))
      const docked = parseFloat(s.getPropertyValue('--layout-docked-min'))
      return { sidebar, list, detail, docked, sum: sidebar + list + detail }
    })
    expect(tokens.docked, 'CSS --layout-docked-min follows VIEWPORT_DOCKED_MIN_PX').toBe(
      VIEWPORT_DOCKED_MIN_PX,
    )
    expect(
      tokens.sum,
      `sidebar ${tokens.sidebar} + list ${tokens.list} + detail ${tokens.detail} must equal docked ${tokens.docked} (was 272+390+440=1102 vs 1100)`,
    ).toBe(tokens.docked)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('stale chip at container ≤1100', () => {
  test.use({ viewport: { width: 800, height: 900 } })

  test('glyph is hidden and a two-digit (or wider) chip fits its slot', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.addInitScript(() => {
      try {
        localStorage.setItem('gadak_locale', 'ko')
      } catch {
        /* ignore */
      }
    })
    await page.goto('/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByText(DEMO_ISSUE_COUNT_KO)).toBeVisible({ timeout: 30_000 })
    await waitListRows(page)

    // Search view is where the audit cropped "35일" (SSO login fails).
    const search = searchInput(page)
    await search.click()
    await search.fill('login')
    await waitListRows(page)

    const probes = await probeStale(page)
    expect(probes.length, 'fixture must paint stale chips at 800').toBeGreaterThan(0)
    for (const p of probes) {
      expect(
        p.glyphDisplay,
        `${p.text}: .stale-glyph must be display:none at issuerow ≤1100 (utilities flex used to win)`,
      ).toBe('none')
    }
    const wide = probes.filter((p) => /\d{2}/.test(p.text))
    expect(
      wide.length,
      `need a 2+ digit stale chip (13일/35일); got ${probes.map((p) => p.text).join(',')}`,
    ).toBeGreaterThan(0)
    for (const p of wide) {
      expect(
        p.slotSW,
        `${p.text}: slot scrollWidth ${p.slotSW} > clientWidth ${p.slotCW}`,
      ).toBeLessThanOrEqual(p.slotCW)
    }
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('row trailing at 740', () => {
  test.use({ viewport: { width: 740, height: 900 } })

  test('visible trail cells stay inside the list scroller', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await waitListRows(page)
    const hits = await trailOverflows(page)
    expect(
      hits,
      `trail cells past the scroller/viewport at 740: ${JSON.stringify(hits)}`,
    ).toEqual([])
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('row trailing at docked 1100', () => {
  test.use({ viewport: { width: VIEWPORT_DOCKED_MIN_PX, height: 900 } })

  test('visible trail cells stay inside the list column (not the panel seam)', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await waitListRows(page)
    const rows = page.locator('[data-testid="issue-list-scroller"] [data-issue-key]')
    await rows.nth(1).click()
    await expect(page.locator('[data-testid="issue-layout"][data-detail-open="true"]')).toBeVisible()
    await expect(page.getByTestId('issue-detail-panel')).toHaveClass(/is-open/)
    // Panel width is locked immediately; opacity/transform slide 160ms.
    // Wait on the resting grid track, not a duration.
    await expect
      .poll(async () =>
        page.locator('[data-testid="issue-layout"]').evaluate((el) => {
          const panel = el.querySelector<HTMLElement>('[data-testid="issue-detail-panel"]')
          if (!panel) return 0
          return Math.round(panel.getBoundingClientRect().width)
        }),
      )
      .toBeGreaterThan(400)

    const hits = await trailOverflows(page)
    expect(
      hits,
      `trail cells past the list scroller at docked ${VIEWPORT_DOCKED_MIN_PX}: ${JSON.stringify(hits)}`,
    ).toEqual([])

    const seam = await page.evaluate(() => {
      const scroller = document.querySelector<HTMLElement>('[data-testid="issue-list-scroller"]')
      const panel = document.querySelector<HTMLElement>('[data-testid="issue-detail-panel"]')
      if (!scroller || !panel) return { scrollerRight: 0, panelLeft: 0, regime: null }
      return {
        scrollerRight: Math.round(scroller.getBoundingClientRect().right * 10) / 10,
        panelLeft: Math.round(panel.getBoundingClientRect().left * 10) / 10,
        regime: document.querySelector('[data-testid="issue-layout"]')?.getAttribute(
          'data-viewport-regime',
        ),
      }
    })
    expect(seam.regime, '1100 is the docked floor').toBe('docked')
    expect(
      seam.scrollerRight,
      `list scroller right ${seam.scrollerRight} must not pass panel left ${seam.panelLeft}`,
    ).toBeLessThanOrEqual(seam.panelLeft + 0.5)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('demo banner at en 800', () => {
  test.use({ viewport: { width: 800, height: 900 } })

  test('banner copy and HostedLinks boxes do not overlap', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.route('**/config.json', async (route) => {
      const resp = await route.fetch()
      const json = (await resp.json()) as Record<string, unknown>
      json.hostedDemo = true
      await route.fulfill({ json })
    })
    await forceLocale(page, 'en')
    await page.goto('/')
    await expect(page.getByTestId('demo-banner')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })

    const links = page.getByTestId('hosted-links')
    // gadak serve compiles HostedLinks (unconditional import). Mount follows
    // isHostedDemo(), which this test turns on via config.json.
    await expect(links).toBeVisible()
    const banner = page.getByTestId('demo-banner')
    const copy = banner.locator('a').first()
    await expect(copy).toBeVisible()
    const copyBox = await copy.boundingBox()
    const linksBox = await links.boundingBox()
    expect(copyBox, 'banner CTA must have a box').toBeTruthy()
    expect(linksBox, 'hosted-links must have a box').toBeTruthy()
    expect(
      boxesOverlap(copyBox!, linksBox!),
      `banner CTA ${JSON.stringify(copyBox)} overlaps hosted-links ${JSON.stringify(linksBox)}`,
    ).toBe(false)

    const overflow = await banner.evaluate((el) => ({
      sw: el.scrollWidth,
      cw: el.clientWidth,
    }))
    expect(
      overflow.sw,
      `demo-banner scrollWidth ${overflow.sw} > clientWidth ${overflow.cw}`,
    ).toBeLessThanOrEqual(overflow.cw)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
