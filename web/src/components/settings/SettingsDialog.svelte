<script module lang="ts">
  import { isDesktop } from '../../lib/config'
  import { isVisibleSettingsTab, visibleSettingsTabs } from '../../lib/integrations'

  /** The settings tabs, in header order. Exported so App can validate an
   *  incoming `settings=` place param (lib/url-state) against the real tab
   *  list instead of a second copy; TABS below attaches labels to these ids. */
  export type Tab =
    | 'sync'
    | 'sources'
    | 'features'
    | 'groups'
    | 'members'
    | 'fields'
    | 'integrations'
    | 'about'
  export const SETTINGS_TABS: readonly Tab[] = [
    'sync',
    'sources',
    'features',
    'groups',
    'members',
    'fields',
    'integrations',
    'about',
  ]

  /** Type guard for URL values: an unknown tab name (a link from before this
   *  build renamed or added tabs) must land on the default, not a blank
   *  dialog. `integrations` counts as unknown outside the desktop app, whose
   *  mux is the only place its `/desktop/*` routes exist — a link pasted out
   *  of the app must not open a tab that can only fail here. */
  export function isSettingsTab(v: string): v is Tab {
    return isVisibleSettingsTab(v, SETTINGS_TABS, isDesktop())
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
    persistThemePreference,
    readThemePreference,
    type ThemePreference,
  } from '../../lib/theme'
  import { onMount } from 'svelte'
  import * as api from '../../lib/api'
  import type { GadakSettings, SettingsRuntime } from '../../lib/api'
  import { write } from '../../stores/write.svelte'
  import type { ScopeOption } from './ScopePicker.svelte'
  import { emptyDraft, toDraft, toSettings } from './draft'
  import { SELECT, SELECT_CHEVRON } from './controls'
  import SyncTab from './SyncTab.svelte'
  import SourcesTab from './SourcesTab.svelte'
  import FeaturesTab from './FeaturesTab.svelte'
  import GroupsTab from './GroupsTab.svelte'
  import MembersTab from './MembersTab.svelte'
  import FieldsTab from './FieldsTab.svelte'
  import IntegrationsTab from './IntegrationsTab.svelte'
  import AboutTab from './AboutTab.svelte'
  import { trapFocus } from '../../lib/focus-trap'
  import Icon from '../ui/Icon.svelte'
  import DialogShell from '../ui/DialogShell.svelte'

  // `tab` is bindable so the `settings=` URL binding in App can read which tab
  // is open and set one on arrival; the default matches every open before the
  // prop existed. Closing resets it (App's closeServerSettings), which is what
  // the dialog's unmount used to do for free.
  let { onclose, tab = $bindable('sync') }: { onclose: () => void; tab?: Tab } = $props()

  const LABELS: Record<Tab, string> = {
    sync: t('settings.tabSync'),
    sources: t('settings.tabSources'),
    features: t('settings.tabFeatures'),
    groups: t('settings.tabTeams'),
    members: t('settings.tabMembers'),
    fields: t('settings.tabFields'),
    integrations: t('settings.tabIntegrations'),
    about: t('settings.tabAbout'),
  }
  /* Header order is SETTINGS_TABS order, minus the tabs this surface has no
     server for (Integrations is desktop-only). One list, so the header and the
     `settings=` URL guard can never disagree about what exists. */
  const TABS: [Tab, string][] = visibleSettingsTabs(SETTINGS_TABS, isDesktop()).map((id) => [
    id,
    LABELS[id],
  ])
  const showIntegrations = TABS.some(([id]) => id === 'integrations')

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

  /*
   * Jira-unreachable holds these until the site TCP gives up, which left the
   * Sources tab on "Loading the list…" for the whole wait (GDK-476). Aborting
   * here is a client timeout, not a new poll — the manual-key fallback is
   * already on screen for projects.
   */
  const SCOPE_LIST_MS = 8_000

  function scopeListSignal(): AbortSignal {
    if (typeof AbortSignal.timeout === 'function') return AbortSignal.timeout(SCOPE_LIST_MS)
    const c = new AbortController()
    setTimeout(() => c.abort(), SCOPE_LIST_MS)
    return c.signal
  }

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
        const res = await api.getAvailableProjects({ signal: scopeListSignal() })
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
        const res = await api.getSettingsSpaces({ signal: scopeListSignal() })
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

  // Instant apply + write-through to this workspace's settings. Not the
  // dialog draft — persist reads the server document so an unsaved form
  // is not flushed by a theme click.
  let themePref = $state<ThemePreference>(readThemePreference())

  function onThemeChange(event: Event): void {
    const next = parseThemePreference((event.currentTarget as HTMLSelectElement).value)
    void persistThemePreference(next)
    themePref = next
  }
