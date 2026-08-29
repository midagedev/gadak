import { describe, expect, test } from 'vitest'
import { en } from './i18n/en'
import {
  docsEmptyClickAction,
  docsEmptyCopy,
  docsEmptyGlyph,
  docsEmptyState,
  docsListEmptyKind,
  type DocsEmptyState,
} from './docs-empty'

const base = {
  hasDocsServer: true,
  confluenceEnabled: true,
  fetchingDocuments: false,
  indexLoadFailed: false,
  confluenceRuns: [] as { error?: string }[] | null,
}

describe('docsEmptyState', () => {
  test('no docs server is unavailable, even when Confluence looks configured', () => {
    expect(
      docsEmptyState({
        ...base,
        hasDocsServer: false,
        confluenceEnabled: true,
        fetchingDocuments: true,
        confluenceRuns: [{ error: '403' }],
      }),
    ).toBe('unavailable')
  })

  test('configured-off is off, and only off', () => {
    expect(docsEmptyState({ ...base, confluenceEnabled: false, confluenceRuns: null })).toBe('off')
  })

  test('a documents pass outranks a failed last run', () => {
    expect(
      docsEmptyState({
        ...base,
        fetchingDocuments: true,
        confluenceRuns: [{ error: 'confluence: 403 forbidden' }],
      }),
    ).toBe('syncing')
  })

  test('runs not asked yet is never, not failed', () => {
    expect(docsEmptyState({ ...base, confluenceRuns: null })).toBe('never')
  })

  test('empty run history is never fetched', () => {
    expect(docsEmptyState({ ...base, confluenceRuns: [] })).toBe('never')
  })

  test('the newest run\'s error is failed', () => {
    expect(
      docsEmptyState({
        ...base,
        confluenceRuns: [{ error: 'confluence: 403 forbidden' }, {}],
      }),
    ).toBe('failed')
  })

  test('a successful pass with no pages blames the selection', () => {
    expect(docsEmptyState({ ...base, confluenceRuns: [{}] })).toBe('empty')
  })

  // GDK-1067: the index request failing used to read as this table's
  // onboarding answers — 'empty' after a clean run, 'never' while the run
  // fetch (killed by the same outage) had not answered.
  test('a failed index after a clean pass is loadfailed, not empty', () => {
    expect(docsEmptyState({ ...base, indexLoadFailed: true, confluenceRuns: [{}] })).toBe(
      'loadfailed',
    )
  })

  test('a failed index outranks runs-not-asked', () => {
    expect(docsEmptyState({ ...base, indexLoadFailed: true, confluenceRuns: null })).toBe(
      'loadfailed',
    )
  })

  test('an errored run outranks a failed index — it carries the cause', () => {
    expect(
      docsEmptyState({
        ...base,
        indexLoadFailed: true,
        confluenceRuns: [{ error: 'confluence: 503 unavailable' }],
      }),
    ).toBe('failed')
  })
})

describe('docsEmptyCopy', () => {
  test('each state owns its own sentence keys', () => {
    const table: Record<DocsEmptyState, { title: string; hint: string | null; prefersBusy: boolean }> = {
      unavailable: { title: en['sidebar.docsUnavailable'], hint: null, prefersBusy: false },
      off: { title: en['sidebar.docsNoneTitle'], hint: en['sidebar.docsNoneHint'], prefersBusy: false },
      syncing: { title: en['sidebar.docsSyncing'], hint: null, prefersBusy: true },
      never: {
        title: en['sidebar.docsNotFetched'],
        hint: en['sidebar.docsNotFetchedHint'],
        prefersBusy: false,
      },
      failed: {
        title: en['sidebar.docsFetchFailed'],
        hint: en['sidebar.docsFetchFailedHint'],
        prefersBusy: false,
      },
      empty: {
        title: en['sidebar.docsEmptySpaces'],
        hint: en['sidebar.docsEmptySpacesHint'],
        prefersBusy: false,
      },
      loadfailed: { title: en['docs.loadFailed'], hint: null, prefersBusy: false },
    }
    for (const [state, want] of Object.entries(table) as [DocsEmptyState, (typeof table)['off']][]) {
      const copy = docsEmptyCopy(state)
      expect(en[copy.titleKey], state).toBe(want.title)
      expect(copy.hintKey ? en[copy.hintKey] : null, state).toBe(want.hint)
      expect(copy.titlePrefersBusy, state).toBe(want.prefersBusy)
    }
  })

  test('off is the only copy that asks to turn Confluence on', () => {
    const states: DocsEmptyState[] = [
      'off',
      'never',
      'failed',
      'empty',
      'syncing',
      'unavailable',
      'loadfailed',
    ]
    const hits = states.filter((s) => {
      const c = docsEmptyCopy(s)
      const title = en[c.titleKey]
      const hint = c.hintKey ? en[c.hintKey] : ''
      return /Turn on Confluence/.test(title) || /Turn on Confluence/.test(hint)
    })
    expect(hits).toEqual(['off'])
  })
})

describe('docsEmptyClickAction / glyph', () => {
  test('click: never syncs, loadfailed retries, off/failed/empty open settings', () => {
    expect(docsEmptyClickAction('unavailable')).toBe('none')
    expect(docsEmptyClickAction('syncing')).toBe('none')
    expect(docsEmptyClickAction('never')).toBe('sync')
    expect(docsEmptyClickAction('loadfailed')).toBe('retry')
    expect(docsEmptyClickAction('off')).toBe('settings')
    expect(docsEmptyClickAction('failed')).toBe('settings')
    expect(docsEmptyClickAction('empty')).toBe('settings')
  })

  test('glyph: gear is reserved for unconfigured', () => {
    expect(docsEmptyGlyph('off')).toBe('settings')
    expect(docsEmptyGlyph('failed')).toBe('warning')
    expect(docsEmptyGlyph('loadfailed')).toBe('warning')
    expect(docsEmptyGlyph('syncing')).toBe('refresh')
    expect(docsEmptyGlyph('never')).toBe('refresh')
    expect(docsEmptyGlyph('empty')).toBe('search-x')
    expect(docsEmptyGlyph('unavailable')).toBe('search-x')
  })
})

describe('docsListEmptyKind', () => {
  test('rows present is not an empty state', () => {
    expect(
      docsListEmptyKind({ empty: false, filtering: true, hasNeedle: true, tab: 'viewed' }),
    ).toBeNull()
  })

  test('filter vs viewed vs activity are four distinct branches', () => {
    expect(
      docsListEmptyKind({ empty: true, filtering: true, hasNeedle: true, tab: 'updated' }),
    ).toBe('filter-text')
    expect(
      docsListEmptyKind({ empty: true, filtering: true, hasNeedle: false, tab: 'updated' }),
    ).toBe('filter-label')
    expect(
      docsListEmptyKind({ empty: true, filtering: false, hasNeedle: false, tab: 'viewed' }),
    ).toBe('viewed')
    expect(
      docsListEmptyKind({ empty: true, filtering: false, hasNeedle: false, tab: 'updated' }),
    ).toBe('recent')
    expect(
      docsListEmptyKind({ empty: true, filtering: false, hasNeedle: false, tab: 'author' }),
    ).toBe('recent')
  })
})
