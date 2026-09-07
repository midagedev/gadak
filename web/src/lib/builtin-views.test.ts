import { describe, expect, test } from 'vitest'
import { builtinViews } from './builtin-views'
import { configToParams } from './view-config'

/*
 * my-work pack: two stances in the built-in views (THEORY.md "Two stances" —
 * contributor first, steward second; G4: the arrangement is the coaching).
 *
 * Contract ↔ assertion table (clause → assertion names):
 *  C3 five views, exact stance partition in spec order (2026-09-07
 *     subtractions: recently-updated / stale / resolved-week deleted,
 *     all-open / unassigned-new moved mine → team; then aging-in-progress /
 *     epic-breakdown deleted — GDK-1493)
 *     five views in the spec order, mine stance first
 *     exactly my-work and delegated need identity
 *  C6 my-work is tenant-neutral — the identity flag + status_category only
 *     my-work: mine flag, open categories, urgent-first priority sort
 *  delegated is the hand-off ledger, quietest first
 *     delegated: delegated flag, quietest first, hand-off glyph
 *
 * FAIL-first 2026-09-06: against the pre-change list every test in this
 * block failed — builtinViews() had eight views, no stance/needsIdentity
 * fields, and find('my-work')/find('delegated') returned undefined.
 * FAIL-first 2026-09-07 (sidebar subtraction): the pre-change list was ten
 * views with a 5+5 partition (all-open, unassigned-new, recently-updated
 * mine; stale, resolved-week team) — the seven-view partition below failed
 * against it on shape, not just counts.
 */
describe('builtinViews: my-work pack stances', () => {
  test('five views in the spec order, mine stance first', () => {
    // FAIL-first 2026-09-07 (GDK-1493): the pre-change list had seven —
    // aging-in-progress and epic-breakdown sat in the team stance.
    expect(builtinViews().map((v) => [v.id, v.stance])).toEqual([
      ['my-work', 'mine'],
      ['delegated', 'mine'],
      ['all-open', 'team'],
      ['unassigned-new', 'team'],
      ['reopened', 'team'],
    ])
  })

  test('nothing site-shaped is a built-in: no view groups by epic or sorts by an age proxy', () => {
    // GDK-1493: a built-in earns its row by meaning the same thing on every
    // site. group_by epic needs a hierarchy the site may not use; the aging
    // view was the in-progress pool under a sort — a saved view's job.
    for (const v of builtinViews()) {
      expect(v.config.display.group_by, v.id).not.toBe('epic')
      expect(v.id).not.toBe('aging-in-progress')
      expect(v.id).not.toBe('epic-breakdown')
    }
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
