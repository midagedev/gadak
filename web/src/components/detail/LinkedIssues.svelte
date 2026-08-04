<script lang="ts">
  /*
   * 연결 이슈 ([detail]). 타입/방향 라벨 + 키 + 요약.
   * 클릭 시 selection.select(key) — 로컬 풀에 있으면 즉시 전환, 없으면 detail 이 로드한다.
   */
  import { t } from '../../lib/i18n'
  import type { LinkedIssue } from '../../lib/types'
  import { selection } from '../../stores/selection.svelte'
  import { issues } from '../../stores/issues.svelte'

  let { linked }: { linked: LinkedIssue[] } = $props()

  /** 방향/타입에서 사람이 읽는 라벨. 백엔드가 direction 에 문구를 주면 그대로 쓴다. */
  function label(l: LinkedIssue): string {
    return (l.direction && l.direction.trim()) || l.type || t('detail.linked')
  }

  // 백엔드가 같은 링크(key+direction)를 중복으로 줄 때가 있다. 중복 키로 렌더하면
  // Svelte each_key_duplicate 로 상세 패널(그리고 앱 전체)이 죽으므로 먼저 dedupe 한다.
  const uniqueLinked = $derived.by(() => {
    const seen = new Set<string>()
    const out: LinkedIssue[] = []
    for (const l of linked) {
      const k = `${l.key ?? ''}|${l.direction ?? ''}`
      if (seen.has(k)) continue
      seen.add(k)
      out.push(l)
    }
    return out
  })
</script>

{#if uniqueLinked.length === 0}
  <p class="text-[12px] text-text-muted italic">{t('detail.noLinks')}</p>
{:else}
  <ul class="flex flex-col gap-1">
    {#each uniqueLinked as l (l.key + l.direction)}
      <li>
        <button
          type="button"
          onclick={() => selection.select(l.key)}
          class="group flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-bg-hover"
        >
          <span class="mt-px flex-none text-[10px] text-text-muted">{label(l)}</span>
          <span class="min-w-0 flex-1">
            <span class="flex items-center gap-1.5">
              <span class="font-mono text-[11px] font-medium text-accent-text">{l.key}</span>
              {#if issues.get(l.key)}
                <span class="h-1 w-1 rounded-full bg-status-done" title={t('detail.inLocalPool')}></span>
              {/if}
            </span>
            <span class="block truncate text-[12px] text-text-secondary group-hover:text-text-primary">
              {l.summary ?? issues.get(l.key)?.summary ?? ''}
            </span>
          </span>
        </button>
      </li>
    {/each}
  </ul>
{/if}
