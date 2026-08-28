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
 *
 * GDK-826: the two tests that never needed this browser moved to vitest —
 * the HostedLinks source walk (web/src/lib/hosted-banner.test.ts) and the
 * --layout-* token sum (web/src/lib/layout-tokens.test.ts; the tokens are
 * an inline style generated from the TS constants, so the unit owns the
 * whole chain). What stays here is painted geometry.
 */

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

/*
 * GDK-1057 (2026-08-27 walkthrough): with the panel docked, the breakdown
 * strip kept its natural width inside overflow-x-auto, so the narrowed list
 * column cut a chip mid-word at the container edge ("Editor p") with no
 * ellipsis and no summary. Contract: every chip ends inside the strip, and a
 * chip whose label does not fit ellipsizes (the full name survives in the
 * accessible name and the title). Geometry only, like the probes above.
 */
test.describe('breakdown chips at the docked seam (GDK-1057)', () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  type ChipProbe = {
    label: string
    right: number
    stripRight: number
    labelScrollW: number
    labelClientW: number
    textOverflow: string
  }

  async function probeChips(page: Page): Promise<ChipProbe[]> {
    return page.evaluate(() => {
      // The testid is the GDK-1057 marker; the shape fallback keeps this
      // probe reporting the defect (chips past the edge) on a pre-fix tree
      // instead of failing on a missing selector.
      const strip =
        (document.querySelector<HTMLElement>('[data-testid="breakdown-strip"]') as HTMLElement | null) ??
        ([...document.querySelectorAll('div.overflow-x-auto')].find((d) =>
          d.querySelector('button span.truncate'),
        ) as HTMLElement | undefined) ??
        null
      if (!strip) return []
      const sbox = strip.getBoundingClientRect()
      return [...strip.querySelectorAll<HTMLElement>('button')].map((chip) => {
        const label = chip.querySelector<HTMLElement>('span.truncate')
        return {
          label: (label?.textContent ?? '').trim(),
          right: Math.round(chip.getBoundingClientRect().right * 10) / 10,
          stripRight: Math.round(sbox.right * 10) / 10,
          labelScrollW: label ? label.scrollWidth : -1,
          labelClientW: label ? label.clientWidth : -1,
          textOverflow: label ? getComputedStyle(label).textOverflow : '',
        }
      })
    })
  }

  test('chips ellipsize inside the strip, never cut at its edge', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await waitListRows(page)

    // Actor axis: many narrow chips — the axis the walkthrough cut. Picked
    // before the panel opens because below 1440 the panel covers the toolbar
    // seam and the breakdown trigger click does not land (breakdown-esc).
    await page.getByRole('button', { name: /Breakdown/ }).click()
    await page.getByRole('button', { name: 'Actor', exact: true }).click()

    // Dock the panel: at 1280 the list column shrinks to ~570px.
    await page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first().click()
    await expect(page.getByTestId('issue-detail-panel')).toHaveClass(/is-open/)

    const chips = await probeChips(page)
    expect(chips.length, 'fixture must paint breakdown chips on the actor axis').toBeGreaterThan(0)
    for (const c of chips) {
      expect(
        c.right,
        `${c.label}: chip right ${c.right} passes the strip edge ${c.stripRight} — mid-word cut`,
      ).toBeLessThanOrEqual(c.stripRight + 0.5)
    }
    const squeezed = chips.filter((c) => c.labelScrollW > c.labelClientW)
    expect(
      squeezed.length,
      'at 1280 with the panel docked at least one actor label must be squeezed, else this is vacuous',
    ).toBeGreaterThan(0)
    for (const c of squeezed) {
      expect(
        c.textOverflow,
        `${c.label}: a squeezed label must ellipsize, not clip`,
      ).toBe('ellipsis')
    }

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
