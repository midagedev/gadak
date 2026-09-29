import { describe, expect, test } from 'vitest'
import { METRIC_SPECS, SUMMARY_KEYS, deltaOf, formatDelta, formatValue } from './metrics'

/*
 * The retro's delta rule (GDK-1712) — contract ↔ assertion map.
 *
 *  D1 colour only where a direction is agreed        → 'only the four …'
 *  D2 the running bucket is never scored             → 'a partial bucket …'
 *  D3 no previous bucket, or no change, is no chip   → 'nothing to compare …'
 *  D4 the unit ladder is the CLI's                   → 'the step is printed …'
 *  D5 the summary names four metrics that exist      → 'the summary keys …'
 */
describe('retro deltas', () => {
  test('only the four metrics with an agreed direction get a tone', () => {
    const closed = deltaOf(9, 5, 'count', 'up-good', false)
    expect(closed).toEqual({ text: '+4', tone: 'good', glyph: '↑' })
    // More closed is better; fewer is the amber side, never red (GDK-1336).
    expect(deltaOf(2, 5, 'count', 'up-good', false)?.tone).toBe('bad')
    // Cycle time runs the other way.
    expect(deltaOf(2, 5, 'days', 'down-good', false)?.tone).toBe('good')
    expect(deltaOf(9, 5, 'days', 'down-good', false)?.tone).toBe('bad')
    // Sessions and mismatch have no agreed direction: the number moves, the
    // colour does not. A retro that scores its own reading habits is the
    // scoreboard the coaching grammar refuses (THEORY.md G9).
    expect(deltaOf(9, 5, 'count', 'neutral', false)?.tone).toBe('none')
    expect(deltaOf(1, 5, 'count', 'neutral', false)?.tone).toBe('none')
  })

  test('a partial bucket shows its step without being scored', () => {
    const d = deltaOf(2, 5, 'count', 'up-good', true)
    expect(d?.text).toBe('−3')
    expect(d?.tone).toBe('none')
  })

  test('nothing to compare, or nothing changed, is no chip at all', () => {
    expect(deltaOf(5, null, 'count', 'up-good', false)).toBeNull()
    expect(deltaOf(null, 5, 'count', 'up-good', false)).toBeNull()
    expect(deltaOf(5, 5, 'count', 'up-good', false)).toBeNull()
  })

  test('the step is printed on the CLI ladder, with a real minus sign', () => {
    expect(formatDelta(-3, 'count')).toBe('−3')
    expect(formatDelta(-3, 'count').charCodeAt(0)).toBe(0x2212)
    expect(formatDelta(1.25, 'days')).toBe('+1.3d')
    // Under a day the ladder steps down rather than printing 0.0d (GDK-1683).
    expect(formatDelta(0.5, 'days')).toBe('+12.0h')
    expect(formatDelta(0.01, 'days')).toBe('+14m')
    expect(formatDelta(-90, 'seconds')).toBe('−2m')
  })

  test('formatValue keeps the empty cell the CLI prints', () => {
    expect(formatValue(null, 'count')).toBe('—')
    expect(formatValue(undefined, 'days')).toBe('—')
    expect(formatValue(0, 'count')).toBe('0')
  })

  test('the summary keys are metrics the table also has', () => {
    for (const k of SUMMARY_KEYS) {
      expect(METRIC_SPECS.some((m) => m.key === k)).toBe(true)
    }
    expect(SUMMARY_KEYS).toHaveLength(4)
  })
})
