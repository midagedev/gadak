import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  LAYOUT_DETAIL_MIN_PX,
  LAYOUT_LIST_MIN_PX,
  LAYOUT_SIDEBAR_PX,
  VIEWPORT_DOCKED_MIN_PX,
  applyLayoutDimOverrides,
  effectiveLayout,
  layoutTokenStyle,
  subscribeViewportRegime,
} from './viewport-regime'

const HERE = dirname(fileURLToPath(import.meta.url))

describe('viewport docked floor (GDK-766)', () => {
  test('track mins sum to VIEWPORT_DOCKED_MIN_PX (±0)', () => {
    expect(LAYOUT_SIDEBAR_PX + LAYOUT_LIST_MIN_PX + LAYOUT_DETAIL_MIN_PX).toBe(
      VIEWPORT_DOCKED_MIN_PX,
    )
    expect(VIEWPORT_DOCKED_MIN_PX).toBe(1100)
  })
})

describe('layout dim overrides (GDK-842 chunk 3)', () => {
  afterEach(() => {
    applyLayoutDimOverrides(null)
    vi.unstubAllGlobals()
  })

  test('effectiveLayout ships the defaults and keeps dockedMin a sum', () => {
    const eff = effectiveLayout()
    expect(eff).toEqual({ sidebar: 272, listMin: 390, detailMin: 438, dockedMin: 1100 })
    expect(eff.dockedMin).toBe(eff.sidebar + eff.listMin + eff.detailMin)
  })

  test('a sidebar override moves the docked floor and the token style with it', () => {
    applyLayoutDimOverrides({ '--layout-sidebar': '300px' })
    const eff = effectiveLayout()
    expect(eff.sidebar).toBe(300)
    expect(eff.dockedMin).toBe(300 + 390 + 438)
    expect(layoutTokenStyle()).toContain('--layout-sidebar:300px')
    expect(layoutTokenStyle()).toContain('--layout-docked-min:1128px')
  })

  test('malformed or non-positive values fall back to the defaults', () => {
    // The number is deliberate: an untrusted doc can carry a non-string, and
    // the cast is the test-side way to say so past the Record<string,string> type.
    applyLayoutDimOverrides({
      '--layout-sidebar': 'bogus',
      '--layout-list-min': '0px',
      '--layout-detail-min': 438,
    } as unknown as Record<string, string>)
    expect(effectiveLayout()).toEqual({
      sidebar: 272,
      listMin: 390,
      detailMin: 438,
      dockedMin: 1100,
    })
  })

  test('clearing the overrides restores the shipped floor', () => {
    applyLayoutDimOverrides({ '--layout-sidebar': '300px' })
    expect(effectiveLayout().dockedMin).toBe(1128)
    applyLayoutDimOverrides(null)
    expect(effectiveLayout()).toEqual({
      sidebar: 272,
      listMin: 390,
      detailMin: 438,
      dockedMin: 1100,
    })
  })

  test('a changed floor re-subscribes matchMedia under the new threshold', () => {
    const mqls: Array<{
      media: string
      matches: boolean
      listeners: Set<() => void>
      addEventListener: (t: string, fn: () => void) => void
      removeEventListener: (t: string, fn: () => void) => void
    }> = []
    vi.stubGlobal('window', {
      matchMedia: vi.fn((media: string) => {
        const listeners = new Set<() => void>()
        const mq = {
          media,
          matches: false,
          listeners,
          addEventListener: (_t: string, fn: () => void) => void listeners.add(fn),
          removeEventListener: (_t: string, fn: () => void) => void listeners.delete(fn),
        }
        mqls.push(mq)
        return mq
      }),
    })

    const seen: string[] = []
    const unsub = subscribeViewportRegime((r) => seen.push(r))
    expect(mqls).toHaveLength(1)
    expect(mqls[0].media).toContain('1100px')
    expect(mqls[0].listeners.size).toBe(1)
    expect(seen).toHaveLength(1) // subscription fires once immediately

    applyLayoutDimOverrides({ '--layout-sidebar': '300px' })
    expect(mqls).toHaveLength(2) // new floor ⇒ a new MediaQueryList…
    expect(mqls[0].listeners.size).toBe(0) // …old one released…
    expect(mqls[1].media).toContain('1128px')
    expect(mqls[1].listeners.size).toBe(1)
    expect(seen).toHaveLength(2) // …and subscribers re-notified with the new regime

    unsub()
    expect(mqls[1].listeners.size).toBe(0)
  })
})

