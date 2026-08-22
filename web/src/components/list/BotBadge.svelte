<script lang="ts">
  /*
   * Bot badge (GDK-590). One component, every author surface: comment headers,
   * history entries, dev-link "linked by" lines. The judgement is the
   * server's — member.is_bot from the account catalog, or the account_type a
   * payload carries inline — never a display-name guess here.
   */
  import { t } from '../../lib/i18n'
  import { issues } from '../../stores/issues.svelte'

  let {
    accountId = null,
    accountType = null,
  }: {
    /** Author's Jira accountId — resolves the member (is_bot). */
    accountId?: string | null
    /** Origin account type ('agent' | 'app') when the payload carries it. */
    accountType?: string | null
  } = $props()

  const member = $derived(accountId ? issues.memberOfAccountId(accountId) : undefined)
  const isBot = $derived(
    member?.is_bot ?? (accountType === 'agent' || accountType === 'app'),
  )
</script>

{#if isBot}
  <span
    data-testid="bot-badge"
    class="rounded bg-bg-elevated px-1 py-px text-micro font-medium text-text-muted"
    title={member?.account_type ?? accountType ?? undefined}
  >
    {t('common.bot')}
  </span>
{/if}
