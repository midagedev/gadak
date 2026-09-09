/*
 * Boards as rows (GDK-1689) — the mirror's `boards` table through
 * GET /api/v1/issues/boards/, the same list the retro's board picker reads.
 *
 * It exists for one question the sprint rows cannot answer: does the team
 * whose issues are on screen use sprints? `sprints.any` says only that some
 * sprint exists somewhere in this workspace, which on a mixed site is nearly
 * always true — measured on a company Jira, one site held kanban 10 · scrum
 * 18 · simple 4, so a kanban team saw a sprint axis about someone else's
 * cadence.
 *
 * The origin already knows. A Jira board carries `type` (scrum · kanban ·
 * simple), a Linear team only gets a board row when it turns cycles on, and
 * the built-in tracker only makes one for a project that has sprints. So the
 * answer is `boards`, not a setting: asking the user what the origin already
 * recorded is the mistake here.
 */
import { getBoards } from '../lib/api'
import type { BoardRow } from '../lib/types'

class BoardsStore {
  rows = $state<BoardRow[]>([])
  loaded = $state(false)
  private inflight: Promise<void> | null = null

  /** The boards that carry dated sprints — the report's own predicate. */
  get sprintBoards(): BoardRow[] {
    return this.rows.filter((b) => b.has_sprints)
  }

  /**
   * Whether a sprint control belongs on screen for a view scoped to these
   * project keys.
   *
   * Scoped to projects: only a sprint-bearing board in one of them counts, so
   * a kanban team's board never shows the axis. Unscoped (a cross-project
   * view like "everything open") there is no team to ask about, so the answer
   * falls back to `hasAny` — which is what shipped before this and is the
   * honest answer for a view that spans teams.
   *
   * Before the boards list has loaded the fallback also applies: a control
   * that blinks in after a round trip is worse than one that was right by
   * luck, and this is the same answer the previous rule gave.
   */
  showsSprintAxis(projectKeys: readonly string[], hasAny: boolean): boolean {
    if (!this.loaded || !projectKeys.length) return hasAny
    const boards = this.sprintBoards
    // A sprint-bearing board with no project key cannot be attributed to a
    // team, so it cannot rule one out either: it keeps the fallback.
    if (boards.some((b) => !b.project_key)) return hasAny
    return boards.some((b) => projectKeys.includes(b.project_key ?? ''))
  }

  load(): Promise<void> {
    if (this.inflight) return this.inflight
    this.inflight = getBoards()
      .then((res) => {
        this.rows = res.boards ?? []
        this.loaded = true
      })
      .catch(() => {
        // An origin with no boards answers an empty list, not an error; a
        // failed request leaves the previous rows and tries again next tick.
      })
      .finally(() => {
        this.inflight = null
      })
    return this.inflight
  }

  reset(): void {
    this.rows = []
    this.loaded = false
  }
}

export const boards = new BoardsStore()
