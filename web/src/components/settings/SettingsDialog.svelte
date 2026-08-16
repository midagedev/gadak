<script module lang="ts">
  /** The settings tabs, in header order. Exported so App can validate an
   *  incoming `settings=` place param (lib/url-state) against the real tab
   *  list instead of a second copy; TABS below attaches labels to these ids. */
  export type Tab = 'sync' | 'sources' | 'features' | 'groups' | 'members' | 'fields'
  export const SETTINGS_TABS: readonly Tab[] = [
    'sync',
    'sources',
    'features',
    'groups',
    'members',
    'fields',
  ]

  /** Type guard for URL values: an unknown tab name (a link from before this
   *  build renamed or added tabs) must land on the default, not a blank
   *  dialog. */
  export function isSettingsTab(v: string): v is Tab {
    return (SETTINGS_TABS as readonly string[]).includes(v)
  }
</script>

<script lang="ts">
  /*
   * Server settings editor dialog (`~/.gadak/config.json`, loopback-only API).
   *  - Open → GET settings/; save → PUT settings/ (full replace) → location.reload() on ok.
   *    config.json, bootstrap members, and group inject are all server-derived — full
   *    reload is the honest path.
   *  - One `draft` object is the whole form; the tabs bind into it and `draft.ts`
   *    owns the two translations to and from the wire shape.
   *  - Advanced JSON textarea vs form: last edit wins (successful parse rehydrates form).
   *  Personal Jira token is JiraKeySettings' job — only a link here.
   *  Same modal pattern as JiraKeySettings (Esc / backdrop close).
   */
  import { t, locale, setLocale, type Locale } from '../../lib/i18n'
  import {
    THEME_MODES,
    parseThemePreference,
    readThemePreference,
    setThemePreference,
    type ThemePreference,
  } from '../../lib/theme'
  import { onMount } from 'svelte'
  import * as api from '../../lib/api'
  import type { GadakSettings, SettingsRuntime } from '../../lib/api'
  import { write } from '../../stores/write.svelte'
  import type { ScopeOption } from './ScopePicker.svelte'
  import { emptyDraft, toDraft, toSettings } from './draft'
  import { SELECT, SELECT_CHEVRON } from './controls'
  import RuntimeMirror from './RuntimeMirror.svelte'
  import SyncTab from './SyncTab.svelte'
  import SourcesTab from './SourcesTab.svelte'
  import FeaturesTab from './FeaturesTab.svelte'
  import GroupsTab from './GroupsTab.svelte'
  import MembersTab from './MembersTab.svelte'
  import FieldsTab from './FieldsTab.svelte'
  import { trapFocus } from '../../lib/focus-trap'
  import Icon from '../ui/Icon.svelte'

  // `tab` is bindable so the `settings=` URL binding in App can read which tab
  // is open and set one on arrival; the default matches every open before the
  // prop existed. Closing resets it (App's closeServerSettings), which is what
  // the dialog's unmount used to do for free.
  let { onclose, tab = $bindable('sync') }: { onclose: () => void; tab?: Tab } = $props()

  const TABS: [Tab, string][] = [
    ['sync', t('settings.tabSync')],
    ['sources', t('settings.tabSources')],
    ['features', t('settings.tabFeatures')],
    ['groups', t('settings.tabTeams')],
    ['members', t('settings.tabMembers')],
    ['fields', t('settings.tabFields')],
  ]

  let loading = $state(true)
  let saving = $state(false)
  let error = $state<string | null>(null)

  /** Everything the form edits. Replaced wholesale by a load, mutated in place
   *  by the tabs, and turned back into a payload by `toSettings`. */
  let draft = $state(emptyDraft())
  /** Which member row is expanded — view state, so it stays out of the draft. */
  let openMember = $state<number | null>(null)

  let defaultSyncSec = $state(60)
  let defaultReconcileSec = $state(3600)
  let runtime = $state<SettingsRuntime | null>(null)

  /* ── Sources tab (what the mirror pulls) ──
   * Both lists come from the live site, so they are fetched when the tab is
   * first opened rather than with the dialog: settings gets opened for plenty of
   * reasons that should not cost a Jira round-trip. They live here rather than
   * in the tab because the tab unmounts on every tab switch.
   */
  let sourcesRequested = false
  let projectOptions = $state<ScopeOption[]>([])
  /** Site list unreachable (no credential, Jira down) → keep the old text box,
   *  which is the only way to configure a scope without asking the site. */
  let projectsPickerReady = $state(false)
  let projectsLoading = $state(false)
  /** Hand-edited the manual keys — the list must not replace the field under a
   *  typing user, however late it arrives. */
  let projectsTouched = $state(false)
  let spaceOptions = $state<ScopeOption[]>([])
  let spacesLoading = $state(false)
  let spacesError = $state<string | null>(null)
  let showPersonalSpaces = $state(false)

  /*
   * Choosing a space is the request to mirror it. Without this, picking one
   * while the source was off and pressing Save sent {enabled:false, spaces:[]}
   * — the chip sat on screen and the save discarded it silently, which is the
   * failure this whole screen exists to remove.
   *
   * The button is left to carry the one case that genuinely needs a decision:
   * turning the source on with *no* scope, which mirrors every team space.
   * Turning it off clears the scope, so this can never fight that click.
   *
   * It stays in the dialog, not in the Sources tab: it must hold whether or not
   * that tab is on screen.
   */
  $effect(() => {
    if (draft.spaces.length > 0 && !draft.confluenceOn) draft.confluenceOn = true
  })

  let jsonText = $state('')
  let jsonError = $state<string | null>(null)

  /** Expand server response (or JSON textarea) into form state. */
  function load(s: GadakSettings) {
    draft = toDraft(s, draft.features)
    openMember = null
    if (s.runtime) {
      runtime = s.runtime
      defaultSyncSec = s.runtime.defaultSyncIntervalSec || 60
      defaultReconcileSec = s.runtime.defaultReconcileIntervalSec || 3600
    }
  }

  /** Form state → PUT payload (full replace). Do not send runtime/site. */
  function build(): GadakSettings {
    return toSettings(draft, projectsPickerReady)
  }

  onMount(async () => {
    try {
      load(await api.getSettings())
    } catch (e) {
      error = e instanceof Error ? e.message : t('settings.loadFailed')
    }
    loading = false
  })

  /** Fetch the two scope lists once, the first time the Sources tab is shown. */
  async function loadSources() {
    if (sourcesRequested) return
    sourcesRequested = true

    // Manual entry is on screen from the first frame: asking the site for its
    // projects can take many seconds when it is unreachable, and a spinner over
    // the one field that works without the site is the wrong trade.
    /*
     * Two independent lists, two independent requests. They used to run in
     * sequence, so an unreachable Jira held the Confluence picker at "loading"
     * for as long as the socket took to give up — one dead source hiding the
     * other's controls, on the screen whose job is to configure both.
     */
    projectsLoading = true
    const projects = (async () => {
      try {
        const res = await api.getAvailableProjects()
        projectOptions = res.projects.map((p) => ({
          value: p.key,
          label: p.name,
          hint: p.projectTypeKey,
        }))
        if (!projectsTouched) {
          projectsPickerReady = true
          // The picker is now the field of record; drop the text mirror so a
          // stale string can never win the next build().
          draft.projectsText = ''
        }
      } catch {
        // No credential (409) or the site is unreachable — the manual list stays.
        projectsPickerReady = false
      }
      projectsLoading = false
    })()

    // Loaded whether or not the source is on: picking spaces is how you decide
    // to turn it on, so the list has to arrive first.
    spacesLoading = true
    const spaces = (async () => {
      try {
        const res = await api.getSettingsSpaces()
        spaceOptions = res.spaces.map((s) => ({ value: s.key, label: s.name, hint: s.type }))
        // res.all_global_when_empty is the saved state's version of the same
        // rule; the picker reads the pending switch instead, or the label would
        // contradict the warning above it between a toggle and its save.
      } catch {
        spacesError = t('settings.spacesUnavailable')
      }
      spacesLoading = false
    })()

    await Promise.all([projects, spaces])
  }

  $effect(() => {
    if (tab === 'sources' && !loading) void loadSources()
  })

  function refreshJson() {
    jsonText = JSON.stringify(build(), null, 2)
    jsonError = null
  }

  function applyJson(text: string) {
    jsonText = text
    try {
      load(JSON.parse(text) as GadakSettings)
      jsonError = null
    } catch {
      jsonError = t('settings.jsonParseError')
    }
  }

  async function save() {
    if (saving || jsonError) return
    saving = true
    error = null
    try {
      await api.putSettings(build())
      write.toast(t('settings.savedReload'), 'success')
      setTimeout(() => location.reload(), 600)
    } catch (e) {
      error = e instanceof Error ? e.message : t('settings.saveFailed')
      saving = false
    }
  }

  function openJiraKey() {
    onclose()
    write.openSettings()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose()
  }

  // Per-browser, like locale: not part of the server PUT. Local state so the
  // select updates without a reload (setThemePreference does not navigate).
  let themePref = $state<ThemePreference>(readThemePreference())

  function onThemeChange(event: Event): void {
    const next = parseThemePreference((event.currentTarget as HTMLSelectElement).value)
    setThemePreference(next)
    themePref = next
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-[#1c1812]/28 p-4 backdrop-blur-[2px]"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) onclose()
  }}
