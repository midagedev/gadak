/*
 * GDK-960: a focus payload is applied once per tab, keyed on `at`.
 * sessionStorage keeps that across a refresh; a memory slot is the
 * primary so a blocked store cannot bounce the list every 500ms.
 */

export const UI_FOCUS_AT_KEY = 'gadak:ui-focus-at'

/** True when this write-timestamp has not been applied in this tab yet.
 *  Missing at (older servers that omit it) cannot be deduped and is applied. */
export function shouldApplyUIFocus(
  at: string | null | undefined,
  lastAt: string | null | undefined,
): boolean {
  if (!at) return true
  return at !== lastAt
}

export function readLastFocusAt(memory: string | null): string | null {
  if (memory) return memory
  try {
    if (typeof sessionStorage === 'undefined') return null
    return sessionStorage.getItem(UI_FOCUS_AT_KEY)
  } catch {
    return null
  }
}

export function rememberFocusAt(at: string): void {
  if (!at) return
  try {
    if (typeof sessionStorage === 'undefined') return
    sessionStorage.setItem(UI_FOCUS_AT_KEY, at)
  } catch {
    /* private mode / blocked storage: the memory slot still holds it */
  }
}
