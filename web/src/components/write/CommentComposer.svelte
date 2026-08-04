<script lang="ts">
  /*
   * 코멘트 작성 입력 (쓰기). 코멘트 섹션 하단 고정.
   *  - textarea 자동 높이, ⌘/Ctrl+Enter 제출.
   *  - @멘션 자동완성: '@' 뒤 토큰으로 Jira 사용자 검색 → 선택 시 본문에 `@표시이름` 삽입 +
   *      account_id 를 mentions 에 기록(백엔드가 ADF mention 노드로 변환).
   *  - 첨부: 버튼/붙여넣기/드롭으로 업로드 → 미리보기 칩 → 코멘트 본문에 인라인 임베드.
   *  - 답글: write.replyRequest(작성자 멘션 삽입 요청)를 effect 로 받아 처리.
   *  - 제출 시 write.submitComment()(옵티미스틱). 성공 시 비우고, 실패 시 텍스트 복원.
   */
  import { t } from '../../lib/i18n'
  import { tick } from 'svelte'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'
  import { searchUsers } from '../../lib/api'
  import type { CommentMention, JiraUser, UploadedAttachment } from '../../lib/types'

  let { issueKey }: { issueKey: string } = $props()

  let text = $state('')
  let mentions = $state<CommentMention[]>([])
  let attachments = $state<UploadedAttachment[]>([])
  let uploading = $state(0)
  let busy = $state(false)

  let ta: HTMLTextAreaElement | null = $state(null)
  let fileInput: HTMLInputElement | null = $state(null)
  let dragOver = $state(false)

  /* ── 멘션 자동완성 ── */
  let mOpen = $state(false)
  let mStart = $state(-1) // 본문에서 '@' 위치
  let mResults = $state<JiraUser[]>([])
  let mIndex = $state(0)
  let mSeq = 0 // 검색 레이스 가드
  let mTimer: ReturnType<typeof setTimeout> | null = null

  function autosize() {
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = `${Math.min(ta.scrollHeight, 200)}px`
  }

  function closeMention() {
    mOpen = false
    mStart = -1
    mResults = []
    mIndex = 0
  }

  /** 커서 기준으로 활성 '@토큰'을 찾는다. 없으면 -1/''。 */
  function detectMention(): { start: number; query: string } | null {
    if (!ta) return null
    const cur = ta.selectionStart ?? text.length
    const before = text.slice(0, cur)
    const at = before.lastIndexOf('@')
    if (at === -1) return null
    // '@' 앞은 문두이거나 공백이어야 한다(이메일 a@b 오탐 방지).
    if (at > 0 && !/\s/.test(before[at - 1])) return null
    const token = before.slice(at + 1)
    if (!/^[^\s@]*$/.test(token)) return null // 공백/추가 @ 있으면 토큰 아님
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
    if (m.query.length < 1) {
      mResults = []
      return
    }
    if (mTimer) clearTimeout(mTimer)
    const q = m.query
    mTimer = setTimeout(() => void runSearch(q), 180)
  }

  async function runSearch(q: string) {
    const seq = ++mSeq
    try {
      const res = await searchUsers(q)
      if (seq !== mSeq) return // 최신 쿼리만 반영
      mResults = res.users.filter((u) => u.active && u.account_id).slice(0, 8)
      mIndex = 0
    } catch {
      if (seq === mSeq) mResults = []
    }
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

  /* ── 첨부 ── */

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

  /* ── 제출 ── */

  async function submit() {
    const body = text.trim()
    if ((!body && attachments.length === 0) || busy || uploading > 0) return
    busy = true
    const prev = { text, mentions, attachments }
    // 본문에 실제로 남아있는 멘션만 전송(사용자가 지웠을 수 있음). 백엔드는 문자열 매칭.
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
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault()
      void submit()
    }
  }

  /* ── 답글 요청 처리 (write.replyRequest) ── */
  let lastReplyNonce = -1
  $effect(() => {
    const req = write.replyRequest
    if (!req || req.issueKey !== issueKey || req.nonce === lastReplyNonce) return
    lastReplyNonce = req.nonce
    const insert = `@${req.user.display_name} `
    // 본문 앞에 멘션을 붙인다(빈 본문이면 그대로, 아니면 앞에).
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
      onkeydown={onKeydown}
      onpaste={onPaste}
      rows="2"
      placeholder={me.identified
        ? t('write.commentPlaceholder')
        : t('write.commentNeedCredentials')}
      class="w-full resize-none rounded-md border bg-bg-base px-2.5 py-1.5 text-[13px] text-text-primary outline-none transition-colors focus:border-accent {dragOver
        ? 'border-accent border-dashed'
        : 'border-border-strong'}"
    ></textarea>

    <!-- 멘션 자동완성 드롭다운 -->
    {#if mOpen && mResults.length}
      <div
        class="absolute bottom-full left-0 z-30 mb-1 max-h-56 w-72 overflow-y-auto rounded-lg border border-border-strong bg-bg-elevated p-1 shadow-xl shadow-black/40"
      >
        {#each mResults as u, i (u.account_id)}
          <button
            type="button"
            class="flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-[12px] transition-colors {i ===
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
                class="flex h-5 w-5 flex-none items-center justify-center rounded-full bg-accent-subtle text-[10px] text-accent-text"
                >{(u.display_name || '?')[0]}</span
              >
            {/if}
            <span class="truncate text-text-primary">{u.display_name}</span>
            {#if u.email}<span class="ml-auto truncate text-[10px] text-text-muted">{u.email}</span
              >{/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>

  <!-- 첨부 미리보기 칩 -->
  {#if attachments.length || uploading > 0}
    <div class="flex flex-wrap items-center gap-1.5">
      {#each attachments as a (a.id)}
        <span
          class="inline-flex items-center gap-1.5 rounded-md border border-border-strong bg-bg-elevated py-0.5 pl-1 pr-1.5 text-[11px] text-text-secondary"
        >
          {#if a.is_image}
            <img src={a.content_url} alt="" class="h-5 w-5 rounded object-cover" />
          {:else}
            <span class="text-[12px]">{a.is_video ? '🎬' : '📎'}</span>
          {/if}
          <span class="max-w-[160px] truncate">{a.filename}</span>
          <button
            type="button"
            class="text-text-muted transition-colors hover:text-status-reopen"
            onclick={() => removeAttachment(a.id)}
            title={t('write.removeAttachment')}>✕</button
          >
        </span>
      {/each}
      {#if uploading > 0}
        <span class="text-[11px] text-text-muted">{t('write.uploading', { n: uploading })}</span>
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
      class="rounded-md border border-border-strong px-2 py-1 text-[12px] text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary disabled:opacity-40"
      title={t('write.attachFile')}>{t('write.attachEmoji')}</button
    >
    <span class="mr-auto text-[11px] text-text-muted">⌘Enter</span>
    <button
      type="button"
      onclick={submit}
      disabled={busy || !canSubmit}
      class="rounded-md bg-accent px-3 py-1 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-40"
    >
      {busy ? t('write.commentPosting') : t('write.commentButton')}
    </button>
  </div>
</div>
