/*
 * GDK-759: dragging the sidebar and the list column.
 *
 * The three things a browser has to prove, because nothing below it can:
 *   1. a drag moves the real painted column, and the width SURVIVES A RELOAD
 *      because it went into the settings document — not into localStorage,
 *      which is why the assertion reads the API back rather than the DOM;
 *   2. the drag stops at the dim catalog's range end. Nothing below the
 *      handle enforces that — an out-of-range dimension is a warning the
 *      server carries, not a 400 — so the clamp is only real if a browser
 *      pulling past it is measured;
 *   3. one drag is exactly ONE PUT. The local variable moves on every
 *      pointermove frame; the save happens on pointerup and nowhere else.
 *      This is the only place the frame count and the request count can be
 *      compared against each other.
 */
import { type Page } from '@playwright/test'
import {
  apiURL,
  attachConsoleErrors,
  appConsoleErrors,
  drainTerminalSessions,
  expect,
  gotoApp,
  test,
} from './helpers'

const SETTINGS_URL = apiURL('/api/v1/issues/settings/')

/** The catalog range for the sidebar (internal/config/tokencheck/dim-catalog.json). */
const SIDEBAR_MIN = 208
const SIDEBAR_MAX = 320
const SIDEBAR_DEFAULT = 272

async function sidebarWidth(page: Page): Promise<number> {
  const box = await page.locator('.issue-sidebar').boundingBox()
  return Math.round(box?.width ?? 0)
}

/**
 * Press the handle and walk the pointer to `toX` in `steps` moves. Steps are
 * explicit because the PUT count is only meaningful next to a frame count:
 * a per-frame save would answer `steps`, not 1.
 */
async function dragHandle(page: Page, testid: string, toX: number, steps = 20): Promise<void> {
  const handle = page.getByTestId(testid)
  const box = await handle.boundingBox()
  if (!box) throw new Error(`${testid} has no box`)
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.down()
  await page.mouse.move(toX, box.y + box.height / 2, { steps })
  await page.mouse.up()
}

test.describe('layout resize handles', () => {
  test.afterEach(async ({ request }) => {
    const res = await request.get(SETTINGS_URL)
    if (!res.ok()) return
    const body = (await res.json()) as Record<string, unknown>
    await request.put(SETTINGS_URL, { data: { ...body, ui: {} } })
    await expect
      .poll(async () => {
        const check = await request.get(SETTINGS_URL)
        if (!check.ok()) return null
        const doc = (await check.json()) as { ui?: { tokens?: unknown } }
        return doc.ui?.tokens === undefined ? 'reset' : null
      })
      .toBe('reset')
  })

  test('a sidebar drag moves the column and survives a reload', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    expect(await sidebarWidth(page)).toBe(SIDEBAR_DEFAULT)

    const layoutBox = await page.getByTestId('issue-layout').boundingBox()
    const target = Math.round((layoutBox?.x ?? 0) + 300)
    await dragHandle(page, 'layout-resize-sidebar', target)
    expect(await sidebarWidth(page)).toBe(300)

    // The document, not the browser: this is the assertion that fails if the
    // width were kept in localStorage or a store of its own.
    await expect
      .poll(async () => {
        const doc = (await (await request.get(SETTINGS_URL)).json()) as {
          ui?: { tokens?: { layout?: Record<string, string> } }
        }
        return doc.ui?.tokens?.layout?.sidebar ?? null
      })
      .toBe('300px')

    await page.reload()
    await expect(page.getByTestId('issue-layout')).toBeVisible()
    await expect.poll(async () => sidebarWidth(page)).toBe(300)
    expect(appConsoleErrors(errors)).toEqual([])
  })

  test('the drag stops at the catalog range, not past it', async ({ page, request }) => {
    await gotoApp(page)
    const layoutBox = await page.getByTestId('issue-layout').boundingBox()
    const left = layoutBox?.x ?? 0

    // Far past the floor, then far past the ceiling. The pointer leaves the
    // range in both directions; the column may not follow it out.
    await dragHandle(page, 'layout-resize-sidebar', Math.round(left + 40))
    expect(await sidebarWidth(page)).toBe(SIDEBAR_MIN)

    await dragHandle(page, 'layout-resize-sidebar', Math.round(left + 900))
    expect(await sidebarWidth(page)).toBe(SIDEBAR_MAX)

    // And what was SAVED is the clamped value, not the pointer's. The
    // server would have stored 900px without complaint (range is a WARN),
    // so this is the assertion that the handle did the clamping.
    await expect
      .poll(async () => {
        const doc = (await (await request.get(SETTINGS_URL)).json()) as {
          ui?: { tokens?: { layout?: Record<string, string> } }
        }
        return doc.ui?.tokens?.layout?.sidebar ?? null
      })
      .toBe(`${SIDEBAR_MAX}px`)
  })

  test('one drag is exactly one PUT, whatever the frame count', async ({ page }) => {
    await gotoApp(page)
    const puts: string[] = []
    page.on('request', (req) => {
      if (req.method() === 'PUT' && req.url().includes('/issues/settings/')) puts.push(req.url())
    })

    const layoutBox = await page.getByTestId('issue-layout').boundingBox()
    await dragHandle(page, 'layout-resize-sidebar', Math.round((layoutBox?.x ?? 0) + 310), 40)
    await expect.poll(() => puts.length).toBe(1)
    // Hold it: a debounced-but-repeating save would show up late.
    await page.waitForTimeout(1_500)
    expect(puts, 'one drag, one save').toHaveLength(1)
  })

  test('double-clicking a handle unsets the token rather than pinning a px', async ({
    page,
    request,
  }) => {
    await gotoApp(page)
    const layoutBox = await page.getByTestId('issue-layout').boundingBox()
    await dragHandle(page, 'layout-resize-sidebar', Math.round((layoutBox?.x ?? 0) + 240))
    expect(await sidebarWidth(page)).toBe(240)

    await page.getByTestId('layout-resize-sidebar').dblclick()
    await expect.poll(async () => sidebarWidth(page)).toBe(SIDEBAR_DEFAULT)
    await expect
      .poll(async () => {
        const doc = (await (await request.get(SETTINGS_URL)).json()) as {
          ui?: { tokens?: { layout?: Record<string, string> } }
        }
        return doc.ui?.tokens?.layout?.sidebar ?? 'unset'
      })
      .toBe('unset')
  })

  test('the handle is reachable and resizable from the keyboard', async ({ page }) => {
    await gotoApp(page)
    const handle = page.getByTestId('layout-resize-sidebar')
    await handle.focus()
    await expect(handle).toBeFocused()
    await page.keyboard.press('ArrowRight')
    await page.keyboard.press('ArrowRight')
    expect(await sidebarWidth(page)).toBe(SIDEBAR_DEFAULT + 16)
    await page.keyboard.press('Shift+ArrowLeft')
    expect(await sidebarWidth(page)).toBe(SIDEBAR_DEFAULT + 15)
  })

  test('the list column is draggable too, and lands in its own token', async ({
    page,
    request,
  }) => {
    await gotoApp(page)
    const layoutBox = await page.getByTestId('issue-layout').boundingBox()
    const left = layoutBox?.x ?? 0
    const before = await page.getByTestId('terminal-split').boundingBox()
    await dragHandle(page, 'layout-resize-list', Math.round(left + 272 + 600))
    const after = await page.getByTestId('terminal-split').boundingBox()
    expect(Math.round(after?.width ?? 0)).toBe(600)
    expect(Math.round(after?.width ?? 0)).not.toBe(Math.round(before?.width ?? 0))

    await expect
      .poll(async () => {
        const doc = (await (await request.get(SETTINGS_URL)).json()) as {
          ui?: { tokens?: { layout?: Record<string, string> } }
        }
        return doc.ui?.tokens?.layout?.list ?? null
      })
      .toBe('600px')
  })
})

