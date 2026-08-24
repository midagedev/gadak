/*
 * Recent searches — the answer an empty search screen already holds
 * (ux-report Q4: "빈 화면이 이미 답을 갖고 있다"). localStorage like the
 * pairing meta in settings.ts: not a secret, exportable, survives a re-pair.
 *
 * Contract: newest first, at most MAX_RECENT_SEARCHES entries, deduped
 * case-insensitively (the server FTS is case-insensitive, so "gdk" and "GDK"
 * are the same search — the newest spelling wins and moves to the front).
 * Recording happens at execution, not at success: a search that failed is
 * exactly the one the user wants to retry from the list.
 */

export const MAX_RECENT_SEARCHES = 10

const KEY = 'gadak-mobile.recent-searches'

/** Stored value as-is, null on absent/corrupt storage. */
function readRaw(): unknown {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? 'null')
  } catch {
    return null
  }
}

/** The sanitized list: string entries only, trimmed, non-empty, capped. */
export function readRecentSearches(): string[] {
  const v = readRaw()
  if (!Array.isArray(v)) return []
  const out: string[] = []
  for (const item of v) {
    if (typeof item !== 'string') continue
    const s = item.trim()
    if (s !== '') out.push(s)
  }
  return out.slice(0, MAX_RECENT_SEARCHES)
}

/**
 * Records one executed search and returns the resulting list (so the caller
 * updates its state without a re-read). Blank input is a no-op; storage
 * failures are swallowed — recents are best-effort, never a crash.
 */
export function recordRecentSearch(q: string): string[] {
  const s = q.trim()
  if (s === '') return readRecentSearches()
  const next = [
    s,
    ...readRecentSearches().filter((x) => x.toLowerCase() !== s.toLowerCase()),
  ].slice(0, MAX_RECENT_SEARCHES)
  try {
    localStorage.setItem(KEY, JSON.stringify(next))
  } catch {
    /* private mode / quota — the app runs without recents */
  }
  return next
}
