/*
 * GDK-434 / GDK-435: sidebar section collapse + order persist in localStorage.
 *
 * Contracts:
 *  1. Collapse a section, reload → still collapsed
 *  2. Reorder sections, reload → order kept
 *  3. A collapsed section's header stays visible with aria-expanded=false
 */
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, gotoApp, DEMO_ISSUE_COUNT_EN_RE } from './helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const SHOT_COLLAPSED = path.join(here, '../scratch/gdk-434-sidebar-collapsed.png')
const SHOT_EXPANDED = path.join(here, '../scratch/gdk-434-sidebar-expanded.png')

function sectionHeader(page: Page, id: string) {
  return page.getByTestId(`sidebar-section-header-${id}`)
}

function sectionBody(page: Page, id: string) {
  return page.getByTestId(`sidebar-section-body-${id}`)
}

/**
 * Geometry of the sidebar's mid scroller and the rows whose reachability
 * GDK-1081 pins. Every coordinate is a viewport coordinate; "inside the
 * scroller" means inside its client box, which is the clip that actually
 * hides a row. `visibleRecents` counts recent rows fully inside that box and,
 * once the fix mounts the list's own scroller, fully inside the list's box
 * too — a row clipped by the nested scroll is not reachable without
 * scrolling something.
 */
async function sidebarGeometry(page: Page) {
  return page.evaluate(() => {
    const scroll = document.querySelector('[data-testid="sidebar-scroll"]')
    const rect = (id: string) =>
      document.querySelector(`[data-testid="${id}"]`)?.getBoundingClientRect() ?? null
    const s = scroll?.getBoundingClientRect()
    const scrollBottom = s ? s.top + (scroll?.clientTop ?? 0) + (scroll?.clientHeight ?? 0) : -1
    const rows = [
      ...document.querySelectorAll('[data-testid^="recent-issue-"], [data-testid^="recent-doc-"]'),
    ]
    const list = document.querySelector('[data-testid="recent-list"]')
    const lb = list?.getBoundingClientRect()
    return {
      scrollTop: s?.top ?? -1,
      /** How far the sidebar's mid scroller itself has been scrolled. */
      scrollOffset: scroll?.scrollTop ?? -1,
      scrollBottom,
      viewportH: window.innerHeight,
      recents: document.querySelectorAll('[data-testid^="recent-issue-"]').length,
      visibleRecents: rows.filter((row) => {
        const b = row.getBoundingClientRect()
        if (!(b.top >= (s?.top ?? 0) && b.bottom <= scrollBottom)) return false
        return !lb || (b.top >= lb.top - 0.5 && b.bottom <= lb.bottom + 0.5)
      }).length,
      recentList: {
        top: list?.getBoundingClientRect().top ?? -1,
        clientHeight: list?.clientHeight ?? -1,
        scrollHeight: list?.scrollHeight ?? -1,
      },
      documents: rect('docs-documents'),
      spaces: rect('docs-spaces'),
    }
  })
}

/**
 * Seed the recency list the way a visit history builds it: stores/me reads
 * localStorage `gadak:recent` (one row per opened detail — same idiom
 * keys-focus.spec.ts uses). All twelve keys exist in the committed demo
 * fixture; the tests assert the mounted row count so a fixture change fails
 * loudly instead of measuring a silently shorter list.
 */
const RECENT_SEED_KEYS = Array.from({ length: 12 }, (_, i) => `NMB-${i + 1}`)

function seedRecents(page: Page): void {
  void page.addInitScript((keys) => {
    localStorage.setItem(
      'gadak:recent',
      JSON.stringify(
        keys.map((key, i) => ({
          key,
          viewed_at: new Date(Date.UTC(2026, 0, 12 - i, 9, 0, 0)).toISOString(),
          kind: 'issue',
        })),
      ),
    )
  }, RECENT_SEED_KEYS)
}

async function waitRecentsMounted(page: Page, count: number): Promise<void> {
  await expect(page.locator('[data-testid^="recent-issue-"]')).toHaveCount(count)
}

/** Visible section ids in DOM order (data-section on each listitem). */
async function sectionOrder(page: Page): Promise<string[]> {
  return page
    .locator('[data-testid="sidebar-sections"] [data-section]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-section') ?? ''))
}

