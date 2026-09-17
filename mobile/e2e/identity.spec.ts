/*
 * The declared name on the phone (GDK-1973).
 *
 * The web round's server half taught the serve to read
 * X-Gadak-Actor-Name — attribution only, never authority (the terminal gate
 * ignores it, pinned there by test). This is the phone half: Settings gains
 * "You on this tracker", its verb is local (trim, cap, persist under the
 * host-scoped gadak.actorName key, push onto every request from here on),
 * and what only a browser can prove is exactly the two halves the unit
 * tests cannot:
 *
 *   - the header actually rides a *real* dial — request() threading the
 *     store's push, percent-encoded, on the same request the serve would
 *     attribute. A raw fetch() from the page would bypass the app's dial
 *     and prove nothing, so the trigger is the app's own Sync now.
 *   - boot's restore path on a reload: enterPaired re-reads the key and
 *     pushes the header before the first sync, and the field shows it.
 *
 * The gate's demo serve is a built-in origin — me carries no identity — so
 * the paired branch shows the section under the same condition the
 * no-identity sentence keys on, and the hosted branch asserts the sentence
 * a declared name replaces. Copy is read from the desk's catalog (§3.6):
 * no assertion here types a sentence.
 */
import { expect, test } from './helpers'
import { openSettings, waitPaired } from './nav'
import { en } from '../../web/src/lib/i18n/catalog'

test('the declared name rides the next request and survives a reload', async ({ page }) => {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  await openSettings(page)

  // The demo serve is a built-in origin: no identity of its own, so the
  // section is present and its field starts empty.
  await expect(page.getByTestId('identity')).toBeVisible()
  await expect(page.getByTestId('identity-name')).toHaveValue('')

  await page.getByTestId('identity-name').fill('Dana Kim')
  await page.getByTestId('identity-save').click()
  await expect(page.locator('[data-testid="toast"][data-kind="success"]')).toContainText('Dana Kim')

  // The next dial carries it. Sync now is the app's own request() — the
  // only road that threads the pushed name — and bootstrap is the first
  // call of every cycle, so the caught request is unambiguously post-save.
  // Scoped to the layer: the list header wears its own Sync-now control,
  // and the unscoped role query is a strict-mode violation between them.
  const riding = page.waitForRequest((r) => r.url().includes('/api/v1/issues/bootstrap/'))
  await page.locator('.settings-layer').getByRole('button', { name: en['sync.now'] }).click()
  const req = await riding
  expect(decodeURIComponent(req.headers()['x-gadak-actor-name'] ?? '')).toBe('Dana Kim')

  // A relaunch is boot's restore path: the host-scoped key feeds the field
  // again, and the next sync of that session would carry the header too.
  await page.reload({ waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  await openSettings(page)
  await expect(page.getByTestId('identity-name')).toHaveValue('Dana Kim')

  // Clearing is the same verb in reverse: the key is dropped and the
  // workspace-default attribution (no header) stands again.
  await page.getByTestId('identity-name').fill('')
  await page.getByTestId('identity-save').click()
  const bare = page.waitForRequest((r) => r.url().includes('/api/v1/issues/bootstrap/'))
  await page.locator('.settings-layer').getByRole('button', { name: en['sync.now'] }).click()
  expect((await bare).headers()['x-gadak-actor-name']).toBeUndefined()
})

test('hosted: a declared name replaces the no-viewer sentence and stays', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (err) => errors.push(String(err)))

  await page.goto('/?hosted', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  await openSettings(page)

  // A direct connection, the demo serve predating GET viewer/: the
  // degradation sentence first, and the field under it.
  await expect(page.getByText(en['settings.hostedNoViewer'])).toBeVisible()
  await expect(page.getByTestId('identity')).toBeVisible()

  await page.getByTestId('identity-name').fill('Dana Kim')
  await page.getByTestId('identity-save').click()
  await expect(page.locator('[data-testid="toast"][data-kind="success"]')).toContainText('Dana Kim')

  // The sentence the name replaces: the same connection, now writing under
  // a name chosen here — saying itself that it is not verified.
  await expect(page.getByTestId('hosted-declared')).toContainText('Dana Kim')
  await expect(page.getByText(en['settings.hostedNoViewer'])).toHaveCount(0)

  // The name is hosted's one kept key, this browser's own localStorage at
  // the serve origin: a reload restores the field and the sentence both.
  await page.reload({ waitUntil: 'domcontentloaded' })
  await waitPaired(page)
  await openSettings(page)
  await expect(page.getByTestId('identity-name')).toHaveValue('Dana Kim')
  await expect(page.getByTestId('hosted-declared')).toContainText('Dana Kim')
  expect(errors).toEqual([])
})
