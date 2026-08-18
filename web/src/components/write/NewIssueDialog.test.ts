/*
 * GDK-248: the create dialog must send a catalog priority id, never a
 * hardcoded English display name.
 *
 * There is no component-mount harness (vitest is node, no svelte plugin),
 * so this reads the source the way SearchBox.test.ts / HistoryView.test.ts
 * do. Rendered catalog → POST payload is Playwright's job
 * (e2e/duedate.spec.ts).
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(join(HERE, 'NewIssueDialog.svelte'), 'utf8')

describe('NewIssueDialog create payload (GDK-248)', () => {
  test('does not hardcode Jira default priority display names', () => {
    expect(SRC).not.toMatch(/const PRIORITIES/)
    expect(SRC).not.toMatch(/['"]Highest['"]/)
    expect(SRC).not.toMatch(/['"]Lowest['"]/)
  })

  test('reads GET priorities/ through the existing api wrapper', () => {
    expect(SRC).toMatch(/api\.getPriorities\s*\(/)
  })

  test('select values are catalog ids and labels are catalog names', () => {
    expect(SRC).toMatch(/<option value=\{p\.id\}>\{p\.name\}<\/option>/)
  })

  test('submit sends priority_id and does not send a priority name key', () => {
    expect(SRC).toMatch(/priority_id:\s*priority\s*\|\|\s*undefined/)
    expect(SRC).not.toMatch(/^\s*priority:\s*priority/m)
  })
})
