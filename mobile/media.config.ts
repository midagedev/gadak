import { defineConfig } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gateWebServers, mobileAPIPort, mobileUIPort, UI_ORIGIN } from './e2e/serve'

/*
 * The publication camera — the third Playwright harness in mobile/ (GDK-1501's
 * phone half).
 *
 *   playwright.config.ts + e2e/   the gate: measures geometry, fails, CI.
 *   shots.config.ts + shots/      the review camera: one picture per state,
 *                                 English, written under scratch/ because a
 *                                 review capture is throwaway — its verdict
 *                                 dies with the round that asked for it.
 *   media.config.ts + media/      this one: a small fixed set of frames in
 *                                 three locales, written into docs/media/
 *                                 under the committed-asset naming
 *                                 (phone-<stem>.png, .ko/.ja) — the bytes a
 *                                 release ships on the landing page and in
 *                                 the READMEs, so they are committed rather
 *                                 than scratch.
 *
 * Same servers and same viewport as the other two (mobile/e2e/serve.ts owns
 * both, and its stamp check still refuses another worktree's server). What
 * differs is the testDir and the locale: under ko/ja the fixture is a
 * translated copy of the snapshot, handed to the api server via
 * GADAK_MOBILE_API_DB by `make media-phone`; unset, that variable is empty and
 * the gate-serve.sh exec line stays the one the gate has always run, over the
 * bundled examples/demo.db.
 *
 * Run: make media-phone   (GADAK_MEDIA_LOCALE=ko|ja for the variants)
 * Out: docs/media/phone-<stem>[.<locale>].png
 */
const mobileDir = dirname(fileURLToPath(import.meta.url))

/*
 * The locale handle, phone side. GADAK_MEDIA_LOCALE is the variable every
 * localized media target in this repo reads; e2e/helpers.ts mediaLocale is the
 * desktop owner of its meaning and this file is the phone's — the phone tree
 * does not import from e2e/. Unset or empty is en; anything but the three
 * accepted values throws rather than record a silently English frame.
 */
const raw = process.env.GADAK_MEDIA_LOCALE ?? ''
if (!(raw === '' || raw === 'en' || raw === 'ko' || raw === 'ja')) {
  throw new Error(
    `GADAK_MEDIA_LOCALE must be one of en | ko | ja, or unset (meaning en); got ${JSON.stringify(raw)}`,
  )
}
export const mediaLocale: 'en' | 'ko' | 'ja' = raw === '' ? 'en' : raw

/** Playwright's spelling, which lands in navigator.language. */
const PW_LOCALE = { en: 'en-US', ko: 'ko-KR', ja: 'ja-JP' } as const

/** What <html lang> must read back as (web/src/lib/i18n localeTag). */
export const mediaLocaleTag: string = PW_LOCALE[mediaLocale]

export default defineConfig({
  testDir: './media',
  testMatch: '*.spec.ts',
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 180_000,
  expect: { timeout: 20_000 },
  globalSetup: join(mobileDir, 'e2e', 'serve.ts'),
  // Port-keyed like shots.config.ts, so a media run beside a gate does not
  // share a results directory with it.
  outputDir: join(mobileDir, 'test-results', `media-${mobileUIPort()}-${mobileAPIPort()}`),
  use: {
    baseURL: UI_ORIGIN,
    viewport: { width: 402, height: 874 },
    deviceScaleFactor: 3,
    isMobile: true,
    hasTouch: true,
    locale: PW_LOCALE[mediaLocale],
    trace: 'off',
    screenshot: 'off',
  },
  webServer: gateWebServers(),
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
})
