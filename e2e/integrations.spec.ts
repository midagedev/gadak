/*
 * The Integrations settings tab (GDK-185).
 *
 * The tab drives `/desktop/integrations`, a route that exists on the desktop
 * app's own mux and nowhere else. So the tab has two contracts, and this file
 * asserts both against the same bundle:
 *
 *   - `gadak serve` in a browser must not offer it at all, including through a
 *     `settings=integrations` link pasted out of the app. A tab whose every
 *     click 404s is worse than an absent one.
 *   - the app must list what is installed, show the command it is about to
 *     run, stream that command's output, and take its verdict from the
 *     `exit=<code>` line rather than from silence.
 *
 * Desktop mode is faked the way desktop-chrome.spec.ts fakes it — the same
 * config.json the app serves, plus `desktop` — and the integration routes are
 * fulfilled here, because the served test binary is `gadak serve` and has none.
 * The line-splitting itself (chunk boundaries, multi-byte output) is pinned in
 * web/src/lib/integrations.test.ts; Playwright covers the wiring.
 */
import { test, expect, type Page } from '@playwright/test'
import { attachConsoleErrors, forceLocale, gotoApp, openServerSettings } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/** Serve the config the desktop app serves: same document, plus `desktop`. */
async function pretendDesktop(page: Page): Promise<void> {
  await page.route('**/config.json', async (route) => {
    const res = await route.fetch()
    const doc = JSON.parse(await res.text())
    doc.desktop = true
    await route.fulfill({ response: res, body: JSON.stringify(doc) })
  })
}

const ITEMS = {
  items: [
    {
      id: 'raycast',
      title: 'Raycast extension',
      installed: false,
      detail: '~/.gadak/raycast-extension',
      command: 'gadak raycast install',
      prerequisite: { ok: true, message: '' },
    },
    {
      id: 'skill',
      title: 'Claude Code skill',
      installed: true,
      detail: '~/.claude/skills/gadak',
      command: 'gadak skill install',
      prerequisite: null,
    },
    {
      id: 'mcp-claude',
      title: 'Claude Desktop MCP',
      installed: null,
      detail: '',
      command: 'gadak mcp install claude',
      prerequisite: { ok: false, message: 'Claude Desktop is not installed.' },
    },
  ],
}

const dialog = (page: Page) => page.getByTestId('settings-dialog')
const tabButton = (page: Page, label: string) =>
  dialog(page).getByRole('button', { name: label, exact: true })

