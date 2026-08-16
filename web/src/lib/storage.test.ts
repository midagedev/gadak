import { describe, expect, test } from 'vitest'
import { composeCommentDraftKey } from './storage'

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
