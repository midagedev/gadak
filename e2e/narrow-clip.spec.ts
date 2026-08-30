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
    await expect(page.getByTestId('issue-layout')).toBeVisible({
      timeout: 30_000,
    })
    await expect(page.getByText(DEMO_ISSUE_COUNT_KO)).toBeVisible({
      timeout: 30_000,
    })
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
    expect(hits, `trail cells past the scroller/viewport at 740: ${JSON.stringify(hits)}`).toEqual(
      [],
    )
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('row trailing at docked 1100', () => {
  test.use({ viewport: { width: VIEWPORT_DOCKED_MIN_PX, height: 900 } })

  test('visible trail cells stay inside the list column (not the panel seam)', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await waitListRows(page)
    const rows = page.locator('[data-testid="issue-list-scroller"] [data-issue-key]')
    await rows.nth(1).click()
    await expect(
      page.locator('[data-testid="issue-layout"][data-detail-open="true"]'),
    ).toBeVisible()
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
        regime: document
          .querySelector('[data-testid="issue-layout"]')
          ?.getAttribute('data-viewport-regime'),
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
    await expect(page.getByTestId('demo-banner')).toBeVisible({
      timeout: 30_000,
    })
    await expect(page.getByTestId('issue-layout')).toBeVisible({
      timeout: 30_000,
    })

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
        (document.querySelector<HTMLElement>(
          '[data-testid="breakdown-strip"]',
        ) as HTMLElement | null) ??
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
      expect(c.textOverflow, `${c.label}: a squeezed label must ellipsize, not clip`).toBe(
        'ellipsis',
      )
    }

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

/*
 * GDK-1086 (2026-08-28 UI audit): the same legend at the audit's two narrow
 * layouts. The audit ran on d02045c7, before the GDK-1057 shrink fix landed,
 * and photographed "REST AP" / "In progre" cut mid-letter with "+ N more"
 * pushed out of view. Contract: the strip never scrolls (scrollWidth <=
 * clientWidth — a scrollable strip is an unhinted cut at its edge), every
 * chip and the fold marker end inside it, and a label that does not fit
 * ellipsizes. The 800 case is the audit's first-run/epic default axis; the
 * 1440 case reproduces the audit's terminal-split + docked-detail layout
 * with the status axis whose "In progress" chip was the mid-letter cut.
 */
type LegendProbe = {
  sw: number
  cw: number
  stripRight: number
  chips: {
    label: string
    right: number
    labelScrollW: number
    labelClientW: number
    textOverflow: string
  }[]
  more: { text: string; right: number; display: string }[]
}

async function probeLegend(page: Page): Promise<LegendProbe> {
  return page.evaluate(() => {
    const strip = document.querySelector<HTMLElement>('[data-testid="breakdown-strip"]')
    if (!strip) return { sw: -1, cw: -1, stripRight: 0, chips: [], more: [] }
    const sbox = strip.getBoundingClientRect()
    const chips = [...strip.querySelectorAll<HTMLElement>('button')].map((chip) => {
      const label = chip.querySelector<HTMLElement>('span.truncate')
      return {
        label: (label?.textContent ?? '').trim(),
        right: Math.round(chip.getBoundingClientRect().right * 10) / 10,
        labelScrollW: label ? label.scrollWidth : -1,
        labelClientW: label ? label.clientWidth : -1,
        textOverflow: label ? getComputedStyle(label).textOverflow : '',
      }
    })
    const more = [...strip.querySelectorAll<HTMLElement>(':scope > div > span')].map((s) => ({
      text: (s.textContent ?? '').trim(),
      right: Math.round(s.getBoundingClientRect().right * 10) / 10,
      display: getComputedStyle(s).display,
    }))
    return {
      sw: strip.scrollWidth,
      cw: strip.clientWidth,
      stripRight: Math.round(sbox.right * 10) / 10,
      chips,
      more,
    }
  })
}

function assertLegendContract(probe: LegendProbe, where: string): void {
  expect(
    probe.sw,
    `${where}: strip scrollWidth ${probe.sw} > clientWidth ${probe.cw} — the legend is cut at its edge with no affordance`,
  ).toBeLessThanOrEqual(probe.cw)
  expect(probe.chips.length, `${where}: fixture must paint breakdown chips`).toBeGreaterThan(0)
  for (const c of probe.chips) {
    expect(
      c.right,
      `${where}: ${c.label}: chip right ${c.right} passes the strip edge ${probe.stripRight}`,
    ).toBeLessThanOrEqual(probe.stripRight + 0.5)
    if (c.labelScrollW > c.labelClientW) {
      expect(
        c.textOverflow,
        `${where}: ${c.label}: a squeezed label must ellipsize, not clip mid-letter`,
      ).toBe('ellipsis')
    }
  }
  for (const m of probe.more) {
    expect(m.display, `${where}: fold marker "${m.text}" must be painted`).not.toBe('none')
    expect(
      m.right,
      `${where}: fold marker "${m.text}" right ${m.right} passes the strip edge ${probe.stripRight}`,
    ).toBeLessThanOrEqual(probe.stripRight + 0.5)
  }
}

test.describe('breakdown legend at 800 (GDK-1086)', () => {
  test.use({ viewport: { width: 800, height: 900 } })

  test('default axis legend fits with the fold marker visible', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await waitListRows(page)
    const probe = await probeLegend(page)
    assertLegendContract(probe, '800px default axis')
    expect(
      probe.more.some((m) => m.text.startsWith('+')),
      'at 800 the fixture must fold some groups, else the fold-marker assertion is vacuous',
    ).toBe(true)
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

/*
 * 2026-08-30 (GDK-1194): the audit's third pane was a terminal *beside* the
 * list. The terminal is a bottom dock now and takes no width from this
 * strip, so the layout is reproduced by the viewport that leaves the list
 * the least: VIEWPORT_DOCKED_MIN_PX itself, the last width at which the
 * panel is still docked. The cut this test is named for is a fact
 * about a squeezed legend, not about what did the squeezing, and the
 * "vacuous" guard below still proves the squeeze happened. With the pane
 * gone so is its PTY, and with it the afterEach that reaped the session.
 */
test.describe('breakdown legend at 1100 docked detail (GDK-1086)', () => {
  test.use({ viewport: { width: VIEWPORT_DOCKED_MIN_PX, height: 900 } })

  test('status legend fits (the "In progre" cut)', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await waitListRows(page)
    // Status axis, whose chips carry the audit's "In progress" label — the
    // mid-letter cut in the audit's capture. Picked before the panel docks:
    // the trigger click does not land through the seam.
    await page.getByRole('button', { name: /Breakdown/ }).click()
    await page.getByRole('button', { name: 'Progress', exact: true }).click()
    await page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first().click()
    await expect(page.getByTestId('issue-detail-panel')).toHaveClass(/is-open/)
    const probe = await probeLegend(page)
    assertLegendContract(probe, '1120 docked detail (status axis)')
    /*
     * The "must squeeze" guard assumed the layout forces an overflow here.
     * It did while the terminal pane took width beside the list; the dock
     * takes none, so at the product's narrowest docked layout the squeeze
     * is decided by font metrics — CI run 33318658199 went red with zero
     * squeezed chips because Linux ch is narrower than this machine's.
     * What GDK-1086 protects is the cut's *shape*: a label that lacks
     * space ellipsizes instead of clipping mid-glyph. Assert the mechanism
     * on every chip unconditionally; when the fonts do overflow (as they
     * do here on macOS), assertLegendContract above already checks the cut.
     */
    for (const c of probe.chips) {
      expect(
        c.textOverflow,
        `${c.label}: a chip label without ellipsis armed clips mid-glyph the day it overflows`,
      ).toBe('ellipsis')
    }
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

/*
 * GDK-1088 (2026-08-28 UI audit): the docs column filter at 800 clipped its
 * placeholder mid-word ("Filter — Enter s") with a partial glyph hanging
 * against the `/` keycap. An input clips its placeholder by default;
 * text-overflow: ellipsis applies to it too. Contract: the placeholder
 * ellipsizes when it does not fit, and the keycap keeps a visible gap.
 */
test.describe('docs filter placeholder at 800 (GDK-1088)', () => {
  test.use({ viewport: { width: 800, height: 900 } })

  test('placeholder ellipsizes and keeps its gap to the keycap', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.getByTestId('docs-documents').click()
    await expect(page.getByTestId('docs-view')).toBeVisible()

    const probe = await page.getByTestId('docs-filter').evaluate((wrap) => {
      const input = wrap.querySelector<HTMLInputElement>('[data-testid="docs-filter-input"]')
      const kbd = wrap.querySelector('kbd')
      if (!input) return null
      const ib = input.getBoundingClientRect()
      const kb = kbd ? kbd.getBoundingClientRect() : null
      return {
        textOverflow: getComputedStyle(input).textOverflow,
        gap: kb ? Math.round((kb.left - ib.right) * 10) / 10 : null,
      }
    })
    expect(probe, 'docs filter must be mounted').toBeTruthy()
    expect(
      probe!.textOverflow,
      'the placeholder must ellipsize when the column is narrow, not hard-clip mid-word',
    ).toBe('ellipsis')
    expect(
      probe!.gap,
      `keycap must keep a visible gap to the input text, got ${probe!.gap}px`,
    ).toBeGreaterThanOrEqual(4)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
