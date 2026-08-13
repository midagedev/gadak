<script lang="ts">
  /* Custom fields: what discovery found, and the hand-written aliases. */
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import KeyValueRows from './KeyValueRows.svelte'
  import { INPUT, SELECT, SELECT_CHEVRON } from './controls'
  import type { SettingsDraft } from './draft'

  let { draft = $bindable() }: { draft: SettingsDraft } = $props()

  /** Any hand edit pins the row (auto:false) so `gadak fields --apply` keeps it. */
  function touchSpec(i: number) {
    draft.specsTouched = true
    draft.specs[i] = { ...draft.specs[i], auto: false }
  }
</script>

<div class="flex flex-col gap-5">
  {#if draft.specsSupported}
    <div class="flex flex-col gap-1.5">
      <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
        {t('settings.discoveredFields')}
      </div>
      <p class="text-micro text-text-muted">{t('settings.discoveredFieldsHint')}</p>
      {#if draft.specs.length === 0}
        <p class="text-[12px] text-text-secondary">{t('settings.noDiscoveredFields')}</p>
      {:else}
        <div class="flex flex-col gap-1">
          {#each draft.specs as spec, i (spec.alias)}
            <div class="flex items-center gap-2">
              <span class="w-40 truncate text-[12px] text-text-primary" title={spec.alias}>
                {spec.label}
                {#if spec.auto === false}
                  <span class="ml-1 text-micro text-accent">{t('settings.pinned')}</span>
                {/if}
              </span>
              <span class="relative flex">
                <select
                  class="{SELECT} w-24"
                  value={spec.role}
                  onchange={(e) => {
                    touchSpec(i)
                    draft.specs[i].role = e.currentTarget.value
                  }}
                >
                  <option value="facet">{t('settings.roleFacet')}</option>
                  <option value="body">{t('settings.roleBody')}</option>
                  <option value="user">{t('settings.roleUser')}</option>
                  <option value="plain">{t('settings.rolePlain')}</option>
                </select>
                <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
              </span>
              <span class="relative flex">
                <select
                  class="{SELECT} w-32"
                  value={spec.kind ?? ''}
                  onchange={(e) => {
                    touchSpec(i)
                    draft.specs[i].kind = e.currentTarget.value || undefined
                  }}
                >
                  <option value="">{t('settings.kindNone')}</option>
                  <option value="option">option</option>
                  <option value="multi_option">multi_option</option>
                  <option value="user">user</option>
                  <option value="version_array">version_array</option>
                </select>
                <Icon name="chevron-right" size={13} class={SELECT_CHEVRON} />
              </span>
              <button
                type="button"
                class="text-micro text-text-muted hover:text-status-reopen"
                onclick={() => {
                  draft.specsTouched = true
                  draft.specs = draft.specs.filter((_, j) => j !== i)
                }}>{t('settings.removeField')}</button
              >
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
  <div class="flex flex-col gap-1.5">
    <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('settings.fieldMap')}
    </div>
    <KeyValueRows
      bind:rows={draft.fieldMap}
      keyLabel={t('settings.alias')}
      valueLabel={t('settings.jiraFieldId')}
      keyPlaceholder="severity"
      valuePlaceholder="customfield_10050"
    />
  </div>
  <div class="flex flex-col gap-1.5">
    <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('settings.editableFields')}
    </div>
    <KeyValueRows
      bind:rows={draft.editableFields}
      keyLabel={t('settings.alias')}
      valueLabel={t('settings.jiraFieldId')}
      keyPlaceholder="solution"
      valuePlaceholder="customfield_10092"
    />
  </div>
  <label class="flex flex-col gap-1">
    <span class="text-micro text-text-secondary">
      {t('settings.adfSearchFields')}
    </span>
    <input class="{INPUT} font-mono" bind:value={draft.bodyFieldsText} placeholder="customfield_10101" />
  </label>
</div>
