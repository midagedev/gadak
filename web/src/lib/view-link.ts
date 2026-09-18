/*
 * "Copy link" for the list — the issue detail's rule (copy-issue-link.ts),
 * for a view (GDK-1343, GDK-1858):
 *
 *   <origin address>        Jira: the issue navigator with this view's JQL,
 *                           and the whole clipboard when the JQL carries
 *                           every clause the view has
 *   gadak://view?<hash>     the app, primary or /w/<profile> mount
 *   <http>/#/?<hash>        serve / hosted only — desktop has no http origin
 *
 * The hash is the view's single source of truth (url-state), so the app
 * lines are the address bar. The origin line exists only where the origin
 * can take a filter in a URL: Jira. Linear has no public filter parameter
 * and the built-in tracker has no site, so those copy the app lines alone —
 * the same branch the detail takes on a built-in origin, never a stand-in.
 *
 * The app lines also stay when a clause could not become JQL (`omitted`):
 * the origin line then opens a *wider* list than the view, so the faithful
 * address is the app's, and the toast says which clauses did not travel.
 */
import { emitJql } from './api'
import { appMountPath, config, isDesktop, isHostedDemo, jiraFilterUrl, profileName } from './config'
import type { ViewConfig } from './view-config'
import { isJiraFamily } from './workspace'

export interface ViewLink {
  text: string
  /** The first line is the origin's own address for this view. */
  origin: boolean
  /** Clauses the JQL could not carry (server emit) — the toast names them. */
  omitted: string[]
  /**
   * The origin address could not be built at all (GDK-1861).
   *
   * Distinct from `origin: false`, which is the ordinary answer on a
   * workspace that has no origin address to give — the built-in tracker,
   * Linear, the hosted demo. This one is a Jira workspace where the emit
   * failed, and it used to be swallowed into that same ordinary answer: the
   * app link was copied and the toast said "Copied", indistinguishable from
   * a built-in. The fact that the origin's address could not be made is what
   * disappeared. GDK-1858 closed the opposite direction of the same class —
   * a payload that differed from what the toast promised.
   */
  originFailed: boolean
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
  return `${location.origin}${appMountPath()}#/${params ? `?${params}` : ''}`
}

export async function buildViewLink(cfg: ViewConfig, email?: string | null): Promise<ViewLink> {
  const params = viewHashParams()
  const app = isDesktop() ? gadakViewLink(params) : `${gadakViewLink(params)}\n${httpViewLink(params)}`
  // The hosted demo has no server to emit JQL (every non-GET is a 501).
  if (!isJiraFamily(config().originType) || isHostedDemo())
    return { text: app, origin: false, omitted: [], originFailed: false }
  try {
    const res = await emitJql(cfg.filters, cfg.display, email)
    const url = jiraFilterUrl('', res.jql)
    // No site address to put the JQL in: an ordinary answer, not a failure.
    if (!url) return { text: app, origin: false, omitted: res.omitted ?? [], originFailed: false }
    const omitted = res.omitted ?? []
    // GDK-1858's rule, with the one exception this surface has: when the
    // origin's address says everything the view says, it is the whole
    // clipboard, exactly as it is for an issue. It is only when a clause
    // could not become JQL that the origin line is a wider list than the
    // view — and then the app lines are the view's only faithful address,
    // so they come along and the toast names what the origin line lost.
    return omitted.length
      ? { text: `${url}\n${app}`, origin: true, omitted, originFailed: false }
      : { text: url, origin: true, omitted, originFailed: false }
  } catch {
    return { text: app, origin: false, omitted: [], originFailed: true }
  }
}
