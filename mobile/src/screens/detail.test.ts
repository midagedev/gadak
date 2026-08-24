/*
 * Detail screen contract tests. The screen's decision rules live as pure
 * exports in Detail.svelte's module script (there is no DOM test harness
 * in mobile — vitest runs node), so these pin the three contracts the
 * markup consumes: the header's status owner is the list row (the detail
 * payload carries no status axes), the transition step machine's
 * optimistic/rollback branches, and the ApiError.code → next-move copy.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { tauriFetch } = vi.hoisted(() => ({ tauriFetch: vi.fn() }))
vi.mock('@tauri-apps/plugin-http', () => ({ fetch: tauriFetch }))

const { invoke } = vi.hoisted(() => ({ invoke: vi.fn() }))
vi.mock('@tauri-apps/api/core', () => ({ invoke }))

import type { QueueRow, Transition } from '../lib/api'
import { displayHeader, errorKey, transitionStep, type TransitionState } from './Detail.svelte'

// The measured detail payload shape (flow-report Q3 ②, live serve): body,
// comments, history — and no status_category / priority_rank for the issue
// itself. If a server change ever adds them, this line is the reminder
// that the screen still reads its header from the list row by design.
const DETAIL_FIXTURE = {
  issue_key: 'STD-1',
  description_adf: { type: 'doc', version: 1, content: [] },
  description_text: 'flow-probe body',
  comments: [
    {
      comment_id: '1',
      author_account_id: '5b10a2844c20165700ede21g',
      body: 'first',
      raw_body: null,
      created_at: '2026-08-24T01:02:03Z',
    },
  ],
  history: [],
}

// A Korean-account row: the display name is locale data, the axis is
// status_category — the header must show the former and key on nothing.
const ROW: QueueRow = {
  issue_key: 'STD-1',
  summary: 'summary',
  status: '진행 중',
  status_category: 'inprogress',
  priority: '높음',
  priority_rank: 3,
  assignee: null,
  updated_at: null,
}

const TR_DONE: Transition = { id: '31', name: 'Done', to_status: 'Done', to_category: 'done' }
const TR_START: Transition = { id: '11', name: 'In Progress', to_status: 'In Progress', to_category: 'inprogress' }

beforeEach(() => {
  tauriFetch.mockReset()
  invoke.mockReset()
})

describe('the list row owns the header status', () => {
  it('the detail payload carries no status axes (measured) — the row is required', () => {
    const keys = Object.keys(DETAIL_FIXTURE)
    expect(keys).not.toContain('status_category')
    expect(keys).not.toContain('priority_rank')
  })

  it('the idle header shows the row display name, not a category label', () => {
    expect(displayHeader(ROW, { phase: 'idle' })).toEqual({
      status: '진행 중',
      status_category: 'inprogress',
    })
  })

  it('a detail deep link without a row renders no chip rather than a fake status', () => {
    expect(displayHeader(null, { phase: 'idle' })).toBeNull()
  })
})

describe('transition optimistic / rollback branches', () => {
  it('pick swaps the chip to the transition target before the POST resolves', () => {
    const picked = transitionStep(ROW, { phase: 'idle' }, { type: 'pick', transition: TR_DONE })
    expect(picked.phase).toBe('pending')
    expect(displayHeader(ROW, picked)).toEqual({ status: 'Done', status_category: 'done' })
  })

  it('fail rolls the chip back to the pre-attempt status', () => {
    const picked = transitionStep(ROW, { phase: 'idle' }, { type: 'pick', transition: TR_DONE })
    const failed = transitionStep(ROW, picked, { type: 'fail' })
    expect(failed.phase).toBe('failed')
    expect(displayHeader(ROW, failed)).toEqual({ status: '진행 중', status_category: 'inprogress' })
  })

  it('ack lets the server row win — even when no list row existed', () => {
    const picked = transitionStep(null, { phase: 'idle' }, { type: 'pick', transition: TR_DONE })
    const serverRow: QueueRow = { ...ROW, status: '완료', status_category: 'done' }
    const acked = transitionStep(null, picked, { type: 'ack', row: serverRow })
    expect(acked.phase).toBe('confirmed')
    expect(displayHeader(null, acked)).toEqual({ status: '완료', status_category: 'done' })
  })

  it('a chained pick keeps the original snapshot — rollback returns to the true before', () => {
    const first = transitionStep(ROW, { phase: 'idle' }, { type: 'pick', transition: TR_DONE })
    const second = transitionStep(ROW, first, { type: 'pick', transition: TR_START })
    const failed = transitionStep(ROW, second, { type: 'fail' })
    expect(displayHeader(ROW, failed)).toEqual({ status: '진행 중', status_category: 'inprogress' })
  })
})

describe('error code → next-move copy', () => {
  it('branches on the three server codes', () => {
    expect(errorKey('forbidden_host')).toBe('detail.error.forbiddenHost')
    expect(errorKey('pairing_rejected')).toBe('detail.error.pairingRejected')
    expect(errorKey('credential_required')).toBe('detail.error.credentialRequired')
  })

  it('unknown codes and transport failures share the offline sentence', () => {
    expect(errorKey(undefined)).toBe('detail.error.offline')
    expect(errorKey('whatever_else')).toBe('detail.error.offline')
  })
})
