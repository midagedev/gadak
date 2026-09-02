/*
 * The origin's page for an issue key — one resolver for every "open in
 * origin" affordance (key anchor, copy-link's first line, the `o` command,
 * the palette entry, the 400-refusal toast hatch).
 *
 * It branches on the origin's type and never crosses over (GDK-1308 — the
 * user's rule: "Jira is Jira, Linear is Linear", no fallbacks):
 *
 *   jira    → the site's /browse/KEY, Jira's canonical page;
 *   linear  → the page Linear itself minted, as sync stored it in
 *             `items.url` (a Linear workspace has no site, and
 *             sources.base_url carries no workspace slug). No row, no url,
 *             no link — never a Jira URL built from a site that is not there;
 *   gadak   → none. The built-in tracker's page IS this app (its stored
 *             `/browse/KEY` is relative, not an origin page).
 *
 * Reads the issues store, so store-free modules (command-palette.ts) take
 * the tracker's name from config.ts (`originTrackerName`) instead.
 */

import { config, jiraBrowseUrl } from './config'
import { ORIGIN_JIRA, ORIGIN_LINEAR } from './workspace'
import { issues } from '../stores/issues.svelte'

const ABSOLUTE_HTTP = /^https?:\/\//i

/** Origin page for `issueKey`, or null when this origin has none for it. */
export function issueOriginUrl(issueKey: string): string | null {
  switch (config().originType) {
    case ORIGIN_JIRA:
      return jiraBrowseUrl(issueKey)
    case ORIGIN_LINEAR: {
      const stored = issues.pool.get(issueKey)?.url?.trim() ?? ''
      return ABSOLUTE_HTTP.test(stored) ? stored : null
    }
    default:
      return null
  }
}
