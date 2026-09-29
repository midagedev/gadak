/*
 * GDK-2042 — the status-category fold's recurrence layer.
 *
 * Two axes were empty until this round. The fallback axis: three copies of
 * the fold disagreed about the unknown key, and the phone's said
 * 'inprogress' for a year while the web said 'new'. The surface axis: a
 * 'completed' key read Done under filtering (effectiveCategory) and New
 * under grouping (groupCategory), and no gate could see it because each
 * surface was internally consistent — only their agreement shows it.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { CATEGORY_ALIASES, categoryOf, knownCategory, type StatusCategory } from './status-category'
import { effectiveCategory } from './view-config'
import { groupCategory, type GroupableIssue } from './issue-group'

function row(status_category: string): GroupableIssue {
  return {
    issue_key: 'STD-80',
    status: null,
    status_category,
    assignee: null,
    priority: null,
    priority_rank: null,
    issue_type: null,
  }
}

/**
 * The pinned table — the union of the three copies this module replaced.
 * Spelled out, not derived from CATEGORY_ALIASES: a key someone adds to the
 * table without a pin here is a silent behavior change, and the test below
 * makes it red instead.
 */
const PINNED: Record<string, StatusCategory> = {
  new: 'new',
  todo: 'new',
  inprogress: 'inprogress',
  indeterminate: 'inprogress',
  done: 'done',
  complete: 'done',
  completed: 'done',
}

describe('categoryOf (GDK-2042)', () => {
  test('folds every key of the union table onto the pinned bucket', () => {
    for (const [key, want] of Object.entries(PINNED)) {
      expect(categoryOf(key), `categoryOf(${JSON.stringify(key)})`).toBe(want)
    }
    // And the table enumerates exactly the pinned keys — nothing unnamed.
    expect(Object.keys(CATEGORY_ALIASES).sort()).toEqual(Object.keys(PINNED).sort())
  })

  test("unknown and empty keys fold to 'new', the internal/statuscat answer", () => {
    expect(categoryOf('not-a-category')).toBe('new')
    expect(categoryOf('')).toBe('new')
    expect(categoryOf(null)).toBe('new')
    expect(categoryOf(undefined)).toBe('new')
    // Recognized vs merely folded — the distinction knownCategory exists for.
    expect(knownCategory('not-a-category')).toBe(null)
    expect(knownCategory('')).toBe(null)
  })

  test('a status display name is not a key, in any language', () => {
    // The grouper carried 'in progress' by inheritance (GDK-1993, lifted
    // out of the filters store) until GDK-2042 dropped it: it is the
    // English spelling of a NAME, and recognizing it while refusing
    // '진행 중' and '進行中' is exactly the localized-name trap the three
    // tokens exist to close. No origin sends it — neither demo fixture
    // holds it and Jira's REST keys have no spaces.
    for (const name of ['in progress', 'In Progress', '진행 중', '進行中', '완료', 'Done ']) {
      expect(knownCategory(name), `${name} must not be a category key`).toBe(null)
    }
  })

  test('compares lowercased', () => {
    expect(categoryOf('DONE')).toBe('done')
    expect(categoryOf('Completed')).toBe('done')
    expect(categoryOf('Indeterminate')).toBe('inprogress')
    expect(categoryOf('TODO')).toBe('new')
  })

  test('accepts every key Go accepts, folding to the same bucket', () => {
    // Parsed from the Go owner's source rather than restated, so a key Go
    // grows is red here until the front table grows with it:
    // internal/statuscat/category.go, KnownCategory (repo-relative on purpose:
    // scripts/scan-internal.sh refuses an absolute home path).
    // The parse is anchored to the function name and its result asserted,
    // so a reformat fails loudly instead of passing silently.
    const goFile = join(
      dirname(fileURLToPath(import.meta.url)),
      '../../../internal/statuscat/category.go',
    )
    const src = readFileSync(goFile, 'utf8')
    const start = src.indexOf('func KnownCategory')
    const fn = src.slice(start, src.indexOf('\n}', start))
    const accepted: Record<string, string> = {}
    for (const m of fn.matchAll(/case ([^:]+):\s*\n\s*return "([a-z]+)", true/g)) {
      for (const key of m[1].split(',')) accepted[key.trim().replace(/"/g, '')] = m[2]
    }
    expect(Object.keys(accepted).sort(), 'KnownCategory cases parsed').toEqual([
      'done',
      'indeterminate',
      'inprogress',
      'new',
    ])
    for (const [key, goFold] of Object.entries(accepted)) {
      expect(CATEGORY_ALIASES[key], `${key} is in the front table`).toBe(goFold)
      expect(knownCategory(key), `${key} recognized`).toBe(goFold)
    }
  })
})

describe('every surface folds through the one owner (GDK-2042)', () => {
  test('filtering and grouping answer the same for every union key and for an unknown key', () => {
    const diverged: string[] = []
    for (const key of [...Object.keys(PINNED), 'not-a-category']) {
      const filtered = effectiveCategory(key)
      const grouped = groupCategory(row(key))
      if (grouped !== filtered) diverged.push(`${key}: filter=${filtered} group=${grouped}`)
    }
    // Collected, not thrown-at: one key diverging must not hide the next.
    // On the pre-GDK-2042 source this listed 'in progress' and 'completed'.
    expect(diverged).toEqual([])
  })
})
