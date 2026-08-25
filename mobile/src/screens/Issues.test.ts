import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'Issues.svelte'), 'utf8')

describe('GDK-905 Issues plates are catalog-backed and distinct', () => {
  it('does not claim a last-synced snapshot in the offline banner', () => {
    expect(src).not.toContain('last synced snapshot')
    expect(src).toContain("t('app.offlineBanner')")
  })

  it('gates the offline banner on showOfflineBanner, not on offline alone', () => {
    expect(src).toContain('showOfflineBanner')
    expect(src).not.toMatch(/\{#if app\.offline\}/)
  })

  it('uses list.emptyTitle for an empty mirror and list.noMatchTitle for an empty scope', () => {
    expect(src).toContain("t('list.emptyTitle')")
    expect(src).toContain("t('list.noMatchTitle')")
    expect(src).not.toContain('Nothing here')
    expect(src).not.toContain('No issues on this mirror match this scope.')
  })

  it('paints a failed bootstrap through issuesBootKind, not an infinite skeleton', () => {
    expect(src).toContain('issuesBootKind')
    expect(src).toMatch(/bootKind === 'failed'|=== "failed"/)
    expect(src).toContain("t('list.renderFailedTitle')")
  })
})
