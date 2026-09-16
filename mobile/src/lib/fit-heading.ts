/*
 * The list heading is one line in every language (GDK-1936).
 *
 * The first fix let the header row wrap, which stopped the Japanese view
 * name being cut to `すべて…` but moved the freshness stamp onto a second
 * line — correct in the app and wrong as a picture: in the landing page's
 * three-across exhibit the Japanese tiles carried a header a whole band
 * taller with the stamp floating alone in it (measured on the publication
 * stills: first content row at y=329 against 184 en and 181 ko).
 *
 * So the row stays one line and the least important thing in it sheds its
 * words instead. The stamp keeps its glyph — it is a 44pt control, and the
 * control is the point; the words are the nicety. Nothing else may give:
 * the name is the subject, the count qualifies it, and the two controls owe
 * their touch target.
 *
 * Why an action and not an `$effect`: the measurement has to happen after
 * layout and its only output is a class on this element, which is the DOM's
 * business and not the component's state. `$effect` writing `$state` is the
 * shape GDK-692 forbids, and the gate that enforces it
 * (web/src/lib/effect-assigns-state.test.ts) is right to — this is what the
 * action form is for.
 *
 * What is measured is the NAME, not the row. Flex never lets the row
 * overflow: it hands the shortfall to the one child that can give, and that
 * child is the name. So the question is "did the name have to ellipsize",
 * and it is asked with the stamp's words in, or the answer describes the
 * state the answer produced.
 */

/** The class the element wears while the stamp's words are shed. */
export const TIGHT = 'tight'

/** The name inside the heading — the element that pays for an overflow. */
const NAME = 'h1 .name'

export function fitHeading(node: HTMLElement) {
  let frame = 0

  const measure = () => {
    cancelAnimationFrame(frame)
    node.classList.remove(TIGHT)
    frame = requestAnimationFrame(() => {
      const name = node.querySelector(NAME)
      if (!name) return
      if (name.scrollWidth > name.clientWidth + 1) node.classList.add(TIGHT)
    })
  }

  measure()

  // Re-ask whenever the row's own text or width could have moved: a locale
  // change, a scope change, a count that grew a digit, a rotation.
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
