<script lang="ts">
  /*
   * The report's opening line (GDK-1724).
   *
   * The summary strip above it answers "how is this bucket going" in four
   * labelled numbers; this answers it in a sentence, which is the form a
   * person repeats to somebody else. Four values, each one a door, so
   * reading the sentence and opening the issues behind a clause are the same
   * gesture.
   *
   * Assembled from a split template rather than a substituted string,
   * because the values have to stay their own elements. That also leaves
   * word order where it belongs: Korean puts the counted noun before its
   * number and Japanese ends on the verb, so a sentence built by
   * concatenating English fragments would be neither.
   */
  import { t } from '../../lib/i18n'
  import { splitTemplate } from './materials'

  let {
    values,
    sprint = false,
  }: {
    /** Slot name → the text to print and the issues behind it. An absent
     *  slot prints its own name, which is how a catalog typo shows itself. */
    values: Record<string, { text: string; keys: string[]; onOpen: () => void }>
    /** A sprint cut has a fourth clause: what joined after it started. */
    sprint?: boolean
  } = $props()

  const SLOTS = ['closed', 'unplanned', 'reopened', 'age', 'added'] as const
  const pieces = $derived(splitTemplate(t(sprint ? 'retro.sentenceSprint' : 'retro.sentence'), SLOTS))
</script>

<!--
  Whitespace is content here. Svelte keeps the newlines between an `{#each}`
  body's branches, and the first capture read "Closed 0 ( 2 unplanned) … 41.5d ."
  — a space on both sides of every value, because each branch sat on its own
  indented line. The tags below are joined deliberately; the ugly line breaks
  inside the attributes are where the formatting went instead.
-->
<p class="mb-3 max-w-[720px] text-body leading-relaxed text-text-primary" data-testid="retro-sentence">
  {#each pieces as p, i (i)}{#if 'text' in p}{p.text}{:else}{@const v = values[p.slot]}{#if !v}{`{${p.slot}}`}{:else if v.keys.length}<button
        type="button"
        class="rounded tabular-nums underline decoration-border-strong decoration-dotted underline-offset-4 transition-colors hover:bg-bg-hover hover:decoration-accent"
        data-testid="retro-sentence-value"
        data-slot={p.slot}
        title={t('retro.openIssues')}
        onclick={v.onOpen}>{v.text}</button>{:else}<span
        class="tabular-nums"
        data-testid="retro-sentence-value"
        data-slot={p.slot}>{v.text}</span>{/if}{/if}{/each}
</p>
