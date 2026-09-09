/*
 * The first-sync band's one line (GDK-1677), pure so the wording contract is
 * testable without the store or the DOM: phase × denominator picks the
 * sentence, formatNumber picks the digits, wiki_pending picks the tail. The
 * catalog owns the words; this module owns the arrangement. Plain .ts on
 * purpose — the `unit` vitest project has no svelte plugin, so a formatter
 * living in the .svelte component could not be unit-tested without editing
 * vitest.config.ts's hand-kept project lists.
 */
import { t, formatNumber } from '../../lib/i18n'

/** The subset of api.ts SyncProgress.first_sync the line speaks. Structural,
 *  so this module (and its test) need not import the api surface. */
export interface FirstSyncLineState {
  /** issues | documents — which source fetched/total count. */
  phase: string
  fetched: number
  /** Absent → no denominator ("1,200 so far"). */
  total?: number
  /** Confluence is configured and has not started. */
  wiki_pending?: boolean
}

export function firstSyncLine(fs: FirstSyncLineState): string {
  const documents = fs.phase === 'documents'
  const params = { fetched: formatNumber(fs.fetched) }
  // `total == null`, not truthiness: a denominator the server has not
  // learned yet is absent, and it stays absent until it arrives.
  const withTotal = fs.total == null ? params : { ...params, total: formatNumber(fs.total) }
  let line = documents
    ? t(fs.total == null ? 'firstSync.documentsNoTotal' : 'firstSync.documents', withTotal)
    : t(fs.total == null ? 'firstSync.issuesNoTotal' : 'firstSync.issues', withTotal)
  // The tail is a promise about what comes after the issues phase; once the
  // wiki itself is fetching — or none was configured — there is no next.
  if (!documents && fs.wiki_pending) line = `${line} · ${t('firstSync.wikiNext')}`
  return line
}
