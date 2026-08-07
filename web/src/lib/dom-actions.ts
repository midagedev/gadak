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

/**
 * Esc anywhere in the window, for as long as this node is mounted.
 *
 * The handler gets the event, not just a signal, because the interesting part
 * of an Esc is which surface gets to keep it — see BulkBar (spends its Esc with
 * preventDefault so the detail panel below keeps its own) and DetailPanel
 * (declines an Esc another listener already spent). That negotiation belongs at
 * the site that has an opinion about it, not hidden in an option here.
 */
export function onEscape(_node: HTMLElement, handler: (e: KeyboardEvent) => void) {
  let fn = handler

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') fn(e)
  }

  window.addEventListener('keydown', onKeydown)
  return {
    update(next: (e: KeyboardEvent) => void) {
      fn = next
    },
    destroy() {
      window.removeEventListener('keydown', onKeydown)
    },
  }
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
    if (!e.composedPath().includes(node)) opts.handler()
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
