/*
 * Leftover fieldMap / editableFields editors must not come back.
 * LoadFor clears those maps before GET answers, so the KeyValueRows
 * surface could never hold a value.
 *
 * No component-mount harness: vitest is environment:'node' and the unit
 * project loads no svelte plugin (FeaturesTab.test.ts). This file scans
 * the source the compiler emits.
 */
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const DRAFT = join(HERE, 'draft.ts')
const FIELDS_TAB = join(HERE, 'FieldsTab.svelte')
const KEY_VALUE_ROWS = join(HERE, 'KeyValueRows.svelte')

describe('leftover field-mapping editors are gone', () => {
  test('draft.ts toSettings does not send fieldMap or editableFields', () => {
    const src = readFileSync(DRAFT, 'utf8')
    expect(src, 'PUT payload sent fieldMap: rec(...)').not.toMatch(/fieldMap:\s*rec\(/)
    expect(src, 'PUT payload sent editableFields: rec(...)').not.toMatch(/editableFields:\s*rec\(/)
    expect(src, 'toDraft still carried fieldMap').not.toMatch(/fieldMap:\s*kvRows\(/)
    expect(src, 'toDraft still carried editableFields').not.toMatch(/editableFields:\s*kvRows\(/)
  })

  test('KeyValueRows.svelte does not exist', () => {
    expect(existsSync(KEY_VALUE_ROWS), 'KeyValueRows.svelte is the dead fieldMap / editableFields editor').toBe(
      false,
    )
  })

  test('FieldsTab does not render the leftover editors', () => {
    const src = readFileSync(FIELDS_TAB, 'utf8')
    expect(src).not.toMatch(/KeyValueRows/)
    expect(src).not.toMatch(/settings\.fieldMap/)
    expect(src).not.toMatch(/settings\.editableFields/)
  })
})
