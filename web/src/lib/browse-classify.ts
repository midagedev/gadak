/*
 * Pure classification of Atlassian URLs, shared by the link interceptor
 * (desktop-links), the omnibox, and the browse pane's session refresh.
 *
 * Leaf module on purpose: desktop-links and browse.svelte import each other's
 * neighbors in a cycle otherwise. No I/O, no config reads — callers pass
 * jiraBaseUrl.
 */

export type AtlassianLinkKind = 'issue' | 'page' | 'other'

export interface ClassifiedAtlassianLink {
  inApp: boolean
  kind: AtlassianLinkKind
  key: string | null
}

const ISSUE_KEY_RE = /^[A-Z][A-Z0-9]*-\d+$/
const BROWSE_PATH_RE = /\/browse\/([A-Za-z][A-Za-z0-9]*-\d+)(?:\/|$)/
const WIKI_PAGE_PATH_RE = /\/wiki\/spaces\/[^/]+\/pages\/(\d+)/

/** Issue key in a /browse/KEY path, uppercased. Null when the path is not one. */
export function extractBrowseKey(href: string): string | null {
  const m = href.match(BROWSE_PATH_RE)
  if (!m) return null
  const key = m[1].toUpperCase()
  return ISSUE_KEY_RE.test(key) ? key : null
}

/** Confluence content id in a /wiki/spaces/…/pages/ID path. */
export function extractWikiPageId(href: string): string | null {
  const m = href.match(WIKI_PAGE_PATH_RE)
  return m ? m[1] : null
}

/**
 * Classify an absolute Atlassian URL for in-app browse + optional resync.
 * Pure: no I/O, no config reads — callers pass jiraBaseUrl.
 */
export function classifyAtlassianLink(
  href: string,
  jiraBaseUrl: string | null,
): ClassifiedAtlassianLink {
  if (!jiraBaseUrl) {
    return { inApp: false, kind: 'other', key: null }
  }
  let url: URL
  let base: URL
  try {
    base = new URL(jiraBaseUrl)
    url = new URL(href)
  } catch {
    return { inApp: false, kind: 'other', key: null }
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    return { inApp: false, kind: 'other', key: null }
  }
  if (url.origin !== base.origin) {
    return { inApp: false, kind: 'other', key: null }
  }

  const issueKey = extractBrowseKey(url.pathname)
  if (issueKey) {
    return { inApp: true, kind: 'issue', key: issueKey }
  }

  const wikiId = extractWikiPageId(url.pathname)
  if (wikiId) {
    return { inApp: true, kind: 'page', key: wikiId }
  }

  return { inApp: true, kind: 'other', key: null }
}
