/*
 * A sync somebody asked for out loud must be audible, even when it lands on
 * top of the quiet focus-time pull. Before this, single-flight handed the
 * caller the running promise and kept its quietness, so pressing Sync now
 * during a background pull produced no request and no toast — nothing at all
 * happened, as far as the screen was concerned.
 */
import { beforeEach, describe, expect, test, vi } from 'vitest'

const toast = vi.fn()

vi.mock('./api', () => ({
  ApiError: class ApiError extends Error {
    code?: string
  },
  startSync: vi.fn(async () => {}),
  // Never settles: the run stays in flight for the whole test, which is the
  // window this contract is about.
  getSyncProgress: vi.fn(() => new Promise(() => {})),
}))
vi.mock('./i18n', () => ({ t: (key: string) => key }))
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

describe('runSyncNow voice', () => {
  beforeEach(() => {
    toast.mockClear()
    vi.resetModules()
  })

  test('a background pull alone says nothing', async () => {
    const { runSyncNow } = await import('./sync-now')
    void runSyncNow('incremental', { quiet: true })
    await Promise.resolve()
    expect(toast).not.toHaveBeenCalled()
  })

  test('Sync now during a background pull is acknowledged', async () => {
    const { runSyncNow } = await import('./sync-now')
    void runSyncNow('incremental', { quiet: true })
    await Promise.resolve()
    expect(toast).not.toHaveBeenCalled()

    void runSyncNow('incremental')
    expect(toast).toHaveBeenCalledWith('sync.starting')
  })

  test('a second background pull does not un-mute a run somebody asked for', async () => {
    const { runSyncNow } = await import('./sync-now')
    void runSyncNow('incremental')
    await Promise.resolve()
    toast.mockClear()

    void runSyncNow('incremental', { quiet: true })
    expect(toast).not.toHaveBeenCalled()
  })
})
