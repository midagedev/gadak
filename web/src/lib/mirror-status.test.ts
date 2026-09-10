import { readFileSync } from 'node:fs'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import type { SyncHealth, SyncSourceHealth } from './types'

/*
 * The wording in mirror-status.ts is the one sentence every surface shows
 * about the mirror. These cases pin the decision table so a stale/failed
 * mirror cannot render as "Synced …" (or the reverse) on every chip at once.
 */

const { issues } = vi.hoisted(() => {
  const issues = {
    mirrorActivity: { running: false, source: '', fetched: 0 },
    mirrorSyncing: false,
    get mirrorBusy() {
      return this.mirrorSyncing || this.mirrorActivity.running
    },
    syncHealth: null as SyncHealth | null,
    get mirrorHealth(): SyncSourceHealth | null {
      const sources = this.syncHealth?.sources
      if (!sources || sources.length === 0) return null
      return sources.find((s) => s.key === 'jira') ?? sources[0]
    },
  }
  return { issues }
})

vi.mock('../stores/issues.svelte', () => ({ issues }))

import { busyLabel, fetchingDocuments, mirrorLabel, settledLabel } from './mirror-status'

const NOW = '2026-08-17T12:00:00.000Z'
const THREE_MIN_AGO = '2026-08-17T11:57:00.000Z'

function source(over: Partial<SyncSourceHealth> = {}): SyncSourceHealth {
  return {
    key: 'jira',
    label: 'Jira',
    status: 'healthy',
    synced_at: THREE_MIN_AGO,
    message: '',
    ...over,
  }
}

function settle(over: { overall?: SyncHealth['overall']; source?: Partial<SyncSourceHealth> } = {}): void {
  const src = source(over.source)
  issues.syncHealth = {
    overall: over.overall ?? 'healthy',
    checked_at: NOW,
    sources: [src],
  }
}

function idle(): void {
  issues.mirrorActivity = { running: false, source: '', fetched: 0 }
  issues.mirrorSyncing = false
  issues.syncHealth = null
}

beforeEach(() => {
  idle()
  vi.useFakeTimers()
  vi.setSystemTime(new Date(NOW))
})

afterEach(() => {
  vi.useRealTimers()
})

describe('fetchingDocuments', () => {
  test('true only for a documents pass, or a tab-started pull before a phase is named', () => {
    // Blocks: the DOCS row saying "Fetching documents" during an issues pass
    // (or saying nothing for the first second of a pull this tab started).
    issues.mirrorActivity = { running: true, source: 'documents', fetched: 0 }
    expect(fetchingDocuments()).toBe(true)

    issues.mirrorActivity = { running: true, source: 'issues', fetched: 10 }
    issues.mirrorSyncing = false
    expect(fetchingDocuments()).toBe(false)

    issues.mirrorActivity = { running: false, source: '', fetched: 0 }
    issues.mirrorSyncing = true
    expect(fetchingDocuments()).toBe(true)

    issues.mirrorActivity = { running: false, source: 'issues', fetched: 0 }
    issues.mirrorSyncing = true
    expect(fetchingDocuments()).toBe(false)
  })
})

describe('busyLabel', () => {
  test('idle mirror produces no busy sentence', () => {
    // Blocks: a settled chip overwritten with "Syncing…" when nothing is fetching.
    expect(busyLabel()).toBeNull()
  })

  test('documents pass: count appears only after the first page lands', () => {
    // Blocks: six minutes of an unchanging "Fetching documents…" that looks hung,
    // or a count of zero advertised as progress.
    issues.mirrorActivity = { running: true, source: 'documents', fetched: 0 }
    expect(busyLabel()).toBe('Fetching documents…')

    issues.mirrorActivity = { running: true, source: 'documents', fetched: 12 }
    expect(busyLabel()).toBe('Fetching documents · 12')
  })

  test('issues pass: count appears only after the first page lands', () => {
    // Blocks: the same hung-vs-working ambiguity on the Jira leg.
    issues.mirrorActivity = { running: true, source: 'issues', fetched: 0 }
    expect(busyLabel()).toBe('Syncing issues…')

    issues.mirrorActivity = { running: true, source: 'issues', fetched: 40 }
    expect(busyLabel()).toBe('Syncing issues · 40')
  })

  test('a four-digit count is grouped: 6932 reads 6,932', () => {
    // Blocks: a raw "Syncing issues · 6932" on the leg that actually runs
    // long enough to reach four digits (moved from e2e/docs-ux.spec.ts,
    // which asserted this exact chip sentence in a browser).
    issues.mirrorActivity = { running: true, source: 'issues', fetched: 6932 }
    expect(busyLabel()).toBe('Syncing issues · 6,932')
  })

  test('tab-started pull with no phase yet is the generic busy sentence', () => {
    // Blocks: the first second of a pull this tab started reading as idle.
    issues.mirrorSyncing = true
    issues.mirrorActivity = { running: false, source: '', fetched: 0 }
    expect(busyLabel()).toBe('Syncing…')
  })
})

