import { expect, type Page } from '@playwright/test'

/**
 * GDK-335: the hosted demo paints the SPA immediately. Callers that used
 * to click through the first-frame overlay still go through here so one
 * helper owns the entry assertion: no extra click, issue-layout visible.
 */
export async function dismissHostedFirstFrame(page: Page): Promise<void> {
  await expect(
    page.getByTestId('issue-layout'),
    'hosted demo must paint the SPA with no overlay click — rebuild with node tools/hosted-demo/build.mjs',
  ).toBeVisible({ timeout: 60_000 })
}
