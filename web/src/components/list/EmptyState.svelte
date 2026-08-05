<script lang="ts">
  /*
   * Empty state ([explore]). Clean copy for 0 filter/search hits + optional reset.
   */
  let {
    icon = '🔍',
    title,
    hint = '',
    actionLabel = '',
    onAction,
  }: {
    icon?: string
    title: string
    hint?: string
    actionLabel?: string
    onAction?: () => void
  } = $props()
</script>

<div class="flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
  <span class="text-2xl opacity-60" aria-hidden="true">{icon}</span>
  <span class="text-[13px] font-medium text-text-secondary">{title}</span>
  {#if hint}<span class="max-w-xs text-[12px] text-text-muted">{hint}</span>{/if}
  {#if actionLabel && onAction}
    <button
      type="button"
      class="mt-1 rounded-md border border-border-strong px-3 py-1 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
      data-testid="empty-state-action"
      onclick={onAction}
    >
      {actionLabel}
    </button>
  {/if}
</div>
