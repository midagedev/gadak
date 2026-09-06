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
 *  C4  sort is started asc — longest underway first (work item age, the
 *      flow canon's clock: THEORY.md T4). History of this pin: 'updated' was
 *      the proxy; the my-work pack replaced it with 'status_changed'
 *      (FAIL-first 2026-09-06: old code failed, sort was 'updated'); the
 *      second literature round replaced that with 'started' (FAIL-first
 *      2026-09-07: sort was 'status_changed', which reset at every hand-off
 *      inside progress). Each replacement is a new contract, not a relaxation.
 *  C5  name/hint resolve to non-empty copy in the default locale (en);
 *      en/ko/ja parity is catalog.test.ts's gate, exercised by the i18n suite.
 */
describe('builtinViews: aging-in-progress (team-flow pack)', () => {
  test('exists, tenant-neutral, longest-underway first via started', () => {
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
    expect(f.mine).toBe(false)
    expect(f.delegated).toBe(false)
    expect(f.q).toBe('')

    // G4: the arrangement is the coaching — oldest start first.
    expect(view!.config.display.sort).toBe('started')
    expect(view!.config.display.dir).toBe('asc')

    // URL form: sc=inprogress, s=started, d=asc.
    const params = configToParams(view!.config)
    expect(params.sc).toBe('inprogress')
    expect(params.s).toBe('started')
    expect(params.d).toBe('asc')

    // Copy keys resolve (missing keys render as the key itself).
    expect(view!.name.length).toBeGreaterThan(0)
    expect(view!.name).not.toBe('view.agingInProgress.name')
    expect(view!.hint!.length).toBeGreaterThan(0)
  })
})

/*
 * my-work pack: two stances in the built-in views (THEORY.md "Two stances" —
 * contributor first, steward second; G4: the arrangement is the coaching).
 *
 * Contract ↔ assertion table (clause → assertion names):
 *  C3 ten views, exact stance partition in spec order
 *     ten views in the spec order, mine stance first
 *     exactly my-work and delegated need identity
 *  C6 my-work is tenant-neutral — the identity flag + status_category only
 *     my-work: mine flag, open categories, urgent-first priority sort
 *  delegated is the hand-off ledger, quietest first
 *     delegated: delegated flag, quietest first, hand-off glyph
 *
 * FAIL-first 2026-09-06: against the pre-change list every test in this
 * block failed — builtinViews() had eight views, no stance/needsIdentity
 * fields, and find('my-work')/find('delegated') returned undefined.
 */
describe('builtinViews: my-work pack stances', () => {
  test('ten views in the spec order, mine stance first', () => {
    expect(builtinViews().map((v) => [v.id, v.stance])).toEqual([
      ['my-work', 'mine'],
      ['delegated', 'mine'],
      ['all-open', 'mine'],
      ['unassigned-new', 'mine'],
      ['recently-updated', 'mine'],
      ['aging-in-progress', 'team'],
      ['stale', 'team'],
      ['reopened', 'team'],
      ['epic-breakdown', 'team'],
      ['resolved-week', 'team'],
    ])
  })

  test('exactly my-work and delegated need identity', () => {
    const needing = builtinViews().filter((v) => v.needsIdentity)
    expect(needing.map((v) => v.id)).toEqual(['my-work', 'delegated'])
  })

  test('my-work: mine flag, open categories, urgent-first priority sort', () => {
    const view = builtinViews().find((v) => v.id === 'my-work')
    expect(view, 'my-work builtin must exist').toBeTruthy()
    const f = view!.config.filters
    // Tenant neutrality (C6): the only constraints are the mine flag and the
    // open categories — no status/priority/type names, no projects.
    expect(f.mine).toBe(true)
    expect(f.delegated).toBe(false)
    expect(f.status_category).toEqual(['inprogress', 'new'])
    expect(f.status).toEqual([])
    expect(f.priority).toEqual([])
    expect(f.issue_type).toEqual([])
    expect(f.jira_project).toEqual([])
    expect(f.q).toBe('')
    // Urgent first: priority_rank lower = more urgent, so asc is urgent-first.
    expect(view!.config.display.group_by).toBe('status_category')
    expect(view!.config.display.sort).toBe('priority')
    expect(view!.config.display.dir).toBe('asc')
    // URL form: fl=mine, sc=inprogress,new, s=priority, d=asc.
    const params = configToParams(view!.config)
    expect(params.fl).toBe('mine')
    expect(params.sc).toBe('inprogress,new')
    expect(params.s).toBe('priority')
    expect(params.d).toBe('asc')
    // Copy keys resolve (missing keys render as the key itself).
    expect(view!.name.length).toBeGreaterThan(0)
    expect(view!.hint!.length).toBeGreaterThan(0)
  })

  test('delegated: delegated flag, quietest first, hand-off glyph', () => {
    const view = builtinViews().find((v) => v.id === 'delegated')
    expect(view, 'delegated builtin must exist').toBeTruthy()
    const f = view!.config.filters
    expect(f.delegated).toBe(true)
    expect(f.mine).toBe(false)
    expect(f.status_category).toEqual(['inprogress', 'new'])
    expect(f.q).toBe('')
    // The delegation ledger reads by silence: quietest first.
    expect(view!.config.display.sort).toBe('updated')
    expect(view!.config.display.dir).toBe('asc')
    expect(view!.icon).toBe('arrow-up-right')
    const params = configToParams(view!.config)
    expect(params.fl).toBe('delegated')
    expect(params.sc).toBe('inprogress,new')
    expect(params.d).toBe('asc')
    expect(view!.name.length).toBeGreaterThan(0)
    expect(view!.hint!.length).toBeGreaterThan(0)
  })
})
