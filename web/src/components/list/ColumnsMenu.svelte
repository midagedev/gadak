<script lang="ts">
  /*
   * 컬럼 메뉴 ([explore]). 리스트 행에 노출할 후행 필드를 on/off 한다.
   *  구성은 뷰의 display 에 포함돼 URL·저장 뷰에 함께 직렬화된다(뷰별 컬럼).
   */
  import { filters } from '../../stores/filters.svelte'
  import { columnCatalog, defaultColumns, type ColumnKey } from '../../lib/view-config'

  const catalog = columnCatalog()
  const defaults = defaultColumns()
  const active = $derived(new Set<ColumnKey>(filters.display.columns))
  const isDefault = $derived(
    active.size === defaults.length && defaults.every((k) => active.has(k)),
  )

  let open = $state(false)
  let rootEl = $state<HTMLDivElement | null>(null)

  function onDocClick(e: MouseEvent) {
    if (rootEl && !rootEl.contains(e.target as Node)) open = false
  }
</script>

<svelte:document onclick={onDocClick} />

<div bind:this={rootEl} class="relative">
  <button
    type="button"
    class="inline-flex items-center gap-1.5 rounded-md border border-border-strong/70 bg-bg-elevated px-2.5 py-1.5 text-[12px] text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary"
    onclick={() => (open = !open)}
    title="표시할 컬럼 선택"
  >
    <span>컬럼</span>
    {#if !isDefault}
      <span class="rounded bg-accent-subtle/70 px-1 text-[10px] text-accent-text">{active.size}</span>
    {/if}
  </button>

  {#if open}
    <div
      class="anim-enter absolute right-0 top-full z-30 mt-1 w-52 rounded-lg border border-border-strong bg-bg-elevated p-2 shadow-xl shadow-black/40"
    >
      <div class="mb-1 flex items-center justify-between">
        <span class="text-[11px] font-medium text-text-muted">노출 컬럼</span>
        <button
          type="button"
          class="rounded px-1.5 py-0.5 text-[11px] text-text-secondary transition-colors hover:bg-bg-hover disabled:opacity-40"
          onclick={() => filters.resetColumns()}
          disabled={isDefault}
          title="기본 컬럼으로 되돌리기"
        >
          기본값
        </button>
      </div>
      <div class="max-h-[60vh] overflow-y-auto">
        {#each catalog as col (col.key)}
          <button
            type="button"
            class="flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-[12px] transition-colors hover:bg-bg-hover"
            onclick={() => filters.toggleColumn(col.key)}
            aria-pressed={active.has(col.key)}
          >
            <span
              class="flex h-3.5 w-3.5 flex-none items-center justify-center rounded border transition-colors {active.has(
                col.key,
              )
                ? 'border-accent bg-accent text-white'
                : 'border-border-strong'}"
            >
              {#if active.has(col.key)}<span class="text-[8px]">✓</span>{/if}
            </span>
            <span class={active.has(col.key) ? 'text-text-primary' : 'text-text-secondary'}>
              {col.label}
            </span>
          </button>
        {/each}
      </div>
    </div>
  {/if}
</div>
