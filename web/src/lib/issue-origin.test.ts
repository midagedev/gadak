import { beforeEach, describe, expect, test, vi } from 'vitest'

/*
 * GDK-1308: the origin page for a key is decided by the origin's type, and
 * the branches never cross — Jira builds /browse/KEY from the site, Linear
 * uses the page it minted (the row's stored url), the built-in tracker has
 * none. A Linear row with no url is a missing link, not a Jira URL built
 * from a site the workspace does not have. The store is mocked the way
 * omnibox.test.ts does.
 */

const harness = vi.hoisted(() => ({
  pool: new Map<string, { url?: string }>(),
  cfg: { jiraBaseUrl: '', originType: '' },
}))

vi.mock('../stores/issues.svelte', () => ({
  issues: { pool: harness.pool },
}))
vi.mock('./config', () => ({
  config: () => harness.cfg,
  jiraBrowseUrl: (key: string) =>
    harness.cfg.jiraBaseUrl ? `${harness.cfg.jiraBaseUrl}/browse/${key}` : null,
}))

import { issueOriginUrl } from './issue-origin'

const LINEAR = 'https://linear.app/midagedev/issue/MID-1/get-familiar-with-linear'

beforeEach(() => {
  harness.pool.clear()
  harness.cfg.jiraBaseUrl = ''
  harness.cfg.originType = ''
})

describe('issueOriginUrl', () => {
  test('Jira: the site browse page, whatever the row stores', () => {
    harness.cfg.originType = 'jira'
    harness.cfg.jiraBaseUrl = 'https://nimbus.example.com'
    harness.pool.set('NMB-1', { url: 'https://nimbus.example.com/browse/NMB-1' })
    harness.pool.set('NMB-2', {})
    expect(issueOriginUrl('NMB-1')).toBe('https://nimbus.example.com/browse/NMB-1')
    expect(issueOriginUrl('NMB-2')).toBe('https://nimbus.example.com/browse/NMB-2')
    // The mirror may lag a key the site has — open is the escape hatch.
    expect(issueOriginUrl('NMB-3')).toBe('https://nimbus.example.com/browse/NMB-3')
  })

  test('Jira without a site has no page', () => {
    harness.cfg.originType = 'jira'
    harness.pool.set('NMB-1', { url: LINEAR })
    expect(issueOriginUrl('NMB-1')).toBeNull()
  })

  test('Linear: the page Linear minted, from the row', () => {
    harness.cfg.originType = 'linear'
    harness.pool.set('MID-1', { url: LINEAR })
    expect(issueOriginUrl('MID-1')).toBe(LINEAR)
  })

  test('Linear: no row or no url is no link — never a Jira URL', () => {
    harness.cfg.originType = 'linear'
    // Even with a base url present (a stale or mixed config), Linear does
    // not borrow Jira's page shape.
    harness.cfg.jiraBaseUrl = 'https://nimbus.example.com'
    harness.pool.set('MID-2', {})
    expect(issueOriginUrl('MID-2')).toBeNull()
    expect(issueOriginUrl('MID-404')).toBeNull()
  })

  test('built-in tracker: no origin page, even with a stored url', () => {
    harness.cfg.originType = 'gadak'
    harness.cfg.jiraBaseUrl = 'https://nimbus.example.com'
    harness.pool.set('GDK-1', { url: '/browse/GDK-1' })
    harness.pool.set('GDK-2', { url: 'https://nimbus.example.com/browse/GDK-2' })
    expect(issueOriginUrl('GDK-1')).toBeNull()
    expect(issueOriginUrl('GDK-2')).toBeNull()
    expect(issueOriginUrl('GDK-404')).toBeNull()
  })

  test('Linear: only http(s) counts', () => {
    harness.cfg.originType = 'linear'
    harness.pool.set('X-1', { url: 'javascript:alert(1)' })
    harness.pool.set('X-2', { url: '  HTTPS://linear.app/w/issue/X-2  ' })
    expect(issueOriginUrl('X-1')).toBeNull()
    expect(issueOriginUrl('X-2')).toBe('HTTPS://linear.app/w/issue/X-2')
  })
})
