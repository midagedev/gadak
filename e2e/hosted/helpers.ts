import { expect, type Page } from '@playwright/test'

/**
 * The hosted first frame (tools/hosted-demo/first-frame.html) sits above
 * the SPA until the visitor dismisses it. Existing hosted specs exercise
 * the app, so they have to click through first.
 */
export async function dismissHostedFirstFrame(page: Page): Promise<void> {
  const open = page.getByTestId('hosted-first-frame-open')
  await expect(
    open,
    'hosted first frame is missing — rebuild with node tools/hosted-demo/build.mjs',
  ).toBeVisible({ timeout: 10_000 })
  await open.click()
  await expect(page.getByTestId('hosted-first-frame')).toHaveCount(0)
}
