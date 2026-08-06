import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/**
 * The people axis end to end: find a person in the palette, read what they
 * wrote, and leave through one of the quick links.
 *
 * The fixture mirror has exactly one member — Alex Kim / demo@example.com, the
 * account id every comment in the snapshot carries — with 634 comments, 72
 * assigned issues and 534 reported ones.
 */
test.describe('people axis', () => {
  test('palette finds a person, the panel lists their comments, a quick link filters the list', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await page.waitForTimeout(300)

    // Members ride the bootstrap payload, so matching a person must not cost a
    // request — the same contract the issue section keeps.
    const apiDuringType: string[] = []
    page.on('request', (req) => {
      if (req.url().includes('/api/')) apiDuringType.push(req.url())
    })

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()

    await page.keyboard.type('alex', { delay: 20 })
    await expect(palette.getByText('People', { exact: true })).toBeVisible()

    const personRow = palette.getByTestId('palette-person-row')
    await expect(personRow).toHaveCount(1)
    await expect(personRow).toContainText('Alex Kim')
    await expect(personRow).toContainText('demo@example.com')

    // People lead the list, so Enter opens the person rather than an issue.
    const first = palette.getByRole('option').first()
    await expect(first).toHaveAttribute('aria-selected', 'true')
    await expect(first).toContainText('Alex Kim')

    expect(
      apiDuringType,
      `expected no /api/ requests while typing, got:\n${apiDuringType.join('\n')}`,
    ).toEqual([])

    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const panel = page.getByTestId('person-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByTestId('person-name')).toHaveText('Alex Kim')
    await expect(panel.getByText('demo@example.com')).toBeVisible()

    // Comments — the leg only this axis can answer.
    const rows = panel.getByTestId('person-comment')
    await expect(rows.first()).toBeVisible({ timeout: 15_000 })
    expect(await rows.count()).toBeGreaterThan(1)
    // Section count is the author's full total, and the cap line says so.
    await expect(panel.getByText('634', { exact: true })).toBeVisible()
    await expect(panel.getByTestId('person-comment-cap')).toContainText('634')

    // Quick links carry counts computed from the local pool.
    await expect(panel.getByTestId('person-link-assigned')).toContainText('72')
    await expect(panel.getByTestId('person-link-reported')).toContainText('534')
    await expect(panel.getByTestId('person-link-docs')).toContainText('6')

    // …and the count is exactly what the list lands on.
    await panel.getByTestId('person-link-assigned').click()
    await expect(page.getByText('72 issues')).toBeVisible()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  // The list spans both kinds of parent — the mirror joins issue and page
  // comments into one answer, and a row has to open whichever it belongs to.
  test('a comment row opens what it was written on, replacing the person panel', async ({
    page,
  }) => {
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    await page.keyboard.type('alex', { delay: 20 })
    await page.keyboard.press('Enter')

    const panel = page.getByTestId('person-panel')
    await expect(panel.getByTestId('person-comment').first()).toBeVisible({ timeout: 15_000 })

    const issueRow = panel.locator('[data-comment-kind="issue"]').first()
    const issueKey = await issueRow.getAttribute('data-comment-key')
    expect(issueKey).toBeTruthy()
    await issueRow.click()
    // One right panel at a time: the issue takes it over.
    await expect(page.getByTestId('person-panel')).toBeHidden()
    const detail = page.getByTestId('issue-detail-panel')
    await expect(detail.getByText(issueKey as string).first()).toBeVisible()

    // Back to the person, then the other kind.
    await page.keyboard.press('ControlOrMeta+k')
    await page.keyboard.type('alex', { delay: 20 })
    await page.keyboard.press('Enter')
    const pageRow = panel.locator('[data-comment-kind="page"]').first()
    await expect(pageRow).toBeVisible({ timeout: 15_000 })
    await pageRow.click()
    await expect(page.getByTestId('person-panel')).toBeHidden()
    await expect(page.getByTestId('doc-panel')).toBeVisible()
  })

  test('the docs quick link lands on the By author tab', async ({ page }) => {
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    await page.keyboard.type('alex', { delay: 20 })
    await page.keyboard.press('Enter')

    const panel = page.getByTestId('person-panel')
    await expect(panel).toBeVisible()
    await panel.getByTestId('person-link-docs').click()

    const docs = page.getByTestId('docs-view')
    await expect(docs).toBeVisible()
    await expect(docs.getByTestId('docs-tab').filter({ hasText: 'By author' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    await expect(
      docs.getByTestId('docs-author-group').filter({ hasText: 'Alex Kim' }),
    ).toBeVisible()
  })

  test('Esc closes the person panel', async ({ page }) => {
    await gotoApp(page)

    await page.keyboard.press('ControlOrMeta+k')
    await page.keyboard.type('alex', { delay: 20 })
    await page.keyboard.press('Enter')

    await expect(page.getByTestId('person-panel')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('person-panel')).toBeHidden()
  })
})
