/*
 * The list heading is one line in every language (GDK-1936), and the name
 * keeps its words (GDK-1974, 2026-09-17).
 *
 * The stamp's words are gone for good now, so a squeeze can no longer be
 * paid from them: at 402px, with three 44pt controls beside the name, the
 * name slot holds about 4.5 CJK glyphs at 26px, and the built-in view names
 * run longer than that in ja and ko. Shortening a catalog name does not
 * close the class, and a saved view's name is the user's — so the name
 * gives up size before it gives up words: 26px, a 22px midpoint, then
 * --text-title (19px); if even 19px cannot hold the words, the count
 * (`·N`) is the one other thing that gives — it hides, and the palette
 * still shows every count — and only past that last step does the CSS
 * ellipsis take over (measured 2026-09-17: `Unassigned new` / `未割り当ての新規`
 * at 19px are 130px / 150px into a 119px slot; the count is the ~40px that
 * lets them stay whole). Nothing else in the row gives.
 *
 * Why an action and not an `$effect`: the measurement has to happen after
 * layout and its only output is an attribute on this element, which is the
 * DOM's business and not the component's state. `$effect` writing `$state`
 * is the shape GDK-692 forbids, and the gate that enforces it
 * (web/src/lib/effect-assigns-state.test.ts) is right to — this is what the
 * action form is for.
 *
 * What is measured is the NAME, not the row. Flex never lets the row
 * overflow: it hands the shortfall to the one child that can give, and that
 * child is the name. So the question is "how far down the ladder did the
 * name have to walk", and it is asked by walking from the top inside one
 * frame: every read is synchronous, so each intermediate size is laid out
 * to be measured but never painted.
 */

/**
 * The ladder, index = step. `''` is the attribute's absence — the CSS
 * default (26px) is the first step, so a name that fits whole wears
 * nothing. The CSS half lives in Issues.svelte (`[data-fit='1']` = the
 * 22px midpoint, `[data-fit='2']` = `--text-title`, `[data-fit='3']` =
 * 19px with the count hidden). Exported so the heading gate names the
 * steps it pins (mobile/e2e/heading.spec.ts).
 */
export const FIT_STEPS = ['', '1', '2', '3'] as const

/** The name inside the heading — the element that pays for an overflow. */
const NAME = 'h1 .name'

/** A step holds the name when its content fits the line within 1px. */
function fits(name: Element): boolean {
  return name.scrollWidth <= name.clientWidth + 1
}

export function fitHeading(node: HTMLElement) {
  let frame = 0

  const measure = () => {
    cancelAnimationFrame(frame)
    frame = requestAnimationFrame(() => {
      const name = node.querySelector(NAME)
      if (!name) return
      node.removeAttribute('data-fit')
      if (fits(name)) return
      for (const step of FIT_STEPS.slice(1)) {
        node.setAttribute('data-fit', step)
        if (fits(name)) return
      }
    })
  }

  measure()

  // Re-ask whenever the row's own text or width could have moved: a locale
  // change, a scope change, a count that grew a digit, a rotation.
  // data-fit itself is not observed (attributes are not in the observer's
  // options), so the walk cannot retrigger itself.
  const observer = new MutationObserver(measure)
  observer.observe(node, { childList: true, subtree: true, characterData: true })
  const resize = new ResizeObserver(measure)
  resize.observe(node)

  return {
    destroy() {
      cancelAnimationFrame(frame)
      observer.disconnect()
      resize.disconnect()
    },
  }
}
