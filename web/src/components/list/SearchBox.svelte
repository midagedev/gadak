<script lang="ts">
  /*
   * Search box ([explore]) — dual mode (plan §5.2).
   *  ① Instant local search (key/title/assignee/labels) via filters.setQuery → URL, <16ms.
   *  ② Inline tokens: @assignee · #team · !priority · is:reopened|unassigned|stale —
   *     autocomplete then convert to filters.
   *  ③ Issue-key jump (DEN-123/den123).
   *  ④ Enter → server full-text (body/comments).
   *  `/` focuses from anywhere; Esc clears.
   */
  import { t } from '../../lib/i18n'
  import { onMount } from 'svelte'
  import { filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { fieldEnabled, type MultiField } from '../../lib/view-config'
  import Icon from '../ui/Icon.svelte'

  let text = $state(filters.filters.q)
  let inputEl = $state<HTMLInputElement | null>(null)
  let sugIdx = $state(0)

  // Sync input when q changes externally (view apply, etc.) if not typing.
  $effect(() => {
    const q = filters.filters.q
    if (q !== text && document.activeElement !== inputEl) text = q
  })

  interface Sug {
    kind: 'value' | 'flag' | 'jump'
    field?: MultiField
    value: string
    label: string
    hint?: string
  }

  // Last word (token candidate)
  const lastWord = $derived.by(() => {
    const m = text.match(/(\S+)$/)
    return m ? m[1] : ''
  })

  // Issue-key jump candidate (anywhere in the text)
  const jumpKey = $derived.by(() => {
    const m = text.match(/([A-Za-z]{2,10})-?(\d+)/)
    if (!m) return null
    const key = `${m[1].toUpperCase()}-${m[2]}`
    return issues.pool.has(key) ? key : null
  })

  const suggestions = $derived.by<Sug[]>(() => {
    const w = lastWord
    const out: Sug[] = []
    if (w.startsWith('@')) {
      const q = w.slice(1).toLowerCase()
      for (const v of filters.facets.assignee_email) {
        if (!q || v.label.toLowerCase().includes(q)) {
          out.push({ kind: 'value', field: 'assignee_email', value: v.value, label: v.label, hint: `${v.count}` })
        }
        if (out.length >= 8) break
      }
    } else if (w.startsWith('#') && fieldEnabled('team_group')) {
      const q = w.slice(1).toLowerCase()
      for (const v of filters.facets.team_group) {
        if (!q || v.label.toLowerCase().includes(q)) out.push({ kind: 'value', field: 'team_group', value: v.value, label: v.label, hint: `${v.count}` })
        if (out.length >= 8) break
      }
    } else if (w.startsWith('!')) {
      const q = w.slice(1).toLowerCase()
      for (const v of filters.facets.priority) {
        if (!q || v.label.toLowerCase().includes(q)) out.push({ kind: 'value', field: 'priority', value: v.value, label: v.label, hint: `${v.count}` })
        if (out.length >= 8) break
      }
    } else if (w.startsWith('is:')) {
      const q = w.slice(3).toLowerCase()
      for (const [flag, label] of [
        ['reopened', t('filter.flagReopened')],
        ['unassigned', t('filter.flagUnassigned')],
        ['stale', t('filter.flagStale')],
      ] as const) {
        if (!q || flag.includes(q)) out.push({ kind: 'flag', value: flag, label })
      }
    }
    return out
  })

  const showJump = $derived(jumpKey !== null && suggestions.length === 0)

  $effect(() => {
    // Reset highlight when the suggestion list changes
    void suggestions
    sugIdx = 0
  })

  function stripLastWord() {
    text = text.replace(/(\S+)$/, '').replace(/\s+$/, ' ').trimStart()
    filters.setQuery(text)
  }

  function applySug(s: Sug) {
    if (s.kind === 'jump') {
      selection.select(s.value)
      return
    }
    if (s.kind === 'flag') {
      filters.toggleFlag(s.value as 'reopened' | 'unassigned' | 'stale')
    } else if (s.field) {
      filters.addValue(s.field, s.value)
    }
    stripLastWord()
    inputEl?.focus()
  }

  function onInput() {
    filters.setQuery(text)
  }

  function onKeydown(e: KeyboardEvent) {
    if (suggestions.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        sugIdx = (sugIdx + 1) % suggestions.length
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        sugIdx = (sugIdx - 1 + suggestions.length) % suggestions.length
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        applySug(suggestions[sugIdx])
        return
      }
    }
    if (e.key === 'Enter') {
      e.preventDefault()
      if (showJump && jumpKey) {
        selection.select(jumpKey)
      } else if (text.trim()) {
        void filters.runServerSearch()
      }
    } else if (e.key === 'Escape') {
      e.preventDefault()
      if (suggestions.length) {
        // Close token candidates only — keep last word; blur closes the popup
        ;(e.target as HTMLElement).blur()
      } else if (text) {
        text = ''
        filters.setQuery('')
        filters.clearServerSearch()
      } else {
        inputEl?.blur()
      }
    }
  }

  function inEditable(t: EventTarget | null): boolean {
    const el = t as HTMLElement | null
    if (!el) return false
    const tag = el.tagName
    return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable
  }
  function onGlobalKey(e: KeyboardEvent) {
    if (e.key === '/' && !inEditable(e.target)) {
      e.preventDefault()
      inputEl?.focus()
    }
  }
  onMount(() => {
    window.addEventListener('keydown', onGlobalKey)
    return () => window.removeEventListener('keydown', onGlobalKey)
  })
