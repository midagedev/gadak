<script lang="ts">
  /*
   * Thin wrapper. Kind branching lives in FieldEditor — this file stays so
   * existing QA-field imports keep resolving.
   */
  import type { IssueLite } from '../../lib/types'
  import FieldEditor, { type EditorKind } from './FieldEditor.svelte'

  let {
    issue,
    field,
    kind,
    values,
  }: {
    issue: IssueLite
    field: string
    kind: EditorKind
    values: string[]
  } = $props()
</script>

<FieldEditor {issue} {field} {kind} {values} />
