<script lang="ts">
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import { config } from '../../lib/config'
  import type { QaIssueContext, QaRunContext, QaSuiteRef } from '../../lib/types'

  let { context }: { context: QaIssueContext } = $props()

  const STATE_CLASS: Record<QaRunContext['state'], string> = {
    blocking: 'bg-status-reopen/15 text-status-reopen',
    retest: 'bg-status-stale/15 text-status-stale',
    verified: 'bg-status-done/15 text-status-done',
    linked: 'bg-accent-subtle/60 text-accent-text',
  }

  /*
   * Status marks stay as text, unlike the control glyphs swept out of the rest
   * of the UI. This is a seven-value vocabulary read as a column — pass, fail,
   * blocked, retest, running, untested, skipped — and only two of the seven
   * have an obvious line-icon counterpart. Converting those two would leave one
   * column mixing icons and glyphs, which is worse than either; converting all
   * seven is a redesign of the vocabulary, not a sweep. (2026-08-06)
   */
  const RESULT_META: Record<string, { label: string; cls: string; mark: string }> = {
    passed: { label: t('qa.pass'), cls: 'text-status-done', mark: '✓' },
    failed: { label: t('qa.fail'), cls: 'text-status-reopen', mark: '×' },
    blocked: { label: t('qa.block'), cls: 'text-status-stale', mark: '!' },
    retest: { label: t('qa.retest'), cls: 'text-accent-text', mark: '↻' },
    in_progress: { label: t('qa.inProgress'), cls: 'text-accent-text', mark: '◐' },
    untested: { label: t('qa.untested'), cls: 'text-text-muted', mark: '○' },
    skipped: { label: t('qa.skip'), cls: 'text-text-muted', mark: '−' },
  }

  // External QA dashboard is optional. No URL in config → no link.
  function dashboardUrl(run: QaRunContext, suite?: QaSuiteRef): string | null {
    const base = config().qaDashboardUrl.replace(/\/+$/, '')
    if (!base) return null
    const params = new URLSearchParams({ run: run.key })
    if (suite) params.set('suite', suite.key)
    return `${base}/?${params.toString()}`
  }

  function resultMeta(status: string) {
    return RESULT_META[status] ?? { label: status || t('common.unknown'), cls: 'text-text-muted', mark: '·' }
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
        <Icon name="flask" size={14} class="mt-0.5 text-text-muted" />
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-1.5">
            <span class="truncate text-body font-semibold text-text-primary" title={run.title}>
              {run.product_label} · {run.title}
            </span>
            <span class="flex-none rounded px-1.5 py-0.5 text-micro font-medium {STATE_CLASS[run.state]}">
              {run.state_label}
            </span>
          </div>
          <div class="mt-2 flex items-center gap-2 text-micro text-text-muted tabular-nums">
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
          title={t('qa.openQase')}
          aria-label={t('qa.openQase')}
        >
          <Icon name="arrow-up-right" size={13} />
        </a>
      </div>

      <div class="mt-3 flex flex-wrap gap-2 pl-[22px]">
        {#each run.suites as suite (suite.key)}
          <a
            href={dashboardUrl(run, suite)}
            class="inline-flex max-w-full items-center gap-1 rounded border border-border-subtle bg-bg-elevated px-1.5 py-0.5 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
            title={t('qa.openSuite', { path: suite.path })}
          >
            <Icon name="layout-dashboard" size={10} />
            <span class="truncate">{suite.path}</span>
          </a>
        {/each}
      </div>

      {#if run.cases.length > 0}
        <details class="group/cases mt-3 pl-[22px]">
          <summary class="cursor-pointer select-none text-micro text-text-secondary hover:text-text-primary">
            {t('qa.linkedTc', { n: run.linked_case_count })}
          </summary>
          <div class="mt-2 max-h-52 overflow-y-auto border-t border-border-subtle">
            {#each run.cases as qaCase (`${run.key}-${qaCase.qase_case_id}`)}
              {@const meta = resultMeta(qaCase.status)}
              <div class="flex items-start gap-2 border-b border-border-subtle/70 py-1.5 last:border-b-0">
                <span class="w-3 flex-none text-center text-micro font-semibold {meta.cls}" title={meta.label}>
                  {meta.mark}
                </span>
                <div class="min-w-0 flex-1">
                  <div class="truncate text-micro text-text-secondary" title={qaCase.title || qaCase.case_id}>
                    {qaCase.title || qaCase.case_id}
                  </div>
                  <div class="mt-0.5 flex items-center gap-1.5 text-micro text-text-muted">
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
