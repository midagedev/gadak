/*
 * GDK-965: how a mirror scope's cost reads on a picker row.
 *
 * One owner for the whole rule, because the two ways to get it wrong are a
 * missing answer drawn as "0" and a known-empty space drawn as nothing —
 * and those look identical unless the distinction lives in one place. The
 * wire carries `pages` absent for "the origin could not say"; this returns
 * null for exactly that, and a string for every number the origin stood
 * behind, zero included.
 */
import { t } from '../../lib/i18n'

/** The row's cost label, or null when nothing should be drawn. */
export function pageCountLabel(count: number | undefined | null): string | null {
  if (count === undefined || count === null) return null
  if (!Number.isFinite(count) || count < 0) return null
  // t() groups numeric params for the locale (GDK-1560), so no formatting here.
  return count === 1 ? t('settings.scopePageCountOne') : t('settings.scopePageCount', { n: count })
}
