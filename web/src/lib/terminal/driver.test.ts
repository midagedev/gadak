/*
 * The pane-socket driver's contract (GDK-1767), in two halves.
 *
 * The first is the extraction itself, as a source sweep: the timing
 * constants and the settle schedule had *two* owners, and the sweep fails
 * the moment a pane grows its own copy again — a new pane constant or a
 * direct settleResize call is exactly how the drift the extraction closed
 * would come back. This half was red before the refactor (both panes
 * carried all four markers) and is the FAIL-first record for it.
 *
 * The second is the state machine on a fake transport: the generation
 * guard, the backoff ladder, the 60 s grace, the open timeout, the resize
 * cache, and the never-opened verdicts. vi.useFakeTimers() mocks
 * setTimeout and Date.now together, which is what makes the grace test a
 * loop over wall-clock seconds rather than a sleep.
 *
 * Runs in the root `unit` project (plain ts, node env) — same slot
 * gdk-944.test.ts sits in.
 */

import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createTerminalDriver,
  TERMINAL_GRACE_MS,
  TERMINAL_RECONNECT_BACKOFF_MS,
  TERMINAL_WS_OPEN_MS,
  type TerminalDriver,
  type TerminalDriverHost,
  type DriverStatus,
} from './driver'
import type { DroppedReason, SocketHandle, SocketHandlers } from './protocol'

const here = dirname(fileURLToPath(import.meta.url))

describe('GDK-1767: the driver skeleton has one owner', () => {
  const panes = [
    ['web pane', resolve(here, '../../components/terminal/TerminalPane.svelte')],
    ['phone shell', resolve(here, '../../../../mobile/src/screens/Shell.svelte')],
  ] as const

  // What only ./driver (and ./session's re-export) may spell. A pane that
  // grows one of these back is the drift this extraction closed.
  const markers = [
    'TERMINAL_GRACE_MS',
    'TERMINAL_RECONNECT_BACKOFF_MS',
    'TERMINAL_WS_OPEN_MS',
    'settleResize',
  ]

  it.each(panes)('%s no longer carries the socket skeleton', (_name, path) => {
    const src = readFileSync(path, 'utf8')
    const found = markers.filter((m) => src.includes(m))
    expect(found, `${path} still owns ${found.join(', ')}`).toEqual([])
  })

  it('the skeleton constants are shared, not re-spelled', () => {
    // The phone imports the driver without ./session (which pulls the
    // wails transport), so the constants must live on the driver itself.
    expect(TERMINAL_GRACE_MS).toBe(60_000)
    expect(TERMINAL_RECONNECT_BACKOFF_MS).toEqual([500, 1000, 2000, 4000])
    expect(TERMINAL_WS_OPEN_MS).toBe(8_000)
  })
})

/* ── a fake transport + host: sockets are records tests can speak for ── */

interface FakeSocket {
  n: number
  id: string
  handlers: SocketHandlers
  sent: Uint8Array[]
  resized: Array<[number, number]>
  closed: boolean
  opened: boolean
}

function boot(over: Partial<TerminalDriverHost> = {}): {
  driver: TerminalDriver
  sockets: FakeSocket[]
  host: FakeHost
} {
  const host = new FakeHost()
  Object.assign(host, over)
  return { driver: createTerminalDriver(host), sockets: host.sockets, host }
}

class FakeHost implements TerminalDriverHost {
  sockets: FakeSocket[] = []
  statuses: DriverStatus[] = []
  bytes: Uint8Array[] = []
  lives: boolean[] = []
  exits: number[] = []
  droppeds: string[] = []
  sessionGone = 0
  recreates = 0
  attached = 0
  size: { cols: number; rows: number } = { cols: 100, rows: 30 }
  canMeasure = true
  hide = false
  #seq = 0

  open(id: string, handlers: SocketHandlers): SocketHandle {
    const rec: FakeSocket = {
      n: ++this.#seq,
      id,
      handlers,
      sent: [],
      resized: [],
      closed: false,
      opened: false,
    }
    // wrapped so handle.close() can answer "did this socket ever open" the
    // way a real transport would.
    const wrapped: SocketHandlers = {
      onOpen: () => {
        rec.opened = true
        handlers.onOpen()
      },
      onBytes: (b) => handlers.onBytes(b),
      onExit: (c) => handlers.onExit(c),
      onDropped: (r) => handlers.onDropped(r),
      onClose: (never) => handlers.onClose(never),
    }
    rec.handlers = wrapped
    this.sockets.push(rec)
    return {
      send: (b) => rec.sent.push(b),
      resize: (cols, rows) => rec.resized.push([cols, rows]),
      close: () => {
        if (rec.closed) return
        rec.closed = true
        wrapped.onClose(!rec.opened)
      },
    }
  }

