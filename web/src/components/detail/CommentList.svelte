<script lang="ts">
  /*
   * Comment list ([detail]). Chronological; author avatar+name+relative time;
   * raw_body (ADF). Missing/failed raw_body falls back to plain body (AdfContent).
   *
   * Consecutive comments by one person are one block: only the first carries the
   * avatar/name/time header, the rest are indented body under it. A Jira thread
   * is usually one or two people thinking out loud, so the un-grouped list spent
   * most of its vertical space repeating the same name and the same timestamp.
   */
  import { t } from '../../lib/i18n'
  import type { DetailAttachment, DetailComment } from '../../lib/types'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { effectiveCategory } from '../../lib/view-config'
  import { claimStands, hasDoneWord } from '../../lib/done-words'
  import { relativeTime, absoluteTime } from './format'
  // The list's Avatar, not a detail-local one: a person must wear the same
  // name-derived color here that they wear in every row behind this panel.
  import Avatar from '../list/Avatar.svelte'
  import BotBadge from '../list/BotBadge.svelte'
  import AdfContent from './AdfContent.svelte'

  function reply(c: DetailComment) {
    if (!c.author_account_id || !c.author) return
    write.requestReply(issueKey, { account_id: c.author_account_id, display_name: c.author })
  }

  let {
    comments,
    issueKey,
    attachments = [],
  }: {
    comments: DetailComment[]
    issueKey: string
    attachments?: DetailAttachment[]
  } = $props()

  // Optimistic comments (pre-server confirm) — drop ones already in the server list.
  const pending = $derived.by<DetailComment[]>(() => {
    const list = write.pending(issueKey)
    if (list.length === 0) return []
    const ids = new Set(comments.map((c) => c.comment_id))
    return list.filter((c) => !ids.has(c.comment_id))
  })
  const all = $derived([...comments, ...pending])

  /*
   * Grouping rule: same author, and posted within 30 minutes of the comment
   * above it. Chat clients use ~5 minutes because that is the cadence of chat;
   * issue comments arrive at the pace of someone working, so a same-author run
   * inside half an hour is one train of thought — a root cause, then the patch,
   * then the verification — and reads as one block. Anything longer is a
   * separate visit and gets its own header.
   *
   * Identity is the account id when Jira gave us one, then the email, then the
   * display name. Requiring the *same* field to match on both sides keeps two
   * people who share a display name apart, and it makes the group's reply
   * target unambiguous: every comment in a group would produce the same
   * requestReply payload, which is why one Reply on the header is not a loss.
   */
  const GROUP_WINDOW_MS = 30 * 60_000

  function identity(c: DetailComment): string | null {
    return c.author_account_id ?? c.author_email ?? c.author ?? null
  }

  function continues(prev: DetailComment | null, c: DetailComment): boolean {
    if (!prev) return false
    const id = identity(prev)
    if (!id || id !== identity(c)) return false
    if (!prev.created_at || !c.created_at) return false
    const gap = Date.parse(c.created_at) - Date.parse(prev.created_at)
    // NaN (unparseable date on either side) fails this and starts a new group.
    return gap >= 0 && gap <= GROUP_WINDOW_MS
  }

  const rows = $derived.by(() => {
    const out: { c: DetailComment; head: boolean }[] = []
    let prev: DetailComment | null = null
    for (const c of all) {
      out.push({ c, head: !continues(prev, c) })
      prev = c
    }
    return out
  })

  /* ── Coaching, M2 (G2) ──
     The newest comment says done — with the Go retro's own done-word
     vocabulary (done-words.ts, lockstep with cmd/gadak/retro.go) — while the
     issue's status does not. The offer is a quiet verb on the comment's own
     row header: one click opens the header's existing status menu (write's
     transitionMenuRequest bridge), so no new write path exists. Only the
     newest comment counts, and an optimistic temp- newest falls back to
     nothing rather than to the comment above it — the button claims "the
     latest comment says done", and a stale done-word on an older comment
     would be a different claim. */
  const issueRow = $derived(issues.pool.get(issueKey))
  const newest = $derived(all.length > 0 ? all[all.length - 1] : null)
  const showMoveToDone = $derived.by(() => {
    if (!newest || newest.comment_id.startsWith('temp-')) return false
    if (!hasDoneWord(newest.body)) return false
    if (!issueRow || effectiveCategory(issueRow) === 'done') return false
    // A claim the last status change already answered is not a mismatch —
    // the comment must be newer than status_changed_at (claimStands).
    if (!claimStands(newest.created_at, issueRow.status_changed_at)) return false
    // Same identity gate as the reply button beside it — the click opens a
    // write surface.
    if (!me.identified) return false
    return true
  })
  const moveToDoneWhy = $derived(
    issueRow ? t('detail.moveToDoneWhy', { status: issueRow.status }) : '',
  )
  function moveToDone(): void {
    write.requestTransitionMenu(issueKey)
  }
