<script lang="ts">
  /*
   * Left sidebar frame (desktop 272px, [foundation]).
   * Top logo + `children` snippet slot.
   * Nav content (My Issues / feed / recent / views) is built by [explore]·[personal]
   * under `components/sidebar/` and injected into this slot.
   */
  import type { Snippet } from 'svelte'
  import { isDesktop, sidebarLogoRowClass } from '../../lib/config'
  import BrandMark from '../ui/BrandMark.svelte'

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
    <span class="type-subject text-[18px] leading-none text-text-primary">gadak</span>
  </div>

  <!-- Navigation slot. This nav is the named scroll-region frame; the
       overflowing leaf (and the spacer) is the child .scroll-region.
       overflow-y-auto here was a second clip that never scrolled — the
       child is h-full. pb-3 is inset under the pinned footer, not
       last-row breathing room. -->
  <nav class="scroll-region min-h-0 flex-1 px-2 pb-3">
    {@render children?.()}
  </nav>
</aside>
