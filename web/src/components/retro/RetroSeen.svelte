<script lang="ts">
  /*
   * Seen and moved (GDK-1725).
   *
   * Two counts nothing else on this screen can produce, because they need
   * both halves: the mirror's changelog and this machine's own reading
   * history. Attention and movement come apart quietly — a week can go into
   * issues that never moved, and issues can move that nobody here looked at
   * — and neither direction shows up in a closed count.
   *
   * Deliberately two numbers and no list: naming twelve issues here would
   * make this the biggest section on the report for the smallest of its
   * questions. The numbers are doors; the list is one click away, where
   * every other list on this report lives.
   *
   * When there is no reading history the server says so in `notes`, which
   * the report already prints under the table. This section then draws
   * nothing rather than two confident zeroes.
   */
  import { t } from '../../lib/i18n'
  import { setCount } from './materials'
  import type { RetroBucket } from '../../lib/types'

  let {
    bucket,
    onOpen,
  }: {
    bucket: RetroBucket
    onOpen: (keys: string[]) => void
  } = $props()

  const rows = $derived(
    [
      { id: 'seen-not-moved', key: 'retro.seen.notMoved' as const, set: bucket.seen_not_moved },
      { id: 'moved-not-seen', key: 'retro.moved.notSeen' as const, set: bucket.moved_not_seen },
    ]
      .filter((r) => r.set !== undefined)
      .map((r) => ({ ...r, label: t(r.key, { n: setCount(r.set) }) })),
  )
</script>

<div class="flex flex-wrap gap-6" data-testid="retro-seen">
  {#each rows as r (r.id)}
    {@const n = setCount(r.set)}
    <button
      type="button"
      class="rounded px-1 text-left text-micro text-text-secondary underline decoration-border-strong decoration-dotted underline-offset-4 hover:bg-bg-hover hover:decoration-accent disabled:no-underline disabled:opacity-60"
      data-testid="retro-seen-count"
      data-kind={r.id}
      disabled={n === 0}
      onclick={() => onOpen(r.set?.keys ?? [])}
    >
      {r.label}
    </button>
  {/each}
</div>
