<script lang="ts">
  /*
   * Product name, build version, and the four feedback channels (GDK-336).
   * Version is GET settings/ runtime.gadakVersion — no extra endpoint.
   */
  import { t } from '../../lib/i18n'
  import type { SettingsRuntime } from '../../lib/api'

  let { runtime = null }: { runtime?: SettingsRuntime | null } = $props()

  const GITHUB = 'https://github.com/midagedev/gadak'
  const ISSUES = 'https://github.com/midagedev/gadak/issues'
  const EMAIL = 'mailto:midagedev@gmail.com'
  const X = 'https://x.com/midagedev'

  const LINK =
    'inline-block text-micro text-accent-text hover:underline'
</script>

<div class="flex flex-col gap-5" data-testid="settings-about">
  <div class="flex flex-col gap-1">
    <div class="text-[12px] text-text-primary">gadak</div>
    {#if runtime?.gadakVersion}
      <div class="font-mono text-micro text-text-muted">{runtime.gadakVersion}</div>
    {/if}
  </div>

  <div class="flex flex-col gap-1.5">
    <div class="text-micro font-medium uppercase tracking-wide text-text-muted">
      {t('settings.aboutFeedback')}
    </div>
    <div class="flex flex-col gap-1">
      <a
        class={LINK}
        href={GITHUB}
        target="_blank"
        rel="noopener"
        data-testid="about-link-github"
      >
        {t('settings.aboutGithub')}
      </a>
      <a
        class={LINK}
        href={ISSUES}
        target="_blank"
        rel="noopener"
        data-testid="about-link-issues"
      >
        {t('settings.aboutIssues')}
      </a>
      <a class={LINK} href={EMAIL} data-testid="about-link-email">
        {t('settings.aboutEmail')}
      </a>
      <a class={LINK} href={X} target="_blank" rel="noopener" data-testid="about-link-x">
        {t('settings.aboutX')}
      </a>
    </div>
  </div>
</div>