</script>

<div class="relative">
  <div
    class="flex h-control items-center gap-2 rounded-md border border-border-strong/70 bg-bg-elevated px-3 shadow-sm shadow-black/10 focus-within:border-accent/70"
  >
    <Icon name="search" size={14} class="text-text-muted" />
    <input
      bind:this={inputEl}
      bind:value={text}
      oninput={onInput}
      onkeydown={onKeydown}
      type="text"
      placeholder={t('list.searchPlaceholder')}
      title={t('list.searchHelp')}
      class="min-w-0 flex-1 bg-transparent text-[13px] text-text-primary placeholder:text-text-muted focus:outline-none"
      spellcheck="false"
      autocomplete="off"
    />
    <button
      type="button"
      class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded text-[11px] font-medium text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
      title={t('list.searchHelp')}
      aria-label={t('list.searchHelp')}
      data-testid="search-help"
    >
      ?
    </button>
    {#if filters.searching}
      <span class="flex-none text-[11px] text-text-muted">{t('list.searching')}</span>
    {:else if text}
      <button
        type="button"
        class="flex-none text-text-muted hover:text-text-primary"
        onclick={() => {
          text = ''
          filters.setQuery('')
          filters.clearServerSearch()
          inputEl?.focus()
        }}
        title={t('list.searchClear')}
      >
        ✕
      </button>
    {:else}
      <kbd class="flex-none rounded border border-border-subtle px-1 text-micro text-text-muted">/</kbd>
    {/if}
  </div>

  <!-- Token autocomplete / jump -->
  {#if suggestions.length > 0}
    <div
      class="anim-enter absolute left-0 top-full z-30 mt-1 w-full max-w-md rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-overlay"
    >
      {#each suggestions as s, i (s.kind + s.value)}
        <button
          type="button"
          class="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-[12px] {i === sugIdx
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover'}"
          onmousedown={(e) => {
            e.preventDefault()
            applySug(s)
          }}
        >
          <span class="min-w-0 flex-1 truncate">{s.label}</span>
          {#if s.hint}<span class="flex-none text-[11px] text-text-muted">{s.hint}</span>{/if}
        </button>
      {/each}
    </div>
  {:else if showJump && jumpKey}
    <div
      class="anim-enter absolute left-0 top-full z-30 mt-1 w-full max-w-md rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-overlay"
    >
      <button
        type="button"
        class="flex w-full items-center gap-2 rounded bg-bg-active px-2 py-1 text-left text-[12px] text-text-primary"
        onmousedown={(e) => {
          e.preventDefault()
          if (jumpKey) selection.select(jumpKey)
        }}
      >
        <span class="font-mono text-accent-text">{jumpKey}</span>
        <span class="min-w-0 flex-1 truncate text-text-secondary">
          {issues.pool.get(jumpKey)?.summary ?? ''}
        </span>
        <span class="flex-none text-[11px] text-text-muted">{t('list.searchOpen')}</span>
      </button>
    </div>
  {/if}
</div>
