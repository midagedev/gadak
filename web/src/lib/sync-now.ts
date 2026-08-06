/*
 * Shared "Sync now" action: POST sync/ then poll progress until done/error.
 * Used by the command palette, sidebar badge, and empty-mirror CTA.
 */
import * as api from './api'
import { ApiError } from './api'
import { t } from './i18n'
import { issues } from '../stores/issues.svelte'
import { write, type ToastKind } from '../stores/write.svelte'

const POLL_MS = 1000
let inflight: Promise<void> | null = null

export interface SyncNowOptions {
  /**
   * Suppress toasts and the settings dialog. For the focus-time mirror pull,
   * which nobody asked for: a background sync that opens a credential dialog
   * over the list is an interruption, and the freshness chip already carries
   * the outcome. Errors go to console.warn instead.
   */
  quiet?: boolean
}

/** Start an incremental (or full) sync and toast progress. Single-flight in the UI. */
export function runSyncNow(
  mode: 'full' | 'incremental' = 'incremental',
  opts: SyncNowOptions = {},
): Promise<void> {
  if (inflight) return inflight
  inflight = doSync(mode, opts.quiet ?? false).finally(() => {
    inflight = null
  })
  return inflight
}

async function doSync(mode: 'full' | 'incremental', quiet: boolean): Promise<void> {
  const say = (message: string, kind: ToastKind = 'info') => {
    if (!quiet) {
      write.toast(message, kind)
      return
    }
    if (kind === 'error') console.warn('[sync-now]', message)
  }
  const openSettings = () => {
    if (!quiet) write.openSettings()
  }

  say(t('sync.starting'))
  try {
    await api.startSync(mode)
  } catch (e) {
    if (e instanceof ApiError && e.code === 'sync_in_progress') {
      // Fall through to poll the in-flight job.
    } else if (e instanceof ApiError && e.code === 'credential_required') {
      say(t('write.needToken'))
      openSettings()
      return
    } else if (e instanceof ApiError && e.code === 'credential_rejected') {
      say(t('write.tokenRejected'), 'error')
      openSettings()
      return
    } else if (e instanceof ApiError && e.code === 'projects_required') {
      say(t('sync.projectsRequired'), 'error')
      return
    } else {
      const msg = e instanceof ApiError ? e.code ?? e.message : String(e)
      say(t('sync.failed', { message: msg }), 'error')
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
      say(t('sync.failed', { message: msg }), 'error')
      return
    }
    if (p.running) continue
    if (p.error || p.phase === 'error') {
      say(t('sync.failed', { message: p.error || p.phase }), 'error')
      return
    }
    // Refresh the mirror pool so the list reflects new/updated issues.
    try {
      await issues.refresh()
    } catch {
      /* pool refresh is best-effort; progress already done */
    }
    say(
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

// Hand the mirror pull to the issue store, which owns the poll/focus lifecycle
// and the reactive `mirrorSyncing` the freshness chip renders. Injected in this
// direction because the store must not import back into this module.
issues.setMirrorPuller((mode, quiet) => runSyncNow(mode, { quiet }))
