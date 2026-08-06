/*
 * Built-in views ([explore])
 *
 * Presets shown under the sidebar "Built-in views" section. Each holds a full
 * ViewConfig (filters + display) and is applied wholesale via filters.applyConfig.
 *
 * Discipline: presets use **tenant-neutral axes only**.
 *  - status_category (new/inprogress/done), unassigned, reopened, stale, updated_from
 *    mean the same thing on every Jira.
 *  - status names, priority names, issue_type names, project keys vary by site
 *    (and even by account language), so baking them into a preset yields empty
 *    results elsewhere. Users save those as personal/team views themselves.
 *
 * Date-dependent views (resolved this week) recompute at call time, so they are
 * exposed as a function.
 */

import { t } from './i18n'
import { emptyConfig, type ViewConfig } from './view-config'

export interface BuiltinView {
  id: string // Sidebar active marker and stable key
  icon: string // Emoji marker
  name: string
  hint?: string
  config: ViewConfig
}

/** ISO date (YYYY-MM-DD) of this week's Monday 00:00 local. */
function startOfWeekISO(): string {
  const now = new Date()
  const day = (now.getDay() + 6) % 7 // Mon = 0
  const mon = new Date(now.getFullYear(), now.getMonth(), now.getDate() - day)
  return `${mon.getFullYear()}-${String(mon.getMonth() + 1).padStart(2, '0')}-${String(mon.getDate()).padStart(2, '0')}`
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
      icon: '📋',
      name: t('view.allOpen.name'),
      hint: t('view.allOpen.hint'),
      config: make({ filters: { status_category: ['new', 'inprogress'] } }),
    },
    {
      id: 'unassigned-new',
      icon: '🆕',
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
      icon: '🧭',
      name: t('view.epicBreakdown.name'),
      hint: t('view.epicBreakdown.hint'),
      config: make({
        filters: { status_category: ['new', 'inprogress'] },
        display: { group_by: 'epic' },
      }),
    },
    {
      id: 'reopened',
      icon: '🔁',
      name: t('view.reopened.name'),
      hint: t('view.reopened.hint'),
      config: make({
        filters: { reopened: true },
        display: { sort: 'reopen_count', dir: 'desc' },
      }),
    },
    {
      id: 'stale',
      icon: '⏳',
      name: t('view.stale.name'),
      hint: t('view.stale.hint'),
      config: make({ filters: { stale: true }, display: { sort: 'updated', dir: 'asc' } }),
    },
    {
      id: 'recently-updated',
      icon: '⚡',
      name: t('view.recentlyUpdated.name'),
      hint: t('view.recentlyUpdated.hint'),
      config: make({
        filters: { status_category: ['new', 'inprogress'] },
        display: { sort: 'updated', dir: 'desc' },
      }),
    },
    {
      id: 'resolved-week',
      icon: '✅',
      name: t('view.resolvedWeek.name'),
      hint: t('view.resolvedWeek.hint'),
      config: make({
        filters: { status_category: ['done'], updated_from: startOfWeekISO() },
        display: { sort: 'updated', dir: 'desc' },
      }),
    },
  ]
}
