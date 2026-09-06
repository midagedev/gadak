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
  /**
   * The two reading stances (THEORY.md "Two stances"): `mine` = contributor
   * (my issues — execution), `team` = steward (the team's issues —
   * supervision). The sidebar renders one label row per stance, mine first;
   * the first screen is a mine-stance view (G4).
   */
  stance: 'mine' | 'team'
  /**
   * True when the view's filters are identity flags: without an identified
   * account the list is empty, so the sidebar and palette hide the row
   * entirely (absent, not disabled — an anonymous reader has no "mine").
   */
  needsIdentity?: boolean
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
    /* ── mine stance: the contributor's views (G4: the first screen is
     * "my work"; unit = the issue, question = "what do I do now"). ── */
    {
      // The contributor's front door: open issues assigned to this account,
      // urgent first. Tenant-neutral by construction — the only constraint
      // beside the open categories is the `mine` flag, which person-match
      // evaluates per identity; nothing here names a status or priority.
      id: 'my-work',
      icon: 'user',
      name: t('view.myWork.name'),
      hint: t('view.myWork.hint'),
      stance: 'mine',
      needsIdentity: true,
      config: make({
        filters: { mine: true, status_category: ['inprogress', 'new'] },
        // Urgent first: priority_rank lower = more urgent, so asc is
        // urgent-first. Groups read in progress → new (IN_RANK).
        display: { group_by: 'status_category', sort: 'priority', dir: 'asc' },
      }),
    },
    {
      // The delegation ledger: issues this account reported and someone else
      // holds, quietest first — the waiting list reads by silence (T3).
      id: 'delegated',
      icon: 'arrow-up-right',
      name: t('view.delegated.name'),
      hint: t('view.delegated.hint'),
      stance: 'mine',
      needsIdentity: true,
      config: make({
        filters: { delegated: true, status_category: ['inprogress', 'new'] },
        display: { sort: 'updated', dir: 'asc' },
      }),
    },
    {
      id: 'all-open',
      icon: 'inbox',
      name: t('view.allOpen.name'),
      hint: t('view.allOpen.hint'),
      stance: 'mine',
      config: make({ filters: { status_category: ['new', 'inprogress'] } }),
    },
    {
      id: 'unassigned-new',
      icon: 'plus-circle',
      name: t('view.unassignedNew.name'),
      hint: t('view.unassignedNew.hint'),
      stance: 'mine',
      config: make({
        filters: { unassigned: true, status_category: ['new'] },
        display: { sort: 'created', dir: 'desc' },
      }),
    },
    {
      id: 'recently-updated',
      icon: 'zap',
      name: t('view.recentlyUpdated.name'),
      hint: t('view.recentlyUpdated.hint'),
      stance: 'mine',
      config: make({
        filters: { status_category: ['new', 'inprogress'] },
        display: { sort: 'updated', dir: 'desc' },
      }),
    },
    /* ── team stance: the steward's views (unit = the distribution; the
     * exception surfaces, not the list — THEORY.md "Two stances"). ── */
    {
      // Steward view (THEORY.md T4/G4): the aging tail of in-progress work,
      // longest underway first — the arrangement is the coaching, no
      // sentence. 'started' is work item age (since started_at, the flow
      // canon's clock — 2026-09-07); it replaced status_changed, which reset
      // at every hand-off inside progress, and before that the updated-at
      // proxy.
      id: 'aging-in-progress',
      icon: 'hourglass',
      name: t('view.agingInProgress.name'),
      hint: t('view.agingInProgress.hint'),
      stance: 'team',
      config: make({
        filters: { status_category: ['inprogress'] },
        display: { sort: 'started', dir: 'asc' },
      }),
    },
    {
      id: 'stale',
      icon: 'clock',
      name: t('view.stale.name'),
      hint: t('view.stale.hint'),
      stance: 'team',
      config: make({ filters: { stale: true }, display: { sort: 'updated', dir: 'asc' } }),
    },
    {
      id: 'reopened',
      icon: 'rotate-ccw',
      name: t('view.reopened.name'),
      hint: t('view.reopened.hint'),
      stance: 'team',
      config: make({
        filters: { reopened: true },
        display: { sort: 'reopen_count', dir: 'desc' },
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
      stance: 'team',
      config: make({
        filters: { status_category: ['new', 'inprogress'] },
        display: { group_by: 'epic' },
      }),
    },
    {
      id: 'resolved-week',
      icon: 'check-circle',
      name: t('view.resolvedWeek.name'),
      hint: t('view.resolvedWeek.hint'),
      stance: 'team',
      config: make({
        filters: { status_category: ['done'], resolved_from: startOfWeekISO() },
        display: { sort: 'updated', dir: 'desc' },
      }),
    },
  ]
}