/*
 * GDK-1585: the overlay chrome is declarative. applyOverlayChrome — the
 * post-render DOM walk that set inert/role/aria-modal by class and testid —
 * is gone; the same DOM result must now come from props on the three frames
 * App renders. The runtime half (the list really goes inert, Esc closes) is
 * measured by e2e/narrow-viewport.spec.ts (GDK-201); this pins that the
 * declarative half exists and the walker is not reborn — a mount-based unit
 * is not possible in the runes-free node project (see effect-assigns-state).
 */
describe('overlay chrome is props on the shell (GDK-1585)', () => {
  const root = join(HERE, '..', '..')
  const SRC = {
    regime: readFileSync(join(root, 'src/lib/viewport-regime.ts'), 'utf8'),
    sidebar: readFileSync(join(root, 'src/components/shell/Sidebar.svelte'), 'utf8'),
    main: readFileSync(join(root, 'src/components/shell/MainColumn.svelte'), 'utf8'),
    panel: readFileSync(join(root, 'src/components/shell/RightPanel.svelte'), 'utf8'),
    app: readFileSync(join(root, 'src/App.svelte'), 'utf8'),
  }

  test('the imperative walker is gone from viewport-regime', () => {
    expect(SRC.regime.includes('applyOverlayChrome')).toBe(false)
    expect(SRC.regime.includes('focus-trap')).toBe(false)
  })

  test('the background frames carry inert as a prop', () => {
    expect(SRC.sidebar).toMatch(/inert\?: boolean/)
    expect(SRC.sidebar).toMatch(/\{inert\}/)
    expect(SRC.main).toMatch(/inert\?: boolean/)
    expect(SRC.main).toMatch(/\{inert\}/)
  })

  test('the panel frame is a dialog while modal: role, aria-modal, trap', () => {
    expect(SRC.panel).toMatch(/modal\?: boolean/)
    expect(SRC.panel).toMatch(/role=\{modal \? 'dialog' : undefined\}/)
    expect(SRC.panel).toMatch(/aria-modal=\{modal \? 'true' : undefined\}/)
    expect(SRC.panel).toMatch(/use:trapWhileModal=\{modal\}/)
  })

  test('App drives the chrome with the overlay verdict alone', () => {
    expect(SRC.app).toMatch(/<Sidebar inert=\{overlayModal\}>/)
    expect(SRC.app).toMatch(/<MainColumn inert=\{overlayModal\}>/)
    expect(SRC.app).toMatch(/<RightPanel open=\{panelOpen\} modal=\{overlayModal\}>/)
    expect(SRC.app.includes('applyOverlayChrome')).toBe(false)
  })
})

/*
 * GDK-696: the live regime has one owner. App.svelte and DetailPanel.svelte
 * each held a `$state` copy fed by their own subscribeViewportRegime call —
 * duplicated state kept honest only by both subscriptions firing on the same
 * matchMedia change. The value is now module state in
 * viewport-regime.svelte.ts (which owns the one subscription); a component
 * re-declaring the copy or subscribing for itself is the duplication coming
 * back, and this sweep is what sees it.
 *
 * The state-invalid-export rule says the owner cannot be a reassigned
 * `export let` — Svelte forbids exporting state that is written later — so
 * the owner is a `$state` object mutated through `.regime`. A component
 * re-declaring its copy would spell regime and $state on one line, whatever
 * the binding is named; the one-line window is the contract, the owner files
 * are exempt.
 */
describe('the live regime is module state, once (GDK-696)', () => {
  const root = join(HERE, '..', '..')
  const OWNERS = new Set([
    'src/lib/viewport-regime.svelte.ts', // the state and its subscription
    'src/lib/viewport-regime.ts', // the subscription plumbing itself
    'src/lib/viewport-regime.test.ts', // this gate
  ])

  test('no component holds a second copy of the regime', () => {
    const files = readdirSync(join(root, 'src'), { recursive: true, encoding: 'utf8' }).filter(
      (f) => f.endsWith('.svelte') || f.endsWith('.ts'),
    )
    expect(files.length, 'the sweep found no sources to read').toBeGreaterThan(100)

    const offenders: string[] = []
    for (const rel of files) {
      if (OWNERS.has(`src/${rel}`)) continue
      const source = readFileSync(join(root, 'src', rel), 'utf8')
      if (/[Rr]egime[^\n]{0,80}=\s*\$state|\$state[^\n]{0,80}[Rr]egime/.test(source)) {
        offenders.push(rel)
      }
      if (source.includes('subscribeViewportRegime')) offenders.push(rel)
    }
    expect(offenders).toEqual([])
  })
})
