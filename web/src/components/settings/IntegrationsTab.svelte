<script lang="ts">
  /*
   * Integrations (desktop only) — GDK-185.
   *
   * One card per agent surface gadak can install into: is it there, what
   * command puts it there, and what that command printed. The command is on
   * screen next to a copy button on purpose — the install is a shell command
   * either way, and a button that runs something the user cannot read is a
   * button they have to trust blindly. The same reason the output streams into
   * the card instead of resolving into a toast.
   *
   * The rule this screen is built around: NEVER GUESS SUCCESS. Installing and
   * detecting are both full of exceptions — a socket that dies mid-run, a
   * command that exits 0 while the check still cannot find anything, a run
   * started by another window, a detection that is best-effort by nature. Every
   * one of those has a state of its own here, and "we do not know" is one of
   * them. A check mark is only ever the server's answer to a fresh look.
   *
   * All of the wire reading and state precedence is in lib/integrations.ts,
   * under vitest; this file paints and sequences.
   */
  import { onMount, tick } from 'svelte'
  import { t, type MessageKey } from '../../lib/i18n'
  import { copyText } from '../../lib/copy-text'
  import LoadingState from '../ui/LoadingState.svelte'
  import {
    actionLabelKind,
    fetchIntegrations,
    installBlocked,
    integrationStatus,
    newInstallStream,
    postInstall,
    postRunNote,
    readInstallStream,
    startFailureOutcome,
    type IntegrationItem,
    type IntegrationStatus,
    type InstallStreamState,
  } from '../../lib/integrations'
  import { ADD_BTN, COPY_BTN } from './controls'

  let items = $state<IntegrationItem[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)

  /** What this session's install attempt did, per row. */
  interface Run {
    /** We are streaming an install for this row right now. */
    running: boolean
    /** The server says one is running that we cannot watch (409). */
    foreignRunning: boolean
    stream: InstallStreamState
    /** Exit code of the finished attempt; null while running or never run. */
    exitCode: number | null
    /** The attempt ended with no `exit=` line — outcome genuinely unknown. */
    resultUnknown: boolean
    /** Why we could not run it, or what to make of how it ended. */
    note: string | null
  }
  let runs = $state<Record<string, Run>>({})

  let copiedId = $state<string | null>(null)
  /** Log panels, so a live stream keeps its tail in view. */
  const logEls: Record<string, HTMLElement | undefined> = {}

  const STATUS_LABEL: Record<IntegrationStatus, MessageKey> = {
    checking: 'settings.integrationChecking',
    running: 'settings.integrationRunning',
    failed: 'settings.integrationFailed',
    'result-unknown': 'settings.integrationResultUnknown',
    installed: 'settings.integrationInstalled',
    'not-installed': 'settings.integrationNotInstalled',
    unknown: 'settings.integrationUnknown',
  }

  const STATUS_DOT: Record<IntegrationStatus, string> = {
    checking: 'bg-status-inprogress',
    running: 'bg-status-inprogress animate-pulse',
    failed: 'bg-status-reopen',
    // Its own colour: neither the red of a reported failure nor the grey of a
    // settled "no", because it is a question, and the answer is Re-check.
    'result-unknown': 'bg-status-stale',
    installed: 'bg-status-done',
    'not-installed': 'bg-border-strong',
    // Amber like result-unknown, not grey like a settled "no": "not installed"
    // is actionable, "unknown" is a question — the dot should split them at a
    // scan, not just the label (vision verdict 2026-08-17).
    unknown: 'bg-status-stale',
  }

  const ACTION_LABEL = {
    install: 'settings.integrationInstall',
    update: 'settings.integrationUpdate',
    retry: 'common.retry',
  } as const satisfies Record<string, MessageKey>

  const NOTE_LABEL = {
    busy: 'settings.integrationBusy',
    'unknown-id': 'settings.integrationUnknownId',
    'start-failed': 'settings.integrationStartFailed',
    'no-verdict': 'settings.integrationNoExit',
    'ok-undetected': 'settings.integrationOkUndetected',
  } as const satisfies Record<string, MessageKey>

  function statusOf(item: IntegrationItem): IntegrationStatus {
    const run = runs[item.id]
    return integrationStatus({
      loading,
      running: (run?.running ?? false) || (run?.foreignRunning ?? false),
      installed: item.installed,
      failedExit: run?.exitCode ?? null,
      resultUnknown: run?.resultUnknown ?? false,
    })
  }

  function patchRun(id: string, next: Partial<Run>): void {
    const base: Run = runs[id] ?? {
      running: false,
      foreignRunning: false,
      stream: newInstallStream(),
      exitCode: null,
      resultUnknown: false,
      note: null,
    }
    runs = { ...runs, [id]: { ...base, ...next } }
  }

  async function load(): Promise<void> {
    loading = true
    try {
      items = await fetchIntegrations()
      error = null
    } catch {
      // The route lives on the desktop app's mux only. Reaching this in the
      // desktop app means the server is older or broken, not that the list is
      // empty — so say "could not read", never draw zero integrations. Any
      // items and logs already on screen stay: the error is a banner, not a
      // replacement, or a failed re-read would take away the log the user is
      // reading to find out what happened.
      error = t('settings.integrationsLoadFailed')
    }
    loading = false
  }

  onMount(load)

  /**
   * Ask the server again about one row.
   *
   * The GET covers every row, but the verdict cleared is only this row's: a
   * Re-check on the MCP card must not erase the Setup failed that the Raycast
   * card is still explaining. The log survives — it is history, not status.
   */
  async function recheck(id: string): Promise<void> {
    patchRun(id, { exitCode: null, resultUnknown: false, foreignRunning: false, note: null })
    await load()
  }

  /** Keep the newest output visible without stealing the page's scroll. */
  async function pinLogToBottom(id: string): Promise<void> {
    await tick()
    const el = logEls[id]
    if (el) el.scrollTop = el.scrollHeight
  }

  async function install(item: IntegrationItem): Promise<void> {
    const current = runs[item.id]
    if (current?.running || current?.foreignRunning || installBlocked(item)) return
    patchRun(item.id, {
      running: true,
      foreignRunning: false,
      stream: newInstallStream(),
      exitCode: null,
      resultUnknown: false,
      note: null,
    })

    const started = await postInstall(item.id)
    if ('failure' in started) {
      const { foreignRunning, noteKind } = startFailureOutcome(started.failure)
      // The previous run's log is left alone: nothing new ran, so erasing the
      // only account of the last attempt would be a lie by omission.
      patchRun(item.id, {
        running: false,
        foreignRunning,
        stream: current?.stream ?? newInstallStream(),
        note: t(NOTE_LABEL[noteKind]),
      })
      return
    }

    const final = await readInstallStream(started.stream, (stream) => {
      patchRun(item.id, { stream })
      void pinLogToBottom(item.id)
    })
    patchRun(item.id, {
      running: false,
      foreignRunning: false,
      stream: final,
      // No `exit=` means no verdict. exitCode stays null so nothing reads as a
      // failure either — the pill says the result is unknown and points at
      // Re-check, which is the only honest move left.
      exitCode: final.terminated ? final.exitCode : null,
      resultUnknown: !final.terminated,
    })
    void pinLogToBottom(item.id)

    // Only a reported success re-reads the list, and the check mark comes from
    // that read rather than from our optimism about the exit code.
    if (final.exitCode === 0) {
      patchRun(item.id, { exitCode: null })
      await load()
    }
    const note = postRunNote({
      terminated: final.terminated,
      exitCode: final.exitCode,
      installedAfter: items.find((i) => i.id === item.id)?.installed ?? null,
    })
    if (note !== 'none') patchRun(item.id, { note: t(NOTE_LABEL[note]) })
  }

  async function copyCommand(item: IntegrationItem): Promise<void> {
    if (!item.command) return
    // copy-text.ts owns the desktop-vs-web transport (GDK-178); "copied" shows
    // only for a write that actually happened.
    if (await copyText(item.command)) {
      copiedId = item.id
      setTimeout(() => {
        if (copiedId === item.id) copiedId = null
      }, 1500)
    }
  }
