/*
 * GDK-873 end to end: a gadak:// URL delivered to the running app opens that
 * issue's detail screen.
 *
 * What this can and cannot prove. A browser never receives a custom-scheme
 * URL, and no simulator runs here — whether iOS actually routes `gadak://`
 * to this bundle is decided by CFBundleURLTypes in src-tauri/Info.ios.plist
 * and can only be measured on a device (that proof is the lead's). What a
 * browser CAN prove is the half that is ours: that the app's entry point is
 * subscribed, that a URL handed to it lands on the right screen, and that
 * the refusals actually refuse. Those are the parts that break silently.
 *
 * So the spec delivers URLs through the entry point's test hook
 * (DEEP_LINK_TEST_HOOK, src/lib/deeplink-entry.ts) — the same function the
 * plugin's onOpenUrl callback calls, with the same parser behind it. The
 * seam is the OS, and only the OS.
 *
 * Nothing is mocked: a real `gadak demo` serve backs the app, so the key
 * this spec deep-links to is a key that serve actually has.
 */
import { expect, test } from './helpers'

/** Mirrors DEEP_LINK_TEST_HOOK in src/lib/deeplink-entry.ts. */
const HOOK = '__gadakDeepLink'

/** Hands one URL to the app exactly as the OS callback would. */
async function deliver(page: import('@playwright/test').Page, url: string): Promise<void> {
  await page.evaluate(
    ([hook, raw]) => {
      const fn = (window as unknown as Record<string, unknown>)[hook!]
      if (typeof fn !== 'function') throw new Error(`${hook} is not exposed — the entry point did not bind`)
      ;(fn as (u: string) => void)(raw!)
    },
    [HOOK, url] as const,
  )
}

/** Boots the app and returns an issue key this serve really has. */
async function bootAndPickKey(page: import('@playwright/test').Page): Promise<string> {
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.locator('nav.safe-bottom').waitFor()
  const firstRow = page.locator('.pane:not(.off) button.row').first()
  await firstRow.waitFor()
  await firstRow.click()
  await page.locator('button.back').waitFor()
  const key = (await page.locator('.bar-key').first().innerText()).trim()
  expect(key, 'the demo serve produced an issue key').not.toBe('')
  // Back to the list, so the deep link is what opens the detail below.
  await page.locator('button.back').click()
  await expect(page.locator('button.back')).toHaveCount(0)
  return key
}

test('a gadak:// link opens that issue', async ({ page }) => {
  const key = await bootAndPickKey(page)

  await deliver(page, `gadak://view?issue=${encodeURIComponent(key)}`)

  await page.locator('button.back').waitFor()
  await expect(page.locator('.bar-key').first()).toHaveText(key)
})

test('a link naming someone else’s workspace opens nothing', async ({ page }) => {
  // The risk GDK-873 names: `/w/<profile>` addresses a mount on a computer.
  // This phone has no workspaces, so the same key there is a different
  // issue here — and opening it anyway would be a silent lie.
  const key = await bootAndPickKey(page)

  await deliver(page, `gadak://view/w/someone-else?issue=${encodeURIComponent(key)}`)

  // Give the app the same chance to navigate that the accepted link got.
  await page.waitForTimeout(300)
  await expect(page.locator('button.back')).toHaveCount(0)
})

test('a malformed link, an unknown action, and another app’s scheme all open nothing', async ({ page }) => {
  const key = await bootAndPickKey(page)
  const encoded = encodeURIComponent(key)

  for (const url of [
    `gadak://view?issue=${encoded}#panel`, // fragments are not in the grammar
    `gadak://view?issue=../../etc/passwd`, // not an issue key
    `gadak://timeline?issue=${encoded}`, // an action this build has no screen for
    `myapp://view?issue=${encoded}`, // not ours at all
    `gadak://view?pj=GDK&sc=inprogress`, // a filtered list, which the phone cannot show
  ]) {
    await deliver(page, url)
  }

  await page.waitForTimeout(300)
  await expect(page.locator('button.back')).toHaveCount(0)

  // And the app is still alive and usable afterwards — a refused link must
  // not leave the webview in a broken state.
  await deliver(page, `gadak://view?issue=${encoded}`)
  await page.locator('button.back').waitFor()
  await expect(page.locator('.bar-key').first()).toHaveText(key)
})
