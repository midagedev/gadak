import { describe, expect, test } from 'vitest'
import { pageAuthorGroupKey } from './pages.svelte'

describe('pageAuthorGroupKey', () => {
  test('I8: group key is author_id, then display name', () => {
    expect(pageAuthorGroupKey({ author: 'Kim', author_id: 'acc-1' })).toBe('acc-1')
    expect(pageAuthorGroupKey({ author: 'Kim', author_id: 'acc-2' })).toBe('acc-2')
    expect(pageAuthorGroupKey({ author: 'Kim', author_id: '' })).toBe('Kim')
    expect(pageAuthorGroupKey({ author: 'Kim' })).toBe('Kim')
    expect(pageAuthorGroupKey({ author: '', author_id: '' })).toBe('')
  })
})
