<script lang="ts">
  /*
   * Visual shell for the six same-class modal dialogs (GDK-316).
   * Owns backdrop, panel chrome, header (title + X), optional footer.
   * Focus-trap is passed in so each dialog keeps the call; Esc stays
   * with each dialog (see lib/focus-trap.ts).
   */
  import type { Snippet } from 'svelte'
  import { t } from '../../lib/i18n'
  import Icon from './Icon.svelte'

  type Trap = (node: HTMLElement) => { destroy?: () => void }

  let {
    onclose,
    title,
    titleLead,
    headerExtra,
    headerClass = 'flex flex-none flex-col border-b border-border-subtle px-5 py-3',
    titleRowClass = 'flex items-center justify-between',
    ariaLabel,
    panelClass = '',
    backdropClass = 'items-center p-4',
    children,
    footer,
    footerClass = 'flex flex-none items-center gap-2 border-t border-border-subtle px-5 py-3',
    asForm = false,
    onSubmit,
    trap,
    ...rest
  }: {
    onclose: () => void
    title?: string
    titleLead?: Snippet
    headerExtra?: Snippet
    headerClass?: string
    titleRowClass?: string
    ariaLabel: string
    panelClass?: string
    backdropClass?: string
    children?: Snippet
    footer?: Snippet
    footerClass?: string
    asForm?: boolean
    onSubmit?: (e: Event) => void
    trap: Trap
    [key: string]: unknown
  } = $props()

  const BACKDROP =
    'fixed inset-0 z-50 flex justify-center bg-[#1c1812]/28 backdrop-blur-[2px]'
  const PANEL =
    'flex w-full flex-col overflow-hidden rounded-lg border border-border-strong bg-bg-panel shadow-overlay'
</script>

<div
  class="{BACKDROP} {backdropClass}"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) onclose()
  }}
>
  <div
    use:trap
    class="{panelClass} {PANEL}"
    role="dialog"
    aria-modal="true"
    aria-label={ariaLabel}
    {...rest}
  >
    <div class={headerClass}>
      <div class={titleRowClass}>
        {#if titleLead}
          {@render titleLead()}
        {:else}
          <h2 class="type-subject text-heading leading-snug text-text-primary">{title}</h2>
        {/if}
        <button
          type="button"
          class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
          onclick={onclose}
          aria-label={t('common.closeEsc')}
          title={t('common.closeEsc')}
        >
          <Icon name="x" size={14} />
        </button>
      </div>
      {#if headerExtra}
        {@render headerExtra()}
      {/if}
    </div>

    {#if asForm}
      <form onsubmit={onSubmit} class="flex min-h-0 flex-1 flex-col">
        {@render children?.()}
        {#if footer}
          <div class={footerClass} data-dialog-footer>
            {@render footer()}
          </div>
        {/if}
      </form>
    {:else}
      {@render children?.()}
      {#if footer}
        <div class={footerClass} data-dialog-footer>
          {@render footer()}
        </div>
      {/if}
    {/if}
  </div>
</div>
