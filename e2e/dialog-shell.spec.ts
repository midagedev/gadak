import { expect, test, type Locator, type Page, type Route } from '@playwright/test'
import { gotoApp, openServerSettings } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/**
 * GDK-297: six same-class modal dialogs must share one shell contract.
 * Visual chrome lives in web/src/components/ui/DialogShell.svelte (GDK-316);
 * this table is still the contract a seventh row has to join.
 *
 * Driven from DIALOGS below so a seventh is one row — a test that only
 * asserted `role=dialog` exists would pass every inconsistency this
 * round is closing.
 *
 * GDK-649: importer ↔ row coverage lives in e2e/dialog-shell.unit.ts (file
 * walk, no browser). This file is the shell contract in a real dialog.
 *
 * These assertions will not catch:
 * - Backdrop-click dismiss, focus-trap Tab cycling, or which control is
 *   focused on open (trapFocus is a different owner).
 * - Whether Close vs Cancel is the *right* word beyond the per-row
 *   label in this table (the table is the decision; the test only
 *   checks the row).
 * - Visual language (colour, radius, type scale) and whether the X is
 *   optically in the header vs a sibling that still has an accessible name.
 * - Dialogs that carry role=dialog but are a different class (MediaViewer,
 *   BulkBar, CommandPalette, AssigneePicker) — they are not in the table.
 * - Mid-list rows cut by overflow at the fold (only the last row of the
 *   scroller, at scroll-end, is measured).
 * - i18n of the labels (the suite forces en).
 */

const CLOSE_ESC = en['common.closeEsc']
const VIEWPORTS = [
  { width: 1280, height: 900 },
  { width: 1280, height: 720 },
] as const

type DialogRow = {
  id: string
  hasCommit: boolean
  dismissLabel: string | null
  primaryLabel: string | null
  open: (page: Page) => Promise<void>
  locate: (page: Page) => Locator
}

const CREATE_PROJECTS = [
  {
    key: 'NMB',
    name: 'Numbers',
    issue_types: [
      { id: '10001', name: 'Task' },
      { id: '10004', name: 'Bug' },
    ],
  },
]

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

/** Same seed as e2e/duedate.spec.ts — fixture Jira never answers create-meta. */
async function stubCreateMeta(page: Page): Promise<void> {
  await page.route('**/api/v1/issues/meta/write/', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await fulfillJSON(route, {
      transitions: {},
      create_meta: { projects: CREATE_PROJECTS },
      updated_at: '2026-08-18T00:00:00.000Z',
    })
  })
  await page.route('**/api/v1/issues/create-meta/**', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    if (route.request().url().includes('/create-meta/fields')) {
      await fulfillJSON(route, { fields: [] })
      return
    }
    await fulfillJSON(route, { projects: CREATE_PROJECTS })
  })
  await page.route('**/priorities/', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await fulfillJSON(route, { priorities: [{ id: '3', name: '보통' }] })
  })
}

/** Same delta inject as e2e/update-notice.spec.ts. */
async function injectNotesDelta(page: Page): Promise<void> {
  const latest = '0.99.0'
  const release = `https://github.com/midagedev/gadak/releases/tag/v${latest}`
  const notes = 'Fixed the flaky upload.\nSecond line.'
  await page.route((url) => url.pathname.includes('/delta/'), async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as Record<string, unknown>
    await route.fulfill({
      response,
      json: { ...body, latest_version: latest, release_url: release, release_notes: notes },
    })
  })
}

async function flushDelta(page: Page): Promise<void> {
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => 'visible',
    })
    document.dispatchEvent(new Event('visibilitychange'))
  })
}

async function pressUntilCursor(page: Page): Promise<void> {
  await page.keyboard.press('j')
  await expect
    .poll(async () =>
      page.evaluate(() => document.querySelector('[data-cursor="true"]')?.getAttribute('data-issue-key') ?? ''),
    )
    .not.toEqual('')
}

type ShellGeometry = {
  pinnedBelowBody: number
  clipOverflow: number
  clipScrolls: boolean
  clipLastTag: string
  overlap: number
  note: string
}

/**
 * Geometry of the contract — copied from e2e/scroll-region.spec.ts's
 * last-child-vs-client-box measure (including the clientTop/clientHeight
 * guard) and extended with pinned-footer count + body/footer overlap.
 */
