import { test, expect } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'

/**
 * The people axis end to end: find a person in the palette, read what they
 * wrote, and leave through one of the quick links.
 *
 * The fixture mirror seeds four people from assignees with emails (Alex Kim /
 * demo@example.com, Dana/Marco/Priya @example.com). Comments and reporters are
 * redistributed across those four account ids (demo-alex|dana|marco|priya).
 * Alex retains the plurality: 265 comments, 72 assigned, 198 reported
 * (2026-08-06 데모 저자 분산으로 전제 변경 — was single-member / 634 / 534).
 */
test.describe('people axis', () => {
  test('account IDs keep assigned and reported filters working when Jira hides emails', async ({
    page,
  }) => {
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = await response.json()
      for (const issue of body.issues) {
        if (issue.assignee_id === 'demo-alex') issue.assignee_email = null
        if (issue.reporter_id === 'demo-alex') issue.reporter_email = null
      }
      await route.fulfill({ response, json: body })
    })
    await gotoApp(page)

    const openAlex = async () => {
      await page.keyboard.press('ControlOrMeta+k')
      await page.keyboard.type('alex', { delay: 20 })
      await page.keyboard.press('Enter')
      await expect(page.getByTestId('person-panel')).toBeVisible()
    }

    await openAlex()
    const panel = page.getByTestId('person-panel')
    await expect(panel.getByTestId('person-link-assigned')).toContainText('72')
    await panel.getByTestId('person-link-assigned').click()
    await expect(page.getByText('72 issues')).toBeVisible()
    expect(decodeURIComponent(page.url())).toContain('as=demo-alex')

    await openAlex()
    await expect(panel.getByTestId('person-link-reported')).toContainText('198')
    await panel.getByTestId('person-link-reported').click()
    await expect(page.getByText('198 issues')).toBeVisible()
    expect(decodeURIComponent(page.url())).toContain('rp=demo-alex')

    // Existing saved links keep their email token. The member directory maps
    // it to the account ID even though the intercepted issue rows hide email.
    await page.goto('/#/?as=Demo%40Example.com')
    await expect(page.getByText('72 issues')).toBeVisible()
    await page.goto('/#/?rp=DEMO%40example.com')
    await expect(page.getByText('198 issues')).toBeVisible()
  })

  test('a configured user field filters by account ID while displaying the person name', async ({
    page,
  }) => {
    await page.route('**/api/v1/issues/bootstrap/', async (route) => {
      const response = await route.fetch()
      const body = await response.json()
      body.field_specs = [
        ...(body.field_specs ?? []),
        { alias: 'reviewer', label: 'Reviewer', role: 'user' },
      ]
      const project = body.issues[0].project_key
      body.field_usage ??= {}
      body.field_usage[project] ??= {}
      body.field_usage[project].reviewer = 3
      for (const [index, issue] of body.issues.slice(0, 3).entries()) {
        const first = index < 2
        issue.reviewer = first ? 'Reviewer One' : 'Reviewer Two'
        issue.reviewer_account_ids = [first ? 'acc-reviewer-1' : 'acc-reviewer-2']
      }
      await route.fulfill({ response, json: body })
    })
    await gotoApp(page)

    await page.getByRole('button', { name: '+ Filter' }).click()
    await page.getByRole('button', { name: 'Reviewer', exact: true }).click()
    await page.getByRole('button', { name: /Reviewer One\s+2/ }).click()

    await expect(page.getByText('2 issues')).toBeVisible()
    await expect(page.getByRole('button', { name: /Reviewer: Reviewer One/ })).toBeVisible()
    expect(decodeURIComponent(page.url())).toContain('f.reviewer=acc-reviewer-1')
  })

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
    // 2026-08-06 데모 저자 분산: Alex 코멘트 265 (was 634 — FAIL-first confirmed).
    await expect(panel.getByText('265', { exact: true })).toBeVisible()
    await expect(panel.getByTestId('person-comment-cap')).toContainText('265')

    // Quick links carry counts computed from the local pool.
    await expect(panel.getByTestId('person-link-assigned')).toContainText('72')
    // 2026-08-06 데모 저자 분산: Alex 리포터 198 (was 534).
    await expect(panel.getByTestId('person-link-reported')).toContainText('198')
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
