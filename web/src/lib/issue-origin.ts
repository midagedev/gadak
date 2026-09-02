/*
 * The origin's page for an issue key — one resolver for every "open in
 * origin" affordance (key anchor, copy-link's first line, the `o` command,
 * the palette entry, the 400-refusal toast hatch).
 *
 * Source order is the same as `gadak open` (cmd/gadak/agent.go): the row's
 * stored `items.url` first, then the site's /browse/KEY. The stored url is
 * what makes Linear work (GDK-1149) — a Linear workspace has no site, and
 * sources.base_url carries no workspace slug, but every mirrored row holds
 * the page Linear itself minted. The built-in tracker stores a relative
 * `/browse/KEY`, which is not an origin page, so only absolute http(s)
 * values count.
 *
 * Reads the issues store, so store-free modules (command-palette.ts) take
 * the tracker's name from config.ts (`originTrackerName`) instead.
 */

import { jiraBrowseUrl } from './config'
import { issues } from '../stores/issues.svelte'

const ABSOLUTE_HTTP = /^https?:\/\//i

/** Origin page for `issueKey`, or null when this workspace has none. */
export function issueOriginUrl(issueKey: string): string | null {
  const stored = issues.pool.get(issueKey)?.url?.trim() ?? ''
  if (ABSOLUTE_HTTP.test(stored)) return stored
  return jiraBrowseUrl(issueKey)
}
