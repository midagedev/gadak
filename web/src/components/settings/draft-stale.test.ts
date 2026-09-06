/*
 * The stale threshold's half of the settings form model. An unset threshold
 * (0/absent in the document) means "learned from the workspace" — the
 * server sends the p85 cycle time as bootstrap `flow` only while the
 * setting is unset (internal/server/read.go flowFields). The form must
 * therefore round-trip "unset" as unset: show it empty, save it as 0.
 *
 * FAIL-first: before this change toDraft rendered an unset threshold as
 * "0" and toSettings wrote 72 for any empty or non-positive text, so the
 * first save of any unrelated setting pinned 72 into config.json and
 * switched the learning off (observed on a real workspace, 2026-09-06).
 */
import { describe, expect, test } from 'vitest'
import { toDraft, toSettings } from './draft'

describe('stale threshold draft', () => {
  test('GET → form: unset and 0 show empty, a set value shows as typed', () => {
    expect(toDraft({}).staleText).toBe('')
    expect(toDraft({ staleThresholdHours: 0 }).staleText).toBe('')
    expect(toDraft({ staleThresholdHours: 48 }).staleText).toBe('48')
  })

  test('form → PUT: empty and junk save as 0 (learn), a number saves verbatim', () => {
    const d = toDraft({})
    expect(toSettings(d, false).staleThresholdHours).toBe(0)
    d.staleText = '   '
    expect(toSettings(d, false).staleThresholdHours).toBe(0)
    d.staleText = 'abc'
    expect(toSettings(d, false).staleThresholdHours).toBe(0)
    d.staleText = '-5'
    expect(toSettings(d, false).staleThresholdHours).toBe(0)
    d.staleText = '48'
    expect(toSettings(d, false).staleThresholdHours).toBe(48)
  })

  test('round-trip: a stored 72 stays an explicit 72, an unset stays unset', () => {
    expect(toSettings(toDraft({ staleThresholdHours: 72 }), false).staleThresholdHours).toBe(72)
    expect(toSettings(toDraft({}), false).staleThresholdHours).toBe(0)
  })
})
