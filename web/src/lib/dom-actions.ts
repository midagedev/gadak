/*
 * The two window listeners every dismissable surface needs, as Svelte actions.
 *
 * Both listen on `window` rather than on the node, because the thing they have
 * to hear about happens outside it — an Esc typed into a search box that is not
 * the dialog, a click on the far side of the screen. The node is what bounds
 * the listener's life and, for the click, what counts as "inside": mount the
 * node and the listener exists, unmount it and it is gone, so a surface can
 * never leave one behind.
 */

/*
 * ── The Esc claim stack (GDK-1565) ─────────────────────────────────────────
 *
 * One Esc used to close a modal dialog AND the surface under it: every
 * surface attached its own window listener, delivery was registration order,
 * and the detail panel (opened first) ran before the dialog over it. The
 * fix is not another listener — it is one listener per phase that walks the
 * claims in a fixed tier order. The stack replaces only that ordering; it
 * does not decide who wins:
 *
 *  - delivery is ordered, not exclusive — every claim of the phase is called
 *    (highest tier first, latest registration first within a tier), so a
 *    claim that arms on an already-spent key (DetailPanel's
 *    defaultPrevented branch) still sees it. A first-winner walk would turn
 *    the pinned two-Esc composer contract (GDK-462) into three;
 *  - `preventDefault` stays the spend convention (what the keymap records as
 *    `ignore` and DetailPanel declines on), `stopPropagation` stays each
 *    handler's own choice for the world outside the stack — stopping the
 *    phase at window suppresses the other phase's walk exactly as it
 *    suppressed other window listeners before;
 *  - each handler keeps its own decline conditions (key, open state,
 *    defaultPrevented, editable target). Nothing here declines on a claim's
 *    behalf.
 *
 * Capture claims run before any element handler and before the shell keymap,
 * which is why the surfaces that must beat the keymap declare capture — the
 * same reason they registered capture listeners before. Bubble claims are the
 * ordered second chance: App's <svelte:window> keymap attaches with App
 * itself, and a claim can only mount inside App's subtree, so the stack's
 * bubble listener — attached by the first claim — always follows the keymap.
 * That is the registration order every pre-stack surface already had, and it
 * is load-bearing: the keymap is the arbiter between the surfaces its ladder
 * owns (Esc clears the bulk selection before it closes a panel), so a claim
 * must never spend the key first. What the walk adds is order among the
 * claims themselves, for the keys the keymap declined (a dialog blocks it) or
 * never owned: dialog before menu before the surface under them.
 *
 * Tier table — surface × tier × why (evidence is the z-index / registration
 * each surface already documents):
 *
 *   capture  40  overlay        MediaViewer — z-[70], the topmost chrome
 *   capture  30  toast          ToastHost — z-[60]; declines to the viewer
 *   capture  20  nestedConfirm  WorkspacesTab's confirm inside settings
 *   capture  10  draft          CommentComposer's unfocused draft (GDK-462)
 *   bubble   40  dialog         the five modal dialogs (DialogShell family)
 *   bubble   30  menu           pickers + the bulk bar's dropdowns
 *   bubble   10  surface        DetailPanel — what the tiers above protect
 *
 * A new surface picks a tier by what it sits on, not by number: inside the
 * dialog tier it is a dialog, over a menu it outranks the menu only if it is
 * a dialog too.
 */

/** Named Esc tiers. Same numbers per phase are one ladder, not one scale. */
export const ESC_TIER = {
  /** capture: full-screen overlay above every other chrome (media viewer). */
  overlay: 40,
  /** capture: the toast stack, under the viewer, above the dialogs. */
  toast: 30,
  /** capture: a nested confirm living inside an open dialog. */
  nestedConfirm: 20,
  /** capture: an unfocused non-empty comment draft (GDK-462). */
  draft: 10,
  /** bubble: a modal dialog over every other bubble surface. */
  dialog: 40,
  /** bubble: an open dropdown menu / picker. */
  menu: 30,
  /** bubble: the surface a menu or dialog sits on (detail panel). */
  surface: 10,
} as const

/** Esc across browsers that report it as `Esc` (old IE/Edge). */
export function isEscapeKey(e: KeyboardEvent): boolean {
  return e.key === 'Escape' || e.key === 'Esc'
}

interface EscapeClaim {
  handler: (e: KeyboardEvent) => void
  phase: 'capture' | 'bubble'
  priority: number
  /** Registration sequence — LIFO within a tier. */
  seq: number
  label: string
}

const escapeClaims: EscapeClaim[] = []
let escapeSeq = 0
let captureAttached = false
let bubbleAttached = false

/** Deliver one Esc to every claim of the phase, highest tier first. */
function walkEscapeClaims(e: KeyboardEvent, phase: 'capture' | 'bubble'): void {
  if (!isEscapeKey(e)) return
  // Snapshot: a handler may unmount its own claim mid-walk.
  const phaseClaims = escapeClaims
    .filter((c) => c.phase === phase)
    .sort((a, b) => b.priority - a.priority || b.seq - a.seq)
  for (const claim of phaseClaims) claim.handler(e)
}

function onWindowCapture(e: Event): void {
  walkEscapeClaims(e as KeyboardEvent, 'capture')
}

function onWindowBubble(e: Event): void {
  walkEscapeClaims(e as KeyboardEvent, 'bubble')
}

