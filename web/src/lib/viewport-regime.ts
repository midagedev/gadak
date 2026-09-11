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
 *   sidebar            272px  (`--layout-sidebar`; steps to 208 under 900, narrow-independent floor)
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
 *
 * GDK-1091 (audit A-8) + GDK-1369 (2026-09-11): the narrow sidebar step is
 * JS-owned here. It used to be a CSS `@media (max-width: 760px)` block that
 * re-declared the token on each consuming element (five surfaces by the end)
 * because the inline install on .issue-layout out-ranks anything a block
 * could say there — every new consumer had to remember to re-declare, and
 * the one that didn't was the terminal-sheet 64px strip (GDK-1371). The
 * stepped value now rides the same inline install: layoutTokenStyle() reads
 * the narrow matchMedia synchronously, a watcher rewrites the install when
 * the boundary flips, and inheritance delivers the value to every consumer.
 * The boundary moved 760 → under 900 (A-8: 761–899 kept a 272px sidebar
 * starving the list) and is one number with the terminal overlay sheet —
 * terminal/layout.ts re-exports it as TERMINAL_OVERLAY_MAX_PX.
 *
 * GDK-759 (2026-09-11): the sidebar and the list are draggable. The drag
 * writes the same `ui.tokens.layout` tokens the CLI writes, so this file
 * stays the single owner of "what the width is" — commitLayoutOverrides()
 * is the one body both the config document and a handle go through, and the
 * catalog range in LAYOUT_DRAG_CLAMP is both the drag limit and the server's
 * supported range. The behaviour half (pointer, keys, the one PUT on drop) is
 * lib/layout-resize.ts; this file never talks to the network.
 */

export const LAYOUT_SIDEBAR_PX = 272
export const LAYOUT_LIST_MIN_PX = 390
export const LAYOUT_DETAIL_MIN_PX = 438
export const VIEWPORT_DOCKED_MIN_PX =
  LAYOUT_SIDEBAR_PX + LAYOUT_LIST_MIN_PX + LAYOUT_DETAIL_MIN_PX
/** The sidebar's narrow step: what --layout-sidebar paints below LAYOUT_NARROW_MAX_PX. */
export const LAYOUT_SIDEBAR_NARROW_PX = 208
/** The last viewport px the narrow step paints at — under 900, shared with the terminal overlay sheet. */
export const LAYOUT_NARROW_MAX_PX = 899

export type ViewportRegime = 'docked' | 'overlay'

/** The track values in force: shipped defaults, or the user's ui.tokens layout overrides. */
export interface EffectiveLayout {
  sidebar: number
  /** The list column's pinned width, or undefined when the user set none —
   *  undefined is a value here, not a missing one: it is what makes the CSS
   *  var() FALLBACK resolve, which is how each track keeps its own shipped
   *  width (GDK-769). Never substitute a number for it. */
  list?: number
  /** The narrow step in force (user-overridable); never widens past `sidebar`. */
  sidebarNarrow: number
  listMin: number
  detailMin: number
  /** sidebar + listMin + detailMin — the matchMedia floor, kept a sum so the grid and the regime cannot drift (GDK-201/766/842). */
  dockedMin: number
}

// User layout dimension overrides (GDK-842): parsed from the config doc's
// ui.dims by applyLayoutDimOverrides. Missing/invalid → default.
interface LayoutOverrides {
  sidebar?: number
  sidebarNarrow?: number
  list?: number
  listMin?: number
  detailMin?: number
}

let layoutOverrides: LayoutOverrides = {}

