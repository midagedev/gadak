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

/*
 * GDK-302: a write that cannot run must not spin on Loading.
 *
 * vitest cannot mount the dialog (no svelte plugin). The request-not-made
 * assertion is Playwright's (e2e/duedate.spec.ts). This file holds the
 * wiring: one owner, two distinct terminal keys, no create-meta GET when
 * that owner already knows the answer.
 */
describe('NewIssueDialog create-meta gate (GDK-302)', () => {
  test('keeps no-credential and meta-failed as distinct catalog keys', () => {
    expect(SRC).toContain("t('write.needToken')")
    expect(SRC).toContain("t('write.metaFailed')")
    expect(SRC).toContain("t('common.setCredentials')")
    expect(SRC).toContain("t('common.retry')")
    expect(SRC).toContain("t('common.loading')")
  })

  test('asks write.configured and write.writeMetaLoaded before any create-meta GET', () => {
    expect(SRC).toContain('write.configured')
    expect(SRC).toContain('write.writeMetaLoaded')
    expect(SRC).toContain('write.credentialLoaded')
    expect(SRC).toMatch(/api\.getCreateMeta\s*\(/)
    // The empty-local-meta path must not be an unconditional fallback GET.
    expect(SRC).not.toMatch(
      /if \(write\.writeMetaProjects\.length\) \{[\s\S]*?\} else \{\s*void loadFallback\(\)/,
    )
  })

  test('exposes a free-when-unread dialog state marker', () => {
    expect(SRC).toMatch(/data-write-state=\{writeState\}/)
    expect(SRC).toMatch(/data-testid="new-issue-dialog"/)
  })
})

/*
 * GDK-254: required create-fields are advisory. The dialog still submits
 * when the extra-required warning is showing, and still submits when the
 * endpoint is missing — Playwright holds the live path (e2e/create-fields.spec.ts).
 */
describe('NewIssueDialog create fields (GDK-254)', () => {
  test('loads create fields by project key and issue type id', () => {
    expect(SRC).toMatch(/api\.getCreateFields\s*\(/)
    expect(SRC).toContain('issueTypeId')
    expect(SRC).not.toMatch(/getCreateFields\([^)]*\.name/)
  })

  test('classifies extra required fields through the shared helper', () => {
    expect(SRC).toContain('extraRequiredCreateFields')
    expect(SRC).toContain('isCreateFieldRequired')
    expect(SRC).toContain('CREATE_DIALOG_ALWAYS_SENT')
  })

  test('does not disable submit when extra required fields exist', () => {
    expect(SRC).not.toMatch(/disabled=\{submitting\s*\|\|/)
    expect(SRC).toMatch(/disabled=\{submitting\}/)
  })

  test('reuses the existing required-star token on known fields', () => {
    expect(SRC).toContain('text-status-reopen')
    expect(SRC).toContain('new-issue-required-warn')
    expect(SRC).toContain("t('write.createRequiresMore'")
  })
})
