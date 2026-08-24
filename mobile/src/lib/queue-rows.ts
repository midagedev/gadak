/*
 * Queue row view-model (GDK-801) — the pure half of screens/Queue.svelte.
 *
 * Two contracts live here so vitest can pin them without a DOM:
 *
 *  - Row grammar (ux-report Q1): the status COLUMN is the origin's own
 *    `status` display name; `status_category` is the dot's color and nothing
 *    else. Category words ("New") as row text were the scaffold's mistake —
 *    the same list on web reads dot = category, letters = status
 *    (web/src/components/list/IssueRow.svelte).
 *  - Age grammar: web relativeTime compact buckets ("just now" / "3m" /
 *    "2h" / "2d" / "1w" / "1mo" / "1y", per-locale units). Same thresholds,
 *    same flooring — copied semantics from web/src/lib/i18n (messages/
 *    common.ts time.*), not a new ladder.
 *
 * The mine filter keys on `assignee_id` — account ids, never display names
 * (CLAUDE.md schema rules; flow-report Q3: bootstrap carries assignee_id).
 * api.ts's QueueRow view omits assignee_id because the queue never needed
 * it; the payload carries it (store.IssueLite), so this module widens the
 * view locally instead of touching the transport owner.
 */
import { categoryInk, priorityInk, queueRows, type QueueRow } from './api'
import { t, type Locale } from './i18n'

/** QueueRow plus the account-id axis bootstrap already ships. */
export type QueueRowFull = QueueRow & { assignee_id?: string | null }

/** The bootstrap issue itself may lack the field (older caches) — read it null-safely. */
function assigneeIdOf(row: QueueRowFull): string {
  return typeof row.assignee_id === 'string' ? row.assignee_id : ''
}

export type AgeUnit = 'minute' | 'hour' | 'day' | 'week' | 'month' | 'year'
export type AgeParts = { kind: 'just_now' } | { kind: 'duration'; n: number; unit: AgeUnit }

const MIN = 60_000
const HOUR = 60 * MIN
const DAY = 24 * HOUR

/**
 * Age buckets, web relativeTime thresholds exactly. null when iso is
 * missing or unparseable — the caller renders nothing rather than a lie.
 */
export function ageParts(iso: string | null, now = Date.now()): AgeParts | null {
  if (!iso) return null
  const ts = Date.parse(iso)
  if (Number.isNaN(ts)) return null
  const diff = now - ts
  if (diff < MIN) return { kind: 'just_now' }
  if (diff < HOUR) return { kind: 'duration', n: Math.floor(diff / MIN), unit: 'minute' }
  if (diff < DAY) return { kind: 'duration', n: Math.floor(diff / HOUR), unit: 'hour' }
  const days = Math.floor(diff / DAY)
  if (days < 7) return { kind: 'duration', n: days, unit: 'day' }
  if (days < 30) return { kind: 'duration', n: Math.floor(days / 7), unit: 'week' }
  if (days < 365) return { kind: 'duration', n: Math.floor(days / 30), unit: 'month' }
  return { kind: 'duration', n: Math.floor(days / 365), unit: 'year' }
}

const AGE_KEY: Record<AgeUnit, 'queue.age.minute' | 'queue.age.hour' | 'queue.age.day' | 'queue.age.week' | 'queue.age.month' | 'queue.age.year'> = {
  minute: 'queue.age.minute',
  hour: 'queue.age.hour',
  day: 'queue.age.day',
  week: 'queue.age.week',
  month: 'queue.age.month',
  year: 'queue.age.year',
}

/** Compact row age: "just now" / "3m" / "2d" ('' when absent — no cell text). */
export function ageCompact(iso: string | null, now = Date.now(), locale?: Locale): string {
  const parts = ageParts(iso, now)
  if (!parts) return ''
  if (parts.kind === 'just_now') return t('queue.age.justNow', undefined, locale)
  return t(AGE_KEY[parts.unit], { n: parts.n }, locale)
}

/**
 * Freshness-line sentence: "Synced 3m ago" / "방금 동기화". null when iso is
 * absent (never synced — the line shows the pair label only). Age uses the
 * same compact units; the sentence keys are the queue's own family.
 */
export function syncAgeLabel(iso: string | null, now = Date.now(), locale?: Locale): string {
  const parts = ageParts(iso, now)
  if (!parts) return ''
  if (parts.kind === 'just_now') return t('queue.freshness.syncedJustNow', undefined, locale)
  const unit = t(AGE_KEY[parts.unit], { n: parts.n }, locale)
  return t('queue.freshness.syncedAgo', { ago: unit }, locale)
}

export type QueueMode = 'mine' | 'all'

/**
 * me() decided default: a connected home (account_id set) opens on "mine",
 * a standalone home has no "me" and opens on the whole open queue.
 */
export function defaultMode(accountId: string | null): QueueMode {
  return accountId ? 'mine' : 'all'
}

/**
 * The visible queue: mine narrowing happens BEFORE the sort/slice so a row
 * ranked below the global top-N is still first-class in "mine" — narrowing
 * the sliced list would silently hide it until the next sync. Sorting,
 * unset-rank-last, and the 50-row limit are queueRows' own contract
 * (lib/api.ts) and are not re-derived here.
 *
 * 'mine' with an empty account id (me() pending or failed) is not a filter
 * anyone can satisfy — it returns the full queue rather than an empty one;
 * an empty screen is the #5049 failure mode, never ours.
 */
export function visibleRows(
  issues: QueueRowFull[],
  mode: QueueMode,
  accountId: string,
  limit?: number,
): QueueRow[] {
  if (mode === 'mine' && accountId !== '') {
    return queueRows(issues.filter((i) => assigneeIdOf(i) === accountId), limit)
  }
  return queueRows(issues, limit)
}

/** What one queue row renders — the component is a template over this. */
export interface RowView {
  issue_key: string
  summary: string
  /** Priority tick color (priorityInk ladder — color is the priority's only glyph). */
  priority_ink: string
  /** The origin's own status name. Never the category word. */
  status_text: string
  /** Category ink for the status dot — the category's only surface. */
  status_ink: string
  /** Compact age ('' when updated_at is absent). */
  age: string
}

export function rowView(row: QueueRow, now = Date.now(), locale?: Locale): RowView {
  return {
    issue_key: row.issue_key,
    summary: row.summary,
    priority_ink: priorityInk(row.priority_rank),
    status_text: row.status,
    status_ink: categoryInk(row.status_category),
    age: ageCompact(row.updated_at, now, locale),
  }
}
