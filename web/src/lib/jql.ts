/*
 * Client-side JQL paste detection. The parser itself lives in Go
 * (internal/jql) so CLI and UI cannot drift; this file only decides whether
 * a keystroke is FTS or a parse request. Keep the regex in lockstep with
 * internal/jql/extract.go.
 */

const fieldOpRe =
  /\b(project|projectkey|status|statuscategory|statuscategoryid|assignee|reporter|labels?|priority|issuetype|type|components?|fixversions?|created|createddate|updated|updateddate|resolution|text|summary|description|comment|key|issuekey|issue)\b\s*(=|!=|~|!~|>=|<=|>|<|\bin\b|\bis\b)/i
const orderByRe = /\border\s+by\b/i
const jqlEqRe = /(?:\?|&|#|^)jql=/i

export function looksLikeJql(raw: string): boolean {
  const s = raw.trim()
  if (!s) return false
  const ex = extractJql(s)
  if (ex.isUrl || ex.filterId) return true
  if (fieldOpRe.test(s)) return true
  if (s.toLowerCase().includes('currentuser()')) return true
  return orderByRe.test(s)
}

export function extractJql(raw: string): { jql: string; filterId: string; isUrl: boolean } {
  const s = raw.trim()
  if (!s) return { jql: '', filterId: '', isUrl: false }
  try {
    const u = new URL(s)
    if (u.protocol === 'http:' || u.protocol === 'https:') {
      const jql = (u.searchParams.get('jql') ?? '').trim()
      const filterId = (u.searchParams.get('filter') ?? '').trim()
      if (!jql && u.hash) {
        const frag = new URLSearchParams(u.hash.replace(/^#\??/, ''))
        return {
          jql: (frag.get('jql') ?? '').trim(),
          filterId: filterId || (frag.get('filter') ?? '').trim(),
          isUrl: true,
        }
      }
      return { jql, filterId, isUrl: true }
    }
  } catch {
    /* not a URL */
  }
  const m = jqlEqRe.exec(s)
  if (m) {
    let rest = s.slice(m.index + m[0].length)
    const amp = rest.indexOf('&')
    if (amp >= 0) rest = rest.slice(0, amp)
    try {
      rest = decodeURIComponent(rest)
    } catch {
      /* keep raw */
    }
    if (rest.trim()) return { jql: rest.trim(), filterId: '', isUrl: true }
  }
  return { jql: s, filterId: '', isUrl: false }
}
