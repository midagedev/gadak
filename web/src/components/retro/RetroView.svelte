<script lang="ts">
  /*
   * Weekly retro (GDK-1660) — the document `gadak retro` prints, as a
   * main-column view. One column per ISO week (or per sprint, GDK-1693), one
   * row per metric. A cell that has issue keys behind it is a door
   * (THEORY.md G8): clicking puts those issues on the list, the same move
   * `retro --open` makes. Visual language follows HistoryView — header,
   * segmented range, no new tokens.
   *
   * GDK-1712 made it a report rather than a grid. Three changes, one reason:
   * the table was complete and could not be read at a glance.
   *   - a summary strip for the bucket you are in, with the step from the one
   *     before (RetroSummary);
   *   - a sparkline per row, so direction survives twelve columns without
   *     reading twelve numbers (Sparkline);
   *   - deltas per cell, coloured only where a direction is actually agreed
   *     and never on the running bucket, whose numbers are not final.
   * And the definitions folded away. They were printed under every label,
   * which is what forced a 260px label column — eight paragraphs of prose
   * standing permanently between the reader and the numbers. They are still
   * one click away, and one hover away, because a number without its
   * definition is the thing this view refuses to show.
   *
   * GDK-1713 gives the sprint cut its board. Several boards with sprints is a
   * 409, not a failure: sprint windows from two boards overlap, so a merged
   * column set would be neither team's cadence. The server hands over the
   * rows; the picker is built from them.
   */
  import ColumnHeader from '../ui/ColumnHeader.svelte'
  import Icon from '../ui/Icon.svelte'
  import LoadingState from '../ui/LoadingState.svelte'
  import EmptyState from '../list/EmptyState.svelte'
  import Sparkline from './Sparkline.svelte'
  import RetroSummary from './RetroSummary.svelte'
  import { t, locale } from '../../lib/i18n'
  import { ApiError, getBoards, getRetro } from '../../lib/api'
  import type { BoardRow, RetroBucket } from '../../lib/types'
  import { emptyConfig } from '../../lib/view-config'
  import { showIssueList } from '../../lib/show-issue-list'
  import { pages } from '../../stores/pages.svelte'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { createResource } from '../../lib/resource.svelte'
  import { sprints } from '../../stores/sprints.svelte'
  import { METRIC_SPECS, TONE_CLASS, deltaOf, formatValue, type MetricSpec } from './metrics'

  // Three windows, plus the sprint cut when this workspace has sprints
  // (GDK-1693). Sprint is not a fourth window — it is a different bucket
  // source — but it sits on the same control because it answers the same
  // question the reader is asking of it: how wide is a column.
  const RANGES = ['4w', '8w', '12w'] as const
  type Range = (typeof RANGES)[number] | 'sprint'
  const RANGE_LABEL: Record<Range, string> = {
    '4w': t('retro.range4w'),
    '8w': t('retro.range8w'),
    '12w': t('retro.range12w'),
    sprint: t('retro.bySprint'),
  }
  const ranges = $derived<Range[]>(sprints.any ? [...RANGES, 'sprint'] : [...RANGES])

  const DEFS_KEY = 'gadak.retro.definitions'
  const BOARD_KEY = 'gadak.retro.board'

  function readLocal(key: string): string | null {
    try {
      return localStorage.getItem(key)
    } catch {
      return null
    }
  }
  function writeLocal(key: string, value: string): void {
    try {
      localStorage.setItem(key, value)
    } catch {
      /* private window, blocked site data — the view still works */
    }
  }

  let since = $state<Range>('4w')
  // Which board the sprint cut is cut by. Remembered per browser rather than
  // put in the URL: `since` is not addressable either, and the URL's place
  // params say which screen you are on, not how you have that screen set
  // (lib/url-state's three categories).
  let board = $state<number>(Number(readLocal(BOARD_KEY)) || 0)
  // Definitions open or shut. Shut by default — see the header comment —
  // and remembered, because whichever way a person reads this table is how
  // they read it every week.
  let showDefs = $state(readLocal(DEFS_KEY) === '1')

  // The shared resource rune: key change → reload, stale answers dropped.
  // The board is part of the key, so picking one is a reload.
  const res = createResource(
    () => (since === 'sprint' ? `sprint:${board}` : since),
    () => getRetro(since, since === 'sprint' ? board : 0),
  )
  const doc = $derived(res.data)
  const loading = $derived(res.loading)
  const failed = $derived(res.errorKind !== null)

  /*
   * The refusal, when the server sent one it expects to be acted on. A 409
   * carrying `ambiguous_board` is not a load failure — it is the report
   * asking which board, with the boards attached (GDK-1713). Everything else
   * stays the one line it was.
   */
  const refusal = $derived.by(() => {
    const e = res.error
    if (!(e instanceof ApiError) || e.status !== 409) return null
    const body = e.body ?? {}
    const boards = Array.isArray(body.boards) ? (body.boards as { id: number; name: string }[]) : []
    // `body.message` is deliberately not read: it is the report's own
    // sentence, written for `gadak retro`, and the EmptyState below says why
    // this surface writes its own instead.
    return { code: typeof body.error === 'string' ? body.error : '', boards }
  })

  /*
   * The picker's options. Two sources, because the refusal only exists while
   * the report is refused: once a board is chosen the request succeeds and
   * the 409's list is gone, so a reload with a board already remembered would
   * have lost the way back to the other one. `boards/` is the durable list;
   * the refusal's rows are what shows before it lands.
   */
  let boardList = $state<BoardRow[]>([])
  let boardsAsked = false
  /** Asked once, when the sprint cut is chosen — a list nobody has opened
   *  the cut for is a request nobody needs. */
  function loadBoards(): void {
    if (boardsAsked) return
    boardsAsked = true
    void getBoards()
      .then((r) => (boardList = r.boards.filter((b) => b.has_sprints)))
      .catch(() => {
        /* the refusal's own rows are the fallback */
      })
  }
  const boardOptions = $derived<{ id: number; name: string }[]>(
    boardList.length ? boardList : (refusal?.boards ?? []),
  )
  // The picker is a question only when there is a choice. One board answers
  // itself — the server picks it — and no board means this cut is not on.
  const showBoardPicker = $derived(since === 'sprint' && boardOptions.length > 1)

  const skeleton = createSkeletonGrace(() => loading && !doc)

  type Metric = MetricSpec & { label: string; def: string }
  // The split gap the report ran with, as the report states it — never a
  // literal in the translation, because --session-gap moves it.
  const gap = $derived(doc?.session_gap ?? '30m')
  // The bucket the definitions name. The server says which it computed, so a
  // stale answer cannot make the sentences describe the other one.
  const bucket = $derived(
    doc?.bucket_noun === 'sprint' ? t('retro.bucket.sprint') : t('retro.bucket.week'),
  )
  /*
   * Labels and definitions. Written here, not read out of `doc.definitions` —
   * that object is `internal/retro`'s CLI footer and is English on every
   * locale, so the row labels translated and their definitions did not
   * (GDK-1692).
   */
  const LABEL: Record<string, string> = $derived({
    sessions: t('retro.sessions'),
    'resume (median)': t('retro.resume'),
    closed: t('retro.closed'),
    'cycle p50': t('retro.cycleP50'),
    'cycle p85': t('retro.cycleP85'),
    'in progress': t('retro.inProgress'),
    'wip age max': t('retro.wipAge'),
    mismatch: t('retro.mismatch'),
  })
  const DEF: Record<string, string> = $derived({
    sessions: t('retro.def.sessions', { gap, bucket }),
    'resume (median)': t('retro.def.resume', { bucket }),
    closed: t('retro.def.closed', { bucket }),
    'cycle p50': t('retro.def.cycleP50', { bucket }),
    'cycle p85': t('retro.def.cycleP85', { bucket }),
    'in progress': t('retro.def.inProgress', { bucket }),
    'wip age max': t('retro.def.wipAge', { bucket }),
    mismatch: t('retro.def.mismatch', { bucket }),
  })
  const METRICS: Metric[] = $derived(
    METRIC_SPECS.map((m) => ({ ...m, label: LABEL[m.key] ?? m.key, def: DEF[m.key] ?? '' })),
  )

  const buckets = $derived(doc?.buckets ?? [])
  // The report's own empty-cell reasons, printed under the table the way the
  // CLI prints them under its own (GDK-1679).
  const notes = $derived(doc?.notes ?? [])
  // A report that carries a note is never "empty": the note is the answer.
  // Without this, a cold mirror — no status_catalog, no visits — got
  // "No sessions in this range", which blames sessions for a missing table
  // and hides the one sentence that says what to do (GDK-1679).
  const empty = $derived(
    notes.length === 0 && buckets.every((b) => b.sessions === 0 && !b.closed && !b['in progress']),
  )

  // The bucket the summary speaks for: the one still filling, or the newest
  // finished one when nothing is running.
  const currentIndex = $derived.by(() => {
    const p = buckets.findIndex((b) => b.partial)
    return p >= 0 ? p : buckets.length - 1
  })

  function weekLabel(b: RetroBucket): string {
    const from = new Date(b.from)
    // `to` is exclusive; the header names the last day the week holds.
    const last = new Date(new Date(b.to).getTime() - 86_400_000)
    const f = new Intl.DateTimeFormat(locale(), { month: 'short', day: 'numeric' })
    const sameMonth = from.getMonth() === last.getMonth()
    const end = sameMonth ? new Intl.DateTimeFormat(locale(), { day: 'numeric' }).format(last) : f.format(last)
    return `${f.format(from)} – ${end}`
  }

  /** The column's title, with the running marker the header shows. */
  function bucketTitle(b: RetroBucket): string {
    const name = b.name || weekLabel(b)
    if (!b.partial) return name
    return `${name} · ${b.name ? t('retro.thisSprint') : t('retro.thisWeek')}`
  }

  function cell(b: RetroBucket, m: Metric): string {
    return formatValue(b[m.key] as number | null | undefined, m.unit)
  }

  function keysOf(b: RetroBucket, m: Metric): string[] {
    return m.keys ? b.keys[m.keys] : []
  }

  function seriesOf(m: Metric): (number | null)[] {
    return buckets.map((b) => (b[m.key] as number | null | undefined) ?? null)
  }

  function open(keys: string[]): void {
    if (!keys.length) return
    const c = emptyConfig()
    c.filters.keys = keys
    c.display.group_by = 'none'
    showIssueList(c)
  }

  function toggleDefs(): void {
    showDefs = !showDefs
    writeLocal(DEFS_KEY, showDefs ? '1' : '0')
  }

  function pickBoard(e: Event): void {
    board = Number((e.currentTarget as HTMLSelectElement).value) || 0
    writeLocal(BOARD_KEY, String(board))
  }
