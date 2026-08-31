import { describe, expect, it } from 'vitest'
import { SELF_ECHO_MS, movedExternally, selfEchoFresh } from './board-moves'
import type { IssueLite } from './types'

/*
 * The asymmetry is decided here and nowhere else, so this is where it is
 * pinned: a status the pool already held is this tab's own write coming back
 * (the optimistic patch put it there before the delta did), and animating
 * that would replay the user's own gesture at them.
 */

function issue(over: Partial<IssueLite>): IssueLite {
  return { issue_key: 'GDK-1', status_category: 'new', ...over } as IssueLite
}

describe('movedExternally', () => {
  it('is false when the pool already holds the incoming status (own write, confirmed)', () => {
    const prev = issue({ status_id: '3', status_category: 'inprogress' })
    const next = issue({ status_id: '3', status_category: 'inprogress' })
    expect(movedExternally(prev, next)).toBe(false)
  })

  it('is true when the status id changed under us', () => {
    expect(
      movedExternally(issue({ status_id: '1' }), issue({ status_id: '3' })),
    ).toBe(true)
  })

  it('falls back to status_category when ids are missing (older cached rows)', () => {
    expect(
      movedExternally(issue({ status_category: 'new' }), issue({ status_category: 'done' })),
    ).toBe(true)
    expect(
      movedExternally(issue({ status_category: 'new' }), issue({ status_category: 'new' })),
    ).toBe(false)
  })

  it('never keys on the display name — a renamed status in one language is not a move', () => {
    const prev = issue({ status_id: '3', status: 'In Progress', status_category: 'inprogress' })
    const next = issue({ status_id: '3', status: '진행 중', status_category: 'inprogress' })
    expect(movedExternally(prev, next)).toBe(false)
  })

  it('a first sighting is an arrival, not a move (no old position to fly from)', () => {
    expect(movedExternally(undefined, issue({ status_id: '3' }))).toBe(false)
  })

  /*
   * The race selfEchoFresh exists for (GDK-1176): the optimistic patch keeps
   * the *old* status_id (a Transition carries no target id), so an echo delta
   * that beats the origin confirm compares old id → new id and reads this
   * tab's own drop as an outside move. This test is the bug demonstrated;
   * the two below are the guard that swallows it.
   */
  it('an echo landing before the confirm would read as external — the id gap', () => {
    const optimistic = issue({ status_id: '1', status_category: 'indeterminate' })
    const echo = issue({ status_id: '3', status_category: 'indeterminate' })
    expect(movedExternally(optimistic, echo)).toBe(true)
  })
})

describe('selfEchoFresh', () => {
  it('a key this tab just transitioned keeps its echo silent', () => {
    expect(selfEchoFresh(1_000, 1_000 + SELF_ECHO_MS - 1)).toBe(true)
  })

  it('an unflagged key, or one past the window, is nobody’s echo', () => {
    expect(selfEchoFresh(undefined, 1_000)).toBe(false)
    expect(selfEchoFresh(1_000, 1_000 + SELF_ECHO_MS)).toBe(false)
  })
})
