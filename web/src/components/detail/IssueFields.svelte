<script lang="ts">
  import type { IssueLite } from '../../lib/types'
  import QaFieldEditor from './QaFieldEditor.svelte'

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
    /** 설정 시 인라인 편집 지원(QaFieldEditor). 없으면 읽기 전용. */
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
      label: '개발 테스트 담당자',
      values: split(issue.development_test_assignee || issue.development_test_assignee_email),
      edit: 'user',
    },
    {
      key: 'development_test_result',
      label: '개발 테스트 결과',
      values: split(issue.development_test_result),
      edit: 'option',
    },
    { key: 'environment', label: '발생 환경', values: split(issue.environment) },
    { key: 'browser', label: '브라우저', values: split(issue.browser) },
    { key: 'dev_project_number', label: '발생 프로젝트 번호', values: split(issue.dev_project_number) },
    { key: 'found_version', label: '발생 버전', values: split(issue.found_version) },
    { key: 'occurrence', label: '발생 빈도', values: split(issue.occurrence) },
    { key: 'components', label: '컴포넌트', values: issue.components ?? [] },
    { key: 'solution', label: '수정 방법', values: split(issue.solution), edit: 'option' },
    {
      key: 'fix_versions',
      label: '수정 버전',
      values: issue.fix_versions ?? [],
      edit: 'version_array',
    },
    { key: 'critical_phenomenon', label: '크리티컬 현상', values: split(issue.critical_phenomenon) },
    { key: 'development_area', label: '개발 영역', values: split(issue.development_area) },
    { key: 'development_opinion', label: '개발 의견', values: split(developmentOpinion) },
    { key: 'cs', label: 'CS', values: split(issue.cs) },
  ])

  function resultClass(value: string): string {
    const normalized = value.toLowerCase()
    if (normalized === 'pass') return 'bg-status-done/15 text-status-done'
    if (normalized === 'fail') return 'bg-status-reopen/15 text-status-reopen'
    return 'bg-bg-elevated text-text-secondary'
  }
</script>

<dl class="grid grid-cols-[116px_minmax(0,1fr)] gap-x-3 gap-y-2 text-[12px]">
  {#each rows as row (row.key)}
    <dt class="pt-0.5 text-text-muted">{row.label}</dt>
    <dd class="min-w-0">
      {#if row.edit}
        <QaFieldEditor {issue} field={row.key} kind={row.edit} values={row.values} />
      {:else if row.values.length === 0}
        <span class="text-text-muted">없음</span>
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
