import { spawnSync } from 'node:child_process'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { test, expect } from '@playwright/test'
import { attachConsoleErrors, e2eHomeDir, gotoApp, DEMO_ISSUE_COUNT_RE } from './helpers'

const here = path.dirname(fileURLToPath(import.meta.url))
const gadakBin = path.join(here, '.tmp/gadak')
const gadakHome = e2eHomeDir()

test.describe('JQL paste', () => {
  test('Enter on navigator JQL applies chips and Copy link carries them back as a navigator URL', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await gotoApp(page)

    const box = page.getByTestId('search-input')
    await box.click()
    await box.fill('project = NMA AND statusCategory = "In Progress"')
    await box.press('Enter')

    await expect(page.getByTestId('view-copy-link')).toBeVisible({ timeout: 10_000 })
    await expect(page).toHaveURL(/pj=NMA/)
    await expect(page).toHaveURL(/sc=inprogress/)

    const emit = page.waitForResponse(
      (r) => r.url().includes('/jql/emit/') && r.request().method() === 'POST',
    )
    await page.getByTestId('view-copy-link').click()
    const res = await emit
    expect(res.ok()).toBeTruthy()
    const body = (await res.json()) as { jql: string }
    expect(body.jql).toContain('project = NMA')
    expect(body.jql).toMatch(/statusCategory = "In Progress"/)

    // GDK-1343: the same three lines the issue detail copies — the origin's
    // own address for this view (the navigator with the JQL), then the app.
    const origin = new URL(page.url()).origin
    const hash = new URL(page.url()).hash.replace(/^#\/?\??/, '')
    const want = `https://nimbus.example.com/issues/?jql=${encodeURIComponent(body.jql)}\ngadak://view?${hash}\n${origin}/#/?${hash}`
    await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe(want)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a mirrored Jira filter applies from the sidebar', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const section = page.getByTestId('sidebar-jira-filters')
    await expect(section).toBeVisible()
    await expect(section.getByTestId('sidebar-jira-filter')).toContainText('Open in NMA')
    await section.getByTestId('sidebar-jira-filter').click()
    await expect(page).toHaveURL(/pj=NMA/)
    await expect(page).toHaveURL(/sc=inprogress/)

    const open = section.getByTestId('sidebar-jira-filter-open')
    await expect(open).toHaveAttribute('href', /\/issues\/\?filter=e2e-open-nma/)
    await expect(open).toHaveAttribute('target', '_blank')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('gadak views open focuses the running window', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await expect(page).not.toHaveURL(/pj=NMA/)

    const ran = spawnSync(
      gadakBin,
      [
        'views',
        'open',
        '--jql',
        'project = NMA AND statusCategory = "In Progress"',
        '--no-open',
        '--json',
      ],
      { env: { ...process.env, GADAK_HOME: gadakHome }, encoding: 'utf8' },
    )
    expect(ran.status, ran.stderr || ran.stdout).toBe(0)
    expect(ran.stdout).toContain('pj=NMA')

    await expect(page).toHaveURL(/pj=NMA/, { timeout: 5_000 })
    await expect(page).toHaveURL(/sc=inprogress/)
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'NMA' })).toBeVisible()
    await expect(page.getByTestId('filter-chip').filter({ hasText: 'In Progress' })).toBeVisible()
    await expect(page.getByTestId('list-count')).not.toHaveText(DEMO_ISSUE_COUNT_RE)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
