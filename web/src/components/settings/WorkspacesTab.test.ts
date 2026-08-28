/*
 * GDK-1096 gates for the Workspaces settings tab. Same split as
 * DevicesTab.test.ts: the unit vitest project is environment 'node' with
 * no svelte plugin, so the render contract is asserted against the source
 * the compiler emits, and everything with real logic (the api client) is a
 * genuine unit test with fetch stubbed (api.reachability.test.ts pattern).
 *
 * The behaviors this file owns outright:
 *   1. Removal is the two-step contract: probe DELETE without yes=1, and a
 *      commit that only carries destroy_origin through the checkbox.
 *   2. The serving profile's Delete button is disabled before any
 *      round-trip (the server would refuse self_delete anyway).
 *   3. forbidden_host collapses the tab to the local-only notice.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  createWorkspace,
  listWorkspaces,
  removeWorkspace,
  WorkspaceManageError,
} from '../../lib/api'
import { SETTINGS_TABS } from '../../lib/settings-tabs'
import { isVisibleSettingsTab, visibleSettingsTabs } from '../../lib/integrations'

const HERE = dirname(fileURLToPath(import.meta.url))
const WORKSPACES_TAB = join(HERE, 'WorkspacesTab.svelte')
const SETTINGS_DIALOG = join(HERE, 'SettingsDialog.svelte')
const SETTINGS_TABS_TS = join(HERE, '../../lib/settings-tabs.ts')
const MESSAGES = join(HERE, '../../lib/i18n/messages/settings.ts')

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

/* ── The api client (real logic, really tested) ── */

describe('GDK-1096 listWorkspaces refuses where getWorkspaces collapses', () => {
  test('200 → the workspaces array', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => jsonResponse(200, { workspaces: [{ name: 'demo', active: true }] })),
    )
    expect(await listWorkspaces()).toEqual([{ name: 'demo', active: true }])
  })

  test('403 forbidden_host is a thrown WorkspaceManageError, not an empty list', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => jsonResponse(403, { error: 'forbidden_host' })),
    )
    const err = await listWorkspaces().catch((e: unknown) => e)
    expect(err).toBeInstanceOf(WorkspaceManageError)
    expect(err).toMatchObject({ status: 403, error: 'forbidden_host', detail: null })
  })
})

describe('GDK-1096 createWorkspace', () => {
  test('POSTs the A1 body: name + kind standalone, projects only when given', async () => {
    const fetchMock = vi.fn(async () => jsonResponse(201, { name: 'side', kind: 'standalone', persist: '/x' }))
    vi.stubGlobal('fetch', fetchMock)
    await createWorkspace('side')
    await createWorkspace('other', ' NMB, NMA ')
    const [first, second] = fetchMock.mock.calls as unknown as [unknown, RequestInit][]
    for (const call of [first, second]) {
      expect(call[0]).toBe('/api/v1/workspaces')
      expect(call[1].method).toBe('POST')
      expect(call[1].credentials).toBe('same-origin')
    }
    expect(JSON.parse(String(first[1].body))).toEqual({ name: 'side', kind: 'standalone' })
    expect(JSON.parse(String(second[1].body))).toEqual({
      name: 'other',
      kind: 'standalone',
      projects: 'NMB, NMA',
    })
  })

  test('409 exists surfaces as {error, detail}', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(409, { error: 'exists' })))
    const err = await createWorkspace('side').catch((e: unknown) => e)
    expect(err).toMatchObject({ status: 409, error: 'exists', detail: null })
  })
})

describe('GDK-1096 removeWorkspace — probe vs commit', () => {
  test('the probe DELETE carries no query at all', async () => {
    const fetchMock = vi.fn(async () => jsonResponse(400, { error: 'needs_yes', detail: 'refusing…' }))
    vi.stubGlobal('fetch', fetchMock)
    await removeWorkspace('side').catch(() => undefined)
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/v1/workspaces/side')
    expect(url).not.toContain('?')
    expect(init.method).toBe('DELETE')
  })

  test('yes+destroyOrigin map to ?yes=1&destroy_origin=1, name is encoded', async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse(200, { removed: 'a b', kind: 'standalone', origin_destroyed: true, advisories: ['note'] }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const res = await removeWorkspace('a b', { yes: true, destroyOrigin: true })
    const [url] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/v1/workspaces/a%20b?yes=1&destroy_origin=1')
    expect(res.advisories).toEqual(['note'])
    expect(res.origin_destroyed).toBe(true)
  })

  test('the needs_destroy_origin detail survives verbatim, newlines included', async () => {
    const detail = 'refusing: "side" is a standalone workspace\n  persist: /somewhere/side/origin.sqlite\n  to remove it anyway: …'
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => jsonResponse(400, { error: 'needs_destroy_origin', detail })),
    )
    const err = await removeWorkspace('side').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(WorkspaceManageError)
    expect((err as WorkspaceManageError).detail).toBe(detail)
  })
})

/* ── The component's render contract (source the compiler emits) ── */

