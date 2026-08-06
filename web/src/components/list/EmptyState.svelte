<script lang="ts">
  /*
   * Empty state ([explore]). Clean copy for 0 filter/search hits + optional reset.
   * An empty `icon` is a deliberate choice, not a missing value — some callers
   * carry their own heading above and want the glyph out of the way.
   */
  import Icon, { type IconName } from '../ui/Icon.svelte'

  let {
    icon = 'search',
    title,
    hint = '',
    actionLabel = '',
    onAction,
  }: {
    icon?: IconName | ''
    title: string
    hint?: string
    actionLabel?: string
    onAction?: () => void
  } = $props()
</script>

<div class="flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
  {#if icon}
    <Icon name={icon} size={22} class="mb-1 text-text-muted opacity-70" />
  {/if}
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
