/*
 * The app's one wall clock (GDK-1584). Three relative-time surfaces used to
 * run their own setInterval — IssueRow 60s per row, FreshnessChip 10s,
 * FavoritesNav 30s — and the row one scaled with the virtual window: at
 * --spacing-row 36px with OVERSCAN 8, a 900px list mounted ~41 of them.
 * Consumers subscribe here instead; the interval exists only while at least
 * one subscriber does, and there is never more than one.
 *
 * The tick is the shortest consumer's cadence (10s, FreshnessChip's):
 * relativeTime is minute-granular below the hour, so a 1s tick would reprint
 * the same string 59 times. Coarser consumers (IssueRow's minute) simply
 * re-read a fresher clock than they strictly need — the derivation prints
 * the same string either way, and one broadcast beats 41 private timers.
 *
 * Data polls must NOT ride this clock: a poll owns its cadence (a 2s sync
 * status cannot live on a 10s tick) and belongs in a store module — see the
 * gate in no-component-interval.test.ts, which keeps component intervals
 * out entirely.
 */

/** Coarse tick — see above. Exported so the gate and tests name one value. */
export const WALL_CLOCK_TICK_MS = 10_000

type Listener = () => void

const listeners = new Set<Listener>()
let timer: ReturnType<typeof setInterval> | null = null

/** Inspectable next to the uiFocusPoll / browseNative dataset idiom: how
 *  many surfaces currently ride the clock (0 means no interval at all). */
function mirrorStats(): void {
  if (typeof document === 'undefined') return
  document.documentElement.dataset.wallClockSubs = `${listeners.size}`
}

function sync(): void {
  if (listeners.size > 0) {
    if (timer === null) {
      timer = setInterval(() => {
        for (const listener of [...listeners]) listener()
      }, WALL_CLOCK_TICK_MS)
    }
  } else if (timer !== null) {
    clearInterval(timer)
    timer = null
  }
  mirrorStats()
}

/**
 * Coarse wall-clock subscription: `listener` fires once per
 * WALL_CLOCK_TICK_MS while subscribed. The first subscriber starts the
 * interval; the last unsubscribe clears it. Bump your own tick/now state in
 * the listener — the derivation that re-reads it stays where it renders.
 */
export function subscribeWallClock(listener: Listener): () => void {
  listeners.add(listener)
  sync()
  return () => {
    listeners.delete(listener)
    sync()
  }
}

/** Subscriber count, for probes and the dataset mirror above. */
export function wallClockSubscribers(): number {
  return listeners.size
}
