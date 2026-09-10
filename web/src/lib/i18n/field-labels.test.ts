/*
 * GDK-921. `fieldLabel(field)` builds `field.${field}` at runtime, so nothing
 * ties the catalog's field.* keys to the ids the app actually asks for: a
 * typo, a renamed axis, or a key deleted from messages/fields.ts all fail the
 * same silent way — the raw id ('qa_impact', 'status_category') is rendered
 * where a name belongs, and every gate stays green. catalog.test.ts cannot
 * see it either: field.* is on its DYNAMIC_KEY_PREFIXES allowlist, on
 * purpose, because ids also arrive from mirrored data.
 *
 * The list this file replaces was FIELD_KEYS — 29 hand-kept names that no
 * code read (fieldLabel takes a plain string) and that had already drifted
 * from the catalog it claimed to describe: it never learned assignee, actor,
 * sprint, sprint_ids, sprint_state, parent, resolution or due. A second list
 * of names is not the gate; asking the two real sources is.
 *
 * Two directions are checkable without inventing a third list:
 *   1. every id spelled as a literal `fieldLabel('x')` in the tree resolves;
 *   2. every filter axis (MULTI_FIELDS) and grouping axis (GROUP_BY_VALUES)
 *      that carries a field's name resolves.
 * The reverse ("every field.* key is used") is deliberately NOT asserted:
 * ids like critical_phenomenon or dev_project_number come from a mirrored
 * Jira site's custom fields, never from a literal in this tree.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { en } from './catalog'
import { fieldLabel } from './index'
import { GROUP_BY_VALUES, MULTI_FIELDS } from '../view-config'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_SRC = join(HERE, '../..')
const E2E_ROOT = join(HERE, '../../../../e2e')

function walkSourceFiles(dir: string, acc: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name === 'dist') continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      walkSourceFiles(p, acc)
      continue
    }
    if (name.endsWith('.ts') || name.endsWith('.svelte') || name.endsWith('.js')) acc.push(p)
  }
  return acc
}

/** `fieldLabel('x')` call sites, as file → id. */
function literalFieldLabelCalls(): { file: string; id: string }[] {
  const out: { file: string; id: string }[] = []
  const SELF = fileURLToPath(import.meta.url)
  for (const file of [...walkSourceFiles(WEB_SRC), ...walkSourceFiles(E2E_ROOT)]) {
    // This file spells the call shape in its own prose and regex.
    if (file === SELF) continue
    const src = readFileSync(file, 'utf8')
    for (const m of src.matchAll(/fieldLabel\(\s*'([A-Za-z0-9_]+)'\s*\)/g)) {
      out.push({ file, id: m[1] })
    }
  }
  return out
}

/*
 * Axes whose value is not a field name, so no field.* key is expected:
 * 'none' / 'product' / 'epic' are grouping axes with labels of their own
 * (group.*), and 'keys' is a filter axis that is a key list, not a field.
 * The last assertion keeps this list from becoming a place to hide a real
 * miss: an entry that DOES have a field.* key must leave.
 */
const NOT_A_FIELD = ['none', 'product', 'epic', 'keys'] as const

describe('fieldLabel resolves every id the app spells (GDK-921)', () => {
  test("literal fieldLabel('x') call sites all have a catalog key", () => {
    const calls = literalFieldLabelCalls()
    expect(calls.length, 'the call sites should still be parsed').toBeGreaterThan(10)
    const missing = calls
      .filter(({ id }) => !(`field.${id}` in en))
      // A test that asserts the fallback is the one place a raw id is legal.
      .filter(({ id }) => id !== 'customfield_12345')
      .map(({ file, id }) => `${file}: fieldLabel('${id}') has no field.${id} in the catalog`)
    expect([...new Set(missing)], missing.join('\n')).toEqual([])
  })

  test('every filter and grouping axis that names a field has a catalog key', () => {
    const skip = new Set<string>(NOT_A_FIELD)
    const axes = [...MULTI_FIELDS, ...GROUP_BY_VALUES].filter((a) => !skip.has(a))
    const missing = axes
      .filter((a) => !(`field.${a}` in en))
      .map((a) => `axis '${a}' is offered in the UI but has no field.${a} — it renders as its id`)
    expect([...new Set(missing)], missing.join('\n')).toEqual([])
  })

  test('an axis renders its catalog name, not its id', () => {
    // The failure this whole file exists for, stated once as a value.
    expect(fieldLabel('status_category')).toBe(en['field.status_category'])
    expect(fieldLabel('status_category')).not.toBe('status_category')
  })

  test('NOT_A_FIELD holds only axes that really have no field name', () => {
    const stale = NOT_A_FIELD.filter((a) => `field.${a}` in en).map(
      (a) => `${a} has field.${a} now — drop it from NOT_A_FIELD so the axis is checked`,
    )
    expect(stale, stale.join('\n')).toEqual([])
  })
})
