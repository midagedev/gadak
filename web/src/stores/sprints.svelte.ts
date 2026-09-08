/*
 * Sprints as rows (GDK-1656) — the mirror's `sprints` table through
 * GET /api/v1/issues/sprints/, the list `gadak sprint list` prints. Two
 * readers: the board's sprint scope (does this workspace have sprints at
 * all; which one is active) and the filter bar's Sprint axis labels.
 *
 * Loaded once at boot and again whenever the mirror advances (the 15s delta
 * poll moves issues.lastSync), the same trigger the dashboards use. Never
 * derived from issue rows: a sprint with no issues exists here and nowhere
 * else, and that is exactly the case the table was made for (GDK-1654).
 */
import { getSprints } from '../lib/api'
import type { SprintRow } from '../lib/types'

class SprintsStore {
  rows = $state<SprintRow[]>([])
  loaded = $state(false)
  private inflight: Promise<void> | null = null

  /** Whether this workspace has any sprint — the board scope's on switch. */
  get any(): boolean {
    return this.rows.length > 0
  }
  /** The active sprints, mirror order. One is the common case; a workspace
   *  with several boards can have several. */
  get active(): SprintRow[] {
    return this.rows.filter((r) => r.state === 'active')
  }

  load(): Promise<void> {
    if (this.inflight) return this.inflight
    this.inflight = getSprints()
      .then((res) => {
        this.rows = res.sprints ?? []
        this.loaded = true
      })
      .catch(() => {
        // An origin without sprints answers an empty list, not an error; a
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

export const sprints = new SprintsStore()
