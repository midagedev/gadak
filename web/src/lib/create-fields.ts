/*
 * Create-time field classification (GDK-254).
 *
 * Keys on field_id, never on a localized name (한국어 계정에서 "Task"는
 * "작업"). The dialog always sends project / issuetype / summary; Jira
 * fills has_default required fields (reporter). Everything else that is
 * required with no default splits in two (GDK-533): the kinds this dialog
 * has editors for get editors, the rest block the submit instead of
 * warning over a create the origin will reject.
 */

import type { CreateFieldMeta } from './types'

export type { CreateFieldMeta, CreateFieldsResponse } from './types'

/** Field ids POST create/ always includes (handleCreate in write.go). */
export const CREATE_DIALOG_ALWAYS_SENT = ['project', 'issuetype', 'summary'] as const

/** Field ids this dialog has its own editor for — duedate never gets a
 * second editor as a "custom field", and its empty state keeps today's
 * marker-only behavior (GDK-533 scope is the extras the warning named). */
export const CREATE_DIALOG_EDITED_IDS = [
  'project',
  'issuetype',
  'summary',
  'description',
  'assignee',
  'priority',
  'labels',
  'duedate',
] as const

export function extraRequiredCreateFields(
  fields: readonly CreateFieldMeta[],
  sentFieldIds: Iterable<string> = CREATE_DIALOG_ALWAYS_SENT,
): CreateFieldMeta[] {
  const sent = new Set(sentFieldIds)
  return fields.filter(
    (f) => f.required && !f.has_default && f.field_id !== '' && !sent.has(f.field_id),
  )
}

export function isCreateFieldRequired(
  fields: readonly CreateFieldMeta[],
  fieldId: string,
): boolean {
  return fields.some((f) => f.field_id === fieldId && f.required)
}

/**
 * The editor kind this dialog can render for a create-time field, or null
 * when it cannot: scalar kinds reuse the dialog's own input idioms (select
 * for option, text input, date input), everything else — user pickers,
 * multi-checkbox sets, unknown kinds, option fields with no options — is
 * honestly "cannot fill here" rather than a wrong editor.
 */
export function createFieldEditorKind(
  f: CreateFieldMeta,
): 'option' | 'text' | 'number' | 'date' | null {
  if (f.kind === 'option') return (f.options?.length ?? 0) > 0 ? 'option' : null
  if (f.kind === 'text' || f.kind === 'number' || f.kind === 'date') return f.kind
  return null
}

/** Extra-required fields the dialog renders editors for (GDK-533). */
export function fillableCreateFields(
  fields: readonly CreateFieldMeta[],
  sentFieldIds: Iterable<string> = CREATE_DIALOG_ALWAYS_SENT,
): CreateFieldMeta[] {
  const edited = new Set<string>(CREATE_DIALOG_EDITED_IDS)
  return extraRequiredCreateFields(fields, sentFieldIds).filter(
    (f) => !edited.has(f.field_id) && createFieldEditorKind(f) !== null,
  )
}

/** Extra-required fields no editor exists for — these block the submit. */
export function unfillableCreateFields(
  fields: readonly CreateFieldMeta[],
  sentFieldIds: Iterable<string> = CREATE_DIALOG_ALWAYS_SENT,
): CreateFieldMeta[] {
  const edited = new Set<string>(CREATE_DIALOG_EDITED_IDS)
  return extraRequiredCreateFields(fields, sentFieldIds).filter(
    (f) => !edited.has(f.field_id) && createFieldEditorKind(f) === null,
  )
}
