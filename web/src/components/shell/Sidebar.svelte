<script lang="ts">
  /*
   * Left sidebar frame (desktop 272px, [foundation]).
   * Top logo + `children` snippet slot.
   * Nav content (My Issues / feed / recent / views) is built by [explore]·[personal]
   * under `components/sidebar/` and injected into this slot.
   */
  import type { Snippet } from 'svelte'
  import { isDesktop } from '../../lib/config'

  let { children }: { children?: Snippet } = $props()
  /** Desktop app: the hidden title bar puts the window controls in this row. */
  const desktop = isDesktop()
</script>

<aside
  class="issue-sidebar flex h-full flex-none flex-col border-r border-border-subtle bg-bg-panel"
>
  <!-- Logo wordmark (no header chrome — density first). In the desktop app this
       row doubles as the title bar: it holds the window controls and drags the
       window. -->
  <div
    class="flex h-12 flex-none items-center gap-2 {desktop ? 'desktop-titlebar-row' : 'px-4'}"
    data-testid="sidebar-logo-row"
  >
    <span
      class="inline-block h-2.5 w-2.5 rounded-[3px] bg-accent"
      aria-hidden="true"
    ></span>
    <span class="text-body font-semibold tracking-tight text-text-primary">scry</span>
  </div>

  <!-- Navigation slot -->
  <nav class="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
    {@render children?.()}
  </nav>
</aside>
