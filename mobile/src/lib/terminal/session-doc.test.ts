import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import type { FetchLike } from '../api'
import {
  createShellSession,
  normalizeSessionDoc,
  TERMINAL_CURSOR_BLINK_FALLBACK,
  TERMINAL_SCROLLBACK_FALLBACK,
} from './api'

/*
 * GDK-896 R3 — the phone consumes the create response's behavior
 * (scrollback, cursorBlink) the way the web pane already does (R2; the
 * sibling tests are web/src/lib/terminal/renderer.test.ts). Two halves:
 * normalize fills what an older serve never sent, and the pane wiring
 * carries the values into the live xterm options. The wiring half is a
 * source contract, not a live renderer run: this tree's vitest is the
 * node environment (vite.config.ts test.environment), so createRenderer
 * is out of reach by design — the same decision renderer.test.ts made
 * for GDK-899.
 */

const here = dirname(fileURLToPath(import.meta.url))
const rendererSrc = readFileSync(join(here, 'renderer.ts'), 'utf8')
const shellSrc = readFileSync(join(here, '../../screens/Shell.svelte'), 'utf8')

const TOKEN = '<terminal-token>'
const session = { endpoint: 'https://home.example.ts.net', token: TOKEN }

function fakeFetch(status: number, body: unknown): FetchLike {
  return async () => new Response(body === null ? null : JSON.stringify(body), { status })
}

describe('GDK-896 — normalizeSessionDoc', () => {
  it('fills the behavior fields an older serve never sent with the fallbacks', () => {
    const doc = normalizeSessionDoc({ id: 's1', cols: 90, rows: 30 })
    expect(doc.scrollback).toBe(TERMINAL_SCROLLBACK_FALLBACK)
    expect(doc.cursorBlink).toBe(TERMINAL_CURSOR_BLINK_FALLBACK)
    // The fallbacks are the server's own defaults (EffectiveTerminal), so
    // an old serve behind a new app renders what the literals hardcoded.
    expect(TERMINAL_SCROLLBACK_FALLBACK).toBe(5000)
    expect(TERMINAL_CURSOR_BLINK_FALLBACK).toBe(false)
  })

  it('passes a response carrying behavior through untouched', () => {
    expect(
      normalizeSessionDoc({ id: 's1', cols: 90, rows: 30, scrollback: 20000, cursorBlink: true }),
    ).toEqual({ id: 's1', cols: 90, rows: 30, scrollback: 20000, cursorBlink: true })
  })

  it('reads a zero or non-finite scrollback as absent, not as a budget of 0', () => {
    expect(normalizeSessionDoc({ id: 's', cols: 1, rows: 1, scrollback: 0 }).scrollback).toBe(
      TERMINAL_SCROLLBACK_FALLBACK,
    )
    expect(
      normalizeSessionDoc({ id: 's', cols: 1, rows: 1, scrollback: Number.NaN }).scrollback,
    ).toBe(TERMINAL_SCROLLBACK_FALLBACK)
  })

  it('floors a fractional scrollback (the buffer is an integer line count)', () => {
    expect(
      normalizeSessionDoc({ id: 's', cols: 1, rows: 1, scrollback: 9000.7 }).scrollback,
    ).toBe(9000)
  })

  it('reads a non-boolean cursorBlink as absent', () => {
    expect(
      normalizeSessionDoc({ id: 's', cols: 1, rows: 1, cursorBlink: 'true' as unknown as boolean })
        .cursorBlink,
    ).toBe(TERMINAL_CURSOR_BLINK_FALLBACK)
  })
})

describe('GDK-896 — createShellSession normalizes the response', () => {
  it('a response without behavior fields still yields the defaults', async () => {
    const fn = fakeFetch(200, { id: 'sess-1', cols: 80, rows: 24 })
    await expect(createShellSession(80, 24, session, fn)).resolves.toEqual({
      id: 'sess-1',
      cols: 80,
      rows: 24,
      scrollback: 5000,
      cursorBlink: false,
    })
  })

  it('a response carrying behavior reaches the caller unchanged', async () => {
    const fn = fakeFetch(200, {
      id: 'sess-1',
      cols: 80,
      rows: 24,
      scrollback: 20000,
      cursorBlink: true,
    })
    await expect(createShellSession(80, 24, session, fn)).resolves.toEqual({
      id: 'sess-1',
      cols: 80,
      rows: 24,
      scrollback: 20000,
      cursorBlink: true,
    })
  })
})

describe('GDK-896 — behavior wiring (source contract)', () => {
  it('the renderer owns no behavior literals: they land through applyBehavior', () => {
    expect(rendererSrc).toContain('applyBehavior(b: TerminalBehavior): void')
    expect(rendererSrc).toContain('term.options.scrollback = b.scrollback')
    expect(rendererSrc).toContain('term.options.cursorBlink = b.cursorBlink')
    expect(rendererSrc).not.toContain('scrollback: 5000')
    expect(rendererSrc).not.toContain('cursorBlink: false')
  })

  it('the pane applies the create response before the first attach', () => {
    const applyIdx = shellSrc.indexOf(
      'renderer?.applyBehavior({ scrollback: doc.scrollback, cursorBlink: doc.cursorBlink })',
    )
    const attachIdx = shellSrc.indexOf('attachSocket(doc.id, { afterCreate: true })')
    expect(applyIdx).toBeGreaterThan(-1)
    expect(attachIdx).toBeGreaterThan(applyIdx)
  })

  it('the pane starts at the fallback, so a kept-session reattach never runs on xterm defaults', () => {
    expect(shellSrc).toContain('scrollback: TERMINAL_SCROLLBACK_FALLBACK')
    expect(shellSrc).toContain('cursorBlink: TERMINAL_CURSOR_BLINK_FALLBACK')
  })
})
