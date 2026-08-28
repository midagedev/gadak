/*
 * GDK-1065: the detail changelog timeline must label fields through the
 * catalog owner (fieldLabel + field.*), not a third local mapping that
 * covered only status/assignee/priority and left machine ids
 * ("issueparentassociation") raw.
 *
 * No component-mount harness: vitest is environment:'node' and the unit
 * project loads no svelte plugin, so importing .svelte fails outright
 * (EpicProgress.test.ts / PersonalFeed.test.ts document the same limit).
 * What this file proves: the catalog owner resolves the ids the timeline
 * ships (including the changelog's plain "assignee", which field.* lacked
 * until GDK-1065), and the HistoryTimeline source routes e.field through
 * that owner — no local mapping function, no common.* re-lookup.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { fieldLabel } from '../../lib/i18n'
import { en, ja, ko } from '../../lib/i18n/catalog'

const HERE = dirname(fileURLToPath(import.meta.url))
const TIMELINE = join(HERE, 'HistoryTimeline.svelte')

describe('GDK-1065 history timeline field labels go through the catalog owner', () => {
  test('fieldLabel resolves the ids the changelog ships', () => {
    // Node has no localStorage and navigator.language is en*, so the
    // active locale here is en.
    expect(fieldLabel('status')).toBe('Status')
    expect(fieldLabel('assignee')).toBe('Assignee')
    expect(fieldLabel('priority')).toBe('Priority')
    // Machine ids the old local mapping left raw (GDK-1055 catalog keys).
    expect(fieldLabel('issueparentassociation')).toBe('Parent')
    expect(fieldLabel('resolution')).toBe('Resolution')
    // A truly unknown id degrades visibly to the raw id, never blank.
    expect(fieldLabel('customfield_12345')).toBe('customfield_12345')
  })

  test('field.assignee is present in every locale', () => {
    // The changelog's assignee axis is plain "assignee"; field.* had only
    // assignee_email, so the timeline needed its own mapping. Wording is
    // common.assignee's (Status/Priority already matched field.* verbatim).
    expect(en['field.assignee']).toBe('Assignee')
    expect(ko['field.assignee']).toBe('담당자')
    expect(ja['field.assignee']).toBe('担当者')
  })

  test('HistoryTimeline routes e.field through the shared fieldLabel, never a local mapping', () => {
    const src = readFileSync(TIMELINE, 'utf8')
    expect(src).toMatch(/import \{ t, fieldLabel \} from '\.\.\/\.\.\/lib\/i18n'/)
    expect(src).toContain('fieldLabel(e.field)')
    // The pre-GDK-1065 local mapping must stay gone.
    expect(src).not.toMatch(/function fieldLabel/)
    expect(src).not.toContain("t('common.assignee')")
    expect(src).not.toContain("t('common.status')")
    expect(src).not.toContain("t('common.priority')")
  })
})
