import { describe, expect, it } from 'vitest'
import {
  nextSelectedAfterKill,
  RUNNING_WINDOW_MS,
  sessionIssueAside,
  sessionLabel,
  sessionNamedByIssue,
  sessionState,
  stripRows,
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

  // GDK-1387: a server that sends the creation ordinal gets a readable
  // default instead of hex; the caller's localized form is used.
  it('builds a readable default from seq before falling back to the id', () => {
    expect(sessionLabel(info({ seq: 3 }))).toBe('shell 3')
    expect(sessionLabel(info({ seq: 3 }), (n) => `셸 ${n}`)).toBe('셸 3')
    expect(sessionLabel(info({ seq: 0 }))).toBe('a1b2c3d4…')
  })

  // GDK-1195: a person's name wins over the issue key; the key stays beside
  // it, and stays the thing the card-to-shell join reads.
  it('prefers the name a person gave, with the issue key as an aside', () => {
    const named = info({ name: 'fix-tax', issue_key: 'GDK-1195', seq: 2 })
    expect(sessionLabel(named)).toBe('fix-tax')
    expect(sessionIssueAside(named)).toBe('GDK-1195')
    expect(sessionNamedByIssue(named)).toBe(true)
    expect(sessionIssueAside(info({ issue_key: 'GDK-1195' }))).toBeNull()
    expect(sessionLabel(info({ name: '   ', issue_key: 'GDK-1195' }))).toBe('GDK-1195')
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
  it('is empty with no sessions — that space is the start action', () => {
    expect(stripRows([], null, NOW)).toEqual([])
  })

  it('a single session is still a tab: the row is the name now (GDK-1199)', () => {
    const rows = stripRows([info({ id: 'one', issue_key: 'GDK-1' })], 'one', NOW)
    expect(rows).toHaveLength(1)
    expect(rows[0].selected).toBe(true)
    expect(rows[0].label).toBe('GDK-1')
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
  })

  it('keeps the server’s order, which is creation order — a row must not move under the pointer', () => {
    const sessions = [info({ id: 'first' }), info({ id: 'second' }), info({ id: 'third' })]
    expect(stripRows(sessions, null, NOW).map((r) => r.id)).toEqual(['first', 'second', 'third'])
  })
})

describe('nextSelectedAfterKill (GDK-1200)', () => {
  const ids = ['a', 'b', 'c']

  it('killing an unshown session moves nothing', () => {
    expect(nextSelectedAfterKill(ids, 'a', 'b')).toBe('b')
    expect(nextSelectedAfterKill(ids, 'c', null)).toBe(null)
  })

  it('killing the shown session hands the pane to the right-hand neighbour first', () => {
    expect(nextSelectedAfterKill(ids, 'b', 'b')).toBe('c')
  })

  it('falls back to the left neighbour at the end of the row', () => {
    expect(nextSelectedAfterKill(ids, 'c', 'c')).toBe('b')
  })

  it('an emptied roster selects nothing — the exit path takes it from there', () => {
    expect(nextSelectedAfterKill(['a'], 'a', 'a')).toBe(null)
  })

  it('a kill the roster no longer knows selects nothing rather than guessing', () => {
    expect(nextSelectedAfterKill(ids, 'zz', 'zz')).toBe(null)
  })
})