/** Attach each phase's single listener when its first claim arrives. */
function syncEscapeListener(phase: 'capture' | 'bubble'): void {
  const attached = phase === 'capture' ? captureAttached : bubbleAttached
  const has = escapeClaims.some((c) => c.phase === phase)
  if (has === attached) return
  if (phase === 'capture') {
    captureAttached = has
    if (has) window.addEventListener('keydown', onWindowCapture, true)
    else window.removeEventListener('keydown', onWindowCapture, true)
  } else {
    bubbleAttached = has
    if (has) window.addEventListener('keydown', onWindowBubble)
    else window.removeEventListener('keydown', onWindowBubble)
  }
}

export interface EscapeOptions {
  /** Called for an Esc while this node is mounted, in tier order. */
  handler: (e: KeyboardEvent) => void
  /** ESC_TIER member. Higher wins within the phase; default surface. */
  priority?: number
  /**
   * Delivery phase. Capture runs before element handlers and the shell
   * keymap; bubble runs after both — the keymap stays the first arbiter.
   * Default bubble.
   */
  phase?: 'capture' | 'bubble'
  /** Debug identity — what window.__gadakEscapeClaims() prints. */
  label?: string
}

/**
 * Esc anywhere in the window, for as long as this node is mounted — as a
 * claim on the ordered stack above, not a listener of its own.
 *
 * The handler gets the event, not just a signal, because the interesting part
 * of an Esc is which surface gets to keep it — see BulkBar (spends its Esc with
 * preventDefault so the detail panel below keeps its own) and DetailPanel
 * (declines an Esc another claim already spent). That negotiation belongs at
 * the site that has an opinion about it, not hidden in an option here.
 *
 * The node is a lifecycle anchor only; a dialog built on DialogShell attaches
 * this to an element in its own template (slot-content root), since actions
 * do not go on components.
 */
export function onEscape(_node: HTMLElement, options: EscapeOptions) {
  const claim: EscapeClaim = {
    handler: options.handler,
    phase: options.phase ?? 'bubble',
    priority: options.priority ?? ESC_TIER.surface,
    seq: escapeSeq++,
    label: options.label ?? 'unlabelled',
  }
  escapeClaims.push(claim)
  syncEscapeListener(claim.phase)
  return {
    update(next: EscapeOptions) {
      claim.handler = next.handler
      claim.priority = next.priority ?? ESC_TIER.surface
      claim.label = next.label ?? 'unlabelled'
      if ((next.phase ?? 'bubble') !== claim.phase) {
        escapeClaims.splice(escapeClaims.indexOf(claim), 1)
        claim.phase = next.phase ?? 'bubble'
        escapeClaims.push(claim)
        syncEscapeListener(claim.phase === 'capture' ? 'bubble' : 'capture')
        syncEscapeListener(claim.phase)
      }
    },
    destroy() {
      const i = escapeClaims.indexOf(claim)
      if (i >= 0) escapeClaims.splice(i, 1)
      syncEscapeListener('capture')
      syncEscapeListener('bubble')
    },
  }
}

/* Debug: what the stack holds right now (__gadakTermSessions idiom). */
declare global {
  interface Window {
    __gadakEscapeClaims?: () => { label: string; phase: string; priority: number }[]
  }
}
if (typeof window !== 'undefined') {
  window.__gadakEscapeClaims = () =>
    [...escapeClaims]
      .sort((a, b) => b.priority - a.priority || b.seq - a.seq)
      .map((c) => ({ label: c.label, phase: c.phase, priority: c.priority }))
}

export interface OutsideClickOptions {
  /** Called on a mousedown whose path does not run through the node. */
  handler: () => void
  /**
   * Listen only while this is true. Dropdowns anchor the boundary on their
   * always-mounted root (the trigger has to count as inside, or clicking it to
   * close would close and immediately reopen), so the node cannot carry the
   * open/closed state by existing — this flag does.
   */
  enabled?: boolean
  /**
   * Bind one task late. The palette runs its commands on mousedown, so a
   * listener attached during that same dispatch would see the opening click as
   * an outside click and close the surface at once.
   */
  defer?: boolean
  /**
   * A second node that counts as inside although it is not a DOM child —
   * FieldEditor's menu, portaled to document.body, so "inside this picker"
   * spans two subtrees. Re-read on every mousedown: a null answer means the
   * portal is not mounted.
   */
  alsoInside?: () => HTMLElement | null
}

/**
 * Mousedown outside the node.
 *
 * Membership is decided by `composedPath()` rather than `contains(e.target)`:
 * the path is captured at dispatch, so it still answers correctly for a target
 * that has been re-targeted or has left the tree by the time we look.
 */
export function onOutsideClick(node: HTMLElement, options: OutsideClickOptions) {
  let opts = options
  let listening = false
  let timer: ReturnType<typeof setTimeout> | undefined

  function onDown(e: MouseEvent) {
    if (e.composedPath().includes(node)) return
    const also = opts.alsoInside?.()
    if (also && e.composedPath().includes(also)) return
    opts.handler()
  }

  function attach() {
    if (listening) return
    listening = true
    if (opts.defer) timer = setTimeout(() => window.addEventListener('mousedown', onDown), 0)
    else window.addEventListener('mousedown', onDown)
  }

  function detach() {
    listening = false
    clearTimeout(timer)
    timer = undefined
    window.removeEventListener('mousedown', onDown)
  }

  function sync() {
    if (opts.enabled ?? true) attach()
    else detach()
  }

  sync()
  return {
    update(next: OutsideClickOptions) {
      opts = next
      sync()
    },
    destroy: detach,
  }
}
