<script lang="ts">
  /*
   * Workspaces (GDK-1096) — the management half of the workspaces list.
   *
   * Reads the same GET /api/v1/workspaces the sidebar does; the two writes
   * (create, remove) are the outer-mux API that landed with A1
   * (internal/workspace/manage.go). Every refusal the server sends carries
   * the CLI's own wording in `detail` — rendered verbatim below, never
   * re-translated, so the server stays the single owner of what a refusal
   * means and this tab cannot drift from the terminal's story.
   *
   * Removal is a two-step flow on purpose. The first DELETE is sent WITHOUT
   * yes=1, so the server refuses with exactly the paragraph this workspace's
   * removal needs read (needs_destroy_origin carries the persist path — that
   * persist is the only copy of the tracker; needs_yes carries the
   * mirror-only wording). That refusal IS the confirmation dialog's body.
   * Confirming re-sends with yes=1, plus destroy_origin=1 when the only-copy
   * checkbox is checked. The checkbox is mandatory, not advisory: the server
   * refuses a bare yes for a workspace with a persist, and mirroring that
   * rule here (button disabled until checked) is cheaper than round-tripping
   * it.
   *
   * These routes answer 403 forbidden_host under a DNS-named Host — a remote
   * tab may read nothing here. A 403 on the GET replaces the whole tab with
   * one line; a 403 on a write mid-session (list already loaded) hides the
   * management controls instead of surfacing an error banner: the boundary
   * is structural, not a failure.
   */
  import { onMount } from 'svelte'
  import { t } from '../../lib/i18n'
  import * as api from '../../lib/api'
  import type { WorkspaceInfo } from '../../lib/api'
  import LoadingState from '../ui/LoadingState.svelte'
  import DialogShell from '../ui/DialogShell.svelte'
  import { trapFocus } from '../../lib/focus-trap'
  import { ADD_BTN, COPY_BTN, INPUT } from './controls'

  let rows = $state<WorkspaceInfo[]>([])
  let loading = $state(true)
  let loadError = $state(false)
  /** GET answered 403 — this browser is remote, the whole tab is one notice. */
  let remoteBlocked = $state(false)
  /** A write answered 403 mid-session — list stays, controls fold away. */
  let manageBlocked = $state(false)

  let newName = $state('')
  let newProjects = $state('')
  let creating = $state(false)
  let createError = $state<string | null>(null)
  /** A removal that failed before a dialog could say it (network throw). */
  let removeError = $state<string | null>(null)

  /** The removal dialog. `refusal` is the probe's error code: the two
   *  confirmable ones name the commit's shape, anything else (root,
   *  unreadable, self) is a hard refusal with no confirm button at all. */
  let confirm = $state<{
    row: WorkspaceInfo
    refusal: string | null
    detail: string
    destroyOrigin: boolean
    busy: boolean
    error: string | null
  } | null>(null)

  /** Server-written notes from the last successful removal, shown verbatim. */
  let advisories = $state<string[]>([])

  function isRefusal(e: unknown, code: string): boolean {
    return e instanceof api.WorkspaceManageError && e.error === code
  }

  async function load(): Promise<void> {
    loading = true
    try {
      rows = await api.listWorkspaces()
      loadError = false
    } catch (e) {
      if (isRefusal(e, 'forbidden_host')) remoteBlocked = true
      else loadError = true
    }
    loading = false
  }

  onMount(() => {
    void load()
    // The confirm dialog lives inside the settings dialog, which also listens
    // for Escape on the window (bubble phase). Stopping the key in the
    // capture phase is what keeps one Esc closing only the confirm — bubble
    // listeners never run — while a plain Esc with no confirm open passes
    // through and closes the settings dialog as before.
    window.addEventListener('keydown', onCaptureKeydown, { capture: true })
    return () => window.removeEventListener('keydown', onCaptureKeydown, { capture: true })
  })

  function onCaptureKeydown(e: KeyboardEvent): void {
    if (confirm && e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      confirm = null
    }
  }

  async function create(): Promise<void> {
    if (creating || newName.trim() === '') return
    // Capture before the await: the error line must name what was sent, not
    // whatever the input holds by the time the answer lands.
    const name = newName.trim()
    creating = true
    createError = null
    try {
      await api.createWorkspace(name, newProjects)
      newName = ''
      newProjects = ''
      await load()
    } catch (e) {
      if (isRefusal(e, 'forbidden_host')) {
        manageBlocked = true
      } else if (isRefusal(e, 'exists')) {
        createError = t('settings.workspacesErrExists', { name })
      } else if (isRefusal(e, 'invalid_name')) {
        createError = t('settings.workspacesErrInvalidName')
      } else {
        createError = t('settings.workspacesCreateFailed')
      }
    } finally {
      creating = false
    }
  }

  /** Open the removal dialog and run the probe: DELETE without yes, so the
   *  server's refusal paragraph becomes the dialog's body. */
  async function beginRemove(row: WorkspaceInfo): Promise<void> {
    // UI precedes the server here: the serving profile's refusal is a fact,
    // not a race — show it without a round-trip.
    if (row.active) return
    removeError = null
    confirm = { row, refusal: null, detail: '', destroyOrigin: false, busy: true, error: null }
    try {
      // A server that removed without confirmation (older contract, or the
      // row was a leftover): accept the result, don't argue with it.
      const res = await api.removeWorkspace(row.name)
      confirm = null
      advisories = res.advisories ?? []
      await load()
    } catch (e) {
      if (isRefusal(e, 'not_found')) {
        // Stale row — the list knows better than the dialog.
        confirm = null
        await load()
        return
      }
      if (e instanceof api.WorkspaceManageError) {
        confirm = {
          row,
          refusal: e.error,
          detail: e.detail ?? '',
          destroyOrigin: false,
          busy: false,
          error: null,
        }
      } else {
        // A network throw — the list did not fail, so the list's error banner
        // would be a lie. Say it where the button was pressed instead.
        confirm = null
        removeError = t('settings.workspacesRemoveFailed')
      }
    }
  }

  /** The second DELETE: yes=1, plus destroy_origin=1 when the only-copy
   *  checkbox is checked. needs_destroy_origin is the one refusal that makes
   *  the checkbox appear, so the gate below is exactly the server's rule. */
  async function commitRemove(): Promise<void> {
    if (!confirm || confirm.busy) return
    confirm.busy = true
    confirm.error = null
    try {
      const res = await api.removeWorkspace(confirm.row.name, {
        yes: true,
        destroyOrigin: confirm.destroyOrigin,
      })
      confirm = null
      advisories = res.advisories ?? []
      await load()
    } catch (e) {
      if (!confirm) return
      if (isRefusal(e, 'forbidden_host')) {
        confirm = null
        manageBlocked = true
        return
      }
      if (isRefusal(e, 'not_found')) {
        confirm = null
        await load()
        return
      }
      confirm.busy = false
      confirm.error =
        e instanceof api.WorkspaceManageError && e.detail
          ? e.detail
          : t('settings.workspacesRemoveFailed')
    }
  }

  /** SidebarNav's host-shortening rule: a site URL reads as its host; a
   *  workspace with no site (standalone seeds have none) is a dash. */
  function siteHost(w: WorkspaceInfo): string {
    if (!w.site) return ''
    try {
      return new URL(w.site).host
    } catch {
      return w.site
    }
  }
