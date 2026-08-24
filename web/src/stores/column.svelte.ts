/*
 * The main column — what it is showing, as one value (GDK-821).
 *
 * Same move the right panel made (stores/panel): the column holds one thing
 * at a time — the issue list, a document screen, a space, history, a
 * dashboard, or the feed — and that used to be an agreement restated at every
 * entry point. pages (docsView/spaceView/historyView), me (feedOpen) and
 * dashboards (openId) each held a latch and closed the others on the way in,
 * and a surface that forgot one painted itself behind whatever was up
 * (GDK-815: the list behind a dashboard). Here exclusivity is the shape of
 * the state: showing one view is what releases the other, and there is no
 * order in which two can hold the column.
 *
 * The stores keep their public fields (`pages.docsView`, `me.feedOpen`,
 * `dashboards.openId`) as views onto this value, so readers — keymap, sidebar
 * tint, URL sync — did not have to learn a new call.
 *
 * Only "what owns the column" lives here. Everything else those stores own
 * (the page index, feed items, the dashboard row) stays with them, and the
 * per-kind resets that were never teardown (docs keeps its label narrowing
 * when a space takes over — that is carryover, not leakage) stay in the
 * owning store's methods, not in onLeave.
 */

import type { FeedFocus } from '../lib/types'

/** Everything the main column can show. `list` is the resting state. */
export type ColumnView =
  | { view: 'list' }
  | { view: 'docs' }
  | { view: 'space'; key: string }
  | { view: 'history' }
  | { view: 'dashboard'; id: string }
  | { view: 'feed'; focus: FeedFocus }

/** The discriminant alone — what `is()`/`close()` key on. */
export type ColumnKind = ColumnView['view']

/** Same view, same payload: a re-show that changes nothing is a no-op. */
function sameView(a: ColumnView, b: ColumnView): boolean {
  if (a.view !== b.view) return false
  if (a.view === 'space' && b.view === 'space') return a.key === b.key
  if (a.view === 'dashboard' && b.view === 'dashboard') return a.id === b.id
  if (a.view === 'feed' && b.view === 'feed') return a.focus === b.focus
  return true
}

class ColumnStore {
  #view = $state<ColumnView>({ view: 'list' })

  /**
   * Per-kind teardown, registered by the store that owns that kind — only
   * state kept *for the thing currently shown* (dashboards' loaded row).
   * Registration runs from the owning store's constructor, which is what
   * keeps this file runtime-import-free and store cycles from growing back.
   */
  #onLeave = new Map<ColumnKind, () => void>()

  /** What the column is showing. */
  get view(): ColumnView {
    return this.#view
  }

  /** Is one kind holding the column. */
  is(kind: ColumnKind): boolean {
    return this.#view.view === kind
  }

  /** The open key/id for space/dashboard — null when something else, or the
   *  list, is up. What `pages.spaceView` / `dashboards.openId` read. */
  keyOf(kind: 'space' | 'dashboard'): string | null {
    const v = this.#view
    if (kind === 'space' && v.view === 'space') return v.key
    if (kind === 'dashboard' && v.view === 'dashboard') return v.id
    return null
  }

  onLeave(kind: ColumnKind, fn: () => void): void {
    this.#onLeave.set(kind, fn)
  }

  /** Show one thing. Whatever held the column is released — one value holds
   *  it. A same-view re-show (other space/doc/feed payloads included) changes
   *  nothing; swapping payloads within a kind runs no teardown. */
  show(next: ColumnView): void {
    const prev = this.#view
    if (sameView(prev, next)) return
    this.#view = next
    if (prev.view !== next.view) this.#onLeave.get(prev.view)?.()
  }

  /** Release, but only if this kind is what holds the column. A closer asks
   *  about its own surface; by the time the click lands, another may have
   *  taken it. Releasing lands on the list — the column's resting state. */
  close(kind: ColumnKind): void {
    if (this.#view.view !== kind) return
    this.show({ view: 'list' })
  }
}

export const column = new ColumnStore()
