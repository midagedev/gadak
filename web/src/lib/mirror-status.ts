/*
 * One sentence for "what is the mirror doing right now".
 *
 * The app used to tell this in three places that could disagree: the sidebar's
 * sync row said "Synced 3m ago", the freshness chip said "Syncing", and the
 * DOCS section said "Fetching documents" — at the same moment, about the same
 * mirror. Reading them together, issue sync and document sync looked like two
 * systems rather than two phases of one.
 *
 * So the wording lives here and every surface renders the same string. They can
 * be placed differently; they cannot say different things.
 */
import { formatNumber, relativeTime, t } from './i18n'
import { issues } from '../stores/issues.svelte'

/** True while the mirror is fetching wiki pages specifically. */
export function fetchingDocuments(): boolean {
  const { running, source } = issues.mirrorActivity
  // A pull this tab started before the server has named a phase counts too:
  // the alternative is a row that claims idleness during the first second.
  return (running && source === 'documents') || (issues.mirrorSyncing && source === '')
}

/**
 * What the mirror is fetching, with how much so far, or null when it is idle.
 *
 * The count is the point. A long first backfill and a hung one look identical
 * without it — six and a half minutes of an unchanging "Fetching documents…"
 * is what sent someone looking for a way to tell whether it had stopped.
 */
export function busyLabel(): string | null {
  if (!issues.mirrorBusy) return null
  const { source, fetched } = issues.mirrorActivity
  const n = formatNumber(fetched)
  if (source === 'documents') {
    return fetched > 0 ? t('sync.busyDocumentsN', { n }) : t('sync.busyDocuments')
  }
  if (source === 'issues') {
    return fetched > 0 ? t('sync.busyIssuesN', { n }) : t('sync.busyIssues')
  }
  // This tab started a pull and the server has not reported a phase yet.
  return t('sync.busy')
}

/**
 * What the mirror is at rest: the verdict and the age in one sentence.
 *
 * These were two sentences in two places, and worse, about two different
 * things — the sidebar reported the browser↔server delta cursor ("Synced
 * 18:12") while the chip reported the mirror's own age ("Synced yesterday").
 * Same verb, different facts, and only one of them is what anyone means by
 * "synced": a delta poll keeps the screen current with a mirror that stopped
 * yesterday. So both read the mirror, and the verdict travels with the age —
 * "delayed" alone never says how far behind, and "yesterday" alone never says
 * that being a day behind is a problem here.
 */
export function settledLabel(): string {
  const health = issues.mirrorHealth
  const when = relativeTime(health?.synced_at ?? null, 'long')
  if (!health) return t('sync.settledChecking')
  const overall = issues.syncHealth?.overall
  if (overall === 'failed' || health.status === 'failed') {
    return when ? t('sync.settledFailedWhen', { when }) : t('sync.settledFailed')
  }
  if (!when || health.status === 'missing') return t('sync.settledNever')
  if (overall === 'warning' || health.status === 'stale') {
    return t('sync.settledDelayedWhen', { when })
  }
  return t('sync.settledOk', { when })
}

/** The one line for the mirror's state, running or not. */
export function mirrorLabel(): string {
  return busyLabel() ?? settledLabel()
}
