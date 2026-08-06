<script module lang="ts">
  export interface ScopeOption {
    value: string
    label: string
    /** Secondary text on the row (space type, project type…). */
    hint?: string
  }
</script>

<script lang="ts">
  /*
   * Typeahead multi-select for a mirror scope (Jira projects, Confluence spaces).
   *
   * The list is whatever the caller discovered from the site, but the selection
   * is not limited to it: a key that is configured and no longer returned (lost
   * access, renamed project) stays as a chip rather than disappearing on the
   * next save. Unknown chips show the raw value, which is exactly what the old
   * comma-separated text box showed.
   *
   * Keyboard: ↑↓ move, Enter picks the highlighted row, Esc closes the list,
   * Backspace on an empty input drops the last chip.
   */
  import type { Snippet } from 'svelte'
  import { t } from '../../lib/i18n'

  let {
    label,
    hint,
    options,
    selected = $bindable(),
    placeholder,
    emptyLabel,
    action,
    testid,
  }: {
    label: string
    hint?: string
    options: ScopeOption[]
    selected: string[]
    placeholder: string
    /** Shown in place of the chip row while nothing is selected. */
    emptyLabel?: string
    /** Control that belongs to the list itself (a filter toggle). It rides the
     *  label row, where the open dropdown cannot cover it. */
    action?: Snippet
    testid?: string
  } = $props()

  const INPUT =
    'w-full rounded-md border border-border-strong bg-bg-base px-2 py-1 text-[12px] text-text-primary outline-none focus:border-accent'

  // Two pickers share the settings dialog, so the input↔listbox pairing needs an
  // id that is unique per instance.
  const uid = $props.id()
  const listId = `scope-options-${uid}`

  let query = $state('')
  let open = $state(false)
  let idx = $state(0)
  let rootEl = $state<HTMLDivElement | null>(null)

  const byValue = $derived(new Map(options.map((o) => [o.value, o])))

  const matches = $derived.by(() => {
    const needle = query.trim().toLowerCase()
    return options.filter((o) => {
      if (selected.includes(o.value)) return false
      if (!needle) return true
      return o.value.toLowerCase().includes(needle) || o.label.toLowerCase().includes(needle)
    })
  })

  // The highlight must not survive a shrinking list, or Enter picks a row that
  // is no longer on screen.
  $effect(() => {
    void matches
    idx = 0
  })

  function add(value: string) {
    if (!value || selected.includes(value)) return
    selected = [...selected, value]
    query = ''
  }

  function remove(value: string) {
    selected = selected.filter((v) => v !== value)
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      open = true
      if (matches.length) idx = (idx + 1) % matches.length
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      open = true
      if (matches.length) idx = (idx - 1 + matches.length) % matches.length
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const pick = matches[idx]
      if (pick) {
        add(pick.value)
        open = true
      }
    } else if (e.key === 'Escape') {
      // Stop here: the settings dialog closes on Escape, and closing the whole
      // dialog to dismiss a dropdown would lose every unsaved edit.
      e.preventDefault()
      e.stopPropagation()
      open = false
    } else if (e.key === 'Backspace' && !query && selected.length) {
      remove(selected[selected.length - 1])
    }
  }

  function onDocClick(e: MouseEvent) {
    if (open && rootEl && !e.composedPath().includes(rootEl)) open = false
  }
</script>

<svelte:document onclick={onDocClick} />

<div class="flex flex-col gap-1.5" bind:this={rootEl} data-testid={testid}>
  <div class="flex items-center justify-between gap-2">
    <span class="text-[11px] text-text-secondary">{label}</span>
    {#if action}{@render action()}{/if}
  </div>

  {#if selected.length}
    <div class="flex flex-wrap gap-1">
      {#each selected as value (value)}
        {@const option = byValue.get(value)}
        <span
          class="flex max-w-full items-center gap-1 rounded-md border border-border-strong bg-bg-elevated py-0.5 pl-1.5 pr-1 text-[11px] text-text-primary"
          data-testid="scope-chip"
        >
          <!-- Same emphasis order as the dropdown rows: mono accent key, then name. -->
          <span class="font-mono text-accent-text">{value}</span>
          {#if option && option.label !== value}
            <span class="min-w-0 truncate">{option.label}</span>
          {/if}
          <button
            type="button"
            class="flex-none rounded px-0.5 text-text-muted transition-colors hover:text-status-reopen"
            aria-label={t('settings.scopeRemove', { name: option?.label || value })}
            onclick={() => remove(value)}>✕</button
          >
        </span>
      {/each}
    </div>
  {:else if emptyLabel}
    <p class="text-[11px] text-text-muted" data-testid="scope-empty">{emptyLabel}</p>
  {/if}

  <div class="relative">
    <input
      class={INPUT}
      type="text"
      role="combobox"
      aria-expanded={open}
      aria-controls={listId}
      aria-autocomplete="list"
      aria-label={label}
      autocomplete="off"
      spellcheck="false"
      {placeholder}
      bind:value={query}
      onfocus={() => (open = true)}
      oninput={() => (open = true)}
      onkeydown={onKeydown}
      data-testid="scope-input"
    />
    {#if open}
      <!-- Shadow deliberately one step below the palette's: this floats over the
           next settings section, and a heavy drop made that section read as
           disabled rather than covered. Height is capped for the same reason. -->
      <div
        class="anim-enter absolute left-0 right-0 top-full z-30 mt-2 overflow-hidden rounded-md border border-border-strong bg-bg-elevated shadow-xl shadow-black/50"
      >
        <div
          class="max-h-40 overflow-y-auto p-1"
          id={listId}
          role="listbox"
          aria-label={label}
          data-testid="scope-options"
        >
          {#if matches.length === 0}
            <p class="px-2 py-1.5 text-[11px] text-text-muted">{t('settings.scopeNoMatch')}</p>
          {:else}
            {#each matches as option, i (option.value)}
              <button
                type="button"
                role="option"
                aria-selected={i === idx}
                class="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-[12px] {i ===
                idx
                  ? 'bg-bg-active text-text-primary'
                  : 'text-text-secondary hover:bg-bg-hover'}"
                data-testid="scope-option"
                onmousemove={() => (idx = i)}
                onmousedown={(e) => {
                  e.preventDefault()
                  add(option.value)
                }}
              >
                <span class="flex-none font-mono text-[11px] text-accent-text">{option.value}</span>
                <span class="min-w-0 flex-1 truncate">{option.label}</span>
                {#if option.hint}
                  <!-- Raw API vocabulary (service_desk, personal) — shown as it
                       is, but quiet: it disambiguates, it does not label. -->
                  <span class="flex-none text-[10px] text-text-muted">{option.hint}</span>
                {/if}
              </button>
            {/each}
          {/if}
        </div>
        <!-- Same hint grammar as the palette's footer: the keys are only
             discoverable while the list is open, so they live in it. -->
        <div
          class="flex-none border-t border-border-subtle px-2 py-1 text-[10px] text-text-muted"
          data-testid="scope-hint"
        >
          {t('settings.scopeHint')}
        </div>
      </div>
    {/if}
  </div>

  {#if hint}
    <span class="text-[11px] text-text-muted">{hint}</span>
  {/if}
</div>
