<script lang="ts">
  /* The team taxonomy: group labels and colors, product buckets, and the
     top-down rules that decide which group an issue belongs to. */
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import { INPUT, INPUT_BARE, ADD_BTN, DEL_BTN } from './controls'
  import type { SettingsDraft } from './draft'

  let { draft = $bindable() }: { draft: SettingsDraft } = $props()
</script>

<div class="flex flex-col gap-5">
  <!-- Group labels + colors -->
  <div class="flex flex-col gap-1.5">
    <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('settings.groupLabels')}
    </div>
    {#if draft.groups.length === 0}
      <p class="text-micro text-text-secondary">{t('settings.groupsEmpty')}</p>
    {:else}
      <div class="flex gap-1.5 text-micro text-text-muted">
        <span class="flex-1">{t('settings.groupKey')}</span>
        <span class="flex-1">{t('settings.label')}</span>
        <span class="w-16 flex-none">{t('settings.color')}</span>
        <span class="w-6 flex-none"></span>
      </div>
      {#each draft.groups as row, i (i)}
        <div class="flex items-center gap-1.5">
          <input class="{INPUT} flex-1 font-mono" bind:value={row.key} placeholder="cloud" />
          <input class="{INPUT} flex-1" bind:value={row.label} placeholder={t('settings.label')} />
          <input
            type="color"
            class="h-control w-16 flex-none rounded-md border border-border-strong bg-bg-base"
            value={row.color || '#888888'}
            oninput={(e) => (row.color = e.currentTarget.value)}
            title={row.color || t('common.unspecified')}
          />
          <button
            type="button"
            class={DEL_BTN}
            title={t('settings.deleteRow')}
            onclick={() => (draft.groups = draft.groups.filter((_, j) => j !== i))}
            ><Icon name="x" size={13} /></button
          >
        </div>
      {/each}
    {/if}
    <button
      type="button"
      class={ADD_BTN}
      onclick={() => (draft.groups = [...draft.groups, { key: '', label: '', color: '' }])}
      >{t('settings.addRow')}</button
    >
  </div>

  <!-- Product buckets -->
  <div class="flex flex-col gap-1.5">
    <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('settings.groupToProduct')}
    </div>
    {#if draft.products.length === 0}
      <p class="text-micro text-text-secondary">{t('settings.productsEmpty')}</p>
    {:else}
      <div class="flex gap-1.5 text-micro text-text-muted">
        <span class="flex-1">{t('settings.groupKey')}</span>
        <span class="flex-1">{t('settings.productKey')}</span>
        <span class="flex-1">{t('settings.productLabel')}</span>
        <span class="w-6 flex-none"></span>
      </div>
      {#each draft.products as row, i (i)}
        <div class="flex items-center gap-1.5">
          <input class="{INPUT} flex-1 font-mono" bind:value={row.group} placeholder="cloud" />
          <input class="{INPUT} flex-1 font-mono" bind:value={row.key} placeholder="CLOUD" />
          <input class="{INPUT} flex-1" bind:value={row.label} placeholder="Cloud" />
          <button
            type="button"
            class={DEL_BTN}
            title={t('settings.deleteRow')}
            onclick={() => (draft.products = draft.products.filter((_, j) => j !== i))}
            ><Icon name="x" size={13} /></button
          >
        </div>
      {/each}
    {/if}
    <button
      type="button"
      class={ADD_BTN}
      onclick={() =>
        (draft.products = [...draft.products, { group: '', key: '', label: '' }])}
      >{t('settings.addRow')}</button
    >
  </div>

  <!-- Group classification rules -->
  <div class="flex flex-col gap-1.5">
    <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('settings.groupRules')}
    </div>
    <p class="text-micro leading-relaxed text-text-muted">
      {t('settings.rulesTopDown')} <span class="text-text-secondary">{t('settings.rulesFirstWins')}</span>{t('settings.rulesDetail')}
    </p>
    {#if draft.rules.length === 0}
      <p class="text-micro text-text-secondary">{t('settings.rulesEmpty')}</p>
    {:else}
      <div class="flex gap-1.5 text-micro text-text-muted">
        <span class="w-24 flex-none">{t('common.group')}</span>
        <span class="flex-1">{t('settings.projectsCol')}</span>
        <span class="flex-1">{t('settings.label')}</span>
        <span class="flex-1">{t('settings.componentsCol')}</span>
        <span class="w-6 flex-none"></span>
      </div>
      {#each draft.rules as row, i (i)}
        <div class="flex items-center gap-1.5">
          <input class="{INPUT_BARE} w-24 flex-none font-mono" bind:value={row.group} placeholder="cloud" />
          <input class="{INPUT} flex-1" bind:value={row.projects} placeholder="NMA, NMB" />
          <input class="{INPUT} flex-1" bind:value={row.labels} placeholder="backend" />
          <input class="{INPUT} flex-1" bind:value={row.components} placeholder="api" />
          <button
            type="button"
            class={DEL_BTN}
            title={t('settings.deleteRow')}
            onclick={() => (draft.rules = draft.rules.filter((_, j) => j !== i))}
            ><Icon name="x" size={13} /></button
          >
        </div>
      {/each}
    {/if}
    <button
      type="button"
      class={ADD_BTN}
      onclick={() =>
        (draft.rules = [
          ...draft.rules,
          { group: '', projects: '', labels: '', components: '' },
        ])}>{t('settings.addRow')}</button
    >
  </div>

  <div class="flex flex-col gap-1.5">
    <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('settings.groupQuery')}
    </div>
    <p class="text-micro leading-relaxed text-text-muted">
      {t('settings.groupQueryHint')}
    </p>
    <textarea
      class="{INPUT} min-h-28 font-mono text-micro"
      bind:value={draft.groupQuery}
      spellcheck="false"
      placeholder={'SELECT key, NULL FROM issues_full'}
    ></textarea>
  </div>
</div>