async function reloadSidebar(page: Page): Promise<void> {
  await page.reload()
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(page.getByTestId('sidebar-sections')).toBeVisible()
}

test.describe('sidebar section collapse and order', () => {
  test('collapsing a section survives reload; header stays visible', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const header = sectionHeader(page, 'builtin')
    await expect(header).toBeVisible()
    await expect(header).toHaveAttribute('aria-expanded', 'true')
    await expect(sectionBody(page, 'builtin')).toBeVisible()

    await page.locator('aside.issue-sidebar').screenshot({ path: SHOT_EXPANDED })

    await header.click()
    await expect(header).toHaveAttribute('aria-expanded', 'false')
    await expect(header).toBeVisible()
    await expect(sectionBody(page, 'builtin')).toBeHidden()

    await page.locator('aside.issue-sidebar').screenshot({ path: SHOT_COLLAPSED })

    await reloadSidebar(page)

    const after = sectionHeader(page, 'builtin')
    await expect(after).toBeVisible()
    await expect(after).toHaveAttribute('aria-expanded', 'false')
    await expect(sectionBody(page, 'builtin')).toBeHidden()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('reordering sections with Alt+Arrow survives reload', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const before = await sectionOrder(page)
    expect(before.length, 'demo fixture should show at least two sections').toBeGreaterThanOrEqual(2)
    expect(before[0]).toBe('builtin')

    const header = sectionHeader(page, 'builtin')
    await header.focus()
    await header.press('Alt+ArrowDown')

    const moved = await sectionOrder(page)
    expect(moved[0]).not.toBe('builtin')
    expect(moved).toContain('builtin')
    expect(moved.length).toBe(before.length)

    await reloadSidebar(page)

    expect(await sectionOrder(page)).toEqual(moved)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

/*
 * GDK-1081. The sidebar's mid region is one scroller (sidebar-scroll), so
 * "reachable" means "inside that scroller's client box" — a row past the
 * client box is clipped even when the viewport rectangle still contains it.
 * The personal lists above the sections grow one 48px row per visit and are
 * the only blocks that push DOCUMENTS toward that clip edge. This gate pins
 * the zero-recents baseline the GDK-1081 fix must preserve: a fresh context
 * has no localStorage visits, so the fold has slack and both DOCUMENTS rows
 * sit fully inside the scroller at 900px.
 */
test.describe('GDK-1081 sidebar reachability baseline', () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  test('zero-recents layout keeps both DOCUMENTS rows inside the scroller', async ({ page }) => {
    await gotoApp(page)

    const geo = await sidebarGeometry(page)

    expect(geo.recents, 'fresh e2e context must carry no sidebar visits').toBe(0)
    for (const [name, r] of [
      ['documents', geo.documents],
      ['spaces', geo.spaces],
    ] as const) {
      expect(r, `${name} row must exist on the demo fixture`).not.toBeNull()
      expect.soft(
        r!.top,
        `${name}: top ${r!.top} above the scroller top ${geo.scrollTop}`,
      ).toBeGreaterThanOrEqual(geo.scrollTop)
      expect.soft(
        r!.bottom,
        `${name}: bottom ${r!.bottom} clipped by the scroller bottom ${geo.scrollBottom}`,
      ).toBeLessThanOrEqual(geo.scrollBottom + 1)
      expect.soft(
        r!.bottom,
        `${name}: bottom ${r!.bottom} past the viewport ${geo.viewportH}`,
      ).toBeLessThanOrEqual(geo.viewportH)
    }
  })
})

/*
 * GDK-1081's defect: the recency list has no yielding axis, so every visited
 * issue adds a 48px row that pushes BUILT-IN VIEWS / JIRA FILTERS / DOCUMENTS
 * past the scroller's clip edge. Twelve visits at 900px put DOCUMENTS at
 * y≈919 against a scroller bottom of 742 — gone, with an overlay scrollbar
 * that gives no hint anything is off-screen. The fix's invariant: the
 * structural sections keep their natural height and the recent list is the
 * block that shrinks (scrolling inside itself once it runs out of room), so
 * sections below stay reachable without scrolling the sidebar.
 */
test.describe('GDK-1081 recents yield before the sections below them', () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  test('twelve recents leave DOCUMENTS and SPACES reachable at 900px', async ({ page }) => {
    seedRecents(page)
    await gotoApp(page)
    await waitRecentsMounted(page, 12)

    const geo = await sidebarGeometry(page)

    expect(geo.recents, 'the twelve seeded visits must all mount as sidebar rows').toBe(12)
    expect(
      geo.scrollOffset,
      `the sidebar's own scroller must sit at its top — the sections must be ` +
        `reachable without scrolling it (offset ${geo.scrollOffset})`,
    ).toBe(0)
    for (const [name, r] of [
      ['documents', geo.documents],
      ['spaces', geo.spaces],
    ] as const) {
      expect(r, `${name} row must exist on the demo fixture`).not.toBeNull()
      expect.soft(
        r!.top,
        `${name}: top ${r!.top} above the scroller top ${geo.scrollTop}`,
      ).toBeGreaterThanOrEqual(geo.scrollTop)
      expect.soft(
        r!.bottom,
        `${name}: bottom ${r!.bottom} clipped by the scroller bottom ${geo.scrollBottom}`,
      ).toBeLessThanOrEqual(geo.scrollBottom + 1)
      expect.soft(
        r!.bottom,
        `${name}: bottom ${r!.bottom} past the viewport ${geo.viewportH}`,
      ).toBeLessThanOrEqual(geo.viewportH)
    }
  })
})

/*
 * The other half of the same invariant: yielding must not overshoot. A fix
 * that clamps the recent list to a fixed sliver would pass the 900px gate
 * above and waste every roomy window. At 1400px the twelve seeded rows do not
 * all fit (measured: 576px of rows against 523px of list — the sidebar's
 * sections alone need 623px of the 1123px scroller), so six-visible is the
 * contract there, and "the list itself does not scroll" belongs to the window
 * that is actually roomy for the whole rail: 1600px, where every section and
 * all twelve rows fit with slack.
 */
test.describe('GDK-1081 roomy window spends its slack on the recent list', () => {
  test.use({ viewport: { width: 1280, height: 1400 } })

  test('twelve recents show six or more rows at 1400px', async ({ page }) => {
    seedRecents(page)
    await gotoApp(page)
    await waitRecentsMounted(page, 12)

    const geo = await sidebarGeometry(page)

    expect(geo.recents, 'the twelve seeded visits must all mount as sidebar rows').toBe(12)
    expect(
      geo.recentList.clientHeight,
      'recent-list scroller must be mounted (post-fix handle)',
    ).toBeGreaterThan(0)
    expect(
      geo.visibleRecents,
      `1400px window must show at least six recent rows without scrolling ` +
        `(showing ${geo.visibleRecents} of ${geo.recents})`,
    ).toBeGreaterThanOrEqual(6)
  })
})

test.describe('GDK-1081 window tall enough for the whole rail shows every row', () => {
  test.use({ viewport: { width: 1280, height: 1600 } })

  test('twelve recents scroll nothing and keep the sections reachable', async ({ page }) => {
    seedRecents(page)
    await gotoApp(page)
    await waitRecentsMounted(page, 12)

    const geo = await sidebarGeometry(page)

    expect(geo.recents, 'the twelve seeded visits must all mount as sidebar rows').toBe(12)
    expect(
      geo.recentList.clientHeight,
      'recent-list scroller must be mounted (post-fix handle)',
    ).toBeGreaterThan(0)
    expect(
      geo.recentList.scrollHeight,
      `roomy window must leave the recent list unscrolled (scrollHeight ` +
        `${geo.recentList.scrollHeight} > clientHeight ${geo.recentList.clientHeight})`,
    ).toBeLessThanOrEqual(geo.recentList.clientHeight)
    for (const [name, r] of [
      ['documents', geo.documents],
      ['spaces', geo.spaces],
    ] as const) {
      expect(r, `${name} row must exist on the demo fixture`).not.toBeNull()
      expect.soft(
        r!.bottom,
        `${name}: bottom ${r!.bottom} clipped by the scroller bottom ${geo.scrollBottom}`,
      ).toBeLessThanOrEqual(geo.scrollBottom + 1)
    }
  })
})
