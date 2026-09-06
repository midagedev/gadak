<script module>
  import type { SessionDelta } from '../../lib/session-strip'

  /*
   * Session strip ([list]) — tab-scoped state. The strip speaks once, at the
   * session-start boundary (G3): the count is a snapshot taken the moment
   * bootstrap lands the boundary, and dismissal lasts the tab's life. A
   * remount (screen switch and back) must re-derive neither — a $derived over
   * the pool would grow the count with every mid-session delta, and a
   * re-appearing strip would be the interruption the boundary rule forbids.
   */
  let snapshot: { since: string; delta: SessionDelta | null } | null = $state(null)
  let dismissed = $state(false)
  let computed = false
</script>

<script lang="ts">
  /*
   * One quiet line above the list: what changed since the previous session
   * (spec r2-session; THEORY.md "Session start", T2/T3; UX §14 G1/G3/G4/G5/G7).
   *
   * All decisions live in lib/session-strip.ts (pure); this component owns
   * the once-per-tab timing, the click, and the quiet form. Absence is the
   * design: no boundary, no changes, or a click → nothing rendered, no empty
   * state. The answer is an arrangement (G4): the click applies a keys view —
   * only the changed issues, statuses untouched (a closure is a change worth
   * seeing).
   *
   * Form (C4): ResumeCard's classes on a native button — elevated chip, micro
   * secondary text, no icon, no border, no new colour. Hover states the basis
   * (G7): the absolute time the previous session ended.
   */
  import { t, relativeTime, absTime } from '../../lib/i18n'
  import { changedSince, stripLabel, viewKeys } from '../../lib/session-strip'
  import { emptyConfig } from '../../lib/view-config'
  import { filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'

  // Wait for the boundary and the identity in one order-independent pass:
  // bootstrap can land before or after auth/me resolves, and the mine count
  // needs the resolved identity. The `computed` latch is what makes this a
  // snapshot — the effect re-runs on every delta, the guard stops it from
  // ever recomputing.
  $effect(() => {
    if (computed) return
    const since = issues.lastSessionEndedAt
    if (!since || !me.authChecked) return
    computed = true
    snapshot = {
      since,
      delta: changedSince(issues.pool.values(), since, {
        accountId: me.accountId,
        email: me.email,
      }),
    }
  })

  const label = $derived(
    snapshot?.delta && !dismissed
      ? stripLabel(
          snapshot.delta,
          relativeTime(snapshot.since),
          t,
          snapshot.delta.mine > 0 ? { accountId: me.accountId, email: me.email } : null,
        )
      : '',
  )

  /** G4: the click is an arrangement — the view becomes exactly the changed
   *  issues (capped at KEYS_CAP the way any keys view is), nothing else. */
  function showChanged(): void {
    if (!snapshot?.delta) return
    dismissed = true
    const cfg = emptyConfig()
    cfg.filters.keys = viewKeys(snapshot.delta.keys).keys
    filters.applyConfig(cfg)
  }
</script>

{#if snapshot?.delta && !dismissed}
  <div class="px-3 pt-2">
    <button
      type="button"
      data-testid="session-strip"
      class="block max-w-full truncate rounded-md bg-bg-elevated px-2 py-0.5 text-left text-micro text-text-secondary hover:bg-bg-hover"
      title={absTime(snapshot.since)}
      onclick={showChanged}
    >{label}</button>
  </div>
{/if}
