<script lang="ts">
  import { ArrowUpRight, FlaskConical, LayoutDashboard } from '@lucide/svelte'
  import { config } from '../../lib/config'
  import type { QaIssueContext, QaRunContext, QaSuiteRef } from '../../lib/types'

  let { context }: { context: QaIssueContext } = $props()

  const STATE_CLASS: Record<QaRunContext['state'], string> = {
    blocking: 'bg-status-reopen/15 text-status-reopen',
    retest: 'bg-status-stale/15 text-status-stale',
    verified: 'bg-status-done/15 text-status-done',
    linked: 'bg-accent-subtle/60 text-accent-text',
  }

  const RESULT_META: Record<string, { label: string; cls: string; mark: string }> = {
    passed: { label: '합격', cls: 'text-status-done', mark: '✓' },
    failed: { label: '실패', cls: 'text-status-reopen', mark: '×' },
    blocked: { label: '블록', cls: 'text-status-stale', mark: '!' },
    retest: { label: '재검증', cls: 'text-accent-text', mark: '↻' },
    in_progress: { label: '진행', cls: 'text-accent-text', mark: '◐' },
    untested: { label: '미검증', cls: 'text-text-muted', mark: '○' },
    skipped: { label: '스킵', cls: 'text-text-muted', mark: '−' },
  }

  // 외부 QA 대시보드는 선택 연동이다. config 에 URL 이 없으면 링크를 만들지 않는다.
  function dashboardUrl(run: QaRunContext, suite?: QaSuiteRef): string | null {
    const base = config().qaDashboardUrl.replace(/\/+$/, '')
    if (!base) return null
    const params = new URLSearchParams({ run: run.key })
    if (suite) params.set('suite', suite.key)
    return `${base}/?${params.toString()}`
  }

  function resultMeta(status: string) {
    return RESULT_META[status] ?? { label: status || '알 수 없음', cls: 'text-text-muted', mark: '·' }
  }

  function formatTime(value: string | null): string {
    if (!value) return ''
    return new Intl.DateTimeFormat('ko-KR', {
      month: 'numeric',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(value))
  }
</script>

<div class="flex flex-col gap-4">
  {#each context.runs as run (run.key)}
    <div class="border-b border-border-subtle pb-4 last:border-b-0 last:pb-0">
      <div class="flex min-w-0 items-start gap-2">
        <FlaskConical size={14} class="mt-0.5 flex-none text-text-muted" />
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-1.5">
            <span class="truncate text-[12px] font-semibold text-text-primary" title={run.title}>
              {run.product_label} · {run.title}
            </span>
            <span class="flex-none rounded px-1.5 py-0.5 text-[10px] font-medium {STATE_CLASS[run.state]}">
              {run.state_label}
            </span>
          </div>
          <div class="mt-1.5 flex items-center gap-2 text-[10px] text-text-muted tabular-nums">
            <div class="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-bg-elevated">
              <div
                class="h-full rounded-full bg-text-muted/50"
                style:width={`${Math.max(0, Math.min(100, Math.round(run.completion * 100)))}%`}
              ></div>
            </div>
            <span>{Math.round(run.completion * 100)}%</span>
            <span>{run.executed}/{run.total}</span>
          </div>
        </div>
        <a
          href={run.url}
          target="_blank"
          rel="noopener noreferrer"
          class="flex h-6 w-6 flex-none items-center justify-center rounded text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
          title="Qase에서 열기"
          aria-label="Qase에서 열기"
        >
          <ArrowUpRight size={13} />
        </a>
      </div>

      <div class="mt-2.5 flex flex-wrap gap-1.5 pl-[22px]">
        {#each run.suites as suite (suite.key)}
          <a
            href={dashboardUrl(run, suite)}
            class="inline-flex max-w-full items-center gap-1 rounded border border-border-subtle bg-bg-elevated px-1.5 py-0.5 text-[10px] text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
            title={`${suite.path} 영역을 QA 대시보드에서 열기`}
          >
            <LayoutDashboard size={10} class="flex-none" />
            <span class="truncate">{suite.path}</span>
          </a>
        {/each}
      </div>

      {#if run.cases.length > 0}
        <details class="group/cases mt-2.5 pl-[22px]">
          <summary class="cursor-pointer select-none text-[11px] text-text-secondary hover:text-text-primary">
            연결 TC {run.linked_case_count}개
          </summary>
          <div class="mt-1.5 max-h-52 overflow-y-auto border-t border-border-subtle">
            {#each run.cases as qaCase (`${run.key}-${qaCase.qase_case_id}`)}
              {@const meta = resultMeta(qaCase.status)}
              <div class="flex items-start gap-2 border-b border-border-subtle/70 py-1.5 last:border-b-0">
                <span class="w-3 flex-none text-center text-[11px] font-semibold {meta.cls}" title={meta.label}>
                  {meta.mark}
                </span>
                <div class="min-w-0 flex-1">
                  <div class="truncate text-[11px] text-text-secondary" title={qaCase.title || qaCase.case_id}>
                    {qaCase.title || qaCase.case_id}
                  </div>
                  <div class="mt-0.5 flex items-center gap-1.5 text-[9px] text-text-muted">
                    <span class="font-mono">{qaCase.case_id}</span>
                    <span>{meta.label}</span>
                    {#if qaCase.result_time}<span>{formatTime(qaCase.result_time)}</span>{/if}
                  </div>
                </div>
              </div>
            {/each}
          </div>
        </details>
      {/if}
    </div>
  {/each}
</div>