/*
 * GDK-1815: the third seam. The dock's grip had a pointer and nothing else —
 * the only drag affordance in the app with no keyboard door — and it is now
 * the same component the two column grips are, so what is worth measuring in
 * a browser is that the shared component really is mounted on this seam and
 * really moves THIS pane.
 *
 * Deliberately not in `terminal*.spec.ts`: nothing here reads the terminal
 * buffer, so none of the wide-prompt shell conditions (24-column prompt,
 * title OSC, no banner) can reach it, and being in that glob would put a
 * geometry test behind a shell-environment gate for nothing.
 *
 * The rungs below this one are already taken: the arrow mapping and the
 * paint-many/save-once cadence are unit tests
 * (web/src/lib/resize-grip.test.ts), and "the dock has a palette row at all"
 * is the coverage gate (web/src/lib/palette-coverage.test.ts). What is left
 * genuinely needs a browser: focus, a real pane box, and the clamp against a
 * real window height.
 */
test.describe('the terminal dock grip', () => {
  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })

  async function openDock(page: Page): Promise<void> {
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
  }

  async function dockHeight(page: Page): Promise<number> {
    const box = await page.getByTestId('terminal-pane').boundingBox()
    return Math.round(box?.height ?? 0)
  }

  test('announces itself as a slider on the vertical axis', async ({ page }) => {
    await gotoApp(page)
    await openDock(page)
    const grip = page.getByTestId('terminal-resize')
    await expect(grip).toHaveAttribute('role', 'slider')
    // The direction the VALUE moves, which is the direction of the arrow
    // keys — the opposite of the way the seam is drawn.
    await expect(grip).toHaveAttribute('aria-orientation', 'vertical')
    const now = Number(await grip.getAttribute('aria-valuenow'))
    const min = Number(await grip.getAttribute('aria-valuemin'))
    const max = Number(await grip.getAttribute('aria-valuemax'))
    expect(now, 'the announced height is the painted one').toBe(await dockHeight(page))
    expect(min).toBeLessThan(now)
    expect(max).toBeGreaterThan(now)
  })

  test('arrows resize the dock and Backspace puts it back', async ({ page }) => {
    await gotoApp(page)
    await openDock(page)
    const start = await dockHeight(page)
    const grip = page.getByTestId('terminal-resize')
    await grip.focus()
    await expect(grip).toBeFocused()

    // Up is taller: the grip is on the dock's TOP edge.
    await page.keyboard.press('ArrowUp')
    await page.keyboard.press('ArrowUp')
    expect(await dockHeight(page)).toBe(start + 16)
    await page.keyboard.press('Shift+ArrowDown')
    expect(await dockHeight(page)).toBe(start + 15)
    await expect(grip).toHaveAttribute('aria-valuenow', String(start + 15))

    await page.keyboard.press('Backspace')
    expect(await dockHeight(page), 'back to the shipped height').toBe(start)
  })

  test('the palette row leaves the keyboard on the grip', async ({ page }) => {
    await gotoApp(page)
    await openDock(page)
    await page.keyboard.press('ControlOrMeta+k')
    const row = page.getByTestId('palette-action-resize-terminal')
    await expect(row).toBeVisible()
    await row.click()
    // Focus-shaped, not click-shaped: the row hands over the keyboard and
    // the grip's own handler does the sizing.
    await expect(page.getByTestId('terminal-resize')).toBeFocused()
    const start = await dockHeight(page)
    await page.keyboard.press('ArrowUp')
    expect(await dockHeight(page)).toBe(start + 8)
  })
})
