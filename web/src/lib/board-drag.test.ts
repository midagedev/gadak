import { describe, expect, it } from 'vitest'
import { dropVerdict, transitionsInto } from './board-drag'
import type { IssueLite, Transition } from './types'

/*
 * The meaning of a drop (GDK-1176), pinned where it is decided. The pointer
 * plumbing in board-drag.svelte.ts only ever asks these two questions; the
 * key trap under test is the double vocabulary — Transition.to_category is
 * Jira's REST key (indeterminate), a column key is the mirror's
 * status_category (inprogress) — folded by effectiveCategory on both sides.
 */

function tr(over: Partial<Transition>): Transition {
  return { id: '1', name: 'T', to_status: 'S', to_category: 'new', ...over } as Transition
}

function issue(cat: string): IssueLite {
  return { issue_key: 'GDK-1', status_category: cat } as IssueLite
}

describe('transitionsInto', () => {
  it('folds Jira indeterminate onto the inprogress column', () => {
    const start = tr({ id: '21', to_category: 'indeterminate' })
    expect(transitionsInto([start], 'inprogress')).toEqual([start])
    expect(transitionsInto([start], 'done')).toEqual([])
  })

  it('keeps every transition that reaches the column — 2+ is the ambiguous drop', () => {
    const a = tr({ id: '31', name: 'Resolve', to_category: 'done' })
    const b = tr({ id: '41', name: "Won't do", to_category: 'done' })
    const c = tr({ id: '21', to_category: 'indeterminate' })
    expect(transitionsInto([a, b, c], 'done')).toEqual([a, b])
  })
})

describe('dropVerdict', () => {
  const list = [tr({ id: '21', to_category: 'indeterminate' })]

  it('the card’s own column is never a target, even via the indeterminate alias', () => {
    // A mirror row carries the raw REST key; the column key is the folded one.
    expect(dropVerdict(issue('indeterminate'), 'inprogress', list)).toBe('illegal')
  })

  it('an unknown list is legal — the preview must not block what the 400 enforces', () => {
    expect(dropVerdict(issue('new'), 'done', null)).toBe('legal')
  })

  it('a known list decides: reachable is legal, unreachable is illegal', () => {
    expect(dropVerdict(issue('new'), 'inprogress', list)).toBe('legal')
    expect(dropVerdict(issue('new'), 'done', list)).toBe('illegal')
  })
})
