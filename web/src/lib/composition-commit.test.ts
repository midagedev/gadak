/*
 * GDK-169: mid-composition states are never committed as queries.
 *
 * Recorded event order from typing 딥링크 (2026-08-17). The helper is the
 * single owner SearchBox and CommandPalette share; these cases are the
 * recurrence gate for that owner.
 */
import { describe, expect, test } from 'vitest'
import { createCompositionCommit } from './composition-commit'

const JAMO = ['디', '딥', '딥ㄹ', '딥리', '딥링', '딥링ㅋ'] as const
const FINAL = '딥링크'
const COMPOSING = [...JAMO, FINAL] as const

describe('createCompositionCommit (GDK-169)', () => {
  test('딥링크 composition commits only the final string, never mid-jamo states', () => {
    const commits: string[] = []
    const ime = createCompositionCommit((q) => commits.push(q))

    ime.oncompositionstart()
    for (const text of COMPOSING) {
      ime.oninput({ isComposing: true }, text)
    }
    ime.oncompositionend({ isComposing: false, data: FINAL }, FINAL)
    // Chrome: final input after compositionend with isComposing false.
    ime.oninput({ isComposing: false }, FINAL)

    expect(
      commits.filter((c) => (JAMO as readonly string[]).includes(c)),
      `intermediate jamo reached commit: ${JSON.stringify(commits)}`,
    ).toEqual([])
    expect(commits).toContain(FINAL)
    expect(commits.every((c) => c === FINAL)).toBe(true)
  })

  test('Safari: final input after compositionend still isComposing does not drop the commit', () => {
    const commits: string[] = []
    const ime = createCompositionCommit((q) => commits.push(q))

    ime.oncompositionstart()
    for (const text of COMPOSING) {
      ime.oninput({ isComposing: true }, text)
    }
    ime.oncompositionend({ isComposing: false, data: FINAL }, FINAL)
    ime.oninput({ isComposing: true }, FINAL)

    expect(commits).toEqual([FINAL])
  })

  test('plain English keystrokes (no composition events) commit every character', () => {
    const commits: string[] = []
    const ime = createCompositionCommit((q) => commits.push(q))
    for (const text of ['d', 'de', 'dee', 'deep']) {
      ime.oninput({ isComposing: false }, text)
    }
    expect(commits).toEqual(['d', 'de', 'dee', 'deep'])
  })

  test('deleting after a composition still commits', () => {
    const commits: string[] = []
    const ime = createCompositionCommit((q) => commits.push(q))
    ime.oncompositionstart()
    ime.oninput({ isComposing: true }, FINAL)
    ime.oncompositionend({ data: FINAL }, FINAL)
    ime.oninput({ isComposing: false }, '딥링')
    ime.oninput({ isComposing: false }, '')
    expect(commits).toEqual([FINAL, '딥링', ''])
  })

  test('composing is true only between start and end (Enter/Tab callers)', () => {
    const ime = createCompositionCommit(() => {})
    expect(ime.composing).toBe(false)
    ime.oncompositionstart()
    expect(ime.composing).toBe(true)
    ime.oninput({ isComposing: true }, '디')
    expect(ime.composing).toBe(true)
    ime.oncompositionend({}, '디')
    expect(ime.composing).toBe(false)
  })
})
