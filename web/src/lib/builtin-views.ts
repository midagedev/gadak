/*
 * Built-in views ([explore])
 *
 * Presets shown under the sidebar "Built-in views" section. Each holds a full
 * ViewConfig (filters + display) and is applied wholesale via filters.applyConfig.
 *
 * Five built-ins in two stances (THEORY.md "Two stances"): mine holds the
 * contributor's two questions — what do I do now (My issues), what am I
 * waiting on (Handed off); team holds the pool (All open), the intake
 * (Unassigned new) and the one exception surface no Jira can show on its own
 * (Reopened). Two subtractions on 2026-09-07 took ten down to five (GDK-1493):
 * recently-updated was all-open under the default sort, stale was a flag over
 * the in-progress pool, resolved-week was the retro's closed cell, aging
 * in-progress was the in-progress pool with a sort (its count was the board
 * column's, its name promised a subset), and the epic breakdown was a layout
 * of the open pool that only means something on a site that uses a hierarchy.
 * The rule that decided: a built-in earns its row by being impactful on every
 * site regardless of that site's conventions; everything else is a view the
 * user saves (`gadak views save`, Jira filters).
 *
 * Discipline: presets use **tenant-neutral axes only**.
 *  - status_category (new/inprogress/done), unassigned, reopened, stale, resolved_from
 *    mean the same thing on every Jira.
 *  - status names, priority names, issue_type names, project keys vary by site
 *    (and even by account language), so baking them into a preset yields empty
 *    results elsewhere. Users save those as personal/team views themselves.
 *
 * Exposed as a function so names and hints resolve in the current locale at
 * call time.
 */

import { t } from './i18n'
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
    /* ── mine stance: the contributor's two questions (G4: the first screen
     * is "my work"; unit = the issue) — what do I do now, and what am I
     * waiting on. ── */
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
    /* ── team stance: the steward's views (unit = the distribution; the
     * exception surfaces, not the list — THEORY.md "Two stances"). ── */
    {
      // Team-side since 2026-09-07: the whole open pool is the team's, not
      // mine — this is the steward's ground floor.
      id: 'all-open',
      icon: 'inbox',
      name: t('view.allOpen.name'),
      hint: t('view.allOpen.hint'),
      stance: 'team',
      config: make({ filters: { status_category: ['new', 'inprogress'] } }),
    },
    {
      // Team-side since 2026-09-07: intake is a team surface — an unassigned
      // issue is nobody's yet.
      id: 'unassigned-new',
      icon: 'plus-circle',
      name: t('view.unassignedNew.name'),
      hint: t('view.unassignedNew.hint'),
      stance: 'team',
      config: make({
        filters: { unassigned: true, status_category: ['new'] },
        display: { sort: 'created', dir: 'desc' },
      }),
    },
    {
      // The one exception surface no Jira shows on its own: reopen_count is
      // derived from the changelog while syncing, so this view exists only
      // here — and means the same thing on every site.
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
  ]
}
