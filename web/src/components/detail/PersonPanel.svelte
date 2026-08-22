<script lang="ts">
  /*
   * Person panel ([detail]) — one member, and what the mirror has seen them do.
   *
   * The people axis exists because the mirror can answer a question Jira's own
   * UI makes expensive: everything one person wrote, across issues and pages, in
   * one list. That list is the panel's body. The three links above it are the
   * other legs of the same question — assigned, reported, authored — and each is
   * an existing view, not a new axis: they apply the assignee/reporter filter or
   * open the docs By-author tab.
   *
   * Same shell as DetailPanel and DocumentPanel: no props, subscribes to its
   * store, pinned header over a scrolling body, Esc closes.
   *
   * Counts next to the links are the pool's own answer, and each link lands on a
   * view built from an empty config — so the number on the chip is exactly the
   * number of rows that arrive. A count that cannot be computed locally is not
   * shown; nothing here is estimated.
   */
  import { t, relativeTime, absTime, formatNumber } from '../../lib/i18n'
  import { person } from '../../stores/person.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { pages } from '../../stores/pages.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { issueMatchesPerson } from '../../stores/filters.svelte'
  import { showIssueList } from '../../lib/show-issue-list'
  import { emptyConfig, type ViewConfig } from '../../lib/view-config'
  import type { AuthorComment, IssueLite } from '../../lib/types'
  import { onEscape } from '../../lib/dom-actions'
  // The list's Avatar, not detail/'s: the panel owner must wear the same
  // name-derived color the rows repeat, or the identity link breaks.
  import Avatar from '../list/Avatar.svelte'
  import Icon from '../ui/Icon.svelte'
  import Section from './Section.svelte'

  const email = $derived(person.selectedEmail)
  const member = $derived(person.member)
  const identity = $derived(member?.jira_account_id ?? email)
  /** Address line: real email when the directory has one, else the identity key. */
  const contact = $derived(member?.email || email || '')
  const matches = (issue: IssueLite, role: 'assignee' | 'reporter') =>
    issueMatchesPerson(issue, role, member?.jira_account_id) || issueMatchesPerson(issue, role, email)
  // What this person is called on screen. Falls back to the email so a member
  // the directory has lost still gets a header rather than a blank one.
  const name = $derived(member?.display_name || member?.name || email || '')

  const assignedCount = $derived(
    identity
      ? issues.allIssues.reduce((n, i) => (matches(i, 'assignee') ? n + 1 : n), 0)
      : 0,
  )
  const reportedCount = $derived(
    identity
      ? issues.allIssues.reduce((n, i) => (matches(i, 'reporter') ? n + 1 : n), 0)
      : 0,
  )
  // Pages are grouped by author name, not by account id, so this is the count
  // the By-author tab will actually show. Zero hides the link: sending someone
  // to a tab with no group of theirs in it is a dead end, not a destination.
  const docsCount = $derived(pages.pagesByAuthorCount(name))

  /** Land on a list built from nothing but this filter, so the chip's count and
   *  the resulting list are the same number. */
  function applyView(config: ViewConfig): void {
    showIssueList(config, true)
  }

  function openAssigned(): void {
    if (!identity) return
    const c = emptyConfig()
    c.filters.assignee_email = [identity]
    applyView(c)
  }

  function openReported(): void {
    if (!identity) return
    const c = emptyConfig()
    c.filters.reporter_email = [identity]
    applyView(c)
  }

  function openComment(c: AuthorComment): void {
    if (c.kind === 'page') pages.select(c.key)
    else selection.select(c.key)
  }

  // Same negotiation as DetailPanel: decline an Esc another listener already
  // spent (keymap clear-bulk preventDefault-s first), and spend this one
  // so a later surface does not close with us.
  function onEscapeKey(e: KeyboardEvent): void {
    if (e.defaultPrevented) return
    e.preventDefault()
    person.clear()
  }

</script>

