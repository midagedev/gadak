<script lang="ts">
  /*
   * Left sidebar frame (desktop 272px, [foundation]).
   * Top logo + `children` snippet slot.
   * Nav content (My Issues / feed / recent / views) is built by [explore]·[personal]
   * under `components/sidebar/` and injected into this slot.
   */
  import type { Snippet } from 'svelte'
  import { isDesktop, sidebarLogoRowClass } from '../../lib/config'
  import { t } from '../../lib/i18n'
  import { terminalChrome } from '../../lib/terminal/pane.svelte'
  import BrandMark from '../ui/BrandMark.svelte'
  import Icon from '../ui/Icon.svelte'

  let { children }: { children?: Snippet } = $props()
  /** Desktop app: BrandMark is omitted so it is not a fourth traffic light. */
  const desktop = isDesktop()
  /** Traffic lights in this row (not merely "we are the desktop app"). */
  const logoRowClass = sidebarLogoRowClass()
</script>

<aside
  class="issue-sidebar flex h-full flex-none flex-col border-r border-border-subtle bg-bg-panel"
>
  <!-- Logo wordmark (no header chrome — density first). When traffic lights
       sit in the content this row reserves their corner and is a drag handle. -->
  <div
    class="flex h-12 flex-none items-center gap-2 {logoRowClass}"
    data-testid="sidebar-logo-row"
  >
    {#if !desktop}
      <!-- Dropped next to the window controls in the app: a second small mark
           beside three 12px circles reads as a fourth traffic light. The Dock
           already says Gadak. -->
      <BrandMark size={18} class="text-accent" data-testid="sidebar-mark" />
    {/if}
    <span class="type-subject wordmark leading-none text-text-primary">gadak</span>
  </div>

  <!-- Navigation slot. This nav is the named scroll-region frame; the
       overflowing leaf (and the spacer) is the child .scroll-region.
       overflow-y-auto here was a second clip that never scrolled — the
       child is h-full. pb-3 is inset under the pinned footer, not
       last-row breathing room. -->
  <nav class="scroll-region min-h-0 flex-1 px-2 pb-3">
    {@render children?.()}
  </nav>

  <div class="flex-none border-t border-border-subtle px-2 py-2">
    <button
      type="button"
      class="flex h-control w-full items-center gap-1.5 rounded-md px-3 text-left text-body transition-colors {terminalChrome.open
        ? 'bg-bg-active text-text-primary'
        : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
      aria-pressed={terminalChrome.open}
      title="{t('sidebar.terminal')} ({t('terminal.shortcut')})"
      data-testid="sidebar-terminal"
      onclick={() => terminalChrome.toggle()}
    >
      <Icon name="terminal" size={14} class="flex-none" />
      <span class="min-w-0 flex-1 truncate">{t('sidebar.terminal')}</span>
    </button>
  </div>
</aside>
