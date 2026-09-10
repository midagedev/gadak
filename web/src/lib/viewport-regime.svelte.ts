/*
 * GDK-696: the live viewport regime, as one runes-owned value.
 *
 * App.svelte and DetailPanel.svelte each used to hold their own `$state` copy
 * fed by subscribeViewportRegime — duplicated state that could only agree
 * because both subscriptions fired on the same matchMedia change. The class
 * defect this closes is a copy that misses an update: a component reading
 * readViewportRegime() once at setup and waiting for the next flip to catch
 * up. Now the value lives here; components read it and nothing else writes
 * it, so there is no second copy to fall behind.
 *
 * The state is an object whose property is reassigned, not a reassigned
 * `export let` — Svelte forbids exporting a state binding that is written
 * later (state_invalid_export; the e2e web server's vite build is what
 * catches it, svelte-check stays quiet). Same shape as the stores: `me`,
 * `pages` — a $state object mutated through its fields.
 *
 * The pure half (constants, floor math, token strings, the matchMedia
 * plumbing) stays in viewport-regime.ts on purpose: its tests run in the
 * plugin-less unit project, which cannot load a runes module — the same split
 * keymap.svelte.ts makes (resolution logic plain, state reached another way).
 * Importers use the explicit `.svelte` suffix, the store convention
 * (panel.svelte).
 */
import { readViewportRegime, subscribeViewportRegime } from './viewport-regime'

export const viewport = $state({ regime: readViewportRegime() })

// Module-lifetime on purpose: the regime is an app-lifetime value and the
// underlying matchMedia singleton (viewport-regime.ts) already persists the
// same way — one listener for every reader, torn down never.
if (typeof window !== 'undefined') {
  subscribeViewportRegime((r) => {
    viewport.regime = r
  })
}
