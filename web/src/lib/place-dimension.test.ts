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
  COLUMN_PARAM,
  COLUMN_PARAM_KEYS,
  OTHER_PARAM,
  OTHER_PARAM_KEYS,
  PANEL_PARAM,
  PANEL_PARAM_KEYS,
  PLACE_DIMENSION,
  classifyOpenParams,
  mergePanelParams,
  placeDimension,
  resolveOpen,
} from './place-dimension'
import { PLACE_PARAM_KEYS, type PlaceParamKey } from './url-state'

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
  test('every place param maps to the dimension it owns', () => {
    const expected: { readonly [K in PlaceParamKey]: 'column' | 'panel' | 'other' } = {
      dash: 'column',
      docs: 'column',
      hist: 'column',
      space: 'column',
      feed: 'column',
      issue: 'panel',
      doc: 'panel',
      person: 'panel',
      dview: 'other',
      settings: 'other',
    }
    for (const key of PLACE_PARAM_KEYS) {
      expect(PLACE_DIMENSION[key], key).toBe(expected[key])
      expect(placeDimension(key), key).toBe(expected[key])
    }
  })

  test('view params and unknown keys are other, not a silent column/panel', () => {
    expect(placeDimension('sc')).toBe('other')
    expect(placeDimension('q')).toBe('other')
    expect(placeDimension('g')).toBe('other')
    expect(placeDimension('nope')).toBe('other')
    expect(placeDimension('f.story_points')).toBe('other')
  })

  test('identity maps cover exactly the keys of each dimension', () => {
    expect([...COLUMN_PARAM_KEYS].sort()).toEqual([...Object.values(COLUMN_PARAM)].sort())
    expect([...PANEL_PARAM_KEYS].sort()).toEqual([...Object.values(PANEL_PARAM)].sort())
    expect([...OTHER_PARAM_KEYS].sort()).toEqual([...Object.values(OTHER_PARAM)].sort())
    const all = [...COLUMN_PARAM_KEYS, ...PANEL_PARAM_KEYS, ...OTHER_PARAM_KEYS].sort()
    expect(all).toEqual([...PLACE_PARAM_KEYS].sort())
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
    expect(src, 'open decision must be inspectable on <html>').toMatch(
      /dataset\.lastDashOpen/,
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
