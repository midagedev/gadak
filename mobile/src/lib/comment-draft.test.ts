/*
 * Comment draft contract tests. The pinned contract (ux-report Q2,
 * market complaint #6): save on input, blank deletes, the draft survives
 * an in-flight send, and only the success ack clears it — a failure
 * keeps it so the composer can restore the body.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { clearDraft, commentDraftKey, readDraft, saveDraft, sendWithDraft } from './comment-draft'

/** Minimal localStorage for the node test environment (settings.test.ts pattern). */
class MemStorage {
  private m = new Map<string, string>()
  getItem(k: string): string | null {
    return this.m.has(k) ? (this.m.get(k) as string) : null
  }
  setItem(k: string, v: string): void {
    this.m.set(k, v)
  }
  removeItem(k: string): void {
    this.m.delete(k)
  }
}

let storage: MemStorage

beforeEach(() => {
  storage = new MemStorage()
  vi.stubGlobal('localStorage', storage)
})

const EP = 'https://home.example.ts.net'

describe('key shape and scoping', () => {
  it('builds the endpoint-scoped key', () => {
    expect(commentDraftKey(EP, 'STD-1')).toBe('gadak-mobile.comment-draft:https://home.example.ts.net:STD-1')
  })

  it('never leaks a draft across a re-pair to another home', () => {
    saveDraft('https://a.example.ts.net', 'STD-1', 'for home A')
    saveDraft('https://b.example.ts.net', 'STD-1', 'for home B')
    clearDraft('https://b.example.ts.net', 'STD-1')
    expect(readDraft('https://a.example.ts.net', 'STD-1')).toBe('for home A')
    expect(readDraft('https://b.example.ts.net', 'STD-1')).toBe('')
  })

  it('never leaks a draft across issue keys on the same home', () => {
    saveDraft(EP, 'STD-1', 'one')
    clearDraft(EP, 'STD-1')
    saveDraft(EP, 'STD-2', 'two')
    expect(readDraft(EP, 'STD-2')).toBe('two')
  })
})

describe('save-on-input', () => {
  it('round-trips a saved body', () => {
    saveDraft(EP, 'STD-1', 'half-written thought')
    expect(readDraft(EP, 'STD-1')).toBe('half-written thought')
  })

  it('returns an empty string for a missing draft', () => {
    expect(readDraft(EP, 'STD-404')).toBe('')
  })

  it('deletes the key when the body goes blank', () => {
    saveDraft(EP, 'STD-1', 'x')
    saveDraft(EP, 'STD-1', '')
    expect(readDraft(EP, 'STD-1')).toBe('')
    saveDraft(EP, 'STD-1', 'y')
    saveDraft(EP, 'STD-1', '   ')
    expect(readDraft(EP, 'STD-1')).toBe('')
  })
})

describe('send protocol', () => {
  it('keeps the stored draft while the post is in flight, clears it only on the ack', async () => {
    let release: () => void = () => {}
    const gate = new Promise<void>((r) => (release = r))
    const sent = sendWithDraft(EP, 'STD-1', 'draft body', () => gate)
    // In-flight: the draft must still be stored (an app kill here loses nothing).
    expect(readDraft(EP, 'STD-1')).toBe('draft body')
    release()
    const res = await sent
    expect(res.ok).toBe(true)
    expect(readDraft(EP, 'STD-1')).toBe('')
  })

  it('keeps the draft when the post fails', async () => {
    const res = await sendWithDraft(EP, 'STD-1', 'keep me', () =>
      Promise.reject(new Error('network')),
    )
    expect(res.ok).toBe(false)
    expect(readDraft(EP, 'STD-1')).toBe('keep me')
  })

  it('hands the failure back so the screen can restore the body', async () => {
    const boom = new Error('offline')
    const res = await sendWithDraft(EP, 'STD-1', 'x', () => Promise.reject(boom))
    expect(res.ok).toBe(false)
    if (!res.ok) expect(res.error).toBe(boom)
  })
})

describe('best-effort storage', () => {
  it('does not break sending when localStorage throws (private mode)', async () => {
    vi.stubGlobal(
      'localStorage',
      new (class {
        getItem(): string | null {
          throw new Error('unavailable')
        }
        setItem(): void {
          throw new Error('unavailable')
        }
        removeItem(): void {
          throw new Error('unavailable')
        }
      })(),
    )
    const res = await sendWithDraft(EP, 'STD-1', 'x', (b) => Promise.resolve(b.length))
    expect(res.ok).toBe(true)
    expect(readDraft(EP, 'STD-1')).toBe('')
  })
})
