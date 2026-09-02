<script lang="ts">
  /*
   * One SidebarNav section: collapse toggle, HTML5 drag reorder, Alt+↑/↓.
   * Header chrome matches the old static label (text-micro uppercase muted)
   * plus the docs-tree chevron rotate already used in this rail.
   */
  import type { Snippet } from 'svelte'
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import {
    isSectionId,
    SECTION_DND_TYPE,
    sidebarSections,
    type SectionDrag,
    type SectionId,
  } from '../../stores/sidebar-sections.svelte'

  let {
    id,
    label,
    testid,
    visibleIds,
    drag,
    children,
  }: {
    id: SectionId
    label: string
    testid?: string
    visibleIds: readonly SectionId[]
    drag: SectionDrag
    children: Snippet
  } = $props()

  const expanded = $derived(!sidebarSections.collapsedIds.includes(id))
  const bodyId = $derived(`sidebar-section-body-${id}`)
  const dragging = $derived(drag.draggingId === id)
  const dropTarget = $derived(drag.dropTargetId === id)

  let suppressClick = false

  function onToggle() {
    if (suppressClick) {
      suppressClick = false
      return
    }
    sidebarSections.toggle(id)
  }

  function onKeydown(e: KeyboardEvent) {
    if (!e.altKey) return
    if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
    e.preventDefault()
    sidebarSections.move(id, e.key === 'ArrowDown' ? 1 : -1, visibleIds)
  }

  function onDragStart(e: DragEvent) {
    if (!e.dataTransfer) return
    e.dataTransfer.setData('text/plain', id)
    e.dataTransfer.setData(SECTION_DND_TYPE, id)
    e.dataTransfer.effectAllowed = 'move'
    drag.start(id)
    suppressClick = true
  }

  function onDragEnd() {
    drag.clear()
    window.setTimeout(() => {
      suppressClick = false
    }, 0)
  }

  function onDragOver(e: DragEvent) {
    const source = drag.draggingId
    if (!source || source === id) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    drag.hover(id)
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    const raw =
      e.dataTransfer?.getData(SECTION_DND_TYPE) || e.dataTransfer?.getData('text/plain') || ''
    drag.clear()
    if (!isSectionId(raw) || raw === id) return
    sidebarSections.reorder(raw, id)
  }
</script>

<div
  role="listitem"
  data-section={id}
  data-testid={testid ?? `sidebar-section-${id}`}
  class="mb-2 {dropTarget
    ? 'shadow-[inset_0_2px_0_var(--color-accent)]'
    : ''} {dragging ? 'opacity-50' : ''}"
  ondragover={onDragOver}
  ondrop={onDrop}
>
  <button
    type="button"
    class="group flex h-6 w-full cursor-grab items-center gap-1 px-3 text-left text-micro font-medium uppercase tracking-wide text-text-muted hover:text-text-secondary focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent active:cursor-grabbing"
    aria-expanded={expanded}
    aria-controls={bodyId}
    aria-keyshortcuts="Alt+ArrowUp Alt+ArrowDown"
    title={t('sidebar.sectionReorderHint')}
    data-testid={`sidebar-section-header-${id}`}
    draggable="true"
    onclick={onToggle}
    onkeydown={onKeydown}
    ondragstart={onDragStart}
    ondragover={onDragOver}
    ondrop={onDrop}
    ondragend={onDragEnd}
  >
    <span
      class="flex-none text-text-muted transition-transform duration-150 {expanded
        ? 'rotate-90'
        : ''}"
      aria-hidden="true"
    >
      <Icon name="chevron-right" size={12} />
    </span>
    <span class="min-w-0 flex-1 truncate">{label}</span>
    <span
      class="flex-none text-text-muted opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100"
      data-testid="sidebar-section-grip"
      aria-hidden="true"
    >
      <Icon name="grip" size={12} />
    </span>
  </button>
  <div id={bodyId} data-testid={`sidebar-section-body-${id}`} hidden={!expanded}>
    {#if expanded}
      {@render children()}
    {/if}
  </div>
</div>
