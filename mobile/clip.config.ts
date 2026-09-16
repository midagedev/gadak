import { defineConfig } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gateWebServers, mobileAPIPort, mobileUIPort, UI_ORIGIN } from './e2e/serve'
import { mediaLocale, mediaLocaleTag } from './media.config'

/*
 * The publication camera's moving half (GDK-1955) — the fourth Playwright
 * harness in mobile/, and the only one that records a video.
 *
 *   playwright.config.ts + e2e/   the gate: measures geometry, fails, CI.
 *   shots.config.ts + shots/      the review camera, scratch/, throwaway.
 *   media.config.ts + media/      the publication stills.
 *   clip.config.ts + clip/        this one: one take, three locales, cut to
 *                                 docs/media/phone[.<locale>].mp4 + .gif +
 *                                 phone-poster[.<locale>].png.
 *
 * Why a clip at all: the landing media policy sends a phone screen to a still
 * ("it reads from one frame"), and the user reversed that on 2026-09-16 —
 * "나는 영상이 들어가는게 좋아 이미지 말고". What the still could not say is
 * the part that is actually new: the phone is the same cache, navigated.
 *
 * Servers, viewport and locale handling are media.config.ts's, imported
 * rather than re-spelled. What differs is the testDir and `video`.
 */
const mobileDir = dirname(fileURLToPath(import.meta.url))

/** The phone's own viewport, shared with the gate and the stills camera. */
export const CLIP_VIEWPORT = { width: 402, height: 874 } as const

/*
 * How many pixels the take carries per phone pixel.
 *
 * Playwright's video is a CSS-pixel capture: `video.size` may only scale a
 * frame *down*, and a size larger than the viewport is padded, not rendered.
 * Measured here on 2026-09-16, and the trap is that it fails quietly —
 * viewport 402×874 at deviceScaleFactor 3 with `size` 1206×2622 produced a
 * 1206×2622 webm whose phone sat in the top-left ninth on a flat grey field,
 * 89% of every frame. Nothing errored; the mp4, the gif and the poster were
 * all written; it took a vision round on the rendered page to see it.
 *
 * So the resolution comes from layout instead: the viewport is the phone's
 * size times CLIP_SCALE and the document is zoomed by the same factor, which
 * puts the app back at 402×874 of its own css px while every glyph, hairline
 * and icon is laid out and painted at three times the size. A 1× take is the
 * unreadable 1:1 phone footage docs/runbooks/release-video.md measured; this
 * is the same rectangle the stills camera shoots at deviceScaleFactor 3.
 */
export const CLIP_SCALE = 3

/** The capture surface: the phone's viewport, scaled up by CLIP_SCALE. */
const RECORDED = {
  width: CLIP_VIEWPORT.width * CLIP_SCALE,
  height: CLIP_VIEWPORT.height * CLIP_SCALE,
} as const

export { mediaLocale, mediaLocaleTag }

export default defineConfig({
  testDir: './clip',
  testMatch: '*.spec.ts',
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: 'list',
  timeout: 240_000,
  expect: { timeout: 20_000 },
  globalSetup: join(mobileDir, 'e2e', 'serve.ts'),
  // Port-keyed like the sibling cameras, so a clip run beside a gate does not
  // share a results directory with it.
  outputDir: join(mobileDir, 'test-results', `clip-${mobileUIPort()}-${mobileAPIPort()}`),
  use: {
    baseURL: UI_ORIGIN,
    viewport: RECORDED,
    // 1, not CLIP_SCALE: the zoom above is what carries the resolution, and
    // a device scale factor on top of it would only cost memory — the
    // screencast never reads it.
    deviceScaleFactor: 1,
    isMobile: true,
    hasTouch: true,
    locale: mediaLocaleTag,
    trace: 'off',
    screenshot: 'off',
    video: { mode: 'on', size: RECORDED },
  },
  webServer: gateWebServers(),
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
})
