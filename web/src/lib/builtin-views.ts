/*
 * Built-in views ([explore])
 *
 * Presets shown under the sidebar "Built-in views" section. Each holds a full
 * ViewConfig (filters + display) and is applied wholesale via filters.applyConfig.
 *
 * Discipline: presets use **tenant-neutral axes only**.
 *  - status_category (new/inprogress/done), unassigned, reopened, stale, resolved_from
 *    mean the same thing on every Jira.
 *  - status names, priority names, issue_type names, project keys vary by site
 *    (and even by account language), so baking them into a preset yields empty
 *    results elsewhere. Users save those as personal/team views themselves.
 *
 * Date-dependent views (resolved this week) recompute at call time, so they are
 * exposed as a function.
 */

import { t } from './i18n'
import { startOfWeekMonday } from './calendar'
import { emptyConfig, type ViewConfig } from './view-config'
import type { IconName } from '../components/ui/Icon.svelte'

export interface BuiltinView {
  id: string // Sidebar active marker and stable key
  icon: IconName // Line glyph, drawn by components/ui/Icon.svelte
  name: string
  hint?: string
  config: ViewConfig
}

/** ISO date (YYYY-MM-DD) of this week's Monday 00:00 in the viewer zone. */
function startOfWeekISO(): string {
  return startOfWeekMonday()
}

/** Config assembly helper — start from empty config, overwrite partials. */
function make(over: {
  filters?: Partial<ViewConfig['filters']>
  display?: Partial<ViewConfig['display']>
}): ViewConfig {
  const c = emptyConfig()
  if (over.filters) Object.assign(c.filters, over.filters)
  if (over.display) Object.assign(c.display, over.display)
  return c
}

export function builtinViews(): BuiltinView[] {
  return [
    {
      id: 'all-open',
      icon: 'inbox',
      name: t('view.allOpen.name'),
      hint: t('view.allOpen.hint'),
      config: make({ filters: { status_category: ['new', 'inprogress'] } }),
    },
    {
      id: 'unassigned-new',
      icon: 'plus-circle',
      name: t('view.unassignedNew.name'),
      hint: t('view.unassignedNew.hint'),
      config: make({
        filters: { unassigned: true, status_category: ['new'] },
        display: { sort: 'created', dir: 'desc' },
      }),
    },
    {
      // group_by is tenant-neutral: epic_key is derived from the hierarchy,
      // never from a site-specific type name.
      id: 'epic-breakdown',
      // Layers, not a compass: the view's whole content is a list sectioned by
      // epic, and a stack says "grouped" where a compass would say "explore".
      icon: 'layers',
      name: t('view.epicBreakdown.name'),
      hint: t('view.epicBreakdown.hint'),
      config: make({
        filters: { status_category: ['new', 'inprogress'] },
        display: { group_by: 'epic' },
      }),
    },
    {
      id: 'reopened',
      icon: 'rotate-ccw',
      name: t('view.reopened.name'),
      hint: t('view.reopened.hint'),
      config: make({
        filters: { reopened: true },
        display: { sort: 'reopen_count', dir: 'desc' },
      }),
    },
    {
      id: 'stale',
      icon: 'clock',
      name: t('view.stale.name'),
      hint: t('view.stale.hint'),
      config: make({ filters: { stale: true }, display: { sort: 'updated', dir: 'asc' } }),
    },
    {
      // Steward view (THEORY.md T4/G4): the aging tail of in-progress work,
      // longest-waiting first — the arrangement is the coaching, no sentence.
      // Sort is 'updated' asc because ViewDisplay has no status_changed_at
      // axis (that column feeds the stale flag instead); oldest update is the
      // closest existing proxy for oldest in status.
      id: 'aging-in-progress',
      icon: 'hourglass',
      name: t('view.agingInProgress.name'),
      hint: t('view.agingInProgress.hint'),
      config: make({
        filters: { status_category: ['inprogress'] },
        display: { sort: 'updated', dir: 'asc' },
      }),
    },
    {
      id: 'recently-updated',
      icon: 'zap',
      name: t('view.recentlyUpdated.name'),
      hint: t('view.recentlyUpdated.hint'),
      config: make({
        filters: { status_category: ['new', 'inprogress'] },
        display: { sort: 'updated', dir: 'desc' },
      }),
    },
    {
      id: 'resolved-week',
      icon: 'check-circle',
      name: t('view.resolvedWeek.name'),
      hint: t('view.resolvedWeek.hint'),
      config: make({
        filters: { status_category: ['done'], resolved_from: startOfWeekISO() },
        display: { sort: 'updated', dir: 'desc' },
      }),
    },
  ]
}
