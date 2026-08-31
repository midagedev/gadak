/*
 * Drop = transition (GDK-1176) — the judgment half.
 *
 * Split from ./board-drag.svelte.ts the way board-moves is split from its
 * runes sibling: what a drop on a column *means* is a pure function of the
 * transition list and two category keys, and belongs in plain vitest.
 *
 * The vocabulary trap this file owns once: a board column key is the
 * mirror's status_category (new|inprogress|done) while Transition.to_category
 * is Jira's REST key (new|indeterminate|done). effectiveCategory folds both
 * sides; neither is ever compared raw.
 */

import type { IssueLite, Transition } from './types'
import { effectiveCategory } from './view-config'

/** The transitions that would land the card in column `colKey`. */
export function transitionsInto(list: readonly Transition[], colKey: string): Transition[] {
  return list.filter((t) => effectiveCategory(t.to_category) === colKey)
}

/**
 * What dropping on `colKey` would be. The verdict is a preview, never a gate:
 * the card's own column is a no-op by design (an intra-category move is the
 * detail panel's job), an unknown list is legal (best-effort dimming must not
 * block what the write path's 400 already enforces), and a known list without
 * a matching transition is illegal.
 */
export function dropVerdict(
  issue: IssueLite,
  colKey: string,
  candidates: readonly Transition[] | null,
): 'legal' | 'illegal' {
  if (effectiveCategory(issue) === colKey) return 'illegal'
  if (!candidates) return 'legal'
  return transitionsInto(candidates, colKey).length > 0 ? 'legal' : 'illegal'
}
