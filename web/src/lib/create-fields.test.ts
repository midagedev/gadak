/*
 * GDK-254: create-dialog warning set. Keys on field_id, never on a
 * localized name (한국어 계정에서 "Task"는 "작업").
 * GDK-533: fillability split — which extra-required fields the dialog can
 * render an editor for, and which must block the submit instead of warning.
 */
import { describe, expect, test } from 'vitest'
import {
  CREATE_DIALOG_ALWAYS_SENT,
  CREATE_DIALOG_EDITED_IDS,
  extraRequiredCreateFields,
  fillableCreateFields,
  isCreateFieldRequired,
  unfillableCreateFields,
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

// Server sends kind/options since GDK-533; kind is absent on an older server
// or when the schema is one the kind interpreter does not map.
const EXTRAS: CreateFieldMeta[] = [
  {
    field_id: 'customfield_10092', name: 'Solution', required: true, has_default: false,
    type: 'option', kind: 'option', options: [{ id: '10160', value: 'Fixed' }],
  },
  { field_id: 'customfield_10030', name: 'Customer', required: true, has_default: false, type: 'string', kind: 'text' },
  { field_id: 'customfield_10040', name: 'Renewal', required: true, has_default: false, type: 'date', kind: 'date' },
  { field_id: 'customfield_10035', name: 'Score', required: true, has_default: false, type: 'number', kind: 'number' },
  {
    field_id: 'customfield_10050', name: 'Sprint', required: true, has_default: false,
    type: 'array', kind: 'multi_option', options: [{ id: '1', value: 'S1' }],
  },
  { field_id: 'customfield_10060', name: 'Mystery', required: true, has_default: false, type: 'string' },
  { field_id: 'customfield_10070', name: 'Team', required: true, has_default: false, type: 'option', kind: 'option' },
  { field_id: 'duedate', name: 'Due date', required: true, has_default: false, type: 'date', kind: 'date' },
]

describe('fillableCreateFields (GDK-533)', () => {
  test('option-with-options, text, number, date get editors', () => {
    expect(fillableCreateFields(EXTRAS).map((f) => f.field_id)).toEqual([
      'customfield_10092',
      'customfield_10030',
      'customfield_10040',
      'customfield_10035',
    ])
  })

  test('multi kinds, no kind, and option-without-options are not fillable', () => {
    expect(unfillableCreateFields(EXTRAS).map((f) => f.field_id)).toEqual([
      'customfield_10050',
      'customfield_10060',
      'customfield_10070',
    ])
  })

  test('fields the dialog already edits (duedate) are never duplicated as custom editors', () => {
    const ids = new Set<string>(CREATE_DIALOG_EDITED_IDS)
    for (const f of [...fillableCreateFields(EXTRAS), ...unfillableCreateFields(EXTRAS)]) {
      expect(ids.has(f.field_id)).toBe(false)
    }
  })

  test('a filled custom field drops out of both lists', () => {
    const sent = new Set<string>([...CREATE_DIALOG_ALWAYS_SENT])
    // Filling happens through custom_values, which sent ids model here.
    sent.add('customfield_10030')
    expect(fillableCreateFields(EXTRAS, sent).map((f) => f.field_id)).not.toContain('customfield_10030')
  })

  test('split is exhaustive: fillable + unfillable = extra required minus dialog-edited', () => {
    const extra = extraRequiredCreateFields(EXTRAS)
      .filter((f) => !new Set<string>(CREATE_DIALOG_EDITED_IDS).has(f.field_id))
      .map((f) => f.field_id)
    const split = [...fillableCreateFields(EXTRAS), ...unfillableCreateFields(EXTRAS)]
      .map((f) => f.field_id)
      .sort()
    expect(split).toEqual([...extra].sort())
  })
})
