<script lang="ts">
  /*
   * Comment composer (write). Anchored under the comments section.
   *  - Auto-height textarea; ⌘/Ctrl+Enter to submit.
   *  - @mention autocomplete: token after '@' searches Jira users → insert `@DisplayName`
   *      and record account_id in mentions (backend turns it into an ADF mention).
   *  - Attachments: button/paste/drop upload → preview chips → inline embed in body.
   *  - Reply: effect consumes write.replyRequest (insert author mention).
   *  - Submit via write.submitComment() optimistic; clear on success, restore text on fail.
   */
  import { t } from '../../lib/i18n'
  import { onMount, tick } from 'svelte'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { createUserSearch } from '../../lib/user-search.svelte'
  import type { CommentMention, JiraUser, UploadedAttachment } from '../../lib/types'
  import { isHostedDemo } from '../../lib/config'
  import { commentDraftKey } from '../../lib/storage'
  import { modifierSymbol } from '../../lib/unified-search'
  import Icon from '../ui/Icon.svelte'

  /** onsubmitted fires after a comment commits — the quick-comment dialog closes on it. */
  let { issueKey, onsubmitted }: { issueKey: string; onsubmitted?: () => void } = $props()

  let text = $state('')
  let mentions = $state<CommentMention[]>([])
  let attachments = $state<UploadedAttachment[]>([])
  let uploading = $state(0)
  let busy = $state(false)
  /** Skip persisting while swapping drafts between issues. */
  let hydrating = $state(false)

  let ta: HTMLTextAreaElement | null = $state(null)
  let fileInput: HTMLInputElement | null = $state(null)
  let dragOver = $state(false)
  // GDK-462: one Esc already spent on this draft (blur, or unfocused consume).
  let escConsumed = $state(false)

  function draftKey(key: string): string {
    return commentDraftKey(key)
  }

  function loadDraft(key: string): string {
    try {
      return localStorage.getItem(draftKey(key)) ?? ''
    } catch {
      return ''
    }
  }

  function saveDraft(key: string, body: string): void {
    try {
      if (body.trim()) localStorage.setItem(draftKey(key), body)
      else localStorage.removeItem(draftKey(key))
    } catch {
      /* private mode / quota — draft is best-effort */
    }
  }

  function clearDraft(key: string): void {
    try {
      localStorage.removeItem(draftKey(key))
    } catch {
      /* noop */
    }
  }

  // Per-issue draft: load when the key changes; persist while typing.
  $effect(() => {
    const key = issueKey
    hydrating = true
    text = loadDraft(key)
    mentions = []
    attachments = []
    escConsumed = false
    closeMention()
    queueMicrotask(() => {
      hydrating = false
      autosize()
    })
  })

  $effect(() => {
    const key = issueKey
    const body = text
    if (hydrating) return
    saveDraft(key, body)
  })

  /* ── Mention autocomplete ── */
  let mOpen = $state(false)
  let mStart = $state(-1) // index of '@' in body
  let mQuery = $state('') // drives createUserSearch; set from onInput
  let mIndex = $state(0)

  const mentionSearch = createUserSearch(() => mQuery, {
    debounceMs: 180,
    minLength: 1,
    onResults: () => {
      mIndex = 0
    },
  })
  // Preserve prior filter: active users with account_id, top 8.
  const mResults = $derived(
    mentionSearch.results.filter((u) => u.active && u.account_id).slice(0, 8),
  )

  function autosize() {
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = `${Math.min(ta.scrollHeight, 200)}px`
  }

  function closeMention() {
    mOpen = false
    mStart = -1
    mQuery = ''
    mIndex = 0
  }

  /** Active '@token' relative to the caret; null if none. */
  function detectMention(): { start: number; query: string } | null {
    if (!ta) return null
    const cur = ta.selectionStart ?? text.length
    const before = text.slice(0, cur)
    const at = before.lastIndexOf('@')
    if (at === -1) return null
    // '@' must be at start or after whitespace (avoid email a@b false positives).
    if (at > 0 && !/\s/.test(before[at - 1])) return null
    const token = before.slice(at + 1)
    if (!/^[^\s@]*$/.test(token)) return null // space/extra @ ends the token
    return { start: at, query: token }
  }

  function onInput() {
    autosize()
    const m = detectMention()
    if (!m) {
      closeMention()
      return
    }
    mStart = m.start
    mOpen = true
    mQuery = m.query
  }

  async function pickMention(u: JiraUser) {
    if (mStart < 0 || !ta) {
      closeMention()
      return
    }
    const cur = ta.selectionStart ?? text.length
    const insert = `@${u.display_name} `
    text = text.slice(0, mStart) + insert + text.slice(cur)
    if (!mentions.some((m) => m.account_id === u.account_id)) {
      mentions = [...mentions, { account_id: u.account_id, display_name: u.display_name }]
    }
    const caret = mStart + insert.length
    closeMention()
    await tick()
    if (ta) {
      ta.focus()
      ta.setSelectionRange(caret, caret)
    }
    autosize()
  }

  /* ── Attachments ── */

  async function handleFiles(list: FileList | File[] | null | undefined) {
    const files = Array.from(list ?? [])
    if (!files.length) return
    await Promise.all(
      files.map(async (f) => {
        uploading++
        try {
          const res = await write.uploadAttachment(issueKey, f)
          if (res) attachments = [...attachments, ...res]
        } finally {
          uploading--
        }
      }),
    )
  }

  function onPaste(e: ClipboardEvent) {
    const files = e.clipboardData?.files
    if (files && files.length) {
      e.preventDefault()
      void handleFiles(files)
    }
  }

  function onDrop(e: DragEvent) {
    dragOver = false
    const files = e.dataTransfer?.files
    if (files && files.length) {
      e.preventDefault()
      void handleFiles(files)
    }
  }

  function removeAttachment(id: string) {
    attachments = attachments.filter((a) => a.id !== id)
  }

  /* ── Submit ── */

  async function submit() {
    const body = text.trim()
    if ((!body && attachments.length === 0) || busy || uploading > 0) return
    busy = true
    const prev = { text, mentions, attachments }
    // Only mentions still present in the body (user may have deleted them). Backend matches strings.
    const used = mentions.filter((m) => body.includes(`@${m.display_name}`))
    text = ''
    mentions = []
    attachments = []
    closeMention()
    queueMicrotask(autosize)
    const ok = await write.submitComment(issueKey, body, used, prev.attachments)
    busy = false
    if (!ok) {
      text = prev.text
      mentions = prev.mentions
      attachments = prev.attachments
      queueMicrotask(autosize)
    } else {
      clearDraft(issueKey)
      onsubmitted?.()
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (mOpen && mResults.length) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        mIndex = (mIndex + 1) % mResults.length
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        mIndex = (mIndex - 1 + mResults.length) % mResults.length
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        void pickMention(mResults[mIndex])
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        closeMention()
        return
      }
    }
    if (e.key === 'Escape') {
      // GDK-462: a destructive default is forbidden. Esc blurs; it does not
      // close the panel or clear the draft. DetailPanel / keymap honour
      // defaultPrevented.
      e.preventDefault()
      escConsumed = true
      ta?.blur()
      return
    }
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault()
      void submit()
    }
  }

  /* ── Handle reply request (write.replyRequest) ── */
  let lastReplyNonce = -1
  $effect(() => {
    const req = write.replyRequest
    if (!req || req.issueKey !== issueKey || req.nonce === lastReplyNonce) return
    lastReplyNonce = req.nonce
    const insert = `@${req.user.display_name} `
    // Prefix the body with the mention (or leave if already there).
    text = text.startsWith(insert) ? text : insert + text
    if (!mentions.some((m) => m.account_id === req.user.account_id)) {
      mentions = [...mentions, { ...req.user }]
    }
    void tick().then(() => {
      if (ta) {
        ta.focus()
        const caret = insert.length
        ta.setSelectionRange(caret, caret)
      }
      autosize()
    })
  })

  const canSubmit = $derived((text.trim().length > 0 || attachments.length > 0) && uploading === 0)

  // Capture phase so an unfocused non-empty draft spends Esc before the
  // shell keymap (registered on bubble at App mount) closes the panel.
  onMount(() => {
    function onWin(e: KeyboardEvent) {
      if (e.key !== 'Escape' || e.defaultPrevented) return
      if (ta && document.activeElement === ta) return
      if (!text.trim() || escConsumed) return
      e.preventDefault()
      escConsumed = true
    }
    window.addEventListener('keydown', onWin, true)
    return () => window.removeEventListener('keydown', onWin, true)
  })
