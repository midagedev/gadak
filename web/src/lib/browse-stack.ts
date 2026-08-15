/*
 * Single owner for how the browse pane stacks against SPA dialogs and toasts.
 *
 * The native WKWebView paints over every SPA pixel in its rectangle, so
 * "what is in front" cannot be a CSS fight: this function is the only
 * answer, and the store / chrome / frame reporter just apply it.
 *
 * Inputs are facts the shell already knows (pane up, a dialog up, a toast
 * up). Outputs are the four levers that exist:
 *
 *   nativeVisible  — POST /desktop/browse/activate with an id, or ""
 *   chromeMounted  — the tab strip + empty viewport are in the DOM
 *   chromeYields   — that chrome sits below the SPA dialog tier (z-50)
 *   reserveToast   — the reported native rectangle must miss the toast host
 *
 * GDK-76 / GDK-77. Do not add a fifth lever (native toast view, extra
 * z-index ladder) without replacing this function.
 */

export interface BrowseStackInput {
  paneOpen: boolean
  dialogOpen: boolean
  toastVisible: boolean
}

export interface BrowseStack {
  nativeVisible: boolean
  chromeMounted: boolean
  chromeYields: boolean
  reserveToast: boolean
}

/** Axis-aligned box in viewport CSS px, y from the top. Same shape as FrameRect. */
export interface StackRect {
  x: number
  y: number
  w: number
  h: number
}

/**
 * Decide the stack from the three facts the shell already has.
 *
 * A dialog hides the native view (it always did) *and* drops the SPA chrome
 * below the dialog tier, so ⌘K at the shipped 1280×820 window is not a
 * dead control. A toast never hides the page — that flickers — it only
 * asks the frame reporter to lift the native bottom edge above the toast
 * host. Dialog + toast together is just the dialog case: native is already
 * gone, the toast sits on the SPA at z-60.
 */
export function resolveBrowseStack(input: BrowseStackInput): BrowseStack {
  const paneOpen = input.paneOpen
  const nativeVisible = paneOpen && !input.dialogOpen
  return {
    nativeVisible,
    chromeMounted: paneOpen,
    chromeYields: paneOpen && input.dialogOpen,
    reserveToast: nativeVisible && input.toastVisible,
  }
}

/**
 * Shrink `frame` so it no longer covers `toast`. A native view is one
 * rectangle, so the only honest cut is the bottom edge (the toast host
 * lives in the bottom-right). No-ops when the toast has no area or does
 * not overlap — the reported frame must stay byte-identical to the
 * viewport box on the no-toast path.
 */
export function applyToastReservation(frame: StackRect, toast: StackRect): StackRect {
  if (toast.w <= 0 || toast.h <= 0) return frame
  const frameRight = frame.x + frame.w
  const frameBottom = frame.y + frame.h
  const toastRight = toast.x + toast.w
  const toastBottom = toast.y + toast.h
  const overlapX = toast.x < frameRight && toastRight > frame.x
  const overlapY = toast.y < frameBottom && toastBottom > frame.y
  if (!overlapX || !overlapY) return frame
  const h = toast.y - frame.y
  if (h <= 0 || h >= frame.h) return frame
  return { x: frame.x, y: frame.y, w: frame.w, h }
}
