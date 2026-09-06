import { describe, expect, test } from 'vitest'
import { startOfWeekMonday } from './calendar'
import { builtinViews } from './builtin-views'
import { configToParams } from './view-config'

describe('builtinViews (moved from e2e/dates.spec.ts)', () => {
  test('resolved-week serialises to rf= (resolved_from), not uf=', () => {
    const view = builtinViews().find((v) => v.id === 'resolved-week')
    expect(view, 'resolved-week builtin must exist').toBeTruthy()
    const monday = startOfWeekMonday()
    expect(view!.config.filters.status_category).toEqual(['done'])
    expect(view!.config.filters.resolved_from).toBe(monday)
    expect(view!.config.filters.updated_from).toBeNull()
    const params = configToParams(view!.config)
    expect(params.rf).toBe(monday)
    expect(params.rf).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    expect(params.uf).toBeNull()
    expect(params.sc).toBe('done')
  })
})

/*
 * team-flow pack: the "Aging in progress" built-in view.
 *
 * Contract ↔ assertion table (spec C1–C6; the rows this file can see):
 *  C2/C4 tenant-neutral filters — only status_category, every other axis the
 *      empty default: status/priority/issue_type/jira_project empty, no flags,
 *      no text query. [FAIL-first 2026-09-06: find(id) returned undefined →
 *      "aging-in-progress builtin must exist" failed before the entry existed]
 *  C4  existing sort key only, no new ViewDisplay axis: sort 'updated',
 *      dir 'asc' — longest-waiting first. ViewDisplay has no status_changed_at
 *      sort (SORT_KEY_VALUES), so updated-at ascending is the documented
 *      proxy (the task spec says so; status_changed_at itself feeds the
 *      stale flag). URL form: s omitted (updated is the default key), d=asc.
 *  C5  name/hint resolve to non-empty copy in the default locale (en);
 *      en/ko/ja parity is catalog.test.ts's gate, exercised by the i18n suite.
 */
describe('builtinViews: aging-in-progress (team-flow pack)', () => {
  test('exists, tenant-neutral, oldest-first via the existing updated axis', () => {
    const view = builtinViews().find((v) => v.id === 'aging-in-progress')
    expect(view, 'aging-in-progress builtin must exist').toBeTruthy()

    const f = view!.config.filters
    expect(f.status_category).toEqual(['inprogress'])
    // Tenant neutrality: no site-specific axis may ride along.
    expect(f.status).toEqual([])
    expect(f.priority).toEqual([])
    expect(f.issue_type).toEqual([])
    expect(f.jira_project).toEqual([])
    expect(f.labels).toEqual([])
    expect(f.reopened).toBe(false)
    expect(f.unassigned).toBe(false)
    expect(f.stale).toBe(false)
    expect(f.q).toBe('')

    // G4: the arrangement is the coaching — oldest updates first.
    expect(view!.config.display.sort).toBe('updated')
    expect(view!.config.display.dir).toBe('asc')

    // URL form: sc=inprogress, d=asc, sort key omitted (it is the default).
    const params = configToParams(view!.config)
    expect(params.sc).toBe('inprogress')
    expect(params.d).toBe('asc')
    expect(params.s).toBeNull()

    // Copy keys resolve (missing keys render as the key itself).
    expect(view!.name.length).toBeGreaterThan(0)
    expect(view!.name).not.toBe('view.agingInProgress.name')
    expect(view!.hint!.length).toBeGreaterThan(0)
  })
})
