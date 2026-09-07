<script lang="ts">
  import Sheet from './Sheet.svelte'
  import { t } from '../lib/i18n'
  import type { Scope, ScopeSection } from '../lib/domain'

  // The scope picker (GDK-885). Apple Mail's Mailboxes list, not a chip
  // strip: one list screen whose heading is the current scope's name, and
  // this sheet is where that name changes. Every section heading and both
  // hardcoded scope names come from the desktop catalog — the phone owns
  // none of the vocabulary here (DESIGN.md §3.6).
  let {
    scopes,
    current,
    counts,
    onpick,
    onclose,
  }: {
    scopes: Scope[]
    current: string
    /** Match count per scope id; null = the phone refuses this view. */
    counts: Map<string, number | null>
    onpick: (id: string) => void
    onclose: () => void
  } = $props()

  /*
   * Inside the built-in section the desk draws two stance rows before their
   * views (THEORY.md "Two stances", SidebarNav): what do I do now, and what
   * is the team's flow. The phone wears the same two, from the same keys —
   * without them the picker runs five unrelated names together right under
   * a heading that is itself a view's name (GDK-1495 ④).
   *
   * "Assigned to me" is inside that section too, first in the `mine` stance
   * (vision FIX 2026-09-07 — buildScopes owns the placement). It had a
   * heading of its own above the built-ins, which split one set into two
   * groups for the reader.
   */
  const STANCE: Record<'mine' | 'team', string> = {
    mine: 'sidebar.stanceMine',
    team: 'sidebar.stanceTeam',
  }

  const ORDER: ScopeSection[] = ['builtin', 'views', 'filters', 'docs']
  const HEADING: Record<ScopeSection, string> = {
    builtin: 'sidebar.builtinViews',
    views: 'sidebar.myViews',
    filters: 'sidebar.jiraFilters',
    docs: 'sidebar.docs',
  }

  // UX_PRINCIPLES §6: a list in a sheet is capped, not infinite. A desk with
  // forty saved views shows eight and one way to see the rest.
  const CAP = 8
  let expanded = $state(new Set<ScopeSection>())

  const groups = $derived(
    ORDER.map((section) => ({
      section,
      heading: t(HEADING[section] as Parameters<typeof t>[0]),
      rows: scopes.filter((s) => s.section === section),
    })).filter((g) => g.rows.length > 0),
  )

  function shown(section: ScopeSection, rows: Scope[]): Scope[] {
    return expanded.has(section) ? rows : rows.slice(0, CAP)
  }

  function expand(section: ScopeSection): void {
    const next = new Set(expanded)
    next.add(section)
    expanded = next
  }
</script>

<Sheet title={t('doc.issues')} {onclose}>
  <div class="list">
    {#each groups as group (group.section)}
      <div class="section">{group.heading}</div>
      {#each shown(group.section, group.rows) as scope, i (scope.id)}
        {@const blocked = scope.unsupported.length > 0}
        {@const n = counts.get(scope.id) ?? null}
        {#if scope.stance && scope.stance !== group.rows[i - 1]?.stance}
          <div class="stance">{t(STANCE[scope.stance] as Parameters<typeof t>[0])}</div>
        {/if}
        <button
          class="row"
          class:on={scope.id === current}
          disabled={blocked}
          aria-current={scope.id === current ? 'true' : undefined}
          onclick={() => onpick(scope.id)}
        >
          <span class="name">{scope.name}</span>
          {#if blocked}
            <span class="why">Open on the desktop</span>
          {:else if n !== null}
            <span class="n">{n}</span>
          {/if}
        </button>
      {/each}
      {#if group.rows.length > CAP && !expanded.has(group.section)}
        <button class="more" onclick={() => expand(group.section)}>
          Show all {group.rows.length}
        </button>
      {/if}
    {/each}
  </div>
</Sheet>

<style>
  .list {
    overflow-y: auto;
    padding: 0 8px 8px;
  }
  .section {
    padding: 10px 8px 4px;
    font-size: var(--text-micro);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }
  .stance {
    padding: 8px 8px 2px;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .row {
    display: flex;
    width: 100%;
    align-items: center;
    gap: 10px;
    padding: 6px 8px;
    border-radius: 6px;
    text-align: left;
    min-width: 0;
  }
  .row:active:not(:disabled) {
    background: var(--color-bg-hover);
  }
  .row:disabled {
    opacity: 0.5;
  }
  .row.on .name {
    font-weight: 600;
    color: var(--color-accent-text);
  }
  .name {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
  }
  .n {
    flex: none;
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .why {
    flex: none;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .more {
    display: flex;
    align-items: center;
    padding: 0 8px;
    color: var(--color-accent-text);
    font-size: var(--text-micro);
  }
</style>
