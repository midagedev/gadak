import {
  LINEAR_ISSUE_COUNT_RE,
  appConsoleErrors,
  attachConsoleErrors,
  expect,
  forceLocale,
  linearApiURL,
  searchInput,
  test,
  type Page,
} from './helpers'
import type { Locator } from '@playwright/test'

/*
 * GDK-1298: the Linear fixture (examples/demo-linear.db) served on the
 * suite's second port, exercised through the real mirror — no route
 * patching. detail.spec.ts's "Linear origin" describe (GDK-1149) proves the
 * same affordances by rewriting a Jira bootstrap in flight; this file proves
 * them against a mirror sync.RunLinear actually wrote: the deeplink is the
 * stored items.url, the link labels are the mirrored catalog's own phrases,
 * the write refusal is the server's own 400, and no tracker-naming surface
 * says Jira.
 *
 * The keys are the fixture's hand-shaped relation set
 * (tools/seed-demo/linear_seed.go linearFixtureRelations): LNX-1 owns a
 * blocks and a related relation, LNX-2 a duplicate — and sees LNX-1's blocks
 * back on its inward side.
 */

/** The URL prefix Linear mints for this workspace's issues (items.url). */
const LINEAR_URL_PREFIX = 'https://linear.app/example/issue/'

async function bootLinear(page: Page): Promise<void> {
  await forceLocale(page, 'en')
  await page.goto(linearApiURL('/'))
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  // Sidebar pool size, bare digits for the same locale-proof reason gotoApp
  // waits on DEMO_ISSUE_COUNT_RE.
  await expect(page.getByText(LINEAR_ISSUE_COUNT_RE).first()).toBeVisible({ timeout: 30_000 })
  // Startup view committed (the GDK-39 wait, adapted to this port).
  await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
  // The same mine-steer gotoApp does: Dana (the config's account) has open
  // assigned work in this fixture too, so a fresh context lands on "My
  // issues" — and LNX-2 is Marco's. The open pool without a group axis: a
  // Linear mirror has no epics, so gotoApp's g=epic address has no meaning
  // here.
  if (/[#?&]fl=mine(&|$)/.test(page.url())) {
    const mineCount = await page.getByTestId('list-count').textContent()
    await page.goto(linearApiURL('/#/?sc=new%2Cinprogress'))
    await expect(page).not.toHaveURL(/[#?&]fl=mine(&|$)/)
    await expect(page.getByTestId('list-count')).not.toHaveText(mineCount ?? '')
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  }
}

/** Open `key`'s detail panel by searching the pool, and return the panel. */
async function openIssue(page: Page, key: string): Promise<Locator> {
  await searchInput(page).fill(key)
  // \b so LNX-1 does not also hit LNX-10..19 — digits are word chars, so the
  // boundary fails inside "LNX-10" and holds at "LNX-1 " / "LNX-1\n".
  await page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: new RegExp(`\\b${key}\\b`) })
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

/** The label span of one linked-issue row (the phrase, never the direction token). */
function linkRow(panel: Locator, key: string): Locator {
  return panel.getByTestId('linked-issues').getByRole('button').filter({ hasText: key })
}

test.describe('linear fixture — the origin the mirror says it is', () => {
  test('deeplink and owning-side link labels (LNX-1)', async ({ page }) => {
    const errors = appConsoleErrors(attachConsoleErrors(page))
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await bootLinear(page)

    const panel = await openIssue(page, 'LNX-1')

    // The key anchor is the page Linear minted (items.url, identifier-uuid
    // form — the uuid is the seeder's, so pin the prefix) and its title names
    // the tracker the mirror actually speaks.
    const anchor = panel.getByRole('link', { name: 'LNX-1', exact: true }).first()
    await expect(anchor).toHaveAttribute('href', new RegExp(`^${LINEAR_URL_PREFIX}LNX-1-`))
    await expect(anchor).not.toHaveAttribute('href', /atlassian\.net|\/browse\//)
    await expect(anchor).toHaveAttribute('title', 'Open in Linear')

    // Copy-link names Linear (the real-mirror twin of the GDK-1149 toast).
    await panel.getByTestId('issue-copy-link').click()
    await expect(page.getByTestId('toast')).toContainText('Linear link copied')
    await expect
      .poll(async () => page.evaluate(() => navigator.clipboard.readText()))
      .toContain(`${LINEAR_URL_PREFIX}LNX-1-`)

    // Owning side: the blocks relation renders the catalog's outward phrase,
    // related (no catalog row) falls to the type name. The label is the
    // row's first span — linkLabel's ladder, never the direction token.
    await expect(linkRow(panel, 'LNX-2').locator('span').first()).toHaveText('blocks')
    await expect(linkRow(panel, 'LNX-3').locator('span').first()).toHaveText('Relates')

    // No add form on a Linear origin: the UI refuses what the server refuses.
    await expect(panel.getByTestId('linked-issues-add')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('target-side labels read the catalog inward phrase (LNX-2)', async ({ page }) => {
    const errors = appConsoleErrors(attachConsoleErrors(page))
    await bootLinear(page)

    const panel = await openIssue(page, 'LNX-2')

    // The blocks relation LNX-1 owns comes back as the inward phrase, and
    // the catalog-less duplicate falls to its type name.
    await expect(linkRow(panel, 'LNX-1').locator('span').first()).toHaveText('is blocked by')
    await expect(linkRow(panel, 'LNX-4').locator('span').first()).toHaveText('Duplicate')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('issue-link writes are refused with the Linear sentence', async ({ page }) => {
    const errors = appConsoleErrors(attachConsoleErrors(page))
    await bootLinear(page)

    // The refusal is the Linear writer's own, from origin.ErrNoIssueLinks,
    // before any network — the route reaches AsIssueLinker first
    // (internal/server/write_test.go pins the same sentence).
    // originWritable is false on this serve by design: that bool mirrors
    // HasAtlassianCredential, the Jira-family predicate, and a Linear key
    // deliberately does not flip it. The sentence is the product's
    // write-rejection copy for links on this origin; pinning it here keeps
    // the web's toast wording (writeErrorMessage) and the API in one
    // contract.
    const res = await page.request.post(linearApiURL('/api/v1/issues/LNX-1/link/'), {
      data: { type: 'blocks', key: 'LNX-9' },
    })
    expect(res.status()).toBe(400)
    expect(await res.json()).toMatchObject({
      error: 'linear: issue links are not supported on this origin',
    })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the freshness chip never names Jira', async ({ page }) => {
    const errors = appConsoleErrors(attachConsoleErrors(page))
    await bootLinear(page)

    // Age-proof by construction: only the 'fresh' wording names the tracker
    // (GDK-1325), and the committed fixture ages between regens — so the
    // durable contract is the negative one. A crossover back to "pulled
    // from Jira" fails here in every chip state.
    const chip = page.getByTestId('freshness-chip')
    await expect(chip).toBeVisible()
    await expect(chip).toHaveAttribute('title', /[\s\S]+/)
    await expect(chip).not.toHaveAttribute('title', /Jira/i)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
