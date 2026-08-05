/*
 * Visible issue keys in the order the list paints them (groups flattened).
 * Shared by the cursor (j/k) and by shift-range multi-select so the two can
 * never disagree about what "the next row" is.
 */

import { filters } from '../stores/filters.svelte'

export function visibleKeys(): string[] {
  const out: string[] = []
  for (const g of filters.groups) for (const it of g.items) out.push(it.issue_key)
  return out
}
