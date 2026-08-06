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
  import { relativeTime, absoluteTime } from './format'
  // The list's Avatar, not a detail-local one: a person must wear the same
  // name-derived color here that they wear in every row behind this panel.
  import Avatar from '../list/Avatar.svelte'
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
</script>

{#if all.length === 0}
  <p class="text-[12px] text-text-muted italic">{t('detail.noComments')}</p>
{:else}
  <ol>
    {#each rows as { c, head } (c.comment_id)}
      <!-- 20px and an avatar row open a new speaker; 8px and a hairline inside
           the text column continue the current one. The continuation needs the
           hairline: a paragraph inside one comment is already separated by
           6.5px of its own margin, so any gap small enough to read as "same
           person" is too small to read as "different comment", and four
           comments in a row collapse into one four-paragraph wall. -->
      <li
        class="group anim-enter {head ? 'mt-5 first:mt-0' : 'mt-2'}"
        class:opacity-60={c.comment_id.startsWith('temp-')}
      >
        {#if head}
          <div class="mb-1 flex items-center gap-2">
            <Avatar email={c.author_email ?? null} name={c.author} size={20} />
            <span class="text-[12px] font-medium text-text-primary">
              {c.author ?? c.author_email ?? t('detail.unknownAuthor')}
            </span>
            <span class="text-[11px] text-text-muted" title={absoluteTime(c.created_at)}>
              {relativeTime(c.created_at)}
            </span>
            {#if me.identified && c.author_account_id && !c.comment_id.startsWith('temp-')}
              <button
                type="button"
                class="ml-auto rounded px-1.5 py-0.5 text-[11px] text-text-muted opacity-0 transition-colors hover:bg-bg-hover hover:text-text-primary group-hover:opacity-100"
                onclick={() => reply(c)}
                title={t('detail.replyToComment')}>{t('common.reply')}</button
              >
            {/if}
          </div>
        {/if}
        <!-- 28px = 20px avatar + the 8px gap beside it, so a continuation lines
             up with the first comment's text rather than with its avatar. A
             continuation has no visible time of its own, so it carries the
             absolute one as its tooltip. -->
        <div
          class="ml-7 text-[13px] leading-relaxed text-text-secondary {head
            ? ''
            : 'border-t border-border-subtle pt-2'}"
          title={head ? undefined : absoluteTime(c.created_at)}
        >
          <AdfContent node={c.raw_body} {issueKey} {attachments} fallback={c.body} emptyLabel={t('detail.emptyComment')} />
        </div>
      </li>
    {/each}
  </ol>
{/if}
