import { test, expect, type Page } from '@playwright/test'
import { appConsoleErrors, attachConsoleErrors, forceLocale, gotoApp, openServerSettings } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * GDK-604: one Esc closes only the topmost surface.
 *
 * Pins the same negotiation triage.spec already owns for bulk-then-detail
 * (first Esc clears the selection, second Esc closes the panel) and
 * list-menus-esc owns for a list menu over an open panel. The viewer and
 * the person panel were missing from that contract.
 */

function lastKeyCmd(page: Page): Promise<string | null> {
  return page.locator('html').getAttribute('data-last-key-cmd')
}

async function openIssueWithAttachment(page: Page) {
  await forceLocale(page, 'en')
  await page.goto('/#/?issue=NMB-110')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  await expect(panel).toHaveClass(/is-open/)

  // Gallery tile, not the same file inlined in a comment (adf-media-image).
  const thumb = panel.locator('button.group[aria-label^="Enlarge "]').first()
  await expect(thumb).toBeVisible({ timeout: 15_000 })
  const label = await thumb.getAttribute('aria-label')
  expect(label, 'attachment enlarge control should name the file').toBeTruthy()
  const filename = label!.slice('Enlarge '.length)
  expect(filename.length).toBeGreaterThan(0)
  return { panel, filename, thumb }
}

async function openViewer(page: Page, thumb: ReturnType<Page['locator']>, filename: string) {
  await thumb.click()
  const viewer = page.getByRole('dialog', { name: filename })
  await expect(viewer).toBeVisible()
  return viewer
}

test.describe('Esc negotiation (GDK-604)', () => {
  test('one Esc closes the media viewer and leaves the detail panel open', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const { panel, filename, thumb } = await openIssueWithAttachment(page)
    const viewer = await openViewer(page, thumb, filename)

    await page.keyboard.press('Escape')

    await expect(viewer).toBeHidden()
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)
    await expect(panel.getByText('NMB-110').first()).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('ignore')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Tab stays inside the media viewer', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const { filename, thumb } = await openIssueWithAttachment(page)
    const viewer = await openViewer(page, thumb, filename)

    await expect
      .poll(async () => viewer.evaluate((el) => el.contains(document.activeElement)))
      .toBe(true)

    for (let i = 0; i < 6; i++) {
      await page.keyboard.press('Tab')
      expect(
        await viewer.evaluate((el) => el.contains(document.activeElement)),
        `Tab ${i + 1} left the media viewer`,
      ).toBe(true)
    }

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('one Esc over bulk + person panel clears the selection only', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await page.setViewportSize({ width: 1280, height: 800 })
    await gotoApp(page)

    await page.keyboard.press('j')
    await expect(page.locator('[data-cursor="true"]')).toHaveCount(1)
    await page.keyboard.press('x')
    const bar = page.getByTestId('bulk-bar')
    await expect(bar).toBeVisible()
    await expect(bar.getByText('1 selected')).toBeVisible()

    await page.keyboard.press('ControlOrMeta+k')
    await page.keyboard.type('alex', { delay: 20 })
    await page.keyboard.press('Enter')
    const person = page.getByTestId('person-panel')
    await expect(person).toBeVisible()
    await expect(bar).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(bar).toBeHidden()
    await expect(person).toBeVisible()

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('each Esc dismisses one surface: toast, dialog, panel, column view — in that order', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // GDK-829: a toast is a surface too. The cheapest deterministic toast in
    // the demo app is docs.spec's page-comment 409 — the refused POST toasts
    // the catalog sentence instead of the wire error. The same 409 also
    // opens the replace-token dialog over the doc panel, so this flow pins
    // the whole stack at once: capture toast above a bubble dialog above
    // the panel above the column view (GDK-1565). Before the ordered stack,
    // the second Esc closed the panel AND the dialog together — the panel's
    // listener had registered first, and nothing ranked the dialog above it.
    await page.route('**/api/v1/issues/pages/*/comment/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        json: { error: 'credential_required' },
      })
    })
    await gotoApp(page)
    await page.getByTestId('docs-documents').click()
    await expect(page.getByTestId('docs-view')).toBeVisible()
    // The default tab is Viewed, which a fresh profile has none of — Updated
    // lists the mirrored pages, so the row to open is always there.
    await page.locator('[data-testid="docs-tab"][data-tab="updated"]').click()
    await page.getByTestId('doc-row').first().click()
    const panel = page.getByTestId('doc-panel')
    await expect(panel.getByTestId('doc-comment-composer')).toBeVisible()
    await panel.getByTestId('doc-comment-composer').fill('wt-e2e esc toast')
    await panel.getByTestId('doc-comment-submit').click()

    const toast = page.getByTestId('toast')
    await expect(toast).toBeVisible()
    await expect(toast).toContainText(en['write.needToken'])
    const dialog = page.getByRole('dialog', { name: en['jiraSettings.title'] })
    await expect(dialog).toBeVisible()
    // Focus may rest on the submit button; either way the next Esc must be
    // the toast's, not the field's, the dialog's, or the panel's.
    await page.evaluate(() => {
      const active = document.activeElement
      if (active instanceof HTMLElement) active.blur()
    })

    // 1 — the toast outranks the dialog under it (capture 30 vs bubble 40:
    // the phases are separate ladders, and the toast owns its capture).
    await page.keyboard.press('Escape')
    await expect(toast).toHaveCount(0)
    await expect(dialog).toBeVisible()
    await expect(panel).toBeVisible()
    await expect(page.getByTestId('docs-view')).toBeVisible()

    // The refused POST leaves the draft in the composer, and an unfocused
    // non-empty draft spends an Esc of its own (GDK-462) — clear it (and
    // drop the focus fill() left there) so the next Escs walk the rest of
    // the chain this test pins.
    await panel.getByTestId('doc-comment-composer').fill('')
    await page.evaluate(() => {
      const active = document.activeElement
      if (active instanceof HTMLElement) active.blur()
    })

    // 2 — the dialog outranks the panel under it (bubble dialog 40 vs
    // surface 10): one Esc, the dialog only.
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(panel).toBeVisible()
    await expect(page.getByTestId('docs-view')).toBeVisible()

    // 3 — the panel, then 4 — the column view under it.
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('doc-panel')).toHaveCount(0)
    await expect(page.getByTestId('docs-view')).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(page.getByTestId('docs-view')).toHaveCount(0)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    expect(await lastKeyCmd(page)).toBe('close-docs')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

