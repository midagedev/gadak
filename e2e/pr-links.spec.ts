/*
 * Linked PRs rendered from the committed fixture (GDK-1755). The dev panel
 * used to reach e2e only through serve.sh's single injected 'open' row on
 * NMB-139 — the merged/declined chip classes were exercised by no test, and
 * examples/demo.db itself carried 0 dev_links rows so no other demo surface
 * (desktop, hosted) could show the section either. tools/demo-enrich now
 * seeds three PRs on NMB-5 straight into examples/demo-source.db, covering
 * the whole jira.DevPRStatus vocabulary; this spec reads them back through
 * the real detail path (store → MergedPRLinks → PrList) the way a user does.
 *
 * The second test is the GDK-114④ surfacing: a comment whose /wiki/spaces/…
 * URL was extracted into item_refs by store's own grammar shows up as a
 * Related doc on the issue, not just as raw text in the comment.
 */
import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

async function openDetail(page: Page, key: string) {
  const input = searchInput(page)
  await input.fill(key)
  await page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: key })
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('linked PRs (GDK-1755)', () => {
  test('all three PR states render with repo, title, and who linked them', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    const panel = await openDetail(page, 'NMB-5')
    await expect(panel.locator('a[href^="https://github.com/nimbus-dev/board/pull/"]')).toHaveCount(3)

    // One row per PR: the state chip is DevPRStatus.Stored() verbatim — a
    // mapping regression that stores 'DECLINED' or '' shows up as the wrong
    // (or missing) chip text, so the exact vocabulary is asserted per URL.
    const pr = (n: number) => panel.locator(`a[href="https://github.com/nimbus-dev/board/pull/${n}"]`)
    await expect(pr(479).locator('span').first()).toHaveText(/declined/i)
    await expect(pr(482).locator('span').first()).toHaveText(/merged/i)
    await expect(pr(517).locator('span').first()).toHaveText(/open/i)

    // repo#number is parsed from the URL (githubPRURL), repo = last path
    // segment of the org/repo pair.
    await expect(pr(479)).toContainText('board#479')
    await expect(pr(482)).toContainText('board#482')
    await expect(pr(517)).toContainText('board#517')

    // Titles and the actor trail (dev_links actor_name, GDK-589 — distinct
    // from each PR's own author).
    await expect(pr(482)).toContainText('Fix contrast token lookup for empty workspaces')
    await expect(panel.getByText('Linked by Priya Sharma')).toHaveCount(3)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a wiki-URL comment surfaces the page as a related doc', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)

    // NMA-100 carries the fixture's comment naming the Component Map page;
    // item_refs → ref_pages → RelatedDocs is the derived path under test.
    const panel = await openDetail(page, 'NMA-100')
    const row = panel
      .getByTestId('related-doc-row')
      .filter({ hasText: 'Component Map — Platform API' })
    await expect(row).toHaveCount(1)
    await expect(row.first()).toHaveAttribute('data-doc-key', /131106/)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
