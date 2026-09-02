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
  import { isHostedDemo } from '../../lib/config'
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

  // GDK-463: ≤960px the toolbar is compact — hide the palette entry's label.
  // Viewport-based on purpose (overlay-regime is 1100): it sizes a sibling
  // button, where an input-width trigger would feed back through the layout
  // (label hides → field grows → trigger flips → label returns → …).
  const NARROW_TOOLBAR_MQ = '(max-width: 960px)'
  let viewportNarrow = $state(false)
  onMount(() => {
    const mq = window.matchMedia(NARROW_TOOLBAR_MQ)
    const apply = () => (viewportNarrow = mq.matches)
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  })

  /*
   * GDK-1056: a docked detail panel or a terminal split shrinks this input
   * without touching the viewport, so the placeholder switch measures the
   * field itself — the full copy renders only where it actually fits. Same
   * font recipe the e2e fits-check uses (e2e/ux-f7.spec.ts); +8px slack so
   * sub-pixel rounding at the boundary cannot flicker the switch.
   */
  let narrowPlaceholder = $state(false)
  /* The suggestion and jump lists hang under the field only while it has
     focus. They are absolute and z-30, and since GDK-1336 the Breakdown band
     sits directly under the toolbar — an open jump card after the reader had
     clicked into the list was covering that band's controls. */
  let focused = $state(false)
  $effect(() => {
    const el = inputEl
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => {
      const cs = getComputedStyle(el)
      const ctx = document.createElement('canvas').getContext('2d')
      if (!ctx) return
      ctx.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`.trim()
      narrowPlaceholder = ctx.measureText(t('list.searchPlaceholder')).width + 8 > el.clientWidth
    })
    ro.observe(el)
    return () => ro.disconnect()
  })

  /*
   * GDK-1059: a docked detail panel narrows this whole slot (flex-1 of the
   * toolbar row), not the viewport, and below ~350px the palette button's
   * full label no longer fit beside the field's 150px floor — the button
   * wrapped to a lone second row (GDK-201's fallback firing one regime too
   * early). Compact it to icon+shortcut, the render ≤960 already uses.
   * Measured on the slot itself, and the full label's width is cached only
   * while it is rendered: the slot's width is layout-determined (its parent
   * has an explicit min-width, so no content-based minimum feeds back), and
   * a decision made from the cached full width cannot oscillate when the
   * button then shrinks. A stale cache after a locale switch while compact
   * only errs toward staying compact — the safe direction.
   */
  let slotCompact = $state(false)
  let boxEl = $state<HTMLDivElement | null>(null)
  // The palette button by reference, not by its e2e testid: data-testid is
  // the e2e contract, and runtime logic leaning on the same selector means
  // renaming a test hook breaks the layout switch.
  let paletteBtn = $state<HTMLButtonElement | null>(null)
  $effect(() => {
    const el = boxEl
    if (!el || typeof ResizeObserver === 'undefined') return
    let fullLabelWidth = 0
    const ro = new ResizeObserver(() => {
      const button = paletteBtn
      if (button && button.querySelector('span')) {
        fullLabelWidth = Math.max(fullLabelWidth, button.offsetWidth)
      }
      if (!fullLabelWidth) return
      const field = el.firstElementChild as HTMLElement | null
      const floor = field ? parseFloat(getComputedStyle(field).minWidth) || 0 : 0
      const gap = parseFloat(getComputedStyle(el).columnGap) || 0
      slotCompact = el.clientWidth < floor + gap + fullLabelWidth + 8
    })
    ro.observe(el)
    return () => ro.disconnect()
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
        write.toast(t('filter.notJql'), 'info')
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
      if (isHostedDemo()) write.toast(t('filter.jqlNotAvailable'), 'info')
      else write.toast(t('filter.jqlFailed'), 'error')
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
     the palette button (seen at 1120 docked, GDK-201). The floor is 200 so the
     short placeholder ("Search this list…", ~105px) always renders unclipped
     — since GDK-1336 the field shares its row with every list control and is
     at the floor far more often. GDK-1059: the palette
     button compacts to icon+shortcut before that point, so the wrap below the
     floor stays what it was meant to be — the last resort, not the first. -->
<div class="flex flex-wrap items-center gap-2" bind:this={boxEl}>
  <!-- The boundary for the help popover's outside click: it has to hold the
       `?` too, or the click that closes the panel would reopen it. -->
  <div
    class="relative min-w-[200px] flex-1"
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
      onfocus={() => (focused = true)}
      onblur={() => (focused = false)}
      type="text"
      data-testid="search-input"
      data-enter="widen"
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
      <span class="flex-none text-micro text-text-muted">{t('common.searching')}</span>
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
        aria-label={t('list.searchClear')}
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
      class="anim-enter absolute left-0 top-full z-30 mt-10 w-full max-w-md rounded-lg border border-border-strong bg-bg-elevated p-2 text-body leading-relaxed text-text-secondary shadow-overlay"
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
  {:else if focused && suggestions.length > 0}
    <div
      class="anim-enter absolute left-0 top-full z-30 mt-1 w-full max-w-md rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-overlay"
    >
      {#each suggestions as s, i (s.kind + s.value)}
        <button
          type="button"
          class="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-body {i === sugIdx
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
  {:else if focused && showJump && jumpKey}
    <div
      class="anim-enter absolute left-0 top-full z-30 mt-1 w-full max-w-md rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-overlay"
    >
      <button
        type="button"
        class="flex w-full items-center gap-2 rounded bg-bg-active px-2 py-1 text-left text-body text-text-primary"
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
    bind:this={paletteBtn}
    data-testid="palette-open"
    class="flex h-control flex-none items-center gap-1.5 rounded-md border border-border-strong/70 bg-bg-elevated px-2.5 text-body text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary"
    title={t('palette.entryTitle', { shortcut: paletteShortcutLabel() })}
    aria-label={t('palette.entryTitle', { shortcut: paletteShortcutLabel() })}
    onclick={() => requestOpenPalette()}
  >
    <Icon name="search" size={14} class="text-text-muted" />
    {#if !viewportNarrow && !slotCompact}
      <span>{t('palette.entryLabel')}</span>
    {/if}
    <kbd class="rounded border border-border-subtle px-1 text-micro text-text-muted">{paletteShortcutLabel()}</kbd>
  </button>
</div>

<style>
  /* GDK-1056: if a future copy outgrows the field in a regime the width
     switch above does not predict, degrade to an ellipsis — never a silent
     mid-word cut. */
  input::placeholder {
    text-overflow: ellipsis;
  }
</style>
