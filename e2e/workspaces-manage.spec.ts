import { expect, test, type Route } from '@playwright/test'
import { apiURL, attachConsoleErrors, gotoApp, openServerSettings } from './helpers'

/*
 * GDK-1096 A2 — the Workspaces settings tab against the real serve: create
 * a local-origin workspace, see it listed, then walk the two-step removal
 * (the probe DELETE surfaces the server's own refusal wording — which for a
 * local-origin workspace is the only-copy warning with the persist path —
 * before yes=1 commits with destroy_origin).
 *
 * GDK-1099 — the paired half. No real pairing home is stood up in this
 * suite: the real verify-before-save contract (decode, the /myself round
 * trip, the refusal taxonomy, the offer-never-echoed rule) is owned by the
 * Go tests in internal/workspace/manage_test.go. This spec mocks the POST's
 * answers with page.route — the dialog-shell.spec.ts stubWorkspacesRemove
 * discipline — and pins the three UI states only: the new row after a
 * successful register, the server's refusal detail rendered inline, and
 * the invalid-offer line.
 *
 * Port discipline: every URL goes through apiURL(), whose single owner is
 * GADAK_E2E_PORT (helpers.ts). The delegated round that authored this spec
 * ran the suite under GADAK_E2E_PORT=7891 (the GDK-1099 round: 7893) to
 * stay off the lead's 7877; CI and the lead's full-suite runs use their own
 * port and home (e2e/.tmp/home-<port>), which is why nothing here may
 * spell a host.
 *
 * The workspace this spec creates lives under the e2e home, never a real
 * one — and the spec removes it itself, so a rerun starts clean even before
 * the defensive pre-delete below.
 */

const WS = 'e2e-tmp-ws'

/** dialog-shell.spec.ts's fulfillJSON: one-line JSON fulfill, status-bound. */
async function fulfillJSON(route: Route, json: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, contentType: 'application/json', json })
}

test.describe('workspaces settings tab', () => {
  test('create, list, and two-step remove of a local-origin workspace', async ({ page, request }) => {
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
    // refusal into the dialog — for a local-origin workspace that wording is
    // the only-copy warning and names the persist.
    await tab.getByTestId(`workspaces-remove-${WS}`).click()
    const confirm = page.getByTestId('workspaces-remove-dialog')
    await expect(confirm).toBeVisible()
    const detail = confirm.getByTestId('workspaces-refusal-detail')
    // GDK-1281: the refusal names what it is protecting rather than a kind
    // word. The point of the assertion — that the persist is the only copy
    // — is what the sentence still has to say.
    await expect(detail).toContainText('the only copy')
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

test.describe('workspaces settings tab — register remote (GDK-1099)', () => {
  /*
   * Mocked POST answers, three UI states (see the GDK-1099 note up top —
   * the real pairing contract is the Go tests'). The mock is stateful on
   * purpose: the success path reloads the list, so the GET must learn the
   * new row from the POST that created it, exactly as the real serve
   * would answer.
   */
  test('register remote: refusals render inline, success adds a connected row and clears the code', async ({ page }) => {
    const errors = attachConsoleErrors(page)

    const PAIRED = 'e2e-paired-ws'
    let answer: 'refused' | 'invalid' | 'ok' = 'refused'
    let registered = false
    await page.route('**/api/v1/workspaces', async (route) => {
      const req = route.request()
      if (req.method() === 'GET') {
        await fulfillJSON(route, {
          workspaces: [
            { name: 'default', site: 'https://nimbus.example.com', projects: ['NMB'], active: true },
            ...(registered ? [{ name: PAIRED, site: 'http://127.0.0.1:7900' }] : []),
          ],
        })
        return
      }
      if (req.method() === 'POST') {
        const body = req.postDataJSON() as { kind?: string }
        if (body.kind !== 'paired') {
          await route.continue()
          return
        }
        if (answer === 'refused') {
          await fulfillJSON(
            route,
            {
              error: 'pairing_refused',
              detail:
                'the serve answered but refused this pairing token (401) — ask the home machine to mint a fresh offer',
            },
            400,
          )
          return
        }
        if (answer === 'invalid') {
          await fulfillJSON(route, { error: 'invalid_offer', detail: 'pairing offer: not base64url' }, 400)
          return
        }
        registered = true
        await fulfillJSON(
          route,
          {
            name: PAIRED,
            kind: 'connected',
            endpoint: 'http://127.0.0.1:7900',
            label: 'web-tab',
            account: 'Home Human',
          },
          201,
        )
        return
      }
      await route.continue()
    })

    await gotoApp(page)
    await openServerSettings(page)
    const dialog = page.getByRole('dialog', { name: 'Settings' })
    await dialog.getByRole('button', { name: 'Workspaces', exact: true }).click()
    const tab = dialog.getByTestId('workspaces-tab')

    // The mode switch swaps the local-origin form for the pairing form.
    await tab.getByTestId('workspaces-mode-paired').click()
    await expect(tab.getByTestId('workspaces-pair-form')).toBeVisible()
    await expect(tab.getByTestId('workspaces-form')).toHaveCount(0)

    // pairing_refused: the server's own wording, inline — and the offer
    // stays in the box for a re-submit.
    await tab.getByTestId('workspaces-pair-name-input').fill(PAIRED)
    await tab.getByTestId('workspaces-offer-input').fill('e2e-mock-offer-line')
    await tab.getByTestId('workspaces-pair-button').click()
    await expect(tab.getByTestId('workspaces-pair-error')).toContainText('refused this pairing token')
    await expect(tab.getByTestId('workspaces-offer-input')).toHaveValue('e2e-mock-offer-line')

    // invalid_offer: the decode defect's wording, same inline lane.
    answer = 'invalid'
    await tab.getByTestId('workspaces-pair-button').click()
    await expect(tab.getByTestId('workspaces-pair-error')).toContainText('pairing offer: not base64url')

    // Success: the reloaded list carries the new row, and the code — a
    // credential — is cleared from the input.
    answer = 'ok'
    await tab.getByTestId('workspaces-pair-button').click()
    await expect(tab.getByTestId(`workspaces-row-${PAIRED}`)).toBeVisible({ timeout: 30_000 })
    await expect(tab.getByTestId('workspaces-offer-input')).toHaveValue('')

    // The mocked 400s are this spec's own staged answers, filtered the way
    // the local-origin test filters the protocol's 400/409 above.
    const unexpected = errors.filter((e) => !e.includes('400'))
    expect(unexpected, `console errors:\n${unexpected.join('\n')}`).toEqual([])
  })
})
