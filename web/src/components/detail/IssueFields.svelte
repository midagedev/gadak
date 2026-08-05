<script lang="ts">
  /*
   * Detail field rows, driven by the discovered field specs (issues.fieldSpecs).
   * Boards differ in which fields they actually use — a team board may fill 2,
   * a QA board 30+ — so rows render only when this issue carries a value;
   * editable rows stay even when empty (that is how a value gets set). Body-role
   * specs are not rows: DetailPanel renders them as document sections.
   */
  import { t, fieldLabel } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { feature } from '../../lib/config'
  import { issues } from '../../stores/issues.svelte'
  import QaFieldEditor from './QaFieldEditor.svelte'
  import Section from './Section.svelte'

  let {
    issue,
    developmentOpinion,
  }: {
    issue: IssueLite
    developmentOpinion: string
  } = $props()

  type EditKind = 'option' | 'user' | 'version_array' | 'multi_option'
  const EDITOR_KINDS: readonly string[] = ['option', 'user', 'version_array', 'multi_option']

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

  /**
   * Jira custom values arrive in their raw API shapes: option `{value}`,
   * version/component `{name}`, user `{displayName}`, arrays of any of those,
   * plain strings, numbers. Normalize everything to display strings.
   */
  function displayValues(v: unknown): string[] {
    if (v == null) return []
    if (Array.isArray(v)) return v.flatMap(displayValues)
    if (typeof v === 'string') return v.trim() ? [v.trim()] : []
    if (typeof v === 'number') return [String(v)]
    if (typeof v === 'boolean') return [v ? 'true' : 'false']
    if (typeof v === 'object') {
      const o = v as Record<string, unknown>
      for (const k of ['value', 'name', 'displayName']) {
        if (typeof o[k] === 'string' && (o[k] as string).trim()) return [o[k] as string]
      }
      return []
    }
    return []
  }

  /** Label: i18n override for well-known aliases, else the Jira display name. */
  function specLabel(alias: string, label: string): string {
    const viaI18n = fieldLabel(alias)
    return viaI18n !== alias ? viaI18n : label || alias
  }

  const rows = $derived.by<FieldRow[]>(() => {
    const out: FieldRow[] = [
      // System fields every Jira issue can carry.
      { key: 'components', label: fieldLabel('components'), values: issue.components ?? [] },
      {
        key: 'fix_versions',
        label: fieldLabel('fix_versions'),
        values: issue.fix_versions ?? [],
        edit: 'version_array',
      },
    ]
    const record = issue as unknown as Record<string, unknown>
    for (const spec of issues.fieldSpecs) {
      if (spec.role === 'body') continue // rendered as a document section
      const editable =
        spec.kind && EDITOR_KINDS.includes(spec.kind) ? (spec.kind as EditKind) : undefined
      out.push({
        key: spec.alias,
        label: specLabel(spec.alias, spec.label),
        values: displayValues(record[spec.alias]),
        edit: editable,
      })
    }
    // Legacy plugin surface (docs/PLUGINS.md): development opinion from enrichments.
    if (developmentOpinion) {
      out.push({
        key: 'development_opinion',
        label: t('field.development_opinion'),
        values: split(developmentOpinion),
      })
    }
    return out
  })

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
