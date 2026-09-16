/*
 * GDK-1966 — the runtime mode's one owner.
 *
 * The phone bundle runs in three shapes, and until now every module decided
 * which one it was in by itself: api.ts read `import.meta.env.DEV`, secure.ts
 * re-derived `hasTauri()` from the window, the terminal transport had its own
 * copy of both. The hosted mode (the bundle a serve hands out at /m/, opened
 * in the phone's browser over the tailnet — no app, no pairing) is a fourth
 * answer to that question, and the only way it stays coherent with the other
 * three is a single ladder every consumer reads:
 *
 *   tauri    the packaged webview (window.__TAURI_INTERNALS__ present)
 *   hosted   a plain browser page — forced by the ?hosted URL param when the
 *            build is a dev one, and by the fallthrough when it is not
 *   dev      the vite webview (import.meta.env.DEV)
 *
 * The param sits *under* tauri and *over* dev: the packaged webview must
 * never be re-classified by a stray query string, while the e2e gate's bundle
 * is a DEV build on purpose (gate-serve.sh — DEV is load-bearing for every
 * other spec), so DEV alone cannot say hosted there. That ordering is the
 * whole reason the param exists; it is the same idiom `?demo-tour` uses.
 *
 * Globals are read at call time, never captured at module load — a test (or
 * a webview) that installs `window.__TAURI_INTERNALS__` after this module
 * first ran still gets the right answer, which is what runtime.test.ts
 * asserts.
 */

/** The Tauri bridge's marker on the window — this module owns the check. */
export function hasTauri(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

export type RuntimeMode = 'tauri' | 'hosted' | 'dev'

function hostedForcedByUrl(): boolean {
  if (typeof location === 'undefined') return false
  return new URLSearchParams(location.search).has('hosted')
}

export function runtimeMode(): RuntimeMode {
  if (hasTauri()) return 'tauri'
  if (hostedForcedByUrl()) return 'hosted'
  if (import.meta.env.DEV) return 'dev'
  // A production bundle in a plain browser tab is the hosted page itself:
  // served by gadak serve at /m/, same-origin, no pairing to read.
  return 'hosted'
}
