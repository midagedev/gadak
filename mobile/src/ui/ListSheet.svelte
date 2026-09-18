<script lang="ts">
  import Sheet from './Sheet.svelte'
  import CurrentTick from './detail/CurrentTick.svelte'
  import { t } from '../lib/i18n'
  import { narrowCount, narrowHas, type NarrowRow, type NarrowSection } from '../lib/domain'
  import { GROUPABLE_ON_LITE, type LiteGroupBy } from '../../../web/src/lib/issue-group'
  import { LIST_SORT_KEYS, type ListOrder, type ListSortKey } from '../../../web/src/lib/issue-sort'
  import { fieldLabel } from '../lib/i18n'
  import type { ViewFilters } from '../lib/types'

  /*
   * "This list" (GDK-1994) — the chevron's own body.
   *
   * The heading and the header magnifier used to open one panel that differed
   * by a single bit (whether the field took focus), which is two doors into
   * one room. They part here: the magnifier still opens the palette, which is
   * for finding one issue in the whole snapshot, and the chevron opens this,
   * which is about the list already on screen. The scope row at the top is the
   * road between them — the picker is still one tap from the heading, it just
   * arrives as a choice rather than as the only thing behind the glyph.
   *
   * The shape is iOS Mail's filter, cited in DESIGN.md §3.1 and now paid off:
   * toggle rows, live behind the scrim, no apply button, no chips, no save.
   * Nothing here authors a view — that is the desk's (GDK-1875) — and nothing
   * here goes to the origin. The rows are discovered by `narrowFacets`, so a
   * toggle that could not change the list never renders.
   */
  let {
    scopeName,
    total,
    facets,
    narrow,
    groupBy,
    order,
    ontoggle,
    ongroup,
    onorder,
    onclear,
    onchangeview,
    onclose,
  }: {
    scopeName: string
    /** Rows on screen right now — the narrowed count, the heading's number. */
    total: number
    facets: NarrowSection[]
    narrow: Partial<ViewFilters>
    /** The cut and the order actually painted — the view's, or the session's. */
    groupBy: LiteGroupBy
    order: ListOrder
    ontoggle: (row: NarrowRow) => void
    ongroup: (by: LiteGroupBy) => void
    onorder: (order: ListOrder) => void
    onclear: () => void
    onchangeview: () => void
    onclose: () => void
  } = $props()

  const narrowed = $derived(narrowCount(narrow) > 0)

  /*
   * The cut and the order are the desk's two view-settings controls, brought
   * over as rows (GDK-1993). Every word is the desk's: axes that are fields
   * wear `fieldLabel` — the same owner the desk's own breakdown bar reads, so
   * "Category" here and "Category" there cannot drift — and only the two with
   * no field twin carry a key of their own.
   */
  const GROUP_LABEL: Record<LiteGroupBy, () => string> = {
    none: () => t('group.sectionNone'),
    status_category: () => fieldLabel('status_category'),
    status: () => fieldLabel('status'),
    assignee: () => fieldLabel('assignee'),
    priority: () => fieldLabel('priority'),
    issue_type: () => fieldLabel('issue_type'),
    source_project: () => fieldLabel('source_project'),
    epic: () => t('group.byEpic'),
  }
  const SORT_LABEL: Record<ListSortKey, () => string> = {
    updated: () => t('sort.updated'),
    created: () => t('sort.created'),
    status_changed: () => t('sort.statusChanged'),
    started: () => t('sort.started'),
    due: () => t('sort.due'),
    priority: () => t('sort.priority'),
    reopen_count: () => t('sort.reopenCount'),
  }
</script>

