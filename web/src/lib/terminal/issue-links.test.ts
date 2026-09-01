import { describe, expect, it } from 'vitest'
import { findIssueKeyMatches, knownProjectKeys } from './issue-links'

const MIRROR = ['GDK', 'NMB', 'OPS']

describe('findIssueKeyMatches (GDK-1160)', () => {
  it('finds a key whose project the mirror covers', () => {
    expect(findIssueKeyMatches('fix(term): GDK-1160 link provider', MIRROR)).toEqual([
      { key: 'GDK-1160', start: 11, end: 19 },
    ])
  })

  it('leaves a key-shaped string alone when no such project exists — this is the whole feature', () => {
    expect(findIssueKeyMatches('warning: ABC-123 is deprecated, see RFC-2119', MIRROR)).toEqual([])
  })

  it('offers nothing at all when the mirror covers no projects', () => {
    expect(findIssueKeyMatches('GDK-1160 landed', [])).toEqual([])
  })

  it('finds every key on a line, in order', () => {
    const line = 'GDK-1 depends on NMB-42 and on ABC-7'
    expect(findIssueKeyMatches(line, MIRROR).map((m) => m.key)).toEqual(['GDK-1', 'NMB-42'])
  })

  it('does not match a key glued to other word characters', () => {
    expect(findIssueKeyMatches('xGDK-1160 and GDK-1160x', MIRROR)).toEqual([])
  })

  it('is case-sensitive: lowercase prose is not a ticket', () => {
    expect(findIssueKeyMatches('the gdk-1160 branch', MIRROR)).toEqual([])
  })

  it('matches inside a URL, a path and parentheses — keys arrive wrapped', () => {
    const line = '(GDK-1160) https://example.invalid/browse/NMB-9 docs/OPS-3.md'
    expect(findIssueKeyMatches(line, MIRROR).map((m) => m.key)).toEqual([
      'GDK-1160',
      'NMB-9',
      'OPS-3',
    ])
  })

  it('reports offsets that slice the key back out of the line', () => {
    const line = 'see GDK-1160 for the rest'
    const [m] = findIssueKeyMatches(line, MIRROR)
    expect(line.slice(m.start, m.end)).toBe('GDK-1160')
  })

  it('does not carry a regex lastIndex between calls', () => {
    const line = 'GDK-1 GDK-2'
    expect(findIssueKeyMatches(line, MIRROR)).toHaveLength(2)
    expect(findIssueKeyMatches(line, MIRROR)).toHaveLength(2)
  })

  it('accepts a Set as well as a list — the caller unions two sources', () => {
    expect(findIssueKeyMatches('GDK-1160', new Set(['GDK'])).map((m) => m.key)).toEqual([
      'GDK-1160',
    ])
  })
})

describe('knownProjectKeys (GDK-1177)', () => {
  it('prefers the configured projects list', () => {
    expect([...knownProjectKeys(['NMB'], [{ issue_key: 'STD-1' }])]).toEqual(['NMB'])
  })

  it('falls back to the pool key prefixes when projects is unset (localOrigin)', () => {
    const keys = knownProjectKeys(undefined, [
      { issue_key: 'STD-1' },
      { issue_key: 'STD-2' },
      { issue_key: 'OPS-9' },
    ])
    expect([...keys].sort()).toEqual(['OPS', 'STD'])
    expect(findIssueKeyMatches('see STD-2 now', keys)).toHaveLength(1)
  })

  it('ignores a malformed key', () => {
    expect([...knownProjectKeys([], [{ issue_key: '-3' }, { issue_key: 'X' }])]).toEqual([])
  })
})
