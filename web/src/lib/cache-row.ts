/*
 * Shape guard for IssueLite rows that leave IndexedDB.
 *
 * A cached row can predate a field the list indexes into (`labels`, and the
 * same family). TypeScript cannot see that — the field is required on the
 * type — so the guard lives here, at the one door every cached row enters
 * the app through. This is not a data fixer: missing summary stays missing;
 * a missing issue_key is not a row.
 */

import type { IssueLite } from './types'

/** Array fields the list indexes into. Add the next one here, not at call sites. */
export const CACHED_ISSUE_ARRAY_FIELDS = ['labels', 'fix_versions', 'components'] as const

export type CachedIssueArrayField = (typeof CACHED_ISSUE_ARRAY_FIELDS)[number]

/**
 * Return a row the list can index into, or null when it is not a row at all.
 * Coerces {@link CACHED_ISSUE_ARRAY_FIELDS} to `[]` when absent or not an array.
 * Optional fields are left untouched. A healthy row is the same object.
 */
export function normalizeCachedIssue(row: unknown): IssueLite | null {
  if (typeof row !== 'object' || row === null) return null
  const rec = row as Record<string, unknown>
  if (typeof rec.issue_key !== 'string' || rec.issue_key === '') return null

  let patch: Partial<Record<CachedIssueArrayField, string[]>> | null = null
  for (const field of CACHED_ISSUE_ARRAY_FIELDS) {
    if (Array.isArray(rec[field])) continue
    if (!patch) patch = {}
    patch[field] = []
  }
  if (!patch) return row as IssueLite
  return { ...(row as IssueLite), ...patch }
}
