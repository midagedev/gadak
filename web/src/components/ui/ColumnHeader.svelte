<script lang="ts">
  /*
   * The band at the top of a main-column screen that is not the issue list —
   * Documents, a space, History, a dashboard. One component so the four read
   * as the same row the list toolbar is (GDK-1339): same height, same
   * border, same tinted ground, same 12px inset. Before this each screen
   * carried its own <header> with a hairline border on the panel colour, so
   * switching from the list to Documents moved the top edge and lost the tint.
   *
   * Left to right: title · count · whatever the screen adds (tabs, a chip,
   * an action) · the narrowing field on the right · the way back. The screen
   * owns its middle and its field; this owns the frame and the back button.
   */
  import type { Snippet } from 'svelte'
  import Icon, { type IconName } from './Icon.svelte'
  import { t } from '../../lib/i18n'

  let {
    title,
    titleAttr,
    icon,
    count,
    countTestid,
    closeTestid,
    onClose,
    children,
    trailing,
  }: {
    title: string
    /** Hover text when the visible title may be an abbreviation. */
    titleAttr?: string
    icon?: IconName
    /** Already formatted — a fraction while a filter is on. */
    count?: string
    countTestid?: string
    closeTestid: string
    onClose: () => void
    children?: Snippet
    trailing?: Snippet
  } = $props()
</script>

<header
  class="flex flex-none flex-wrap items-center gap-2 border-b border-border-strong/70 bg-bg-panel/35 px-3 py-2"
  data-testid="column-header"
>
  {#if icon}
    <Icon name={icon} size={14} class="flex-none text-text-muted" />
  {/if}
  <h2 class="min-w-0 truncate text-body font-medium text-text-primary" title={titleAttr ?? title}>
    {title}
  </h2>
  {#if count !== undefined}
    <span class="flex-none text-micro tabular-nums text-text-muted" data-testid={countTestid}>
      {count}
    </span>
  {/if}
  {@render children?.()}
  <!-- The right edge: the screen's narrowing field or its one bulk action,
       then the way back. One flex-1 group rather than auto margins on each,
       because two auto margins split the free space between them and the
       field would float mid-band. -->
  <div class="ml-auto flex min-w-0 flex-1 items-center justify-end gap-2">
    {@render trailing?.()}
    <button
      type="button"
      class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded-md text-text-secondary hover:bg-bg-hover hover:text-text-primary"
      onclick={onClose}
      title={t('feed.backToList')}
      aria-label={t('feed.backToList')}
      data-testid={closeTestid}
    >
      <Icon name="arrow-left" size={15} />
    </button>
  </div>
</header>
