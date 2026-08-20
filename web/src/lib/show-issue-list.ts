/*
 * Single owner for "put the issue list in the main column".
 *
 * feedOpen / spaceView / docsView are independent latches. App.svelte renders
 * feed > space > docs > ListView, so a caller that only applyConfig-s leaves
 * the person on the document screen (URL updates, list stays hidden).
 *
 * Not called from applyStartupView or selection.select: a #/?docs=1 boot
 * must keep the document screen, and picking an issue/favorite is a right-
 * panel open, not a column handoff.
 */
import { filters } from '../stores/filters.svelte'
import { me } from '../stores/me.svelte'
import { pages } from '../stores/pages.svelte'
import type { ViewConfig } from './view-config'

/**
 * Close the latches that hide ListView, then apply the view.
 * `asView` (GDK-479): the config is a named view — chip rendering subtracts
 * these filters so the sidebar highlight is the only expression of them.
 */
export function showIssueList(config: ViewConfig, asView = false): void {
  me.closeFeed()
  pages.closeDocs()
  filters.applyConfig(config)
  filters.setViewOrigin(asView ? config.filters : null)
}
