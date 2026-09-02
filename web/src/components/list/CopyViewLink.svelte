<script lang="ts">
  /*
   * The toolbar's "Copy link" — the view's counterpart of the issue
   * detail's copy link (GDK-1343). Always present: a built-in view, a saved
   * view and a Jira filter view have no chips, and they are exactly the
   * views people send to each other.
   */
  import { t } from '../../lib/i18n'
  import { copyViewLink } from './copy-view-link'
  import Icon from '../ui/Icon.svelte'
</script>

<button
  type="button"
  class="inline-flex h-control w-control flex-none items-center justify-center rounded-md border border-border-strong/70 bg-bg-elevated text-text-secondary hover:border-border-strong hover:text-text-primary"
  onclick={() => void copyViewLink()}
  title={t('view.copyLink')}
  aria-label={t('view.copyLink')}
  data-testid="view-copy-link"
>
  <Icon name="link" size={14} />
</button>
