import { describe, expect, test } from 'vitest'
import {
  calendarDay,
  explain,
  formatAbs,
  inRange,
  startOfWeekMonday,
  utcZone,
  zoneNamed,
} from './calendar'

const seoul = zoneNamed('Asia/Seoul')
const la = zoneNamed('America/Los_Angeles')

describe('calendar owner (aligned with internal/calendar)', () => {
  test('instant 01:00 KST is calendar day 18 in Seoul, 17 in UTC', () => {
    expect(calendarDay('2026-08-17T16:00:00.000Z', 'instant', seoul)).toBe('2026-08-18')
    expect(calendarDay('2026-08-17T16:00:00.000Z', 'instant', utcZone())).toBe('2026-08-17')
  })

  test('created_from=2026-08-18 includes the KST 01:00 instant only in Seoul', () => {
    expect(inRange('2026-08-17T16:00:00.000Z', 'instant', '2026-08-18', null, seoul)).toBe(true)
    expect(inRange('2026-08-17T16:00:00.000Z', 'instant', '2026-08-18', null, utcZone())).toBe(
      false,
    )
  })

  test('date-only duedate is not shifted in America/Los_Angeles', () => {
    expect(calendarDay('2026-08-20', 'date', la)).toBe('2026-08-20')
    expect(calendarDay('2026-08-20', 'instant', la)).toBe('2026-08-20')
    // Date.parse("2026-08-20") is UTC midnight → 19th in LA. Owner must not.
    const parsed = new Date('2026-08-20')
    const laDay = new Intl.DateTimeFormat('en-CA', {
      timeZone: 'America/Los_Angeles',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).format(parsed)
    expect(laDay).toBe('2026-08-19')
  })

  test('empty raw: pass only when there is no lower bound', () => {
    expect(inRange(null, 'instant', '2026-08-18', null, seoul)).toBe(false)
    expect(inRange(null, 'instant', null, null, seoul)).toBe(true)
  })

  test('explain reports the decision', () => {
    expect(explain('2026-08-17T16:00:00.000Z', 'instant', seoul)).toEqual({
      raw: '2026-08-17T16:00:00.000Z',
      kind: 'instant',
      zone: 'Asia/Seoul',
      calendar_day: '2026-08-18',
      ok: true,
    })
    expect(explain('2026-08-20', 'date', la).calendar_day).toBe('2026-08-20')
  })

  test('startOfWeekMonday in KST: Wed 08-19 01:00 → Mon 08-17', () => {
    const now = new Date('2026-08-18T16:00:00.000Z')
    expect(startOfWeekMonday(now, seoul)).toBe('2026-08-17')
  })

  test('formatAbs of a date-only string does not emit a time', () => {
    const shown = formatAbs('2026-08-20', 'date', la, 'en-US')
    expect(shown).toMatch(/08/)
    expect(shown).toMatch(/20/)
    expect(shown).not.toMatch(/:/)
  })
})
