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

/*
 * GDK-1970 — the detail is a history entry, and the page is never left.
 *
 * In a browser the edge swipe walks session history, so the detail living
 * only in store state meant one swipe slid the whole page to whatever the
 * browser had below it, and a second could leave the app. The unit half
 * (lib/back.test.ts) pins the frame mechanics with a fake entry stack;
 * this is the half only a real browser history can prove: the open writes
 * the #/KEY hash, one back closes the detail onto the sentinel with the
 * URL following, forward reopens the same key, and a back at the root is
 * the bounce — still this document, still the list, URL never moved.
 *
 * The order is back → forward → back → back, not back → back → forward:
 * the bounce re-arms by pushing a fresh sentinel, and a pushState is what
 * clears the browser's forward stack — after a double back there is no
 * forward entry to reopen. Each assertion the round wants is still here,
 * asserted from a history position where it is reachable.
 */
test('hosted: back closes the detail onto the sentinel, forward reopens it, the page is never left', async ({
  page,
}) => {
  const errors: string[] = []
  page.on('pageerror', (err) => errors.push(String(err)))

  await page.goto('/?hosted')
  await waitPaired(page)
  const startUrl = page.url()

  // The first row's key, read before the tap that opens it.
  const key = ((await page.locator('.pane:not(.off) .row .key').first().textContent()) ?? '').trim()
  expect(key).toMatch(/^[A-Z][A-Z0-9_]*-\d+$/)
  const pathname = await page.evaluate(() => location.pathname)

  await page.locator('.pane:not(.off) button.row').first().click()

  // The open is a real pushState: the hash names the key.
  await expect(page.locator('.detail-layer')).toHaveCount(1)
  expect(await page.evaluate(() => location.hash)).toBe(`#/${key}`)

  // One back — the edge swipe: the detail closes onto the sentinel, the
  // URL returns to no hash, and the document never moved.
  await page.goBack()
  await expect(page.locator('.detail-layer')).toHaveCount(0)
  await waitPaired(page)
  expect(await page.evaluate(() => location.hash)).toBe('')
  expect(await page.evaluate(() => location.pathname)).toBe(pathname)

  // Forward reopens the same key — the frame survived as a forward entry.
  await page.goForward()
  await expect(page.locator('.detail-layer')).toHaveCount(1)
  expect(await page.evaluate(() => location.hash)).toBe(`#/${key}`)

  // Close it again, then the second back is the bounce: the re-arm's
  // pushState eats the gesture, which is the whole point of the sentinel.
  await page.goBack()
  await expect(page.locator('.detail-layer')).toHaveCount(0)
  await page.goBack()
  await waitPaired(page)
  expect(page.url()).toBe(startUrl)
  expect(errors).toEqual([])

  // The install hint (Layer A′) says what the home screen buys: shown in
  // hosted Settings while the page is still a browser tab.
  await openSettings(page)
  await expect(page.getByTestId('hosted-add-home')).toBeVisible()
  expect(errors).toEqual([])
})
