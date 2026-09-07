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
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'
import { fieldLabel } from '../../lib/i18n'
import { en, ja, ko } from '../../lib/i18n/catalog'

const HERE = dirname(fileURLToPath(import.meta.url))
const TIMELINE = join(HERE, 'HistoryTimeline.svelte')

/*
 * GDK-1474: the import assertion below used to be a regex over one spelling
 * of the import line, so adding a second name to it — which the sibling
 * PersonalFeed.test.ts had already been through once — would fail a test
 * about something else entirely. Read from the AST instead; the contract is
 * "fieldLabel is bound from the i18n owner", not the order of a list.
 */
type AnyNode = { type: string } & Record<string, unknown>

/** Named bindings this component imports from `specifier`. */
function namedImports(path: string, filename: string, specifier: string): string[] {
  const ast = parse(readFileSync(path, 'utf8'), { modern: true, filename }) as unknown as {
    instance: { content: { body: AnyNode[] } } | null
    module: { content: { body: AnyNode[] } } | null
  }
  const names: string[] = []
  for (const part of [ast.instance, ast.module]) {
    for (const node of part?.content.body ?? []) {
      if (node.type !== 'ImportDeclaration') continue
      const source = node.source as { value?: unknown } | undefined
      if (source?.value !== specifier) continue
      for (const spec of (node.specifiers ?? []) as AnyNode[]) {
        if (spec.type !== 'ImportSpecifier') continue
        const imported = spec.imported as { name?: unknown } | undefined
        if (typeof imported?.name === 'string') names.push(imported.name)
      }
    }
  }
  return names
}

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
    expect(namedImports(TIMELINE, 'HistoryTimeline.svelte', '../../lib/i18n')).toContain(
      'fieldLabel',
    )
    expect(src).toContain('fieldLabel(e.field)')
    // The pre-GDK-1065 local mapping must stay gone.
    expect(src).not.toMatch(/function fieldLabel/)
    expect(src).not.toContain("t('common.assignee')")
    expect(src).not.toContain("t('common.status')")
    expect(src).not.toContain("t('common.priority')")
  })
})
