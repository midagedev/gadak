import { describe, expect, test } from 'vitest'
import { composeCommentDraftKey, THEME_STORAGE_KEY, themeStorageKeyFromPath } from './storage'

describe('composeCommentDraftKey', () => {
  test('C6: draft keys split by workspace and by site', () => {
    const a = composeCommentDraftKey('', 'https://a.example.com', 'NMB-1')
    const b = composeCommentDraftKey('', 'https://b.example.com', 'NMB-1')
    const ws = composeCommentDraftKey('work', 'https://a.example.com', 'NMB-1')
    expect(a).toBe('gadak:comment-draft:site:a.example.com:NMB-1')
    expect(b).toBe('gadak:comment-draft:site:b.example.com:NMB-1')
    expect(ws).toBe('gadak:comment-draft:ws:work|site:a.example.com:NMB-1')
    expect(a).not.toBe(b)
    expect(a).not.toBe(ws)
  })
})

describe('themeStorageKeyFromPath', () => {
  test('default mount keeps the unscoped key; /w/<name> is a distinct mirror', () => {
    expect(themeStorageKeyFromPath('/')).toBe(THEME_STORAGE_KEY)
    expect(themeStorageKeyFromPath('/issues')).toBe(THEME_STORAGE_KEY)
    expect(themeStorageKeyFromPath('/wiki')).toBe(THEME_STORAGE_KEY)
    expect(themeStorageKeyFromPath('/w/')).toBe(THEME_STORAGE_KEY)
    expect(themeStorageKeyFromPath('/w/oss')).toBe(`${THEME_STORAGE_KEY}:oss`)
    expect(themeStorageKeyFromPath('/w/oss/')).toBe(`${THEME_STORAGE_KEY}:oss`)
    expect(themeStorageKeyFromPath('/w/oss/issues')).toBe(`${THEME_STORAGE_KEY}:oss`)
    expect(themeStorageKeyFromPath('/w/work')).toBe(`${THEME_STORAGE_KEY}:work`)
    expect(themeStorageKeyFromPath('/w/oss')).not.toBe(themeStorageKeyFromPath('/'))
    expect(themeStorageKeyFromPath('/w/oss')).not.toBe(themeStorageKeyFromPath('/w/work'))
  })
})
