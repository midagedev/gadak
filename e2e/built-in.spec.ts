import { type Page } from '@playwright/test'
import { expect, test } from './helpers'
import { attachConsoleErrors, forceLocale, gotoApp, openServerSettings, searchInput } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/**
 * Track D: a built-in workspace must show its kind indicator; a connected
 * one must not. Key off data-testid + the served kind — not a locale string
 * or a platform command.
 *
 * The create path is the CLI (this round does not add a write endpoint).
 * Copy is asserted against the real clipboard, not the "Copied" label
 * (GDK-178: a toast that lies is worse than a button that fails aloud).
 */

const STANDALONE_INIT_COMMAND = 'gadak --workspace <name> init --local'

/** Open an issue's detail panel: search narrows the list, click the row. */
async function openIssueDetail(page: Page, key: string) {
  const input = searchInput(page)
  await input.fill(key)
  // Exact key, not hasText: 'NMB-1' is a substring of 111 other keys, and the
  // windowed list renders only the rows in view (GDK-1313: green locally, red in CI).
  await page.getByTestId('issue-list-scroller').locator(`[data-issue-key="${key}"]`).click()
  const panel = page.getByTestId('issue-detail-panel')
  await expect(panel).toBeVisible()
  return panel
}

async function serveWorkspaceKind(
  page: Page,
  kind: 'standalone' | 'connected',
  extra: Record<string, unknown> = {},
): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const res = await route.fetch()
    const doc = (await res.json()) as Record<string, unknown>
    // GDK-1152: the mock must state the capabilities block the real server
    // would send for this shape. A present block wins over the legacy
    // originWritable field, so overriding only the field stopped reaching
    // the surfaces. standalone = issuetap in-process: writes issues and
    // pages, anonymous, no origin page, no site token. connected = a Jira
    // site across the serve API: the token dialog is the way in and out.
    // extra.capabilities deep-merges on top — the paired test uses that to
    // subtract the site-token axes a paired serve would never state.
    const writable = extra.originWritable === true
    const caps =
      kind === 'standalone'
        ? {
            issueWrite: true,
            wikiWrite: true,
            identity: false,
            originDeepLink: false,
            originBaseUrl: '',
            credentialRequired: false,
          }
        : {
            issueWrite: writable,
            wikiWrite: writable,
            identity: writable,
            originDeepLink: true,
            originBaseUrl:
              (doc.capabilities as Record<string, unknown> | undefined)?.originBaseUrl ?? '',
            credentialRequired: true,
          }
    const merged = { ...caps, ...((extra.capabilities as object | undefined) ?? {}) }
    const { capabilities: _override, ...rest } = extra
    await route.fulfill({
      response: res,
      json: { ...doc, ...rest, workspaceKind: kind, capabilities: merged },
    })
  })
}

