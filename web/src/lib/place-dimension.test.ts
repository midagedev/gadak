/*
 * GDK-880: which params own the column vs the right panel, and which of
 * the three `open` rules a hash fires. The table is exhaustive over
 * PLACE_PARAM_KEYS; the open classifier is what DashboardView consults;
 * boot restore is pinned by reading App.svelte's source so a new key
 * cannot be classified here and forgotten there.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { parseHash } from './hash'
import {
  COLUMN_PARAM_KEYS,
  OTHER_PARAM_KEYS,
  PANEL_PARAM_KEYS,
  classifyOpenParams,
  mergePanelParams,
  placeDimension,
  resolveOpen,
} from './place-dimension'

const HERE = dirname(fileURLToPath(import.meta.url))
const APP = join(HERE, '..', 'App.svelte')
const DASH = join(HERE, '..', 'components', 'dashboard', 'DashboardView.svelte')

function params(hash: string): URLSearchParams {
  return parseHash(hash).params
}

function entries(sp: URLSearchParams): Record<string, string> {
  return Object.fromEntries(sp)
}

describe('PLACE_DIMENSION', () => {
  test('view params and unknown keys are other, not a silent column/panel', () => {
    expect(placeDimension('sc')).toBe('other')
    expect(placeDimension('q')).toBe('other')
    expect(placeDimension('g')).toBe('other')
    expect(placeDimension('nope')).toBe('other')
    expect(placeDimension('f.story_points')).toBe('other')
  })
})

describe('classifyOpenParams — three rules', () => {
  test('rule 1: a column param takes the column', () => {
    expect(classifyOpenParams(params('#/?docs=1'))).toBe('column')
    expect(classifyOpenParams(params('#/?dash=abc'))).toBe('column')
    expect(classifyOpenParams(params('#/?hist=1'))).toBe('column')
    expect(classifyOpenParams(params('#/?space=LOC'))).toBe('column')
    expect(classifyOpenParams(params('#/?feed=all'))).toBe('column')
    // Column wins over a panel param on the same hash.
    expect(classifyOpenParams(params('#/?docs=1&issue=K'))).toBe('column')
  })

  test('rule 2: only panel params keep the column', () => {
    expect(classifyOpenParams(params('#/?issue=NMS-134'))).toBe('panel')
    expect(classifyOpenParams(params('#/?doc=123'))).toBe('panel')
    expect(classifyOpenParams(params('#/?person=x'))).toBe('panel')
    expect(classifyOpenParams(params('#/?issue=K&doc=1'))).toBe('panel')
  })

  test('rule 3: filters, unknown, empty, and mixed go to the list', () => {
    expect(classifyOpenParams(params('#/?sc=new'))).toBe('list')
    expect(classifyOpenParams(params('#/?q=foo'))).toBe('list')
    expect(classifyOpenParams(params('#/?g=status'))).toBe('list')
    expect(classifyOpenParams(params('#/?issue=K&sc=new'))).toBe('list')
    expect(classifyOpenParams(params('#/'))).toBe('list')
    expect(classifyOpenParams(params('#/?nope=1'))).toBe('list')
    expect(classifyOpenParams(params('#/?settings=sync'))).toBe('list')
    expect(classifyOpenParams(params('#/?dview=tree'))).toBe('list')
  })
})

describe('resolveOpen', () => {
  test('panel-only merge keeps dash and replaces the panel identity', () => {
    const { rule, params: next } = resolveOpen(
      params('#/?dash=wall-1'),
      params('#/?issue=NMS-134'),
    )
    expect(rule).toBe('panel')
    expect(entries(next)).toEqual({ dash: 'wall-1', issue: 'NMS-134' })
  })

  test('panel-only merge clears a previous panel kind', () => {
    const { params: next } = resolveOpen(
      params('#/?dash=wall-1&issue=OLD'),
      params('#/?doc=42'),
    )
    expect(entries(next)).toEqual({ dash: 'wall-1', doc: '42' })
  })

  test('filter open replaces the hash wholesale (dash drops)', () => {
    const { rule, params: next } = resolveOpen(params('#/?dash=wall-1'), params('#/?sc=new'))
    expect(rule).toBe('list')
    expect(entries(next)).toEqual({ sc: 'new' })
  })

  test('mixed panel+filter is list, not a kept wall', () => {
    const { rule, params: next } = resolveOpen(
      params('#/?dash=wall-1'),
      params('#/?issue=K&sc=new'),
    )
    expect(rule).toBe('list')
    expect(entries(next)).toEqual({ issue: 'K', sc: 'new' })
  })

  test('column open replaces the hash wholesale', () => {
    const { rule, params: next } = resolveOpen(params('#/?dash=wall-1'), params('#/?docs=1'))
    expect(rule).toBe('column')
    expect(entries(next)).toEqual({ docs: '1' })
  })

  test('mergePanelParams is the panel half of resolveOpen', () => {
    const merged = mergePanelParams(params('#/?dash=x&person=a'), params('#/?issue=K'))
    expect(entries(merged)).toEqual({ dash: 'x', issue: 'K' })
  })
})

describe('open handler and boot both read the classification', () => {
  test('DashboardView classifies through resolveOpen and records the rule', () => {
    const src = readFileSync(DASH, 'utf8')
    expect(src, 'open handler must import the classification').toMatch(/place-dimension/)
    expect(src, 'open handler must call resolveOpen').toMatch(/\bresolveOpen\b/)
    // GDK-931/GDK-1146 (2026-08-29): the rule used to be written straight
    // onto dataset.lastDashOpen. Test-observability attributes now go
    // through the single writer in lib/debug-attrs (off in prod unless
    // opted in). Same contract — the fired rule is still named on <html> —
    // re-pinned to the owner's call form, so a regression to a bare
    // dataset write goes red here and in debug-attrs.test.ts.
    expect(src, 'open decision must be inspectable on <html>').toMatch(
      /publishDebugAttr\('lastDashOpen'/,
    )
  })

  test('boot restore reads every place param through its dimension map', () => {
    const src = readFileSync(APP, 'utf8')
    expect(src, 'boot must import the classification').toMatch(/place-dimension/)
    for (const key of COLUMN_PARAM_KEYS) {
      expect(src, `boot must read COLUMN_PARAM.${key}`).toContain(`COLUMN_PARAM.${key}`)
    }
    for (const key of PANEL_PARAM_KEYS) {
      expect(src, `boot must read PANEL_PARAM.${key}`).toContain(`PANEL_PARAM.${key}`)
    }
    for (const key of OTHER_PARAM_KEYS) {
      expect(src, `boot must read OTHER_PARAM.${key}`).toContain(`OTHER_PARAM.${key}`)
    }
  })
})