  fittedSize() {
    return { ...this.size }
  }

  measurable() {
    return this.canMeasure
  }

  onAttached() {
    this.attached++
  }

  onBytes(data: Uint8Array) {
    this.bytes.push(data)
  }

  onExit(code: number) {
    this.exits.push(code)
  }

  onDropped(reason: DroppedReason) {
    this.droppeds.push(reason)
  }

  onSessionGone() {
    this.sessionGone++
  }

  onRecreate() {
    this.recreates++
  }

  onStatus(status: DriverStatus) {
    this.statuses.push(status)
  }

  onLive(live: boolean) {
    this.lives.push(live)
  }

  hidden() {
    return this.hide
  }
}

const last = <T>(xs: T[]): T => xs[xs.length - 1]

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('driver state machine', () => {
  it('a socket the pane has moved off cannot speak (GDK-1153)', () => {
    const { driver, sockets, host } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    driver.attach('b', { afterCreate: false }) // detach closed socket 0
    // Late traffic from the socket the pane left:
    sockets[0].handlers.onBytes(new Uint8Array([65]))
    sockets[0].handlers.onClose(true)
    expect(host.bytes).toEqual([]) // stale bytes never reach the pane
    expect(host.statuses.filter((s) => s.kind === 'unavailable')).toEqual([])
    expect(sockets.filter((s) => s.id === 'a')).toHaveLength(1) // no reconnect for the old id
    // The pane's socket still works:
    sockets[1].handlers.onOpen()
    sockets[1].handlers.onBytes(new Uint8Array([66]))
    expect(host.bytes.map((b) => Array.from(b))).toEqual([[66]])
    expect(driver.currentId).toBe('b')
  })

  it('detach() forgets the id, so the row just left is clickable again (GDK-1185)', () => {
    // The web pane's switchTo returns early on `want === driver.currentId`.
    // newSession detaches and nulls the selection while the create POST is
    // in flight; if the driver kept naming the old session, a click on that
    // row during the create read as "already there" and no attach happened —
    // the exact GDK-1185 symptom, re-measured 2026-09-11 on
    // e2e/terminal-strip.spec.ts:391 after the extraction.
    const { driver, sockets } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    expect(driver.currentId).toBe('a')
    driver.detach()
    expect(driver.currentId).toBeNull()
    driver.attach('a', { afterCreate: false })
    expect(driver.currentId).toBe('a')
    expect(sockets.filter((s) => s.id === 'a')).toHaveLength(2)
  })

  it('a close reconnects on the backoff ladder, capped', () => {
    const { driver, sockets, host } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    sockets[0].handlers.onClose(false)
    expect(last(host.statuses)).toEqual({ kind: 'reconnecting' })
    expect(sockets).toHaveLength(1)
    vi.advanceTimersByTime(500)
    expect(sockets).toHaveLength(2)
    last(sockets).handlers.onOpen()
    last(sockets).handlers.onClose(false)
    vi.advanceTimersByTime(1000)
    expect(sockets).toHaveLength(3)
    last(sockets).handlers.onOpen()
    last(sockets).handlers.onClose(false)
    vi.advanceTimersByTime(2000)
    expect(sockets).toHaveLength(4)
    for (let i = 0; i < 3; i++) {
      last(sockets).handlers.onOpen()
      last(sockets).handlers.onClose(false)
      vi.advanceTimersByTime(4000)
      // Cap: the ladder stops climbing at 4000.
      expect(sockets).toHaveLength(5 + i)
    }
  })

  it('sixty seconds without a reopen ends the attachment, out loud', () => {
    const { driver, sockets, host } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    sockets[0].handlers.onClose(false) // grace clock starts here
    // Cycle never-opened reconnects by advancing the ladder's cap each
    // round; fake timers move Date.now with the same call.
    let guard = 0
    while (guard++ < 200) {
      vi.advanceTimersByTime(4000)
      const s = last(sockets)
      s.handlers.onClose(false) // never opened
      if (last(host.statuses).kind === 'dropped') break
    }
    expect(guard).toBeLessThan(200) // it ended before the guard ran out
    const final = host.statuses.find((s) => s.kind === 'dropped')
    expect(final).toEqual({ kind: 'dropped', reason: 'idle_timeout' })
    expect(host.sessionGone).toBeGreaterThan(0)
    // And it stays ended: no socket opens after the grace.
    const count = sockets.length
    vi.advanceTimersByTime(60_000)
    expect(sockets).toHaveLength(count)
  })

  it('a socket that never opens is closed at the bound', () => {
    const { driver, sockets } = boot()
    driver.attach('a', { afterCreate: false })
    vi.advanceTimersByTime(TERMINAL_WS_OPEN_MS - 1)
    expect(sockets[0].closed).toBe(false)
    vi.advanceTimersByTime(1)
    expect(sockets[0].closed).toBe(true) // onClose then drives the ladder
  })

  it('an open that lands cancels the open timer', () => {
    const { driver, sockets } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    vi.advanceTimersByTime(TERMINAL_WS_OPEN_MS * 2)
    expect(sockets[0].closed).toBe(false)
  })

  it('a never-opened kept session recreates rather than reconnects', () => {
    const { driver, sockets, host } = boot()
    driver.attach('a', { afterCreate: false, recreateOnFail: true })
    sockets[0].handlers.onClose(true)
    expect(host.recreates).toBe(1)
    expect(host.statuses.filter((s) => s.kind === 'unavailable')).toEqual([])
    expect(sockets).toHaveLength(1) // the driver waits; the surface starts the new one
  })

  it('the web first-attach ladder retries once, then calls it network', () => {
    const ladder = (attempt: number) => (attempt === 0 ? 500 : null)
    const { driver, sockets, host } = boot({ firstAttachRetry: ladder })
    driver.attach('fresh', { afterCreate: true })
    sockets[0].handlers.onClose(true)
    expect(last(host.statuses)).toEqual({ kind: 'reconnecting' })
    vi.advanceTimersByTime(500)
    expect(sockets).toHaveLength(2)
    expect(last(sockets).id).toBe('fresh')
    last(sockets).handlers.onClose(true) // attempt 1 → ladder says give up
    expect(last(host.statuses)).toEqual({ kind: 'unavailable', cause: 'network' })
    expect(host.sessionGone).toBe(1)
  })

  it('without the ladder, a never-opened create is unavailable at once (phone)', () => {
    const { driver, sockets, host } = boot()
    driver.attach('fresh', { afterCreate: true })
    sockets[0].handlers.onClose(true)
    expect(last(host.statuses)).toEqual({ kind: 'unavailable', cause: 'network' })
    expect(host.sessionGone).toBe(1)
    vi.advanceTimersByTime(10_000)
    expect(sockets).toHaveLength(1)
  })

  it('a hidden surface reconnects on visibility, not on a timer', () => {
    const { driver, sockets, host } = boot()
    host.hide = true
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    sockets[0].handlers.onClose(false)
    expect(last(host.statuses)).toEqual({ kind: 'reconnecting' })
    vi.advanceTimersByTime(TERMINAL_GRACE_MS * 2)
    expect(sockets).toHaveLength(1) // no ladder ran in the background
    host.hide = false
    driver.reattach('a')
    expect(sockets).toHaveLength(2)
    last(sockets).handlers.onOpen()
    expect(driver.live).toBe(true)
  })

  it('drop() closes without detaching, so the close reconnects', () => {
    // The e2e drop hook's contract (GDK-865): after the hook, the pane must
    // reattach and replay — not sit on a stale handle.
    const { driver, sockets } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    driver.drop()
    expect(sockets[0].closed).toBe(true)
    expect(driver.live).toBe(false)
    vi.advanceTimersByTime(500)
    expect(sockets).toHaveLength(2)
    last(sockets).handlers.onOpen()
    expect(driver.live).toBe(true)
  })

  it('exit and drop-from-serve end the attachment; later closes stay quiet', () => {
    const { driver, sockets, host } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    sockets[0].handlers.onExit(0)
    expect(last(host.statuses)).toEqual({ kind: 'exited', code: 0 })
    expect(host.exits).toEqual([0])
    sockets[0].handlers.onClose(false) // the exit's own close
    vi.advanceTimersByTime(10_000)
    expect(sockets).toHaveLength(1) // ended is not reconnectable

    driver.revive() // the Enter-restart path
    expect(driver.phase).toBe('live')
    sockets[0].handlers.onDropped('idle_timeout')
    expect(last(host.statuses)).toEqual({ kind: 'dropped', reason: 'idle_timeout' })
    expect(host.droppeds).toEqual(['idle_timeout'])
  })

  it('end() kills without letting the socket read as a drop', () => {
    const { driver, sockets, host } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    driver.end('closed')
    expect(last(host.statuses)).toEqual({ kind: 'dropped', reason: 'closed' })
    const painted = host.statuses.length
    sockets[0].handlers.onClose(false) // the kill's own close arrives late
    expect(host.statuses).toHaveLength(painted)
    vi.advanceTimersByTime(10_000)
    expect(sockets).toHaveLength(1)
  })

  it('markUnavailable() records a failed create for the keystroke gate', () => {
    // The surface owns the create POST and its (richer) status; the driver
    // owns the phase the Enter-restart gate reads. Without this, a pane
    // whose create failed still reads live and swallows the Enter.
    const { driver } = boot()
    expect(driver.phase).toBe('live')
    driver.markUnavailable()
    expect(driver.phase).toBe('unavailable')
    driver.revive()
    expect(driver.phase).toBe('live')
  })

  it('send obeys the phase gate, not the open state', () => {
    // The originals gated keystrokes on "socket && phase live" — the open
    // state is the transport's business (a queued ws send is its call), so
    // the driver forwards exactly what those gates forwarded.
    const { driver, sockets } = boot()
    driver.attach('a', { afterCreate: false })
    driver.send(new Uint8Array([1]))
    sockets[0].handlers.onOpen()
    driver.send(new Uint8Array([2]))
    sockets[0].handlers.onExit(0)
    driver.send(new Uint8Array([3]))
    expect(sockets[0].sent.map((b) => Array.from(b))).toEqual([[1], [2]])
  })

  it('dispose() ends timers, closes the socket, and refuses later attaches', () => {
    const { driver, sockets } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    sockets[0].handlers.onClose(false)
    driver.dispose()
    expect(sockets[0].closed).toBe(true)
    const count = sockets.length
    vi.advanceTimersByTime(60_000)
    expect(sockets).toHaveLength(count)
    driver.attach('b', { afterCreate: false })
    expect(sockets).toHaveLength(count)
  })

  it('on open: live, status none, one resize from the measured size', () => {
    const { driver, sockets, host } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    // detach() announced false first — the attached flag clears when a
    // previous attachment ends, exactly as both panes' detachSocket did.
    expect(host.lives).toEqual([false, true])
    expect(last(host.statuses)).toEqual({ kind: 'none' })
    expect(host.attached).toBe(1)
    expect(sockets[0].resized).toEqual([[100, 30]])
  })

  it('an unmeasurable pane ships no size at all (GDK-1154)', () => {
    const { driver, sockets } = boot({ measurable: () => false })
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    vi.advanceTimersByTime(3000) // settle ticks all fire
    expect(sockets[0].resized).toEqual([])
  })

  it('the resize cache dedupes; a new socket is told again', () => {
    const { driver, sockets, host } = boot()
    driver.attach('a', { afterCreate: false })
    sockets[0].handlers.onOpen()
    driver.resizeNow() // same size: no second send
    driver.scheduleFit()
    driver.scheduleFit()
    vi.advanceTimersByTime(100) // coalesced into one check
    expect(sockets[0].resized).toEqual([[100, 30]])
    host.size = { cols: 120, rows: 40 }
    driver.scheduleFit()
    vi.advanceTimersByTime(100)
    expect(sockets[0].resized).toEqual([[100, 30], [120, 40]])
    // Reconnect: the new socket has been told nothing, so it is told now —
    // even though the size did not change in between.
    sockets[0].handlers.onClose(false)
    vi.advanceTimersByTime(500)
    last(sockets).handlers.onOpen()
    expect(last(sockets).resized).toEqual([[120, 40]])
  })

  it('measureForCreate tells the POST, and the socket is told again at open', () => {
    // The create path advances the cache so a *later* resizeNow between
    // create and open is deduped against what the POST already said. The
    // socket itself is still told once on open — the originals sent an
    // unconditional resize there, covering a size that changed between
    // POST and socket, and onOpen empties the cache to keep that.
    const { driver, sockets } = boot()
    expect(driver.measureForCreate()).toEqual({ cols: 100, rows: 30 })
    driver.attach('a', { afterCreate: false })
    driver.resizeNow() // between create and open: deduped against the POST
    expect(sockets[0].resized).toEqual([])
    sockets[0].handlers.onOpen()
    expect(sockets[0].resized).toEqual([[100, 30]])
  })
})
