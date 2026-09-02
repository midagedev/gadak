import { beforeEach, describe, expect, test, vi } from 'vitest'

/*
 * GDK-1149: the origin page for a key comes from the row's stored url
 * first (what makes Linear work — no site, no slug in base_url), then the
 * site's /browse/KEY. A relative url (the built-in tracker's /browse/KEY)
 * is not an origin page. The store is mocked the way omnibox.test.ts does.
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
})

describe('issueOriginUrl', () => {
  test('Linear: the stored url wins with no site configured', () => {
    harness.pool.set('MID-1', { url: LINEAR })
    expect(issueOriginUrl('MID-1')).toBe(LINEAR)
  })

  test('Jira: a stored /browse/ url and the site fallback agree; older rows fall back', () => {
    harness.cfg.jiraBaseUrl = 'https://nimbus.example.com'
    harness.pool.set('NMB-1', { url: 'https://nimbus.example.com/browse/NMB-1' })
    harness.pool.set('NMB-2', {})
    expect(issueOriginUrl('NMB-1')).toBe('https://nimbus.example.com/browse/NMB-1')
    expect(issueOriginUrl('NMB-2')).toBe('https://nimbus.example.com/browse/NMB-2')
    expect(issueOriginUrl('NMB-3')).toBe('https://nimbus.example.com/browse/NMB-3')
  })

  test('built-in tracker: a relative /browse/KEY is not an origin page', () => {
    harness.pool.set('GDK-1', { url: '/browse/GDK-1' })
    expect(issueOriginUrl('GDK-1')).toBeNull()
    expect(issueOriginUrl('GDK-404')).toBeNull()
  })

  test('only http(s) counts', () => {
    harness.pool.set('X-1', { url: 'javascript:alert(1)' })
    harness.pool.set('X-2', { url: '  HTTPS://linear.app/w/issue/X-2  ' })
    expect(issueOriginUrl('X-1')).toBeNull()
    expect(issueOriginUrl('X-2')).toBe('HTTPS://linear.app/w/issue/X-2')
  })
})
