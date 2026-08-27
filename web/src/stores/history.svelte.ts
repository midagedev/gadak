/*
 * Personal history (visits + searches) loaded from GET history/.
 *
 * The pages store owns whether this view holds the main column (`historyView`).
 * This store owns the rows, the kind chip, and the text filter — so leaving
 * the screen and coming back does not drop what was narrowed.
 */

import * as api from '../lib/api'
import type { HistoryKindFilter } from '../lib/history'
import type { HistoryItem } from '../lib/types'

const PAGE_LIMIT = 200

class HistoryStore {
  items = $state<HistoryItem[]>([])
  nextCursor = $state('')
  loading = $state(false)
  loaded = $state(false)
  /**
   * The last load failed. Failure is not emptiness: wiping items on an error
   * made a 503 render the "nothing viewed or searched yet" copy (GDK-1054).
   * On failure the items stay (stale-but-real) and the screen shows the
   * error line instead.
   */
  loadFailed = $state(false)
  /** Server `kind` query. Empty = mixed visits + searches. */
  kind = $state<HistoryKindFilter>('')
  /** Local text narrowing — not a recorded search. */
  filterText = $state('')

  async reload(): Promise<void> {
    this.loading = true
    this.loadFailed = false
    try {
      const page = await api.getHistory({
        kind: this.kind || undefined,
        limit: PAGE_LIMIT,
      })
      this.items = page.items ?? []
      this.nextCursor = page.next_cursor ?? ''
      this.loaded = true
    } catch (e) {
      console.debug('[history] load failed', e)
      this.loadFailed = true
    } finally {
      this.loading = false
    }
  }

  async loadMore(): Promise<void> {
    if (!this.nextCursor || this.loading) return
    this.loading = true
    try {
      const page = await api.getHistory({
        kind: this.kind || undefined,
        limit: PAGE_LIMIT,
        cursor: this.nextCursor,
      })
      this.items = [...this.items, ...(page.items ?? [])]
      this.nextCursor = page.next_cursor ?? ''
    } catch (e) {
      console.debug('[history] load more failed', e)
    } finally {
      this.loading = false
    }
  }

  setKind(kind: HistoryKindFilter): void {
    if (this.kind === kind) return
    this.kind = kind
    void this.reload()
  }

  /** Prepend a just-recorded event so an open view does not wait for reload. */
  noteItem(item: HistoryItem): void {
    if (!this.loaded) return
    if (this.kind === 'search' && item.type !== 'search') return
    if (
      (this.kind === 'issue' || this.kind === 'page') &&
      (item.type !== 'visit' || item.kind !== this.kind)
    ) {
      return
    }
    const idx = this.items.findIndex((row) => row.type === item.type && row.id === item.id)
    if (idx >= 0) {
      const next = this.items.slice()
      next[idx] = item
      this.items = next
      return
    }
    this.items = [item, ...this.items]
  }

  /** HistoryView mounts call this name. */
  load(): Promise<void> {
    return this.reload()
  }
}

export const history = new HistoryStore()
