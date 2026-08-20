/*
 * Browser ↔ gadak-serve reachability. Single owner for the offline banner,
 * the sync-history popover, and the detail-panel auto-retry (GDK-477).
 *
 * Down is any network throw from lib/api (except a caller AbortError, which
 * is a client timeout, not a dead server). Up is a successful pool sync —
 * the same signal that already took the banner down. No new poll.
 */

import { setNetworkDownHandler } from './api'

let offline = $state(false)

function publish(): void {
  if (typeof document === 'undefined') return
  document.documentElement.dataset.serverReachability = offline ? 'down' : 'up'
}

export const reachability = {
  get offline(): boolean {
    return offline
  },
  markDown(): void {
    if (offline) return
    offline = true
    publish()
  },
  markUp(): void {
    if (!offline) return
    offline = false
    publish()
  },
}

setNetworkDownHandler(() => reachability.markDown())
