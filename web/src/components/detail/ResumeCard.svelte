<script lang="ts">
  /*
   * Resume card ([detail]) — one line under the header saying what changed
   * since this issue was last opened (spec w1-resume; THEORY.md T2/T3,
   * UX §14 G1/G3/G5).
   *
   * Rendered only when a previous visit exists AND something changed since:
   * no previous visit → no card, nothing changed → no card, and no empty
   * state — the absence is the design. All decisions live in
   * lib/resume-card.ts (pure, one pass, no fetch, no store reads).
   *
   * Form (C4): the duration-chip's classes on a native button — bg-elevated
   * chip, micro secondary text, no icon, no border, no new colour. Native
   * <button> gives role=button and Enter/Space activation. Click scrolls to
   * History; hover states the basis — the absolute time of the previous
   * visit (G7). One line by contract: the chip truncates with an ellipsis
   * rather than wrapping (vision FIX 2026-09-06 — a wrapped card was twice
   * the chips' height and read as a block, not a line).
   */
  import { t, relativeTime, absTime } from '../../lib/i18n'
  import { pickSince, resumeDelta, resumeLabel } from '../../lib/resume-card'
  import type { DetailResponse } from '../../lib/types'

  let { detail }: { detail: DetailResponse } = $props()

  const since = $derived(pickSince(detail.last_visited_at, detail.previous_visit_at))
  const delta = $derived(resumeDelta(detail, since))
  const label = $derived(
    delta && since ? resumeLabel(delta, relativeTime(since), t) : '',
  )

  function scrollToHistory(): void {
    document.getElementById('detail-history-section')?.scrollIntoView({ block: 'start' })
  }
</script>

{#if delta && since}
  <div class="px-5 pt-4">
    <button
      type="button"
      data-testid="resume-card"
      class="block max-w-full truncate rounded-md bg-bg-elevated px-2 py-0.5 text-left text-micro text-text-secondary hover:bg-bg-hover"
      title={absTime(since)}
      onclick={scrollToHistory}
    >{label}</button>
  </div>
{/if}
