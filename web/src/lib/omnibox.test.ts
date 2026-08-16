import { beforeEach, describe, expect, test, vi } from 'vitest'

/*
 * classifyOmnibox is sync so a paste can preventDefault before the clipboard
 * text lands in the search box. A wrong kind is a silent misroute: a Jira
 * link becomes a text search, a wiki URL opens the wrong panel.
 *
 * The unit project has no svelte plugin, so store / desktop-link modules
 * are mocked and the classifier is driven through its public surface.
 */

const harness = vi.hoisted(() => ({
  pool: new Map<string, true>(),
  pages: new Map<string, true>(),
  cfg: { jiraBaseUrl: '' },
}))

vi.mock('../stores/issues.svelte', () => ({
  issues: { pool: harness.pool },
}))

vi.mock('../stores/pages.svelte', () => ({
  pages: { byKey: harness.pages },
}))

vi.mock('../stores/selection.svelte', () => ({ selection: { select() {} } }))
vi.mock('../stores/views.svelte', () => ({ views: { source: [] } }))
vi.mock('../stores/write.svelte', () => ({ write: { toast() {} } }))
vi.mock('./show-issue-list', () => ({ showIssueList() {} }))
vi.mock('./config', () => ({
  config: () => harness.cfg,
}))
vi.mock('./desktop-links', async () => {
  const real = await import('./browse-classify')
  return {
    classifyAtlassianLink: real.classifyAtlassianLink,
    extractBrowseKey: real.extractBrowseKey,
    extractWikiPageId: real.extractWikiPageId,
    openContainedUrl() {},
  }
})

import { classifyOmnibox } from './omnibox'

const SITE = 'https://example.atlassian.net'

beforeEach(() => {
  harness.pool.clear()
  harness.pages.clear()
  harness.cfg.jiraBaseUrl = ''
})

describe('classifyOmnibox', () => {
  test('a bare issue key is text, not an issue jump', () => {
    // Blocks: inventing an issue-kind for "NMB-140". classifyOmnibox only
    // reads /browse/KEY (extractBrowseKey); a typed key stays a text search
    // so SearchBox's own jump path can own it. Treating it as issue here
    // would toast "missing" on paste before the box can jump.
    harness.pool.set('NMB-140', true)
    expect(classifyOmnibox('NMB-140')).toEqual({ kind: 'text' })
    expect(classifyOmnibox('  NMB-140  ')).toEqual({ kind: 'text' })
  })

  test('/browse/KEY in the pool opens the issue; a miss is named, not searched', () => {
    // Blocks: pasting /browse/NMB-140 running a text search for that path
    // (or opening a blank panel when the key is not in the mirror).
    harness.pool.set('NMB-140', true)
    expect(classifyOmnibox('/browse/NMB-140')).toEqual({ kind: 'issue', key: 'NMB-140' })
    expect(classifyOmnibox('/browse/nmb-140/')).toEqual({ kind: 'issue', key: 'NMB-140' })
    expect(classifyOmnibox('/browse/ZZZ-9')).toEqual({ kind: 'issue-miss', key: 'ZZZ-9' })
  })

  test('an Atlassian /browse/KEY URL follows the same issue table', () => {
    // Blocks: pasting a Jira issue link and getting a text search for the URL.
    harness.pool.set('NMB-140', true)
    expect(classifyOmnibox(`${SITE}/browse/NMB-140`)).toEqual({ kind: 'issue', key: 'NMB-140' })
    expect(classifyOmnibox(`${SITE}/browse/NMB-999`)).toEqual({ kind: 'issue-miss', key: 'NMB-999' })
  })

  test('a /browse/KEY URL with a query string is not an issue (extractBrowseKey requires end or /)', () => {
    // Blocks: a future change silently flipping this. Today a focused-comment
    // link does not classify as issue — pin the current miss so a fix is a
    // deliberate assertion change, not an unnoticed route change.
    harness.pool.set('NMB-140', true)
    harness.cfg.jiraBaseUrl = SITE
    expect(classifyOmnibox(`${SITE}/browse/NMB-140?focusedCommentId=1`)).toEqual({
      kind: 'contained',
      url: `${SITE}/browse/NMB-140?focusedCommentId=1`,
    })
    harness.cfg.jiraBaseUrl = ''
    expect(classifyOmnibox(`${SITE}/browse/NMB-140?focusedCommentId=1`)).toEqual({ kind: 'text' })
  })

  test('a mirrored Confluence page URL opens the page', () => {
    // Blocks: pasting a wiki link and running a text search (or opening the
    // in-app browser) for a page the mirror already has.
    harness.pages.set('4213', true)
    expect(classifyOmnibox(`${SITE}/wiki/spaces/ENG/pages/4213`)).toEqual({
      kind: 'page',
      key: '4213',
    })
    expect(classifyOmnibox(`${SITE}/wiki/spaces/ENG/pages/4213?src=link`)).toEqual({
      kind: 'page',
      key: '4213',
    })
  })

  test('an unmirrored same-site wiki URL is contained; off-site / unconfigured is text', () => {
    // Blocks: an unmirrored page URL becoming a text search when the site is
    // known (should open the in-app browser), or a foreign wiki URL being
    // forced into the contained pane.
    const href = `${SITE}/wiki/spaces/ENG/pages/9999`
    harness.cfg.jiraBaseUrl = SITE
    expect(classifyOmnibox(href)).toEqual({ kind: 'contained', url: href })

    harness.cfg.jiraBaseUrl = ''
    expect(classifyOmnibox(href)).toEqual({ kind: 'text' })

    harness.cfg.jiraBaseUrl = SITE
    expect(classifyOmnibox('https://other.example/wiki/spaces/X/pages/1')).toEqual({ kind: 'text' })
  })

  test('free text and junk stay text so FTS runs', () => {
    // Blocks: a normal query or clipboard junk being treated as JQL / a
    // filter / a contained URL — the search box swallowing the paste.
    expect(classifyOmnibox('flaky upload')).toEqual({ kind: 'text' })
    expect(classifyOmnibox('??!!')).toEqual({ kind: 'text' })
    expect(classifyOmnibox('')).toEqual({ kind: 'text' })
    expect(classifyOmnibox('   ')).toEqual({ kind: 'text' })
  })

  test('a JQL paste or a navigator URL with jql= is jql, not text', () => {
    // Blocks: pasting `project = NMA AND statusCategory = "In Progress"`
    // (or the /issues/?jql= URL) running FTS against the raw string.
    expect(classifyOmnibox('project = NMA AND statusCategory = "In Progress"')).toEqual({
      kind: 'jql',
      input: 'project = NMA AND statusCategory = "In Progress"',
    })
    const url = `${SITE}/issues/?jql=project%20%3D%20NMA`
    expect(classifyOmnibox(url)).toEqual({ kind: 'jql', input: url })
  })

  test('a navigator URL with only filter= is a filter, not a text search', () => {
    // Blocks: pasting ?filter=10000 running FTS for the URL instead of
    // applying the saved Jira filter.
    const url = `${SITE}/issues/?filter=10000`
    expect(classifyOmnibox(url)).toEqual({ kind: 'filter', id: '10000', input: url })
  })
})