export function effectiveLayout(): EffectiveLayout {
  const sidebar = layoutOverrides.sidebar ?? LAYOUT_SIDEBAR_PX
  const sidebarNarrow = layoutOverrides.sidebarNarrow ?? LAYOUT_SIDEBAR_NARROW_PX
  const listMin = layoutOverrides.listMin ?? LAYOUT_LIST_MIN_PX
  const detailMin = layoutOverrides.detailMin ?? LAYOUT_DETAIL_MIN_PX
  return {
    sidebar,
    sidebarNarrow,
    list: layoutOverrides.list,
    listMin,
    detailMin,
    dockedMin: sidebar + listMin + detailMin,
  }
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

/**
 * Whether the narrow sidebar step is in force, read synchronously. In node
 * (no window) the answer is false — the pure half stays deterministic for
 * the plugin-less unit tests.
 */
export function readNarrowViewport(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia(`(max-width: ${LAYOUT_NARROW_MAX_PX}px)`).matches
}

/**
 * The px --layout-sidebar paints right now: the narrow step under the
 * boundary, clamped so a step wider than the sidebar can never widen the
 * rail by crossing 900. The dim catalog's relation (sidebar-narrow ≤
 * sidebar, dimcheck.go) was the CSS era's only inversion guard; the owner
 * clamps locally now, so an illegal config degrades to the wide value
 * instead of inverting the paint.
 */
function effectiveSidebarPx(): number {
  const eff = effectiveLayout()
  return readNarrowViewport() ? Math.min(eff.sidebarNarrow, eff.sidebar) : eff.sidebar
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

type NarrowListener = (narrow: boolean) => void

// The narrow step's matchMedia singleton (GDK-1369): same shape as the
// regime's `media` above — one MediaQueryList for every subscriber, torn
// down when the last listener leaves. Unlike the regime's, the threshold is
// a fixed constant (not override-derived), so it never re-subscribes.
let narrowMedia: {
  mq: MediaQueryList
  listeners: Set<NarrowListener>
  apply: () => void
} | null = null

function teardownNarrowMedia(): void {
  if (!narrowMedia) return
  narrowMedia.mq.removeEventListener('change', narrowMedia.apply)
  narrowMedia = null
}

/**
 * One matchMedia for the narrow boundary. Crossing it must rewrite the
 * inline install — the template has no reactive path back to
 * layoutTokenStyle() — so the watcher's apply refreshes the install as
 * well as notifying listeners (viewport-regime.svelte.ts holds the one
 * subscription, app-lifetime).
 */
export function subscribeViewportNarrow(onChange: NarrowListener): () => void {
  if (typeof window === 'undefined') return () => {}
  if (!narrowMedia) {
    const mq = window.matchMedia(`(max-width: ${LAYOUT_NARROW_MAX_PX}px)`)
    const listeners = new Set<NarrowListener>()
    const apply = () => {
      const narrow = mq.matches
      for (const fn of listeners) fn(narrow)
      refreshLayoutTokenInstall()
    }
    mq.addEventListener('change', apply)
    narrowMedia = { mq, listeners, apply }
  }
  narrowMedia.listeners.add(onChange)
  onChange(narrowMedia.mq.matches)
  return () => {
    if (!narrowMedia) return
    narrowMedia.listeners.delete(onChange)
    if (narrowMedia.listeners.size === 0) teardownNarrowMedia()
  }
}

/** Inline style installing the token set on `.issue-layout`. CSS must not restate the px. */
export function layoutTokenStyle(): string {
  const eff = effectiveLayout()
  const decls = [
    `--layout-sidebar:${effectiveSidebarPx()}px`,
    `--layout-list-min:${eff.listMin}px`,
    `--layout-detail-min:${eff.detailMin}px`,
    `--layout-docked-min:${eff.dockedMin}px`,
  ]
  // Conditional, unlike the four above: an unset --layout-list must stay
  // ABSENT so app.css's per-track var() fallbacks resolve (1360px capped,
  // 1fr docked, the browse clamp). Installing a number here — even the
  // catalog default — would flatten those three tracks into one width.
  if (eff.list !== undefined) decls.push(`--layout-list:${eff.list}px`)
  return decls.join(';')
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
  el.style.setProperty('--layout-sidebar', `${effectiveSidebarPx()}px`)
  el.style.setProperty('--layout-list-min', `${eff.listMin}px`)
  el.style.setProperty('--layout-detail-min', `${eff.detailMin}px`)
  el.style.setProperty('--layout-docked-min', `${eff.dockedMin}px`)
  // removeProperty, not a default px: see layoutTokenStyle(). This is the
  // line that makes a double-click reset actually restore the shipped
  // per-track widths rather than pinning the catalog default.
  if (eff.list === undefined) el.style.removeProperty('--layout-list')
  else el.style.setProperty('--layout-list', `${eff.list}px`)
}

/**
 * Install the user's layout dimension overrides (the --layout-* entries of
 * the config document's ui.dims) and rebuild everything derived from them:
 * the docked floor (still the sum of the three tracks in force), the
 * matchMedia subscription (a new threshold needs a new MediaQueryList), and
 * the inline token install on .issue-layout. Called by applyUserTokens with
 * the sanitized dims map, and with null/empty to restore the defaults.
 *
 * The narrow step (--layout-sidebar-narrow) is consumed here too, since
 * GDK-1369 moved its paint from app.css's 760px block into this install: a
 * changed step value repaints the install (it is part of --layout-sidebar
 * under the boundary) but moves no floor and re-subscribes nothing — the
 * boundary is the fixed LAYOUT_NARROW_MAX_PX, not override-derived.
 */
export function applyLayoutDimOverrides(
  dims: Record<string, string> | null | undefined,
): void {
  const next: LayoutOverrides = {
    sidebar: dimPx(dims, '--layout-sidebar'),
    sidebarNarrow: dimPx(dims, '--layout-sidebar-narrow'),
    list: dimPx(dims, '--layout-list'),
    listMin: dimPx(dims, '--layout-list-min'),
    detailMin: dimPx(dims, '--layout-detail-min'),
  }
  // A handle under the pointer out-ranks the document for its own axis
  // until the drop. The ui-focus poll refetches on any configVersion move,
  // so a change made in ANOTHER tab (or by `gadak config set`) mid-drag
  // would otherwise yank the column out from under the hand that is
  // holding it, and the PUT on drop would then start from a value the
  // person never saw. Only the dragged axis is pinned; everything else in
  // the same document lands normally.
  for (const axis of pinnedAxes) next[axis] = layoutOverrides[axis]
  commitLayoutOverrides(next)
}

/**
 * Adopt `next` as the values in force and rebuild everything derived from
 * them. The single owner of "what the width is": the config document
 * (applyLayoutDimOverrides) and the resize handles (setLayoutOverride) are
 * two callers of this one body, so a dragged width and a written one cannot
 * take different roads into the layout — which is the whole reason the drag
 * writes the same tokens instead of a second store (GDK-759).
 */
function commitLayoutOverrides(next: LayoutOverrides): void {
  const cur = effectiveLayout()
  const nextEff = {
    sidebar: next.sidebar ?? LAYOUT_SIDEBAR_PX,
    sidebarNarrow: next.sidebarNarrow ?? LAYOUT_SIDEBAR_NARROW_PX,
    list: next.list,
    listMin: next.listMin ?? LAYOUT_LIST_MIN_PX,
    detailMin: next.detailMin ?? LAYOUT_DETAIL_MIN_PX,
  }
  if (
    cur.sidebar === nextEff.sidebar &&
    cur.sidebarNarrow === nextEff.sidebarNarrow &&
    cur.list === nextEff.list &&
    cur.listMin === nextEff.listMin &&
    cur.detailMin === nextEff.detailMin
  ) {
    return
  }
  // The list is the one axis with no matchMedia coupling: it is not a term
  // of the docked floor (sidebar + list-min + detail-min), so dragging it
  // must not tear down and rebuild the subscription on every pointermove
  // frame. Every other axis keeps the pre-GDK-759 behaviour.
  const floorAxisMoved =
    cur.sidebar !== nextEff.sidebar ||
    cur.sidebarNarrow !== nextEff.sidebarNarrow ||
    cur.listMin !== nextEff.listMin ||
    cur.detailMin !== nextEff.detailMin
  layoutOverrides = next
  if (media && floorAxisMoved) {
    // Re-subscribe under the new floor. The old MediaQueryList is pinned to
    // the old threshold; subscribeViewportRegime() also fires each listener
    // once immediately, so subscribers land on the regime the new floor
    // implies instead of waiting for a resize. Guarded on the floor since
    // GDK-759: a list drag moves no floor, and tearing the subscription
    // down on every pointermove frame would rebuild a MediaQueryList ~60
    // times a second for nothing.
    const listeners = [...media.listeners]
    teardownMedia()
    for (const fn of listeners) subscribeViewportRegime(fn)
  }
  refreshLayoutTokenInstall()
}

/** The two tracks a person can drag (GDK-759). Both are settable tokens. */
export type DraggableLayoutAxis = 'sidebar' | 'list'

/**
 * The drag limits, read off internal/config/tokencheck/dim-catalog.json's
 * layout.sidebar and layout.list ranges.
 *
 * The catalog range is the supported range, and the drag must be exactly it.
 * Measured, not assumed (tokencheck/dimcheck.go ValidateDimensions): a value
 * outside the range does NOT refuse the write — it is a WARN the server
 * carries, "applied, but only the range is tested". So nothing downstream
 * would stop a handle that could be pulled to 900px; it would simply paint
 * a layout nobody has ever tested, and hand `gadak config get` a standing
 * advisory the person never asked for. The clamp is therefore the one place
 * that decision is made, and it is the catalog rather than a second opinion
 * about it: layout-resize.test.ts reads the catalog off disk and pins both
 * pairs, so widening a range there without widening the drag fails the unit
 * suite.
 *
 * (The CLI keeps the wider door on purpose — a hand-written config.json may
 * still go outside with its eyes open. A pointer has no way to say that.)
 */
export const LAYOUT_DRAG_CLAMP: Record<DraggableLayoutAxis, { min: number; max: number }> = {
  sidebar: { min: 208, max: 320 },
  list: { min: 480, max: 2000 },
}

/** `px` brought inside the catalog range for `axis`, rounded to a whole pixel. */
export function clampLayoutPx(axis: DraggableLayoutAxis, px: number): number {
  const { min, max } = LAYOUT_DRAG_CLAMP[axis]
  return Math.min(max, Math.max(min, Math.round(px)))
}

/** Axes whose handle is currently held — see applyLayoutDimOverrides. */
const pinnedAxes = new Set<DraggableLayoutAxis>()

/** Pin `axis` to the local value for the lifetime of a pointer drag. */
export function pinLayoutAxis(axis: DraggableLayoutAxis): void {
  pinnedAxes.add(axis)
}

/** Release the pin. The next config document wins the axis again. */
export function unpinLayoutAxis(axis: DraggableLayoutAxis): void {
  pinnedAxes.delete(axis)
}

/**
 * Paint `axis` at `px` locally — or, with `undefined`, restore the shipped
 * width by dropping the override entirely (which is what the catalog spells
 * as "not set", and what a double-click on the handle means). Local only:
 * persisting is the handle's job on drop, exactly once, through
 * lib/layout-resize.ts. Values are clamped here as well as there so no
 * caller can install a width outside the catalog's tested range.
 */
export function setLayoutOverride(
  axis: DraggableLayoutAxis,
  px: number | undefined,
): void {
  commitLayoutOverrides({
    ...layoutOverrides,
    [axis]: px === undefined ? undefined : clampLayoutPx(axis, px),
  })
}

/**
 * Standing debug read-out (GDK-759, CLAUDE.md §9c): `__gadakLayout()` in the
 * console answers "why is my column this wide" in one line — the values in
 * force, which of them are the user's rather than the shipped default, what
 * is actually installed on the element, and the clamp the handle obeys. The
 * scratch probe for this round was three getComputedStyle calls pasted by
 * hand; this is that probe, kept.
 */
export function layoutGeometryDebug(): {
  effective: EffectiveLayout
  overrides: LayoutOverrides
  pinned: DraggableLayoutAxis[]
  installed: Record<string, string>
  clamp: typeof LAYOUT_DRAG_CLAMP
  narrow: boolean
} {
  const installed: Record<string, string> = {}
  if (typeof document !== 'undefined') {
    const el = document.querySelector<HTMLElement>('[data-testid="issue-layout"]')
    for (const name of [
      '--layout-sidebar',
      '--layout-list',
      '--layout-list-min',
      '--layout-detail-min',
      '--layout-docked-min',
    ]) {
      installed[name] = el?.style.getPropertyValue(name) || '(unset)'
    }
  }
  return {
    effective: effectiveLayout(),
    overrides: { ...layoutOverrides },
    pinned: [...pinnedAxes],
    installed,
    clamp: LAYOUT_DRAG_CLAMP,
    narrow: readNarrowViewport(),
  }
}

if (typeof window !== 'undefined') {
  ;(window as unknown as { __gadakLayout?: () => unknown }).__gadakLayout =
    layoutGeometryDebug
}