test.describe('integrations tab is desktop-only', () => {
  test('a browser tab is not offered the tab, by header or by link', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await gotoApp(page)
    await openServerSettings(page)

    // The other six are still there: this must read as "one tab is gated", not
    // as "the header broke".
    for (const key of [
      'settings.tabSync',
      'settings.tabSources',
      'settings.tabFeatures',
      'settings.tabTeams',
      'settings.tabMembers',
      'settings.tabFields',
    ] as const) {
      await expect(tabButton(page, en[key])).toBeVisible()
    }
    await expect(tabButton(page, en['settings.tabIntegrations'])).toHaveCount(0)
    await expect(page.getByTestId('integrations-tab')).toHaveCount(0)

    // A link out of the desktop app lands on the default tab, not on a tab
    // whose server this deployment does not have. Standing somewhere other
    // than the default first, so "it stayed on sync" cannot pass by accident.
    await tabButton(page, en['settings.tabMembers']).click()
    await expect(page).toHaveURL(/settings=members/)
    await forceLocale(page, 'en')
    await page.goto('/#/?settings=integrations')
    await expect(dialog(page)).toBeVisible()
    await expect(tabButton(page, en['settings.tabSync'])).toHaveAttribute(
      'aria-current',
      'true',
    )
    await expect(page).toHaveURL(/settings=sync/)
    await expect(page.getByTestId('integrations-tab')).toHaveCount(0)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

test.describe('integrations tab in the app', () => {
  test('lists what is installed, showing the command it would run', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await pretendDesktop(page)
    await page.route('**/desktop/integrations', (route) => route.fulfill({ json: ITEMS }))
    await gotoApp(page)
    await openServerSettings(page)
    await tabButton(page, en['settings.tabIntegrations']).click()

    const tab = page.getByTestId('integrations-tab')
    await expect(tab).toBeVisible()

    // Three pill states, not two: "the server could not tell" is its own answer.
    await expect(page.getByTestId('integration-status-skill')).toHaveAttribute(
      'data-state',
      'installed',
    )
    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'not-installed',
    )
    await expect(page.getByTestId('integration-status-mcp-claude')).toHaveAttribute(
      'data-state',
      'unknown',
    )

    // The command is on screen, not behind the button.
    await expect(tab.getByText('gadak raycast install')).toBeVisible()
    await expect(tab.getByText('~/.claude/skills/gadak')).toBeVisible()

    // Already installed → the re-run is an Update, not a second Install.
    await expect(page.getByTestId('integration-install-skill')).toHaveText(
      en['settings.integrationUpdate'],
    )
    await expect(page.getByTestId('integration-install-raycast')).toHaveText(
      en['settings.integrationInstall'],
    )

    // An unmet prerequisite says why and disables the button rather than
    // letting the click fail in a log.
    await expect(page.getByTestId('integration-prereq-mcp-claude')).toContainText(
      'Claude Desktop is not installed.',
    )
    await expect(page.getByTestId('integration-install-mcp-claude')).toBeDisabled()

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a failed install shows the exit code and offers a retry', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await pretendDesktop(page)
    await page.route('**/desktop/integrations', (route) => route.fulfill({ json: ITEMS }))
    await page.route('**/desktop/integrations/raycast/install', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'text/plain',
        body: 'copying files\ncould not write ~/.gadak\nexit=13\n',
      }),
    )
    await gotoApp(page)
    await openServerSettings(page)
    await tabButton(page, en['settings.tabIntegrations']).click()
    await page.getByTestId('integration-install-raycast').click()

    // The output is in the card, and the exit line is not part of it: the code
    // is reported as a status, the panel stays the command's own output.
    const log = page.getByTestId('integration-log-raycast')
    await expect(log).toContainText('could not write ~/.gadak')
    await expect(log).not.toContainText('exit=13')

    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'failed',
    )
    await expect(page.getByTestId('integration-row-raycast')).toContainText('code 13')
    await expect(page.getByTestId('integration-install-raycast')).toHaveText(
      en['settings.integrationRetry'],
    )

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('a successful install re-reads the list instead of assuming', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    await pretendDesktop(page)

    // The second GET is the point: the check mark has to come from the server
    // looking again, not from the exit code being zero.
    let gets = 0
    await page.route('**/desktop/integrations', (route) => {
      gets += 1
      const items = JSON.parse(JSON.stringify(ITEMS)) as typeof ITEMS
      if (gets > 1) items.items[0].installed = true
      return route.fulfill({ json: items })
    })
    await page.route('**/desktop/integrations/raycast/install', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'text/plain',
        body: 'linked\nexit=0\n',
      }),
    )
    await gotoApp(page)
    await openServerSettings(page)
    await tabButton(page, en['settings.tabIntegrations']).click()

    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'not-installed',
    )
    await page.getByTestId('integration-install-raycast').click()

    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'installed',
    )
    await expect(page.getByTestId('integration-install-raycast')).toHaveText(
      en['settings.integrationUpdate'],
    )
    expect(gets).toBeGreaterThan(1)

    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('an unreadable list says so rather than showing zero integrations', async ({ page }) => {
    await pretendDesktop(page)
    await page.route('**/desktop/integrations', (route) => route.fulfill({ status: 404, body: '' }))
    await gotoApp(page)
    await openServerSettings(page)
    await tabButton(page, en['settings.tabIntegrations']).click()

    const tab = page.getByTestId('integrations-tab')
    await expect(tab).toContainText(en['settings.integrationsLoadFailed'])
    await expect(tab.getByText(en['settings.integrationsEmpty'])).toHaveCount(0)
  })
})