</script>

<svelte:window onkeydown={onKeydown} />

<!-- 92vh, not 88: the Sync tab runs THIS MIRROR plus four groups plus the
     personal-token entry point, and at 88vh the entry point sat 35px below
     the fold — an action nobody scrolls to find, because nothing above it
     suggests there is more. The alternative was to compress the groups, but
     they are already at 16px apart with 4px between label and control, at or
     under the floor that fix was given, so the height had to come from the
     dialog instead. 8vh still leaves 40px of backdrop above and below at a
     1000px viewport. (2026-08-06) -->
<DialogShell
  title={t('settings.title')}
  ariaLabel={t('settings.title')}
  data-testid="settings-dialog"
  {onclose}
  trap={trapFocus}
  panelClass="anim-pop max-h-[92vh] max-w-3xl"
  headerClass="flex flex-none flex-col border-b border-border-subtle px-5 pt-4"
  titleRowClass="mb-0.5 flex items-center justify-between"
  footerClass="flex flex-none flex-wrap items-center gap-2 border-t border-border-subtle px-5 py-3"
>
  {#snippet headerExtra()}
    <p class="mb-3 text-micro text-text-muted" data-testid="settings-intro">
      {t('settings.intro')}
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
  {/snippet}

  <div
    class="scroll-region min-h-0 flex-1 px-5 pt-4 text-[12px]"
    style="--scroll-pad-bottom: 1rem"
    data-testid="settings-scroll"
  >
      {#if loading}
        <p class="py-8 text-center text-text-muted">{t('settings.loading')}</p>
      {:else}
        <!-- The runtime mirror (read-only instance facts) is the Sync tab's
             own footer now, not a block above every tab: repeated on all seven
             it pushed each tab's subject down — far enough on Integrations to
             put all three install cards below the fold (vision verdict
             2026-08-17) — to state facts about the sync. (GDK-188) -->
        {#if tab === 'sync'}
          <SyncTab
            bind:draft
            {defaultSyncSec}
            {defaultReconcileSec}
            {runtime}
            onOpenJiraKey={openJiraKey}
          />
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
          <FeaturesTab bind:draft osNotifySupported={runtime?.osNotifySupported ?? true} />
        {:else if tab === 'groups'}
          <GroupsTab bind:draft />
        {:else if tab === 'members'}
          <MembersTab bind:draft bind:openMember />
        {:else if tab === 'integrations' && showIntegrations}
          <IntegrationsTab />
        {:else if tab === 'about'}
          <AboutTab {runtime} />
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


  {#snippet footer()}
    <!-- One pinned footer: theme/locale write-throughs (theme immediately; locale
         stays per-browser) sit with Close/Save so they cannot stack over the body. -->
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
    <span class="min-w-0 flex-1 truncate text-[12px] text-status-reopen">{error ?? ''}</span>
    <button
      type="button"
      onclick={onclose}
      class="inline-flex h-control items-center rounded-md px-3 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
    >
      {t('common.cancel')}
    </button>
    <button
      type="button"
      onclick={save}
      disabled={loading || saving || !!jsonError}
      class="inline-flex h-control items-center rounded-md bg-accent px-3 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
    >
      {saving ? t('common.saving') : t('common.save')}
    </button>
  {/snippet}
</DialogShell>
