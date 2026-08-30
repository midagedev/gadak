/*
 * GDK-960: a focus payload is applied once per tab.
 * sessionStorage keeps that across a refresh; a memory slot is the
 * primary so a blocked store cannot bounce the list every 500ms.
 *
 * GDK-981: the key is (at, hash), not `at` alone. `at` is a wall-clock
 * stamp, so it can repeat — an older server stamps it at second
 * resolution, and `gadak views open A && gadak views open B` then hands
 * both writes the same one. Keyed on `at` alone, every tab that applied A
 * drops B in silence. The hash is where the tab is being sent, so pairing
 * the two makes the key change whenever the destination does.
 */

export const UI_FOCUS_KEY = 'gadak:ui-focus-key'

/** The dedup key for one focus payload. Opaque: only ever compared. */
export function uiFocusKey(at: string, hash: string): string {
  return `${at} ${hash}`
}

/** True when this payload has not been applied in this tab yet.
 *  Missing at (older servers that omit it) cannot be deduped and is applied. */
export function shouldApplyUIFocus(
  at: string | null | undefined,
  hash: string,
  lastKey: string | null | undefined,
): boolean {
  if (!at) return true
  return uiFocusKey(at, hash) !== lastKey
}

export function readLastFocusKey(memory: string | null): string | null {
  if (memory) return memory
  try {
    if (typeof sessionStorage === 'undefined') return null
    return sessionStorage.getItem(UI_FOCUS_KEY)
  } catch {
    return null
  }
}

/**
 * What the tab does about the mirrorVersion on one ui-focus tick (GDK-1170).
 *
 * - `ignore`  — no signal (older server, no mirror, failed poll) or the mirror
 *               has not moved. The issue store's 15s backstop still runs.
 * - `baseline` — first sighting. This boot already has this mirror; adopting it
 *               as the baseline is not a change and must not pull.
 * - `pull`    — the mirror moved. Pull a delta now, and adopt the new value.
 * - `wait`    — it moved while a pull is still in flight. Skip this tick and
 *               keep the old baseline, so the next one (500ms later) decides
 *               `pull` again. Two deltas stacked on a 500ms poll would be a
 *               new defect, and dropping the baseline would lose the write.
 */
export type MirrorPollDecision = 'ignore' | 'baseline' | 'pull' | 'wait'

export function decideMirrorPull(
  next: string | null | undefined,
  last: string | null,
  pulling: boolean,
): MirrorPollDecision {
  if (!next) return 'ignore'
  if (last === null) return 'baseline'
  if (next === last) return 'ignore'
  return pulling ? 'wait' : 'pull'
}

export function rememberFocusKey(key: string): void {
  if (!key) return
  try {
    if (typeof sessionStorage === 'undefined') return
    sessionStorage.setItem(UI_FOCUS_KEY, key)
  } catch {
    /* private mode / blocked storage: the memory slot still holds it */
  }
}
