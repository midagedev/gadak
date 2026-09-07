/*
 * GDK-1566: one cap for the origin round-trips a dropdown menu waits on.
 *
 * The write-path catalog endpoints (site + per-issue priorities, transitions)
 * proxy straight to the origin. When the origin is unreachable the proxy holds
 * the socket for its own upstream timeout — measured 15.02 s on a 502 — while
 * the menu sits on a bare "Loading…" with no elapsed signal and no way out.
 * The mirror is local and fast; anything still unanswered at this cap is not
 * coming back in time to be useful, so the menu must say something (cached
 * catalog or an error with Retry) instead of nothing.
 *
 * One constant because the three loaders (BulkBar priority, PriorityPicker,
 * StatusTransition) are one defect — the audit found them by their shared
 * shape, and a per-caller number would let the next copy reintroduce the wait.
 * e2e imports the constant so no spec re-derives it (no magic numbers).
 *
 * The cap is a client-side Promise.race — the request itself is not aborted:
 * a late success still lands in the store for the next open.
 */

/** How long a menu waits on one origin round-trip before falling back. */
export const MENU_ORIGIN_TIMEOUT_MS = 6000

/** Rejects with this when the race times out. */
export class MenuOriginTimeout extends Error {
  constructor() {
    super(`origin did not answer within ${MENU_ORIGIN_TIMEOUT_MS} ms`)
    this.name = 'MenuOriginTimeout'
  }
}

/**
 * Race a menu's origin call against the shared cap.
 * Resolves with the call's own result; rejects with MenuOriginTimeout at the
 * cap. Store errors (ApiError) pass through unwrapped — credential refusals
 * keep their existing dialog path.
 */
export function withMenuTimeout<T>(call: Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new MenuOriginTimeout()), MENU_ORIGIN_TIMEOUT_MS)
    call.then(
      (v) => {
        clearTimeout(timer)
        resolve(v)
      },
      (e) => {
        clearTimeout(timer)
        reject(e)
      },
    )
  })
}
