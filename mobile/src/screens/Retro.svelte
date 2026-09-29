<script lang="ts">
  import Screen from '../ui/Screen.svelte'
  import EmptyState from '../ui/EmptyState.svelte'
  import Skeleton from '../ui/Skeleton.svelte'
  import { t } from '../lib/i18n'
  import { app, openIssue, setOwner } from '../lib/store.svelte'
  import { request } from '../lib/api'
  import { agingChart, hasMaterials, setCount, splitTemplate } from '../../../web/src/lib/retro/materials'
  import {
    METRIC_SPECS,
    SUMMARY_KEYS,
    deltaOf,
    formatDays,
    formatValue,
  } from '../../../web/src/lib/retro/metrics'
  import type { RetroBucket, RetroDoc } from '../../../web/src/lib/types'

  /*
   * The weekly retro as a phone reading (GDK-1827, second half) — the same
   * document the desk's RetroView renders, read for the question a phone
   * actually asks there: how did the last week go. The desk's organizing
   * axis is the ISO-week column table, one column per week; 402px holds no
   * such grid, and a squeezed copy of it would answer a question this screen
   * was not asked. So this is the report's current reading — the sentence
   * the desk opens with, the four numbers its summary strip carries, the
   * stalled tail a person can still act on, and what closed — and one line
   * saying where the whole table lives.
   *
   * Every derivation is the desk's own module (web/src/lib/retro, moved out
   * of components/ for exactly this): the same splitTemplate so the sentence
   * keeps Korean and Japanese word order, the same formatValue ladder so the
   * two surfaces cannot print different numbers for one bucket, the same
   * agingChart sort and cut so the tail is the same list. The phone authors
   * no arithmetic here — only the reading order.
   *
   * The report is fetched by the screen, not the store: the doc moves with
   * the serve's clock (the running bucket ends at now, which is why the
   * route has no ETag), so there is nothing worth caching across entries.
   * Refetch on every entry keeps a week-old pane honest for the price of
   * one request the reader just asked for.
   */
  let doc = $state<RetroDoc | null>(null)
  let loading = $state(false)
  let failed = $state(false)
  // Entry sequence: a re-entry while a fetch is still in flight must not
  // let the older answer land last (the race App.svelte's lastDetail comment
  // documents for details — same class, one screen over).
  let seq = 0

  async function load(): Promise<void> {
    const mine = ++seq
    loading = true
    failed = false
    try {
      const res = await request<RetroDoc>('issues/retro/')
      if (mine !== seq) return
      if (res.body) doc = res.body
    } catch {
      if (mine !== seq) return
      failed = true
    } finally {
      if (mine === seq) loading = false
    }
  }

  // Mount coincides with the first entry (the pane mounts on the latch
  // setOwner arms), so one effect covers both: fetch when this screen is
  // the owner, which is to say fetch on every arrival.
  $effect(() => {
    if (app.owner === 'retro') void load()
  })

  const buckets = $derived(doc?.buckets ?? [])
  const notes = $derived(doc?.notes ?? [])
  // A report that carries a note is never "empty": the note is the answer
  // (GDK-1679) — without this, a cold mirror blames sessions for a missing
  // table and hides the one sentence that says what to do.
  const empty = $derived(
    notes.length === 0 && buckets.every((b) => b.sessions === 0 && !b.closed && !b['in progress']),
  )

  // The bucket the whole screen speaks for: the one still filling, or the
  // newest finished one — the desk's own currentIndex rule, so the sentence,
  // the numbers and the closures are one week's reading, never three.
  const currentIndex = $derived.by(() => {
    const p = buckets.findIndex((b) => b.partial)
    return p >= 0 ? p : buckets.length - 1
  })
  const cur = $derived<RetroBucket | undefined>(buckets[currentIndex])
  const prev = $derived(currentIndex > 0 ? buckets[currentIndex - 1] : undefined)
  // Whether this server fills the materials at all. Absent on the release
  // before them, and then the sentence is gone rather than drawn empty.
  const materials = $derived(hasMaterials(cur))
  const aging = $derived(doc?.aging)
  const reopenUnavailable = $derived(doc?.reopen_unavailable === true)

  /*
   * The opening sentence (GDK-1724), the desk's own templates and the desk's
   * own cut. The values here are readings, not doors: the desk's sentence
   * opens the issues behind a clause, and the phone has no keys view to open
   * them onto — a number that pretends to be a door is the lie this screen
   * refuses, so the rows below (one issue each) are the doors this screen
   * has. `age` prefers the aging list's own oldest item, exactly as the desk
   * derives it.
   */
  const SLOTS = ['closed', 'unplanned', 'reopened', 'age'] as const
  const sentence = $derived(
    splitTemplate(t(reopenUnavailable ? 'retro.sentenceNoReopen' : 'retro.sentence'), SLOTS),
  )
  const sentenceValues = $derived.by<Record<string, string>>(() => {
    const none: Record<string, string> = {}
    if (!cur) return none
    const reopened = (cur.surprises ?? []).filter((x) => x.kind === 'reopened')
    const oldest = aging?.items?.length
      ? [...aging.items].sort((a, b) => b.days - a.days)[0]
      : null
    return {
      closed: formatValue(cur.closed, 'count'),
      unplanned: String(setCount(cur.unplanned)),
      reopened: String(reopened.length),
      age: oldest ? formatDays(oldest.days) : formatValue(cur['wip age max'], 'days'),
    }
  })

  // The four the desk's summary strip carries (SUMMARY_KEYS' own order),
  // each with its step from the bucket before — the same deltaOf rule,
  // running buckets included: the figure is real, the score is withheld.
  // Labels are the desk's row labels, so one metric is one word everywhere.
  const LABEL: Record<string, string> = {
    closed: t('retro.closed'),
    'cycle p85': t('retro.cycleP85'),
    'in progress': t('retro.inProgress'),
    'wip age max': t('retro.wipAge'),
  }
  const cells = $derived(
    SUMMARY_KEYS.map((key) => {
      const spec = METRIC_SPECS.find((m) => m.key === key)!
      const value = cur ? (cur[spec.key] as number | null) : null
      return {
        key,
        label: LABEL[key] ?? key,
        value: formatValue(value, spec.unit),
        delta: cur
          ? deltaOf(
              value,
              prev ? (prev[spec.key] as number | null) : null,
              spec.unit,
              spec.direction,
              cur.partial,
            )
          : null,
      }
    }),
  )

  // The stalled tail: agingChart's own sort (oldest first), own cut (30 +
  // how many more) and own p85 line position — the same list the desk draws
  // as bars, read here as rows.
  const tail = $derived(aging ? agingChart(aging.items, aging.p85_days) : null)

  /*
   * What closed, grouped by type. The groups carry keys, not titles; the
   * titles and the cost of each closure live on the cycle-time sample, so
   * the join happens here — a row without a point (older server, or a
   * resolution the sample truncates) still names its issue and opens it.
   */
  const closedGroups = $derived(cur?.closed_by_type ?? null)
  const pointsByKey = $derived(new Map((cur?.cycle_points ?? []).map((p) => [p.key, p])))