describe('settledLabel', () => {
  test('no health yet is "Checking sync", not "Never synced"', () => {
    // Blocks: a booting tab telling the user the mirror has never synced.
    issues.syncHealth = null
    expect(settledLabel()).toBe('Checking sync')
  })

  test('failed with an age carries the age; failed with none does not invent one', () => {
    // Blocks: a failed sync rendered as "Synced 3m ago" (fresh) on every surface.
    settle({ overall: 'failed' })
    expect(settledLabel()).toBe('Sync failed · 3m ago')

    settle({ overall: 'healthy', source: { status: 'failed', synced_at: null } })
    expect(settledLabel()).toBe('Sync failed')
  })

  test('missing source, or a source with no parseable age, is "Never synced"', () => {
    // Blocks: a brand-new mirror rendered as "Synced" with an empty when.
    settle({ source: { status: 'missing', synced_at: null } })
    expect(settledLabel()).toBe('Never synced')

    settle({ source: { status: 'healthy', synced_at: null } })
    expect(settledLabel()).toBe('Never synced')
  })

  test('stale / warning is delayed, even when the age itself looks recent', () => {
    // Blocks: a stale mirror (server already said so) rendered as "Synced 3m ago".
    // This file has no clock threshold of its own — it trusts status / overall.
    settle({ source: { status: 'stale', synced_at: THREE_MIN_AGO } })
    expect(settledLabel()).toBe('Sync delayed · 3m ago')

    settle({ overall: 'warning', source: { status: 'healthy', synced_at: THREE_MIN_AGO } })
    expect(settledLabel()).toBe('Sync delayed · 3m ago')
  })

  test('healthy + age is the fresh sentence; age does not flip it to delayed', () => {
    // Blocks: a fresh mirror rendered as "Sync delayed" because the clock is old
    // — or the reverse: status 'healthy' ignored in favour of a homemade threshold.
    settle({ overall: 'healthy', source: { status: 'healthy', synced_at: THREE_MIN_AGO } })
    expect(settledLabel()).toBe('Synced 3m ago')

    settle({
      overall: 'healthy',
      source: { status: 'healthy', synced_at: '2026-08-16T12:00:00.000Z' },
    })
    expect(settledLabel()).toBe('Synced yesterday')
  })
})

describe('mirrorLabel', () => {
  test('busy sentence wins while a pass is running; settled sentence when idle', () => {
    // Blocks: the chip saying "Synced 3m ago" over a live documents backfill.
    settle()
    issues.mirrorActivity = { running: true, source: 'documents', fetched: 8 }
    expect(mirrorLabel()).toBe('Fetching documents · 8')

    idle()
    settle()
    expect(mirrorLabel()).toBe('Synced 3m ago')
  })
})

/*
 * GDK-460 ownership, moved from e2e/docs-ux.spec.ts's "sync status is one
 * sentence" describe by the GDK-1702 cost ladder: the busy sentence
 * belongs to the toolbar chip (and surfaces naming the same pass), and the
 * sidebar row is the sync-history entry — it must not repeat the chip.
 * The kept document-pass e2e case still proves this end to end against a
 * running app; these scans pin where the sentence is allowed to render so
 * a new surface cannot start borrowing it silently.
 */
describe('GDK-460: the busy sentence is the chip\'s, the sidebar row is sync history', () => {
  test('the freshness chip renders mirrorLabel', () => {
    const chip = readFileSync(new URL('../components/shell/FreshnessChip.svelte', import.meta.url), 'utf8')
    expect(chip).toMatch(/mirrorLabel\(\)/)
  })

  test('the sidebar sync row names the log, not the pass', () => {
    const nav = readFileSync(new URL('../components/sidebar/SidebarNav.svelte', import.meta.url), 'utf8')
    const button = nav.slice(
      nav.indexOf('data-testid="sidebar-sync-now"'),
      nav.indexOf('</button>', nav.indexOf('data-testid="sidebar-sync-now"')),
    )
    // Visible text is the log entry; syncLabel may ride the tooltip only.
    // The key moved to the sync.* family (GDK-136) — same sentence, new home.
    expect(button).toMatch(/\{t\('sync\.history'\)\}/)
    expect(button).not.toMatch(/\{syncLabel\}/)
    expect(button).not.toMatch(/\{mirrorLabel\(\)\}/)
  })
})
