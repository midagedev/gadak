import { afterEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

// The tour module imports the store only so tour() can call openIssue /
// switchTab. This suite asserts arming, not the walk, so the store is a stub.
vi.mock('./store.svelte', () => ({
  app: { tab: 'issues', detailKey: null },
  openIssue: vi.fn(),
  closeIssue: vi.fn(),
  switchTab: vi.fn(),
}))

import { armDemoTourInDev, isDemoTourArmed } from './demo-tour'
import { app } from './store.svelte'

const srcPath = fileURLToPath(new URL('./demo-tour.ts', import.meta.url))

describe('demo-tour arming', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('does not fetch a flag URL when nothing is armed (vite SPA fallback returns 200)', async () => {
    vi.stubGlobal('location', { search: '' })
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    armDemoTourInDev()
    await Promise.resolve()
    await Promise.resolve()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('does not probe /__demo-tour__ — vite answers that path with index.html and 200', () => {
    const src = readFileSync(srcPath, 'utf8')
    expect(src).not.toMatch(/fetch\(\s*['"]\/__demo-tour__['"]/)
  })

  it('isDemoTourArmed is false without ?demo-tour and true with it', () => {
    vi.stubGlobal('location', { search: '' })
    expect(isDemoTourArmed()).toBe(false)
    vi.stubGlobal('location', { search: '?foo=1' })
    expect(isDemoTourArmed()).toBe(false)
    vi.stubGlobal('location', { search: '?demo-tour' })
    expect(isDemoTourArmed()).toBe(true)
    vi.stubGlobal('location', { search: '?demo-tour=1' })
    expect(isDemoTourArmed()).toBe(true)
  })

  it('leaves tab and detailKey unchanged when nothing is armed', () => {
    vi.stubGlobal('location', { search: '' })
    const info = vi.spyOn(console, 'info').mockImplementation(() => {})
    const tab = app.tab
    const detailKey = app.detailKey
    armDemoTourInDev()
    expect(app.tab).toBe(tab)
    expect(app.detailKey).toBe(detailKey)
    expect(info).not.toHaveBeenCalled()
  })
})
