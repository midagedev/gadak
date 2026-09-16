import { expect, test } from './helpers'
import { openSettings, waitPaired } from './nav'
import { en } from '../../web/src/lib/i18n/catalog'

/*
 * GDK-1966 — the hosted phone: the bundle a serve hands out at /m/, opened
 * in the phone's browser over the tailnet. No app, no pairing, no QR.
 *
 * The gate's bundle is a DEV build on purpose (gate-serve.sh: DEV is
 * load-bearing for every other spec), and mobile/vite.config.ts in this
 * tree carries no GADAK_PHONE_BASE — the Go round that mounts the bundle
 * at /m/ has not landed — so this spec uses the fallback the task allows:
 * serve at `/` and force the mode with the `?hosted` URL param, the same
 * idiom `?demo-tour` uses. What it proves is the hosted contract itself:
 * no Tauri global ever loads, boot adopts the same-origin serve, Settings
 * answers with the hosted section instead of the pairing roster, and the
 * page produces no uncaught error doing any of it.
 *
 * The demo serve behind this gate predates `GET viewer/` (the Go round
 * owns that route), so the viewer line asserts the honest degradation
 * sentence — the same sentence a direct, non-tailscale connection shows.
 */
test('hosted: boots to the list, Settings says hosted, no Tauri runtime loads', async ({
  page,
}) => {
  const errors: string[] = []
  page.on('pageerror', (err) => errors.push(String(err)))

  await page.goto('/?hosted')

  // Boot adopted the same-origin serve: the list is the owner, rows painted.
  await waitPaired(page)

  await openSettings(page)
  // The one hosted section: the tailnet line naming this serve's host.
  await expect(page.getByText(en['settings.hostedTitle'])).toBeVisible()
  const host = await page.evaluate(() => location.host)
  // Scoped: the page host prints twice by design — here, and as the RAM
  // terminal offer's label — so a page-wide getByText is ambiguous.
  await expect(page.getByTestId('hosted-connection').getByText(host)).toBeVisible()

  // The pairing machinery is absent: no roster heading, no add-host, no
  // unpair (nothing is paired — there is nothing to unpair).
  await expect(page.getByRole('heading', { name: en['app.hosts.title'] })).toHaveCount(0)
  await expect(page.getByRole('button', { name: en['app.unpairPhone'] })).toHaveCount(0)

  // Viewer line: the demo serve has no viewer/ route yet, so this is the
  // degradation sentence, never a blank or a thrown error.
  await expect(page.getByText(en['settings.hostedNoViewer'])).toBeVisible()

  // No Tauri runtime was ever injected, and nothing threw reaching for one.
  expect(await page.evaluate(() => '__TAURI_INTERNALS__' in window)).toBe(false)
  expect(errors).toEqual([])
})
