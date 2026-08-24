import { describe, expect, test } from 'vitest'
import {
  GROUP_BY_VALUES,
  SORT_KEY_VALUES,
  defaultGroupBy,
  groupByEnabled,
  parseConfig,
} from './view-config'

/*
 * GDK-825: the grouping and sort allowlists are derived — the `as const`
 * array owns both the union and the URL guard (isGroupBy / isSortKey read
 * the same array). This pins the contract that used to be two hand lists:
 * every axis the registry names must survive the URL parse (modulo its
 * feature flag), so widening the union without the guard can no longer
 * compile-and-drop silently.
 *
 * Feature-gated axes (team_group / product / qa_impact under DEFAULTS's
 * all-off flags) are the documented drop: a URL must not revive a dead
 * filter (parseView's own rule).
 */

describe('group-by allowlist is the URL contract (GDK-825)', () => {
  test('every GROUP_BY_VALUES member parses, or is the by-design feature drop', () => {
    expect(GROUP_BY_VALUES.length).toBeGreaterThan(0)
    for (const g of GROUP_BY_VALUES) {
      const parsed = parseConfig(new URLSearchParams(`g=${g}`)).display.group_by
      if (groupByEnabled(g)) {
        expect(parsed, `g=${g} is registry member and feature-on → must parse`).toBe(g)
      } else {
        expect(parsed, `g=${g} feature-off → must drop to the default, not half-apply`).toBe(
          defaultGroupBy(null),
        )
      }
    }
  })

  test('a value outside the registry never parses', () => {
    expect(parseConfig(new URLSearchParams('g=not-an-axis')).display.group_by).toBe(
      'status_category',
    )
  })
})

describe('sort allowlist is the URL contract (GDK-825)', () => {
  test('every SORT_KEY_VALUES member parses from s=', () => {
    expect(SORT_KEY_VALUES.length).toBeGreaterThan(0)
    for (const s of SORT_KEY_VALUES) {
      expect(parseConfig(new URLSearchParams(`s=${s}`)).display.sort, `s=${s}`).toBe(s)
    }
  })

  test('a value outside the registry never parses', () => {
    expect(parseConfig(new URLSearchParams('s=not-a-sort')).display.sort).toBe('updated')
  })
})
