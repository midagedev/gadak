/*
 * The link-type catalog, held once per workspace (GDK-1297).
 *
 * The catalog is a workspace asset — the origin's link types — not an
 * issue's. LinkedIssues used to refetch it on every detail open and empty
 * its rows first, so every open flashed the bare type name ("Blocks") for
 * a frame before the direction phrase ("is blocked by") came back. The
 * first successful answer for a scope is kept here and every later open
 * reads it synchronously; a failed fetch is not kept, so the next open
 * retries, exactly as before.
 *
 * Plain module state, not $state: the component copies the rows into its
 * own state, and nothing renders from this store directly.
 */
import type { IssueLinkType } from '../lib/api'

let held: { scope: string; rows: IssueLinkType[] } | null = null

export const linkTypeCatalog = {
  /** The rows held for `scope`, or null when this scope has not answered yet. */
  get(scope: string): IssueLinkType[] | null {
    return held && held.scope === scope ? held.rows : null
  },
  /** Keep a successful answer. A later scope replaces an earlier one — one
   *  tab shows one workspace. */
  set(scope: string, rows: IssueLinkType[]): void {
    held = { scope, rows }
  },
  /** Forget everything (tests; a workspace switch that stays in the tab). */
  reset(): void {
    held = null
  },
}
