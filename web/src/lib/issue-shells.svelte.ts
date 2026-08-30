/*
 * The live session table, as reactive state (GDK-1162/GDK-1164).
 *
 * Judgments and wire calls live in ./issue-shells (no runes there, so they
 * are plain vitest); this file is only "who is asking, and how often".
 *
 * It is a poll for the same reason the revoke watchdog is
 * (internal/server/terminal.go): the binding is written by a *different
 * process* — `gadak claim` running inside a pane's shell — so there is
 * nothing in this tab to notify it. The cost is one loopback GET every
 * POLL_MS, and only while a surface that draws the answer is mounted.
 *
 * Nothing here writes. See ./issue-shells for why that is a rule and not an
 * accident.
 */

import { isHostedDemo } from './config'
import { fetchShellSessions, type ShellSession } from './issue-shells'

/**
 * Poll period. Slow enough to be free on loopback, fast enough that a claim
 * typed in the pane lights the ▶ before the person has finished reading the
 * issue they claimed.
 */
export const SHELL_POLL_MS = 4000

class ShellIndex {
  /** Empty also means "no evidence" — a surface draws nothing either way. */
  sessions = $state<ShellSession[]>([])
  #watchers = 0
  #timer: ReturnType<typeof setInterval> | null = null

  /**
   * Called by a surface that draws the answer; returns its unsubscribe.
   * Refcounted, so two open panels share one poll and closing both stops it.
   */
  track(): () => void {
    // A static snapshot has no serve to ask, and asking anyway would put a
    // 404 in the console of every demo page load.
    if (isHostedDemo() || typeof window === 'undefined') return () => {}
    this.#watchers++
    if (this.#watchers === 1) {
      void this.refresh()
      this.#timer = setInterval(() => void this.refresh(), SHELL_POLL_MS)
    }
    let released = false
    return () => {
      if (released) return
      released = true
      this.#watchers--
      if (this.#watchers > 0) return
      if (this.#timer !== null) {
        clearInterval(this.#timer)
        this.#timer = null
      }
      // Drop the table rather than keep a stale one: the next reader would
      // otherwise draw a ▶ for a shell that has since been reaped.
      this.sessions = []
    }
  }

  async refresh(): Promise<void> {
    this.sessions = await fetchShellSessions()
  }
}

export const shells = new ShellIndex()
