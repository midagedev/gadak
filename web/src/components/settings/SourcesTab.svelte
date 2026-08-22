<script lang="ts">
  /*
   * What the mirror pulls: Jira projects and Confluence spaces.
   * Both option lists are fetched by the dialog (one round-trip to the site,
   * only when this tab is first opened) and arrive here as props.
   */
  import { t } from '../../lib/i18n'
  import ScopePicker, { type ScopeOption } from './ScopePicker.svelte'
  import { INPUT, ADD_BTN } from './controls'
  import type { SettingsDraft } from './draft'

  let {
    draft = $bindable(),
    projectOptions,
    projectsPickerReady,
    projectsLoading,
    projectsTouched = $bindable(),
    spaceOptions,
    spacesLoading,
    spacesError,
    showPersonalSpaces = $bindable(),
  }: {
    draft: SettingsDraft
    projectOptions: ScopeOption[]
    /** Site list unreachable (no credential, Jira down) → the manual text box. */
    projectsPickerReady: boolean
    projectsLoading: boolean
    /** Hand-edited the manual keys — the list must not replace the field under
     *  a typing user, however late it arrives. */
    projectsTouched: boolean
    spaceOptions: ScopeOption[]
    spacesLoading: boolean
    spacesError: string | null
    showPersonalSpaces: boolean
  } = $props()

  // Personal spaces are one per colleague and almost never mirror targets, so
  // they stay out of the list until asked for — except one already selected,
  // which must stay visible or the picker would look like it dropped it.
  const visibleSpaceOptions = $derived(
    showPersonalSpaces
      ? spaceOptions
      : spaceOptions.filter((o) => o.hint !== 'personal' || draft.spaces.includes(o.value)),
  )

  // GDK-476: empty scope = every team space. Same two-click arm as the
  // credential delete button (JiraKeySettings.deleteArmed) — no new dialog.
  let turnOnArmed = $state(false)
  $effect(() => {
    if (draft.spaces.length > 0 || draft.confluenceOn) turnOnArmed = false
  })

  function turnConfluenceOn(): void {
    if (draft.spaces.length > 0) {
      draft.confluenceOn = true
      return
    }
    if (!turnOnArmed) {
      turnOnArmed = true
      return
    }
    draft.confluenceOn = true
    turnOnArmed = false
  }
</script>