</script>

<Screen>
  {#snippet header()}
    <div class="head">
      <!-- The owner's one exit, the Shell's own control verbatim — see the
           sprints screen's copy of this comment for why the glyph, the
           corner and the word are not this screen's to invent. -->
      <button class="back" onclick={() => setOwner('list')} aria-label={t('app.back')}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M15 18l-6-6 6-6" />
        </svg>
      </button>
      <h1 class="type-subject">{t('retro.title')}</h1>
    </div>
  {/snippet}

  {#if loading && !doc}
    <Skeleton />
  {:else if failed}
    <EmptyState title={t('retro.loadFailed')}>
      <button class="link" onclick={() => void load()}>{t('common.retry')}</button>
    </EmptyState>
  {:else if doc && empty}
    <EmptyState title={t('retro.empty')} />
  {:else if doc}
    {#if cur && materials}
      <!-- Whitespace is content here, the desk's RetroSentence learned the
           hard way: the branches below are joined so a value does not arrive
           padded with a space on each side. -->
      <p class="sentence" data-testid="retro-sentence">{#each sentence as p, i (i)}{#if 'text' in p}{p.text}{:else}{sentenceValues[p.slot] ?? `{${p.slot}}`}{/if}{/each}</p>
    {/if}

    <div class="summary" data-testid="retro-summary">
      {#each cells as c, i (c.key)}
        <div class="cell" data-testid="retro-cell" data-metric={c.key}>
          <span class="line1">
            <span class="v" data-testid="retro-value">{c.value}</span>
            {#if c.delta}
              <span class="d tone-{c.delta.tone}" data-testid="retro-delta">{c.delta.glyph}{c.delta.text}</span>
            {/if}
          </span>
          <span class="label">{c.label}</span>
        </div>
      {/each}
    </div>

    {#if tail}
      <div class="section">
        <span class="sec-label">{t('retro.aging.title')}</span>
      </div>
      {#if tail.bars.length === 0}
        <p class="none" data-testid="retro-aging-empty">{t('retro.aging.empty')}</p>
      {:else}
        {#each tail.bars as b (b.key)}
          <button class="row" data-testid="retro-aging-row" data-key={b.key} data-over={b.over ? '1' : '0'} onclick={() => openIssue(b.key)}>
            <span class="line1">
              <span class="key">{b.key}</span>
              <span class="days {b.over ? 'over' : ''}">{formatDays(b.days)}</span>
            </span>
            {#if b.summary}
              <span class="title">{b.summary}</span>
            {/if}
          </button>
        {/each}
        {#if tail.more}
          <!-- The desk's "N more" is a door onto every remaining issue; the
               phone has no list of issues to open, so the count is said
               rather than pretending to open somewhere. -->
          <p class="none">{t('retro.aging.more', { n: tail.more })}</p>
        {/if}
      {/if}
    {/if}

    {#if closedGroups}
      <div class="section">
        <span class="sec-label">{t('retro.closed.title')}</span>
      </div>
      {#if closedGroups.length === 0}
        <p class="none" data-testid="retro-closed-empty">{t('retro.closed.none')}</p>
      {:else}
        {#each closedGroups as g, gi (`${g.issue_type_id ?? gi}`)}
          <div class="section group">
            <span class="sec-label">{g.issue_type || g.issue_type_id || ''}</span>
            <span class="n">{g.count}</span>
          </div>
          {#each g.keys as k (k)}
            {@const p = pointsByKey.get(k)}
            <button class="row" data-testid="retro-closed-row" data-key={k} onclick={() => openIssue(k)}>
              <span class="line1">
                <span class="key">{k}</span>
                {#if p}
                  <span class="days">{formatDays(p.days)}</span>
                {/if}
              </span>
              {#if p?.summary}
                <span class="title">{p.summary}</span>
              {/if}
            </button>
          {/each}
        {/each}
      {/if}
    {/if}

    {#if notes.length}
      <!-- The report's own empty-cell reasons (GDK-1679), the desk's foot
           notes verbatim: server English by design, the report's last word. -->
      <div class="notes">
        {#each notes as n (n.name)}
          <p class="note" data-testid="retro-note">{n.text}</p>
        {/each}
      </div>
    {/if}

    <p class="desktop" data-testid="retro-desktop">{t('retro.tableDesktop')}</p>
    <div class="foot" aria-hidden="true"></div>
  {/if}
</Screen>

<style>
  .head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 4px 0;
    min-width: 0;
  }
  /* The sprints screen's own comment stands: the owner's exit is the
     Shell's control verbatim — 44pt square, glyph alone, at the edge. */
  .back {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: center;
    width: var(--spacing-control);
    margin-left: -12px;
    color: var(--color-accent-text);
  }
  .back svg {
    width: 22px;
    height: 22px;
  }
  .sentence {
    margin: 0;
    padding: 12px 16px 0;
    font-size: var(--text-body);
    line-height: 1.45;
    color: var(--color-text-primary);
  }
  /* The desk's summary strip is one elevated surface divided into four, not
     four cards (RetroSummary); a 402px column holds two pairs, so the same
     surface folds to a 2×2 grid and keeps its dividing rules. */
  .summary {
    display: grid;
    grid-template-columns: 1fr 1fr;
    margin: 12px 16px 0;
    border: 1px solid var(--color-border-subtle);
    border-radius: 8px;
    background: var(--color-bg-panel);
    overflow: hidden;
  }
  .cell {
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding: 8px 12px;
    min-width: 0;
  }
  .cell:first-child,
  .cell:nth-child(3) {
    border-right: 1px solid var(--color-border-subtle);
  }
  .cell:nth-child(-n + 2) {
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .line1 {
    display: flex;
    align-items: baseline;
    gap: 6px;
    min-width: 0;
  }
  .v {
    font-size: var(--text-body);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-primary);
  }
  /* The tone rule is TONE_CLASS's (metrics.ts): colour only where a
     direction is agreed, and never on a running bucket. The classes there
     are the desk's Tailwind tokens; these are the same three meanings in
     this app's own tokens, done=the green, stale=the amber, muted=neither. */
  .d {
    flex: none;
    font-size: var(--text-micro);
    font-variant-numeric: tabular-nums;
  }
  .tone-good {
    color: var(--color-status-done);
  }
  .tone-bad {
    color: var(--color-status-stale);
  }
  .tone-none {
    color: var(--color-text-muted);
  }
  .label {
    font-size: var(--text-micro);
    color: var(--color-text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  /* The section grammar the issue list already wears: sticky label, the
     report's own vocabulary (the desk's section titles, untranslated keys
     at that — one word for one section on both surfaces). */
  .section {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: baseline;
    gap: 6px;
    padding: 14px 16px 4px;
    background: var(--color-bg-base);
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .sec-label {
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .group {
    padding-top: 8px;
  }
  .n {
    font-family: var(--font-mono);
  }
  .none {
    margin: 0;
    padding: 6px 16px;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  /* The row grammar the sprint cards wear: two lines at most — facts, then
     the title under them — each clamped to one line, because a block that
     grows pushes the rows it describes off the screen. The days column is
     right-aligned and tabular so the ages read as one column down the
     screen, the way the desk's gutter reads. */
  .row {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 1px;
    width: 100%;
    min-height: var(--spacing-control);
    padding: 4px 16px;
    text-align: left;
    border-bottom: 1px solid var(--color-border-subtle);
    min-width: 0;
  }
  .row:active {
    background: var(--color-bg-hover);
  }
  .key {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-body);
    color: var(--color-text-primary);
  }
  .days {
    flex: none;
    margin-left: auto;
    font-size: var(--text-micro);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-muted);
  }
  /* Past the p85 line — the amber the desk's bars carry there, and the only
     colour this screen spends on the tail (RetroAging's own rule: under the
     line is muted, or the chart has told the reader nothing). */
  .days.over {
    color: var(--color-status-stale);
  }
  .title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .notes {
    padding: 12px 16px 0;
  }
  .note {
    margin: 0;
    font-size: var(--text-micro);
    line-height: 1.35;
    color: var(--color-text-muted);
  }
  .note + .note {
    margin-top: 4px;
  }
  /* One line, the last word: the complete table is a desk reading, and this
     screen says where it is rather than squeezing it. */
  .desktop {
    margin: 0;
    padding: 16px 16px 0;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .foot {
    height: 24px;
  }
  .link {
    color: var(--color-accent-text);
    font-size: var(--text-body);
    min-height: var(--spacing-control);
    padding: 0 16px;
  }
</style>
