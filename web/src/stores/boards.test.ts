/*
 * GDK-1689: the sprint axis is shown by `boards.type`, not by "some sprint
 * exists somewhere".
 *
 * The old rule was `sprints.any` — one sprint row anywhere in the workspace
 * turned the control on for everybody. On a mixed site that is nearly always
 * true (measured on a company Jira: kanban 10 · scrum 18 · simple 4 in one
 * site), so a kanban team got a scope control about a cadence it does not run.
 *
 * FAIL-first: against `sprints.any` every row below returned true, including
 * the two that are the whole point — a kanban-scoped view on a site that also
 * has scrum boards.
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { boards } from './boards.svelte'
import type { BoardRow } from '../lib/types'

const row = (o: Partial<BoardRow> & { id: number }): BoardRow => ({
  name: `board ${o.id}`,
  type: 'scrum',
  has_sprints: true,
  ...o,
})

// The mixed site the issue measured, in miniature.
const MIXED: BoardRow[] = [
  row({ id: 1, type: 'scrum', project_key: 'SCR', has_sprints: true }),
  row({ id: 2, type: 'kanban', project_key: 'KAN', has_sprints: false }),
  row({ id: 3, type: 'simple', project_key: 'SIM', has_sprints: false }),
]

describe('boards.showsSprintAxis (GDK-1689)', () => {
  beforeEach(() => {
    boards.reset()
  })

  const load = (rows: BoardRow[]) => {
    boards.rows = rows
    boards.loaded = true
  }

  it('a scrum-scoped view shows the axis', () => {
    load(MIXED)
    expect(boards.showsSprintAxis(['SCR'], true)).toBe(true)
  })

  it('a kanban-scoped view does not, even though the site has sprints', () => {
    load(MIXED)
    expect(boards.showsSprintAxis(['KAN'], true)).toBe(false)
  })

  it('a simple-board view does not either', () => {
    load(MIXED)
    expect(boards.showsSprintAxis(['SIM'], true)).toBe(false)
  })

  it('a scope that includes one scrum project among kanban ones shows it', () => {
    load(MIXED)
    expect(boards.showsSprintAxis(['KAN', 'SCR'], true)).toBe(true)
  })

  it('an unscoped cross-project view falls back to "any sprint at all"', () => {
    load(MIXED)
    expect(boards.showsSprintAxis([], true)).toBe(true)
    expect(boards.showsSprintAxis([], false)).toBe(false)
  })

  it('falls back while the boards list has not loaded', () => {
    expect(boards.showsSprintAxis(['KAN'], true)).toBe(true)
  })

  it('a sprint board with no project key cannot rule a scope out', () => {
    // The built-in tracker and some Linear shapes leave project_key empty;
    // an unattributable board is not evidence against this team.
    load([row({ id: 1, type: 'scrum', has_sprints: true })])
    expect(boards.showsSprintAxis(['KAN'], true)).toBe(true)
  })

  it('a workspace whose only sprint board is another team keeps it hidden', () => {
    load([row({ id: 1, type: 'scrum', project_key: 'OTHER', has_sprints: true })])
    expect(boards.showsSprintAxis(['MINE'], true)).toBe(false)
  })

  it('sprintBoards is the report’s own predicate, not the type name', () => {
    // A scrum board whose sprints were never dated cannot answer a sprint cut,
    // so it is not one of the boards the axis is about.
    load([row({ id: 1, type: 'scrum', project_key: 'SCR', has_sprints: false })])
    expect(boards.sprintBoards).toHaveLength(0)
    expect(boards.showsSprintAxis(['SCR'], true)).toBe(false)
  })
})