</script>

<div
  class="mt-3 flex flex-col gap-1.5"
  role="group"
  ondragover={(e) => {
    e.preventDefault()
    dragOver = true
  }}
  ondragleave={() => (dragOver = false)}
  ondrop={onDrop}
>
  <div class="relative">
    <textarea
      bind:this={ta}
      bind:value={text}
      oninput={onInput}
      onfocus={() => (escConsumed = false)}
      onkeydown={onKeydown}
      onpaste={onPaste}
      rows="2"
      data-testid="comment-composer"
      placeholder={me.identified || isHostedDemo()
        ? t('write.commentPlaceholder')
        : t('write.commentNeedCredentials')}
      class="w-full resize-none rounded-md border bg-bg-base px-2.5 py-1.5 text-body text-text-primary outline-none transition-colors focus:border-accent {dragOver
        ? 'border-accent border-dashed'
        : 'border-border-strong'}"
    ></textarea>

    <!-- Mention autocomplete dropdown -->
    {#if mOpen && mResults.length}
      <div
        class="absolute bottom-full left-0 z-30 mb-1 max-h-56 w-72 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-overlay"
      >
        {#each mResults as u, i (u.account_id)}
          <button
            type="button"
            class="flex min-h-control-sm w-full items-center gap-2 rounded px-2 py-1 text-left text-body transition-colors {i ===
            mIndex
              ? 'bg-bg-hover'
              : 'hover:bg-bg-hover'}"
            onmousedown={(e) => {
              e.preventDefault()
              void pickMention(u)
            }}
          >
            {#if u.avatar_url}
              <img src={u.avatar_url} alt="" class="h-5 w-5 flex-none rounded-full" />
            {:else}
              <span
                class="flex h-5 w-5 flex-none items-center justify-center rounded-full bg-accent-subtle text-micro text-accent-text"
                >{(u.display_name || '?')[0]}</span
              >
            {/if}
            <span class="truncate text-text-primary">{u.display_name}</span>
            {#if u.email}<span class="ml-auto truncate text-micro text-text-muted">{u.email}</span
              >{/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Attachment preview chips -->
  {#if attachments.length || uploading > 0}
    <div class="flex flex-wrap items-center gap-1.5">
      {#each attachments as a (a.id)}
        <span
          class="inline-flex items-center gap-1.5 rounded-md border border-border-strong bg-bg-elevated py-0.5 pl-1 pr-1.5 text-micro text-text-secondary"
        >
          {#if a.is_image}
            <img src={a.content_url} alt="" class="h-5 w-5 rounded object-cover" />
          {:else}
            <Icon name={a.is_video ? 'film' : 'paperclip'} size={14} class="text-text-muted" />
          {/if}
          <span class="max-w-[160px] truncate">{a.filename}</span>
          <button
            type="button"
            class="flex items-center text-text-muted transition-colors hover:text-status-reopen"
            onclick={() => removeAttachment(a.id)}
            title={t('write.removeAttachment')}><Icon name="x" size={12} /></button
          >
        </span>
      {/each}
      {#if uploading > 0}
        <span class="text-micro text-text-muted">{t('write.uploading', { n: uploading })}</span>
      {/if}
    </div>
  {/if}

  <div class="flex items-center justify-end gap-2">
    <input
      bind:this={fileInput}
      type="file"
      multiple
      class="hidden"
      onchange={(e) => {
        void handleFiles((e.currentTarget as HTMLInputElement).files)
        ;(e.currentTarget as HTMLInputElement).value = ''
      }}
    />
    <button
      type="button"
      onclick={() => fileInput?.click()}
      disabled={!me.identified || busy}
      class="mr-auto inline-flex h-control items-center gap-1.5 rounded-md border border-border-strong px-2 text-body text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary disabled:opacity-40"
      title={t('write.attachFile')}
    >
      <Icon name="paperclip" size={13} />
      {t('write.attachLabel')}
    </button>
    <kbd
      data-testid="comment-shortcut"
      class="rounded border border-border-subtle px-1 text-micro text-text-muted"
    >{t('write.commentShortcut', { mod: modifierSymbol() })}</kbd>
    <button
      type="button"
      onclick={submit}
      disabled={busy || !canSubmit}
      class="inline-flex h-control items-center rounded-md bg-accent px-3 text-body font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-40"
    >
      {busy ? t('write.commentPosting') : t('write.commentButton')}
    </button>
  </div>
</div>
