import { afterEach, describe, expect, test, vi } from 'vitest'

/**
 * GDK-52: hasServerVerb is the single owner of "can this deployment answer
 * server verbs at all". Surfaces render server-backed entry points only after
 * asking here, so an absent verb is never discovered by failing at click time.
 *
 * State is set through the module's own public API (loadConfig) with fetch
 * stubbed — config.ts deliberately has no setter, the same seam
 * hosted-fetch.test.ts drives from the other side.
 */

async function loadConfigWith(body: unknown, ok = true) {
  vi.resetModules()
  // runtimeBase() reads window.location.pathname; the unit project runs in node.
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

describe('hasServerVerb / serverVerbReport (GDK-52)', () => {
  test('serve (config.json unreachable → defaults): every server verb is answerable', async () => {
    const mod = await loadConfigWith(null, false)
    expect(mod.surface()).toBe('serve')
    for (const v of mod.SERVER_VERBS) {
      expect(mod.hasServerVerb(v)).toBe(true)
    }
    expect(mod.serverVerbReport()).toEqual({
      bodySearch: true,
      docs: true,
      settings: true,
    })
  })

  test('hosted demo: a static snapshot has no server — none are answerable', async () => {
    const mod = await loadConfigWith({ hostedDemo: true })
    expect(mod.surface()).toBe('hosted')
    for (const v of mod.SERVER_VERBS) {
      expect(mod.hasServerVerb(v)).toBe(false)
    }
  })

  test('desktop: serves its own config.json — all answerable', async () => {
    const mod = await loadConfigWith({ desktop: true })
    expect(mod.surface()).toBe('desktop')
    for (const v of mod.SERVER_VERBS) {
      expect(mod.hasServerVerb(v)).toBe(true)
    }
  })

  test('serverVerbReport covers exactly the known verbs', async () => {
    const mod = await loadConfigWith({ hostedDemo: true })
    const report = mod.serverVerbReport()
    expect(Object.keys(report).sort()).toEqual([...mod.SERVER_VERBS].sort())
    expect(report).toEqual({ bodySearch: false, docs: false, settings: false })
  })
})
