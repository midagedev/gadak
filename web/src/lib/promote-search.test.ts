import { describe, expect, test } from 'vitest'
import { promoteSearchToHash } from './promote-search'

describe('promoteSearchToHash', () => {
  test('empty search is a no-op', () => {
    expect(promoteSearchToHash('', '')).toEqual({ hash: '', search: '' })
    expect(promoteSearchToHash('', '#/?sc=done')).toEqual({ hash: '#/?sc=done', search: '' })
  })

  test('unknown params stay in search and the hash is untouched', () => {
    expect(promoteSearchToHash('?utm_source=x', '')).toEqual({
      hash: '',
      search: '?utm_source=x',
    })
    expect(promoteSearchToHash('?utm_source=x&foo=bar', '#/?sc=done')).toEqual({
      hash: '#/?sc=done',
      search: '?utm_source=x&foo=bar',
    })
  })

  test('a place param moves into an empty hash and is stripped from search', () => {
    expect(promoteSearchToHash('?issue=NMB-110', '')).toEqual({
      hash: '#/?issue=NMB-110',
      search: '',
    })
  })

  test('a view param moves into the hash', () => {
    expect(promoteSearchToHash('?sc=done', '')).toEqual({
      hash: '#/?sc=done',
      search: '',
    })
  })

  test('a discovered f.<alias> axis is a view param and is promoted', () => {
    expect(promoteSearchToHash('?f.sprint=12', '')).toEqual({
      hash: '#/?f.sprint=12',
      search: '',
    })
  })

  test('hash wins on key conflict; the search copy is still stripped', () => {
    expect(promoteSearchToHash('?issue=NMB-1', '#/?issue=NMB-110')).toEqual({
      hash: '#/?issue=NMB-110',
      search: '',
    })
  })

  test('unknown params survive next to a promoted one', () => {
    expect(promoteSearchToHash('?issue=NMB-110&utm_source=x', '')).toEqual({
      hash: '#/?issue=NMB-110',
      search: '?utm_source=x',
    })
  })

  test('existing hash params are kept and new keys are appended', () => {
    expect(promoteSearchToHash('?issue=NMB-110', '#/?sc=done')).toEqual({
      hash: '#/?sc=done&issue=NMB-110',
      search: '',
    })
  })

  test('hash path is preserved', () => {
    expect(promoteSearchToHash('?issue=NMB-110', '#/board')).toEqual({
      hash: '#/board?issue=NMB-110',
      search: '',
    })
  })
})
