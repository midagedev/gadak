import { describe, expect, it } from 'vitest'
import {
  RUNNING_WINDOW_MS,
  sessionLabel,
  sessionState,
  stripRows,
  stripShowsRows,
  type TerminalSessionInfo,
} from './strip'

const NOW = Date.parse('2026-08-30T12:00:00Z')
const iso = (msAgo: number) => new Date(NOW - msAgo).toISOString()

function info(over: Partial<TerminalSessionInfo> = {}): TerminalSessionInfo {
  return { id: 'a1b2c3d4e5f6a7b8', attached: 1, last_output_at: iso(60_000), ...over }
}

describe('sessionLabel (GDK-1153)', () => {
  it('names the row after the issue the session was claimed for', () => {
    expect(sessionLabel(info({ issue_key: 'GDK-1153' }))).toBe('GDK-1153')
  })

  it('falls back to a short id when no issue is bound', () => {
    expect(sessionLabel(info())).toBe('a1b2c3d4…')
  })

  it('treats a blank binding as no binding', () => {
    expect(sessionLabel(info({ issue_key: '   ' }))).toBe('a1b2c3d4…')
  })

  it('does not ellipsize an id that is already short', () => {
    expect(sessionLabel({ id: 'abc' })).toBe('abc')
  })
})

describe('sessionState (GDK-1163)', () => {
  it('a BEL outranks everything: a session that rang is asking for a person', () => {
    // Attached and mid-burst — every other signal says "fine".
    expect(
      sessionState(info({ needs_attention: true, last_output_at: iso(0), attached: 1 }), NOW),
    ).toBe('needs')
  })

  it('recent output reads as running', () => {
    expect(sessionState(info({ last_output_at: iso(RUNNING_WINDOW_MS - 500) }), NOW)).toBe(
      'running',
    )
  })

  it('output older than the window is not running', () => {
    expect(sessionState(info({ last_output_at: iso(RUNNING_WINDOW_MS + 500) }), NOW)).toBe('quiet')
  })

  it('a watched session with nothing to say is quiet, not a ghost', () => {
    expect(sessionState(info({ attached: 1, last_output_at: iso(600_000) }), NOW)).toBe('quiet')
  })

  it('unwatched but with work on the tty is quiet', () => {
    expect(
      sessionState(info({ attached: 0, pids: [10, 11], last_output_at: iso(600_000) }), NOW),
    ).toBe('quiet')
  })

  it('unwatched with only the shell left is a ghost', () => {
    expect(
      sessionState(info({ attached: 0, pids: [10], last_output_at: iso(600_000) }), NOW),
    ).toBe('ghost')
  })

  it("a session that never printed is not 'running' on a zero timestamp", () => {
    expect(
      sessionState({ id: 'x', attached: 0, pids: [10], last_output_at: '0001-01-01T00:00:00Z' }, NOW),
    ).toBe('ghost')
  })
})

describe('stripRows (GDK-1153)', () => {
  it('is empty with no sessions, and the row list still shows — that space is the start action', () => {
    expect(stripRows([], null, NOW)).toEqual([])
    expect(stripShowsRows(0)).toBe(true)
  })

  it('hides the row list at exactly one session: the rail already names it', () => {
    const rows = stripRows([info({ id: 'one', issue_key: 'GDK-1' })], 'one', NOW)
    expect(rows).toHaveLength(1)
    expect(rows[0].selected).toBe(true)
    expect(stripShowsRows(1)).toBe(false)
  })

  it('marks exactly the pane’s own session as selected, whatever the server counts as attached', () => {
    const rows = stripRows(
      [
        info({ id: 'aaa', issue_key: 'GDK-1', attached: 1 }),
        info({ id: 'bbb', issue_key: 'GDK-2', attached: 1 }),
        info({ id: 'ccc0000011112222', attached: 0, pids: [3] }),
      ],
      'bbb',
      NOW,
    )
    expect(rows.map((r) => r.id)).toEqual(['aaa', 'bbb', 'ccc0000011112222'])
    expect(rows.map((r) => r.selected)).toEqual([false, true, false])
    expect(rows.map((r) => r.label)).toEqual(['GDK-1', 'GDK-2', 'ccc00000…'])
    expect(rows.map((r) => r.namedByIssue)).toEqual([true, true, false])
    expect(stripShowsRows(3)).toBe(true)
  })

  it('keeps the server’s order, which is creation order — a row must not move under the pointer', () => {
    const sessions = [info({ id: 'first' }), info({ id: 'second' }), info({ id: 'third' })]
    expect(stripRows(sessions, null, NOW).map((r) => r.id)).toEqual(['first', 'second', 'third'])
  })
})
