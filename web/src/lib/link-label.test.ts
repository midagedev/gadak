import { describe, expect, it } from 'vitest'
import { linkLabel } from './link-label'

const types = [{ name: 'Blocks', inward: 'is blocked by', outward: 'blocks' }]

describe('linkLabel (GDK-1215)', () => {
  it('prefers the phrase the backend rendered', () => {
    expect(linkLabel({ type: 'Blocks', direction: 'inward', phrase: 'is blocked by' }, [], '—')).toBe(
      'is blocked by',
    )
  })

  it('falls back to the local catalog for each side', () => {
    expect(linkLabel({ type: 'Blocks', direction: 'inward' }, types, '—')).toBe('is blocked by')
    expect(linkLabel({ type: 'Blocks', direction: 'outward' }, types, '—')).toBe('blocks')
  })

  it('never renders the raw direction token', () => {
    // No catalog row: the type name, not "outward".
    expect(linkLabel({ type: 'Relates', direction: 'outward' }, types, '—')).toBe('Relates')
    // A direction spelling neither named case matches — the leak that shipped.
    for (const direction of ['OUTWARD', 'both', 'inward ', '']) {
      const got = linkLabel({ type: 'Duplicates', direction }, types, '—')
      expect(got).not.toBe(direction)
      expect(got).toBe('Duplicates')
    }
  })

  it('falls back to the caller word when there is no type either', () => {
    expect(linkLabel({ direction: 'outward' }, types, 'Linked')).toBe('Linked')
  })
})
