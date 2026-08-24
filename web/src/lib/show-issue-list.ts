/*
 * Single owner for "put the issue list in the main column".
 *
 * The column is one value (stores/column, GDK-821): showing the list is what
 * releases whatever held it — feed, a document screen, a space, history, a
 * dashboard. Before the union, each of those was a latch a caller had to
 * remember to close, and the dashboard was forgotten once (GDK-815): applying
 * a view painted the list behind it.
 *
 * Not called from applyStartupView or selection.select: a #/?docs=1 boot
 * must keep the document screen, and picking an issue/favorite is a right-
 * panel open, not a column handoff.
 */
import { column } from '../stores/column.svelte'
import { filters } from '../stores/filters.svelte'
import { pages } from '../stores/pages.svelte'
import type { ViewConfig } from './view-config'

/**
 * Give the column to the issue list, then apply the view.
 * `asView` (GDK-479): the config is a named view — chip rendering subtracts
 * these filters so the sidebar highlight is the only expression of them.
 */
export function showIssueList(config: ViewConfig, asView = false): void {
  column.show({ view: 'list' })
  // The docs screens' own state (label narrowing, tree mode) is theirs to
  // drop on the way out; the union only owns who holds the column.
  pages.closeDocs()
  filters.applyConfig(config)
  filters.setViewOrigin(asView ? config.filters : null)
}
