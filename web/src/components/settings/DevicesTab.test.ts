/*
 * GDK-1047 gates for the Devices tab (desktop only). Same story as
 * FeaturesTab.test.ts: the unit vitest project is environment 'node' with
 * no svelte plugin, so mounting the component is not a thing this project
 * can do — the render contract is asserted against the source the
 * compiler emits, and everything with real logic (tab visibility) is a
 * genuine unit test against the lib.
 *
 * The one behavior this file owns outright: the Devices tab must NOT exist
 * under gadak serve — neither in the header nor via a pasted
 * `settings=devices` URL. No e2e spec asserts the tab set (settings.spec
 * walks tabs by name), so this is where that absence is pinned.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { SETTINGS_TABS } from '../../lib/settings-tabs'
import { isVisibleSettingsTab, visibleSettingsTabs } from '../../lib/integrations'

const HERE = dirname(fileURLToPath(import.meta.url))
const DEVICES_TAB = join(HERE, 'DevicesTab.svelte')
const SETTINGS_DIALOG = join(HERE, 'SettingsDialog.svelte')
const MESSAGES = join(HERE, '../../lib/i18n/messages')

function catalogSource(): string {
  // Copy lives in messages/*.ts ({en,ko,ja} per key), not en.ts/ko.ts/ja.ts.
  return readdirSync(MESSAGES)
    .filter((n) => n.endsWith('.ts'))
    .map((n) => readFileSync(join(MESSAGES, n), 'utf8'))
    .join('\n')
}

describe('GDK-1047 devices tab is desktop-only', () => {
  test('serve hides it from the header (visibleSettingsTabs)', () => {
    expect(visibleSettingsTabs(SETTINGS_TABS, false)).not.toContain('devices')
    expect(visibleSettingsTabs(SETTINGS_TABS, true)).toContain('devices')
  })

  test('serve refuses a pasted settings=devices URL (isVisibleSettingsTab)', () => {
    expect(isVisibleSettingsTab('devices', SETTINGS_TABS, false)).toBe(false)
    expect(isVisibleSettingsTab('devices', SETTINGS_TABS, true)).toBe(true)
  })

  test('the tab sits between integrations and about', () => {
    const tabs = [...SETTINGS_TABS]
    expect(tabs.indexOf('devices')).toBe(tabs.indexOf('integrations') + 1)
    expect(tabs.indexOf('about')).toBe(tabs.indexOf('devices') + 1)
  })
})

describe('GDK-1047 devices tab render contract', () => {
  const src = readFileSync(DEVICES_TAB, 'utf8')

  test('the QR is an <img> fed by the server data URI, not a client-side renderer', () => {
    expect(src).toMatch(/<img\s[^>]*src=\{minted\.qr_png\}/)
    // No new npm dependency: nothing imports a QR library.
    expect(src).not.toMatch(/from '[^']*qr/i)
    expect(src).not.toMatch(/import\s+qrcode/)
  })

  test('the offer string is masked until explicitly revealed', () => {
    // Masked rendering: head…tail, never the full string by default.
    expect(src).toMatch(/minted\.offer\.slice\(0, 6\)/)
    expect(src).toMatch(/minted\.offer\.slice\(-4\)/)
    // The full offer renders only behind the revealed flag.
    expect(src).toMatch(/\{revealed \? minted\.offer :/)
    // Reveal is explicit UI state, default hidden.
    expect(src).toMatch(/let revealed = \$state\(false\)/)
  })

  test('the credential lives in component state only — no persistence', () => {
    expect(src).not.toMatch(/localStorage/)
    expect(src).not.toMatch(/sessionStorage/)
    expect(src).not.toMatch(/indexedDB/)
  })

  test('the scope select offers serve and origin only — terminal is not minted from a form', () => {
    expect(src).toMatch(/<option value="serve">/)
    expect(src).toMatch(/<option value="origin">/)
    expect(src).not.toMatch(/<option value="terminal">/)
  })

  test('the local-routing row (_home) carries no revoke button', () => {
    expect(src).toMatch(/\{#if !isHome\}/)
  })

  test('mint errors map server codes to catalog keys, with a fallback', () => {
    expect(src).toMatch(/MINT_ERROR\[doc\.error \?\? ''\]/)
    expect(src).toMatch(/'settings\.devicesErrFailed'/)
  })
})

describe('GDK-1047 devices tab is wired into the dialog', () => {
  const dialog = readFileSync(SETTINGS_DIALOG, 'utf8')

  test('labeled, shown, and mounted', () => {
    expect(dialog).toMatch(/devices: t\('settings\.tabDevices'\)/)
    expect(dialog).toMatch(/const showDevices = TABS\.some/)
    expect(dialog).toMatch(/tab === 'devices' && showDevices/)
    expect(dialog).toMatch(/<DevicesTab \/>/)
  })
})

describe('GDK-1047 devices copy is complete in every locale', () => {
  const catalog = catalogSource()

  // Every key the component or dialog references, pinned here so a rename
  // fails loudly instead of rendering a raw key.
  const keys = [
    'settings.tabDevices',
    'settings.devicesIntro',
    'settings.devicesLoadFailed',
    'settings.devicesLoading',
    'settings.devicesEmpty',
    'settings.devicesUnavailableNotConfigured',
    'settings.devicesUnavailablePairedAway',
    'settings.devicesColLabel',
    'settings.devicesColScope',
    'settings.devicesColExpires',
    'settings.devicesColState',
    'settings.devicesScopeServe',
    'settings.devicesScopeOrigin',
    'settings.devicesScopeLocalRouting',
    'settings.devicesLabelLabel',
    'settings.devicesScopeLabel',
    'settings.devicesEndpointLabel',
    'settings.devicesEndpointHint',
    'settings.devicesTtlLabel',
    'settings.devicesMint',
    'settings.devicesMintBusy',
    'settings.devicesMinted',
    'settings.devicesLoopbackWarning',
    'settings.devicesOfferLabel',
    'settings.devicesOfferShow',
    'settings.devicesOfferHide',
    'settings.devicesCopyOffer',
    'settings.devicesQrAlt',
    'settings.devicesErrLabelRequired',
    'settings.devicesErrReservedLabel',
    'settings.devicesErrBadScope',
    'settings.devicesErrBadEndpoint',
    'settings.devicesErrBadTtl',
    'settings.devicesErrNoServe',
    'settings.devicesErrLabelExists',
    'settings.devicesErrFailed',
    'settings.devicesRevoke',
    'settings.devicesRevoked',
    'settings.devicesHomeRowHint',
    'settings.devicesStateActive',
    'settings.devicesStateExpired',
    'settings.devicesStateRevoked',
  ]

  test.each(keys)('%s exists in the catalog', (key) => {
    expect(catalog).toMatch(new RegExp(`'${key.replace(/\./g, '\\.')}':`))
  })

  test('the params the copy interpolates are named, not positional', () => {
    expect(catalog).toMatch(/'settings\.devicesMinted':[\s\S]*?\{label\}[\s\S]*?\{expires\}/)
    expect(catalog).toMatch(/'settings\.devicesErrLabelExists':[\s\S]*?\{label\}/)
  })

  test('every devices entry carries all three locales', () => {
    const src = readFileSync(join(MESSAGES, 'settings.ts'), 'utf8')
    const blocks = src.match(/'settings\.devices[A-Za-z]*':\s*\{[\s\S]*?\},/g) ?? []
    const tab = src.match(/'settings\.tabDevices':\s*\{[\s\S]*?\},/)?.[0] ?? ''
    for (const block of [...blocks, tab]) {
      expect(block, `${block.slice(0, 40)}… misses a locale`).toMatch(/en:/)
      expect(block).toMatch(/ko:/)
      expect(block).toMatch(/ja:/)
    }
  })
})
