import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  getActiveHostId,
  hasHostsDoc,
  hostIdForEndpoint,
  listHosts,
  removeHost,
  setActiveHostId,
  touchHost,
  upsertHostFromPairing,
} from './hosts'

// Endpoints follow the repo's TEST-NET fixture rule (192.0.2.x).
const EP = 'http://192.0.2.10:7877'

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

describe('hostIdForEndpoint', () => {
  it('maps the empty endpoint to the local (dev proxy) id', async () => {
    expect(await hostIdForEndpoint('')).toBe('local')
    expect(await hostIdForEndpoint('   ')).toBe('local')
  })

  it('derives paired: + 8 lowercase hex for an endpoint', async () => {
    const id = await hostIdForEndpoint(EP)
    expect(id).toMatch(/^paired:[0-9a-f]{8}$/)
  })

  it('is stable: the same endpoint re-derives the same id', async () => {
    expect(await hostIdForEndpoint(EP)).toBe(await hostIdForEndpoint(EP))
  })

  it('normalizes case and surrounding whitespace, nothing else', async () => {
    expect(await hostIdForEndpoint(`  ${EP.toUpperCase()}  `)).toBe(await hostIdForEndpoint(EP))
    // The URL is otherwise taken as it stands — different path, different id.
    expect(await hostIdForEndpoint(`${EP}/`)).not.toBe(await hostIdForEndpoint(EP))
    expect(await hostIdForEndpoint('http://192.0.2.11:7877')).not.toBe(await hostIdForEndpoint(EP))
  })
})

describe('upsertHostFromPairing', () => {
  it('creates a first host with revision 1 and echoes it in the roster', async () => {
    const host = await upsertHostFromPairing({ endpoint: EP, label: 'desk' })
    expect(host.id).toBe(await hostIdForEndpoint(EP))
    expect(host.label).toBe('desk')
    expect(host.endpoint).toBe(EP)
    expect(host.pairingRevision).toBe(1)
    const roster = listHosts()
    expect(roster).toHaveLength(1)
    expect(roster[0].id).toBe(host.id)
    expect(hasHostsDoc()).toBe(true)
  })

  it('re-pairing the same endpoint bumps the revision instead of adding a row', async () => {
    await upsertHostFromPairing({ endpoint: EP, label: 'desk' })
    const again = await upsertHostFromPairing({ endpoint: ` ${EP.toUpperCase()} `, label: 'study' })
    const roster = listHosts()
    expect(roster).toHaveLength(1)
    expect(again.pairingRevision).toBe(2)
    expect(again.label).toBe('study')
    expect(again.id).toBe(roster[0].id)
  })

  it('keeps a second endpoint as its own row', async () => {
    await upsertHostFromPairing({ endpoint: EP, label: 'desk' })
    await upsertHostFromPairing({ endpoint: 'http://192.0.2.11:7877', label: 'other' })
    expect(listHosts()).toHaveLength(2)
  })
})

describe('touchHost / removeHost', () => {
  it('touchHost advances lastUsedAt and ignores unknown ids', async () => {
    const host = await upsertHostFromPairing({ endpoint: EP, label: 'desk' })
    const before = listHosts()[0].lastUsedAt
    touchHost(host.id)
    const after = listHosts()[0].lastUsedAt
    expect(after >= before).toBe(true)
    expect(() => touchHost('paired:deadbeef')).not.toThrow()
    expect(listHosts()).toHaveLength(1)
  })

  it('removeHost drops the row and the active pointer when it pointed there', async () => {
    const host = await upsertHostFromPairing({ endpoint: EP, label: 'desk' })
    setActiveHostId(host.id)
    removeHost(host.id)
    expect(listHosts()).toHaveLength(0)
    expect(getActiveHostId()).toBeNull()
  })

  it('removeHost leaves another host active untouched', async () => {
    const a = await upsertHostFromPairing({ endpoint: EP, label: 'desk' })
    const b = await upsertHostFromPairing({ endpoint: 'http://192.0.2.11:7877', label: 'other' })
    setActiveHostId(b.id)
    removeHost(a.id)
    expect(getActiveHostId()).toBe(b.id)
    expect(listHosts().map((h) => h.id)).toEqual([b.id])
  })
})

describe('active host pointer', () => {
  it('sets, reads, and clears', async () => {
    expect(getActiveHostId()).toBeNull()
    const host = await upsertHostFromPairing({ endpoint: EP, label: 'desk' })
    setActiveHostId(host.id)
    expect(getActiveHostId()).toBe(host.id)
    setActiveHostId(null)
    expect(getActiveHostId()).toBeNull()
  })

  it('reads garbage as null and refuses to store a malformed id', () => {
    mem.set('gadak.hosts.active', 'not-a-host-id')
    expect(getActiveHostId()).toBeNull()
    setActiveHostId('pairing-token@evil')
    // The malformed id never lands; the stale value stays as it was.
    expect(mem.get('gadak.hosts.active')).toBe('not-a-host-id')
  })
})

describe('schema discipline', () => {
  it('treats a doc with a newer schema as an empty roster', () => {
    mem.set('gadak.hosts.v1', JSON.stringify({ schema: 2, hosts: [{ id: 'local' }] }))
    expect(listHosts()).toEqual([])
    expect(hasHostsDoc()).toBe(true)
  })

  it('never overwrites a newer-schema doc', async () => {
    const future = JSON.stringify({ schema: 2, hosts: [], extra: true })
    mem.set('gadak.hosts.v1', future)
    await upsertHostFromPairing({ endpoint: EP, label: 'desk' })
    expect(mem.get('gadak.hosts.v1')).toBe(future)
  })

  it('treats an unparsable doc as an empty roster', () => {
    mem.set('gadak.hosts.v1', '{not json')
    expect(listHosts()).toEqual([])
  })
})

describe('storage unavailable — quiet no-ops', () => {
  function breakStorage(): void {
    globalThis.localStorage = {
      getItem: () => {
        throw new Error('unavailable')
      },
      setItem: () => {
        throw new Error('unavailable')
      },
      removeItem: () => {
        throw new Error('unavailable')
      },
      clear: () => {
        throw new Error('unavailable')
      },
      key: () => null,
      length: 0,
    } as Storage
  }

  it('every API answers without throwing', async () => {
    breakStorage()
    expect(listHosts()).toEqual([])
    expect(hasHostsDoc()).toBe(false)
    expect(getActiveHostId()).toBeNull()
    expect(() => setActiveHostId('local')).not.toThrow()
    expect(() => setActiveHostId(null)).not.toThrow()
    expect(() => touchHost('local')).not.toThrow()
    expect(() => removeHost('local')).not.toThrow()
    const host = await upsertHostFromPairing({ endpoint: EP, label: 'desk' })
    expect(host.id).toBe(await hostIdForEndpoint(EP))
    expect(host.pairingRevision).toBe(1)
  })
})
