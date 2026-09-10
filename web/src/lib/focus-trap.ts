/*
 * Modal focus trap (Svelte action).
 *
 * Keeps Tab/Shift+Tab inside the dialog and restores the previously focused
 * element on close. Esc handling stays with each dialog — it already owns the
 * close semantics (clear selection vs. dismiss vs. cancel).
 */

// Every clause carries :not([tabindex="-1"]), not just the generic one
// (GDK-142 V3). A roving tabindex — the settings tablist, where only the
// selected tab is tabbable — parks tabindex="-1" on real <button>s, and
// `button:not([disabled])` matched them anyway: Shift+Tab off the first
// control landed on a tab the browser itself would never stop at, and the
// wrap-around cycled through nine tabs the role exists to collapse into one
// stop. The trap's idea of "focusable" now says what the browser's does.
export const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]',
]
  .map((s) => `${s}:not([tabindex="-1"])`)
  .join(',')

export function trapFocus(node: HTMLElement) {
  const previous = document.activeElement as HTMLElement | null

  const focusable = () =>
    [...node.querySelectorAll<HTMLElement>(FOCUSABLE)].filter(
      (el) => el.offsetWidth > 0 || el.offsetHeight > 0 || el === document.activeElement,
    )

  // Only take focus when the dialog did not already focus something itself
  // (the palette focuses its input on mount).
  queueMicrotask(() => {
    if (!node.contains(document.activeElement)) focusable()[0]?.focus()
  })

  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Tab') return
    const els = focusable()
    if (els.length === 0) return
    const first = els[0]
    const last = els[els.length - 1]
    const active = document.activeElement
    if (e.shiftKey && (active === first || !node.contains(active))) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && active === last) {
      e.preventDefault()
      first.focus()
    }
  }

  node.addEventListener('keydown', onKeydown)
  return {
    destroy() {
      node.removeEventListener('keydown', onKeydown)
      previous?.focus?.()
    },
  }
}