</script>

<section class="flex h-full min-h-0 flex-col bg-bg-base" data-testid="retro-view" data-skeleton={skeleton.attr}>
  <ColumnHeader title={t('retro.title')} closeTestid="retro-close" onClose={() => pages.closeRetro()}>
    <div class="ml-1 flex flex-none items-center gap-0.5 rounded-md bg-bg-elevated p-1">
      {#each ranges as r (r)}
        <button
          type="button"
          class="flex h-control-sm items-center rounded px-2 text-micro font-medium {since === r
            ? 'bg-bg-active text-text-primary'
            : 'text-text-muted hover:text-text-secondary'}"
          aria-pressed={since === r}
          data-testid="retro-range"
          data-range={r}
          onclick={() => {
            since = r
            if (r === 'sprint') loadBoards()
          }}
        >
          {RANGE_LABEL[r]}
        </button>
      {/each}
    </div>
    {#if showBoardPicker}
      <select
        class="ml-1 h-control-sm max-w-[11rem] flex-none appearance-none rounded-md border border-border-strong bg-bg-base pl-2 pr-2 text-micro text-text-primary outline-none focus:border-accent"
        data-testid="retro-board"
        aria-label={t('retro.board')}
        value={String(board)}
        onchange={pickBoard}
      >
        <option value="0" disabled>{t('retro.boardPick')}</option>
        {#each boardOptions as b (b.id)}
          <option value={String(b.id)}>{b.name || `#${b.id}`}</option>
        {/each}
      </select>
    {/if}
    {#snippet trailing()}
      <!--
        The right edge, through ColumnHeader's own `trailing` slot rather
        than an `ml-auto` of our own: the header already gives the right side
        a flex-1 group, so a second auto margin would have parked this
        mid-band instead of at the edge (the component says so at that div).
      -->
      <button
        type="button"
        class="flex h-control-sm flex-none items-center rounded px-2 text-micro font-medium {showDefs
          ? 'bg-bg-active text-text-primary'
          : 'text-text-muted hover:text-text-secondary'}"
        aria-pressed={showDefs}
        data-testid="retro-defs-toggle"
        onclick={toggleDefs}
      >
        {t('retro.definitions')}
      </button>
    {/snippet}
  </ColumnHeader>

  {#if refusal}
    <!--
      The report refusing is not the report failing: it is a question, and
      the question gets asked here rather than behind `retro.loadFailed`
      (GDK-1713).

      The words are the view's, not the server's. The 409 carries a `message`
      too — the report's own sentence, written for `gadak retro`, so it names
      `--board`, a flag nobody reading this has, and it prints the board list
      as CLI rows that collapse into one line here (measured on the capture).
      The picker in the header is this surface's answer to the same question,
      so the copy points at it. The sentence stays on the wire for the
      callers that only have a line to print; `boards` is what this side reads.
    -->
    <EmptyState
      icon="info"
      title={refusal.code === 'ambiguous_board' ? t('retro.pickBoard') : t('retro.noSprints')}
      hint={refusal.code === 'ambiguous_board' ? t('retro.pickBoardHint') : t('retro.noSprintsHint')}
    />
  {:else if failed}
    <EmptyState icon="warning" title={t('retro.loadFailed')} actionLabel={t('common.retry')} onAction={() => res.reload()} />
  {:else if loading && !doc}
    {#if skeleton.visible}
      <LoadingState />
    {/if}
  {:else if doc && empty}
    <EmptyState icon="" title={t('retro.empty')} />
  {:else if doc}
    <div class="min-h-0 flex-1 overflow-auto px-3 py-3">
      {#if buckets.length && currentIndex >= 0}
        <RetroSummary
          bucket={buckets[currentIndex]}
          previous={currentIndex > 0 ? buckets[currentIndex - 1] : undefined}
          title={bucketTitle(buckets[currentIndex])}
          labels={LABEL}
        />
      {/if}
      <!--
        `w-max` alone, not `w-max min-w-full` (GDK-1706). With many columns
        the two agree and the row scrolls; with few, `min-w-full` won and
        stretched the table to the container, sharing the slack out between
        the columns — a two-sprint cut left the metric names at one edge and
        their two numbers a third of the screen away. A spacer cell is not
        the fix either: `w-full` on a table cell demands the whole container
        and crushes the label column to one character (measured on the same
        frame). The table simply takes the width it needs.
      -->
      <table class="w-max border-separate border-spacing-0 text-body" data-testid="retro-table">
        <thead>
          <tr>
            <th class="sticky left-0 z-10 bg-bg-base pb-2 pr-6 text-left text-micro font-medium text-text-muted"></th>
            {#each buckets as b (b.from)}
              <th class="whitespace-nowrap pb-2 pl-6 text-right text-micro font-medium {b.partial ? 'text-text-secondary' : 'text-text-muted'}" data-testid="retro-week">
                {b.name || weekLabel(b)}
                {#if b.partial}<span class="ml-1 font-normal text-text-muted"
                  >· {b.name ? t('retro.thisSprint') : t('retro.thisWeek')}</span
                >{/if}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each METRICS as m (m.key)}
            <tr class="border-t border-border-subtle">
              <th
                class="sticky left-0 z-10 bg-bg-base py-2 pr-6 text-left align-top font-normal"
              >
                <!--
                  The label stays on one line. It could not before: the
                  definition under it set the column's width, so the name
                  wrapped inside whatever the paragraph left. With the
                  definitions folded the row is one line of text plus its
                  marks, and the column is as wide as the longest name —
                  which in every locale measured is narrower than the 260px
                  the prose used to demand (GDK-1706's `w-max` still decides
                  the table's total).
                -->
                <div class="flex w-full min-w-[200px] items-center justify-between gap-2 whitespace-nowrap">
                  <span class="flex items-center gap-1.5">
                    <span class="text-body text-text-primary">{m.label}</span>
                  <!--
                    The definition, always reachable without taking the space
                    it used to: the glyph carries it as a title, the toggle
                    unfolds all eight at once.
                  -->
                    <Icon
                      name="info"
                      size={12}
                      class="flex-none text-text-muted opacity-60"
                      title={m.def}
                    />
                  </span>
                  <!-- The lines share one right edge, so eight rows read as
                       one small chart rather than eight ragged ones. -->
                  <Sparkline values={seriesOf(m)} metric={m.key} />
                </div>
                {#if showDefs}
                  <div class="mt-0.5 max-w-[260px] whitespace-normal text-micro leading-snug text-text-muted" data-testid="retro-def">{m.def}</div>
                {/if}
              </th>
              {#each buckets as b, i (b.from)}
                {@const keys = keysOf(b, m)}
                {@const text = cell(b, m)}
                {@const d = deltaOf(
                  b[m.key] as number | null,
                  i > 0 ? (buckets[i - 1][m.key] as number | null) : null,
                  m.unit,
                  m.direction,
                  b.partial,
                )}
                <td class="py-2 pl-6 text-right align-top tabular-nums {text === '—' ? 'text-text-muted' : 'text-text-primary'}">
                  <span class="inline-flex items-baseline gap-1">
                    {#if keys.length}
                      <button
                        type="button"
                        class="-mr-1 rounded px-1 py-0.5 tabular-nums underline decoration-border-strong decoration-dotted underline-offset-4 transition-colors hover:bg-bg-hover hover:decoration-accent"
                        data-testid="retro-cell"
                        data-metric={m.key}
                        title={t('retro.openIssues')}
                        onclick={() => open(keys)}
                      >
                        {text}{#if b.keys.keys_truncated && keys.length >= 500}<span class="ml-0.5 text-micro text-text-muted">{t('retro.truncated')}</span>{/if}
                      </button>
                    {:else}
                      {text}
                    {/if}
                    {#if d}
                      <span
                        class="text-micro tabular-nums {TONE_CLASS[d.tone]}"
                        data-testid="retro-delta"
                        data-metric={m.key}
                        data-tone={d.tone}
                        title={t('retro.vsPrevious')}>{d.glyph}{d.text}</span
                      >
                    {/if}
                  </span>
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
      {#if notes.length}
        <dl class="mt-4 max-w-[720px] border-t border-border-subtle pt-3 text-micro leading-snug text-text-muted" data-testid="retro-notes">
          {#each notes as n (n.name)}
            <div class="mt-1 first:mt-0">
              <dt class="inline font-medium text-text-secondary">{n.name}</dt>
              <dd class="inline">: {n.text}</dd>
            </div>
          {/each}
        </dl>
      {/if}
    </div>
  {/if}
</section>
