<script lang="ts">
  /*
   * Detail panel entry ([detail]).
   *
   * No props — subscribes to selection store (`selectedKey`) directly.
   * Mounted inside RightPanel (own overflow-y-auto); content flows here and
   * only the header is sticky (scroll within the panel).
   *
   * Latency hide: on selectedKey change, render header immediately from local
   * pool IssueLite (issues.get); body shows skeleton until getDetailCached arrives.
   */
  import { t } from '../../lib/i18n'
  import { selection } from '../../stores/selection.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { ApiError } from '../../lib/api'
  import { feature, isHostedDemo } from '../../lib/config'
  import type { AdfNode, DetailResponse } from '../../lib/types'
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
  // Local-pool row for instant header (may be missing: linked issues not in pool)
  const lite = $derived(key ? issues.get(key) : undefined)

  let detail = $state<DetailResponse | null>(null)
  let errorKind = $state<null | 'notfound' | 'network'>(null)

  // Race guard: only the last load wins when selection changes quickly.
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

  // Load on selectedKey change; clear state when selection clears.
  // Depend on write.detailNonce so cache updates (e.g. comment confirm) re-read
  // (cache hit → no network; temp comment swaps to the real one).
  $effect(() => {
    const k = selection.selectedKey
    void write.detailNonce
    if (!k) {
      gen++ // invalidate inflight
      detail = null
      errorKind = null
      return
    }
    void load(k)
  })

  // Esc to close
  $effect(() => {
    if (!key) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') selection.clear()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  // detail must match the current key (avoid showing previous detail mid-switch)
  const detailForKey = $derived(detail && key === detail.issue_key ? detail : null)

  // Body-role custom fields, in spec order, only when this issue carries them.
  const bodySections = $derived.by(() => {
    const bodies = detailForKey?.bodies
    if (!bodies) return []
    const out: { alias: string; label: string; node: AdfNode | null; text: string | null }[] = []
    for (const spec of issues.fieldSpecs) {
      if (spec.role !== 'body') continue
      const v = bodies[spec.alias]
      if (v == null) continue
      if (typeof v === 'string') {
        if (v.trim()) out.push({ alias: spec.alias, label: spec.label, node: null, text: v })
      } else {
        out.push({ alias: spec.alias, label: spec.label, node: v, text: null })
      }
    }
    return out
  })
</script>

{#if key}
  <div class="flex h-full flex-col text-text-primary">
    <!-- Header (sticky) -->
    <div class="sticky top-0 z-10 flex-none bg-bg-panel">
      {#if lite}
        <DetailHeader issue={lite} />
        {#if isHostedDemo() && write.demoEdits.has(key)}
          <!-- The banner counts demo edits; this says which issue is one, so a
               changed status here is never mistaken for the snapshot's own. -->
          <p
            class="border-b border-border-subtle bg-accent-strong/10 px-4 py-1.5 text-[11px] text-text-secondary"
            data-testid="demo-edited-notice"
          >
            {t('app.demoEditedIssue')}
          </p>
        {/if}
      {:else}
        <!-- Issue not in pool: minimal header -->
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

    <!-- Body -->
    <div class="min-h-0 flex-1">
      {#if errorKind}
        <!-- Error: 404 (deleted) / network -->
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
        <!-- Skeleton (body loading) -->
        <div class="flex flex-col gap-2 px-4 py-4" aria-hidden="true">
          <div class="h-3 w-3/4 animate-pulse rounded bg-bg-elevated"></div>
          <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
          <div class="h-3 w-5/6 animate-pulse rounded bg-bg-elevated"></div>
          <div class="mt-4 h-3 w-1/2 animate-pulse rounded bg-bg-elevated"></div>
          <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
        </div>
      {:else}
        <!-- Detail body -->
        <div class="anim-enter divide-y divide-border-subtle">
          {#if lite}
            <!-- Renders its own Section, and nothing at all when every field is empty. -->
            <IssueFields issue={lite} developmentOpinion={detailForKey.development_opinion} />
          {/if}

          <!-- Description -->
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

          <!-- Body-role custom fields (재현 단계, QA comment, …) — documents, not chips.
               Board templates put real prose in these; each gets its own section. -->
          {#each bodySections as body (body.alias)}
            <Section title={body.label}>
              <div class="text-[13px] text-text-secondary">
                <AdfContent
                  node={body.node}
                  fallback={body.text}
                  issueKey={key}
                  attachments={detailForKey.attachments}
                />
              </div>
            </Section>
          {/each}

          {#if detailForKey.attachments.length > 0}
            <Section title={t('detail.attachments')} count={detailForKey.attachments.length}>
              <AttachmentGallery attachments={detailForKey.attachments} />
            </Section>
          {/if}

          <!-- QA run context is secondary, after Jira body/evidence. -->
          {#if feature('qa') && detailForKey.qa_context}
            <Section title={t('detail.qaImpact')} count={detailForKey.qa_context.runs.length}>
              <QaImpact context={detailForKey.qa_context} />
            </Section>
          {/if}

          <!-- Comments (+ composer) -->
          <Section title={t('detail.comments')} count={detailForKey.comments.length}>
            <CommentList
              comments={detailForKey.comments}
              issueKey={key}
              attachments={detailForKey.attachments}
            />
            <CommentComposer issueKey={key} />
          </Section>

          <!-- History -->
          {#if detailForKey.history.length > 0}
            <Section title={t('detail.history')} count={detailForKey.history.length}>
              <HistoryTimeline history={detailForKey.history} />
            </Section>
          {/if}

          <!-- Linked issues -->
          {#if detailForKey.linked_issues.length > 0}
            <Section title={t('detail.links')} count={detailForKey.linked_issues.length}>
              <LinkedIssues linked={detailForKey.linked_issues} />
            </Section>
          {/if}

          <!-- Deploy status (only when deploy.state present — old-server compat) -->
          {#if feature('deploy') && detailForKey.deploy?.state}
            <Section title={t('detail.deploy')}>
              <DeployTimeline deploy={detailForKey.deploy} />
            </Section>
          {/if}

          <!-- Linked PRs (same CI/CD source as deploy — gated on deploy flag) -->
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
