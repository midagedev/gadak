import { expect, test } from '@playwright/test'
import { attachConsoleErrors, gotoApp } from './helpers'
import { en } from '../web/src/lib/i18n/en'

/*
 * GDK-455: the configured-credential dialog.
 *
 * Defects (captured /tmp/ux-audit/web-shots/06-jira-credentials.png):
 *  1. Token label says "only when replacing" but the password input is required,
 *     so an expiry-only save cannot submit.
 *  2. Footer Delete sends DELETE on the first click.
 *  3. token_hint already includes an ellipsis (write.go credential()) and
 *     jiraSettings.tokenDots wraps it in another, so the line reads "Token ……oken".
 *  4. The same email is painted as the card title (displayName fallback), the
 *     card subtitle, and the form value.
 *
 * The fixture credential is e2e-fake-token → hint "…oken", email dana@example.com,
 * no TokenOwner so displayName is empty.
 */

const API_CREDENTIAL = '**/api/v1/issues/credential/'

async function openCreds(page: Parameters<typeof gotoApp>[0]) {
  await gotoApp(page)
  await page.getByRole('button', { name: en['sidebar.jiraCreds'], exact: true }).click()
  const dialog = page.getByRole('dialog', { name: en['jiraSettings.title'] })
  await expect(dialog).toBeVisible()
  return dialog
}

test.describe('Jira credentials dialog (GDK-455)', () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  test('expiry-only save is not blocked by a required token field', async ({ page }) => {
    const errors = attachConsoleErrors(page)
    const dialog = await openCreds(page)

    const token = dialog.locator('input[type="password"]')
    await expect(token).toBeVisible()
    await expect(token, 'configured token field must not be HTML-required').not.toHaveAttribute(
      'required',
    )

    let putBody: Record<string, unknown> | null = null
    await page.route(API_CREDENTIAL, async (route) => {
      if (route.request().method() === 'PUT') {
        putBody = route.request().postDataJSON() as Record<string, unknown>
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          json: {
            configured: true,
            jira_email: 'dana@example.com',
            display_name: '',
            verified_at: '2026-08-04T00:00:00.000Z',
            token_hint: '…oken',
          },
        })
        return
      }
      await route.continue()
    })

    await dialog.locator('input[type="date"]').fill('2027-12-31')
    await dialog.getByRole('button', { name: en['jiraSettings.replaceToken'], exact: true }).click()

    await expect.poll(() => putBody).not.toBeNull()
    expect(putBody!.api_token).toBe('')
    expect(putBody!.token_expires_at).toBe('2027-12-31')
    expect(putBody!.jira_email).toBe('dana@example.com')
    expect(errors, `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('Delete asks for a second click before sending DELETE', async ({ page }) => {
    const dialog = await openCreds(page)

    let deleteCount = 0
    await page.route(API_CREDENTIAL, async (route) => {
      if (route.request().method() === 'DELETE') {
        deleteCount += 1
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          json: {
            configured: false,
            jira_email: '',
            display_name: '',
            verified_at: null,
            token_hint: '',
          },
        })
        return
      }
      await route.continue()
    })

    await dialog.getByRole('button', { name: en['common.delete'], exact: true }).click()
    expect(deleteCount, 'first click must not DELETE').toBe(0)
    await expect(
      dialog.getByRole('button', { name: en['jiraSettings.deleteConfirm'], exact: true }),
    ).toBeVisible()

    await dialog.getByRole('button', { name: en['jiraSettings.deleteConfirm'], exact: true }).click()
    await expect.poll(() => deleteCount).toBe(1)
  })

  test('token hint is a single ellipsis and the email is not shown three times', async ({
    page,
  }) => {
    const dialog = await openCreds(page)

    await expect(dialog.getByText('Token …oken', { exact: true })).toBeVisible()
    await expect(dialog.getByText('Token ……oken')).toHaveCount(0)

    // Card title (displayName fallback) plus the form value (not in getByText).
    // A leftover subtitle span would make this 2.
    await expect(dialog.getByText('dana@example.com')).toHaveCount(1)
  })
})
