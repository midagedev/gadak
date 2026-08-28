import { expect, test } from '@playwright/test'
import { apiURL, attachConsoleErrors, gotoApp, openServerSettings } from './helpers'

/*
 * GDK-1096 A2 — the Workspaces settings tab against the real serve: create
 * a standalone workspace, see it listed, then walk the two-step removal
 * (the probe DELETE surfaces the server's own refusal wording — which for a
 * standalone workspace is the only-copy warning with the persist path —
 * before yes=1 commits with destroy_origin).
 *
 * Port discipline: every URL goes through apiURL(), whose single owner is
 * GADAK_E2E_PORT (helpers.ts). The delegated round that authored this spec
 * ran the suite under GADAK_E2E_PORT=7891 to stay off the lead's 7877; CI
 * and the lead's full-suite runs use their own port and home
 * (e2e/.tmp/home-<port>), which is why nothing here may spell a host.
 *
 * The workspace this spec creates lives under the e2e home, never a real
 * one — and the spec removes it itself, so a rerun starts clean even before
 * the defensive pre-delete below.
 */

const WS = 'e2e-tmp-ws'

test.describe('workspaces settings tab', () => {
  test('create, list, and two-step remove of a standalone workspace', async ({ page, request }) => {
    const errors = attachConsoleErrors(page)

    // Defensive pre-delete: a crashed earlier run may have left the row
    // behind (the home is reused between runs on the same port). Any answer
    // is fine — 404 is the expected one.
    await request.delete(apiURL(`/api/v1/workspaces/${WS}?yes=1&destroy_origin=1`))

    // The serving profile's name is the server's fact, not ours to guess.
    const listRes = await request.get(apiURL('/api/v1/workspaces'))
    expect(listRes.ok()).toBeTruthy()
    const list = (await listRes.json()) as { workspaces?: { name: string; active?: boolean }[] }
    const active = list.workspaces?.find((w) => w.active)
    expect(active, 'the serve must list its own profile as active').toBeTruthy()

    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Workspaces', exact: true }).click()

    const tab = dialog.getByTestId('workspaces-tab')
    await expect(tab).toBeVisible()

    // The serving profile's Delete is disabled up front — self_delete is a
    // fact the UI states before the server has to refuse it.
    await expect(tab.getByTestId(`workspaces-row-${active!.name}`)).toBeVisible()
    await expect(tab.getByTestId(`workspaces-remove-${active!.name}`)).toBeDisabled()

    // Create: name only (projects CSV stays optional).
    await tab.getByTestId('workspaces-name-input').fill(WS)
    await tab.getByTestId('workspaces-create-button').click()
    await expect(tab.getByTestId(`workspaces-row-${WS}`)).toBeVisible({ timeout: 30_000 })

    // A same-name create is an inline error, not a toast and not a reset.
    await tab.getByTestId('workspaces-name-input').fill(WS)
    await tab.getByTestId('workspaces-create-button').click()
    await expect(tab.getByTestId('workspaces-create-error')).toContainText('already exists')

    // Removal, step one: the probe DELETE (no yes=1) puts the server's own
    // refusal into the dialog — for a standalone workspace that wording is
    // the only-copy warning and names the persist.
    await tab.getByTestId(`workspaces-remove-${WS}`).click()
    const confirm = page.getByTestId('workspaces-remove-dialog')
    await expect(confirm).toBeVisible()
    const detail = confirm.getByTestId('workspaces-refusal-detail')
    await expect(detail).toContainText('standalone workspace')
    await expect(detail).toContainText('persist:')

    // The only-copy checkbox is offered unchecked, and until it is checked
    // the commit stays disabled — the server would refuse a bare yes.
    const destroy = confirm.getByTestId('workspaces-destroy-origin')
    await expect(destroy).not.toBeChecked()
    await expect(confirm.getByTestId('workspaces-remove-confirm')).toBeDisabled()
    await destroy.check()
    await expect(confirm.getByTestId('workspaces-remove-confirm')).toBeEnabled()

    // Step two: yes=1&destroy_origin=1 — the row goes, and the server's
    // advisories render under the list.
    await confirm.getByTestId('workspaces-remove-confirm').click()
    await expect(confirm).toHaveCount(0)
    await expect(tab.getByTestId(`workspaces-row-${WS}`)).toHaveCount(0, { timeout: 30_000 })
    await expect(tab.getByTestId('workspaces-advisories')).toContainText('keeps its last view')

    // 400/409 are the protocol's own answers here (the probe's refusal, the
    // duplicate create) — Chromium logs each as a console error, so both are
    // filtered the way helpers.ts filters the fixture's write-409s.
    const unexpected = errors.filter((e) => !e.includes('409') && !e.includes('400'))
    expect(unexpected, `console errors:\n${unexpected.join('\n')}`).toEqual([])
  })
})