/*
 * Failure modes. Installing and detecting are mostly exceptions — the happy path
 * is the short one — so each of these is a state the card must have rather than
 * a guess it must make. The rule under all of them: never present success that
 * was not reported, and never present a failure that was not reported either.
 */
test.describe('integrations failure modes', () => {
  /** Open the tab with the list routed, and the install routed as `install`. */
  async function openTab(
    page: Page,
    install: (route: Parameters<Parameters<Page['route']>[1]>[0]) => unknown,
    list: () => typeof ITEMS = () => ITEMS,
  ): Promise<void> {
    await pretendDesktop(page)
    await page.route('**/desktop/integrations', (route) => route.fulfill({ json: list() }))
    await page.route('**/desktop/integrations/raycast/install', install)
    await gotoApp(page)
    await openServerSettings(page)
    await tabButton(page, en['settings.tabIntegrations']).click()
    await expect(page.getByTestId('integrations-tab')).toBeVisible()
  }

  test('output that stops before the status is "result unknown", not a verdict', async ({
    page,
  }) => {
    // A server restart, a killed app, a socket that goes away: the stream ends
    // with no `exit=` line. Neither "Installed" nor "Setup failed" is true.
    await openTab(page, (route) =>
      route.fulfill({ status: 200, contentType: 'text/plain', body: 'copying files\n' }),
    )
    await page.getByTestId('integration-install-raycast').click()

    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'result-unknown',
    )
    // It says what to do about it, and keeps the output it did get.
    await expect(page.getByTestId('integration-note-raycast')).toContainText(
      en['settings.integrationNoExit'],
    )
    await expect(page.getByTestId('integration-log-raycast')).toContainText('copying files')
  })

  test('an exit= line in the middle of the output is output, not the verdict', async ({ page }) => {
    // The wrapper echoes `exit=0` and then really fails. Reading the first one
    // as the end is what would turn a broken install into a check mark.
    await openTab(page, (route) =>
      route.fulfill({
        status: 200,
        contentType: 'text/plain',
        body: 'exit=0\nrolling back\nexit=2\n',
      }),
    )
    await page.getByTestId('integration-install-raycast').click()

    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'failed',
    )
    await expect(page.getByTestId('integration-row-raycast')).toContainText('code 2')
    // The echoed line is kept in the log verbatim — it was output all along.
    const log = page.getByTestId('integration-log-raycast')
    await expect(log).toContainText('exit=0')
    await expect(log).toContainText('rolling back')
  })

  test('409 reads as a run in flight, and the button stays out of the way', async ({ page }) => {
    await openTab(page, (route) =>
      route.fulfill({
        status: 409,
        contentType: 'text/plain',
        body: '{"error":"install_in_progress"}',
      }),
    )
    await page.getByTestId('integration-install-raycast').click()

    // Wait for the refusal to have been handled before judging the pill: the
    // click itself flips the row to "running" for a moment, and an assertion
    // that catches that transient would pass against a build that drops 409 on
    // the floor. The note only exists once the 409 came back.
    await expect(page.getByTestId('integration-note-raycast')).toContainText(
      en['settings.integrationBusy'],
    )
    // Not an error: an install really is running, just not one we can watch.
    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'running',
    )
    await expect(page.getByTestId('integration-install-raycast')).toBeDisabled()

    // Re-check is the way out: it asks the server and releases the row.
    await page.getByTestId('integration-recheck-raycast').click()
    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'not-installed',
    )
    await expect(page.getByTestId('integration-install-raycast')).toBeEnabled()
  })

  test('a refused start keeps the previous run\'s log', async ({ page }) => {
    // First attempt fails and leaves a log; the second is refused before
    // anything runs. Erasing the log there would delete the only account of the
    // failure the user is trying to read.
    let attempts = 0
    await openTab(page, (route) => {
      attempts += 1
      if (attempts === 1) {
        return route.fulfill({
          status: 200,
          contentType: 'text/plain',
          body: 'could not write ~/.gadak\nexit=13\n',
        })
      }
      return route.fulfill({ status: 500, body: '' })
    })

    await page.getByTestId('integration-install-raycast').click()
    await expect(page.getByTestId('integration-log-raycast')).toContainText('could not write')

    await page.getByTestId('integration-install-raycast').click()
    await expect(page.getByTestId('integration-note-raycast')).toContainText(
      en['settings.integrationStartFailed'],
    )
    await expect(page.getByTestId('integration-log-raycast')).toContainText('could not write')
  })

  test('exit 0 with the check still saying no does not become a check mark', async ({ page }) => {
    // Registration settles late, or the command did less than it reported. The
    // detection is what is shown; the command's claim is written next to it.
    await openTab(page, (route) =>
      route.fulfill({ status: 200, contentType: 'text/plain', body: 'linked\nexit=0\n' }),
    )
    await page.getByTestId('integration-install-raycast').click()

    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'not-installed',
    )
    await expect(page.getByTestId('integration-note-raycast')).toContainText(
      en['settings.integrationOkUndetected'],
    )
    await expect(page.getByTestId('integration-log-raycast')).toContainText('linked')
  })

  test('unknown detection says so and says what to do', async ({ page }) => {
    await openTab(page, (route) =>
      route.fulfill({ status: 409, contentType: 'text/plain', body: '{"error":"install_in_progress"}' }),
    )
    await expect(page.getByTestId('integration-status-mcp-claude')).toHaveAttribute(
      'data-state',
      'unknown',
    )
    await expect(page.getByTestId('integration-unknown-hint-mcp-claude')).toContainText(
      en['settings.integrationUnknownHint'],
    )
    // A row the server could decide gets no such hint.
    await expect(page.getByTestId('integration-unknown-hint-skill')).toHaveCount(0)
  })

  test('a failed re-read is a banner over the list, not a blank tab', async ({ page }) => {
    let gets = 0
    await pretendDesktop(page)
    await page.route('**/desktop/integrations', (route) => {
      gets += 1
      if (gets === 1) return route.fulfill({ json: ITEMS })
      return route.fulfill({ status: 500, body: '' })
    })
    await gotoApp(page)
    await openServerSettings(page)
    await tabButton(page, en['settings.tabIntegrations']).click()
    await expect(page.getByTestId('integration-row-raycast')).toBeVisible()

    await page.getByTestId('integration-recheck-raycast').click()
    await expect(page.getByTestId('integrations-error')).toContainText(
      en['settings.integrationsLoadFailed'],
    )
    // The cards — and any logs on them — survive the failed read.
    await expect(page.getByTestId('integration-row-raycast')).toBeVisible()
    await expect(page.getByTestId('integration-row-skill')).toBeVisible()
  })

  test('a half-installed integration shows the server\'s explanation verbatim', async ({ page }) => {
    // The server can report a partial install (files there, node_modules not)
    // by appending to `detail`. That sentence is the whole value of the row —
    // it must not be reformatted, shortened, or replaced by our own wording.
    const partial = JSON.parse(JSON.stringify(ITEMS)) as typeof ITEMS
    partial.items[0].detail =
      '~/.gadak/raycast-extension (incomplete — node_modules missing, run install again)'
    await pretendDesktop(page)
    await page.route('**/desktop/integrations', (route) => route.fulfill({ json: partial }))
    await gotoApp(page)
    await openServerSettings(page)
    await tabButton(page, en['settings.tabIntegrations']).click()

    await expect(page.getByTestId('integration-row-raycast')).toContainText(
      '(incomplete — node_modules missing, run install again)',
    )
    // And it is still "not installed" — a partial install is not an install.
    await expect(page.getByTestId('integration-status-raycast')).toHaveAttribute(
      'data-state',
      'not-installed',
    )
  })
})