<div class="flex flex-col gap-5" data-testid="settings-sources">
  <!-- Jira projects -->
  {#if projectsPickerReady}
    <ScopePicker
      label={t('settings.sourcesProjects')}
      hint={t('settings.sourcesProjectsHint')}
      options={projectOptions}
      bind:selected={draft.projects}
      placeholder={t('settings.scopeProjectPlaceholder')}
      emptyLabel={t('settings.sourcesNoProjects')}
      testid="scope-projects"
    />
  {:else}
    <label class="flex flex-col gap-1" data-testid="scope-projects-fallback">
      <span class="text-micro text-text-secondary">{t('settings.projects')}</span>
      <input
        class={INPUT}
        bind:value={draft.projectsText}
        oninput={() => (projectsTouched = true)}
        placeholder="NMB, NMA"
      />
      <span class="text-micro text-text-muted">
        {projectsLoading ? t('settings.scopeLoading') : t('settings.projectsManual')}
      </span>
      <!-- Same meaning as the picker's emptyLabel: [] = every visible project. -->
      {#if !projectsLoading && !draft.projectsText.trim()}
        <span class="text-micro text-text-muted">{t('settings.sourcesNoProjects')}</span>
      {/if}
    </label>
  {/if}

  <!--
    Confluence. Present whether the source is on or off — while it was
    hidden, a profile with the source off looked exactly like a build
    without the feature, and this screen was the only place it could
    have been turned on.

    Order is deliberate: choose spaces, then turn it on. The button says
    what it will mirror, because with nothing selected the answer is
    "the whole wiki" and that must be a sentence someone read, not the
    side effect of a generic Turn on.
  -->
  <div class="border-t border-border-subtle pt-4" data-testid="sources-confluence">
    <div class="mb-2.5 flex items-start justify-between gap-3">
      <div class="min-w-0">
        <!-- Same weight as the Jira projects label above: they are two
             sources of one mirror, and a bolder heading here made
             Confluence outrank Jira on a scan. -->
        <span class="text-micro text-text-secondary">
          {t('settings.confluenceTitle')}
        </span>
        <!--
          One state line, and the consequence lives in it rather than in
          an extra line underneath. The block sits near the bottom of a
          scrolling dialog, so a line added below the button rendered
          under the fold at exactly the scroll position where the button
          is clicked — the warning was invisible at the moment it was
          earned.
        -->
        <p
          class="mt-0.5 text-micro leading-relaxed {draft.confluenceOn && draft.spaces.length === 0
            ? 'text-status-stale'
            : 'text-text-muted'}"
          data-testid={draft.confluenceOn && draft.spaces.length === 0
            ? 'confluence-all-warning'
            : undefined}
        >
          {#if draft.confluenceOn && draft.spaces.length === 0}
            {t('settings.confluenceAllWarning')}
          {:else if draft.confluenceOn}
            {t('settings.confluenceOnHint')}
          {:else}
            {t('settings.confluenceOffHint')}
          {/if}
        </p>
      </div>
      <!-- Both states use the dialog's secondary button. The filled
           accent belongs to Save alone: this control changes a pending
           value like every other field here, and a second primary made
           it look like it committed on click. -->
      {#if draft.confluenceOn}
        <button
          type="button"
          class="{ADD_BTN} flex-none self-start"
          onclick={() => {
            draft.confluenceOn = false
            // Scope goes with it: a stored selection under an off
            // source is a promise nothing keeps, and leaving it would
            // make the dialog's effect switch the source straight back on.
            draft.spaces = []
          }}
          data-testid="confluence-turn-off"
        >
          {t('settings.confluenceTurnOff')}
        </button>
      {:else}
        <button
          type="button"
          class="{ADD_BTN} flex-none self-start"
          onclick={turnConfluenceOn}
          data-testid="confluence-turn-on"
        >
          {draft.spaces.length
            ? t('settings.confluenceTurnOnCount', { n: String(draft.spaces.length) })
            : turnOnArmed
              ? t('settings.confluenceTurnOnAllConfirm')
              : t('settings.confluenceTurnOnAll')}
        </button>
      {/if}
    </div>

    {#if spacesLoading}
      <p class="text-micro text-text-muted">{t('settings.scopeLoading')}</p>
    {:else if spacesError}
      <p class="text-body text-status-stale" data-testid="scope-spaces-error">
        {spacesError}
      </p>
    {:else}
      <!-- The hint reads "Only these spaces are mirrored", which is
           false with nothing selected — that is precisely the case
           where every team space is — so it waits for a selection to
           describe rather than contradicting the line above it. -->
      <ScopePicker
        label={t('settings.sourcesSpaces')}
        hint={draft.spaces.length ? t('settings.sourcesSpacesHint') : ''}
        options={visibleSpaceOptions}
        bind:selected={draft.spaces}
        placeholder={t('settings.scopeSpacePlaceholder')}
        emptyLabel={draft.confluenceOn
          ? t('settings.sourcesAllGlobal')
          : t('settings.sourcesNoSpaces')}
        testid="scope-spaces"
      >
        {#snippet action()}
          <label class="flex cursor-pointer items-center gap-1.5 text-micro text-text-muted">
            <input
              type="checkbox"
              class="accent-[var(--color-accent,#3b82f6)]"
              bind:checked={showPersonalSpaces}
            />
            {t('settings.showPersonalSpaces')}
          </label>
        {/snippet}
      </ScopePicker>
    {/if}
  </div>

  <p class="border-t border-border-subtle pt-3 text-micro leading-relaxed text-text-muted">
    {t('settings.sourcesApplyHint')}
  </p>
</div>
