/*
 * GDK-1560 recurrence gate, second half. Bare interpolations
 * ({v.count}) are caught by lib/counts-formatted.test.ts; this covers the
 * other way a count reaches the screen — as a t() parameter.
 *
 * There were eighteen `t(key, { n: someCount })` call sites and each one
 * would have needed its own formatNumber() wrapper, which is the arrangement
 * that produced the bug in the first place: one surface is always missed.
 * So the substitution itself owns the formatting. A number handed to t() is
 * a number a human reads, and it gets the locale's digit grouping; a string
 * is passed through untouched, which is the escape hatch for the identity
 * numbers (an exit code, a page revision) that must not be grouped.
 *
 * FAIL-first at authoring time, before t() formatted its params:
 *   expected '1195 issues' to be '1,195 issues'
 */
import { describe, expect, test } from 'vitest'

import { t } from './index'

describe('GDK-1560 t() groups numeric params', () => {
  test('a number param is grouped', () => {
    expect(t('sidebar.issueCount', { n: 1195 })).toContain('1,195')
    expect(t('sidebar.issueCount', { n: 10184 })).toContain('10,184')
  })

  test('a small number is unchanged', () => {
    expect(t('sidebar.issueCount', { n: 999 })).toContain('999')
  })

  // The escape hatch, so an identity number can opt out at the call site
  // (DocumentPanel passes String(head.version) for exactly this reason).
  test('a string param is passed through verbatim', () => {
    expect(t('sidebar.issueCount', { n: '10184' })).toContain('10184')
  })
})
