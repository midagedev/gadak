/*
 * serve REST client for the queue screen. Endpoints are the read REST the
 * local serve already exposes (internal/server/server.go):
 *
 *   GET /api/v1/issues/bootstrap/   — full lite-rows snapshot
 *
 * The 1차 scope is localhost dev: `gadak demo --addr 127.0.0.1:7899` (or any
 * serve). The tailnet + Bearer path (GDK-797/798) is not deployed yet; the
 * Authorization header below is wired but unverified against a real gate.
 *
 * List logic keys on ids/categories only — status_category and priority_rank,
 * never display names (CLAUDE.md schema rules). priority_rank 0 is unset, not
 * "most urgent" (specs/001 L1 trap), so unset rows sort last.
 */

/** The subset of store.IssueLite the queue renders. */
export interface QueueRow {
  issue_key: string
  summary: string
  status: string
  status_category: string
  priority: string | null
  priority_rank: number
  assignee: string | null
  updated_at: string | null
}

export interface Bootstrap {
  issues: QueueRow[]
  sync_version: number
  server_time: string
}

export class ApiError extends Error {}

/**
 * Origin the requests go to. In dev (vite dev server, and the tauri dev
 * window which loads from it) requests ride the vite proxy to the dev serve
 * port — serve sends no CORS headers by design. The packaged app talks to the
 * configured endpoint directly.
 */
function apiOrigin(endpoint: string): string {
  if (import.meta.env.DEV) return ''
  return endpoint.replace(/\/+$/, '')
}

export async function fetchBootstrap(endpoint: string, token: string, signal?: AbortSignal): Promise<Bootstrap> {
  const headers: Record<string, string> = {}
  if (token !== '') headers.Authorization = `Bearer ${token}`
  let res: Response
  try {
    res = await fetch(`${apiOrigin(endpoint)}/api/v1/issues/bootstrap/`, { headers, signal })
  } catch (err) {
    if (signal?.aborted) throw err
    throw new ApiError('network')
  }
  if (!res.ok) throw new ApiError(`http ${res.status}`)
  const body: unknown = JSON.parse(await res.text())
  if (typeof body !== 'object' || body === null || !Array.isArray((body as Bootstrap).issues)) {
    throw new ApiError('shape')
  }
  return body as Bootstrap
}

/**
 * The 1차 queue: unresolved rows, priority_rank ascending with unset (0)
 * last, updated_at descending as the tiebreak. `assignee` narrowing waits
 * for auth/me (needs the account id — display names are not a key).
 */
export function queueRows(issues: QueueRow[], limit = 50): QueueRow[] {
  const open = issues.filter((i) => i.status_category !== 'done')
  const rank = (i: QueueRow): number => (Number.isFinite(i.priority_rank) && i.priority_rank > 0 ? i.priority_rank : Number.MAX_SAFE_INTEGER)
  const updated = (i: QueueRow): number => {
    if (!i.updated_at) return 0
    const t = new Date(i.updated_at).getTime()
    return Number.isNaN(t) ? 0 : t
  }
  return open
    .map((i) => ({ i, r: rank(i), u: updated(i) }))
    .sort((a, b) => (a.r - b.r) || (b.u - a.u))
    .map(({ i }) => i)
    .slice(0, limit)
}

/*
 * Ink mapping — copied semantics, not new colors: web/src/lib/format.ts
 * categoryMetaOf (status) and PRIORITY_COLOR (priority). Keep in sync with
 * those two tables; they are the owners.
 */
export function categoryInk(cat: string): string {
  if (cat === 'new') return 'var(--color-status-new)'
  if (cat === 'inprogress') return 'var(--color-status-inprogress)'
  if (cat === 'done') return 'var(--color-status-done)'
  return 'var(--color-text-muted)'
}

const PRIORITY_INK = [
  'var(--color-border-strong)', // 0 unset / missing rank
  'var(--color-status-reopen)', // rank 1
  'var(--color-status-inprogress)', // rank 2
  'var(--color-status-new)', // rank 3
  'var(--color-text-secondary)', // rank 4
  'var(--color-text-muted)', // rank 5+
] as const

export function priorityInk(rank: number): string {
  if (!Number.isFinite(rank) || rank < 1) return PRIORITY_INK[0]
  return PRIORITY_INK[rank >= 5 ? 5 : rank]
}
