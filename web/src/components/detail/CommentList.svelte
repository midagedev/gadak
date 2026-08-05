<script lang="ts">
  /*
   * Comment list ([detail]). Chronological; author avatar+name+relative time;
   * raw_body (ADF). Missing/failed raw_body falls back to plain body (AdfContent).
   */
  import { t } from '../../lib/i18n'
  import type { DetailAttachment, DetailComment } from '../../lib/types'
  import { issues } from '../../stores/issues.svelte'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { relativeTime, absoluteTime } from './format'
  import Avatar from './Avatar.svelte'
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
</script>

{#if all.length === 0}
  <p class="text-[12px] text-text-muted italic">{t('detail.noComments')}</p>
{:else}
  <ol class="flex flex-col gap-4">
    {#each all as c (c.comment_id)}
      <li class="group anim-enter" class:opacity-60={c.comment_id.startsWith('temp-')}>
        <div class="mb-1 flex items-center gap-2">
          <Avatar
            member={issues.memberOf(c.author_email)}
            name={c.author}
            email={c.author_email}
            size={20}
          />
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
        <div class="ml-[28px] text-[13px] leading-relaxed text-text-secondary">
          <AdfContent node={c.raw_body} {issueKey} {attachments} fallback={c.body} emptyLabel={t('detail.emptyComment')} />
        </div>
      </li>
    {/each}
  </ol>
{/if}
