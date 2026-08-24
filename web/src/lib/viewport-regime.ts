/*
 * GDK-201 (2026-08-18): single owner of the issue-layout viewport regime.
 *
 * Docked vs overlay used to be decided in two places that could disagree:
 * a CSS `@media (max-width: 1439px)` query painted the panel as a cover and
 * dimmed the list, while the scrim stayed `pointer-events: none` and the
 * list kept receiving clicks. That is the class of defect this file closes.
 *
 * Every overlay consequence — scrim visibility, pointer-events, inert on
 * the background, dialog role, focus trap — is derived from
 * `isOverlayModal(regime, panelOpen)`. CSS geometry keys off
 * `data-viewport-regime` written from the same `regime` value.
 *
 * Docked floor, derived not chosen:
 *   sidebar            272px  (`--layout-sidebar`; 208px below 760, independent)
 *   list min           390px  (key ~70 + status ~80 + ~30ch summary ~195 + gaps ~45)
 *   detail min         438px  (13px body × ~60ch × ~6.5px ≈ 390 + 24×2 padding)
 *   272 + 390 + 438 = 1100
 * The 440 rounding (GDK-201) made the grid 1102 wide at the 1100 contract and
 * clipped the list seam (GDK-766). VIEWPORT_DOCKED_MIN_PX is the sum, so the
 * two cannot drift. Replaces the GDK-127 1440px split. 1100–1439 is docked.
 *
 * GDK-842 (dim wave): the three track values are user-overridable
 * (`ui.tokens.layout` in config.json). effectiveLayout() holds the values in
 * force — defaults, or the overrides applyLayoutDimOverrides() installed from
 * the config document's ui.dims — and the floor stays the sum of whatever is
 * in force, so the invariant survives a user sidebar of 300px (floor 1128).
 */

import { trapFocus } from './focus-trap'

export const LAYOUT_SIDEBAR_PX = 272
export const LAYOUT_LIST_MIN_PX = 390
export const LAYOUT_DETAIL_MIN_PX = 438
export const VIEWPORT_DOCKED_MIN_PX =
  LAYOUT_SIDEBAR_PX + LAYOUT_LIST_MIN_PX + LAYOUT_DETAIL_MIN_PX

export type ViewportRegime = 'docked' | 'overlay'

/** The track values in force: shipped defaults, or the user's ui.tokens layout overrides. */
export interface EffectiveLayout {
  sidebar: number
  listMin: number
  detailMin: number
  /** sidebar + listMin + detailMin — the matchMedia floor, kept a sum so the grid and the regime cannot drift (GDK-201/766/842). */
  dockedMin: number
}

// User layout dimension overrides (GDK-842): parsed from the config doc's
// ui.dims by applyLayoutDimOverrides. Missing/invalid → default.
let layoutOverrides: {
  sidebar?: number
  listMin?: number
  detailMin?: number
} = {}

export function effectiveLayout(): EffectiveLayout {
  const sidebar = layoutOverrides.sidebar ?? LAYOUT_SIDEBAR_PX
  const listMin = layoutOverrides.listMin ?? LAYOUT_LIST_MIN_PX
  const detailMin = layoutOverrides.detailMin ?? LAYOUT_DETAIL_MIN_PX
  return { sidebar, listMin, detailMin, dockedMin: sidebar + listMin + detailMin }
}

/** A positive one-decimal px string, or undefined. Mirrors the server's dim length gate. */
function dimPx(dims: Record<string, string> | null | undefined, cssVar: string): number | undefined {
  const m = /^([0-9]+(?:\.[0-9])?)px$/.exec(dims?.[cssVar] ?? '')
  if (!m) return undefined
  const n = Number.parseFloat(m[1])
  return n > 0 ? n : undefined
}

export function readViewportRegime(): ViewportRegime {
  if (typeof window === 'undefined') return 'docked'
  return window.matchMedia(`(min-width: ${effectiveLayout().dockedMin}px)`).matches
    ? 'docked'
    : 'overlay'
}

type RegimeListener = (regime: ViewportRegime) => void

let media: {
  mq: MediaQueryList
  listeners: Set<RegimeListener>
  apply: () => void
} | null = null

function teardownMedia(): void {
  if (!media) return
  media.mq.removeEventListener('change', media.apply)
  media = null
}

/**
 * One matchMedia for every subscriber (App + DetailPanel). Two MediaQueryList
 * objects could theoretically disagree across a resize if they were created
 * separately; they never should, but they also never needed to exist twice.
 */
export function subscribeViewportRegime(onChange: RegimeListener): () => void {
  if (typeof window === 'undefined') return () => {}
  if (!media) {
    const mq = window.matchMedia(`(min-width: ${effectiveLayout().dockedMin}px)`)
    const listeners = new Set<RegimeListener>()
    const apply = () => {
      const regime: ViewportRegime = mq.matches ? 'docked' : 'overlay'
      for (const fn of listeners) fn(regime)
    }
    mq.addEventListener('change', apply)
    media = { mq, listeners, apply }
  }
  media.listeners.add(onChange)
  onChange(media.mq.matches ? 'docked' : 'overlay')
  return () => {
    if (!media) return
    media.listeners.delete(onChange)
    if (media.listeners.size === 0) teardownMedia()
  }
}

