<script lang="ts">
  /*
   * Detail panel section wrapper ([detail]).
   * Separates with spacing + section title (optional count), not borders (contract §3).
   *
   * Spacing scale for everything laid out inside a detail panel — 4 / 8 / 12 /
   * 16 / 20, and nothing between those. The panels had drifted onto half-steps
   * (10px section gaps, 6px chip rows, 2px stacks) that read as sloppiness
   * rather than as rhythm, because a 10 next to a 12 is not a distinction
   * anyone perceives; it is just two numbers. The gutter is 20 and a section's
   * vertical padding is 16, so a panel is one column with one rhythm.
   *
   * Two things are deliberately outside the scale: padding *inside* a control
   * (chips, buttons — that is the control-geometry contract, not this one), and
   * offsets derived from another element's size, like the 28px that lines a
   * comment's continuation up with the text beside its 20px avatar.
   */
  import type { Snippet } from 'svelte'

  let {
    title,
    count = undefined,
    id = undefined,
    children,
  }: {
    title: string
    count?: number | undefined
    /** Stable anchor for scroll targets (the resume card → History). */
    id?: string | undefined
    children?: Snippet
  } = $props()
</script>

<section id={id} class="px-5 py-4">
  <h3 class="mb-3 flex items-center gap-2 text-micro font-medium uppercase tracking-wide text-text-muted">
    <span class="flex-none">{title}</span>
    {#if count !== undefined}
      <span class="flex-none text-micro tabular-nums text-text-muted">{count}</span>
    {/if}
    <span class="h-px flex-1 bg-border-subtle"></span>
  </h3>
  {@render children?.()}
</section>
