<script lang="ts">
  /*
   * First-run state. Shown in place of the list only while the mirror is empty
   * AND setup is incomplete (no credential or no projects) — once a single issue
   * has synced this never renders again, so a normal empty filter result keeps
   * using EmptyState.
   */
  import { t } from '../../lib/i18n'
  import { config } from '../../lib/config'
  import { write } from '../../stores/write.svelte'
  import { me } from '../../stores/me.svelte'

  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  // identity === 저장된 Jira 자격증명(서버 계약). 별도 플래그를 볼 이유가 없다.
  const hasCredential = $derived(me.identified)
  const hasProjects = $derived(config().projects.length > 0)

  const steps = $derived([
    {
      id: 'cred',
      done: hasCredential,
      label: t('onboarding.stepCredential'),
      hint: t('onboarding.stepCredentialHint'),
      action: () => write.openSettings(),
      actionLabel: t('common.setCredentials'),
    },
    {
      id: 'projects',
      done: hasProjects,
      label: t('onboarding.stepProjects'),
      hint: t('onboarding.stepProjectsHint'),
      action: onOpenSettings,
      actionLabel: t('onboarding.chooseProjects'),
    },
    {
      id: 'sync',
      done: false,
      label: t('onboarding.stepSync'),
      hint: t('onboarding.stepSyncHint'),
      action: null,
      actionLabel: '',
    },
  ])
</script>

<div
  class="flex h-full items-start justify-center overflow-y-auto px-6 py-12"
  data-testid="onboarding"
>
  <div class="anim-enter w-full max-w-md">
    <h2 class="text-[15px] font-semibold text-text-primary">{t('onboarding.title')}</h2>
    <p class="mt-1.5 text-[12px] text-text-secondary">
      {t('onboarding.introBefore')}
      <code class="rounded bg-bg-elevated px-1 py-0.5 font-mono text-text-primary">scry init</code>
      {t('onboarding.introAfter')}
    </p>

    <ol class="mt-5 flex flex-col gap-1">
      {#each steps as step, i (step.id)}
        <li
          class="flex items-start gap-3 rounded-md border border-border-subtle bg-bg-panel/50 px-3 py-2.5"
        >
          <span
            class="mt-px flex h-5 w-5 flex-none items-center justify-center rounded-full border text-[11px] font-medium {step.done
              ? 'border-status-done/50 bg-status-done/15 text-status-done'
              : 'border-border-strong text-text-muted'}"
            aria-hidden="true"
          >
            {step.done ? '✓' : i + 1}
          </span>
          <span class="min-w-0 flex-1">
            <span class="flex items-center gap-2">
              <span class="text-[13px] font-medium text-text-primary">{step.label}</span>
              <span
                class="flex-none text-[10px] uppercase tracking-wide {step.done
                  ? 'text-status-done'
                  : 'text-text-muted'}"
              >
                {step.done ? t('onboarding.done') : t('onboarding.todo')}
              </span>
            </span>
            <span class="mt-0.5 block text-[12px] text-text-muted">{step.hint}</span>
          </span>
          {#if step.action && !step.done}
            <button
              type="button"
              class="mt-px flex-none rounded-md border border-border-strong px-2.5 py-1 text-[11px] font-medium text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
              onclick={step.action}
            >
              {step.actionLabel}
            </button>
          {/if}
        </li>
      {/each}
    </ol>

    <button
      type="button"
      class="mt-5 rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white transition-colors hover:bg-accent-hover"
      onclick={onOpenSettings}
    >
      {t('onboarding.openSettings')}
    </button>
  </div>
</div>
