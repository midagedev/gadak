<script lang="ts">
  /*
   * Linked issues ([detail]). Type/direction label + key + summary.
   * Click → selection.select(key): instant if in local pool, else detail loads.
   */
  import { t } from '../../lib/i18n'
  import type { LinkedIssue } from '../../lib/types'
  import { selection } from '../../stores/selection.svelte'
  import { issues } from '../../stores/issues.svelte'

  let { linked }: { linked: LinkedIssue[] } = $props()

  /** Human label from direction/type. Prefer backend direction text when present. */
  function label(l: LinkedIssue): string {
    return (l.direction && l.direction.trim()) || l.type || t('detail.linked')
  }

  // Backend sometimes duplicates the same link (key+direction). Duplicate each keys
  // trip Svelte each_key_duplicate and kill the detail panel (and the app) — dedupe first.
  const uniqueLinked = $derived.by(() => {
    const seen = new Set<string>()
    const out: LinkedIssue[] = []
    for (const l of linked) {
      const k = `${l.key ?? ''}|${l.direction ?? ''}`
      if (seen.has(k)) continue
      seen.add(k)
      out.push(l)
    }
    return out
  })
</script>

{#if uniqueLinked.length === 0}
  <p class="text-[12px] text-text-muted italic">{t('detail.noLinks')}</p>
{:else}
  <ul class="flex flex-col gap-1">
    {#each uniqueLinked as l (l.key + l.direction)}
      <li>
        <button
          type="button"
          onclick={() => selection.select(l.key)}
          class="group flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-bg-hover"
        >
          <span class="mt-px flex-none text-micro text-text-muted">{label(l)}</span>
          <span class="min-w-0 flex-1">
            <span class="flex items-center gap-1.5">
              <span class="font-mono text-micro font-medium text-accent-text">{l.key}</span>
              {#if issues.get(l.key)}
                <span class="h-1 w-1 rounded-full bg-status-done" title={t('detail.inLocalPool')}></span>
              {/if}
            </span>
            <span class="block truncate text-[12px] text-text-secondary group-hover:text-text-primary">
              {l.summary ?? issues.get(l.key)?.summary ?? ''}
            </span>
          </span>
        </button>
      </li>
    {/each}
  </ul>
{/if}
