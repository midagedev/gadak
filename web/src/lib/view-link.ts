/*
 * "Copy link" for the list — the same three-line contract the issue
 * detail's copy link keeps (DetailHeader.copyLink), for a view (GDK-1343):
 *
 *   <origin address>        Jira: the issue navigator with this view's JQL
 *   gadak://view?<hash>     the app, primary or /w/<profile> mount
 *   <http>/#/?<hash>        serve / hosted only — desktop has no http origin
 *
 * The hash is the view's single source of truth (url-state), so the app
 * lines are the address bar. The origin line exists only where the origin
 * can take a filter in a URL: Jira. Linear has no public filter parameter
 * and the built-in tracker has no site, so those copy the app lines alone —
 * the same branch the detail takes on a built-in origin, never a stand-in.
 */
import { emitJql } from './api'
import { config, isDesktop, isHostedDemo, jiraFilterUrl, profileName, workspaceName } from './config'
import type { ViewConfig } from './view-config'
import { ORIGIN_JIRA } from './workspace'

export interface ViewLink {
  text: string
  /** The first line is the origin's own address for this view. */
  origin: boolean
  /** Clauses the JQL could not carry (server emit) — the toast names them. */
  omitted: string[]
}

/** The view's hash without its `#/?` — what gadak://view and /#/? both take. */
export function viewHashParams(): string {
  return location.hash.replace(/^#\/?\??/, '')
}

export function gadakViewLink(params: string): string {
  const p = profileName(config().profile)
  const prefix = p !== 'default' ? `/w/${p}` : ''
  return `gadak://view${prefix}${params ? `?${params}` : ''}`
}

export function httpViewLink(params: string): string {
  const ws = workspaceName()
  const prefix = ws ? `/w/${ws}` : ''
  return `${location.origin}${prefix}/#/${params ? `?${params}` : ''}`
}

export async function buildViewLink(cfg: ViewConfig, email?: string | null): Promise<ViewLink> {
  const params = viewHashParams()
  const app = isDesktop() ? gadakViewLink(params) : `${gadakViewLink(params)}\n${httpViewLink(params)}`
  // The hosted demo has no server to emit JQL (every non-GET is a 501).
  if (config().originType !== ORIGIN_JIRA || isHostedDemo()) return { text: app, origin: false, omitted: [] }
  try {
    const res = await emitJql(cfg.filters, cfg.display, email)
    const url = jiraFilterUrl('', res.jql)
    if (!url) return { text: app, origin: false, omitted: res.omitted ?? [] }
    return { text: `${url}\n${app}`, origin: true, omitted: res.omitted ?? [] }
  } catch {
    return { text: app, origin: false, omitted: [] }
  }
}
