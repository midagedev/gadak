/*
 * GDK-254: create-dialog warning set. Keys on field_id, never on a
 * localized name (한국어 계정에서 "Task"는 "작업").
 */
import { describe, expect, test } from 'vitest'
import {
  CREATE_DIALOG_ALWAYS_SENT,
  extraRequiredCreateFields,
  isCreateFieldRequired,
  type CreateFieldMeta,
} from './create-fields'

const SAMPLE: CreateFieldMeta[] = [
  { field_id: 'issuetype', name: '작업', required: true, has_default: false, type: 'issuetype' },
  { field_id: 'project', name: 'Project', required: true, has_default: false, type: 'project' },
  { field_id: 'reporter', name: 'Reporter', required: true, has_default: true, type: 'user' },
  { field_id: 'summary', name: '요약', required: true, has_default: false, type: 'string' },
  { field_id: 'customfield_10050', name: 'Sprint', required: true, has_default: false, type: 'array' },
  { field_id: 'duedate', name: 'Due date', required: false, has_default: false, type: 'date' },
  { field_id: 'labels', name: 'Labels', required: false, has_default: false, type: 'array' },
]

describe('extraRequiredCreateFields (GDK-254)', () => {
  test('drops has_default reporter and always-sent project/issuetype/summary', () => {
    const extra = extraRequiredCreateFields(SAMPLE)
    expect(extra.map((f) => f.field_id)).toEqual(['customfield_10050'])
  })

  test('a required field the dialog does not send is in the warning', () => {
    const fields: CreateFieldMeta[] = [
      ...SAMPLE,
      {
        field_id: 'components',
        name: 'Components',
        required: true,
        has_default: false,
        type: 'array',
      },
    ]
    expect(extraRequiredCreateFields(fields).map((f) => f.field_id).sort()).toEqual([
      'components',
      'customfield_10050',
    ])
  })

  test('keys on field_id, not the localized name', () => {
    const fields: CreateFieldMeta[] = [
      { field_id: 'issuetype', name: 'Task', required: true, has_default: false, type: 'issuetype' },
      { field_id: 'customfield_1', name: 'Task', required: true, has_default: false, type: 'string' },
    ]
    expect(extraRequiredCreateFields(fields).map((f) => f.field_id)).toEqual(['customfield_1'])
  })

  test('a filled known field drops out of the warning', () => {
    const fields: CreateFieldMeta[] = [
      { field_id: 'summary', name: 'Summary', required: true, has_default: false, type: 'string' },
      { field_id: 'duedate', name: 'Due date', required: true, has_default: false, type: 'date' },
    ]
    const sent = new Set<string>([...CREATE_DIALOG_ALWAYS_SENT, 'duedate'])
    expect(extraRequiredCreateFields(fields, sent)).toEqual([])
  })

  test('optional fields never warn', () => {
    expect(
      extraRequiredCreateFields(SAMPLE.filter((f) => !f.required)),
    ).toEqual([])
  })
})

describe('isCreateFieldRequired (GDK-254)', () => {
  test('marks known fields that are required, not reporter-on-assignee', () => {
    expect(isCreateFieldRequired(SAMPLE, 'summary')).toBe(true)
    expect(isCreateFieldRequired(SAMPLE, 'project')).toBe(true)
    expect(isCreateFieldRequired(SAMPLE, 'issuetype')).toBe(true)
    expect(isCreateFieldRequired(SAMPLE, 'duedate')).toBe(false)
    expect(isCreateFieldRequired(SAMPLE, 'assignee')).toBe(false)
    expect(isCreateFieldRequired(SAMPLE, 'reporter')).toBe(true)
  })
})
