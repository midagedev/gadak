/*
 * GDK-738: the docs-empty store's resolved hint key is docsEmptyCopy's, for
 * every input combination lib/docs-empty.test.ts already names. That file
 * owns the table; this one only checks the store does not invent a second.
 */
import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  docsEmptyCopy,
  docsEmptyState,
  type DocsEmptyRun,
} from '../lib/docs-empty'

const deps = vi.hoisted(() => ({
  confluenceEnabled: true,
  hasDocs: true,
  fetching: false,
  bySpace: [] as unknown[],
  getSyncRuns: vi.fn(async () => ({ runs: [] as DocsEmptyRun[] })),
}))

vi.mock('../lib/config', () => ({
  config: () => ({ confluenceEnabled: deps.confluenceEnabled }),
  hasServerVerb: (v: string) => (v === 'docs' ? deps.hasDocs : true),
}))

vi.mock('../lib/mirror-status', () => ({
  fetchingDocuments: () => deps.fetching,
  busyLabel: () => null,
}))

vi.mock('../lib/api', () => ({
  getSyncRuns: deps.getSyncRuns,
}))

vi.mock('./pages.svelte', () => ({
  pages: {
    get bySpace() {
      return deps.bySpace
    },
  },
}))

import { docsEmpty } from './docs-empty.svelte'

/** Same combinations as web/src/lib/docs-empty.test.ts (docsEmptyState describe). */
const matrix: { name: string; input: Parameters<typeof docsEmptyState>[0] }[] = [
  {
    name: 'unavailable',
    input: {
      hasDocsServer: false,
      confluenceEnabled: true,
      fetchingDocuments: true,
      confluenceRuns: [{ error: '403' }],
    },
  },
  {
    name: 'off',
    input: {
      hasDocsServer: true,
      confluenceEnabled: false,
      fetchingDocuments: false,
      confluenceRuns: null,
    },
  },
  {
    name: 'syncing',
    input: {
      hasDocsServer: true,
      confluenceEnabled: true,
      fetchingDocuments: true,
      confluenceRuns: [{ error: 'confluence: 403 forbidden' }],
    },
  },
  {
    name: 'never (not asked)',
    input: {
      hasDocsServer: true,
      confluenceEnabled: true,
      fetchingDocuments: false,
      confluenceRuns: null,
    },
  },
  {
    name: 'never (empty history)',
    input: {
      hasDocsServer: true,
      confluenceEnabled: true,
      fetchingDocuments: false,
      confluenceRuns: [],
    },
  },
  {
    name: 'failed',
    input: {
      hasDocsServer: true,
      confluenceEnabled: true,
      fetchingDocuments: false,
      confluenceRuns: [{ error: 'confluence: 403 forbidden' }, {}],
    },
  },
  {
    name: 'empty',
    input: {
      hasDocsServer: true,
      confluenceEnabled: true,
      fetchingDocuments: false,
      confluenceRuns: [{}],
    },
  },
]

function apply(input: Parameters<typeof docsEmptyState>[0]): void {
  deps.hasDocs = input.hasDocsServer
  deps.confluenceEnabled = input.confluenceEnabled
  deps.fetching = input.fetchingDocuments
  docsEmpty.confluenceRuns = input.confluenceRuns
}

afterEach(() => {
  deps.confluenceEnabled = true
  deps.hasDocs = true
  deps.fetching = false
  deps.bySpace = []
  docsEmpty.confluenceRuns = null
})

describe('docsEmpty store resolution', () => {
  test('each matrix row resolves the hint key docsEmptyCopy names', () => {
    for (const row of matrix) {
      apply(row.input)
      const want = docsEmptyCopy(docsEmptyState(row.input)).hintKey
      expect(docsEmpty.copy.hintKey, row.name).toBe(want)
      expect(docsEmpty.state, row.name).toBe(docsEmptyState(row.input))
    }
  })
})
