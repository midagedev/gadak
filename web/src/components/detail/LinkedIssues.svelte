<script lang="ts">
  /*
   * Linked issues ([detail]). Type/direction label + key + summary.
   * Click → selection.select(key): instant if in local pool, else detail loads.
   * GDK-85: add form (type + key) writes through POST <key>/link/.
   */
  import { t } from '../../lib/i18n'
  import { writeErrorMessage } from '../../lib/i18n/en'
  import type { LinkedIssue } from '../../lib/types'
  import { ApiError, createIssueLink, getIssueLinkTypes, type IssueLinkType } from '../../lib/api'
  import { isHostedDemo } from '../../lib/config'
  import { invalidate } from '../../lib/detail-cache.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'

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

  const issueKey = $derived(selection.selectedKey)
  const linear = $derived(issueKey ? issues.get(issueKey)?.source === 'linear' : false)

  let types = $state<IssueLinkType[]>([])
  let catalogFor = $state('')
  let selectedType = $state('')
  let keyQuery = $state('')
  let busy = $state(false)

  function linkTypeOptions(rows: IssueLinkType[]): { value: string; label: string }[] {
    const out: { value: string; label: string }[] = []
    for (const row of rows) {
      const outward = (row.outward || row.name || row.id).trim()
      const inward = (row.inward || '').trim()
      if (outward) out.push({ value: outward, label: outward })
      if (inward && inward.toLowerCase() !== outward.toLowerCase()) {
        out.push({ value: inward, label: inward })
      }
    }
    return out
  }

  const typeOptions = $derived(linkTypeOptions(types))

  // Catalog fetch is I/O. Writes live in loadTypes (GDK-692).
  $effect(() => {
    const k = issueKey
    if (!k || linear) return
    if (k === catalogFor) return
    void loadTypes(k)
  })

  function setTypes(rows: IssueLinkType[]) {
    types = rows
    // Same turn as types so bind:value={selectedType} matches an option.
    if (!selectedType) selectedType = linkTypeOptions(rows)[0]?.value ?? ''
  }

  async function loadTypes(k: string) {
    catalogFor = k
    types = []
    selectedType = ''
    if (isHostedDemo()) {
      setTypes([{ id: '10000', name: 'Blocks', inward: 'is blocked by', outward: 'blocks' }])
      return
    }
    try {
      const res = await getIssueLinkTypes(k)
      setTypes(res.link_types ?? [])
    } catch {
      setTypes([])
    }
  }

  const keyCands = $derived.by(() => {
    const q = keyQuery.trim().toLowerCase()
    const self = issueKey
    const out: { issue_key: string; summary: string | null }[] = []
    for (const it of issues.allIssues) {
      if (self && it.issue_key === self) continue
      if (q) {
        const keyHit = it.issue_key.toLowerCase().startsWith(q)
        const summaryHit = (it.summary ?? '').toLowerCase().startsWith(q)
        if (!keyHit && !summaryHit) continue
      }
      out.push({ issue_key: it.issue_key, summary: it.summary ?? null })
      if (out.length >= 8) break
    }
    return out
  })

  async function submit() {
    const from = issueKey
    if (!from) return
    const token = selectedType.trim()
    const to = keyQuery.trim().toUpperCase()
    if (!token || !to) return
    if (to === from.toUpperCase()) {
      write.toast(t('detail.linkSelf'), 'error')
      return
    }
    if (!(await write.ensureWritableFor(from))) return
    if (isHostedDemo()) {
      write.toast(t('app.demoWriteNotice'), 'info')
      return
    }
    if (typeOptions.length === 0) {
      await loadTypes(from)
    }
    busy = true
    try {
      await createIssueLink(from, token, to)
      keyQuery = ''
      invalidate(from)
      invalidate(to)
      write.bumpDetail()
    } catch (e) {
      const fallback = t('detail.linkAddFailed')
      if (e instanceof ApiError) {
        write.toast(writeErrorMessage(e.code, fallback, t), 'error')
      } else {
        write.toast(fallback, 'error')
      }
    } finally {
      busy = false
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    void submit()
  }
</script>

<div class="flex flex-col gap-1" data-testid="linked-issues">
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
            <span class="block truncate text-body text-text-secondary group-hover:text-text-primary">
              {l.summary ?? issues.get(l.key)?.summary ?? ''}
            </span>
          </span>
        </button>
      </li>
    {/each}
  </ul>

  {#if !linear}
    <form
      class="flex flex-col gap-1 {uniqueLinked.length ? 'mt-2' : ''}"
      data-testid="linked-issues-add"
      onsubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <label class="flex flex-col gap-1">
        <span class="text-micro text-text-muted">{t('detail.linkType')}</span>
        <select
          bind:value={selectedType}
          disabled={busy || typeOptions.length === 0}
          data-testid="linked-issues-type"
          class="w-full rounded border border-border-subtle bg-bg-base px-2 py-1 text-body text-text-primary focus:border-accent focus:outline-none disabled:opacity-50"
        >
          {#each typeOptions as opt (opt.value)}
            <option value={opt.value}>{opt.label}</option>
          {/each}
        </select>
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-micro text-text-muted">{t('detail.linkKey')}</span>
        <input
          bind:value={keyQuery}
          type="text"
          disabled={busy}
          placeholder={t('fieldEditor.searchOptions')}
          data-testid="linked-issues-key"
          class="w-full rounded border border-border-subtle bg-bg-base px-2 py-1 text-body text-text-primary placeholder:text-text-muted focus:border-accent focus:outline-none disabled:opacity-50"
          onkeydown={onKeydown}
        />
      </label>
      {#if keyQuery.trim()}
        <ul class="flex flex-col">
          {#each keyCands as it (it.issue_key)}
            <li>
              <button
                type="button"
                disabled={busy}
                onclick={() => {
                  keyQuery = it.issue_key
                }}
                class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-body text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary disabled:opacity-50"
              >
                <span class="flex-none font-mono text-micro">{it.issue_key}</span>
                {#if it.summary}
                  <span class="min-w-0 flex-1 truncate">{it.summary}</span>
                {/if}
              </button>
            </li>
          {/each}
          {#if keyCands.length === 0}
            <li class="px-2 py-1.5 text-micro text-text-muted">{t('common.noResults')}</li>
          {/if}
        </ul>
      {/if}
      <button
        type="submit"
        disabled={busy || !selectedType.trim() || !keyQuery.trim()}
        data-testid="linked-issues-submit"
        class="self-start rounded-md px-2 py-1 text-micro text-text-secondary transition-colors hover:bg-bg-hover disabled:opacity-50"
      >
        {t('detail.linkAdd')}
      </button>
    </form>
  {/if}
</div>
