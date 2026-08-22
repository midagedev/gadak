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
 *   detail min         440px  (13px body × ~60ch × ~6.5px ≈ 390 + 24×2 padding)
 *   272 + 390 + 440 = 1102 → contract 1100px
 * Replaces the GDK-127 1440px split. 1100–1439 is now docked, not a fake modal.
 */

import { trapFocus } from './focus-trap'

const LAYOUT_SIDEBAR_PX = 272
const LAYOUT_LIST_MIN_PX = 390
const LAYOUT_DETAIL_MIN_PX = 440
export const VIEWPORT_DOCKED_MIN_PX = 1100

export type ViewportRegime = 'docked' | 'overlay'

export function readViewportRegime(): ViewportRegime {
  if (typeof window === 'undefined') return 'docked'
  return window.matchMedia(`(min-width: ${VIEWPORT_DOCKED_MIN_PX}px)`).matches
    ? 'docked'
    : 'overlay'
}

type RegimeListener = (regime: ViewportRegime) => void

let media: {
  mq: MediaQueryList
  listeners: Set<RegimeListener>
  apply: () => void
} | null = null

/**
 * One matchMedia for every subscriber (App + DetailPanel). Two MediaQueryList
 * objects could theoretically disagree across a resize if they were created
 * separately; they never should, but they also never needed to exist twice.
 */
export function subscribeViewportRegime(onChange: RegimeListener): () => void {
  if (typeof window === 'undefined') return () => {}
  if (!media) {
    const mq = window.matchMedia(`(min-width: ${VIEWPORT_DOCKED_MIN_PX}px)`)
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
    if (media.listeners.size === 0) {
      media.mq.removeEventListener('change', media.apply)
      media = null
    }
  }
}

export function isOverlayModal(regime: ViewportRegime, panelOpen: boolean): boolean {
  return regime === 'overlay' && panelOpen
}

/** Inline style installing the token set on `.issue-layout`. CSS must not restate the px. */
export function layoutTokenStyle(): string {
  return [
    `--layout-sidebar:${LAYOUT_SIDEBAR_PX}px`,
    `--layout-list-min:${LAYOUT_LIST_MIN_PX}px`,
    `--layout-detail-min:${LAYOUT_DETAIL_MIN_PX}px`,
    `--layout-docked-min:${VIEWPORT_DOCKED_MIN_PX}px`,
  ].join(';')
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
