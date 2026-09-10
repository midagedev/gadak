/*
 * Keyboard reordering shared by the sidebar sections (GDK-434/435) and the
 * favorites rows (GDK-733).
 *
 * Both lists are drag-reorderable, and both answer the same keyboard gesture:
 * Alt+ArrowUp / Alt+ArrowDown moves the focused row one step. The gesture and
 * the step arithmetic live here so the second list cannot drift into a
 * different key, a different edge behaviour, or a different wrap rule — the
 * two stores still own their own persistence, which is why only these two
 * pieces are shared.
 */

/** -1 (up) / 1 (down) for Alt+Arrow, null for every other key. */
export function altReorderDelta(e: KeyboardEvent): -1 | 1 | null {
  if (!e.altKey) return null
  if (e.key === 'ArrowUp') return -1
  if (e.key === 'ArrowDown') return 1
  return null
}

/**
 * The neighbour `id` should swap with, or null at either edge (no wrap) and
 * when `id` is not in the list. `list` is the *visible* order: hidden entries
 * stay put because they are not steps.
 */
export function stepTarget<T>(list: readonly T[], id: T, delta: -1 | 1): T | null {
  const from = list.indexOf(id)
  if (from < 0) return null
  const to = from + delta
  if (to < 0 || to >= list.length) return null
  return list[to]
}
