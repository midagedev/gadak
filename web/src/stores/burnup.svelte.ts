/*
 * The burn-up document for the sprint the strip is about (GDK-1710/1752) —
 * GET /api/v1/issues/sprints/{id}/burnup/, the document `gadak sprint show
 * --json` prints. One sprint at a time: the strip only ever names one, so
 * the store keeps one document and the id it answers for. An answer that
 * arrives for an id the caller has since moved away from is dropped rather
 * than drawn under the wrong sprint's name.
 *
 * A store rather than component state so the fetch is a method call from
 * the strip's $effect, not an assignment to $state inside it (GDK-692's
 * gate: an effect that writes state is a synchronization where a derivation
 * would do; here the write is the network's answer, and it lives with the
 * request).
 */
import { getBurnup } from '../lib/api'
import type { BurnupResponse } from '../lib/types'

class BurnupStore {
  doc = $state<BurnupResponse | null>(null)
  private forId: number | null = null

  /** Fetch for `id`; clears the previous document first so a stale chart
   *  never sits under a new sprint's name. A failed fetch leaves it absent —
   *  the strip reads without the chart. */
  load(id: number): void {
    this.forId = id
    this.doc = null
    getBurnup(id)
      .then((res) => {
        if (this.forId === id) this.doc = res
      })
      .catch((e) => console.debug('[burnup] fetch failed', e))
  }

  reset(): void {
    this.forId = null
    this.doc = null
  }
}

export const burnup = new BurnupStore()
