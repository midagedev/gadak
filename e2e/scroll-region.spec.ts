import { expect, test, type Locator } from '@playwright/test'
import { gotoApp, openServerSettings } from './helpers'

/**
 * GDK-131: last row of a named scroll region must sit fully inside the
 * client box at scroll-end (Korean 받침 is unreadable when the clip cuts
 * through x-height). Geometry only — no screenshot comparison.
 */

type Edge = {
  overflow: number
  after: number
  scrolls: boolean
  lastTag: string
  hasClass: boolean
}

/**
 * Find the overflowing overflow-y scroller under `root` (self or descendant),
 * scroll it to the end, and measure the last element's box against the
 * scroller's client box.
 *
 * `overflow` is last.bottom − clientBottom: ≤1 means fully inside (spec).
 * `after` is scrollHeight − last border-box end: the breathing room the
 * shared `.scroll-region` class owns (default 12px).
 */
async function measureScrollEnd(root: Locator): Promise<Edge> {
  return root.evaluate((rootEl) => {
    const nodes = [rootEl, ...rootEl.querySelectorAll('*')] as HTMLElement[]
    const el = nodes.find((n) => {
      const s = getComputedStyle(n)
      return (
        (s.overflowY === 'auto' || s.overflowY === 'scroll') &&
        n.scrollHeight > n.clientHeight + 1
      )
    })
    if (!el) {
      throw new Error(
        `no overflowing scroller under <${rootEl.tagName.toLowerCase()}>`,
      )
    }
    el.scrollTop = el.scrollHeight
    const kids = [...el.children].filter((c) => (c as HTMLElement).offsetHeight > 0)
    const last = kids[kids.length - 1] as HTMLElement | undefined
    if (!last) throw new Error('scroller has no visible children')
    const lastBox = last.getBoundingClientRect()
    const box = el.getBoundingClientRect()
    // Client box bottom (padding edge), not the border-box bottom.
    const clientBottom = box.top + el.clientTop + el.clientHeight
    const overflow = lastBox.bottom - clientBottom
    return {
      overflow,
      // Space between last border-box and the clip edge. offsetTop is the
      // wrong parent here (sidebar last child's offsetParent is the rail,
      // not the scroller).
      after: -overflow,
      scrolls: true,
      lastTag: last.tagName.toLowerCase(),
      hasClass: el.classList.contains('scroll-region'),
    }
  })
}

test.describe('scroll-region last item stays inside the client box', () => {
  // Short enough that sidebar / settings / palette actually scroll.
  test.use({ viewport: { width: 1280, height: 640 } })

  test('sidebar, settings, and palette last children are fully visible at scroll-end', async ({
    page,
  }) => {
    await gotoApp(page)

    const sidebar = await measureScrollEnd(page.locator('aside.issue-sidebar'))
    expect(sidebar.scrolls, 'sidebar must overflow at 640px').toBe(true)

    await openServerSettings(page)
    const dialog = page.getByTestId('settings-dialog')
    await expect(dialog.getByText('This local copy').or(dialog.getByText('이 로컬 사본'))).toBeVisible()
    const settings = await measureScrollEnd(dialog)
    expect(settings.scrolls, 'settings body must overflow at 640px').toBe(true)
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: /Command palette|커맨드 팔레트/ })
    await expect(palette).toBeVisible()
    await expect(palette.getByRole('option').first()).toBeVisible()
    const list = await measureScrollEnd(page.locator('#palette-list'))
    expect(list.scrolls, 'palette list must overflow at 640px').toBe(true)

    const cases: [string, Edge][] = [
      ['sidebar', sidebar],
      ['settings', settings],
      ['palette', list],
    ]
    // Printed so FAIL-first / green-after logs carry the raw geometry.
    console.log('scroll-region geometry', JSON.stringify(cases))
    for (const [name, m] of cases) {
      expect.soft(
        m.hasClass,
        `${name}: overflowing scroller must use the shared .scroll-region class`,
      ).toBe(true)
      expect.soft(
        m.overflow,
        `${name}: last <${m.lastTag}> overflows client box by ${m.overflow.toFixed(2)}px (after=${m.after.toFixed(2)})`,
      ).toBeLessThanOrEqual(1)
    }
  })
})
