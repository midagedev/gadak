<script lang="ts">
  /*
   * What happened, and what was surprising about it (GDK-1722).
   *
   * A retro's first question is not "how many" but "how did the week go",
   * and a total cannot answer it: a sprint where everything closed on the
   * last afternoon has the same closed count as one that flowed. The strip
   * is one column per day, as tall as the day was busy, so the rhythm is the
   * shape rather than a number to interpret.
   *
   * Three colours only, and all three are already on the screen elsewhere:
   * a start takes the in-progress tone, a finish the done tone, and every
   * other kind of event is grey. Comments and sprint moves are the texture
   * a busy day is made of — they belong in the height, not in a legend.
   *
   * Underneath, the same window said in words. The strip shows that
   * something happened on Thursday; the list says it was an issue coming
   * back for the third time, which is the sentence a person actually
   * repeats out loud in the meeting.
   */
  import Icon from '../ui/Icon.svelte'
  import type { IconName } from '../ui/Icon.svelte'
  import { t, locale } from '../../lib/i18n'
  import { densityStrip } from './materials'
  import type { RetroBucket, RetroSurpriseKind } from '../../lib/types'

  let {
    bucket,
    onOpen,
  }: {
    bucket: RetroBucket
    onOpen: (keys: string[]) => void
  } = $props()

  const H = 28
  const days = $derived(densityStrip(bucket.events ?? [], bucket.from, bucket.to))
  const surprises = $derived(bucket.surprises ?? [])

  /** Existing glyphs only — the reopen arrow is already "came back" on every
   *  issue row, so the strip and the list name the same thing the same way. */
  const ICON: Record<RetroSurpriseKind, IconName> = {
    reopened: 'rotate-ccw',
    reversal: 'refresh',
    added_after_start: 'plus-circle',
    carried: 'arrow-up-right',
  }
  const KIND_LABEL: Record<RetroSurpriseKind, string> = $derived({
    reopened: t('retro.surprise.reopened'),
    reversal: t('retro.surprise.reversal'),
    added_after_start: t('retro.surprise.added_after_start'),
    carried: t('retro.surprise.carried'),
  })

  /*
   * A surprise's detail, in the reader's own terms.
   *
   * `added_after_start` carries the moment it joined, which the wire sends as
   * RFC3339 — printed raw it is a machine timestamp sitting in the middle of
   * a sentence (the first capture had one). Everything else is already prose
   * or a count and passes through untouched.
   */
  function detailOf(s: { kind: RetroSurpriseKind; detail?: string }): string {
    const d = s.detail ?? ''
    if (s.kind !== 'added_after_start') return d
    const t0 = Date.parse(d)
    if (!Number.isFinite(t0)) return d
    return new Intl.DateTimeFormat(locale(), { month: 'short', day: 'numeric' }).format(new Date(t0))
  }

  function dayTitle(day: string, n: number): string {
    const d = new Date(day + 'T00:00:00Z')
    const label = new Intl.DateTimeFormat(locale(), {
      month: 'short',
      day: 'numeric',
      timeZone: 'UTC',
    }).format(d)
    return t('retro.events.day', { date: label, n })
  }
</script>

{#if days.length}
  <!-- Flexed rather than an SVG grid: the columns share the width the
       section has, and a day is a box, not a curve. -->
  <!--
    Fixed-width columns, not `flex-1` ones. The first capture caught it: the
    running week had two days in it, so two columns each took half the
    section and the strip read as one green slab rather than as two days.
    A day is at most a day wide whether the window holds two of them or
    ninety; the cap is what stops a two-day week from becoming a banner, and
    the floor is what keeps a twelve-week one from becoming a hairline.
  -->
  <div class="flex max-w-[720px] items-end gap-px" style:height="{H}px" data-testid="retro-density">
    {#each days as d (d.day)}
      <div
        class="flex min-w-[6px] max-w-[14px] flex-1 flex-col justify-end"
        style:height="{H}px"
        data-testid="retro-density-day"
        data-day={d.day}
        data-total={d.total}
        title={dayTitle(d.day, d.total)}
      >
        {#if d.total === 0}
          <!-- An empty day still has a floor, so the gaps read as part of the
               strip rather than as the strip ending early. -->
          <div class="h-px w-full bg-border-subtle"></div>
        {:else}
          {@const px = Math.max(Math.round(d.height * H), 2)}
          {#if d.resolved}
            <div class="w-full bg-status-done" style:height="{Math.max(Math.round((d.resolved / d.total) * px), 1)}px"></div>
          {/if}
          {#if d.started}
            <div class="w-full bg-status-inprogress" style:height="{Math.max(Math.round((d.started / d.total) * px), 1)}px"></div>
          {/if}
          {#if d.other}
            <div class="w-full bg-text-muted opacity-40" style:height="{Math.max(Math.round((d.other / d.total) * px), 1)}px"></div>
          {/if}
        {/if}
      </div>
    {/each}
  </div>
{:else}
  <p class="text-micro text-text-muted" data-testid="retro-density-empty">{t('retro.events.none')}</p>
{/if}

{#if surprises.length}
  <div class="mt-3 max-w-[720px]">
    <div class="text-micro font-medium text-text-muted">{t('retro.surprises.title')}</div>
    <ul class="mt-1" data-testid="retro-surprises">
      {#each surprises as s, i (`${s.kind}:${s.key}:${i}`)}
        <li class="flex items-baseline gap-1.5 py-0.5 text-micro leading-snug" data-testid="retro-surprise" data-kind={s.kind}>
          <Icon name={ICON[s.kind] ?? 'info'} size={12} class="flex-none translate-y-px text-text-muted" />
          <button
            type="button"
            class="rounded px-0.5 tabular-nums text-text-secondary underline decoration-border-strong decoration-dotted underline-offset-4 hover:bg-bg-hover hover:decoration-accent"
            onclick={() => onOpen([s.key])}>{s.key}</button
          >
          <span class="text-text-muted"
            >{KIND_LABEL[s.kind] ?? s.kind}{#if s.detail}&nbsp;· {detailOf(s)}{/if}</span
          >
        </li>
      {/each}
    </ul>
  </div>
{/if}
