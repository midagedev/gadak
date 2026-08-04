<script lang="ts">
  /*
   * 배포 현황 타임라인 ([detail]).
   *
   * 단계: 머지(N/M PR) → dev 릴리즈 → qa 릴리즈 → qa 스왑(QA 확인 가능) → prod.
   *  도달한 단계는 채운 원 + 강조, 미도달은 회색 빈 원. qa 스왑은 QA팀 핵심이라 강조한다.
   *
   * 방어적 파싱: deploy 는 구 서버 미전송/부분 응답일 수 있어 optional chaining 으로 접근한다.
   *  (state 자체가 없으면 상위 DetailPanel 에서 섹션을 렌더하지 않는다.)
   */
  import type { DeployDetail, DeployState } from '../../lib/types'
  import { absoluteTime } from './format'

  let { deploy }: { deploy: DeployDetail } = $props()

  // 단계 서열 — 도달 여부 판정용.
  const RANK: Record<DeployState, number> = {
    none: 0,
    merged: 1,
    dev: 2,
    qa_preview: 3,
    qa: 4,
    prod: 5,
  }

  const state = $derived((deploy.state ?? 'none') as DeployState)
  const rank = $derived(RANK[state] ?? 0)

  interface Step {
    /** 이 단계 도달에 필요한 최소 서열. */
    at: number
    label: string
    /** 보조 설명(태그/시각 등). */
    detail: string | null
    /** 외부 링크(릴리즈 html_url 등). */
    href: string | null
    /** QA 확인 가능 강조 여부. */
    highlight: boolean
  }

  /** 포함 릴리즈 근거에서 채널로 html_url 을 찾는다(있으면). */
  function releaseUrl(channel: string): string | null {
    const found = (deploy.releases ?? []).find((r) => (r.channel ?? '') === channel)
    return found?.html_url ?? null
  }

  const steps = $derived.by<Step[]>(() => {
    const mergedText =
      deploy.total_prs != null
        ? `${deploy.merged_prs ?? 0}/${deploy.total_prs} PR 머지`
        : deploy.merged_prs != null
          ? `${deploy.merged_prs} PR 머지`
          : null
    const devText = deploy.dev ? `${deploy.dev.tag} · ${absoluteTime(deploy.dev.at)}` : null
    const qaRelText = deploy.qa_release
      ? `${deploy.qa_release.tag} · ${absoluteTime(deploy.qa_release.at)}`
      : null
    const swapText = deploy.qa_swapped_at ? absoluteTime(deploy.qa_swapped_at) : null
    const prodText = deploy.prod_at ? absoluteTime(deploy.prod_at) : null

    return [
      { at: 1, label: '머지', detail: mergedText, href: null, highlight: false },
      { at: 2, label: 'dev 릴리즈', detail: devText, href: releaseUrl('dev'), highlight: false },
      { at: 3, label: 'qa 릴리즈', detail: qaRelText, href: releaseUrl('qa'), highlight: false },
      {
        at: 4,
        label: 'qa 스왑 · QA 확인 가능',
        detail: swapText,
        href: null,
        highlight: true,
      },
      { at: 5, label: 'prod 배포', detail: prodText, href: releaseUrl('prod'), highlight: false },
    ]
  })

  // PR별 포함 여부(근거) — 있으면 접이식 목록으로.
  const prList = $derived(deploy.prs ?? [])
</script>

<ol class="flex flex-col">
  {#each steps as step, i (step.at)}
    {@const reached = rank >= step.at}
    {@const isQaSwap = step.highlight && reached}
    <li class="flex gap-2.5">
      <!-- 좌측 마커 + 연결선 -->
      <div class="flex flex-none flex-col items-center">
        <span
          class="mt-0.5 flex h-3 w-3 flex-none items-center justify-center rounded-full border transition-colors
            {isQaSwap
            ? 'border-[#2dd4bf] bg-[#2dd4bf]'
            : reached
              ? 'border-accent bg-accent'
              : 'border-border-strong bg-transparent'}"
        >
          {#if reached}
            <span class="h-1 w-1 rounded-full {isQaSwap ? 'bg-[#083344]' : 'bg-white'}"></span>
          {/if}
        </span>
        {#if i < steps.length - 1}
          <span
            class="my-0.5 w-px flex-1 {rank > step.at ? 'bg-border-strong' : 'bg-border-subtle'}"
          ></span>
        {/if}
      </div>

      <!-- 단계 내용 -->
      <div class="min-w-0 flex-1 pb-3">
        <div class="flex items-center gap-1.5">
          <span
            class="text-[12px] font-medium {isQaSwap
              ? 'text-[#5eead4]'
              : reached
                ? 'text-text-primary'
                : 'text-text-muted'}"
          >
            {step.label}
          </span>
          {#if step.href}
            <a
              href={step.href}
              target="_blank"
              rel="noopener noreferrer"
              class="text-[11px] text-accent-text hover:underline"
            >
              ↗
            </a>
          {/if}
        </div>
        {#if step.detail}
          <div class="mt-0.5 truncate font-mono text-[11px] text-text-muted" title={step.detail}>
            {step.detail}
          </div>
        {/if}
      </div>
    </li>
  {/each}
</ol>

<!-- PR별 포함 근거(있을 때만) -->
{#if prList.length > 0}
  <div class="mt-1 border-t border-border-subtle pt-3">
    <div class="mb-1.5 text-[11px] font-medium text-text-muted">PR별 포함 여부</div>
    <ul class="flex flex-col gap-1">
      {#each prList as pr (pr.number)}
        <li class="flex items-center gap-2 text-[12px]">
          <span
            class="h-1.5 w-1.5 flex-none rounded-full {pr.included_in
              ? 'bg-status-done'
              : pr.merged
                ? 'bg-status-stale'
                : 'bg-border-strong'}"
            title={pr.included_in
              ? `포함: ${pr.included_in}`
              : pr.merged
                ? '머지됨 · 릴리즈 미포함'
                : '미머지'}
          ></span>
          {#if pr.url}
            <a
              href={pr.url}
              target="_blank"
              rel="noopener noreferrer"
              class="font-mono text-[11px] text-accent-text hover:underline"
            >
              #{pr.number}
            </a>
          {:else}
            <span class="font-mono text-[11px] text-text-muted">#{pr.number}</span>
          {/if}
          <span class="min-w-0 flex-1 truncate text-text-secondary" title={pr.title ?? ''}>
            {pr.title ?? ''}
          </span>
          {#if pr.included_in}
            <span class="flex-none font-mono text-[10px] text-text-muted">{pr.included_in}</span>
          {/if}
        </li>
      {/each}
    </ul>
  </div>
{/if}
