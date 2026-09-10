<script lang="ts">
  /*
   * A proportion bar — track plus fill (GDK-142 V13).
   *
   * The audit read the epic progress bar as an underline, and the measurement
   * says why: its track was `bg-bg-elevated` while the detail panel's ground
   * is `bg-bg-panel`, which is 1.09:1 apart in the dark palette (1.00:1 when
   * the bar sits on an elevated card — literally invisible). A bar with no
   * visible empty half cannot say "40% of the way"; it can only say "there is
   * a mark here".
   *
   * The track is `bg-bg-active`, the ground ladder's top step, measured at
   * 1.40–1.76:1 against base/panel and 1.28–1.46:1 against elevated across
   * all four palettes. lib/chrome-vocabulary.test.ts pins those floors and
   * asserts no other component draws a track of its own.
   */
  let {
    percent,
    fill = 'bg-text-muted/50',
    height = 'h-1.5',
    width = 'w-full',
    class: klass = '',
  }: {
    /** 0–100; clamped here so no caller has to. */
    percent: number
    /** Tailwind class or a CSS colour for the filled part. */
    fill?: string
    height?: string
    /** The caller owns the track's width; the bar is a single element so a
     *  caller never has to wrap it (an extra box would change the DOM depth
     *  e2e/detail-coaching.spec.ts walks). */
    width?: string
    class?: string
  } = $props()

  const pct = $derived(Math.max(0, Math.min(100, percent)))
  const isColor = $derived(/^(#|rgb|hsl|var\()/.test(fill))
</script>

<div
  class="{height} {width} min-w-0 overflow-hidden rounded-full bg-bg-active {klass}"
  aria-hidden="true"
>
  <div
    class="h-full rounded-full transition-[width] {isColor ? '' : fill}"
    style:width="{pct}%"
    style:background={isColor ? fill : undefined}
  ></div>
</div>
