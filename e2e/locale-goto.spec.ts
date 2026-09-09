import { test, expect } from './helpers'
import { forceLocale, gotoApp, DEMO_ISSUE_COUNT_KO } from './helpers'

/*
 * GDK-1727: gotoApp's boot wait used to match the English pool label
 * ("534 issues"), so a spec that booted the app in ko hung 30 s waiting
 * for a string the UI never renders — the shared helper was en-only by
 * accident. The helper now waits on the bare count (digits are identical
 * in every locale), and this spec is the standing proof: it boots in ko
 * and expects gotoApp to come back.
 *
 * The locale sticks because forceLocale only writes localStorage when it
 * is unset: this spec's ko script registers before gotoApp's internal en
 * one, so the en write is a no-op and the UI renders ko.
 */
test('gotoApp completes a Korean boot (GDK-1727)', async ({ page }) => {
  await forceLocale(page, 'ko')
  await gotoApp(page)
  // gotoApp returned: the sidebar pool size rendered in the ko unit.
  await expect(page.getByText(DEMO_ISSUE_COUNT_KO).first()).toBeVisible()
})
