<script lang="ts">
  /*
   * Detail panel entry ([detail]).
   *
   * No props — subscribes to selection store (`selectedKey`) directly.
   *
   * Shell shape, shared with DocumentPanel and PersonPanel: a full-height flex
   * column whose header is a flex-none sibling of the scrolling body, so the
   * header is outside the scroll rather than sticky inside it. It used to be
   * `sticky top-0` in RightPanel's scroller with the root at `h-full` — which
   * caps the sticky containing block at one screen, so the header slid away
   * after roughly a screen of scrolling (measured: the issue panel's header sat
   * 189px above the viewport at its own scroll bottom, the person panel's
   * 1327px). Nothing here is sticky now, so there is no stacking to get wrong.
   *
   * Latency hide: on selectedKey change, render header immediately from local
   * pool IssueLite (issues.get); body shows skeleton until getDetailCached arrives.
   */
  import { onMount } from 'svelte'
  import { t } from '../../lib/i18n'
  import { selection } from '../../stores/selection.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { feature, isHostedDemo, isStandaloneWorkspace } from '../../lib/config'
  import {
    readViewportRegime,
    subscribeViewportRegime,
    type ViewportRegime,
  } from '../../lib/viewport-regime'
  import type { AdfNode } from '../../lib/types'
  import { cacheEpoch, getDetailCached, invalidate } from '../../lib/detail-cache.svelte'
  import { createResource } from '../../lib/resource.svelte'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { onEscape } from '../../lib/dom-actions'
  import { shells } from '../../lib/issue-shells.svelte'
  import { jiraUrl } from './format'
  import DetailHeader from './DetailHeader.svelte'
  import IssueFields from './IssueFields.svelte'
  import QaImpact from './QaImpact.svelte'
  import AdfContent from './AdfContent.svelte'
  import DescriptionEditor from './DescriptionEditor.svelte'
  import AttachmentGallery from './AttachmentGallery.svelte'
  import Section from './Section.svelte'
  import EpicProgress from './EpicProgress.svelte'
  import CommentList from './CommentList.svelte'
  import CommentComposer from '../write/CommentComposer.svelte'
  import Icon from '../ui/Icon.svelte'
  import HistoryTimeline from './HistoryTimeline.svelte'
  import LinkedIssues from './LinkedIssues.svelte'
  import RelatedDocs from './RelatedDocs.svelte'
  import PrList from './PrList.svelte'
  import IssueRefs from './IssueRefs.svelte'
  import DeployTimeline from './DeployTimeline.svelte'

  const key = $derived(selection.selectedKey)
  // Local-pool row for instant header (may be missing: linked issues not in pool)
  const lite = $derived(key ? issues.get(key) : undefined)

  // GDK-463: overlay-regime detail covers the list. A named back control
  // returns to it; X and the scrim stay. Docked keeps the list beside the
  // panel, so the control is absent there.
  let viewportRegime = $state<ViewportRegime>(readViewportRegime())
  onMount(() => subscribeViewportRegime((r) => (viewportRegime = r)))

  // The live session table, polled only while this panel is up (GDK-1162 /
  // GDK-1164-A): the ▶ needs to know which shell is on this issue, and the
  // header's mark needs to know that none is. Refcounted in the store, so the
  // poll stops when the last panel closes.
  onMount(() => shells.track())
  const overlay = $derived(viewportRegime === 'overlay')

  // Load on selectedKey change; clear when selection clears.
  // watch detailNonce so cache updates (e.g. comment confirm) re-read
  // (cache hit → no network; temp comment swaps to the real one).
  const resource = createResource(
    () => selection.selectedKey,
    (k) => getDetailCached(k),
    { watch: () => write.detailNonce + cacheEpoch() },
  )
  const detail = $derived(resource.data)
  const errorKind = $derived(resource.errorKind)

  function retry(): void {
    if (key) {
      invalidate(key)
      resource.reload()
    }
  }

  // Esc to close, unless this Esc was already spent — the shell gives the first
  // one back to a live multi-selection and BulkBar to an open popover, and
  // closing the panel in the same keystroke would cost the user a batch they
  // are still assembling. defaultPrevented is the signal rather than reading the
  // stores: listener order is registration order, and this one registers last
  // (on selection), so by the time it runs the stores are already cleared.
  //
  // GDK-462: a focused composer preventDefault+blurs; this listener then arms
  // so the *next* Esc can close. A non-empty draft with no focus spends one
  // Esc the same way. Clearing the draft is forbidden — the localStorage
  // composer already keeps it, and reopening the issue restores it.
  // Armed-for-key rather than a flag an effect resets on selection change:
  // "armed" is per-issue, so it is the comparison — when the key moves, this
  // reads false in the same update, with no effect ordering to keep honest
  // (the GDK-692 shape this repo guards against).
  let commentEscArmedFor = $state<string | null>(null)
  let composer = $state<ReturnType<typeof CommentComposer> | null>(null)
  const commentEscArmed = $derived(commentEscArmedFor !== null && commentEscArmedFor === key)

  function onEscapeKey(e: KeyboardEvent): void {
    if (e.defaultPrevented) {
      commentEscArmedFor = key
      return
    }
    const el = composer?.composerEl() ?? null
    const hasDraft = !!el && el.value.trim().length > 0
    const focused = !!el && document.activeElement === el
    if ((focused || hasDraft) && !commentEscArmed) {
      e.preventDefault()
      if (focused) el.blur()
      commentEscArmedFor = key
      return
    }
    selection.clear()
  }

  // detail must match the current key (avoid showing previous detail mid-switch)
  const detailForKey = $derived(detail && key === detail.issue_key ? detail : null)

  const skeleton = createSkeletonGrace(
    () => !!key && !errorKind && !detailForKey,
    () => key,
  )

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

  // Text-derived page references, both directions in one count — the section
  // renders one merged list, so a header counting them separately would be
  // counting something the reader cannot see.
  const relatedDocCount = $derived.by(() => {
    const keys = new Set<string>()
    for (const p of detailForKey?.ref_pages ?? []) keys.add(p.key)
    for (const p of detailForKey?.backlink_pages ?? []) keys.add(p.key)
    return keys.size
  })
