import { describe, expect, it } from 'vitest'
import { IME_INPUT_ATTRS, imeReduce, type ImeEvent, type ImeState } from './ime'

function play(events: ImeEvent[], init: ImeState = { composing: false }) {
  const emits: string[] = []
  let state = init
  for (const ev of events) {
    const next = imeReduce(state, ev)
    state = next.state
    emits.push(next.emit)
  }
  return { state, emits }
}

describe('imeReduce — sequences', () => {
  it('Korean syllable: start → updates → end emits once, at the end', () => {
    const { state, emits } = play([
      { kind: 'compositionstart' },
      { kind: 'compositionupdate', data: 'ㅎ' },
      { kind: 'compositionupdate', data: '하' },
      { kind: 'compositionupdate', data: '한' },
      { kind: 'compositionend', data: '한' },
    ])
    expect(emits).toEqual(['', '', '', '', '한'])
    expect(emits.filter((e) => e !== '')).toEqual(['한'])
    expect(state.composing).toBe(false)
  })

  it('input while composing (isComposing: true) emits nothing', () => {
    const { state, emits } = play([
      { kind: 'compositionstart' },
      { kind: 'compositionupdate', data: 'ㅎ' },
      { kind: 'input', data: 'ㅎ', isComposing: true },
      { kind: 'compositionupdate', data: '하' },
      { kind: 'input', data: '하', isComposing: true },
      { kind: 'compositionupdate', data: '한' },
      { kind: 'input', data: '한', isComposing: true },
      { kind: 'compositionend', data: '한' },
    ])
    expect(emits.filter((e) => e !== '')).toEqual(['한'])
    expect(state.composing).toBe(false)
  })

  it('input with isComposing false and no composition in progress emits its data', () => {
    const { state, emits } = play([
      { kind: 'input', data: 'l', isComposing: false },
      { kind: 'input', data: 's', isComposing: false },
    ])
    expect(emits).toEqual(['l', 's'])
    expect(state.composing).toBe(false)
  })

  it('compositionend with empty data emits nothing (dismissed candidate)', () => {
    const { state, emits } = play([
      { kind: 'compositionstart' },
      { kind: 'compositionupdate', data: 'ㅎ' },
      { kind: 'compositionend', data: '' },
    ])
    expect(emits).toEqual(['', '', ''])
    expect(state.composing).toBe(false)
  })

  it('compositionend with no preceding start does not throw and does not stick composing', () => {
    expect(() => imeReduce({ composing: false }, { kind: 'compositionend', data: '한' })).not.toThrow()
    const { state, emits } = play([{ kind: 'compositionend', data: '한' }])
    expect(state.composing).toBe(false)
    expect(emits).toEqual(['한'])
  })

  it('two syllables composed back to back emit twice, in order, with nothing in between', () => {
    const { state, emits } = play([
      { kind: 'compositionstart' },
      { kind: 'compositionupdate', data: 'ㅎ' },
      { kind: 'compositionend', data: '한' },
      { kind: 'compositionstart' },
      { kind: 'compositionupdate', data: 'ㄱ' },
      { kind: 'compositionend', data: '글' },
    ])
    expect(emits.filter((e) => e !== '')).toEqual(['한', '글'])
    expect(emits).toEqual(['', '', '한', '', '', '글'])
    expect(state.composing).toBe(false)
  })

  it('does not mutate the input state', () => {
    const state: ImeState = { composing: false }
    imeReduce(state, { kind: 'compositionstart' })
    expect(state).toEqual({ composing: false })
  })
})

describe('IME_INPUT_ATTRS', () => {
  it('disables every rewrite iOS would apply to a shell', () => {
    expect(IME_INPUT_ATTRS.autocapitalize).toBe('off')
    expect(IME_INPUT_ATTRS.autocorrect).toBe('off')
    expect(IME_INPUT_ATTRS.autocomplete).toBe('off')
    expect(IME_INPUT_ATTRS.spellcheck).toBe('false')
  })
})
