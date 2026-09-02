<script lang="ts">
  /*
   * One card (GDK-1175).
   *
   * Deliberately less than a row. `IssueRow` can show nineteen columns; a
   * card shows five things and stops — key, title, priority, who has touched
   * it, and whether a shell is on it. Linear's board makes the same trade and
   * it is the whole reason a column reads at a glance: density comes from
   * what is left out.
   *
   * The two marks that are ours and not Linear's — the shell edge and the
   * external-move ring — are drawn here but decided elsewhere: the state
   * comes from terminal/strip.ts, and the ring is a data attribute BoardView
   * sets when it flies the card. This component owns no judgment.
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import type { TerminalSessionState } from '../../lib/terminal/strip'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { boardDrag } from '../../lib/board-drag.svelte'
  import { prefetchDetail } from '../../lib/detail-cache.svelte'
  import { shellForIssue } from '../../lib/issue-shells'
  import { shells } from '../../lib/issue-shells.svelte'
  import { enterShell } from '../../lib/terminal/sessions.svelte'
  import PriorityIcon from '../list/PriorityIcon.svelte'
  import Avatar from '../list/Avatar.svelte'
  import Icon from '../ui/Icon.svelte'

  let {
    issue,
    shell = null,
  }: {
    issue: IssueLite
    /** The four states of this issue's shell, or null when none is on it. */
    shell?: TerminalSessionState | null
  } = $props()

  const active = $derived(selection.selectedKey === issue.issue_key)

  /* At most two actors: the point is "an agent is on this", not a roster.
   * `actor_ids` are account ids — the member catalog owns the names. The id
   * stays on as the render key: names are not unique (every deleted account
   * resolves to the same "Former user" string, and one such card took the
   * whole board down through the keyed each — GDK-1218). */
  // Recent actors as faces, not name chips (GDK-1338): a card with "Alex Kim"
  // in a chip and AK in the corner said one person twice. The assignee's own
  // face is the corner one, so it is left out of the actor row.
  const actorIds = $derived(
    (issue.actor_ids ?? []).filter((id) => !issue.assignee_id || id !== issue.assignee_id),
  )
  const actors = $derived(
    actorIds.slice(0, 3).map((id) => {
      const m = issues.memberOfAccountId(id)
      return { id, name: m?.name ?? id, email: m?.email ?? null }
    }),
  )
  const extraActors = $derived(Math.max(0, actorIds.length - actors.length))

  /* `ghost` draws nothing on purpose: it is the session the reap grace is
   * about to take, and a mark for it would be a card decorated with something
   * that is about to stop being true. */
  const edge = $derived(
    shell === 'needs'
      ? 'var(--color-status-stale)'
      : shell === 'running'
        ? 'var(--color-accent)'
        : shell === 'quiet'
          ? 'var(--color-border-strong)'
          : null,
  )
  /* GDK-1197 — the card's way *into* that shell.
   *
   * The edge is 3px of decoration and stays that way: it is too thin to aim
   * at, and making a mark clickable teaches nothing on the way past. This is
   * the session behind it, resolved with the same lookup the body's ▶ uses so
   * the two can never disagree. Gated on `edge` as well, which keeps it off a
   * ghost — the card that draws nothing because its session is about to be
   * reaped is not one to offer a way into. */
  const session = $derived(edge ? shellForIssue(shells.sessions, issue.issue_key) : null)

  const shellTitle = $derived(
    shell === 'needs'
      ? t('board.shellNeeds')
      : shell === 'running'
        ? t('board.shellRunning')
        : shell === 'quiet'
          ? t('board.shellQuiet')
          : '',
  )
</script>

<button
  type="button"
  data-testid="board-card"
  data-board-key={issue.issue_key}
  data-shell={shell ?? undefined}
  data-dragging={boardDrag.draggingKey === issue.issue_key ? '1' : undefined}
  class="board-card group relative w-full flex-none overflow-hidden rounded-md border px-2.5 py-2 text-left
    transition-colors duration-150
    {active
      ? 'border-accent bg-bg-active'
      : 'border-border-subtle bg-bg-panel hover:border-border-strong hover:bg-bg-hover'}"
  onmouseenter={() => prefetchDetail(issue.issue_key)}
  onpointerdown={(e) => boardDrag.start(e, issue)}
  onclick={() => {
    // A finished drag ends in a click on this same button; it was a drop, not a select.
    if (boardDrag.consumeClick()) return
    selection.toggle(issue.issue_key)
  }}
>
  <!-- Shell edge. Absolute so it never joins the flow the FLIP measures. -->
  {#if edge}
    <span
      aria-hidden="true"
      title={shellTitle}
      class="absolute inset-y-0 left-0 w-[3px] {shell === 'running' ? 'board-shell-running' : ''}"
      style:background={edge}
    ></span>
  {/if}

  <div class="flex items-center gap-1.5">
    <span class="min-w-0 flex-1 truncate font-mono text-micro text-text-secondary">
      {issue.issue_key}
    </span>
    {#if session}
      <!-- A <button> here would be a button inside the card's own <button>
           (invalid DOM, and Firefox drops the inner one), so this carries the
           role and the keys itself. Both handlers stop propagation: entering a
           shell is not selecting the card. -->
      <span
        role="button"
        tabindex="0"
        data-testid="board-card-shell-enter"
        title={t('board.openShell')}
        aria-label={t('board.openShell')}
        class="flex h-4 w-4 flex-none items-center justify-center rounded opacity-0 transition-opacity
          hover:bg-bg-hover focus-visible:opacity-100 group-hover:opacity-100"
        style:color={edge ?? undefined}
        onclick={(e) => {
          e.stopPropagation()
          enterShell(session.id)
        }}
        onkeydown={(e) => {
          if (e.key !== 'Enter' && e.key !== ' ') return
          e.preventDefault()
          e.stopPropagation()
          enterShell(session.id)
        }}
      >
        <Icon name="terminal" size={12} />
      </span>
    {/if}
    {#if shell === 'needs'}
      <!-- The one state that is a request rather than a description, so it
           gets the only thing on the card that is allowed to be a signal. -->
      <span
        data-testid="board-card-needs"
        class="h-1.5 w-1.5 flex-none rounded-full"
        style:background="var(--color-status-stale)"
        title={shellTitle}
      ></span>
    {/if}
    <PriorityIcon priority={issue.priority} rank={issue.priority_rank} />
  </div>

  <p class="mt-1 line-clamp-2 text-body leading-snug text-text-primary">
    {issue.summary}
  </p>

  {#if actors.length || issue.assignee}
    <div class="mt-1.5 flex items-center gap-1">
      {#each actors as actor (actor.id)}
        <span
          data-testid="board-actor"
          class="board-actor flex h-4 w-4 flex-none items-center justify-center overflow-hidden rounded-full"
          title={actor.name}
        >
          <Avatar accountId={actor.id} email={actor.email} name={actor.name} size={16} />
        </span>
      {/each}
      {#if extraActors}
        <span class="text-micro tabular-nums text-text-muted">+{extraActors}</span>
      {/if}
      <span class="flex-1"></span>
      {#if issue.assignee}
        <span class="flex h-5 w-5 flex-none items-center justify-center overflow-hidden">
          <Avatar
            email={issue.assignee_email}
            accountId={issue.assignee_id}
            name={issue.assignee}
          />
        </span>
      {/if}
    </div>
  {/if}
</button>
