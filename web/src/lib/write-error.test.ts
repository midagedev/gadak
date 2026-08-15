import { beforeAll, describe, expect, test, vi } from 'vitest'
import { en, WRITE_ERROR_KEYS, writeErrorMessage } from './i18n/en'
import { ko } from './i18n/ko'
import { initLocale, locale, t } from './i18n'

beforeAll(() => {
  const mem = new Map<string, string>([['gadak_locale', 'en']])
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => mem.get(k) ?? null,
    setItem: (k: string, v: string) => {
      mem.set(k, v)
    },
    removeItem: (k: string) => {
      mem.delete(k)
    },
    clear: () => {
      mem.clear()
    },
    key: (i: number) => [...mem.keys()][i] ?? null,
    get length() {
      return mem.size
    },
  })
  initLocale()
  expect(locale()).toBe('en')
})

const FALLBACK = 'Could not transition status.'

describe('writeErrorMessage', () => {
  test('jira_unavailable is the catalog sentence, never the raw code', () => {
    expect(writeErrorMessage('jira_unavailable', FALLBACK, t)).toBe(en['write.jiraUnavailable'])
    expect(writeErrorMessage('jira_unavailable', FALLBACK, t)).not.toBe('jira_unavailable')
  })

  test('mapped write.go codes resolve to catalog sentences', () => {
    const want: Record<keyof typeof WRITE_ERROR_KEYS, string> = {
      credential_required: en['write.needToken'],
      credential_rejected: en['write.tokenRejected'],
      jira_unavailable: en['write.jiraUnavailable'],
      write_applied_mirror_stale: en['write.mirrorStale'],
      not_found: en['write.notFound'],
      summary_required: en['write.titleRequired'],
      summary_too_long: en['write.summaryTooLong'],
      project_issue_type_and_summary_required: en['write.requiredFields'],
      project_not_mirrored: en['write.projectNotMirrored'],
      field_not_editable: en['write.fieldNotEditable'],
      site_required: en['write.siteRequired'],
      invalid_token_expires: en['onboarding.errExpires'],
    }
    for (const [code, sentence] of Object.entries(want)) {
      expect(writeErrorMessage(code, FALLBACK, t), code).toBe(sentence)
    }
  })

  test('unmapped write.go codes and unknown snake_case fall back — never the raw code', () => {
    const unmapped = [
      'invalid_body',
      'email_and_token_required',
      'transition_id_required',
      'text_required',
      'file_required',
      'invalid_value',
      'totally_new_code',
    ]
    for (const code of unmapped) {
      expect(writeErrorMessage(code, FALLBACK, t), code).toBe(FALLBACK)
    }
  })

  test('empty / missing code uses the operation fallback', () => {
    expect(writeErrorMessage(null, FALLBACK, t)).toBe(FALLBACK)
    expect(writeErrorMessage(undefined, FALLBACK, t)).toBe(FALLBACK)
    expect(writeErrorMessage('', FALLBACK, t)).toBe(FALLBACK)
  })

  test('Jira prose from failJira APIError.Message() is shown as-is', () => {
    const prose = 'You do not have permission to transition this issue.'
    expect(writeErrorMessage(prose, FALLBACK, t)).toBe(prose)
    expect(writeErrorMessage('issuetype: Specify a valid issue type', FALLBACK, t)).toBe(
      'issuetype: Specify a valid issue type',
    )
  })

  test('every mapped key exists in en and ko', () => {
    for (const key of Object.values(WRITE_ERROR_KEYS)) {
      expect(en[key].length, `en ${key}`).toBeGreaterThan(0)
      expect(ko[key].length, `ko ${key}`).toBeGreaterThan(0)
    }
  })
})
