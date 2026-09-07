<script lang="ts">
  /*
   * Detail-header label row: chips you can filter on or remove, plus an add
   * field. Suggestions are the same ranking as the new-issue dialog — recent
   * first, then this project's frequency. Writes go through write.setLabels
   * (full-array replace, optimistic).
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { recentOf } from '../../lib/recency'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'
  import { DETAIL_TESTID } from '../../lib/commands'
  import Icon from '../ui/Icon.svelte'

  /** bare: no inline label — the caller's <dt> names the row (GDK-1337). */
  let { issue, bare = false }: { issue: IssueLite; bare?: boolean } = $props()

  let adding = $state(false)
  let labelInput = $state('')
  let busy = $state(false)
  let inputEl: HTMLInputElement | null = $state(null)

  const projectKey = $derived(write.projectOf(issue))

  const labelFreq = $derived.by(() => {
    const freq = new Map<string, number>()
    for (const it of issues.allIssues) {
      if (write.projectOf(it) !== projectKey) continue
      for (const l of it.labels) freq.set(l, (freq.get(l) ?? 0) + 1)
    }
    return freq
  })

  const typed = $derived(labelInput.trim().replace(/\s+/g, '-'))

  const suggestions = $derived.by<string[]>(() => {
    const q = typed.toLowerCase()
    const recent = recentOf('label')
    const all = new Set<string>([...recent, ...labelFreq.keys()])
    const arr = [...all].filter(
      (l) => !issue.labels.includes(l) && (!q || l.toLowerCase().includes(q)),
    )
    arr.sort((a, b) => {
      const ra = recent.indexOf(a)
      const rb = recent.indexOf(b)
      const rra = ra === -1 ? Infinity : ra
      const rrb = rb === -1 ? Infinity : rb
      if (rra !== rrb) return rra - rrb
      return (labelFreq.get(b) ?? 0) - (labelFreq.get(a) ?? 0)
    })
    return arr.slice(0, 8)
  })

  const canCreate = $derived(Boolean(typed && !issue.labels.includes(typed)))

  async function openAdd() {
    if (!(await write.ensureWritableFor(issue.issue_key))) return
    adding = true
    labelInput = ''
    queueMicrotask(() => inputEl?.focus())
  }

  function closeAdd() {
    adding = false
    labelInput = ''
  }

  async function add(raw: string) {
    const v = raw.trim().replace(/\s+/g, '-')
    if (!v || issue.labels.includes(v) || busy) {
      labelInput = ''
      return
    }
    busy = true
    const ok = await write.setLabels(issue.issue_key, [...issue.labels, v])
    busy = false
    if (ok) labelInput = ''
  }

  async function remove(label: string) {
    if (busy) return
    busy = true
    await write.setLabels(
      issue.issue_key,
      issue.labels.filter((x) => x !== label),
    )
    busy = false
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      void add(suggestions[0] ?? typed)
    } else if (e.key === 'Escape') {
      // Spend the key so the detail panel behind this row stays open.
      e.preventDefault()
      closeAdd()
    }
  }

  function onTyped(e: Event) {
    const el = e.currentTarget as HTMLInputElement
    // Jira rejects spaces in a label; show the hyphen as they type.
    const next = el.value.replace(/\s+/g, '-')
    if (next !== el.value) el.value = next
    labelInput = next
  }
</script>

<div
  class="relative flex items-start gap-2"
  data-testid="label-editor"
  use:onOutsideClick={{ handler: closeAdd, enabled: adding }}
>
  {#if !bare}<span class="w-12 flex-none pt-0.5 text-text-muted">{t('common.labels')}</span>{/if}
  <div class="flex min-w-0 flex-1 flex-wrap items-center gap-1">
    {#each issue.labels as l (l)}
      <span
        class="inline-flex max-w-full items-center gap-0.5 rounded bg-bg-elevated px-1.5 py-0.5 text-text-secondary"
        data-testid="label-editor-chip"
      >
        <button
          type="button"
          class="min-w-0 truncate transition-colors hover:text-text-primary"
          title={t('list.fieldValue', { field: t('common.labels'), value: l })}
          onclick={() => filters.addValue('labels', l)}
        >
          {l}
        </button>
        <button
          type="button"
          class="flex flex-none items-center text-text-muted hover:text-status-reopen disabled:opacity-50"
          title={t('write.removeLabel', { label: l })}
          aria-label={t('write.removeLabel', { label: l })}
          disabled={busy}
          onclick={() => void remove(l)}
        >
          <Icon name="x" size={11} />
        </button>
      </span>
    {/each}

    {#if adding}
      <input
        bind:this={inputEl}
        value={labelInput}
        oninput={onTyped}
        onkeydown={onKeydown}
        type="text"
        data-testid={DETAIL_TESTID.labelInput}
        placeholder={t('write.addLabel')}
        disabled={busy}
        class="h-[22px] w-36 flex-none rounded border border-border-strong bg-bg-base px-1.5 text-body text-text-primary outline-none placeholder:text-text-muted focus:border-accent disabled:opacity-50"
      />
    {:else if issue.labels.length === 0}
      <button
        type="button"
        data-testid={DETAIL_TESTID.labelAdd}
        onclick={() => void openAdd()}
        class="rounded-md px-1 py-0.5 text-left text-text-muted italic transition-colors hover:bg-bg-hover hover:text-text-primary"
      >
        {t('write.addLabel')}
      </button>
    {:else}
      <button
        type="button"
        data-testid={DETAIL_TESTID.labelAdd}
        onclick={() => void openAdd()}
        class="flex h-5 w-5 flex-none items-center justify-center rounded text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
        title={t('write.addLabel')}
        aria-label={t('write.addLabel')}
      >
        <Icon name="plus" size={12} />
      </button>
    {/if}
  </div>

  {#if adding && (suggestions.length > 0 || canCreate)}
    <div
      use:onEscape={(e) => {
        e.preventDefault()
        closeAdd()
      }}
      class="absolute left-12 top-full z-30 mt-1 w-56 rounded-lg border border-border-strong bg-bg-elevated py-1 shadow-overlay"
      aria-label={t('common.labels')}
    >
      {#each suggestions as l (l)}
        <button
          type="button"
          onclick={() => void add(l)}
          disabled={busy}
          class="flex w-full items-center justify-between gap-2 px-3 py-1 text-left text-body text-text-secondary hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
        >
          <span class="min-w-0 flex-1 truncate">{l}</span>
          {#if labelFreq.get(l)}
            <span class="flex-none text-micro text-text-muted">{labelFreq.get(l)}</span>
          {/if}
        </button>
      {/each}
      {#if canCreate && !suggestions.includes(typed)}
        <button
          type="button"
          onclick={() => void add(typed)}
          disabled={busy}
          class="flex w-full items-center px-3 py-1 text-left text-body text-text-primary hover:bg-bg-hover disabled:opacity-50"
        >
          {t('write.addLabelNamed', { label: typed })}
        </button>
      {/if}
    </div>
  {/if}
</div>
