/*
 * Issue Navigator — keyboard triage state (list context)
 *
 * The list cursor lives here rather than inside IssueList because the triage
 * keys act on it from the shell's global handler (x/s/a/l/c): a component-local
 * cursorKey would be invisible to them, and two handlers racing on window is
 * how j/k and Esc used to fight each other.
 *
 *  - cursorKey: row under the keyboard cursor (highlight + scroll follow).
 *  - listActive: IssueList is mounted. Feed / onboarding / empty states replace
 *      it, and the triage keys must stay inert there.
 *  - keysReady / startupViewApplied: the boot-time first view has committed
 *      (GDK-46). j/k/x arriving earlier are held and replayed against the
 *      committed list; they never land on the unfiltered pool.
 *  - menu: which BulkBar popover is open. BulkBar renders it; mouse and keyboard
 *      both go through this so `s` / `a` / `l` can open it without a click.
 *  - commentKey: issue the quick-comment dialog is composing on (null = closed).
 *
 * ⚠️ No writes here — status/assignee batches stay in BulkBar, comments in the
 *    composer, so the credential gate keeps its single implementation.
 */

import { replayHeldListKeys } from '../lib/keymap.svelte'
import { visibleKeys } from '../lib/visible-order'
import { bulk } from './bulk.svelte'

export type TriageMenu = 'status' | 'assignee' | 'labels'

class TriageStore {
  /** Row under the keyboard cursor. */
  cursorKey = $state<string | null>(null)
  /** IssueList is on screen. */
  listActive = $state(false)
  /**
   * App has run applyStartupView (keep-url or write). The list is not a
   * keyboard target until it has observed this and flipped keysReady.
   */
  startupViewApplied = $state(false)
  /** The list has absorbed the boot-time first view. Subsequent viewKey changes reset the cursor. */
  keysReady = $state(false)
  /** Open bulk popover, or null. */
  menu = $state<TriageMenu | null>(null)
  /** Quick-comment target issue, or null. */
  commentKey = $state<string | null>(null)
  #heldBootKeys: string[] = []

  setCursor(key: string | null): void {
    this.cursorKey = key
  }

  /**
   * Move the cursor in visible order. With no cursor yet it seeds at the first
   * row going down and the last going up, so j/k both work from a cold list.
   * Returns the new cursor key (null when there is nothing to move onto).
   */
  move(delta: number): string | null {
    const order = visibleKeys()
    if (order.length === 0) return null
    const at = this.cursorKey ? order.indexOf(this.cursorKey) : -1
    let idx = at === -1 ? (delta > 0 ? 0 : order.length - 1) : at + delta
    idx = Math.max(0, Math.min(order.length - 1, idx))
    this.cursorKey = order[idx]
    return this.cursorKey
  }

  /**
   * Open a list menu for the current target. A cursor-only action joins the row
   * to the selection first: the "N selected" bar is what names what the pick is
   * about to change, and the bar is also where the popover hangs.
   * Returns false when there is nothing to act on.
   */
  requestMenu(m: TriageMenu): boolean {
    if (!bulk.active) {
      if (!this.cursorKey) return false
      bulk.add(this.cursorKey)
    }
    this.menu = m
    return true
  }

  closeMenu(): void {
    this.menu = null
  }

  openComment(key: string): void {
    this.commentKey = key
  }

  closeComment(): void {
    this.commentKey = null
  }

  /** View change (filter/sort/group): the old cursor row may be gone. */
  resetCursor(): void {
    this.cursorKey = null
  }

  /** Call once the startup view decision has been applied (or kept). */
  noteStartupViewApplied(): void {
    this.startupViewApplied = true
  }

  /** Hold a j/k/x that arrived before keysReady. No-op once ready. */
  holdBootKey(key: string): void {
    if (this.keysReady) return
    this.#heldBootKeys.push(key)
  }

  /**
   * Flip keysReady and replay held j/k/x against the *current* visible
   * list (the committed view). A row the startup filter removed is never
   * the cursor: move() reads visibleKeys() as of this call.
   */
  markKeysReady(): void {
    if (this.keysReady) return
    this.keysReady = true
    const held = this.#heldBootKeys
    this.#heldBootKeys = []
    replayHeldListKeys(held, {
      move: (dir) => {
        this.move(dir)
      },
      toggleCursor: () => {
        if (this.cursorKey) bulk.toggle(this.cursorKey)
      },
    })
  }
}

/** App-wide singleton. */
export const triage = new TriageStore()
