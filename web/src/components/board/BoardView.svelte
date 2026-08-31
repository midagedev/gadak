<script lang="ts">
  /*
   * The board (GDK-1175) — `filters.groups` laid across the screen instead of
   * down it. Everything upstream of this file is shared with the list: the
   * same filters, the same sort, the same `buildGroups` over the same
   * thirteen axes. A board is a layout, not a data model.
   *
   * ── The asymmetry this file exists for ──
   *
   * A card you moved does not animate. A card somebody else moved flies.
   *
   * That is not a preference; it is what makes the board a monitor. When you
   * transition an issue here, the optimistic patch has already put the card
   * where you told it to go, and replaying that as motion would be the app
   * miming your own gesture back at you a beat late. When `gadak transition`
   * runs in the next window, or an agent moves its own ticket, nothing on
   * this screen was touched — so the movement is the only thing that can
   * tell you it happened. `board-moves.svelte.ts` draws the line, on the
   * evidence the write path already leaves behind.
   *
   * The flight is First-Last-Invert-Play against rects taken in `$effect.pre`
   * (before Svelte mutates the DOM) and played in `$effect` (after). Not
   * `animate:flip`, which only knows about reorder inside one keyed block: a
   * card changing column is a destroy in one `{#each}` and a mount in
   * another, and the two have no ordering guarantee between them. Transform
   * and opacity only — the same discipline as the rest of app.css.
   */
  import { onMount } from 'svelte'
  import { untrack } from 'svelte'
  import { t } from '../../lib/i18n'
  import { filters, type IssueGroup } from '../../stores/filters.svelte'
  import { externalMoves } from '../../lib/board-moves.svelte'
  import { shells } from '../../lib/issue-shells.svelte'
  import { shellStateForIssue } from '../../lib/issue-shells'
  import type { TerminalSessionState } from '../../lib/terminal/strip'
  import { categoryMetaOf } from '../../lib/format'
  import type { StatusCategory } from '../../lib/view-config'
  import { boardDrag } from '../../lib/board-drag.svelte'
  import BoardColumn from './BoardColumn.svelte'
  import BoardDropChoice from './BoardDropChoice.svelte'

  /** How long an externally-moved card takes to cross. Longer than the app's
   *  120–180ms chrome on purpose: this one is meant to be *seen*, and it is
   *  crossing a column, not fading a menu. */
  const FLIGHT_MS = 260
  /** How long the landing ring stays. Long enough to look up at. */
  const LANDED_MS = 1_200

  const CATEGORIES: StatusCategory[] = ['new', 'inprogress', 'done']

  let boardEl = $state<HTMLElement | null>(null)

  // One poll for the whole board (refcounted with the detail panel's).
  onMount(() => shells.track())

  /* Time is re-read when — and only when — the session table is. The four
   * states are a function of (row, now), so pinning now to the poll keeps a
   * card from ageing out of `running` in a frame that had no new evidence. */
  const nowMs = $derived.by(() => {
    void shells.sessions
    return Date.now()
  })
  function shellOf(key: string): TerminalSessionState | null {
    return shellStateForIssue(shells.sessions, key, nowMs)
  }

  const byStatusCategory = $derived(filters.display.group_by === 'status_category')

  /*
   * On the status axis the three columns are always all three, even when one
   * is empty. A column that vanishes when it empties is a column a card
   * cannot be seen arriving in — and "Done is empty" is itself the answer
   * somebody opened the board for.
   */
  const columns = $derived.by((): IssueGroup[] => {
    const groups = filters.groups
    if (!byStatusCategory) return groups
    const found = new Map(groups.map((g) => [g.key, g]))
    const padded = CATEGORIES.map(
      (c) =>
        found.get(c) ?? {
          key: c,
          label: categoryMetaOf(c).label,
          items: [],
          counts: { total: 0, category: { new: 0, inprogress: 0, done: 0 }, severity: {} },
        },
    )
    // Anything the mirror carries that is not one of the three keeps its place
    // after them rather than being dropped.
    return [...padded, ...groups.filter((g) => !CATEGORIES.includes(g.key as StatusCategory))]
  })

  /** Card rects as they were before this layout change. */
  const before = new Map<string, DOMRect>()

  function cardsIn(root: HTMLElement): HTMLElement[] {
    return [...root.querySelectorAll<HTMLElement>('[data-board-key]')]
  }

  function reducedMotion(): boolean {
    return (
      typeof window !== 'undefined' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
    )
  }

  // First + Last: snapshot before Svelte writes the new layout.
  //
  // Only when something is actually flagged. The flag is set inside
  // `applyDelta`, synchronously, before the pool change reaches this
  // component — so by the time this runs, "is anyone flying?" is already
  // answerable, and every other layout change (a keystroke in the search
  // box, a filter chip) skips a forced layout over every card on screen.
  $effect.pre(() => {
    void columns
    const root = boardEl
    if (!root) return
    before.clear()
    if (untrack(() => externalMoves.fresh()).size === 0) return
    for (const el of cardsIn(root)) {
      const key = el.dataset.boardKey
      if (key) before.set(key, el.getBoundingClientRect())
    }
  })

  // Invert + Play, for the flagged keys only. Untracked so consuming a flag
  // cannot re-trigger the effect that consumed it.
  $effect(() => {
    void columns
    const root = boardEl
    if (!root) return
    const moved = untrack(() => externalMoves.fresh())
    if (moved.size === 0) return
    for (const key of moved) {
      untrack(() => externalMoves.clear(key))
      const el = root.querySelector<HTMLElement>(`[data-board-key="${CSS.escape(key)}"]`)
      if (!el) continue
      // The ring is the part that survives reduced motion: it is information
      // about who moved the card, and only the flight is decoration.
      el.dataset.moved = '1'
      setTimeout(() => delete el.dataset.moved, LANDED_MS)
      // GDK-1190: a landing off-screen is a ring nobody sees. External moves
      // only, by construction — your own drop never reaches this loop.
      el.scrollIntoView({ block: 'nearest', inline: 'nearest' })
      const from = before.get(key)
      if (!from || reducedMotion()) continue
      const to = el.getBoundingClientRect()
      const dx = from.left - to.left
      const dy = from.top - to.top
      if (dx === 0 && dy === 0) continue
      el.animate(
        [{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'none' }],
        { duration: FLIGHT_MS, easing: 'ease-out' },
      )
    }
  })
</script>

<div
  bind:this={boardEl}
  data-testid="board"
  class="flex h-full min-h-0 items-stretch overflow-x-auto"
  aria-label={t('board.label')}
>
  {#each columns as group (group.key)}
    <BoardColumn {group} showCategoryCounts={!byStatusCategory} {shellOf} />
  {/each}
  {#if boardDrag.choice}
    <BoardDropChoice choice={boardDrag.choice} />
  {/if}
</div>
