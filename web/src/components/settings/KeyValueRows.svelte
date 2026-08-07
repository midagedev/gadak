<script lang="ts">
  /*
   * Key-value row editor (shared by fieldMap / editableFields).
   *  Caller drops empty-key rows on save.
   */
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import { INPUT, ADD_BTN, DEL_BTN } from './controls'
  let {
    rows = $bindable(),
    keyLabel,
    valueLabel,
    keyPlaceholder = '',
    valuePlaceholder = '',
  }: {
    rows: { k: string; v: string }[]
    keyLabel: string
    valueLabel: string
    keyPlaceholder?: string
    valuePlaceholder?: string
  } = $props()
</script>

<div class="flex flex-col gap-1.5">
  <div class="flex gap-1.5 text-micro text-text-muted">
    <span class="flex-1">{keyLabel}</span>
    <span class="flex-1">{valueLabel}</span>
    <span class="w-6 flex-none"></span>
  </div>
  {#each rows as row, i (i)}
    <div class="flex items-center gap-1.5">
      <input class="{INPUT} flex-1" bind:value={row.k} placeholder={keyPlaceholder} />
      <input class="{INPUT} flex-1 font-mono" bind:value={row.v} placeholder={valuePlaceholder} />
      <button
        type="button"
        class={DEL_BTN}
        title={t('settings.deleteRow')}
        onclick={() => (rows = rows.filter((_, j) => j !== i))}
      >
        <Icon name="x" size={13} />
      </button>
    </div>
  {/each}
  <button
    type="button"
    class={ADD_BTN}
    onclick={() => (rows = [...rows, { k: '', v: '' }])}
  >
    {t('settings.addRow')}
  </button>
</div>
