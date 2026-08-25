import { describe, expect, test } from 'vitest'
import { CACHED_ISSUE_ARRAY_FIELDS, normalizeCachedIssue } from './cache-row'

function healthyRow() {
  return {
    issue_key: 'NMB-1',
    summary: 'intact',
    labels: ['keep'],
    fix_versions: ['v1'],
    components: ['web'],
    reporter_id: 'acct-1',
    priority_id: 'p1',
  }
}

describe('normalizeCachedIssue', () => {
  test('a legacy row missing labels comes back with empty arrays', () => {
    const row = { issue_key: 'LEGACY-1', summary: 'old', reporter_id: 'acct' }
    const out = normalizeCachedIssue(row)
    expect(out).not.toBeNull()
    expect(out?.labels).toEqual([])
    expect(out?.fix_versions).toEqual([])
    expect(out?.components).toEqual([])
    expect(out?.issue_key).toBe('LEGACY-1')
    expect(out?.summary).toBe('old')
    expect(out?.reporter_id).toBe('acct')
    expect('priority_id' in (out as object)).toBe(false)
  })

  test('a row whose labels is a string comes back with []', () => {
    const row = {
      issue_key: 'CORRUPT-1',
      labels: 'not-an-array',
      fix_versions: ['v1'],
      components: ['web'],
    }
    const out = normalizeCachedIssue(row)
    expect(out?.labels).toEqual([])
    expect(out?.fix_versions).toEqual(['v1'])
    expect(out?.components).toEqual(['web'])
  })

  test('a row with no issue_key is dropped', () => {
    expect(normalizeCachedIssue({ summary: 'no key' })).toBeNull()
    expect(normalizeCachedIssue({ issue_key: '' })).toBeNull()
    expect(normalizeCachedIssue(null)).toBeNull()
    expect(normalizeCachedIssue('LEGACY-1')).toBeNull()
    expect(normalizeCachedIssue(undefined)).toBeNull()
  })

  test('a healthy row is returned unchanged', () => {
    const row = healthyRow()
    const out = normalizeCachedIssue(row)
    expect(out).toBe(row)
    expect(out).toEqual(row)
  })
})

describe('CACHED_ISSUE_ARRAY_FIELDS', () => {
  test('owns the three arrays the list indexes into', () => {
    expect(CACHED_ISSUE_ARRAY_FIELDS).toEqual(['labels', 'fix_versions', 'components'])
  })
})