describe('GDK-1096 WorkspacesTab render contract', () => {
  const src = readFileSync(WORKSPACES_TAB, 'utf8')

  test('removal is two-step: probe without yes, commit with it', () => {
    // Probe: single-argument call — no yes, no destroy_origin.
    expect(src).toMatch(/api\.removeWorkspace\(row\.name\)/)
    // Commit: yes is non-negotiable, destroy_origin rides the checkbox.
    expect(src).toMatch(/api\.removeWorkspace\(confirm\.row\.name, \{[\s\S]*?yes: true[\s\S]*?destroyOrigin: confirm\.destroyOrigin/)
  })

  test('the serving profile row cannot start a removal from the UI', () => {
    expect(src).toMatch(/if \(row\.active\) return/)
    expect(src).toMatch(/disabled=\{row\.active\}/)
    expect(src).toMatch(/workspacesActiveHint/)
  })

  test('the server refusal detail and advisories render verbatim', () => {
    // pre-wrap: the refusal wording is line-broken by its owner, not by us.
    expect(src).toMatch(/whitespace-pre-wrap[\s\S]*?\{confirm\.detail\}/)
    expect(src).toMatch(/\{#each advisories as line\}/)
  })

  test('a standalone persist commit is gated on the only-copy checkbox', () => {
    expect(src).toMatch(/workspaces-destroy-origin/)
    expect(src).toMatch(/confirm\.refusal === 'needs_destroy_origin' && !confirm\.destroyOrigin/)
  })

  test('forbidden_host folds management away instead of erroring', () => {
    expect(src).toMatch(/isRefusal\(e, 'forbidden_host'\)/)
    expect(src).toMatch(/remoteBlocked/)
    expect(src).toMatch(/manageBlocked/)
  })

  test('Esc closes only the confirm dialog while one is open', () => {
    expect(src).toMatch(/addEventListener\('keydown', onCaptureKeydown, \{ capture: true \}\)/)
  })

  test('workspace state is component-only — nothing persisted', () => {
    expect(src).not.toMatch(/localStorage/)
    expect(src).not.toMatch(/sessionStorage/)
  })
})

/* ── Wiring into the dialog ── */

describe('GDK-1096 workspaces tab is wired into the dialog', () => {
  const dialog = readFileSync(SETTINGS_DIALOG, 'utf8')

  test('labeled and mounted, on serve and desktop alike', () => {
    expect(readFileSync(SETTINGS_TABS_TS, 'utf8')).toMatch(/'fields',\n\s+'workspaces',/)
    expect(dialog).toMatch(/workspaces: t\('settings\.tabWorkspaces'\)/)
    expect(dialog).toMatch(/tab === 'workspaces'/)
    expect(dialog).toMatch(/<WorkspacesTab \/>/)
  })

  test('visible under serve (not a desktop-only tab)', () => {
    expect(visibleSettingsTabs(SETTINGS_TABS, false)).toContain('workspaces')
    expect(isVisibleSettingsTab('workspaces', SETTINGS_TABS, false)).toBe(true)
  })

  test('the tab sits after fields and before the desktop-only pair', () => {
    const tabs = [...SETTINGS_TABS]
    expect(tabs.indexOf('workspaces')).toBe(tabs.indexOf('fields') + 1)
    expect(tabs.indexOf('integrations')).toBe(tabs.indexOf('workspaces') + 1)
  })
})

/* ── Copy completeness ── */

describe('GDK-1096 workspaces copy is complete in every locale', () => {
  const catalog = readFileSync(MESSAGES, 'utf8')

  const keys = [
    'settings.tabWorkspaces',
    'settings.workspacesIntro',
    'settings.workspacesLoading',
    'settings.workspacesLoadFailed',
    'settings.workspacesEmpty',
    'settings.workspacesColName',
    'settings.workspacesColSite',
    'settings.workspacesColProjects',
    'settings.workspacesActiveBadge',
    'settings.workspacesUnreadable',
    'settings.workspacesActiveHint',
    'settings.workspacesNameLabel',
    'settings.workspacesProjectsLabel',
    'settings.workspacesCreate',
    'settings.workspacesCreateFailed',
    'settings.workspacesErrExists',
    'settings.workspacesErrInvalidName',
    'settings.workspacesRemote',
    'settings.workspacesRemoveTitle',
    'settings.workspacesDestroyLabel',
    'settings.workspacesDestroyHint',
    'settings.workspacesRemoveFailed',
    'settings.workspacesAdvisories',
  ]

  test.each(keys)('%s exists in the catalog', (key) => {
    expect(catalog).toMatch(new RegExp(`'${key.replace(/\./g, '\\.')}':`))
  })

  test('the params the copy interpolates are named, not positional', () => {
    expect(catalog).toMatch(/'settings\.workspacesErrExists':[\s\S]*?\{name\}/)
  })

  test('every workspaces entry carries all three locales', () => {
    const blocks = catalog.match(/'settings\.workspaces[A-Za-z]*':\s*\{[\s\S]*?\},/g) ?? []
    // tabWorkspaces lives under a different prefix, so it is added by hand.
    const tab = catalog.match(/'settings\.tabWorkspaces':\s*\{[\s\S]*?\},/)?.[0] ?? ''
    const all = [...blocks, tab]
    expect(all.length).toBeGreaterThanOrEqual(keys.length)
    for (const block of all) {
      expect(block, `${block.slice(0, 40)}… misses a locale`).toMatch(/en:/)
      expect(block).toMatch(/ko:/)
      expect(block).toMatch(/ja:/)
    }
  })
})
