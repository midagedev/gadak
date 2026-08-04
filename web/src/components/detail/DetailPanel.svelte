<script lang="ts">
  /*
   * 상세 패널 진입점 ([detail]).
   *
   * props 없음 — selection 스토어(`selectedKey`)를 직접 구독한다.
   * RightPanel(480px, 자체 overflow-y-auto) 안에 마운트되므로 여기선 내용만 흐르게 두고
   *  헤더만 sticky 로 고정한다(패널 내부 스크롤).
   *
   * 레이턴시 은닉: selectedKey 변경 즉시 로컬 풀(issues.get)의 IssueLite 로 헤더를 렌더하고
   *  본문은 스켈레톤을 보이다가 detail(getDetailCached) 도착 시 교체한다.
   */
  import { t } from '../../lib/i18n'
  import { selection } from '../../stores/selection.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { ApiError } from '../../lib/api'
  import { feature } from '../../lib/config'
  import type { DetailResponse } from '../../lib/types'
  import { getDetailCached, invalidate } from './cache.svelte'
  import { jiraUrl } from './format'
  import DetailHeader from './DetailHeader.svelte'
  import IssueFields from './IssueFields.svelte'
  import QaImpact from './QaImpact.svelte'
  import AdfContent from './AdfContent.svelte'
  import AttachmentGallery from './AttachmentGallery.svelte'
  import Section from './Section.svelte'
  import CommentList from './CommentList.svelte'
  import CommentComposer from '../write/CommentComposer.svelte'
  import HistoryTimeline from './HistoryTimeline.svelte'
  import LinkedIssues from './LinkedIssues.svelte'
  import PrList from './PrList.svelte'
  import DeployTimeline from './DeployTimeline.svelte'

  const key = $derived(selection.selectedKey)
  // 헤더 즉시 렌더용 로컬 풀 항목(없을 수도 있음: 풀에 없는 연결 이슈 등)
  const lite = $derived(key ? issues.get(key) : undefined)

  let detail = $state<DetailResponse | null>(null)
  let errorKind = $state<null | 'notfound' | 'network'>(null)

  // 경쟁 조건 방지: 빠르게 선택이 바뀌면 마지막 로드만 반영한다.
  let gen = 0

  async function load(k: string): Promise<void> {
    const my = ++gen
    errorKind = null
    try {
      const d = await getDetailCached(k)
      if (my !== gen) return // stale
      detail = d
    } catch (e) {
      if (my !== gen) return
      const status = e instanceof ApiError ? e.status : 0
      errorKind = status === 404 ? 'notfound' : 'network'
      detail = null
    }
  }

  function retry(): void {
    if (key) {
      invalidate(key)
      void load(key)
    }
  }

  // selectedKey 변경 시 로드. 선택 해제되면 상태 초기화.
  //  write.detailNonce 를 의존성으로 읽어, 코멘트 확정 등으로 캐시가 바뀌면 다시 읽는다
  //  (캐시 히트라 네트워크 왕복 없음 → 임시 코멘트가 실제 코멘트로 교체됨).
  $effect(() => {
    const k = selection.selectedKey
    void write.detailNonce
    if (!k) {
      gen++ // 인플라이트 무효화
      detail = null
      errorKind = null
      return
    }
    void load(k)
  })

  // Esc 로 닫기
  $effect(() => {
    if (!key) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') selection.clear()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  // 현재 detail 이 지금 선택된 키의 것인지(키 전환 직후 이전 detail 표시 방지)
  const detailForKey = $derived(detail && key === detail.issue_key ? detail : null)
</script>

{#if key}
  <div class="flex h-full flex-col text-text-primary">
    <!-- 헤더 (sticky) -->
    <div class="sticky top-0 z-10 flex-none bg-bg-panel">
      {#if lite}
        <DetailHeader issue={lite} />
      {:else}
        <!-- 풀에 없는 이슈: 최소 헤더 -->
        <header class="flex items-center justify-between border-b border-border-subtle px-4 py-3">
          <a
            href={jiraUrl(key)}
            target="_blank"
            rel="noopener noreferrer"
            class="font-mono text-[12px] font-medium text-accent-text hover:underline"
          >
            {key}
          </a>
          <button
            type="button"
            onclick={() => selection.clear()}
            class="flex h-6 w-6 items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
            aria-label={t('common.close')}
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <path d="M3 3l8 8M11 3l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
            </svg>
          </button>
        </header>
      {/if}
    </div>

    <!-- 본문 -->
    <div class="min-h-0 flex-1">
      {#if errorKind}
        <!-- 에러: 404(삭제) / 네트워크 -->
        <div class="flex flex-col items-center gap-3 px-6 py-16 text-center">
          <p class="text-[13px] text-text-secondary">
            {#if errorKind === 'notfound'}
              {t('detail.notFound')}
            {:else}
              {t('detail.loadFailed')}
            {/if}
          </p>
          {#if errorKind === 'network'}
            <button
              type="button"
              onclick={retry}
              class="rounded-md border border-border-strong px-3 py-1.5 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover"
            >
              {t('common.retry')}
            </button>
          {/if}
        </div>
      {:else if !detailForKey}
        <!-- 스켈레톤 (본문 로딩 중) -->
        <div class="flex flex-col gap-2 px-4 py-4" aria-hidden="true">
          <div class="h-3 w-3/4 animate-pulse rounded bg-bg-elevated"></div>
          <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
          <div class="h-3 w-5/6 animate-pulse rounded bg-bg-elevated"></div>
          <div class="mt-4 h-3 w-1/2 animate-pulse rounded bg-bg-elevated"></div>
          <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
        </div>
      {:else}
        <!-- 상세 본문 -->
        <div class="anim-enter divide-y divide-border-subtle">
          {#if lite}
            <Section title={t('detail.details')}>
              <IssueFields issue={lite} developmentOpinion={detailForKey.development_opinion} />
            </Section>
          {/if}

          <!-- 설명 -->
          <Section title={t('detail.description')}>
            <div class="text-[13px] text-text-secondary">
              <AdfContent
                node={detailForKey.description_adf}
                issueKey={key}
                attachments={detailForKey.attachments}
                emptyLabel={t('detail.noDescription')}
              />
            </div>
          </Section>

          {#if detailForKey.attachments.length > 0}
            <Section title={t('detail.attachments')} count={detailForKey.attachments.length}>
              <AttachmentGallery attachments={detailForKey.attachments} />
            </Section>
          {/if}

          <!-- QA 차수 맥락은 Jira 본문/증빙 다음에 보조 정보로 노출한다. -->
          {#if feature('qa') && detailForKey.qa_context}
            <Section title={t('detail.qaImpact')} count={detailForKey.qa_context.runs.length}>
              <QaImpact context={detailForKey.qa_context} />
            </Section>
          {/if}

          <!-- 코멘트 (+ 작성 컴포저) -->
          <Section title={t('detail.comments')} count={detailForKey.comments.length}>
            <CommentList
              comments={detailForKey.comments}
              issueKey={key}
              attachments={detailForKey.attachments}
            />
            <CommentComposer issueKey={key} />
          </Section>

          <!-- 변경 이력 -->
          {#if detailForKey.history.length > 0}
            <Section title={t('detail.history')} count={detailForKey.history.length}>
              <HistoryTimeline history={detailForKey.history} />
            </Section>
          {/if}

          <!-- 연결 이슈 -->
          {#if detailForKey.linked_issues.length > 0}
            <Section title={t('detail.links')} count={detailForKey.linked_issues.length}>
              <LinkedIssues linked={detailForKey.linked_issues} />
            </Section>
          {/if}

          <!-- 배포 현황 (deploy.state 있을 때만 — 구 서버 호환) -->
          {#if feature('deploy') && detailForKey.deploy?.state}
            <Section title={t('detail.deploy')}>
              <DeployTimeline deploy={detailForKey.deploy} />
            </Section>
          {/if}

          <!-- 연결 PR (배포 연동과 같은 CI/CD 소스에서 온다 — deploy 플래그에 함께 묶임) -->
          {#if feature('deploy') && detailForKey.linked_prs.length > 0}
            <Section title={t('detail.prs')} count={detailForKey.linked_prs.length}>
              <PrList prs={detailForKey.linked_prs} />
            </Section>
          {/if}
        </div>
      {/if}
    </div>
  </div>
{/if}