</script>

<div class="flex flex-col gap-2.5" data-testid="workspaces-tab">
  {#if remoteBlocked}
    <p
      class="rounded border border-border-subtle bg-bg-elevated px-2 py-1.5 text-micro leading-relaxed text-text-secondary"
      data-testid="workspaces-remote"
    >
      {t('settings.workspacesRemote')}
    </p>
  {:else}
    <p class="text-micro leading-relaxed text-text-muted">{t('settings.workspacesIntro')}</p>

    {#if loadError}
      <div
        class="flex flex-wrap items-center gap-2 rounded border border-border-subtle bg-bg-elevated px-2 py-1.5"
        data-testid="workspaces-error"
      >
        <span class="min-w-0 flex-1 text-micro text-status-reopen">{t('settings.workspacesLoadFailed')}</span>
        <button type="button" class={COPY_BTN} disabled={loading} onclick={() => void load()}>
          {t('common.retry')}
        </button>
      </div>
    {/if}

    {#if manageBlocked}
      <p
        class="rounded border border-border-subtle bg-bg-elevated px-2 py-1.5 text-micro leading-relaxed text-text-secondary"
        data-testid="workspaces-remote"
      >
        {t('settings.workspacesRemote')}
      </p>
    {/if}

    {#if loading && rows.length === 0}
      <LoadingState label={t('settings.workspacesLoading')} />
    {:else if rows.length === 0}
      {#if !loadError}
        <p class="py-6 text-center text-text-muted">{t('settings.workspacesEmpty')}</p>
      {/if}
    {:else}
      <table class="w-full text-micro" data-testid="workspaces-list">
        <thead>
          <tr class="text-left text-text-muted">
            <th class="py-1 pr-2 font-medium">{t('settings.workspacesColName')}</th>
            <th class="py-1 pr-2 font-medium">{t('settings.workspacesColSite')}</th>
            <th class="py-1 pr-2 font-medium">{t('settings.workspacesColProjects')}</th>
            <th class="py-1 font-medium" aria-label={t('settings.workspacesRemoveTitle')}></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.name)}
            <tr class="border-t border-border-subtle align-top" data-testid="workspaces-row-{row.name}">
              <td class="py-1.5 pr-2">
                <span class="font-mono text-text-primary">{row.name}</span>
                {#if row.active}
                  <span
                    class="ml-1.5 rounded border border-border-subtle bg-bg-elevated px-1 py-px text-micro text-text-secondary"
                    data-testid="workspaces-active"
                  >
                    {t('settings.workspacesActiveBadge')}
                  </span>
                {/if}
                {#if row.error}
                  <p class="mt-0.5 text-micro leading-relaxed text-status-reopen">
                    {t('settings.workspacesUnreadable')}
                  </p>
                {/if}
              </td>
              <td class="whitespace-nowrap py-1.5 pr-2 text-text-secondary">{siteHost(row) || '—'}</td>
              <td class="py-1.5 pr-2 text-text-secondary">{row.projects?.join(', ') || '—'}</td>
              <td class="py-1.5 text-right">
                {#if !manageBlocked}
                  <!-- The serving profile's removal is refused by the server
                       (self_delete); disabling here tells the user before the
                       round-trip, and the tooltip says the next move. -->
                  <button
                    type="button"
                    class={COPY_BTN}
                    disabled={row.active}
                    title={row.active ? t('settings.workspacesActiveHint') : undefined}
                    onclick={() => void beginRemove(row)}
                    data-testid="workspaces-remove-{row.name}"
                  >
                    {t('common.delete')}
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}

    {#if removeError}
      <p class="text-micro text-status-reopen" data-testid="workspaces-remove-error">{removeError}</p>
    {/if}

    {#if advisories.length}
      <!-- The server's own post-removal notes (pairing token still held on a
           home serve, cleared stored default, serve staleness), verbatim —
           same single-owner rule as the refusal detail above. -->
      <section
        class="rounded-md border border-border-subtle bg-bg-base/60 px-3 py-2.5"
        data-testid="workspaces-advisories"
      >
        <p class="text-micro text-text-muted">{t('settings.workspacesAdvisories')}</p>
        <ul class="mt-1 flex flex-col gap-1">
          {#each advisories as line}
            <li class="whitespace-pre-wrap text-micro leading-relaxed text-text-secondary">{line}</li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if !manageBlocked}
      <form
        class="mt-1 flex flex-wrap items-end gap-2"
        onsubmit={(e) => {
          e.preventDefault()
          void create()
        }}
        data-testid="workspaces-form"
      >
        <label class="flex flex-col gap-1">
          <span class="text-micro text-text-muted">{t('settings.workspacesNameLabel')}</span>
          <input
            class={INPUT}
            bind:value={newName}
            data-testid="workspaces-name-input"
            autocomplete="off"
            spellcheck="false"
          />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-micro text-text-muted">{t('settings.workspacesProjectsLabel')}</span>
          <input
            class={INPUT}
            bind:value={newProjects}
            data-testid="workspaces-projects-input"
            autocomplete="off"
            spellcheck="false"
          />
        </label>
        <button
          type="submit"
          class={ADD_BTN}
          disabled={creating || newName.trim() === ''}
          data-testid="workspaces-create-button"
        >
          {t(creating ? 'common.creating' : 'settings.workspacesCreate')}
        </button>
      </form>

      {#if createError}
        <p class="text-micro text-status-reopen" data-testid="workspaces-create-error">{createError}</p>
      {/if}
    {/if}
  {/if}

  {#if confirm}
    <DialogShell
      title={t('settings.workspacesRemoveTitle')}
      ariaLabel={t('settings.workspacesRemoveTitle')}
      data-testid="workspaces-remove-dialog"
      onclose={() => (confirm = null)}
      trap={trapFocus}
      panelClass="max-w-lg"
    >
      <div class="flex flex-col gap-2 px-5 py-4">
        {#if confirm.busy && confirm.refusal === null}
          <p class="text-micro text-text-muted">{t('settings.workspacesLoading')}</p>
        {:else}
          <!-- The server's refusal paragraph, verbatim: it names the
               workspace, says what removal deletes, and (for a standalone
               persist) carries the absolute path. whitespace-pre-wrap because
               the wording is line-broken by its owner. -->
          <p
            class="whitespace-pre-wrap rounded border border-border-subtle bg-bg-base px-2.5 py-2 font-mono text-micro leading-relaxed text-text-secondary"
            data-testid="workspaces-refusal-detail"
          >
            {confirm.detail}
          </p>
          {#if confirm.refusal === 'needs_destroy_origin'}
            <label class="flex items-start gap-2 text-micro leading-relaxed text-text-primary">
              <input
                type="checkbox"
                class="mt-0.5"
                bind:checked={confirm.destroyOrigin}
                data-testid="workspaces-destroy-origin"
              />
              <span>{t('settings.workspacesDestroyLabel')}</span>
            </label>
            <p class="text-micro leading-relaxed text-status-reopen">
              {t('settings.workspacesDestroyHint')}
            </p>
          {/if}
          {#if confirm.error}
            <p class="whitespace-pre-wrap text-micro text-status-reopen" data-testid="workspaces-remove-error">
              {confirm.error}
            </p>
          {/if}
        {/if}
      </div>
      {#snippet footer()}
        <!-- Snippets carry their own scope, so the {#if confirm} around this
             dialog does not narrow in here — the footer guards itself. -->
        {#if confirm}
          <button
            type="button"
            class="inline-flex h-control items-center rounded-md px-3 text-body text-text-secondary transition-colors hover:bg-bg-hover"
            onclick={() => (confirm = null)}
          >
            {t('common.cancel')}
          </button>
          <!-- Confirm exists only for the two refusals that mean "asked, not
               committed". A needs_destroy_origin commit stays disabled until
               the only-copy checkbox decides — the server would refuse a bare
               yes anyway, and the dialog saying so is cheaper than the
               round-trip. -->
          {#if confirm.refusal === 'needs_destroy_origin' || confirm.refusal === 'needs_yes'}
            <button
              type="button"
              class="inline-flex h-control items-center rounded-md px-3 text-body text-status-reopen transition-colors hover:bg-status-reopen/10 disabled:opacity-50"
              disabled={confirm.busy || (confirm.refusal === 'needs_destroy_origin' && !confirm.destroyOrigin)}
              onclick={() => void commitRemove()}
              data-testid="workspaces-remove-confirm"
            >
              {t('common.delete')}
            </button>
          {/if}
        {/if}
      {/snippet}
    </DialogShell>
  {/if}
</div>
