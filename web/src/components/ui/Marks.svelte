<script lang="ts">
  /*
   * One clause's matched slices — the query drawn inside the text it was found
   * in. Every marked clause in the app renders through here, so a hit can never
   * end up drawn one way in a title and another in the line below it.
   *
   * No wrapper element: the segments land directly in whatever the caller's own
   * markup already is, which is what lets a truncating span stay the single
   * layout box it was before the highlight existed.
   *
   * A caller with nothing to mark (a chosung query matches no literal run)
   * passes the plain text instead — `highlightSegments` returns a single unhit
   * segment for that case, so it renders the same either way.
   */
  let { segs }: { segs: { text: string; hit: boolean }[] } = $props()
</script>

{#each segs as seg, i (i)}{#if seg.hit}<mark class="rounded-[2px] bg-status-stale/30 text-inherit">{seg.text}</mark>{:else}{seg.text}{/if}{/each}