async function measureShell(dialog: Locator): Promise<ShellGeometry> {
  return dialog.evaluate((rootEl) => {
    const root = rootEl as HTMLElement

    const isScroll = (el: HTMLElement): boolean => {
      const s = getComputedStyle(el)
      return (
        el.classList.contains('scroll-region') ||
        s.overflowY === 'auto' ||
        s.overflowY === 'scroll'
      )
    }
    const isPinned = (el: HTMLElement): boolean => {
      const s = getComputedStyle(el)
      return s.flexShrink === '0' && !isScroll(el)
    }

    const pinnedBelow = (node: HTMLElement): { count: number; pins: HTMLElement[]; body: HTMLElement | null } => {
      const kids = [...node.children] as HTMLElement[]
      const idx = kids.findIndex(isScroll)
      if (idx >= 0) {
        const pins = kids.slice(idx + 1).filter(isPinned)
        return { count: pins.length, pins, body: kids[idx] }
      }
      for (const k of kids) {
        const s = getComputedStyle(k)
        if ((s.display.includes('flex') || k.tagName === 'FORM') && k.children.length) {
          const inner = pinnedBelow(k)
          if (inner.body) return inner
        }
      }
      return { count: 0, pins: [], body: null }
    }

    const { count: pinnedBelowBody, pins, body } = pinnedBelow(root)

    const lastContentBottom = (el: HTMLElement): { bottom: number; tag: string } => {
      const kids = [...el.children].filter((c) => (c as HTMLElement).offsetHeight > 0) as HTMLElement[]
      if (kids.length) {
        const last = kids[kids.length - 1]
        return { bottom: last.getBoundingClientRect().bottom, tag: last.tagName.toLowerCase() }
      }
      const range = document.createRange()
      range.selectNodeContents(el)
      const rects = [...range.getClientRects()]
      const last = rects[rects.length - 1]
      return {
        bottom: last ? last.bottom : el.getBoundingClientRect().bottom,
        tag: el.tagName.toLowerCase(),
      }
    }

    const nodes = [root, ...root.querySelectorAll('*')] as HTMLElement[]
    const scroller = nodes.find((n) => {
      const s = getComputedStyle(n)
      return (
        (s.overflowY === 'auto' || s.overflowY === 'scroll') &&
        n.scrollHeight > n.clientHeight + 1
      )
    })

    let clipOverflow = 0
    let clipScrolls = false
    let clipLastTag = ''
    let note = 'fits'

    if (scroller) {
      scroller.scrollTop = scroller.scrollHeight
      const last = lastContentBottom(scroller)
      const box = scroller.getBoundingClientRect()
      const clientBottom = box.top + scroller.clientTop + scroller.clientHeight
      clipOverflow = last.bottom - clientBottom
      clipScrolls = true
      clipLastTag = last.tag
      note = 'overflowing-scroller'
    } else {
      const last = lastContentBottom(body ?? root)
      const box = root.getBoundingClientRect()
      const clientBottom = box.top + root.clientTop + root.clientHeight
      clipOverflow = last.bottom - clientBottom
      clipLastTag = last.tag
      note = body ? 'body-fits' : 'dialog-fits'
    }

    // Visual paint-over, not layout-box overflow: a descendant's
    // getBoundingClientRect still extends past an overflow:auto clip, which
    // is not "paints over the footer". Hit-test the first pixel of each pin.
    let overlap = 0
    if (body && pins.length) {
      for (const pin of pins) {
        const p = pin.getBoundingClientRect()
        if (p.width < 1 || p.height < 1) continue
        const hit = document.elementFromPoint(p.left + Math.min(24, p.width / 2), p.top + 1)
        if (hit && body.contains(hit) && !pin.contains(hit)) {
          overlap = Math.max(overlap, 2)
        }
      }
    }

    return {
      pinnedBelowBody,
      clipOverflow,
      clipScrolls,
      clipLastTag,
      overlap,
      note,
    }
  })
}

const DIALOGS: DialogRow[] = [
  {
    id: 'settings',
    hasCommit: true,
    dismissLabel: en['common.cancel'],
    primaryLabel: en['common.save'],
    open: async (page) => {
      await gotoApp(page)
      await openServerSettings(page)
    },
    locate: (page) => page.getByTestId('settings-dialog'),
  },
  {
    id: 'new-issue',
    hasCommit: true,
    dismissLabel: en['common.cancel'],
    primaryLabel: en['common.create'],
    open: async (page) => {
      await stubCreateMeta(page)
      await gotoApp(page)
      await page.getByRole('button', { name: en['write.newIssue'], exact: true }).click()
      const dialog = page.getByTestId('new-issue-dialog')
      await expect(dialog).toBeVisible()
      await expect(dialog.getByPlaceholder(en['write.issueTitle'])).toBeVisible()
    },
    locate: (page) => page.getByTestId('new-issue-dialog'),
  },
  {
    id: 'shortcuts',
    hasCommit: false,
    dismissLabel: null,
    primaryLabel: null,
    open: async (page) => {
      await gotoApp(page)
      await page.keyboard.press('?')
    },
    locate: (page) => page.getByTestId('shortcuts-dialog'),
  },
  {
    id: 'jira-credentials',
    hasCommit: true,
    dismissLabel: en['common.cancel'],
    primaryLabel: en['jiraSettings.replaceToken'],
    open: async (page) => {
      await gotoApp(page)
      await page.getByRole('button', { name: en['sidebar.jiraCreds'], exact: true }).click()
    },
    locate: (page) => page.getByRole('dialog', { name: en['jiraSettings.title'] }),
  },
  {
    id: 'quick-comment',
    hasCommit: false,
    dismissLabel: null,
    primaryLabel: null,
    open: async (page) => {
      await gotoApp(page)
      await pressUntilCursor(page)
      await page.keyboard.press('c')
    },
    locate: (page) => page.getByTestId('quick-comment'),
  },
  {
    id: 'update-notes',
    hasCommit: false,
    dismissLabel: null,
    primaryLabel: null,
    open: async (page) => {
      await injectNotesDelta(page)
      await gotoApp(page)
      await flushDelta(page)
      const notice = page.getByTestId('update-notice')
      await expect(notice).toBeVisible()
      await notice.click()
    },
    locate: (page) => page.getByTestId('update-notes'),
  },
]

