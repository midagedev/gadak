<script lang="ts">
  /*
   * One section of the retro, and the paragraph that says how to read it
   * (GDK-1726).
   *
   * The sections are not cards. A card is a container that says "this is a
   * separate thing", and the whole argument of this screen is that the six
   * readings below are one document — so the divider is a rule and the title
   * is the same small muted line the rest of the app uses for a group header.
   *
   * The explanation is three sentences in a fixed order — what it is, why a
   * retro looks at it, how to read it — because a person who does not know
   * what a p85 line is will not learn it from a tooltip that only defines
   * the words. It is folded with the same toggle the table's definitions use
   * (there is one "explain yourself" control on this header, not two), and
   * the ⓘ beside the title carries the first line as a title attribute so
   * the shortest question is answered without unfolding anything.
   */
  import type { Snippet } from 'svelte'
  import Icon from '../ui/Icon.svelte'
  import { locale } from '../../lib/i18n'

  let {
    id,
    title,
    what,
    why,
    how,
    open = false,
    children,
    trailing,
  }: {
    /** Section name, on the element — what a test or a capture names it by. */
    id: string
    title: string
    /** The three explanation lines. Empty strings draw nothing. */
    what?: string
    why?: string
    how?: string
    /** Whether the paragraph is unfolded. */
    open?: boolean
    children: Snippet
    /** Optional controls on the title's right edge. */
    trailing?: Snippet
  } = $props()

  /*
   * What goes between the three sentences. Japanese sets no space after 。 —
   * the first capture had "です。 古さは", which reads as a typo to anyone who
   * reads Japanese. English and Korean both space their sentences.
   */
  const sep = $derived(locale() === 'ja' ? '' : '\u00a0')
</script>

<section class="border-t border-border-subtle pt-3 first:border-t-0 first:pt-0" data-testid="retro-section" data-section={id}>
  <div class="flex items-center gap-1.5">
    <h3 class="text-micro font-medium text-text-muted">{title}</h3>
    {#if what}
      <Icon name="info" size={12} class="flex-none text-text-muted opacity-60" title={what} />
    {/if}
    {#if trailing}
      <div class="ml-auto flex items-center gap-1">{@render trailing()}</div>
    {/if}
  </div>
  {#if open && (what || why || how)}
    <p
      class="mt-1 max-w-[560px] text-micro leading-snug text-text-muted"
      data-testid="retro-explain"
      data-section={id}
    >
      {what}{#if why}{sep}{why}{/if}{#if how}{sep}{how}{/if}
    </p>
  {/if}
  <div class="mt-2">{@render children()}</div>
</section>
