<script lang="ts">
  /* Bootstrap directory: who is on the team, and how Jira spells them.
     `openMember` lives in the dialog because a JSON edit reloads the rows and
     an index left over from the old list would point at a different person. */
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import { INPUT, ADD_BTN, DEL_BTN } from './controls'
  import type { SettingsDraft } from './draft'

  let {
    draft = $bindable(),
    openMember = $bindable(),
  }: { draft: SettingsDraft; openMember: number | null } = $props()
</script>

<div class="flex flex-col gap-1.5">
  {#if draft.members.length === 0}
    <p class="text-micro text-text-secondary">{t('settings.membersEmpty')}</p>
  {:else}
    <div class="flex gap-1.5 text-micro text-text-muted">
      <span class="flex-1">{t('settings.memberEmail')}</span>
      <span class="w-24 flex-none">{t('settings.memberName')}</span>
      <span class="w-20 flex-none">{t('common.group')}</span>
      <span class="flex-1">{t('settings.memberAccountId')}</span>
      <span class="w-12 flex-none"></span>
    </div>
    {#each draft.members as row, i (i)}
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center gap-1.5">
          <input class="{INPUT} flex-1" bind:value={row.email} placeholder="a@b.c" />
          <input class="{INPUT} w-24 flex-none" bind:value={row.name} />
          <input class="{INPUT} w-20 flex-none font-mono" bind:value={row.group} />
          <input class="{INPUT} flex-1 font-mono" bind:value={row.jira_account_id} />
          <button
            type="button"
            class="flex w-6 flex-none items-center justify-center text-text-muted transition-colors hover:text-text-primary"
            title={t('common.detail')}
            onclick={() => (openMember = openMember === i ? null : i)}
            ><Icon
              name="chevron-right"
              size={13}
              class="transition-transform {openMember === i ? 'rotate-90' : ''}"
            /></button
          >
          <button
            type="button"
            class={DEL_BTN}
            title={t('settings.deleteRow')}
            onclick={() => {
              draft.members = draft.members.filter((_, j) => j !== i)
              openMember = null
            }}><Icon name="x" size={13} /></button
          >
        </div>
        {#if openMember === i}
          <div class="ml-2 grid grid-cols-2 gap-1.5 border-l border-border-subtle pl-3 pb-1">
            <label class="flex flex-col gap-0.5">
              <span class="text-micro text-text-muted">{t('settings.displayName')}</span>
              <input class={INPUT} bind:value={row.display_name} />
            </label>
            <label class="flex flex-col gap-0.5">
              <span class="text-micro text-text-muted">{t('settings.department')}</span>
              <input class={INPUT} bind:value={row.department} />
            </label>
            <label class="flex flex-col gap-0.5">
              <span class="text-micro text-text-muted">{t('settings.jobTitle')}</span>
              <input class={INPUT} bind:value={row.job_role} />
            </label>
            <label class="flex flex-col gap-0.5">
              <span class="text-micro text-text-muted">{t('settings.avatarUrl')}</span>
              <input class={INPUT} bind:value={row.avatar_url} />
            </label>
          </div>
        {/if}
      </div>
    {/each}
  {/if}
  <button
    type="button"
    class={ADD_BTN}
    onclick={() =>
      (draft.members = [
        ...draft.members,
        {
          email: '',
          name: '',
          display_name: '',
          group: '',
          department: '',
          job_role: '',
          jira_account_id: '',
          avatar_url: '',
        },
      ])}>{t('settings.addMember')}</button
  >
</div>
