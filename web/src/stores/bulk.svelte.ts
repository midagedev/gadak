/*
 * Issue Navigator — multi-select (bulk action) store
 *
 * Separate from single selection (selection: opens detail): a set of issue keys
 *  chosen for batch status/assignee changes. UI (checkboxes/BulkBar) is the only
 *  reader/writer of this store.
 *
 *  - selected: selected issue_key set (SvelteSet — add/delete trigger reactivity).
 *  - anchorKey: last single-toggle key; pivot for shift-click range select.
 *  - selectRange: select anchor~current row in "current filtered/sorted visible
 *      list order". Visible order is derived from filters.groups (same visual
 *      order the list paints).
 *  - retain: drop keys no longer visible after a view change (caller passes
 *      visible keys).
 *
 * ⚠️ Actual writes (status/assignee) do not live here — BulkBar batches
 *    write store's per-issue optimistic methods.
 */

import { SvelteSet } from 'svelte/reactivity'
import { visibleKeys } from '../lib/visible-order'

class BulkStore {
  /** Selected issue_key set. */
  selected = new SvelteSet<string>()
  /** Last single-toggle key (shift-range anchor). */
  anchorKey = $state<string | null>(null)

  /** Selection size (reactive). */
  count = $derived(this.selected.size)

  /** True when ≥1 key is selected ("selection mode"). */
  get active(): boolean {
    return this.selected.size > 0
  }

  has(key: string): boolean {
    return this.selected.has(key)
  }

  /** Single toggle (deselect if present, else select) + refresh anchor. */
  toggle(key: string): void {
    if (this.selected.has(key)) this.selected.delete(key)
    else this.selected.add(key)
    this.anchorKey = key
  }

  /** Force-select (not toggle) + refresh anchor. */
  add(key: string): void {
    this.selected.add(key)
    this.anchorKey = key
  }

  /**
   * Shift-click range select: add every key from anchor~target in visible order.
   *  Falls back to single add when anchor is missing or either key is off-list.
   *  Keeps the anchor so repeated shift-clicks expand from the same pivot.
   */
  selectRange(targetKey: string): void {
    const order = visibleKeys()
    const ti = order.indexOf(targetKey)
    const ai = this.anchorKey ? order.indexOf(this.anchorKey) : -1
    if (ti === -1 || ai === -1) {
      this.add(targetKey)
      return
    }
    const [lo, hi] = ai <= ti ? [ai, ti] : [ti, ai]
    for (let i = lo; i <= hi; i++) this.selected.add(order[i])
  }

  clear(): void {
    if (this.selected.size) this.selected.clear()
    this.anchorKey = null
  }

  /** Drop selections not in the visible key set (caller on view/filter change). */
  retain(visibleKeys: Iterable<string>): void {
    const keep = visibleKeys instanceof Set ? visibleKeys : new Set(visibleKeys)
    for (const k of this.selected) if (!keep.has(k)) this.selected.delete(k)
    if (this.anchorKey && !keep.has(this.anchorKey)) this.anchorKey = null
  }

  /** Snapshot of selected keys (for batch execution). */
  keys(): string[] {
    return [...this.selected]
  }
}

/** App-wide singleton. */
export const bulk = new BulkStore()
