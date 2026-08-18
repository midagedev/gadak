import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  resolveWindowChrome,
  sidebarLogoRowClass,
  WINDOW_CHROME_NATIVE,
  WINDOW_CHROME_TRAFFIC_LIGHTS_INSET,
} from './config'

/**
 * GDK-207: the 90px title-bar class follows windowChrome, not desktop.
 * Native chrome (Linux / Windows app, or an explicit native document) must
 * not reserve the macOS traffic-light gap.
 *
 * loadConfig is driven through the public API with fetch stubbed — same seam
 * as config.test.ts. There is no component-mount harness (vitest is node,
 * no svelte plugin), so Sidebar.svelte wiring is checked on the source.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const SIDEBAR = join(HERE, '../components/shell/Sidebar.svelte')

async function loadConfigWith(body: unknown, ok = true) {
  vi.resetModules()
  vi.stubGlobal('window', { location: { pathname: '/' } })
  vi.stubGlobal('fetch', async () =>
    ok
      ? new Response(JSON.stringify(body), { status: 200 })
      : new Response('missing', { status: 404 }),
  )
  const mod = await import('./config')
  await mod.loadConfig()
  return mod
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('resolveWindowChrome', () => {
  test('tokens name the chrome, not the OS', () => {
    expect(WINDOW_CHROME_TRAFFIC_LIGHTS_INSET).toBe('traffic-lights-inset')
    expect(WINDOW_CHROME_NATIVE).toBe('native')
  })

  test('browser / omitted field is native', () => {
    expect(resolveWindowChrome({})).toBe(WINDOW_CHROME_NATIVE)
    expect(resolveWindowChrome({ desktop: false })).toBe(WINDOW_CHROME_NATIVE)
  })

  test('desktop without a chrome field keeps the old inset meaning', () => {
    expect(resolveWindowChrome({ desktop: true })).toBe(WINDOW_CHROME_TRAFFIC_LIGHTS_INSET)
  })

  test('explicit native wins over desktop', () => {
    expect(resolveWindowChrome({ desktop: true, windowChrome: 'native' })).toBe(
      WINDOW_CHROME_NATIVE,
    )
  })

  test('explicit inset is inset', () => {
    expect(
      resolveWindowChrome({ desktop: true, windowChrome: 'traffic-lights-inset' }),
    ).toBe(WINDOW_CHROME_TRAFFIC_LIGHTS_INSET)
  })

  test('unknown token falls back', () => {
    expect(resolveWindowChrome({ desktop: true, windowChrome: 'mac' })).toBe(
      WINDOW_CHROME_TRAFFIC_LIGHTS_INSET,
    )
    expect(resolveWindowChrome({ desktop: false, windowChrome: 'mac' })).toBe(
      WINDOW_CHROME_NATIVE,
    )
  })
})

describe('sidebar logo row class follows chrome', () => {
  test('native desktop does not apply the 90px titlebar class', async () => {
    const mod = await loadConfigWith({ desktop: true, windowChrome: 'native' })
    expect(mod.windowChrome()).toBe(WINDOW_CHROME_NATIVE)
    expect(mod.trafficLightsInContent()).toBe(false)
    expect(mod.sidebarLogoRowClass()).toBe('px-4')
    expect(mod.isDesktop()).toBe(true)
  })

  test('inset desktop applies the 90px titlebar class', async () => {
    const mod = await loadConfigWith({
      desktop: true,
      windowChrome: 'traffic-lights-inset',
    })
    expect(mod.trafficLightsInContent()).toBe(true)
    expect(mod.sidebarLogoRowClass()).toBe('desktop-titlebar-row')
  })

  test('browser tab stays on px-4', async () => {
    const mod = await loadConfigWith(null, false)
    expect(mod.sidebarLogoRowClass()).toBe('px-4')
    expect(mod.trafficLightsInContent()).toBe(false)
  })

  test('Sidebar.svelte consumes sidebarLogoRowClass, not desktop ?', () => {
    const src = readFileSync(SIDEBAR, 'utf8')
    expect(src.includes("desktop ? 'desktop-titlebar-row'")).toBe(false)
    expect(src.includes('sidebarLogoRowClass')).toBe(true)
    expect(src.includes('{logoRowClass}')).toBe(true)
  })
})

describe('sidebarLogoRowClass default (unloaded module)', () => {
  test('DEFAULTS are native / px-4', () => {
    expect(sidebarLogoRowClass()).toBe('px-4')
  })
})
