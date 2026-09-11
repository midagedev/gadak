<script lang="ts">
  /*
   * The active sprint, said out loud (GDK-1709).
   *
   * The scope control beside it already knows which sprint the board is
   * showing — it has known since GDK-1656 — but it says it inside a segment
   * 120px wide, and everything else the sprint carries (its goal, when it
   * ends, how much of it is done) has been reachable only by hovering a
   * button nobody has a reason to hover. This is that same knowledge given a
   * line of its own, on the one screen where it is the question.
   *
   * Not a card. A strip: the same elevated, borderless, micro-text form the
   * session strip uses one screen up, so it reads as a line of the board's
   * chrome rather than a panel that floated in above the columns. It appears
   * only while the board is scoped to exactly one active sprint — the state
   * where "the sprint" has a referent — and draws nothing at all otherwise.
   * A kanban workspace never reaches this component: SprintScope's own gate
   * keeps the scope off the toolbar, and the scope is the only way in.
   *
   * The numbers come from the server (`store.Sprints`), not from the issues
   * on screen. Counting the loaded pool would report whatever the current
   * filter left behind as the sprint's shape — a board narrowed to one
   * assignee would show that person's progress as the team's.
   *
   * The bar fills done and in-progress in the board's own category colours
   * (`categoryMetaOf`, the same function the columns' headers call), left to
   * right in the order work moves; to-do is the bare track. Progress, not a score (G9): the reading is
   * "8 of 20", never a grade, and the only judgement anywhere on the line is
   * the days-left phrase, which states a fact.
   *
   * The burn-up (GDK-1710/1752) sits on this line too, drawn by
   * BurnupSpark from the document the burnup store fetches per sprint; the bar
   * is the element that yields the width for it. The fetch lives here, not
   * in the chart: this is the surface that knows which sprint and when.
   */
  import { t, localeTag } from '../../lib/i18n'
  import BurnupSpark from '../sprint/BurnupSpark.svelte'
  import { burnup } from '../../stores/burnup.svelte'
  import { calendarDay, formatAbs, localZone } from '../../lib/calendar'
  import { categoryMetaOf } from '../../lib/format'
  import { filters } from '../../stores/filters.svelte'
  import { sprints } from '../../stores/sprints.svelte'

  /* The one sprint this strip is about, or nothing. Both halves matter: the
   * board must be scoped to the active sprint (otherwise the columns hold
   * more than this line describes), and there must be exactly one active
   * sprint (a workspace with several boards has several, and "the sprint" is
   * then a question rather than a fact). */
  const sprint = $derived.by(() => {
    const st = filters.filters.sprint_state
    if (st.length !== 1 || st[0] !== 'active') return null
    const active = sprints.active
    return active.length === 1 ? active[0] : null
  })

  /** BCP 47 tag for the date formatter — i18n's own mapping (GDK-1455 made
   *  it public; this used to re-spell the same three-way map here). */
  const tag = $derived(localeTag())

  /** Dates as days, never instants: a sprint boundary is a calendar day, and
   *  `absTime` would spend half the strip printing hours and minutes. */
  function day(iso: string | undefined): string {
    return iso ? formatAbs(iso, 'date', localZone(), tag) : ''
  }

  const range = $derived.by(() => {
    const from = day(sprint?.start_at)
    const to = day(sprint?.end_at)
    if (from && to) return `${from} – ${to}`
    return from || to
  })

  /* Whole days between today and the end day, in the reader's own zone —
   * `calendarDay` is the mirror's existing answer to "which day is this
   * stamp", so a sprint ending at 04:29Z does not read as a day early or
   * late depending on where the person sits. */
  const daysLeft = $derived.by(() => {
    const end = calendarDay(sprint?.end_at, 'instant')
    const today = calendarDay(new Date().toISOString(), 'instant')
    if (!end || !today) return null
    const ms = Date.parse(`${end}T00:00:00Z`) - Date.parse(`${today}T00:00:00Z`)
    return Math.round(ms / 86_400_000)
  })

  /* A sentence, not a "D-6". The abbreviation is a Korean office idiom, and
   * the two locales beside it do not read it at all. */
  const daysLabel = $derived.by(() => {
    const n = daysLeft
    if (n == null) return ''
    if (n === 0) return t('board.sprintEndsToday')
    if (n === 1) return t('board.sprintOneDayLeft')
    if (n > 1) return t('board.sprintDaysLeft', { n })
    if (n === -1) return t('board.sprintEndedYesterday')
    return t('board.sprintEndedAgo', { n: -n })
  })

  /* Ask for the burn-up whenever the sprint on the line changes; the store
   * owns the document and drops answers for ids we have left (GDK-1752). */
  $effect(() => {
    const id = sprint?.id
    if (id == null) burnup.reset()
    else burnup.load(id)
  })

  const total = $derived(sprint?.issue_count ?? 0)
  const done = $derived(sprint?.done ?? 0)
  const inprogress = $derived(sprint?.in_progress ?? 0)
  const todo = $derived(sprint?.todo ?? 0)
  const pct = $derived(total > 0 ? Math.round((done / total) * 100) : 0)

  /* The bar's two filled segments, in the order work moves through them;
   * what is still to do is the track itself, not a third colour. A first
   * cut painted all three and the vision pass read the bar as a blue ribbon
   * before it read it as "30% done" — the unfinished share was the loudest
   * thing on the line. Linear's cycle bar makes the same call. Widths are
   * percentages of the same total the counts came from, so they cannot
   * disagree with the "n / total" beside them; a zero-width segment is
   * dropped rather than rendered as a hairline. */
  const segments = $derived.by(() =>
    (
      [
        { key: 'done' as const, n: done },
        { key: 'inprogress' as const, n: inprogress },
      ] as const
    )
      .filter((s) => s.n > 0)
      .map((s) => ({ ...s, color: categoryMetaOf(s.key).color, w: (s.n / total) * 100 })),
  )

  const barTitle = $derived(
    t('board.sprintBreakdown', { done, inprogress, todo }),
  )

  /* Points are a second unit, drawn only where a workspace actually keeps
   * them. Absent (not zero) on an origin with no `story_points` alias — the
   * demo mirror is one, so nothing here appears on it. */
  const points = $derived.by(() => {
    const p = sprint?.points
    if (p == null) return null
    return t('board.sprintPoints', {
      done: Math.round(sprint?.done_points ?? 0),
      total: Math.round(p),
    })
  })
