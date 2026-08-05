<script lang="ts">
  import { t, fieldLabel } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { feature } from '../../lib/config'
  import QaFieldEditor from './QaFieldEditor.svelte'
  import Section from './Section.svelte'

  let {
    issue,
    developmentOpinion,
  }: {
    issue: IssueLite
    developmentOpinion: string
  } = $props()

  type EditKind = 'option' | 'user' | 'version_array'

  interface FieldRow {
    key: string
    label: string
    values: string[]
    /** When set, supports inline edit (QaFieldEditor). Else read-only. */
    edit?: EditKind
  }

  function split(value: string | null | undefined): string[] {
    return (value ?? '')
      .split(',')
      .map((part) => part.trim())
      .filter(Boolean)
  }

  const rows = $derived.by<FieldRow[]>(() => [
    {
      key: 'development_test_assignee',
      label: fieldLabel('development_test_assignee_email'),
      values: split(issue.development_test_assignee || issue.development_test_assignee_email),
      edit: 'user',
    },
    {
      key: 'development_test_result',
      label: fieldLabel('development_test_result'),
      values: split(issue.development_test_result),
      edit: 'option',
    },
    { key: 'environment', label: fieldLabel('environment'), values: split(issue.environment) },
    { key: 'browser', label: fieldLabel('browser'), values: split(issue.browser) },
    { key: 'dev_project_number', label: fieldLabel('dev_project_number'), values: split(issue.dev_project_number) },
    { key: 'found_version', label: fieldLabel('found_version'), values: split(issue.found_version) },
    { key: 'occurrence', label: fieldLabel('occurrence'), values: split(issue.occurrence) },
    { key: 'components', label: fieldLabel('components'), values: issue.components ?? [] },
    { key: 'solution', label: t('field.solution_method'), values: split(issue.solution), edit: 'option' },
    {
      key: 'fix_versions',
      label: fieldLabel('fix_versions'),
      values: issue.fix_versions ?? [],
      edit: 'version_array',
    },
    { key: 'critical_phenomenon', label: fieldLabel('critical_phenomenon'), values: split(issue.critical_phenomenon) },
    { key: 'development_area', label: fieldLabel('development_area'), values: split(issue.development_area) },
    { key: 'development_opinion', label: t('field.development_opinion'), values: split(developmentOpinion) },
    { key: 'cs', label: 'CS', values: split(issue.cs) },
  ])

  // Boards differ in which fields they actually use (a team board may fill 2,
  // a QA board 30+), so an empty non-editable field is noise, not information.
  // Editable rows stay even when empty — that is how a value gets set.
  const visibleRows = $derived(
    rows.filter((row) => row.values.length > 0 || (row.edit && feature('qa'))),
  )

  function resultClass(value: string): string {
    const normalized = value.toLowerCase()
    if (normalized === 'pass') return 'bg-status-done/15 text-status-done'
    if (normalized === 'fail') return 'bg-status-reopen/15 text-status-reopen'
    return 'bg-bg-elevated text-text-secondary'
  }
</script>

{#if visibleRows.length > 0}
  <Section title={t('detail.details')}>
    <dl class="grid grid-cols-[116px_minmax(0,1fr)] gap-x-3 gap-y-2 text-[12px]">
      {#each visibleRows as row (row.key)}
        <dt class="pt-0.5 text-text-muted">{row.label}</dt>
        <dd class="min-w-0">
          {#if row.edit && feature('qa')}
            <QaFieldEditor {issue} field={row.key} kind={row.edit} values={row.values} />
          {:else}
            <span class="flex flex-wrap gap-1">
              {#each row.values as value (value)}
                <span
                  class="max-w-full break-words rounded px-1.5 py-0.5 {row.key ===
                  'development_test_result'
                    ? resultClass(value)
                    : 'bg-bg-elevated text-text-secondary'}"
                >{value}</span>
              {/each}
            </span>
          {/if}
        </dd>
      {/each}
    </dl>
  </Section>
{/if}
