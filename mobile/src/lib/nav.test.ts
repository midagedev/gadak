/*
 * Navigation state transitions (A-nav). The single-owner contract under
 * pin: one value decides the screen, tab switches are total, Detail is
 * pushed over a tab and pops back to THAT tab, and a notification tap
 * goes straight to Detail whose back is the Queue (ux-report Q5 — there
 * is no notification inbox to restore). Pure steps only; no DOM.
 */
import { describe, expect, it, vi } from 'vitest'

const { tauriFetch } = vi.hoisted(() => ({ tauriFetch: vi.fn() }))
vi.mock('@tauri-apps/plugin-http', () => ({ fetch: tauriFetch }))

const { invoke } = vi.hoisted(() => ({ invoke: vi.fn() }))
vi.mock('@tauri-apps/api/core', () => ({ invoke }))

import type { QueueRow } from './api'
import {
  NAV_HOME,
  openDetailFromNotification,
  openPair,
  openTab,
  popDetail,
  pushDetail,
  rowFor,
} from './nav'

const row = (issue_key: string): QueueRow => ({
  issue_key,
  summary: `${issue_key} summary`,
  status: 'In Progress',
  status_category: 'inprogress',
  priority: null,
  priority_rank: 0,
  assignee: null,
  updated_at: null,
})

describe('home + tab switching', () => {
  it('opens on the Queue tab, paired or not (no forced Pair first screen)', () => {
    expect(NAV_HOME).toEqual({ view: 'tabs', tab: 'queue' })
  })

  it('switches between all three tabs', () => {
    expect(openTab(NAV_HOME, 'search')).toEqual({ view: 'tabs', tab: 'search' })
    expect(openTab({ view: 'tabs', tab: 'search' }, 'more')).toEqual({ view: 'tabs', tab: 'more' })
    expect(openTab({ view: 'tabs', tab: 'more' }, 'queue')).toEqual({ view: 'tabs', tab: 'queue' })
  })

  it('a tab tap is the bail-out from the Pair screen', () => {
    const pair = openPair(NAV_HOME)
    expect(openTab(pair, 'search')).toEqual({ view: 'tabs', tab: 'search' })
  })
})

describe('pair push', () => {
  it('pushes Pair over the current tab (queue-empty CTA)', () => {
    expect(openPair(NAV_HOME)).toEqual({ view: 'pair', back: 'queue' })
  })

  it('pushes Pair over More for the pair-management row', () => {
    expect(openPair({ view: 'tabs', tab: 'more' })).toEqual({ view: 'pair', back: 'more' })
  })
})

describe('detail push/pop + back landing', () => {
  it('a queue row opens Detail with back to the Queue', () => {
    const s = pushDetail(NAV_HOME, 'GDK-1')
    expect(s).toEqual({ view: 'detail', issueKey: 'GDK-1', back: 'queue' })
    expect(popDetail(s)).toEqual({ view: 'tabs', tab: 'queue' })
  })

  it('a search hit opens Detail with back to Search (the query survives)', () => {
    const s = pushDetail({ view: 'tabs', tab: 'search' }, 'GDK-2')
    expect(s).toEqual({ view: 'detail', issueKey: 'GDK-2', back: 'search' })
    expect(popDetail(s)).toEqual({ view: 'tabs', tab: 'search' })
  })

  it('pop is a no-op outside Detail (tab roots have nothing under them)', () => {
    expect(popDetail(NAV_HOME)).toBe(NAV_HOME)
    expect(popDetail({ view: 'tabs', tab: 'search' })).toEqual({ view: 'tabs', tab: 'search' })
  })
})

describe('notification direct open (ux-report Q5)', () => {
  it('a tap with an issue_key goes straight to Detail, back to the Queue', () => {
    const s = openDetailFromNotification('GDK-3')
    expect(s).toEqual({ view: 'detail', issueKey: 'GDK-3', back: 'queue' })
    expect(popDetail(s)).toEqual({ view: 'tabs', tab: 'queue' })
  })

  it('back is the Queue even when the tap arrived over Search', () => {
    // The notification has no tab context — its back is always the Queue.
    const s = openDetailFromNotification('GDK-4')
    expect(s.view === 'detail' && s.back === 'queue').toBe(true)
  })

  it('a tap without an issue_key lands the Queue (no inbox screen exists)', () => {
    expect(openDetailFromNotification(null)).toEqual({ view: 'tabs', tab: 'queue' })
    expect(openDetailFromNotification('')).toEqual({ view: 'tabs', tab: 'queue' })
  })
})

describe('rowFor (the pushed Detail reads the pool)', () => {
  const pool = [row('GDK-1'), row('GDK-2')]

  it('finds the row the tab holds', () => {
    expect(rowFor(pool, 'GDK-2')?.summary).toBe('GDK-2 summary')
  })

  it('null for a key outside the pool (done issue, out-of-queue event)', () => {
    expect(rowFor(pool, 'GDK-9')).toBeNull()
    expect(rowFor([], 'GDK-1')).toBeNull()
  })
})
