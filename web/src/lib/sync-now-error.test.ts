/*
 * GDK-566: a sync failure toast must never interpolate a wire code.
 * workspace_frozen carries the unfreeze sentence; known write.go codes reuse
 * writeErrorMessage; unknown snake_case falls back without leaking the code.
 */
import { beforeAll, describe, expect, test, vi } from 'vitest'
import { ApiError } from './api'
import { en } from './i18n/en'

const toast = vi.fn()

vi.mock('./api', async () => {
  const actual = await vi.importActual<typeof import('./api')>('./api')
  return {
    ...actual,
    startSync: vi.fn(async () => {}),
    getSyncProgress: vi.fn(() => new Promise(() => {})),
  }
})
vi.mock('../stores/issues.svelte', () => ({
  issues: {
    setMirrorPuller: vi.fn(),
    setMirrorBatchHandler: vi.fn(),
    pullMirror: vi.fn(),
  },
}))
vi.mock('../stores/pages.svelte', () => ({ pages: {} }))
vi.mock('../stores/views.svelte', () => ({ views: {} }))
vi.mock('../stores/write.svelte', () => ({ write: { toast, openSettings: vi.fn() } }))

describe('syncFailureMessage', () => {
  beforeAll(async () => {
    const { initLocale, locale } = await import('./i18n')
    const mem = new Map<string, string>([['gadak_locale', 'en']])
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => mem.get(k) ?? null,
      setItem: (k: string, v: string) => {
        mem.set(k, v)
      },
      removeItem: (k: string) => {
        mem.delete(k)
      },
      clear: () => mem.clear(),
      key: (i: number) => [...mem.keys()][i] ?? null,
      get length() {
        return mem.size
      },
    })
    initLocale()
    expect(locale()).toBe('en')
  })

  test('jira_unavailable becomes the catalog sentence, never the raw code', async () => {
    const { syncFailureMessage } = await import('./sync-now')
    const e = new ApiError(502, 'jira_unavailable', 'jira_unavailable')
    const msg = syncFailureMessage(e)
    expect(msg).toBe(`Sync failed: ${en['write.jiraUnavailable']}`)
    expect(msg).not.toContain('jira_unavailable')
  })

  test('workspace_frozen names how to unfreeze', async () => {
    const { syncFailureMessage } = await import('./sync-now')
    const e = new ApiError(409, 'workspace_frozen', 'workspace_frozen')
    expect(syncFailureMessage(e)).toBe(en['sync.frozen'])
    expect(syncFailureMessage(e)).not.toBe('workspace_frozen')
    expect(en['sync.frozen']).toMatch(/frozen/)
    expect(en['sync.frozen']).toMatch(/unfreeze/i)
  })

  test('unknown snake_case falls back without leaking the code', async () => {
    const { syncFailureMessage } = await import('./sync-now')
    const e = new ApiError(500, 'totally_new_code', 'totally_new_code')
    expect(syncFailureMessage(e)).toBe(en['sync.settledFailed'])
    expect(syncFailureMessage(e)).not.toContain('totally_new_code')
  })
})
