import { describe, expect, it } from 'vitest'
import { decideBoundary, decideInput, isHangulText } from './hangul-hold'

/** One `input` event as the DOM presents it: what was inserted, and what the
 *  field holds afterwards. Replaying the pair is the whole contract — the
 *  screen never reads anything else. */
type Step = { data: string; value: string }

function play(steps: Step[]): { sent: string[]; last: string } {
  const sent: string[] = []
  let field = ''
  for (const step of steps) {
    field = step.value
    const out = decideInput(step.data, field)
    if (!out.hold) {
      sent.push(out.send)
      field = ''
    }
  }
  return { sent, last: field }
}

describe('isHangulText', () => {
  it('accepts jamo and syllables', () => {
    expect(isHangulText('ㅎ')).toBe(true) // U+314E, the byte run in the trace
    expect(isHangulText('한')).toBe(true)
    expect(isHangulText('한글')).toBe(true)
  })

  it('rejects empty, ASCII, and anything mixed', () => {
    expect(isHangulText('')).toBe(false)
    expect(isHangulText(' ')).toBe(false)
    expect(isHangulText('a')).toBe(false)
    expect(isHangulText('한a')).toBe(false)
    expect(isHangulText('한 ')).toBe(false)
  })
})

describe('the field is a buffer (GDK-1988)', () => {
  it('iOS assembling 한글 in the field sends nothing until the space', () => {
    const { sent, last } = play([
      { data: 'ㅎ', value: 'ㅎ' },
      { data: '하', value: '하' },
      { data: '한', value: '한' },
      { data: 'ㄱ', value: '한ㄱ' },
      { data: '그', value: '한그' },
      { data: '글', value: '한글' },
      { data: ' ', value: '한글 ' },
    ])
    expect(sent).toEqual(['한글 '])
    expect(last).toBe('')
  })

  it('the boundary sends the run, not the character that ended it', () => {
    // Sending `data` here would hand the PTY a bare space and lose 한글.
    const out = decideInput(' ', '한글 ')
    expect(out).toEqual({ hold: false, send: '한글 ' })
  })

  it('resyllabification stays local: 한 becoming 하나 never half-escapes', () => {
    const { sent } = play([
      { data: 'ㅎ', value: 'ㅎ' },
      { data: '하', value: '하' },
      { data: '한', value: '한' },
      { data: '하나', value: '하나' }, // 한 + ㅏ rewrites a syllable already typed
      { data: '!', value: '하나!' },
    ])
    expect(sent).toEqual(['하나!'])
  })

  it('a rewrite that inserts nothing is not a boundary', () => {
    // deleteContentBackward, and the delete half of an iOS replacement.
    expect(decideInput('', '하')).toEqual({ hold: true, send: '' })
    expect(decideInput('', '')).toEqual({ hold: true, send: '' })
  })

  it('ASCII with no run held behaves exactly as it did before', () => {
    const { sent, last } = play([
      { data: 'l', value: 'l' },
      { data: 's', value: 's' },
    ])
    expect(sent).toEqual(['l', 's'])
    expect(last).toBe('')
  })

  it('the recorded device trace sends nothing at all', () => {
    // The six jamo of 한글 as iOS 18.7 actually delivered them, with the
    // field cleared after each one — the shape HEAD forwarded to the PTY as
    // ㅎㅏㄴㄱㅡㄹ. No reducer can assemble 한글 out of this (that would be a
    // Hangul automaton, which is iOS's job, not ours); the honest contract
    // is that not one of them reaches the shell on its own.
    const { sent } = play([
      { data: 'ㅎ', value: 'ㅎ' },
      { data: 'ㅏ', value: 'ㅏ' },
      { data: 'ㄴ', value: 'ㄴ' },
      { data: 'ㄱ', value: 'ㄱ' },
      { data: 'ㅡ', value: 'ㅡ' },
      { data: 'ㄹ', value: 'ㄹ' },
    ])
    expect(sent).toEqual([])
  })
})

describe('boundaries other than a character', () => {
  it('Enter, Tab and the key bar send the held run whole', () => {
    expect(decideBoundary('한글')).toEqual({ hold: false, send: '한글' })
  })

  it('an empty field is not a boundary emission', () => {
    expect(decideBoundary('')).toEqual({ hold: true, send: '' })
  })
})
