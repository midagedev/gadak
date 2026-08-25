/*
 * Dashboard frame protocol (GDK-781/782) — pure helpers, no DOM.
 *
 * The parent→frame channel is one message shape ({type:'data', …}); the
 * frame→parent channel is a whitelist of exactly two verbs: `refresh`
 * (re-run the datasources) and `open` (navigate the app itself to a `#/…`
 * hash). This module owns both so the component stays markup and the
 * whitelist stays auditable in one place: adding a verb here is a security
 * decision, not a feature toggle.
 */

/** The only message types a dashboard frame may send to the parent. */
export const FRAME_MESSAGE_TYPES = ['refresh', 'open'] as const
export type FrameMessage = { type: 'refresh' } | { type: 'open'; hash: string }

/** Ceiling on an `open` hash. A longer one is dropped whole, not truncated. */
export const OPEN_HASH_MAX = 2048

// A hash a person clicked is '#' followed by printable characters. Raw C0
// bytes have no business in a fragment; refusing them keeps the eventual
// location.hash write free of control characters without re-encoding.
function isOpenHashShape(hash: string): boolean {
  if (!hash.startsWith('#')) return false
  for (let i = 1; i < hash.length; i++) {
    const c = hash.charCodeAt(i)
    if (c < 0x20 || c === 0x7f) return false
  }
  return true
}

/**
 * Parse one message event's data into a whitelisted frame message. `null` for
 * everything else — unknown types, non-objects, arrays (typeof [] is 'object'),
 * missing/extra fields. Callers log-and-drop nulls; nothing here ever throws,
 * because a hostile frame sending garbage must not break the host's listener.
 *
 * `open` is the one verb with a payload, and the payload is a fragment on
 * purpose: a `#…` string is same-document by construction, so the frame can
 * never use it to name an outside URL for the parent to navigate to. Only
 * the validated `hash` survives parsing — no other field rides along.
 */
export function parseFrameMessage(data: unknown): FrameMessage | null {
  if (typeof data !== 'object' || data === null || Array.isArray(data)) return null
  const type = (data as { type?: unknown }).type
  if (typeof type !== 'string') return null
  if (!(FRAME_MESSAGE_TYPES as readonly string[]).includes(type)) return null
  if (type === 'open') {
    const hash = (data as { hash?: unknown }).hash
    if (typeof hash !== 'string') return null
    if (hash.length > OPEN_HASH_MAX) return null
    if (!isOpenHashShape(hash)) return null
    return { type: 'open', hash }
  }
  return { type: 'refresh' }
}

/** One datasource result as pushed to the frame. */
export interface DataMessage {
  type: 'data'
  name: string
  columns: string[]
  rows: unknown[][]
  truncated: boolean
  warning?: string
}

/** The data message for a datasource whose execution failed. */
export function dataMessageFromError(name: string, message: string): DataMessage {
  // The failure rides the same channel as a successful result: the frame is
  // already written to read `warning` on a row set (the server sets it for
  // the display-name trap), so an author sees a failed card, not a missing one.
  return { type: 'data', name, columns: [], rows: [], truncated: false, warning: message }
}

/** The slice of the timer API the throttle needs — injectable for tests (vitest runs node, no window). */
export interface ThrottleTimer {
  setTimeout(fn: () => void, ms: number): unknown
}

/**
 * Flood defense for the frame→parent verbs: the frame may force a host-side
 * action at most once per `minIntervalMs`; bursts coalesce into one trailing
 * run. A hostile or buggy dashboard spamming verbs cannot turn the host into
 * a query pump (`refresh`) or a navigation/history churn (`open`).
 *
 * The returned runner takes a callback and returns whether the call ran
 * immediately (true) or was coalesced/pending (false). `flush()` runs a
 * pending trailing call immediately (used on teardown so no coalesced request
 * is lost).
 */
export function createVerbThrottle(minIntervalMs: number, timer: ThrottleTimer) {
  let last = 0
  let pending: (() => void) | null = null
  let scheduled = false

  function runTrailing() {
    scheduled = false
    const fn = pending
    pending = null
    if (fn) {
      last = Date.now()
      fn()
    }
  }

  return {
    run(fn: () => void): boolean {
      const now = Date.now()
      if (now - last >= minIntervalMs) {
        last = now
        fn()
        return true
      }
      pending = fn
      if (!scheduled) {
        scheduled = true
        timer.setTimeout(runTrailing, minIntervalMs - (now - last))
      }
      return false
    },
    /** True when a trailing run is still scheduled. */
    get pending(): boolean {
      return scheduled
    },
    /** Run a pending trailing call now (teardown path). */
    flush(): void {
      if (!scheduled) return
      runTrailing()
    },
  }
}
