import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { devSlot, parseTokenKind, tokenDel, tokenGet, tokenSet } from './secure'

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

describe('token kind', () => {
  it('defaults missing kind to serve', () => {
    expect(parseTokenKind()).toBe('serve')
    expect(parseTokenKind(undefined)).toBe('serve')
    expect(parseTokenKind(null)).toBe('serve')
  })

  it('accepts serve and terminal', () => {
    expect(parseTokenKind('serve')).toBe('serve')
    expect(parseTokenKind('terminal')).toBe('terminal')
  })

  it('rejects an unknown kind instead of defaulting', () => {
    expect(() => parseTokenKind('push')).toThrow('unknown token kind')
    expect(() => parseTokenKind('')).toThrow('unknown token kind')
  })

  it('does not put a token value in the unknown-kind error', () => {
    try {
      parseTokenKind('<terminal-token>')
      throw new Error('expected throw')
    } catch (err) {
      expect((err as Error).message).not.toContain('<terminal-token>')
    }
  })
})

describe('dev slots', () => {
  it('keeps the serve slot name frozen', () => {
    expect(devSlot()).toBe('gadak.dev.token')
    expect(devSlot('serve')).toBe('gadak.dev.token')
  })

  it('gives the terminal kind its own slot', () => {
    expect(devSlot('terminal')).toBe('gadak.dev.token.terminal')
    expect(devSlot('terminal')).not.toBe(devSlot('serve'))
  })
})

describe('dev storage isolation', () => {
  it('writes each kind to its own slot and does not clobber the other', async () => {
    await tokenSet('<serve-token>')
    await tokenSet('<terminal-token>', 'terminal')
    expect(mem.get('gadak.dev.token')).toBe('<serve-token>')
    expect(mem.get('gadak.dev.token.terminal')).toBe('<terminal-token>')
    expect(await tokenGet()).toBe('<serve-token>')
    expect(await tokenGet('serve')).toBe('<serve-token>')
    expect(await tokenGet('terminal')).toBe('<terminal-token>')
  })

  it('deletes one kind without touching the other', async () => {
    await tokenSet('<serve-token>')
    await tokenSet('<terminal-token>', 'terminal')
    await tokenDel()
    expect(await tokenGet()).toBeNull()
    expect(await tokenGet('terminal')).toBe('<terminal-token>')
    await tokenDel('terminal')
    expect(await tokenGet('terminal')).toBeNull()
  })

  it('returns null when the slot is empty and there is no tauri bridge', async () => {
    expect(await tokenGet()).toBeNull()
    expect(await tokenGet('terminal')).toBeNull()
  })
})

describe('Keychain key freeze', () => {
  it('still stores serve tokens under pairing-token', () => {
    const rs = readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), '../../src-tauri/src/lib.rs'),
      'utf8',
    )
    expect(rs).toMatch(/const TOKEN_KEY_SERVE: &str = "pairing-token";/)
    expect(rs).toMatch(/const TOKEN_KEY_TERMINAL: &str = "pairing-token-terminal";/)
    expect(rs).toMatch(/None \| Some\("serve"\) => Ok\(TOKEN_KEY_SERVE\)/)
    expect(rs).toMatch(/Some\("terminal"\) => Ok\(TOKEN_KEY_TERMINAL\)/)
    expect(rs).toMatch(/Some\(_\) => Err\("unknown token kind"/)
  })

  it('composes host-keyed slots and rejects every other host id (GDK-1097 B1)', () => {
    const rs = readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), '../../src-tauri/src/lib.rs'),
      'utf8',
    )
    expect(rs).toMatch(/fn token_slot\(kind: Option<&str>, host: Option<&str>\) -> Result<String, String>/)
    expect(rs).toMatch(/None => Ok\(base\.to_string\(\)\)/)
    expect(rs).toMatch(/Some\(h\) if valid_host_id\(h\) => Ok\(format!\("\{base\}@\{h\}"\)\)/)
    expect(rs).toMatch(/Some\(_\) => Err\("unknown host id"\.into\(\)\)/)
    expect(rs).toMatch(/fn valid_host_id\(host: &str\) -> bool/)
  })
})

describe('host-keyed slots (GDK-1097 B1)', () => {
  it('composes the dev slot as <dev slot>@<hostId>, the Keychain shape', () => {
    expect(devSlot('serve', 'local')).toBe('gadak.dev.token@local')
    expect(devSlot('terminal', 'paired:0123abcd')).toBe('gadak.dev.token.terminal@paired:0123abcd')
  })

  it('leaves the slot key untouched when no hostId is given', () => {
    expect(devSlot('serve', undefined)).toBe('gadak.dev.token')
    expect(devSlot()).toBe('gadak.dev.token')
    expect(devSlot('terminal')).toBe('gadak.dev.token.terminal')
  })

  it('stores and reads a host-keyed token without touching the legacy slot', async () => {
    await tokenSet('<serve-token>')
    await tokenSet('<host-serve-token>', 'serve', 'paired:0123abcd')
    expect(mem.get('gadak.dev.token')).toBe('<serve-token>')
    expect(mem.get('gadak.dev.token@paired:0123abcd')).toBe('<host-serve-token>')
    expect(await tokenGet()).toBe('<serve-token>')
    expect(await tokenGet('serve', 'paired:0123abcd')).toBe('<host-serve-token>')
  })

  it('deletes one host slot without touching the others', async () => {
    await tokenSet('<legacy>', 'serve')
    await tokenSet('<local>', 'serve', 'local')
    await tokenSet('<paired>', 'serve', 'paired:0123abcd')
    await tokenDel('serve', 'local')
    expect(await tokenGet('serve', 'local')).toBeNull()
    expect(await tokenGet('serve', 'paired:0123abcd')).toBe('<paired>')
    expect(await tokenGet('serve')).toBe('<legacy>')
  })

  it('throws on a hostId that could splice the slot string', async () => {
    await expect(tokenGet('serve', 'paired:0123ABCD')).rejects.toThrow('unknown host id')
    await expect(tokenGet('serve', 'x@y')).rejects.toThrow('unknown host id')
    await expect(tokenGet('serve', 'local!')).rejects.toThrow('unknown host id')
    await expect(tokenSet('t', 'serve', 'pairing-token@evil')).rejects.toThrow('unknown host id')
    await expect(tokenDel('serve', '../../etc')).rejects.toThrow('unknown host id')
    // Nothing was composed or stored along the way.
    expect([...mem.keys()].filter((k) => k.includes('@')).length).toBe(0)
  })
})
