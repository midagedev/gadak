import { describe, expect, it } from 'vitest'
import {
  classifyAtlassianLink,
  extractBrowseKey,
  extractWikiPageId,
} from './browse-classify'

const BASE = 'https://example.atlassian.net'

describe('extractBrowseKey', () => {
  it('reads and uppercases a /browse/KEY path', () => {
    expect(extractBrowseKey('/browse/nma-12')).toBe('NMA-12')
    expect(extractBrowseKey('/browse/NMA-12/')).toBe('NMA-12')
  })

  it('rejects paths that are not issue keys', () => {
    expect(extractBrowseKey('/browse/NMA-')).toBeNull()
    expect(extractBrowseKey('/browse/')).toBeNull()
    expect(extractBrowseKey('/wiki/spaces/X')).toBeNull()
    // A trailing path segment must not turn a key into a match.
    expect(extractBrowseKey('/projects/NMA/issues/NMA-1')).toBeNull()
  })
})

describe('extractWikiPageId', () => {
  it('reads the content id from a wiki pages path', () => {
    expect(extractWikiPageId('/wiki/spaces/TEAM/pages/4213')).toBe('4213')
  })

  it('rejects other wiki paths', () => {
    expect(extractWikiPageId('/wiki/spaces/TEAM')).toBeNull()
    expect(extractWikiPageId('/browse/NMA-1')).toBeNull()
  })
})

describe('classifyAtlassianLink', () => {
  it('classifies an issue page on the site', () => {
    expect(classifyAtlassianLink(`${BASE}/browse/NMA-1`, BASE)).toEqual({
      inApp: true,
      kind: 'issue',
      key: 'NMA-1',
    })
  })

  it('classifies a wiki page on the site', () => {
    expect(classifyAtlassianLink(`${BASE}/wiki/spaces/X/pages/42?q=1`, BASE)).toEqual({
      inApp: true,
      kind: 'page',
      key: '42',
    })
  })

  it('keeps same-site non-entity pages in-app with no key', () => {
    expect(classifyAtlassianLink(`${BASE}/secure/Dashboard.jspa`, BASE)).toEqual({
      inApp: true,
      kind: 'other',
      key: null,
    })
  })

  it('refuses off-site urls, other protocols, and malformed input', () => {
    expect(classifyAtlassianLink('https://elsewhere.example/x', BASE)).toEqual({
      inApp: false,
      kind: 'other',
      key: null,
    })
    expect(classifyAtlassianLink('javascript:alert(1)', BASE).inApp).toBe(false)
    expect(classifyAtlassianLink('not a url', BASE).inApp).toBe(false)
  })

  it('refuses everything when no site is configured', () => {
    expect(classifyAtlassianLink(`${BASE}/browse/NMA-1`, null)).toEqual({
      inApp: false,
      kind: 'other',
      key: null,
    })
  })
})
