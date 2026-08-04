/*
 * Shared "Sync now" action: POST sync/ then poll progress until done/error.
 * Used by the command palette, sidebar badge, and empty-mirror CTA.
 */
import * as api from './api'
import { ApiError } from './api'
import { t } from './i18n'
import { issues } from '../stores/issues.svelte'
import { write } from '../stores/write.svelte'

const POLL_MS = 1000
let inflight: Promise<void> | null = null

/** Start an incremental (or full) sync and toast progress. Single-flight in the UI. */
export function runSyncNow(mode: 'full' | 'incremental' = 'incremental'): Promise<void> {
  if (inflight) return inflight
  inflight = doSync(mode).finally(() => {
    inflight = null
  })
  return inflight
}

async function doSync(mode: 'full' | 'incremental'): Promise<void> {
  write.toast(t('sync.starting'), 'info')
  try {
    await api.startSync(mode)
  } catch (e) {
    if (e instanceof ApiError && e.code === 'sync_in_progress') {
      // Fall through to poll the in-flight job.
    } else if (e instanceof ApiError && e.code === 'credential_required') {
      write.toast(t('write.needToken'), 'info')
      write.openSettings()
      return
    } else if (e instanceof ApiError && e.code === 'credential_rejected') {
      write.toast(t('write.tokenRejected'), 'error')
      write.openSettings()
      return
    } else if (e instanceof ApiError && e.code === 'projects_required') {
      write.toast(t('sync.projectsRequired'), 'error')
      return
    } else {
      const msg = e instanceof ApiError ? e.code ?? e.message : String(e)
      write.toast(t('sync.failed', { message: msg }), 'error')
      return
    }
  }

  // Poll until the job settles.
  for (;;) {
    await sleep(POLL_MS)
    let p: api.SyncProgress
    try {
      p = await api.getSyncProgress()
    } catch (e) {
      const msg = e instanceof ApiError ? e.code ?? e.message : String(e)
      write.toast(t('sync.failed', { message: msg }), 'error')
      return
    }
    if (p.running) continue
    if (p.error || p.phase === 'error') {
      write.toast(t('sync.failed', { message: p.error || p.phase }), 'error')
      return
    }
    // Refresh the mirror pool so the list reflects new/updated issues.
    try {
      await issues.refresh()
    } catch {
      /* pool refresh is best-effort; progress already done */
    }
    write.toast(
      t('sync.done', {
        n: String(p.fetched),
        changed: String(p.changed),
      }),
      'success',
    )
    return
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}
