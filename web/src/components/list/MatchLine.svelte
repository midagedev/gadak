<script lang="ts">
  /*
   * Why a search hit came back — one clipped line of the text the match was
   * found in, with the query highlighted exactly as the row above highlights a
   * title. Comment matches say so out loud: comment search has always worked,
   * but a comment hit looked identical to a title hit, so it read as
   * unsupported.
   *
   * Muted throughout, never accent (accent is for what is yours or actionable).
   * The line is the row's evidence, not a second row.
   *
   * One text column, one left edge. The speech-bubble icon hangs in a fixed
   * gutter that body hits leave empty, and the "in a comment" label sits inside
   * the text column rather than beside it — so a comment line and a body line
   * begin at the same x instead of stepping right by whatever the marker
   * happens to be (vision verdict 2026-08-06: the two kinds were 20px apart,
   * and a document row's line a further 4px off).
   */
  import { t } from '../../lib/i18n'
  import { highlightSegments } from '../../lib/format'
  import type { SearchMatch } from '../../lib/types'
  import Icon from '../ui/Icon.svelte'

  let { match, q }: { match: SearchMatch; q: string } = $props()

  const segs = $derived(highlightSegments(match.snippet, q))
</script>

<span
  class="flex w-full min-w-0 items-center gap-1 text-micro text-text-muted"
  data-testid="match-snippet"
  data-match-field={match.field}
>
  <span class="flex w-3.5 flex-none items-center justify-start" aria-hidden="true">
    {#if match.field === 'comment'}
      <Icon name="message-square" size={12} />
    {/if}
  </span>
  <span class="min-w-0 flex-1 truncate" data-testid="match-text">
    <!-- The gap after the separator is a margin, not a space: Svelte trims
         whitespace at a block boundary, which ran the marker into the text. -->
    {#if match.field === 'comment'}<span class="mr-1">{t('list.matchInComment')} ·</span>{/if}{#each segs as seg, i (i)}{#if seg.hit}<mark class="rounded-[2px] bg-status-stale/30 text-inherit">{seg.text}</mark>{:else}{seg.text}{/if}{/each}
  </span>
</span>
