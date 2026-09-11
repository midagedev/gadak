import { afterEach, describe, expect, test, vi } from 'vitest'

/**
 * GDK-52: hasServer is the single owner of "does this deployment have a server
 * behind it at all". Surfaces render server-backed entry points only after
 * asking here, so an absent server is never discovered by failing at click
 * time. GDK-1482 collapsed the per-verb form (the answer never read the verb).
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

describe('hasServer (GDK-52, GDK-1482)', () => {
  test('serve (config.json unreachable → defaults): there is a server', async () => {
    const mod = await loadConfigWith(null, false)
    expect(mod.surface()).toBe('serve')
    expect(mod.hasServer()).toBe(true)
  })

  test('hosted demo: a static snapshot has no server', async () => {
    const mod = await loadConfigWith({ hostedDemo: true })
    expect(mod.surface()).toBe('hosted')
    expect(mod.hasServer()).toBe(false)
  })

  test('desktop: serves its own config.json — it has a server', async () => {
    const mod = await loadConfigWith({ desktop: true })
    expect(mod.surface()).toBe('desktop')
    expect(mod.hasServer()).toBe(true)
  })
})

describe('originType / transport (GDK-1278, server-owned)', () => {
  test('a paired workspace is a gadak origin reached remotely, whatever kind says', async () => {
    // The case that split the axes: kind still reads 'connected' while the
    // origin is gadak's own tracker one machine away.
    const paired = await loadConfigWith({
      workspaceKind: 'connected',
      originType: 'gadak',
      transport: 'remote',
      jiraBaseUrl: '',
    })
    expect(paired.config().workspaceKind).toBe('connected')
    expect(paired.config().originType).toBe('gadak')
    expect(paired.config().transport).toBe('remote')
  })

  test('unknown documents and unknown values stay empty, never guessed', async () => {
    const older = await loadConfigWith({ workspaceKind: 'standalone', jiraBaseUrl: '' })
    expect(older.config().originType).toBe('')
    expect(older.config().transport).toBe('')

    const garbage = await loadConfigWith({ originType: 'self-hosted', transport: 'tailnet' })
    expect(garbage.config().originType).toBe('')
    expect(garbage.config().transport).toBe('')
  })
})

describe('workspaceKind (server-owned, never inferred)', () => {
  test('defaults and missing/garbage documents are unknown, not built-in', async () => {
    const missing = await loadConfigWith(null, false)
    expect(missing.config().workspaceKind).toBe('')
    expect(missing.isBuiltInWorkspace()).toBe(false)

    const emptySite = await loadConfigWith({ jiraBaseUrl: '' })
    expect(emptySite.config().workspaceKind).toBe('')
    expect(emptySite.isBuiltInWorkspace()).toBe(false)

    const garbage = await loadConfigWith({ workspaceKind: 'local', jiraBaseUrl: '' })
    expect(garbage.parseWorkspaceKind('local')).toBe('')
    expect(garbage.config().workspaceKind).toBe('')
    expect(garbage.isBuiltInWorkspace()).toBe(false)
  })

  test('connected and built-in come from the document only', async () => {
    const connected = await loadConfigWith({
      workspaceKind: 'connected',
      jiraBaseUrl: '',
    })
    expect(connected.config().workspaceKind).toBe('connected')
    expect(connected.isBuiltInWorkspace()).toBe(false)

    const builtIn = await loadConfigWith({ workspaceKind: 'standalone' })
    expect(builtIn.config().workspaceKind).toBe('standalone')
    expect(builtIn.isBuiltInWorkspace()).toBe(true)
  })
})

describe('capabilities (GDK-1152, server-stated)', () => {
  test('the block answers every axis; originWritable() stays its alias', async () => {
    const mod = await loadConfigWith({
      workspaceKind: 'standalone',
      capabilities: {
        issueWrite: true,
        wikiWrite: true,
        identity: false,
        originDeepLink: false,
        originBaseUrl: '',
        credentialRequired: false,
      },
    })
    expect(mod.can('issueWrite')).toBe(true)
    expect(mod.can('wikiWrite')).toBe(true)
    expect(mod.can('identity')).toBe(false)
    expect(mod.can('originDeepLink')).toBe(false)
    expect(mod.credentialRequired()).toBe(false)
    // The built-in row is the point of the vocabulary: writes both, no
    // identity to gate on, no token errand to sell.
    expect(mod.originWritable()).toBe(true)
  })

  test('a connected cloud workspace: token errand yes, identity yes, alias yes', async () => {
    const mod = await loadConfigWith({
      capabilities: {
        issueWrite: true,
        wikiWrite: true,
        identity: true,
        originDeepLink: true,
        originBaseUrl: 'https://x.example',
        credentialRequired: true,
      },
    })
    expect(mod.credentialRequired()).toBe(true)
    expect(mod.can('identity')).toBe(true)
    expect(mod.config().capabilities.originBaseUrl).toBe('https://x.example')
    expect(mod.originWritable()).toBe(mod.can('issueWrite'))
  })

  test('no block (older server / static export): issueWrite falls back to the legacy field, the rest stay false', async () => {
    const legacy = await loadConfigWith({ originWritable: true })
    expect(legacy.can('issueWrite')).toBe(true)
    expect(legacy.originWritable()).toBe(true)
    // The legacy bool only ever answered issueWrite — the other axes must not
    // be guessed from it (that inference is exactly what GDK-1152 removes).
    expect(legacy.can('wikiWrite')).toBe(false)
    expect(legacy.can('identity')).toBe(false)
    expect(legacy.credentialRequired()).toBe(false)

    const hosted = await loadConfigWith({ hostedDemo: true })
    expect(hosted.can('issueWrite')).toBe(false)
    expect(hosted.credentialRequired()).toBe(false)
  })

  test('a present block wins over a disagreeing legacy field; a partial block falls back per axis', async () => {
    const wins = await loadConfigWith({
      originWritable: true,
      capabilities: { issueWrite: false, credentialRequired: true },
    })
    expect(wins.can('issueWrite')).toBe(false)
    expect(wins.originWritable()).toBe(false)

    const partial = await loadConfigWith({
      originWritable: true,
      capabilities: { credentialRequired: true },
    })
    expect(partial.can('issueWrite')).toBe(true)
    expect(partial.credentialRequired()).toBe(true)
  })

  test('garbage values read as false / empty, never guessed', async () => {
    const mod = await loadConfigWith({
      capabilities: {
        issueWrite: 'yes',
        wikiWrite: 1,
        identity: null,
        originDeepLink: 'true',
        originBaseUrl: 42,
        credentialRequired: 'on',
      },
    })
    expect(mod.can('issueWrite')).toBe(false)
    expect(mod.can('wikiWrite')).toBe(false)
    expect(mod.can('identity')).toBe(false)
    expect(mod.can('originDeepLink')).toBe(false)
    expect(mod.config().capabilities.originBaseUrl).toBe('')
    expect(mod.credentialRequired()).toBe(false)
  })
})
