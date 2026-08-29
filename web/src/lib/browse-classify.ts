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
const WIKI_PAGE_PATH_RE = /\/wiki\/spaces\/([^/]+)\/pages\/(\d+)/

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
  return m ? m[2] : null
}

export type BrowseLabelParts =
  | { kind: 'issue'; key: string }
  | { kind: 'page'; space: string; pageId: string }

/** Key-bearing parts of an Atlassian browse path, for naming a tab before its
 *  page answers with a title (GDK-1106): the issue key from a /browse/KEY
 *  path, or the wiki space and page id from a /wiki/spaces/…/pages/ID path.
 *  Null when the path carries neither — callers fall back to the host.
 *  Parsing is the same as classifyAtlassianLink (lowercase keys normalized,
 *  the key must end at a path boundary), so the tab label cannot drift from
 *  the classifier. */
export function browseLabelParts(pathname: string): BrowseLabelParts | null {
  const key = extractBrowseKey(pathname)
  if (key) return { kind: 'issue', key }
  const wiki = pathname.match(WIKI_PAGE_PATH_RE)
  if (wiki) return { kind: 'page', space: wiki[1], pageId: wiki[2] }
  return null
}

const GITHUB_PR_RE = /^\/([^/]+\/[^/]+)\/pull\/(\d+)(?:[/?#]|$)/
const GITHUB_COMMIT_RE = /^\/([^/]+\/[^/]+)\/commit\/([0-9a-f]{7,40})(?:[/?#]|$)/

/**
 * GitHub URLs open in the in-app pane too (GDK-527): PRs and commits linked
 * from issues are part of reading the issue, and the pane's native webview
 * has none of the iframe restrictions github.com sets. Only github.com
 * proper — subdomains (gist, raw) stay in the system browser.
 */
export function isGitHubLink(href: string): boolean {
  try {
    const u = new URL(href)
    return (
      (u.protocol === 'http:' || u.protocol === 'https:') &&
      (u.host === 'github.com' || u.host === 'www.github.com')
    )
  } catch {
    return false
  }
}

/** Compact label for a GitHub tab before its page answers with a title:
 *  `org/repo#42` for a PR, `org/repo@sha7` for a commit, null otherwise. */
export function githubTabLabel(href: string): string | null {
  try {
    const u = new URL(href)
    if (u.host !== 'github.com' && u.host !== 'www.github.com') return null
    const pr = u.pathname.match(GITHUB_PR_RE)
    if (pr) return `${pr[1]}#${pr[2]}`
    const commit = u.pathname.match(GITHUB_COMMIT_RE)
    if (commit) return `${commit[1]}@${commit[2].slice(0, 7)}`
    return null
  } catch {
    return null
  }
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
