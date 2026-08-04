<script lang="ts">
  /*
   * 프레즌스 뷰어 아바타 스택 — 겹쳐 배치, 초과분은 +N.
   *  기존 list/Avatar 재사용(email 로 members 맵에서 프로필/이름 자체 조회, 툴팁 포함).
   *  ring 으로 겹침 경계를 만든다 — 배경색이 다른 곳(행/헤더)에 맞춰 ringClass 로 지정.
   */
  import { t } from '../../lib/i18n'
  import type { Viewer } from '../../stores/presence.svelte'
  import Avatar from '../list/Avatar.svelte'

  let {
    viewers,
    size = 20,
    max = 3,
    ringClass = 'ring-bg-panel',
  }: {
    viewers: readonly Viewer[]
    size?: number
    max?: number
    ringClass?: string
  } = $props()

  const shown = $derived(viewers.slice(0, max))
  const extra = $derived(Math.max(0, viewers.length - max))
  const title = $derived(t('detail.viewingNames', { names: viewers.map((v) => v.name || v.email).join(', ') }))
</script>

{#if viewers.length > 0}
  <div class="flex flex-none items-center" {title}>
    {#each shown as v (v.email)}
      <span class="relative -ml-1 overflow-hidden rounded-full ring-2 first:ml-0 {ringClass}">
        <Avatar email={v.email} name={v.name} {size} />
      </span>
    {/each}
    {#if extra}
      <span
        class="-ml-1 inline-flex flex-none items-center justify-center rounded-full bg-bg-elevated text-[10px] font-medium text-text-secondary ring-2 {ringClass}"
        style:width="{size}px"
        style:height="{size}px"
      >
        +{extra}
      </span>
    {/if}
  </div>
{/if}
