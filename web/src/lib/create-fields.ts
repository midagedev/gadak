/*
 * Create-time field classification (GDK-254).
 *
 * Keys on field_id, never on a localized name (한국어 계정에서 "Task"는
 * "작업"). The dialog always sends project / issuetype / summary; Jira
 * fills has_default required fields (reporter). Everything else that is
 * required with no default is a warning, not a submit block.
 */

import type { CreateFieldMeta } from './types'

export type { CreateFieldMeta, CreateFieldsResponse } from './types'

/** Field ids POST create/ always includes (handleCreate in write.go). */
export const CREATE_DIALOG_ALWAYS_SENT = ['project', 'issuetype', 'summary'] as const

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