</script>

{#if all.length > 0}
  <ol>
    {#each rows as { c, head } (c.comment_id)}
      <!-- 20px and an avatar row open a new speaker; 12px, a hairline and a
           timestamp inside the text column continue the current one.

           The continuation needs all three. A paragraph inside one comment is
           already separated by 6.5px of its own margin, so the earlier 8px gap
           was small enough to read as "same person" only by also being too
           small to read as "different comment": an independent look at NMB-109
           counted its four-comment run as one comment with four paragraphs.
           12px clears the paragraph margin by enough to register as a break,
           the hairline says the break is structural, and the timestamp is what
           actually makes the units countable — it is the only mark that says
           "someone posted this", and it has to be visible statically, not on
           hover, or the count is unavailable to a still screen or a keyboard.
           (2026-08-06) -->
      <li
        class="group anim-enter {head ? 'mt-5 first:mt-0' : 'mt-3'}"
        class:opacity-60={c.comment_id.startsWith('temp-')}
      >
        {#if head}
          <div class="mb-1 flex items-center gap-2">
            <Avatar email={c.author_email ?? null} accountId={c.author_account_id} name={c.author} size={20} />
            <span class="text-body font-medium text-text-primary">
              {c.author ?? c.author_email ?? t('detail.unknownAuthor')}
            </span>
            <BotBadge accountId={c.author_account_id} accountType={c.author_account_type} />
            <span class="text-micro text-text-muted" title={absoluteTime(c.created_at)}>
              {relativeTime(c.created_at)}
            </span>
            {#if showMoveToDone && c.comment_id === newest?.comment_id}
              <!-- Always visible, unlike the reply beside it: the mismatch is
                   the reason this row is interesting at all (C2). -->
              <button
                type="button"
                class="rounded px-1.5 py-0.5 text-micro text-text-muted transition hover:bg-bg-hover hover:text-text-primary"
                onclick={moveToDone}
                data-testid="comment-move-to-done"
                title={moveToDoneWhy}>{t('detail.moveToDone')}</button
              >
            {/if}
            {#if me.identified && c.author_account_id && !c.comment_id.startsWith('temp-')}
              <button
                type="button"
                class="ml-auto rounded px-1.5 py-0.5 text-micro text-text-muted opacity-0 transition hover:bg-bg-hover hover:text-text-primary group-hover:opacity-100"
                onclick={() => reply(c)}
                title={t('detail.replyToComment')}>{t('common.reply')}</button
              >
            {/if}
          </div>
        {/if}
        <!-- 28px = 20px avatar + the 8px gap beside it, so a continuation lines
             up with the first comment's text rather than with its avatar.

             border-strong/70 is the panel's own section-divider rule, measured
             at ΔL* 12.96 on this surface against 10.21 for the border-subtle it
             replaces. Worth recording that the two bounds the fix was given do
             not both hold: the issue-list row divider, named as the ceiling,
             measures ΔL* 8.84 — below the hairline that was already here. It is
             the darker canvas behind the list that makes the same token read
             quieter there, so the ceiling was a judgement about the list's
             ruled-table look (full-width rules at a 42px pitch), not a contrast
             number this column can be held to. Matching the panel's own divider
             is the instruction that survives measurement. -->
        <div
          class="ml-7 {head ? '' : 'border-t border-border-strong/70 pt-3'}"
          title={head ? undefined : absoluteTime(c.created_at)}
        >
          {#if !head}
            <!-- Its own line, left-aligned with the body: a right-aligned or
                 inline time would eat into the text column the prescription
                 protects. Same tier as the group header's time, never brighter.
                 The done-word button rides this line when the newest comment
                 is a grouped continuation (M2). -->
            <div class="mb-1 flex items-center gap-2 text-micro text-text-muted">
              <span>{relativeTime(c.created_at)}</span>
              {#if showMoveToDone && c.comment_id === newest?.comment_id}
                <button
                  type="button"
                  class="rounded px-1.5 py-0.5 text-micro text-text-muted transition hover:bg-bg-hover hover:text-text-primary"
                  onclick={moveToDone}
                  data-testid="comment-move-to-done"
                  title={moveToDoneWhy}>{t('detail.moveToDone')}</button
                >
              {/if}
            </div>
          {/if}
          <div class="text-body leading-relaxed text-text-secondary">
            <AdfContent node={c.raw_body} {issueKey} {attachments} fallback={c.body} emptyLabel={t('detail.emptyComment')} />
          </div>
        </div>
      </li>
    {/each}
  </ol>
{/if}