</script>

{#if key}
  <div class="flex h-full flex-col text-text-primary" use:onEscape={onEscapeKey} data-skeleton={skeleton.attr}>
    <!-- Header — outside the scroll, so it is pinned by structure. -->
    <div class="relative z-10 flex-none bg-bg-panel">
      {#if lite}
        <DetailHeader
          issue={lite}
          {overlay}
          waitMs={detailForKey?.wait_ms ?? null}
          progressMs={detailForKey?.progress_ms ?? null}
        />
        {#if isHostedDemo() && write.demoEdits.has(key)}
          <!-- The banner counts demo edits; this says which issue is one, so a
               changed status here is never mistaken for the snapshot's own. -->
          <p
            class="border-b border-border-subtle bg-accent-strong/10 px-5 py-2 text-micro text-text-secondary"
            data-testid="demo-edited-notice"
          >
            {t('app.demoEditedIssue')}
          </p>
        {/if}
      {:else}
        <!-- Issue not in pool: minimal header -->
        <header class="flex items-center justify-between border-b border-border-strong/70 px-5 pt-4 pb-4">
          <div class="flex min-w-0 items-center gap-2">
            {#if overlay}
              <button
                type="button"
                onclick={() => selection.clear()}
                data-testid="issue-detail-back"
                class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
                aria-label={t('feed.backToList')}
                title={t('feed.backToList')}
              >
                <Icon name="arrow-left" size={14} />
              </button>
            {/if}
            <a
              href={jiraUrl(key)}
              target="_blank"
              rel="noopener noreferrer"
              class="font-mono text-micro font-medium text-accent-text hover:underline"
            >
              {key}
            </a>
          </div>
          <button
            type="button"
            onclick={() => selection.clear()}
            class="flex h-6 w-6 items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
            aria-label={t('common.closeEsc')}
            title={t('common.closeEsc')}
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <path d="M3 3l8 8M11 3l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
            </svg>
          </button>
        </header>
      {/if}
    </div>

    <!-- Body — the panel's own scroller. -->
    <div class="min-h-0 flex-1 overflow-y-auto" data-testid="detail-scroll">
      {#if errorKind}
        <!-- Error: 404 (deleted) / network -->
        <div class="flex flex-col items-center gap-3 px-5 py-16 text-center">
          <p class="text-body text-text-secondary" data-testid="detail-load-error">
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
              class="rounded-md border border-border-strong px-3 py-1.5 text-body font-medium text-text-secondary transition-colors hover:bg-bg-hover"
            >
              {t('common.retry')}
            </button>
          {/if}
        </div>
      {:else if !detailForKey}
        {#if skeleton.visible}
          <!-- Skeleton (body loading). Section groups sketch the loaded body's
               own rhythm (fields / description / comments / history / links),
               so the pane reads as "content below" instead of ending
               half-empty (GDK-1063). -->
          <div class="flex flex-col gap-2 px-5 py-4" aria-hidden="true">
            <!-- Details (field rows) -->
            <div class="h-3 w-1/4 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-5/6 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-3/4 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-2/3 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-1/2 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-3/4 animate-pulse rounded bg-bg-elevated"></div>
            <!-- Description -->
            <div class="mt-4 h-3 w-1/3 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-5/6 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-2/3 animate-pulse rounded bg-bg-elevated"></div>
            <!-- Comments -->
            <div class="mt-4 h-3 w-1/4 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-5/6 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-3/4 animate-pulse rounded bg-bg-elevated"></div>
            <!-- History -->
            <div class="mt-4 h-3 w-1/3 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-5/6 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-1/2 animate-pulse rounded bg-bg-elevated"></div>
            <!-- Linked issues -->
            <div class="mt-4 h-3 w-1/4 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-3/4 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-2/3 animate-pulse rounded bg-bg-elevated"></div>
          </div>
        {/if}
      {:else}
        <!-- Detail body -->
        <div class="anim-enter divide-y divide-border-subtle">
          <!-- Direct children of the open issue: epic rollup when the pool has
               epic_key matches, otherwise parent_key matches (stories with
               sub-tasks). Renders nothing when this issue owns no children. -->
          <EpicProgress issueKey={key} />

          {#if lite}
            <!-- Renders its own Section, and nothing at all when every field is empty. -->
            <IssueFields issue={lite} developmentOpinion={detailForKey.development_opinion} />
          {/if}

          <!-- Description. Keyed on the issue alone: a switch must drop an
               open edit (draft, busy) the way the removed reset effect did —
               by remount, not by an effect writing state (GDK-692). The key
               wraps only DescriptionEditor: taking siblings along would reset
               their state, focus, and scroll too. -->
          <Section title={t('detail.description')}>
            {#key key}
              <DescriptionEditor
                issueKey={key}
                node={detailForKey.description_adf}
                fallback={detailForKey.description_text}
                attachments={detailForKey.attachments}
              />
            {/key}
          </Section>

          <!-- Body-role custom fields (재현 단계, QA comment, …) — documents, not chips.
               Board templates put real prose in these; each gets its own section. -->
          {#each bodySections as body (body.alias)}
            <Section title={body.label}>
              <div class="text-body text-text-secondary">
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
            <CommentComposer bind:this={composer} issueKey={key} />
          </Section>

          <!-- History -->
          {#if detailForKey.history.length > 0}
            <Section title={t('detail.history')} count={detailForKey.history.length}>
              <HistoryTimeline history={detailForKey.history} />
            </Section>
          {/if}

          <!-- Linked issues. Always shown so a first link can be added (GDK-85). -->
          <Section title={t('detail.links')} count={detailForKey.linked_issues.length}>
            <LinkedIssues linked={detailForKey.linked_issues} />
          </Section>

          <!-- Documents that name this issue, or that it names. Derived from
               text, so it is next to Linked issues rather than inside it: one
               is what someone drew in Jira, the other is what was written. -->
          {#if relatedDocCount > 0}
            <Section title={t('detail.docs')} count={relatedDocCount}>
              <RelatedDocs
                refPages={detailForKey.ref_pages}
                backlinkPages={detailForKey.backlink_pages}
              />
            </Section>
          {/if}

          <!-- Deploy status (only when deploy.state present — old-server compat) -->
          {#if feature('deploy') && detailForKey.deploy?.state}
            <Section title={t('detail.deploy')}>
              <DeployTimeline deploy={detailForKey.deploy} />
            </Section>
          {/if}

          <!-- Linked PRs. Not gated on the deploy flag: since GDK-495 the list
               also derives from mirrored PR-URL attachments (Linear), which
               exist on workspaces that never configured a deploy plugin.
               Empty is shown so "not mirrored" is distinct from a true empty
               list (GDK-555). config.json does not expose `devStatus`, so a
               connected empty list uses the not-mirrored sentence. -->
          {#if detailForKey.refs && detailForKey.refs.length > 0}
            <!-- Cross-workspace pointers (GDK-1032). Absent unless this
                 issue carries one, so no empty-state sentence: the section
                 exists only where the feature is in use. -->
            <Section title={t('detail.refs')} count={detailForKey.refs.length}>
              <IssueRefs refs={detailForKey.refs} />
            </Section>
          {/if}

          <Section title={t('detail.prs')} count={detailForKey.linked_prs.length}>
            {#if detailForKey.linked_prs.length > 0}
              <PrList prs={detailForKey.linked_prs} />
            {:else if isStandaloneWorkspace()}
              <p class="text-micro text-text-muted">{t('detail.noPrs')}</p>
            {:else}
              <p class="text-micro text-text-muted">{t('detail.prsNotMirrored')}</p>
            {/if}
          </Section>
        </div>
      {/if}
    </div>
  </div>
{/if}