test.describe('dialog shell contract', () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  for (const row of DIALOGS) {
    test(`${row.id} matches the shell contract`, async ({ page }) => {
      await row.open(page)
      const dialog = row.locate(page)
      await expect(dialog, `${row.id} must open from its real entry point`).toBeVisible()

      const headerClose = dialog.getByRole('button', { name: CLOSE_ESC })
      await expect
        .soft(headerClose, `${row.id}: header close control with accessible name "${CLOSE_ESC}"`)
        .toBeVisible()

      // 'common.close' ("Close") was removed in GDK-621 — every close X now
      // says closeEsc. The excluded name stays the bare word: a footer Close
      // must not reappear next to the header X's "Close (Esc)".
      const exactClose = dialog.getByRole('button', { name: 'Close', exact: true })
      const exactCancel = dialog.getByRole('button', { name: en['common.cancel'], exact: true })

      if (row.hasCommit) {
        const dismiss = dialog.getByRole('button', { name: row.dismissLabel!, exact: true })
        await expect
          .soft(dismiss, `${row.id}: footer dismiss "${row.dismissLabel}"`)
          .toBeVisible()

        const footer = dialog.locator('[data-dialog-footer]')
        const footerCount = await footer.count()
        if (footerCount > 0) {
          const buttons = footer.getByRole('button')
          const last = buttons.last()
          await expect
            .soft(last, `${row.id}: primary "${row.primaryLabel}" is last footer button`)
            .toHaveText(row.primaryLabel!)
        } else {
          // FAIL-first path: no data-dialog-footer yet. Fall back to DOM order
          // of the dismiss + primary among visible dialog buttons excluding the
          // header X, so the unmodified tree still names the real mismatch.
          const order = await dialog.evaluate(
            (root, labels: { dismiss: string; primary: string; closeEsc: string }) => {
              const buttons = [...root.querySelectorAll('button')].filter((b) => {
                const name = (b.getAttribute('aria-label') || b.textContent || '').trim()
                return name !== labels.closeEsc
              })
              const texts = buttons.map((b) => (b.textContent || '').trim())
              const di = texts.lastIndexOf(labels.dismiss)
              const pi = texts.lastIndexOf(labels.primary)
              return { texts, di, pi }
            },
            { dismiss: row.dismissLabel!, primary: row.primaryLabel!, closeEsc: CLOSE_ESC },
          )
          expect
            .soft(
              order.di >= 0,
              `${row.id}: dismiss "${row.dismissLabel}" among dialog buttons (got ${JSON.stringify(order.texts)})`,
            )
            .toBe(true)
          expect
            .soft(
              order.pi > order.di,
              `${row.id}: primary "${row.primaryLabel}" after dismiss in DOM order (di=${order.di} pi=${order.pi} texts=${JSON.stringify(order.texts)})`,
            )
            .toBe(true)
        }
      } else {
        await expect
          .soft(exactClose, `${row.id}: no footer Close (X + Esc are the way out)`)
          .toHaveCount(0)
        await expect
          .soft(exactCancel, `${row.id}: no footer Cancel`)
          .toHaveCount(0)
      }

      for (const vp of VIEWPORTS) {
        await page.setViewportSize(vp)
        const g = await measureShell(dialog)
        expect
          .soft(
            g.pinnedBelowBody,
            `${row.id} @${vp.width}x${vp.height}: at most one non-scrolling bottom-pinned region (pinnedBelowBody=${g.pinnedBelowBody} note=${g.note})`,
          )
          .toBeLessThanOrEqual(1)
        expect
          .soft(
            g.overlap,
            `${row.id} @${vp.width}x${vp.height}: body content must not paint over a pinned footer (overlap=${g.overlap.toFixed(2)}px)`,
          )
          .toBeLessThanOrEqual(1)
        expect
          .soft(
            g.clipOverflow,
            `${row.id} @${vp.width}x${vp.height}: last <${g.clipLastTag}> overflows scroller by ${g.clipOverflow.toFixed(2)}px (scrolls=${g.clipScrolls} note=${g.note})`,
          )
          .toBeLessThanOrEqual(1)
      }

      await page.keyboard.press('Escape')
      await expect(dialog, `${row.id}: Esc closes`).toHaveCount(0)
    })
  }
})
