/*
 * Debug attributes on <html> — the single writer (GDK-1146, GDK-931).
 *
 * Test-observability attributes (data-detail-cache, data-last-dash-open)
 * exist so an e2e poll or a developer with devtools can read app state
 * that has no UI. They are not product surface: in a production build they
 * are off by default, and off must be free — callers hand a THUNK so a
 * disabled publish never computes the value. The original inline write ran
 * [...cache.keys()].join(',') on every cache mutation in the prod bundle
 * (GDK-1146); dataset.lastDashOpen was the same pattern one component over
 * (GDK-931). New debug attributes join DebugAttrName and go through here,
 * never through a bare documentElement.dataset write.
 *
 * On in dev builds (import.meta.env.DEV), or — because the e2e suite
 * serves the production bundle (e2e/serve.sh builds via `npm run build`)
 * — through the localStorage opt-in below, which the specs set in an init
 * script before the app boots.
 */

/** localStorage key that opts a production build into debug attributes. */
export const DEBUG_ATTRS_KEY = 'gadak_debug_attrs'

/** Closed set: every debug attribute this app publishes is named here. */
export type DebugAttrName = 'detailCache' | 'lastDashOpen'

let enabledMemo: boolean | null = null

/**
 * Resolved once: the opt-in is read before the app boots (init script or
 * dev build flag) and cannot meaningfully change afterwards, so the hot
 * path never repeats the localStorage read.
 */
export function debugAttrsEnabled(): boolean {
  if (enabledMemo === null) enabledMemo = computeEnabled()
  return enabledMemo
}

function computeEnabled(): boolean {
  // `?.` on purpose: this module is imported by e2e specs, where
  // import.meta.env does not exist (plain esbuild, no Vite define).
  if (import.meta.env?.DEV) return true
  try {
    return typeof localStorage !== 'undefined' && localStorage.getItem(DEBUG_ATTRS_KEY) === '1'
  } catch {
    /* blocked storage (private mode) is off, not a throw */
    return false
  }
}

/**
 * Publish one debug attribute on <html>. The value is a thunk so the
 * disabled path never evaluates it — the skipped computation, not the
 * skipped attribute write, is the expensive half of GDK-1146.
 */
export function publishDebugAttr(name: DebugAttrName, value: () => string): void {
  if (typeof document === 'undefined' || !debugAttrsEnabled()) return
  document.documentElement.dataset[name] = value()
}
