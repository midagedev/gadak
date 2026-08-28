import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { CACHE_KEY, HOST_SCOPED_KEYS, META_KEY, hostKey, migrateHostKeys } from './host-keys'

// The mem-storage convention of hosts.test.ts / store.test.ts. Fixture
// values follow the repo's TEST-NET rule (192.0.2.x); host ids are real
// roster shapes ('local', 'paired:' + 8 hex).

const mem = new Map<string, string>()

beforeEach(() => {
  mem.clear()
  globalThis.localStorage = {
    getItem: (k: string) => mem.get(k) ?? null,
    setItem: (k: string, v: string) => {
      mem.set(k, v)
    },
    removeItem: (k: string) => {
      mem.delete(k)
    },
    clear: () => mem.clear(),
    key: () => null,
    length: 0,
  } as Storage
})

afterEach(() => {
  mem.clear()
})

describe('hostKey — composition', () => {
  it('keeps the bare key for a null host: the address pre-B2 phones read', () => {
    expect(hostKey(CACHE_KEY, null)).toBe('gadak.snapshot')
    expect(hostKey(META_KEY, null)).toBe('gadak.pairing.meta')
  })

  it('appends the host id in the same shape secure.ts devSlot composes', () => {
    expect(hostKey(CACHE_KEY, 'local')).toBe('gadak.snapshot@local')
    expect(hostKey(META_KEY, 'paired:0123abcd')).toBe('gadak.pairing.meta@paired:0123abcd')
  })

  it('scopes exactly the six session documents', () => {
    expect([...HOST_SCOPED_KEYS].sort()).toEqual(
      [
        'gadak.pairing.meta',
        'gadak.pairing.meta.terminal',
        'gadak.snapshot',
        'gadak.views',
        'gadak.pages',
        'gadak.issues.scope',
      ].sort(),
    )
  })
})

describe('migrateHostKeys — verified rename, per key', () => {
  const HOST = 'paired:0123abcd'

  it('success: renames every present legacy key byte-identically and drops the legacy address', () => {
    mem.set(META_KEY, JSON.stringify({ endpoint: 'http://192.0.2.10:7877', label: 'desk', expires_at: '' }))
    mem.set(CACHE_KEY, JSON.stringify({ etag: null, issues: [], serverTime: '2026-08-01T00:00:00Z' }))

    const renamed = migrateHostKeys(HOST)

    expect(renamed.sort()).toEqual([META_KEY, CACHE_KEY].sort())
    for (const base of [META_KEY, CACHE_KEY]) {
      expect(mem.get(base), `${base} legacy must be dropped`).toBeUndefined()
      expect(mem.get(`${base}@${HOST}`), `${base}@host must exist`).toBeTruthy()
    }
    // Byte-identical copy, not a re-serialization.
    expect(mem.get(`${CACHE_KEY}@${HOST}`)).toBe(
      JSON.stringify({ etag: null, issues: [], serverTime: '2026-08-01T00:00:00Z' }),
    )
  })

  it('no-op without an active host: an unmigrated phone keeps its keys', () => {
    mem.set(CACHE_KEY, 'bytes')
    expect(migrateHostKeys(null)).toEqual([])
    expect(mem.get(CACHE_KEY)).toBe('bytes')
    expect(mem.get(`${CACHE_KEY}@local`)).toBeUndefined()
  })

  it('idempotent per key: an existing namespaced key wins and its legacy twin is left alone', () => {
    mem.set(`${CACHE_KEY}@${HOST}`, 'new-bytes')
    mem.set(CACHE_KEY, 'old-bytes')

    expect(migrateHostKeys(HOST)).toEqual([])
    expect(mem.get(`${CACHE_KEY}@${HOST}`)).toBe('new-bytes')
    expect(mem.get(CACHE_KEY)).toBe('old-bytes')
  })

  it('verify failure keeps the legacy key and clears the bad shadow so the next boot retries', () => {
    mem.set(CACHE_KEY, 'bytes')
    // setItem lands, but the read-back answers something else — storage
    // that cannot be trusted must not cost the phone its snapshot.
    const real = globalThis.localStorage
    globalThis.localStorage = {
      ...real,
      getItem: (k: string) => (k === `${CACHE_KEY}@${HOST}` ? 'corrupted' : real.getItem(k)),
    } as Storage

    expect(migrateHostKeys(HOST)).toEqual([])
    expect(mem.get(CACHE_KEY), 'legacy preserved').toBe('bytes')
    expect(mem.get(`${CACHE_KEY}@${HOST}`), 'shadow cleared for the next retry').toBeUndefined()
  })
})
