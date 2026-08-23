/*
 * Anti-flicker grace for read-path skeletons (UX_PRINCIPLES §1).
 *
 * gadak has no spinners: the mirror is local, and a load that finishes
 * inside this window must paint nothing at all. Boot used to own the
 * delay inline in App.svelte; every other surface ignored it. One
 * definition, one timer owner. Callers start the read immediately and
 * only ask "has this been pending long enough to show a skeleton?"
 *
 * The delay is a display delay. It must never delay the read itself.
 *
 * visible cannot be a $derived — it depends on a clock. The $effect
 * below owns the timer, resets on identity change, and cleans up on
 * destroy (the class GDK-692 closed is "$effect writing state a
 * derivation could have produced"; a timer is not that).
 */

/** Sole definition of the read-path skeleton grace. */
export const SKELETON_GRACE_MS = 120

export interface SkeletonGrace {
  /** True only after pending has been true for SKELETON_GRACE_MS. */
  readonly visible: boolean
  /**
   * Locator / screenshot value for `data-skeleton`.
   * omitted → not pending; "wait" → inside the grace (no skeleton painted);
   * the grace constant as a string → the skeleton is on screen.
   */
  readonly attr: string | undefined
}

/**
 * Call during component init (registers an `$effect`), same as
 * createResource / createUserSearch.
 *
 * `getIdentity` restarts the grace when the subject changes while
 * pending stays true (issue A still loading → issue B). Without it the
 * first subject's elapsed wait would leak onto the next.
 */
export function createSkeletonGrace(
  getPending: () => boolean,
  getIdentity?: () => unknown,
): SkeletonGrace {
  let visible = $state(false)
  let timer: ReturnType<typeof setTimeout> | null = null

  $effect(() => {
    const pending = getPending()
    getIdentity?.()

    if (timer) {
      clearTimeout(timer)
      timer = null
    }

    if (!pending) {
      visible = false
      return
    }

    visible = false
    timer = setTimeout(() => {
      visible = true
      timer = null
    }, SKELETON_GRACE_MS)

    return () => {
      if (timer) {
        clearTimeout(timer)
        timer = null
      }
    }
  })

  return {
    get visible() {
      return visible
    },
    get attr() {
      if (!getPending()) return undefined
      return visible ? String(SKELETON_GRACE_MS) : 'wait'
    },
  }
}