{#if email}
  <!-- Esc closes, the same way it does for an issue or a document. -->
  <div
    class="flex h-full flex-col text-text-primary"
    data-testid="person-panel"
    use:onEscape={onEscapeKey}
  >
    <!-- Header — outside the scroll (see DetailPanel). -->
    <div class="relative z-10 flex-none bg-bg-panel">
      <header class="border-b border-border-strong/70 px-5 pt-4 pb-4">
        <div class="mb-3 flex items-start gap-3">
          <Avatar {name} email={member?.email || email} accountId={member?.jira_account_id} size={36} />
          <div class="min-w-0 flex-1">
            <h2 class="type-subject truncate text-title text-text-primary" data-testid="person-name">
              {name}
            </h2>
            {#if contact}
              <p class="truncate text-micro text-text-muted" title={contact}>{contact}</p>
            {/if}
          </div>
          <button
            type="button"
            onclick={() => person.clear()}
            class="flex h-6 w-6 flex-none items-center justify-center rounded-md text-text-muted transition-colors hover:bg-bg-hover hover:text-text-primary"
            aria-label={t('common.close')}
            title={t('common.closeEsc')}
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <path d="M3 3l8 8M11 3l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
            </svg>
          </button>
        </div>

        <!-- The other three legs of "what has this person done". Short labels:
             whose work it is has already been said by the header two lines up,
             and repeating the name on every chip would wrap the row for no
             information. The full phrasing stays in the tooltip. -->
        <div class="flex flex-wrap items-center gap-2" data-testid="person-links">
          <button
            type="button"
            class="flex h-control-sm items-center gap-1.5 rounded-md border border-border-subtle px-2 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
            onclick={openAssigned}
            title={t('person.assignedTo', { name })}
            data-testid="person-link-assigned"
          >
            <Icon name="user" size={12} class="text-text-muted" />
            <span>{t('person.assigned')}</span>
            <span class="tabular-nums text-text-muted">{formatNumber(assignedCount)}</span>
          </button>
          <button
            type="button"
            class="flex h-control-sm items-center gap-1.5 rounded-md border border-border-subtle px-2 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
            onclick={openReported}
            title={t('person.reportedBy', { name })}
            data-testid="person-link-reported"
          >
            <Icon name="plus-circle" size={12} class="text-text-muted" />
            <span>{t('person.reported')}</span>
            <span class="tabular-nums text-text-muted">{formatNumber(reportedCount)}</span>
          </button>
          {#if docsCount > 0}
            <button
              type="button"
              class="flex h-control-sm items-center gap-1.5 rounded-md border border-border-subtle px-2 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
              onclick={() => pages.openDocsByAuthor(name)}
              title={t('person.docsBy', { name })}
              data-testid="person-link-docs"
            >
              <Icon name="file" size={12} class="text-text-muted" />
              <span>{t('person.docs')}</span>
              <span class="tabular-nums text-text-muted">{formatNumber(docsCount)}</span>
            </button>
          {/if}
        </div>
      </header>
    </div>

    <!-- Body: everything this person wrote, newest first. Own scroller. -->
    <div class="min-h-0 flex-1 overflow-y-auto" data-testid="person-scroll">
      <Section title={t('person.comments')} count={person.total || undefined}>
        {#if person.error === 'unlinked'}
          <p class="text-micro text-text-muted">{t('person.unlinked')}</p>
        {:else if person.error === 'network'}
          <div class="flex flex-col items-start gap-2">
            <p class="text-body text-text-secondary">{t('person.commentsFailed')}</p>
            <button
              type="button"
              onclick={() => person.reload()}
              class="rounded-md border border-border-strong px-3 py-1.5 text-body font-medium text-text-secondary transition-colors hover:bg-bg-hover"
            >
              {t('common.retry')}
            </button>
          </div>
        {:else if person.loading && person.comments.length === 0}
          <!-- Same skeleton as the document body: the header is already on
               screen, only the list is still in flight. -->
          <div class="flex flex-col gap-2" aria-hidden="true">
            <div class="h-3 w-3/4 animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-full animate-pulse rounded bg-bg-elevated"></div>
            <div class="h-3 w-5/6 animate-pulse rounded bg-bg-elevated"></div>
          </div>
        {:else if person.comments.length === 0}
          <p class="text-micro text-text-muted">{t('person.noComments')}</p>
        {:else}
          <div class="-mx-5 anim-enter">
            {#each person.comments as c (`${c.key}-${c.created_at}`)}
              <!-- The row is one sentence: where it was said, on what, when —
                   then the line itself. Clicking opens what it was said on,
                   which is the only reason to read this list. -->
              <button
                type="button"
                class="flex w-full flex-col gap-1 border-b border-border-subtle/70 px-5 py-2 text-left transition-colors hover:bg-bg-hover"
                onclick={() => openComment(c)}
                title={c.title}
                data-testid="person-comment"
                data-comment-key={c.key}
                data-comment-kind={c.kind}
              >
                <span class="flex w-full min-w-0 items-center gap-2">
                  {#if c.kind === 'page'}
                    <span
                      class="flex-none rounded bg-bg-active px-1.5 py-0.5 text-micro font-medium uppercase tracking-wide text-text-muted"
                    >
                      {t('doc.badge')}
                    </span>
                  {:else}
                    <span class="flex-none font-mono text-micro text-accent-text">{c.key}</span>
                  {/if}
                  <span class="min-w-0 flex-1 truncate text-body text-text-primary">{c.title}</span>
                  <span class="flex-none text-micro text-text-muted" title={absTime(c.created_at)}>
                    {relativeTime(c.created_at)}
                  </span>
                </span>
                {#if c.snippet}
                  <span class="w-full truncate text-micro text-text-muted">{c.snippet}</span>
                {/if}
              </button>
            {/each}
          </div>
          {#if person.total > person.comments.length}
            <!-- The list is capped; saying which part of the total is on screen
                 keeps the count above from reading as the list's length. -->
            <p class="pt-3 text-micro text-text-muted" data-testid="person-comment-cap">
              {t('person.showingOf', {
                n: formatNumber(person.comments.length),
                total: formatNumber(person.total),
              })}
            </p>
          {/if}
        {/if}
      </Section>
    </div>
  </div>
{/if}
