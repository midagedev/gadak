import { beforeEach, describe, expect, test } from 'vitest'
import { linkTypeCatalog } from './link-types.svelte'

const BLOCKS = { id: '10000', name: 'Blocks', inward: 'is blocked by', outward: 'blocks' }

describe('linkTypeCatalog (GDK-1297): one catalog per workspace scope', () => {
  beforeEach(() => linkTypeCatalog.reset())

  test('nothing held until a scope answers', () => {
    expect(linkTypeCatalog.get('ws:a')).toBeNull()
  })

  test('the held answer is returned for its scope only', () => {
    linkTypeCatalog.set('ws:a', [BLOCKS])
    expect(linkTypeCatalog.get('ws:a')).toEqual([BLOCKS])
    expect(linkTypeCatalog.get('ws:b')).toBeNull()
  })

  test('an empty catalog is an answer too — no refetch storm on a site with no link types', () => {
    linkTypeCatalog.set('ws:a', [])
    expect(linkTypeCatalog.get('ws:a')).toEqual([])
  })

  test('a later scope replaces the earlier one', () => {
    linkTypeCatalog.set('ws:a', [BLOCKS])
    linkTypeCatalog.set('ws:b', [])
    expect(linkTypeCatalog.get('ws:a')).toBeNull()
    expect(linkTypeCatalog.get('ws:b')).toEqual([])
  })
})
