import { type Page, type Route } from '@playwright/test'
import { expect, test } from './helpers'
import { attachConsoleErrors, gotoApp, searchInput } from './helpers'

/**
 * GDK-528: comment visibility (role/group) and JSM internal, end to end.
 *
 * All posts are browser-fulfilled (page.route, the write-through.spec.ts
 * shape) — nothing reaches the serve, so this file is not in the mutating
 * census. The body contract without a live server is pinned byte-for-byte
 * in api.comment-restriction.test.ts (beside api.ts) — here the journey
 * is the point: controls → POST body → badge on the thread row.
 *
 * The demo fixture's origin is Jira, so the controls draw. The non-Jira
 * half rewrites config.json's originType to gadak — the branch asks the
 * origin the server stated (GDK-1152), never identity or workspaceKind.
 */

const KEY = 'NMB-110'
const TYPED = 'cv-e2e typed comment'

type IssueRow = Record<string, unknown> & { issue_key: string; comment_count?: number }

async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

/** Drop fixture-mirror upserts of KEY so a 15s delta cannot overwrite the write. */
async function ignoreFixtureDeltas(page: Page): Promise<void> {
  await page.route((url) => url.pathname.includes('/delta/'), async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { upserted?: IssueRow[] }
    body.upserted = (body.upserted ?? []).filter((it) => it.issue_key !== KEY)
    await route.fulfill({ response, json: body })
  })
}

async function captureIssue(page: Page): Promise<IssueRow> {
  const held: { issue: IssueRow | null } = { issue: null }
  await page.route('**/api/v1/issues/bootstrap/', async (route) => {
    const response = await route.fetch()
    const body = (await response.json()) as { issues: IssueRow[] }
    held.issue = body.issues.find((it) => it.issue_key === KEY) ?? null
    await route.fulfill({ response, json: body })
  })
  await ignoreFixtureDeltas(page)
  await gotoApp(page)
  expect(held.issue, 'fixture bootstrap must include NMB-110').toBeTruthy()
  return held.issue!
}

async function openComposer(page: Page): Promise<{ panel: ReturnType<Page['locator']> }> {
  const input = searchInput(page)
  await input.fill(KEY)
  await page
    .locator('[data-testid="issue-list-scroller"] [role="button"]')
    .filter({ hasText: KEY })
    .first()
    .click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  await expect(panel.getByRole('heading', { name: 'Comments' })).toBeVisible()
  await expect(panel.getByTestId('comment-composer')).toBeVisible()
  return { panel }
}

/** Fulfil the comment POST like write.go after a healthy Jira + refresh. */
async function stubCommentPost(
  page: Page,
  issue: IssueRow,
  echo: Record<string, unknown>,
): Promise<() => Record<string, unknown> | null> {
  let posted: Record<string, unknown> | null = null
  await page.route(`**/api/v1/issues/${KEY}/comment/`, async (route) => {
    if (route.request().method() !== 'POST') return route.continue()
    posted = route.request().postDataJSON() as Record<string, unknown>
    const count = typeof issue.comment_count === 'number' ? issue.comment_count : 0
    await fulfillJSON(route, {
      issue: { ...issue, comment_count: count + 1 },
      comment: {
        comment_id: 'cv-e2e-c1',
        author: 'Dana Whitfield',
        body: 'cv-e2e mirrored comment',
        created_at: '2026-09-11T12:00:00.000Z',
        ...echo,
      },
    })
  })
  return () => posted
}