</script>

{#if sprint}
  <div class="px-3 pt-2" data-testid="sprint-strip">
    <div
      class="flex min-w-0 items-center gap-3 rounded-md bg-bg-elevated px-2.5 py-1 text-micro text-text-secondary"
    >
      <span class="flex-none truncate text-body text-text-primary" data-testid="sprint-strip-name"
        >{sprint.name}</span
      >

      {#if sprint.goal}
        <!-- The goal has been in the mirror since sprints became rows and no
             surface read it (GDK-1695). One line, truncated; the whole of it
             on hover, which is where a long goal belongs. -->
        <span
          class="min-w-0 flex-1 truncate text-text-secondary"
          data-testid="sprint-strip-goal"
          title="{t('board.sprintGoal')}: {sprint.goal}">{sprint.goal}</span
        >
      {/if}

      <span class="flex-none whitespace-nowrap text-text-muted" data-testid="sprint-strip-dates">
        {range}{#if range && daysLabel}<span class="mx-1">·</span>{/if}{daysLabel}
      </span>

      {#if burnup.doc}
        <BurnupSpark doc={burnup.doc} />
      {/if}

      {#if total > 0}
        <!-- The bar takes what is left; the burn-up before it (GDK-1710) is
             what this element gave up the width for. -->
        <span
          class="flex min-w-[80px] flex-1 items-center gap-2"
          data-testid="sprint-strip-bar"
          title={barTitle}
        >
          <span
            class="flex h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-bg-hover"
            role="img"
            aria-label={t('board.sprintProgress', { done, total })}
          >
            {#each segments as seg (seg.key)}
              <span
                class="h-full"
                data-segment={seg.key}
                style:width="{seg.w}%"
                style:background={seg.color}
              ></span>
            {/each}
          </span>
          <span
            class="flex-none tabular-nums text-text-muted"
            data-testid="sprint-strip-count">{done} / {total} · {pct}%</span
          >
        </span>
      {/if}

      {#if points}
        <span class="flex-none tabular-nums text-text-muted" data-testid="sprint-strip-points"
          >{points}</span
        >
      {/if}
    </div>
  </div>
{/if}
