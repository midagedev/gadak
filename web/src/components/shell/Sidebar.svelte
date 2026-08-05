<script lang="ts">
  /*
   * Left sidebar frame (desktop 272px, [foundation]).
   * Top logo + `children` snippet slot.
   * Nav content (My Issues / feed / recent / views) is built by [explore]·[personal]
   * under `components/sidebar/` and injected into this slot.
   */
  import type { Snippet } from 'svelte'

  let { children }: { children?: Snippet } = $props()
</script>

<aside
  class="issue-sidebar flex h-full flex-none flex-col border-r border-border-subtle bg-bg-panel"
>
  <!-- Logo wordmark (no header chrome — density first) -->
  <div class="flex h-12 flex-none items-center gap-2 px-4">
    <span
      class="inline-block h-2.5 w-2.5 rounded-[3px] bg-accent"
      aria-hidden="true"
    ></span>
    <span class="text-[13px] font-semibold tracking-tight text-text-primary">scry</span>
  </div>

  <!-- Navigation slot -->
  <nav class="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
    {@render children?.()}
  </nav>
</aside>