test.describe('built-in workspace indicator', () => {
  /*
   * GDK-1313: the built-in tracker has no origin page, so nothing may
   * advertise "Open in Built-in" — the palette row, the `o` shortcut's help
   * row, and the header key anchor all promised an action that did nothing
   * (the key was a link-styled <a> with no href). FAIL-first against the
   * unchanged tree: the palette offered the row and the key rendered as <a>.
   */
  test('built-in tracker: no "open in origin" affordance, the key is a label', async ({
    page,
  }) => {
    const errors = attachConsoleErrors(page)
    await serveWorkspaceKind(page, 'standalone', { originType: 'gadak', jiraBaseUrl: '' })
    await forceLocale(page, 'en')
    await gotoApp(page)
    const panel = await openIssueDetail(page, 'NMB-110')

    // The header shows the key as text, not as a dead link.
    await expect(panel.getByTestId('issue-key-label')).toHaveText('NMB-110')
    await expect(panel.locator('a', { hasText: /^NMB-110$/ })).toHaveCount(0)

    // The palette offers no origin row for this issue.
    await page.keyboard.press('Escape')
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('Open in', { delay: 15 })
    await expect(palette.getByRole('option').filter({ hasText: /Open in Built-in/ })).toHaveCount(0)
    await page.keyboard.press('Escape')

    // Nor does the shortcuts sheet teach an `o` that would do nothing.
    await page.keyboard.press('?')
    const sheet = page.getByTestId('shortcuts-dialog')
    await expect(sheet).toBeVisible()
    await expect(sheet.getByText(/Open the issue in/)).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('built-in workspace shows its indicator; connected does not', async ({ page }) => {
    await serveWorkspaceKind(page, 'standalone')
    await gotoApp(page)
    await openServerSettings(page)

    // Scoped to the settings panel: the same badge also rides the sidebar's
    // workspace row, which GDK-1270 made visible on a single-workspace
    // server (the section used to hide below two). Two matches is a strict
    // -mode violation, not a product defect — this assertion is about the
    // settings chip.
    const indicator = page.getByTestId('runtime-mirror').getByTestId('workspace-kind')
    await expect(indicator).toBeVisible()
    await expect(indicator).toHaveAttribute('data-kind', 'standalone')
    // The hint's data-loss claim (GDK-1286 wording): the backup target is the
    // tracker data file, not gadak.db.
    await expect(indicator).toHaveAttribute('aria-label', /not gadak\.db/)
    await expect(page.getByTestId('built-in-init-command')).toHaveText(STANDALONE_INIT_COMMAND)

    // The served document is the source of truth — the chip must match it,
    // not a client-side guess from an empty site URL.
    const served = await page.evaluate(async () => {
      const res = await fetch('/config.json')
      return res.ok ? ((await res.json()) as { workspaceKind?: string }).workspaceKind ?? '' : ''
    })
    expect(served).toBe('standalone')
    await expect(indicator).toHaveAttribute('data-kind', served)
  })

  test('copy writes the init command to the clipboard', async ({ page }) => {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    await serveWorkspaceKind(page, 'standalone')
    await gotoApp(page)
    await openServerSettings(page)

    await page.getByTestId('built-in-init-copy').click()
    await expect.poll(async () => page.evaluate(() => navigator.clipboard.readText())).toBe(
      STANDALONE_INIT_COMMAND,
    )
  })

  test('connected workspace does not show the built-in indicator', async ({ page }) => {
    await serveWorkspaceKind(page, 'connected')
    await gotoApp(page)
    await openServerSettings(page)

    // A connected workspace shows the badge nowhere — sidebar included, so
    // this count stays page-wide.
    await expect(page.getByTestId('workspace-kind')).toHaveCount(0)

    const served = await page.evaluate(async () => {
      const res = await fetch('/config.json')
      return res.ok ? ((await res.json()) as { workspaceKind?: string }).workspaceKind ?? '' : ''
    })
    expect(served).toBe('connected')
    await expect(page.getByTestId('workspace-kind')).toHaveCount(0)
  })

  test('empty site + connected (hosted-demo shape) does not show the badge', async ({ page }) => {
    await serveWorkspaceKind(page, 'connected', { jiraBaseUrl: '' })
    await gotoApp(page)
    await openServerSettings(page)

    await expect(page.getByTestId('workspace-kind')).toHaveCount(0)
    await expect(page.getByTestId('built-in-init-command')).toHaveText(STANDALONE_INIT_COMMAND)
  })

  test('sidebar create control reveals the init command', async ({ page }) => {
    await serveWorkspaceKind(page, 'connected')
    await gotoApp(page)

    // GDK-1335: the create affordance lives in the workspace switcher's menu.
    await page.getByTestId('workspace-switcher').click()
    const create = page.getByTestId('built-in-create')
    await expect(create).toBeVisible()
    await create.click()
    await expect(page.getByText(STANDALONE_INIT_COMMAND, { exact: true })).toBeVisible()
  })

  // GDK-1122: a workspace that already IS built-in must not be offered
  // another one, and its personal section must not send a no-account reader
  // to the Jira credential dialog. The real built-in serve answers auth/me
  // with {email:null} (handleMe: no credential, 200 — not an auth failure),
  // so that anonymous branch is the one under test here.
  test('built-in workspace offers no create control and no credentials CTA', async ({
    page,
  }) => {
    await page.route('**/api/v1/auth/me/**', (route) =>
      route.fulfill({ status: 200, json: { email: null } }),
    )
    await serveWorkspaceKind(page, 'standalone')
    await gotoApp(page)

    // Already built-in: the "create a built-in workspace" affordance is
    // absent from the switcher's menu, not merely unhelpful.
    await page.getByTestId('workspace-switcher').click()
    await expect(page.getByTestId('workspace-menu')).toBeVisible()
    await expect(page.getByTestId('built-in-create')).toHaveCount(0)
    await page.keyboard.press('Escape')

    // MY ISSUES: a built-in tracker without an identity has no "mine", so the
    // section is not rendered at all (GDK-1342) — neither the credential CTA
    // nor the sentence that used to explain the section's own emptiness.
    await expect(page.getByTestId('my-issues-built-in-note')).toHaveCount(0)
    await expect(page.getByText('My issues', { exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: /Set credentials to see/ })).toHaveCount(0)
  })

  /*
   * GDK-1148: the surfaces GDK-1122 did not reach also stop selling Jira
   * credentials to a workspace whose origin already answers every write —
   * the sidebar footer CTA, the Sync tab's personal-token button, and both
   * comment composers' "set credentials" placeholder.
   *
   * The predicate is the served originWritable (config.HasAtlassianCredential),
   * never me.identified: auth/me answers from cfg.Email, which is empty on a
   * built-in and on a paired workspace even though both write fine — the
   * same trap GDK-1090 closed for the link-types catalog, restated for copy.
   * auth/me is stubbed anonymous so the branch that used to render the CTA is
   * the one under test.
   */
  test('built-in workspace: footer CTA and credential placeholders are gone', async ({ page }) => {
    await page.route('**/api/v1/auth/me/**', (route) =>
      route.fulfill({ status: 200, json: { email: null } }),
    )
    await serveWorkspaceKind(page, 'standalone', { originWritable: true })
    await gotoApp(page)

    // Footer: the Set credentials button is absent, not disabled — a
    // built-in has no credential to set and no dialog that could help.
    await expect(
      page.getByRole('button', { name: en['common.setCredentials'], exact: true }),
    ).toHaveCount(0)

    // Issue comments actually work without credentials here (writes pass
    // through the in-process origin), so the composer must offer its normal
    // placeholder rather than send a working writer to a token dialog.
    const panel = await openIssueDetail(page, 'NMB-110')
    await expect(panel.getByTestId('comment-composer')).toHaveAttribute(
      'placeholder',
      en['write.commentPlaceholder'],
    )

    // Document comments: same class (doc.commentNeedCredentials).
    const input = searchInput(page)
    await input.fill('빌링')
    await input.press('Enter')
    await page.getByTestId('search-doc-row').first().click()
    const docs = page.getByTestId('doc-panel')
    await expect(docs).toBeVisible()
    await expect(docs.getByTestId('doc-comment-composer')).toHaveAttribute(
      'placeholder',
      en['doc.commentPlaceholder'],
    )

    // Sync tab: the personal-token dialog edits a site credential (email +
    // API token). A built-in has none, so the entry point is absent.
    await openServerSettings(page)
    await expect(
      page.getByRole('dialog', { name: 'Settings' }).getByRole('button', {
        name: en['settings.personalToken'],
      }),
    ).toHaveCount(0)
  })

  /*
   * GDK-1152: the description pencil asks the origin's capability block,
   * not auth/me. A built-in workspace is anonymous AND writable — the exact
   * row an identity gate gets backwards, and the live defect this round
   * migrated away (the pencil was absent on standalone, paired, and Linear
   * alike). FAIL-first: against the pre-vocabulary component the button
   * below is hidden (me.identified false, hostedDemo false).
   */
  test('built-in workspace: an anonymous writer keeps the description pencil', async ({ page }) => {
    await page.route('**/api/v1/auth/me/**', (route) =>
      route.fulfill({ status: 200, json: { email: null } }),
    )
    await serveWorkspaceKind(page, 'standalone', { originWritable: true })
    await gotoApp(page)

    const panel = await openIssueDetail(page, 'NMB-110')
    await expect(panel.getByTestId('description-edit')).toBeVisible()
  })

  test('paired workspace (connected kind, writable origin) loses the footer CTA', async ({
    page,
  }) => {
    await page.route('**/api/v1/auth/me/**', (route) =>
      route.fulfill({ status: 200, json: { email: null } }),
    )
    // Paired shape: the credential lives in remote-origin.json, so the kind is
    // still "connected" while originWritable is true. This is the double error
    // the audit named — advising an already-configured workspace to configure.
    // The capabilities override states what a paired serve really sends
    // (GDK-1152): issuetap answers everything one machine away — anonymous,
    // no origin page, and crucially no site token for the dialog to edit.
    await serveWorkspaceKind(page, 'connected', {
      originWritable: true,
      capabilities: { identity: false, originDeepLink: false, originBaseUrl: '', credentialRequired: false },
    })
    await gotoApp(page)

    await expect(
      page.getByRole('button', { name: en['common.setCredentials'], exact: true }),
    ).toHaveCount(0)

    // The Sync tab's personal-token button was this test's KNOWN RESIDUAL:
    // hiding it on every writable origin was tried and reverted (that also
    // took it away from a connected workspace WITH a site token — the only
    // in-app way to rotate one). What closes it is the axis the origin can
    // state and a kind guess cannot: credentialRequired, false for paired
    // because its credential is remote-origin.json on the home machine.
    // FAIL-first: against the pre-GDK-1152 SyncTab this button was visible.
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await expect(
      dialog.getByRole('button', { name: en['settings.personalToken'] }),
    ).toHaveCount(0)
  })

  // Negative control: the one place the CTAs must stay is a connected
  // workspace with no origin credential — narrowing the condition must not
  // remove the feature. (The fixture's own config carries a fake token, so
  // originWritable is forced false rather than relied on.)
  test('connected workspace without a credential keeps both CTAs', async ({ page }) => {
    await page.route('**/api/v1/auth/me/**', (route) =>
      route.fulfill({ status: 200, json: { email: null } }),
    )
    await serveWorkspaceKind(page, 'connected', { originWritable: false })
    await gotoApp(page)

    await expect(
      page.getByRole('button', { name: en['common.setCredentials'], exact: true }),
    ).toBeVisible()

    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await expect(dialog.getByRole('button', { name: en['settings.personalToken'] })).toBeVisible()
  })
})