/*
 * GDK-1565: a modal dialog over an open detail panel is the topmost surface,
 * so one Esc closes only the dialog. The audit found the inverse: the panel's
 * own window listener had registered before the dialog's, saw the unspent
 * key, and cleared the selection — dialog AND panel (and the `issue` URL
 * param) went on one keystroke. The fix is an ordered claim stack owned by
 * lib/dom-actions.ts; these cases pin the dialog-over-panel shape for the
 * three audit dialogs.
 */

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

/** Same recipe as dialog-shell.spec.ts's stubCreateMeta — fixture Jira never
 *  answers create-meta, and the form state is where the dialog's Esc is real. */
async function stubCreateMeta(page: Page): Promise<void> {
  await page.route('**/api/v1/issues/meta/write/', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: {
        transitions: {},
        create_meta: { projects: CREATE_PROJECTS },
        updated_at: '2026-08-18T00:00:00.000Z',
      },
    })
  })
  await page.route('**/api/v1/issues/create-meta/**', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    if (route.request().url().includes('/create-meta/fields')) {
      await route.fulfill({ status: 200, contentType: 'application/json', json: { fields: [] } })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', json: { projects: CREATE_PROJECTS } })
  })
  await page.route('**/priorities/', async (route) => {
    if (route.request().method() !== 'GET') return route.continue()
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      json: { priorities: [{ id: '3', name: 'Medium' }] },
    })
  })
}

/** Detail panel opened from the URL param — the `issue=NMB-110` in the URL is
 *  part of what one Esc must not take away. */
async function openDetailPanel(page: Page) {
  // Docked three-track grid: the toolbar buttons below need to stay clickable.
  await page.setViewportSize({ width: 1440, height: 900 })
  await forceLocale(page, 'en')
  await page.goto('/#/?issue=NMB-110')
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  await expect(panel).toHaveClass(/is-open/)
  return panel
}

test.describe('dialog over detail panel: one Esc closes only the dialog (GDK-1565)', () => {
  test('new-issue dialog: Esc keeps the panel and its URL param', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await stubCreateMeta(page)
    const panel = await openDetailPanel(page)

    await page.getByRole('button', { name: en['write.newIssue'], exact: true }).click()
    const dialog = page.getByTestId('new-issue-dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog.getByPlaceholder(en['write.issueTitle'])).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)
    await expect(page).toHaveURL(/issue=NMB-110/)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('settings dialog: Esc keeps the panel and its URL param', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const panel = await openDetailPanel(page)

    await openServerSettings(page)
    const dialog = page.getByTestId('settings-dialog')
    await expect(dialog).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)
    await expect(page).toHaveURL(/issue=NMB-110/)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('shortcuts dialog: Esc keeps the panel and its URL param', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const panel = await openDetailPanel(page)

    await page.keyboard.press('?')
    const dialog = page.getByTestId('shortcuts-dialog')
    await expect(dialog).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)
    await expect(page).toHaveURL(/issue=NMB-110/)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('one Esc dismisses the top toast and leaves the dialog under it open', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // Same trick as the column-view toast case above: a refused POST answers
    // 409 credential_required, which toasts needToken AND opens the Jira-key
    // settings dialog — the cheapest deterministic toast-over-dialog stack.
    await page.route('**/api/v1/issues/NMB-110/comment/', async (route) => {
      if (route.request().method() !== 'POST') return route.continue()
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        json: { error: 'credential_required' },
      })
    })
    const panel = await openDetailPanel(page)

    const composer = panel.getByTestId('comment-composer')
    await composer.fill('wt-e2e esc toast over dialog')
    await composer.press('Meta+Enter')

    const toast = page.getByTestId('toast')
    await expect(toast).toBeVisible()
    await expect(toast).toContainText(en['write.needToken'])
    const dialog = page.getByRole('dialog', { name: en['jiraSettings.title'] })
    await expect(dialog).toBeVisible()
    // The dialog autofocuses the email field, and the toast declines an Esc
    // typed into a field — blur so the next Esc is the toast's to spend.
    await page.evaluate(() => {
      const active = document.activeElement
      if (active instanceof HTMLElement) active.blur()
    })

    await page.keyboard.press('Escape')
    await expect(toast).toHaveCount(0)
    await expect(dialog).toBeVisible()
    await expect(panel).toBeVisible()
    await expect(panel).toHaveClass(/is-open/)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