>
  <!-- 92vh, not 88: the Sync tab runs THIS MIRROR plus four groups plus the
       personal-token entry point, and at 88vh the entry point sat 35px below
       the fold — an action nobody scrolls to find, because nothing above it
       suggests there is more. The alternative was to compress the groups, but
       they are already at 16px apart with 4px between label and control, at or
       under the floor that fix was given, so the height had to come from the
       dialog instead. 8vh still leaves 40px of backdrop above and below at a
       1000px viewport. (2026-08-06) -->
  <div
    use:trapFocus
    class="anim-pop flex max-h-[92vh] w-full max-w-3xl flex-col rounded-lg border border-border-strong bg-bg-panel shadow-overlay"
    role="dialog"
    aria-modal="true"
    aria-label={t('settings.title')}
    data-testid="settings-dialog"
  >
    <!-- Header + tabs -->
    <div class="flex-none border-b border-border-subtle px-5 pt-4">
      <h2 class="type-subject mb-0.5 text-[18px] leading-snug text-text-primary">{t('settings.title')}</h2>
      <p class="mb-3 text-micro text-text-muted">
        {t('settings.introBefore')} <span class="font-mono">~/.gadak/config.json</span> {t('settings.introAfter')}
      </p>
      <div class="flex gap-1">
        {#each TABS as [id, label] (id)}
          <button
            type="button"
            class="-mb-px flex h-control items-center border-b-2 px-2.5 text-[12px] transition-colors {tab === id
              ? 'border-accent text-text-primary'
              : 'border-transparent text-text-secondary hover:text-text-primary'}"
            onclick={() => (tab = id)}
          >
            {label}
          </button>
        {/each}
      </div>
    </div>

    <!-- Body -->
    <div class="min-h-0 flex-1 overflow-y-auto px-5 py-4 text-[12px]">
      {#if loading}
        <p class="py-8 text-center text-text-muted">{t('settings.loading')}</p>
      {:else}
        <!-- This mirror — read-only runtime facts (always above tab content) -->
        {#if runtime}
          <RuntimeMirror {runtime} />
        {/if}

        {#if tab === 'sync'}
          <SyncTab bind:draft {defaultSyncSec} {defaultReconcileSec} onOpenJiraKey={openJiraKey} />
        {:else if tab === 'sources'}
          <SourcesTab
            bind:draft
            {projectOptions}
            {projectsPickerReady}
            {projectsLoading}
            bind:projectsTouched
            {spaceOptions}
            {spacesLoading}
            {spacesError}
            bind:showPersonalSpaces
          />
        {:else if tab === 'features'}
          <FeaturesTab bind:draft />
        {:else if tab === 'groups'}
          <GroupsTab bind:draft />
        {:else if tab === 'members'}
          <MembersTab bind:draft bind:openMember />
        {:else}
          <FieldsTab bind:draft />
        {/if}

        <!-- Advanced: raw JSON -->
        <details
          class="mt-5 border-t border-border-subtle pt-3"
          ontoggle={(e) => {
            if (e.currentTarget.open) refreshJson()
          }}
        >
          <summary class="cursor-pointer text-micro text-text-secondary hover:text-text-primary">
            {t('settings.advancedJson')}
          </summary>
          <textarea
            class="mt-2 h-56 w-full rounded-md border border-border-strong bg-bg-base p-2 font-mono text-micro text-text-primary outline-none focus:border-accent"
            spellcheck="false"
            value={jsonText}
            oninput={(e) => applyJson(e.currentTarget.value)}
          ></textarea>
          {#if jsonError}
            <p class="mt-1 text-micro text-status-reopen">{jsonError}</p>
          {:else}
            <p class="mt-1 text-micro text-text-muted">
              {t('settings.jsonHint')}
            </p>
          {/if}
        </details>
      {/if}
    </div>


    <!-- per-browser prefs (not server config): theme leads, locale keeps the
         trailing slot above the save footer. -->
    <div class="flex flex-none items-center gap-2 border-t border-border-subtle px-5 py-2">
      <label class="flex items-center gap-2 text-[12px] text-text-secondary">
        <span>{t('theme.label')}</span>
        <span class="relative flex">
          <select
            class="{SELECT} w-auto"
            data-testid="theme-picker"
            value={themePref}
            onchange={onThemeChange}
          >
            {#each THEME_MODES as mode (mode.name)}
              <option value={mode.name}>{t(mode.labelKey)}</option>
            {/each}
          </select>
          <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
        </span>
      </label>
      <label class="flex items-center gap-2 text-[12px] text-text-secondary">
        <span>{t('settings.locale')}</span>
        <span class="relative flex">
          <select
            class="{SELECT} w-auto"
            value={locale()}
            onchange={(e) => setLocale(e.currentTarget.value as Locale)}
          >
            <option value="en">{t('settings.localeEn')}</option>
            <option value="ko">{t('settings.localeKo')}</option>
          </select>
          <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
        </span>
      </label>
    </div>

    <!-- Footer -->
    <div class="flex flex-none items-center justify-between gap-2 border-t border-border-subtle px-5 py-3">
      <span class="min-w-0 flex-1 truncate text-[12px] text-status-reopen">{error ?? ''}</span>
      <button
        type="button"
        onclick={onclose}
        class="inline-flex h-control items-center rounded-md px-3 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
      >
        {t('common.close')}
      </button>
      <button
        type="button"
        onclick={save}
        disabled={loading || saving || !!jsonError}
        class="inline-flex h-control items-center rounded-md bg-accent px-3 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
      >
        {saving ? t('common.saving') : t('common.save')}
      </button>
    </div>
  </div>
</div>
