/*
 * GDK-349: the browser-Notification toggle must stay visible on a desktop
 * host whose OS notifier is a no-op (Windows). Hiding it on every desktop
 * surface assumed osascript always runs — that is false on Windows, and
 * the copy next to the feed toggle claimed system notifications "always run".
 *
 * No component-mount harness: vitest is environment:'node' and the unit
 * project loads no svelte plugin, so importing .svelte fails outright
 * (HistoryView.test.ts / SearchBox.test.ts). Rendered pixels on serve
 * (surface !== desktop) already show the toggle; the desktop+unsupported
 * case cannot be driven in Playwright without a wails surface. What this
 * file can prove is the hide condition in the source the compiler emits.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const FEATURES_TAB = join(HERE, 'FeaturesTab.svelte')
const SETTINGS_DIALOG = join(HERE, 'SettingsDialog.svelte')
const EN = join(HERE, '../../lib/i18n/en.ts')
const KO = join(HERE, '../../lib/i18n/ko.ts')

describe('GDK-349 browser notify visibility', () => {
  const features = readFileSync(FEATURES_TAB, 'utf8')
  const dialog = readFileSync(SETTINGS_DIALOG, 'utf8')

  test('hide condition is desktop AND os-notify-supported, not desktop alone', () => {
    expect(features, 'old hide was `{#if !onDesktop}`').not.toMatch(/\{#if !onDesktop\}/)
    expect(features).toMatch(/onDesktop && osNotifySupported/)
  })

  test('settings dialog passes GET settings/ runtime.osNotifySupported', () => {
    expect(dialog).toMatch(/osNotifySupported=\{runtime\?\.osNotifySupported/)
  })

  test('en/ko feed-desktop copy does not claim system notifications always run', () => {
    const en = readFileSync(EN, 'utf8')
    const ko = readFileSync(KO, 'utf8')
    expect(en).not.toMatch(/always run/)
    expect(ko).not.toMatch(/항상 동작/)
  })
})
