<script lang="ts">
  /*
   * 개인 Jira API 토큰 설정 다이얼로그 (쓰기).
   *  - 안내문 + Atlassian API 토큰 발급 링크.
   *  - jira_email(기본값 me.email) + api_token 입력 → 저장 시 서버가 /myself 로 검증.
   *  - 이미 설정됨: display_name / token_hint / verified_at 표시 + 삭제.
   *  LoginDialog 의 모달 패턴을 따른다(Esc/배경 클릭 닫기).
   */
  import { me } from '../../stores/me.svelte'
  import { write } from '../../stores/write.svelte'
  import { absoluteTime } from '../detail/format'

  const API_TOKEN_URL = 'https://id.atlassian.com/manage-profile/security/api-tokens'

  let jiraEmail = $state(write.jiraEmail || me.email || '')
  let apiToken = $state('')
  let error = $state<string | null>(null)
  let busy = $state(false)
  let emailEl: HTMLInputElement | null = $state(null)

  $effect(() => {
    emailEl?.focus()
  })

  async function submit(e: Event) {
    e.preventDefault()
    if (busy) return
    error = null
    busy = true
    const res = await write.saveCredential(jiraEmail.trim(), apiToken.trim())
    busy = false
    if (res.ok) {
      apiToken = ''
    } else {
      error = res.error ?? '자격증명 저장에 실패했습니다.'
    }
  }

  async function remove() {
    if (busy) return
    busy = true
    await write.deleteCredential()
    busy = false
    apiToken = ''
  }

  function close() {
    write.closeSettings()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) close()
  }}
>
  <div
    class="anim-enter w-full max-w-sm rounded-lg border border-border-strong bg-bg-panel p-5 shadow-xl"
    role="dialog"
    aria-modal="true"
    aria-label="Jira 자격증명 설정"
  >
    <h2 class="mb-1 text-[14px] font-semibold text-text-primary">개인 Jira API 토큰</h2>
    <p class="mb-4 text-[12px] leading-relaxed text-text-muted">
      상태 전환·코멘트·이슈 생성은 <span class="text-text-secondary">본인 Jira 계정</span>으로
      수행됩니다. Atlassian
      <a
        href={API_TOKEN_URL}
        target="_blank"
        rel="noopener noreferrer"
        class="text-accent-text hover:underline">API 토큰</a
      >을 발급해 등록하세요.
    </p>

    {#if write.configured}
      <!-- 설정됨 요약 -->
      <div class="mb-4 rounded-md border border-border-subtle bg-bg-elevated px-3 py-2.5 text-[12px]">
        <div class="flex items-center justify-between gap-2">
          <span class="text-text-secondary">{write.displayName || write.jiraEmail}</span>
          <span class="rounded bg-status-done/15 px-1.5 py-0.5 text-[11px] font-medium text-status-done"
            >연결됨</span
          >
        </div>
        <div class="mt-1 flex flex-col gap-0.5 text-[11px] text-text-muted">
          <span>{write.jiraEmail}</span>
          {#if write.tokenHint}<span class="font-mono">토큰 …{write.tokenHint}</span>{/if}
          {#if write.verifiedAt}<span>검증 {absoluteTime(write.verifiedAt)}</span>{/if}
        </div>
      </div>
    {/if}

    <form onsubmit={submit} class="flex flex-col gap-3">
      <label class="flex flex-col gap-1">
        <span class="text-[11px] text-text-secondary">Jira 이메일</span>
        <input
          bind:this={emailEl}
          bind:value={jiraEmail}
          type="email"
          autocomplete="username"
          required
          class="rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 text-[13px] text-text-primary outline-none focus:border-accent"
          placeholder="you@example.com"
        />
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-[11px] text-text-secondary">
          API 토큰 {#if write.configured}<span class="text-text-muted">(교체 시에만 입력)</span>{/if}
        </span>
        <input
          bind:value={apiToken}
          type="password"
          autocomplete="off"
          required
          class="rounded-md border border-border-strong bg-bg-base px-2.5 py-1.5 font-mono text-[13px] text-text-primary outline-none focus:border-accent"
          placeholder="ATATT3x…"
        />
      </label>

      {#if error}
        <p class="text-[12px] text-status-reopen">{error}</p>
      {/if}

      <div class="mt-1 flex items-center justify-between gap-2">
        {#if write.configured}
          <button
            type="button"
            onclick={remove}
            disabled={busy}
            class="rounded-md px-3 py-1.5 text-[12px] text-status-reopen transition-colors hover:bg-status-reopen/10 disabled:opacity-50"
          >
            삭제
          </button>
        {:else}
          <span></span>
        {/if}
        <div class="flex items-center gap-2">
          <button
            type="button"
            onclick={close}
            class="rounded-md px-3 py-1.5 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
          >
            닫기
          </button>
          <button
            type="submit"
            disabled={busy}
            class="rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover disabled:opacity-50"
          >
            {busy ? '검증 중…' : write.configured ? '토큰 교체' : '저장'}
          </button>
        </div>
      </div>
    </form>
  </div>
</div>