test.describe('comment visibility (GDK-528)', () => {
  test('untouched controls post the pre-GDK-528 body — no visibility, no internal', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const issue = await captureIssue(page)
    const { panel } = await openComposer(page)

    const restriction = panel.getByTestId('comment-restriction')
    await expect(restriction).toBeVisible()
    // Defaults: Everyone, no name field drawn, internal unchecked.
    await expect(panel.getByTestId('comment-visibility-kind')).toHaveValue('')
    await expect(panel.getByTestId('comment-visibility-value')).toHaveCount(0)
    await expect(panel.getByTestId('comment-internal')).not.toBeChecked()

    const getPosted = await stubCommentPost(page, issue, {})
    const composer = panel.getByTestId('comment-composer')
    await composer.fill(TYPED)
    await composer.press('Meta+Enter')

    await expect.poll(() => getPosted()).toEqual({
      text: TYPED,
      mentions: [],
      attachment_ids: [],
    })
    await expect(composer).toHaveValue('')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('role restriction rides the POST; the echo paints the badge', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const issue = await captureIssue(page)
    const { panel } = await openComposer(page)

    const getPosted = await stubCommentPost(page, issue, {
      visibility_type: 'role',
      visibility_value: 'Administrators',
    })
    await panel.getByTestId('comment-visibility-kind').selectOption('role')
    await panel.getByTestId('comment-visibility-value').fill('Administrators')

    const composer = panel.getByTestId('comment-composer')
    await composer.fill(TYPED)
    await composer.press('Meta+Enter')

    await expect.poll(() => getPosted()).toBeTruthy()
    expect(getPosted(), `POST body ${JSON.stringify(getPosted())}`).toMatchObject({
      text: TYPED,
      visibility: { type: 'role', value: 'Administrators' },
    })

    const badge = panel.getByTestId('comment-restricted-badge')
    await expect(badge).toBeVisible()
    await expect(badge).toContainText('Restricted · Administrators')
    await expect(badge).toHaveAttribute('title', 'Visible only to the role “Administrators”.')

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('internal checkbox rides internal:true; jsd_public:false paints the badge', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    const issue = await captureIssue(page)
    const { panel } = await openComposer(page)

    const getPosted = await stubCommentPost(page, issue, { jsd_public: false })
    await panel.getByTestId('comment-internal').check()

    const composer = panel.getByTestId('comment-composer')
    await composer.fill(TYPED)
    await composer.press('Meta+Enter')

    await expect.poll(() => getPosted()).toBeTruthy()
    expect(getPosted(), `POST body ${JSON.stringify(getPosted())}`).toMatchObject({
      text: TYPED,
      internal: true,
    })

    const badge = panel.getByTestId('comment-internal-badge')
    await expect(badge).toBeVisible()
    await expect(badge).toHaveText('Internal')
    await expect(badge).toHaveAttribute(
      'title',
      'Service desk customers cannot see this comment.',
    )

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a chosen kind with no name blocks the submit — button and ⌘↵ both', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const issue = await captureIssue(page)
    const { panel } = await openComposer(page)
    const getPosted = await stubCommentPost(page, issue, {})

    await panel.getByTestId('comment-visibility-kind').selectOption('group')
    await expect(panel.getByTestId('comment-visibility-value')).toBeVisible()

    const composer = panel.getByTestId('comment-composer')
    await composer.fill(TYPED)

    // The value input is empty: the footer button stays disabled and the
    // hint names the way out. ⌘↵ must not sneak past the disabled button.
    const button = panel.getByRole('button', { name: 'Comment', exact: true })
    await expect(button).toBeDisabled()
    await expect(panel.getByTestId('comment-visibility-value')).toHaveClass(/border-status-reopen/)
    await composer.press('Meta+Enter')
    // poll().toBeNull() would pass on the first tick — the negative needs a
    // real window: nothing may leave, even after the shortcut.
    await page.waitForTimeout(1200)
    expect(getPosted(), 'no POST may leave a blocked composer').toBeNull()

    // Filling the name unblocks; setting the kind back to Everyone alone also
    // would — one path is enough here, the unit suite pins the derivation.
    await panel.getByTestId('comment-visibility-value').fill('jira-admins')
    await expect(button).toBeEnabled()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('badges arrive from the detail response too, not only the echo', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const issue = await captureIssue(page)
    // One restricted comment, one internal comment, one plain — the plain
    // row must grow no chip (most comments are public).
    await page.route(`**/api/v1/issues/${KEY}/detail/`, async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      const response = await route.fetch()
      const body = (await response.json()) as { comments?: Record<string, unknown>[] }
      body.comments = [
        ...(body.comments ?? []),
        {
          comment_id: 'cv-e2e-plain',
          author: 'Dana Whitfield',
          body: 'cv-e2e plain note',
          created_at: '2026-09-11T10:00:00.000Z',
        },
        {
          comment_id: 'cv-e2e-restricted',
          author: 'Dana Whitfield',
          body: 'cv-e2e restricted note',
          created_at: '2026-09-11T10:05:00.000Z',
          visibility_type: 'group',
          visibility_value: 'jira-admins',
        },
        {
          comment_id: 'cv-e2e-internal',
          author: 'Dana Whitfield',
          body: 'cv-e2e internal note',
          created_at: '2026-09-11T10:10:00.000Z',
          jsd_public: false,
        },
      ]
      await route.fulfill({ response, json: body })
    })
    const { panel } = await openComposer(page)

    const restricted = panel.getByTestId('comment-restricted-badge')
    await expect(restricted).toBeVisible()
    await expect(restricted).toContainText('Restricted · jira-admins')
    await expect(panel.getByTestId('comment-internal-badge')).toBeVisible()
    // Exactly two chips for three comments — the public row stays bare.
    await expect(panel.getByTestId('comment-restricted-badge')).toHaveCount(1)
    await expect(panel.getByTestId('comment-internal-badge')).toHaveCount(1)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a gadak origin draws no restriction controls at all', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    // Same serve, same fixture — only the stated origin type changes. The
    // built-in/paired tracker has no Jira roles and no JSM, so the composer
    // must not offer a restriction it cannot honour.
    await page.route('**/config.json', async (route) => {
      const response = await route.fetch()
      const doc = (await response.json()) as Record<string, unknown>
      await route.fulfill({ response, json: { ...doc, originType: 'gadak' } })
    })
    const issue = await captureIssue(page)
    const { panel } = await openComposer(page)

    await expect(panel.getByTestId('comment-composer')).toBeVisible()
    await expect(panel.getByTestId('comment-restriction')).toHaveCount(0)
    await expect(panel.getByTestId('comment-visibility-kind')).toHaveCount(0)
    await expect(panel.getByTestId('comment-internal')).toHaveCount(0)

    // And commenting still works — the restriction was additive, not gating.
    const getPosted = await stubCommentPost(page, issue, {})
    const composer = panel.getByTestId('comment-composer')
    await composer.fill(TYPED)
    await composer.press('Meta+Enter')
    await expect.poll(() => getPosted()).toEqual({
      text: TYPED,
      mentions: [],
      attachment_ids: [],
    })

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