<Sheet title={t('app.narrowTitle')} {onclose}>
  <div class="pick-list">
    <!-- The view this list belongs to, and the door to the picker. It reads
         as a row rather than a heading because it is a control: the name is
         the current answer and the chevron says it can change. -->
    <button class="t-row scope-row" onclick={onchangeview}>
      <span class="t-text">
        <span class="t-name">{scopeName}</span>
        <span class="t-sub">{t('app.narrowChangeView')}</span>
      </span>
      <span class="count">·{total}</span>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="m9 18 6-6-6-6" />
      </svg>
    </button>

    {#each facets as section (section.id)}
      <p class="axis">{section.label}</p>
      {#each section.rows as row (row.key)}
        {@const on = narrowHas(narrow, row)}
        <button class="t-row" class:current={on} aria-pressed={on} onclick={() => ontoggle(row)}>
          <span class="t-text"><span class="t-name">{row.label}</span></span>
          <span class="count">·{row.count}</span>
          {#if on}<CurrentTick />{/if}
        </button>
      {/each}
    {/each}

    {#if facets.length === 0}
      <p class="none">{t('app.narrowNothing')}</p>
    {/if}

    <p class="axis">{t('group.breakdown')}</p>
    {#each GROUPABLE_ON_LITE as by (by)}
      {@const on = groupBy === by}
      <button class="t-row" class:current={on} aria-current={on ? 'true' : undefined} onclick={() => ongroup(by)}>
        <span class="t-text"><span class="t-name">{GROUP_LABEL[by]()}</span></span>
        {#if on}<CurrentTick />{/if}
      </button>
    {/each}

    <p class="axis">{t('sort.label')}</p>
    {#each LIST_SORT_KEYS as key (key)}
      {@const on = order.sort === key}
      <button
        class="t-row"
        class:current={on}
        aria-current={on ? 'true' : undefined}
        onclick={() => onorder({ sort: key, dir: order.dir })}
      >
        <span class="t-text"><span class="t-name">{SORT_LABEL[key]()}</span></span>
        {#if on}<CurrentTick />{/if}
      </button>
    {/each}
    <!-- Direction is the sort's property, not a ninth key, so it is one row
         under the list rather than a section of its own. -->
    <button
      class="t-row"
      data-testid="sort-direction"
      onclick={() => onorder({ sort: order.sort, dir: order.dir === 'desc' ? 'asc' : 'desc' })}
    >
      <span class="t-text"><span class="t-name">{t('sort.direction')}</span></span>
      <span class="count">{order.dir === 'desc' ? t('sort.desc') : t('sort.asc')}</span>
    </button>

    {#if narrowed}
      <div class="sheet-foot">
        <button class="clear" onclick={onclear}>{t('filter.clear')}</button>
      </div>
    {/if}
  </div>
</Sheet>

<style>
  /* also in the pick sheets — same tokens, GDK-1925 */
  .pick-list {
    overflow-y: auto;
    padding: 4px 8px 8px;
    display: flex;
    flex-direction: column;
  }
  /* also in the pick sheets — same tokens, GDK-1925 */
  .t-row {
    display: flex;
    width: 100%;
    align-items: center;
    gap: 10px;
    min-height: var(--spacing-control);
    padding: 6px 8px;
    border-radius: 6px;
    text-align: left;
  }
  .t-row:active {
    background: var(--color-bg-hover);
  }
  /* also in the pick sheets — same tokens, GDK-1925 */
  .t-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1;
  }
  /* also in the pick sheets — same tokens, GDK-1925 */
  .t-name {
    color: var(--color-text-primary);
    font-weight: 600;
  }
  .t-sub {
    font-size: var(--text-micro);
    font-weight: 400;
    color: var(--color-text-muted);
  }
  .scope-row {
    border-bottom: 1px solid var(--color-border-subtle);
    border-radius: 6px 6px 0 0;
    margin-bottom: 4px;
  }
  .scope-row svg {
    flex: none;
    width: 16px;
    height: 16px;
    color: var(--color-text-muted);
  }
  /* The count is the row's evidence, not its name: same muted weight as the
     picker's, so a long label never competes with it for the eye. */
  .count {
    flex: none;
    font-size: var(--text-micro);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-muted);
  }
  .axis {
    margin: 10px 0 2px;
    padding: 0 8px;
    font-size: var(--text-micro);
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }
  /* also in the pick sheets — same tokens, GDK-1925 */
  .none {
    margin: 4px 0 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  /* also in LabelsSheet — same tokens, GDK-1925. Sticky inside .pick-list's
     own scroll so the clear stays reachable under a long facet list. */
  .sheet-foot {
    position: sticky;
    bottom: 0;
    display: flex;
    background: var(--color-bg-panel);
    padding: 8px 8px 0;
    border-top: 1px solid var(--color-border-subtle);
  }
  .clear {
    min-height: var(--spacing-control);
    padding: 0 16px;
    border-radius: 6px;
    font-weight: 600;
    background: var(--color-bg-elevated);
    color: var(--color-text-primary);
  }
</style>