</script>

<div class="flex flex-col gap-2.5" data-testid="integrations-tab">
  <p class="text-micro leading-relaxed text-text-muted">{t('settings.integrationsIntro')}</p>

  {#if error}
    <!-- A banner, not a replacement: whatever is already listed stays, logs
         included. An empty screen answers none of the questions the user has
         at the moment a re-read fails. -->
    <div
      class="flex flex-wrap items-center gap-2 rounded border border-border-subtle bg-bg-elevated px-2 py-1.5"
      data-testid="integrations-error"
    >
      <span class="min-w-0 flex-1 text-micro text-status-reopen">{error}</span>
      <button type="button" class={COPY_BTN} disabled={loading} onclick={() => void load()}>
        {t('settings.integrationRecheck')}
      </button>
    </div>
  {/if}

  {#if loading && items.length === 0}
    <LoadingState label={t('settings.integrationsLoading')} />
  {:else if items.length === 0}
    {#if !error}
      <p class="py-6 text-center text-text-muted">{t('settings.integrationsEmpty')}</p>
    {/if}
  {:else}
    {#each items as item (item.id)}
      {@const status = statusOf(item)}
      {@const run = runs[item.id]}
      {@const blocked = installBlocked(item)}
      {@const busy = (run?.running ?? false) || (run?.foreignRunning ?? false)}
      <section
        class="rounded-md border border-border-subtle bg-bg-base/60 px-3 py-2.5"
        data-testid="integration-row-{item.id}"
      >
        <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span class="text-text-primary">{item.title}</span>
          <span
            class="inline-flex items-center gap-1.5 rounded-full border border-border-subtle px-1.5 py-0.5 text-micro {status ===
            'failed'
              ? 'text-status-reopen'
              : 'text-text-secondary'}"
            data-testid="integration-status-{item.id}"
            data-state={status}
          >
            <span class="h-1.5 w-1.5 flex-none rounded-full {STATUS_DOT[status]}" aria-hidden="true"
            ></span>
            {t(STATUS_LABEL[status])}
          </span>
        </div>

        {#if item.detail}
          <p class="mt-0.5 break-all font-mono text-micro text-text-muted">{item.detail}</p>
        {/if}

        {#if item.command}
          <div
            class="mt-1.5 flex items-center gap-1.5 rounded border border-border-subtle bg-bg-base px-2 py-1"
          >
            <code class="min-w-0 flex-1 overflow-x-auto whitespace-nowrap text-micro text-text-secondary"
              >{item.command}</code
            >
            <button
              type="button"
              class="{COPY_BTN} flex-none"
              onclick={() => void copyCommand(item)}
              aria-label={t('settings.integrationCopyCommand')}
            >
              {copiedId === item.id ? t('settings.copied') : t('settings.copy')}
            </button>
          </div>
        {/if}

        <div class="mt-2 flex flex-wrap items-center gap-1.5">
          <button
            type="button"
            class={ADD_BTN}
            disabled={busy || blocked}
            onclick={() => void install(item)}
            data-testid="integration-install-{item.id}"
          >
            {t(ACTION_LABEL[actionLabelKind(item.installed, run?.exitCode ?? null)])}
          </button>
          <button
            type="button"
            class={COPY_BTN}
            disabled={loading}
            onclick={() => void recheck(item.id)}
            data-testid="integration-recheck-{item.id}"
          >
            {t('settings.integrationRecheck')}
          </button>
        </div>

        {#if item.prerequisite && !item.prerequisite.ok}
          <p
            class="mt-1.5 rounded border border-border-subtle bg-bg-elevated px-2 py-1 text-micro leading-relaxed text-text-secondary"
            data-testid="integration-prereq-{item.id}"
          >
            {item.prerequisite.message || t('settings.integrationPrereq')}
          </p>
        {/if}

        {#if status === 'unknown'}
          <!-- Detection is best-effort for some of these (an MCP host that is
               not installed, a config we cannot read), so "unknown" is an
               everyday state and needs to say what to do about it. -->
          <p class="mt-1.5 text-micro leading-relaxed text-text-muted" data-testid="integration-unknown-hint-{item.id}">
            {t('settings.integrationUnknownHint')}
          </p>
        {/if}

        {#if status === 'failed' && run}
          <p class="mt-1.5 text-micro text-status-reopen">
            {t('settings.integrationExitCode', { code: String(run.exitCode) })}
          </p>
        {/if}

        {#if run?.note}
          <p class="mt-1.5 text-micro leading-relaxed text-text-secondary" data-testid="integration-note-{item.id}">
            {run.note}
          </p>
        {/if}

        {#if run && (run.running || run.stream.lines.length > 0)}
          <!-- Live command output, and it stays after the run whatever the
               outcome: on a failure and on an unknown result the log is the
               only account of what happened, so it is the last thing to clear.
               Bounded height with its own scroll — the dialog body is already a
               scroll region, and a growing log inside it would push the buttons
               that end the run off screen. -->
          <pre
            bind:this={logEls[item.id]}
            class="mt-2 max-h-40 overflow-auto rounded border border-border-subtle bg-bg-base p-2 font-mono text-micro leading-relaxed text-text-secondary"
            role="log"
            aria-label={t('settings.integrationOutput')}
            data-testid="integration-log-{item.id}">{run.stream.lines.join('\n')}</pre>
        {/if}
      </section>
    {/each}
  {/if}
</div>
