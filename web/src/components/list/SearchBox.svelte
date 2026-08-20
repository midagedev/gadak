<script lang="ts">
  /*
   * Search box ([explore]) — dual mode (plan §5.2).
   *  ① Instant local search (key/title/assignee/labels) via filters.setQuery → URL, <16ms.
   *  ② Inline tokens: @assignee · #team · !priority · is:reopened|unassigned|stale —
   *     autocomplete then convert to filters.
   *  ③ Issue-key jump (DEN-123/den123).
   *  ④ Paste or Enter of JQL / a Jira jql= URL → POST jql/, apply chips.
   *  ⑤ Enter on ordinary text → server full-text (body/comments).
   *  `/` focuses from anywhere; Esc clears.
   */
  import { onMount } from 'svelte'
  import { t } from '../../lib/i18n'
  import { parseJql } from '../../lib/api'
  import { applyOmniboxAction, classifyOmnibox } from '../../lib/omnibox'
  import { applyServerSearchOutcome } from '../../lib/server-search'
  import { filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { me } from '../../stores/me.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { write } from '../../stores/write.svelte'
  import { fieldEnabled, type MultiField } from '../../lib/view-config'
  import { paletteShortcutLabel, requestOpenPalette, requestOpenShortcuts } from '../../lib/unified-search'
  import { onEscape, onOutsideClick } from '../../lib/dom-actions'
  import { createCompositionCommit } from '../../lib/composition-commit'
  import Icon from '../ui/Icon.svelte'

  let text = $state(filters.filters.q)
  let inputEl = $state<HTMLInputElement | null>(null)
  let sugIdxRaw = $state(0)

  // GDK-463: ≤960px the long placeholder clips inside the toolbar field.
  // Match the catalog switch to the viewport, not to overlay-regime (1100).
  const NARROW_PLACEHOLDER_MQ = '(max-width: 960px)'
  let narrowPlaceholder = $state(false)
  onMount(() => {
    const mq = window.matchMedia(NARROW_PLACEHOLDER_MQ)
    const apply = () => (narrowPlaceholder = mq.matches)
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  })

  /*
   * The `?` spells out what this box accepts. On a mouse the title does it on
   * hover; a touch screen has no hover, so the same string also has to be
   * reachable by tapping. It shares the one popover slot under the field with
   * the token candidates below — help wins it while it is open, and typing
   * gives it back.
   */
  let helpOpen = $state(false)

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

  /*
   * Which candidate the arrows are on. Stored raw and read clamped, because the
   * list is rebuilt from the facets as well as from what has been typed — a
   * background sync can shorten it under a highlight the reader has already
   * moved, and an index past the end would apply a suggestion that is not on
   * screen. Typing sends the highlight home; anything else can only pull it back
   * to the last row.
   */
  const sugIdx = $derived(Math.min(sugIdxRaw, suggestions.length - 1))

  function stripLastWord() {
    text = text.replace(/(\S+)$/, '').replace(/\s+$/, ' ').trimStart()
    filters.setQuery(text)
    sugIdxRaw = 0
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

  // GDK-169: mid-composition states are never committed as queries.
  const ime = createCompositionCommit((q) => {
    filters.setQuery(q)
  })

  function onInput(e: Event) {
    ime.oninput(e, text)
    sugIdxRaw = 0
    helpOpen = false
  }

  function onCompositionEnd(e: CompositionEvent) {
    ime.oncompositionend(e, text)
  }

  let applyingJql = $state(false)

  async function applyJql(raw: string): Promise<boolean> {
    if (applyingJql) return true
    applyingJql = true
    try {
      const res = await parseJql(raw, me.email)
      if (res.error === 'not_jql') {
        write.toast(res.message || t('filter.jqlParseFailed'), 'info')
        return true
      }
      if (res.error) {
        write.toast(res.message || t('filter.jqlParseFailed'), 'error')
        return true
      }
      filters.applyJqlResult(res)
      text = res.filters?.q ?? ''
      if (res.unsupported?.length) {
        write.toast(t('filter.jqlPartial', { clauses: res.unsupported.join('; ') }), 'info')
      } else {
        write.toast(t('filter.jqlApplied'), 'success')
      }
      return true
    } catch {
      write.toast(t('filter.jqlNotAvailable'), 'error')
      return true
    } finally {
      applyingJql = false
    }
  }

  async function onPaste(e: ClipboardEvent) {
    const raw = e.clipboardData?.getData('text') ?? ''
    const action = classifyOmnibox(raw)
    if (action.kind === 'text') return
    e.preventDefault()
    await applyOmniboxAction(action, applyJql)
  }

  function onKeydown(e: KeyboardEvent) {
    // IME confirm (Enter) / next-candidate (Tab) must not run jump or
    // server-search — preventDefault here would also steal the key from the IME.
    if ((e.isComposing || ime.composing) && (e.key === 'Enter' || e.key === 'Tab')) {
      return
    }
    if (suggestions.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        sugIdxRaw = (sugIdx + 1) % suggestions.length
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        sugIdxRaw = (sugIdx - 1 + suggestions.length) % suggestions.length
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
        void (async () => {
          const handled = await applyOmniboxAction(classifyOmnibox(text), applyJql)
          if (!handled) applyServerSearchOutcome(await filters.runServerSearch())
        })()
      }
    } else if (e.key === 'Escape') {
      e.preventDefault()
      if (helpOpen) {
        // The help is over the field; the first Esc gives that slot back
        // before Esc goes back to meaning "clear the query".
        helpOpen = false
      } else if (suggestions.length) {
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

  // `/` is not bound here. It belongs to whatever narrowing field the screen has
  // — this box on the list, the filter on a document screen — so the binding
  // lives in the shell's one window handler (lib/keymap.svelte.ts) and finds
  // this input by its testid.
</script>

<!-- flex-wrap + the input's min-width floor: the field's fixed innards (icon,
     `?`, kbd, padding) are ~90px, so letting it shrink to 0 paints them over
     the palette button (seen at 1120 docked, GDK-201). Below the floor the
     button wraps under the field instead of being overlapped. -->
<div class="flex flex-wrap items-center gap-2">
  <!-- The boundary for the help popover's outside click: it has to hold the
       `?` too, or the click that closes the panel would reopen it. -->
  <div
    class="relative min-w-[150px] flex-1"
    use:onOutsideClick={{ handler: () => (helpOpen = false), enabled: helpOpen }}
  >
  <div
    class="flex h-control items-center gap-2 rounded-md border border-border-strong/70 bg-bg-elevated px-3 shadow-sm shadow-black/10 focus-within:border-accent/70"
  >
    <Icon name="search" size={14} class="text-text-muted" />
    <input
      bind:this={inputEl}
      bind:value={text}
      oninput={onInput}
      oncompositionstart={ime.oncompositionstart}
      oncompositionend={onCompositionEnd}
      onpaste={onPaste}
      onkeydown={onKeydown}
      type="text"
      data-testid="search-input"
      placeholder={t(narrowPlaceholder ? 'list.searchPlaceholderShort' : 'list.searchPlaceholder')}
      title={t('list.searchHelp', { shortcut: paletteShortcutLabel() })}
      class="min-w-0 flex-1 bg-transparent text-body text-text-primary placeholder:text-text-muted focus:outline-none"
      spellcheck="false"
      autocomplete="off"
    />
    <button
      type="button"
      class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded text-micro font-medium text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
      title={t('list.searchHelp', { shortcut: paletteShortcutLabel() })}
      aria-label={t('list.searchHelp', { shortcut: paletteShortcutLabel() })}
      aria-expanded={helpOpen}
      data-testid="search-help"
      onclick={() => (helpOpen = !helpOpen)}
    >
      ?
    </button>
    {#if filters.searching}
      <span class="flex-none text-micro text-text-muted">{t('list.searching')}</span>
    {:else if text}
      <button
        type="button"
        class="flex flex-none items-center text-text-muted hover:text-text-primary"
        onclick={() => {
          text = ''
          filters.setQuery('')
          filters.clearServerSearch()
          inputEl?.focus()
        }}
        title={t('list.searchClear')}
      >
        <Icon name="x" size={13} />
      </button>
    {:else}
      <kbd class="flex-none rounded border border-border-subtle px-1 text-micro text-text-muted">/</kbd>
    {/if}
  </div>

  <!-- Help (tapped `?`) / token autocomplete / jump — one slot under the field -->
  {#if helpOpen}
    <div
      use:onEscape={(e) => {
        e.preventDefault()
        helpOpen = false
      }}
      class="anim-enter absolute left-0 top-full z-30 mt-10 w-full max-w-md rounded-lg border border-border-strong bg-bg-elevated p-2 text-[12px] leading-relaxed text-text-secondary shadow-overlay"
      data-testid="search-help-panel"
    >
      <p>{t('list.searchHelp', { shortcut: paletteShortcutLabel() })}</p>
      <button
        type="button"
        class="mt-1.5 text-left text-micro text-accent-text transition-colors hover:underline"
        data-testid="search-help-shortcuts"
        onclick={() => {
          helpOpen = false
          requestOpenShortcuts()
        }}
      >
        {t('list.searchHelpShortcuts')}
      </button>
    </div>
  {:else if suggestions.length > 0}
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
          {#if s.hint}<span class="flex-none text-micro text-text-muted">{s.hint}</span>{/if}
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
        <span class="flex-none text-micro text-text-muted">{t('list.searchOpen')}</span>
      </button>
    </div>
  {/if}
  </div>
  <button
    type="button"
    data-testid="palette-open"
    class="flex h-control flex-none items-center gap-1.5 rounded-md border border-border-strong/70 bg-bg-elevated px-2.5 text-[12px] text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary"
    title={t('palette.entryTitle', { shortcut: paletteShortcutLabel() })}
    aria-label={t('palette.entryTitle', { shortcut: paletteShortcutLabel() })}
    onclick={() => requestOpenPalette()}
  >
    <Icon name="search" size={14} class="text-text-muted" />
    {#if !narrowPlaceholder}
      <span>{t('palette.entryLabel')}</span>
    {/if}
    <kbd class="rounded border border-border-subtle px-1 text-micro text-text-muted">{paletteShortcutLabel()}</kbd>
  </button>
</div>