export function isOverlayModal(regime: ViewportRegime, panelOpen: boolean): boolean {
  return regime === 'overlay' && panelOpen
}

/** Inline style installing the token set on `.issue-layout`. CSS must not restate the px. */
export function layoutTokenStyle(): string {
  const eff = effectiveLayout()
  return [
    `--layout-sidebar:${eff.sidebar}px`,
    `--layout-list-min:${eff.listMin}px`,
    `--layout-detail-min:${eff.detailMin}px`,
    `--layout-docked-min:${eff.dockedMin}px`,
  ].join(';')
}

/**
 * Rewrite the four inline token vars on the mounted layout element. App
 * mounts the style once from layoutTokenStyle(); a live override change
 * happens outside Svelte's reactivity (the template has no reactive refs to
 * re-run it), and an inline declaration out-ranks the user-token
 * stylesheet's :root rule — so the install must be rewritten by the code
 * that owns the values. setProperty only: the element's other inline styles
 * are not ours to touch, and a future Svelte re-render would re-derive the
 * same values from layoutTokenStyle().
 */
function refreshLayoutTokenInstall(): void {
  if (typeof document === 'undefined') return
  const el = document.querySelector<HTMLElement>('[data-testid="issue-layout"]')
  if (!el) return
  const eff = effectiveLayout()
  el.style.setProperty('--layout-sidebar', `${eff.sidebar}px`)
  el.style.setProperty('--layout-list-min', `${eff.listMin}px`)
  el.style.setProperty('--layout-detail-min', `${eff.detailMin}px`)
  el.style.setProperty('--layout-docked-min', `${eff.dockedMin}px`)
}

/**
 * Install the user's layout dimension overrides (the --layout-* entries of
 * the config document's ui.dims) and rebuild everything derived from them:
 * the docked floor (still the sum of the three tracks in force), the
 * matchMedia subscription (a new threshold needs a new MediaQueryList), and
 * the inline token install on .issue-layout. Called by applyUserTokens with
 * the sanitized dims map, and with null/empty to restore the defaults.
 *
 * The ≤760px narrow step is deliberately not rescaled: the catalog floor for
 * --layout-sidebar (208px) is also the minimum any user sidebar override may
 * take, so narrow = min(208, sidebar) is 208 for every legal override and
 * the CSS-owned step keeps painting exactly what it painted.
 * --layout-sidebar-narrow is accepted into ui.dims; this runtime still does
 * not read it (the docked floor is narrow-independent) — since GDK-849 its
 * paint consumption lives in app.css's 760px block, which steps the sidebar
 * through var(--layout-sidebar-narrow, 208px).
 */
export function applyLayoutDimOverrides(
  dims: Record<string, string> | null | undefined,
): void {
  const next = {
    sidebar: dimPx(dims, '--layout-sidebar'),
    listMin: dimPx(dims, '--layout-list-min'),
    detailMin: dimPx(dims, '--layout-detail-min'),
  }
  const cur = effectiveLayout()
  const nextEff = {
    sidebar: next.sidebar ?? LAYOUT_SIDEBAR_PX,
    listMin: next.listMin ?? LAYOUT_LIST_MIN_PX,
    detailMin: next.detailMin ?? LAYOUT_DETAIL_MIN_PX,
  }
  if (
    cur.sidebar === nextEff.sidebar &&
    cur.listMin === nextEff.listMin &&
    cur.detailMin === nextEff.detailMin
  ) {
    return
  }
  layoutOverrides = next
  if (media) {
    // Re-subscribe under the new floor. The old MediaQueryList is pinned to
    // the old threshold; subscribeViewportRegime() also fires each listener
    // once immediately, so subscribers land on the regime the new floor
    // implies instead of waiting for a resize.
    const listeners = [...media.listeners]
    teardownMedia()
    for (const fn of listeners) subscribeViewportRegime(fn)
  }
  refreshLayoutTokenInstall()
}

/**
 * Apply the overlay-modal chrome (inert background, dialog role, focus trap)
 * or strip it. Caller must invoke the returned cleanup on the next change.
 */
export function applyOverlayChrome(layout: HTMLElement, modal: boolean): () => void {
  const sidebar = layout.querySelector<HTMLElement>('.issue-sidebar')
  const main = layout.querySelector<HTMLElement>('.issue-main-column')
  const panelEl = layout.querySelector<HTMLElement>('[data-testid="issue-detail-panel"]')
  if (sidebar) sidebar.inert = modal
  if (main) main.inert = modal
  if (panelEl) {
    if (modal) {
      panelEl.setAttribute('role', 'dialog')
      panelEl.setAttribute('aria-modal', 'true')
    } else {
      panelEl.removeAttribute('role')
      panelEl.removeAttribute('aria-modal')
    }
  }
  const trapped = modal && panelEl ? trapFocus(panelEl) : null
  return () => {
    trapped?.destroy()
    if (sidebar) sidebar.inert = false
    if (main) main.inert = false
    panelEl?.removeAttribute('role')
    panelEl?.removeAttribute('aria-modal')
  }
}
