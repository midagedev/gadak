/*
 * The one "save the current view under a name" action, shared by the view
 * settings menu and the palette (GDK-732).
 *
 * The policy lives here, not at each caller: GDK-437 — the product picks the
 * store, and a server behind this bundle is where a view belongs (it follows
 * the user across devices). The hosted demo has no server to write to, so it
 * stays in this browser and says so. A second caller re-deciding that would
 * be a second product, which is the class of bug this file closes; the
 * palette row therefore has no separate "share to team" id.
 *
 * GDK-1343: saving a view is a display decision as much as a filter one —
 * "All open, grouped by epic, by priority" has no chip to hang the door on,
 * so the whole current config goes in.
 */
import { t } from '../../lib/i18n'
import { hasServer } from '../../lib/config'
import { filters } from '../../stores/filters.svelte'
import { views } from '../../stores/views.svelte'
import { write } from '../../stores/write.svelte'

/** True when a save lands on the server (team view) rather than this browser. */
export function savesToServer(): boolean {
  return hasServer()
}

/** Save the current filters+display as `name`. No-op on a blank name. */
export async function saveCurrentView(name: string): Promise<void> {
  const trimmed = name.trim()
  if (!trimmed) return
  const config = filters.currentConfig()
  if (!savesToServer()) {
    views.addPersonal(trimmed, config)
    return
  }
  try {
    await views.addTeam(trimmed, config)
  } catch (e) {
    // Never lose the view quietly: keep it in this browser and say so.
    views.addPersonal(trimmed, config)
    write.toast(t('filter.saveServerFailed'), 'error')
    console.warn('[save-view] 서버 뷰 저장 실패, 브라우저 저장으로 폴백', e)
  }
}
