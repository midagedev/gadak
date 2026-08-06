<script lang="ts">
  /*
   * Key-value row editor (shared by fieldMap / editableFields).
   *  Caller drops empty-key rows on save.
   */
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
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

  const INPUT =
    'h-control w-full rounded-md border border-border-strong bg-bg-base px-2 text-[12px] text-text-primary outline-none focus:border-accent'
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
        class="flex w-6 flex-none items-center justify-center text-text-muted transition-colors hover:text-status-reopen"
        title={t('settings.deleteRow')}
        onclick={() => (rows = rows.filter((_, j) => j !== i))}
      >
        <Icon name="x" size={13} />
      </button>
    </div>
  {/each}
  <button
    type="button"
    class="inline-flex h-control-sm items-center self-start rounded-md border border-border-strong px-2 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
    onclick={() => (rows = [...rows, { k: '', v: '' }])}
  >
    {t('settings.addRow')}
  </button>
</div>
