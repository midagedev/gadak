import { describe, expect, test } from 'vitest'
import { composeCacheScope, profileName } from './config'

describe('composeCacheScope / profileName', () => {
  test('C5: distinct sites never share a cache scope', () => {
    const a = composeCacheScope('', 'https://a.example.com')
    const b = composeCacheScope('', 'https://b.example.com')
    expect(a).toBe('site:a.example.com')
    expect(b).toBe('site:b.example.com')
    expect(a).not.toBe(b)
  })

  test('C5: workspace + site both appear, empty site stays workspace-only', () => {
    expect(composeCacheScope('work', 'https://a.example.com')).toBe('ws:work|site:a.example.com')
    expect(composeCacheScope('work', '')).toBe('ws:work')
    expect(composeCacheScope('', '')).toBe('')
  })

  test('C5: named profile on / is a distinct scope; default is omitted', () => {
    const def = composeCacheScope('', 'https://a.example.com', 'default')
    const named = composeCacheScope('', 'https://a.example.com', 'work')
    expect(def).toBe('site:a.example.com')
    expect(named).toBe('site:a.example.com|profile:work')
    expect(def).not.toBe(named)
    // Workspace already partitions; profile is not double-counted.
    expect(composeCacheScope('work', 'https://a.example.com', 'work')).toBe(
      'ws:work|site:a.example.com',
    )
    expect(profileName('')).toBe('default')
    expect(profileName('default')).toBe('default')
    expect(profileName('work')).toBe('work')
  })
})
