/*
 * GDK-9 — the settings surface-gating gate.
 *
 * The complaint the issue records is that each settings item decides for
 * itself whether it appears on `serve`, `desktop` or `hosted`, so the three
 * surfaces drift apart silently. The inventory says the drift has one real
 * shape, not many:
 *
 *   - `hosted` never reaches this dialog at all. App.svelte mounts
 *     SettingsDialog under `hasServer()`, which is false exactly on the
 *     hosted snapshot (lib/config.ts). That is why the tab filter takes a
 *     `desktop: boolean` rather than a GadakSurface — not a missing third
 *     case, a surface that is excluded one layer up. Test 4 pins it; if that
 *     mount gate ever goes, the boolean becomes wrong and this file says so.
 *   - Surface-conditional VISIBILITY inside the dialog is tab-level, and one
 *     registry already owns it: DESKTOP_ONLY_SETTINGS_TABS in
 *     lib/integrations.ts, read through visibleSettingsTabs(). Nothing
 *     measured that the registry still matches the tabs that actually need a
 *     desktop server, and nothing stopped a row inside a tab from growing its
 *     own surface branch. Those two holes are what this file closes.
 *
 * Both sides are read from their own owner, as in lib/palette-coverage.test.ts:
 * the registry from lib/integrations.ts, and the need for it from each tab
 * component's own source — does it fetch a `/desktop/*` route, which exists on
 * the desktop app's mux and nowhere else. There is deliberately no
 * hand-written tab list here; the ids come from SETTINGS_TABS.
 *
 * Exemptions are typed and every entry carries its reason, so an undocumented
 * one cannot be added.
 */
import { existsSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { SETTINGS_TABS, type SettingsTab } from '../../lib/settings-tabs'
import { visibleSettingsTabs } from '../../lib/integrations'

const HERE = dirname(fileURLToPath(import.meta.url))
const DIALOG = join(HERE, 'SettingsDialog.svelte')
const APP = join(HERE, '../../App.svelte')

/** Tab id → its panel component file, by the convention the dialog imports. */
function componentFile(tab: SettingsTab): string {
  const pascal = tab.charAt(0).toUpperCase() + tab.slice(1)
  return join(HERE, `${pascal}Tab.svelte`)
}

function read(path: string): string {
  return readFileSync(path, 'utf8')
}

/**
 * The tabs the registry hides off the desktop, derived rather than restated:
 * a tab is desktop-only iff `visibleSettingsTabs` drops it when desktop=false.
 * Reading it this way means the test cannot disagree with the function the
 * dialog itself calls.
 */
function registryDesktopOnly(): SettingsTab[] {
  const onServe = new Set<string>(visibleSettingsTabs(SETTINGS_TABS, false))
  return SETTINGS_TABS.filter((t) => !onServe.has(t))
}

const DESKTOP_ROUTE = /['"`]\/desktop\//

/**
 * A tab whose panel talks to a route only the desktop app's mux serves.
 *
 * One hop through the panel's own relative imports, because that is where the
 * routes actually live: IntegrationsTab paints what lib/integrations.ts
 * fetches and never names `/desktop/integrations` itself. Reading only the
 * .svelte file would call that tab serve-safe, which is the exact inversion
 * this gate exists to catch.
 */
function usesDesktopRoute(tab: SettingsTab): boolean {
  const file = componentFile(tab)
  if (!existsSync(file)) return false
  const src = read(file)
  if (DESKTOP_ROUTE.test(src)) return true
  for (const m of src.matchAll(/from\s+'(\.[^']+)'/g)) {
    const spec = m[1]
    const base = join(dirname(file), spec)
    for (const cand of [base, `${base}.ts`, `${base}.svelte`]) {
      if (existsSync(cand) && statSync(cand).isFile()) {
        if (DESKTOP_ROUTE.test(read(cand))) return true
        break
      }
    }
  }
  return false
}

/**
 * Tabs that need no desktop server yet are hidden off the desktop anyway, or
 * the reverse. Empty today on purpose: every entry would be a surface the
 * registry and the code disagree about, and the reason is what makes that
 * disagreement reviewable instead of invisible.
 */
const TAB_GATING_EXEMPT: Partial<Record<SettingsTab, string>> = {}

/* ── Row-level surface branching ──
 *
 * A row that decides its own visibility from the surface is the defect this
 * issue names, one layer down from the tabs. The check is a two-hop taint:
 * names bound to a surface predicate, then names derived from those, then any
 * `{#if}` / `{:else if}` condition that mentions one.
 */
const SURFACE_CALLS = /\b(surface\(\)|isDesktop\(\)|isHostedDemo\(\)|hasServer\(\))/

function tainted(src: string): string[] {
  const names = new Set<string>()
  // const X = surface() === 'desktop'  /  const X = $derived(<uses a tainted name>)
  for (let pass = 0; pass < 3; pass++) {
    for (const m of src.matchAll(/\b(?:const|let)\s+([A-Za-z_$][\w$]*)\s*=\s*([^\n]*)/g)) {
      const [, name, rhs] = m
      if (names.has(name)) continue
      if (SURFACE_CALLS.test(rhs) || [...names].some((n) => new RegExp(`\\b${n}\\b`).test(rhs))) {
        names.add(name)
      }
    }
  }
  return [...names]
}

function surfaceGatedBlocks(src: string): string[] {
  const names = tainted(src)
  const out: string[] = []
  for (const m of src.matchAll(/\{[#:](?:else )?if\s+([^}]*)\}/g)) {
    const cond = m[1]
    if (SURFACE_CALLS.test(cond) || names.some((n) => new RegExp(`\\b${n}\\b`).test(cond))) {
      out.push(cond.trim())
    }
  }
  return out
}

/**
 * Rows allowed to branch their own visibility on the surface. The key is the
 * component file; the value names the row and why the registry cannot own it.
 */
const ROW_BRANCH_EXEMPT: Record<string, string> = {
  'FeaturesTab.svelte':
    'GDK-349 browser-Notification toggle: hidden only where the HOST fires a real OS ' +
    'notification (macOS/Linux), which is runtime.osNotifySupported, not the surface — ' +
    'Windows desktop keeps the row. A surface registry cannot express it.',
}

describe('GDK-9: the desktop-only tab registry matches the tabs that need a desktop server', () => {
  test('every tab that fetches a /desktop/ route is registered desktop-only', () => {
    const registered = new Set<string>(registryDesktopOnly())
    const missing = SETTINGS_TABS.filter(
      (t) => usesDesktopRoute(t) && !registered.has(t) && !TAB_GATING_EXEMPT[t],
    )
    expect(
      missing,
      `these tabs call /desktop/* but are offered on serve, where that route 404s: ${missing.join(', ')}`,
    ).toEqual([])
  })

  test('no tab is hidden off the desktop without needing a desktop route', () => {
    const stale = registryDesktopOnly().filter((t) => !usesDesktopRoute(t) && !TAB_GATING_EXEMPT[t])
    expect(
      stale,
      `hidden on serve but nothing there needs the desktop mux — drop it from the registry or exempt it: ${stale.join(', ')}`,
    ).toEqual([])
  })

  test('every tab id resolves to a panel component the dialog imports', () => {
    // Without this, a renamed or added tab would silently be scanned as
    // "no desktop route" and pass the two checks above for free.
    const dialog = read(DIALOG)
    for (const tab of SETTINGS_TABS) {
      const file = componentFile(tab)
      expect(existsSync(file), `no panel component for settings tab '${tab}' at ${file}`).toBe(true)
      const base = file.slice(file.lastIndexOf('/') + 1)
      expect(dialog, `SettingsDialog does not import ${base}`).toContain(`./${base}`)
    }
  })

  test('every tab exemption names a reason', () => {
    for (const [tab, reason] of Object.entries(TAB_GATING_EXEMPT)) {
      expect(reason?.trim(), `exemption for '${tab}' must say why`).toBeTruthy()
    }
  })
})

describe('GDK-9: hosted is excluded at the mount, not per row', () => {
  test('App mounts SettingsDialog only when there is a server to ask', () => {
    // This is what makes `visibleSettingsTabs(…, desktop: boolean)` correct
    // rather than a GadakSurface with a missing third case. If the dialog
    // ever renders on the hosted snapshot, the boolean has to become the
    // three-valued surface and every tab needs a hosted answer.
    expect(read(APP)).toMatch(/serverSettingsOpen && hasServer\(\)/)
  })
})

describe('GDK-9: no settings row restates a surface condition', () => {
  for (const tab of SETTINGS_TABS) {
    const file = componentFile(tab)
    if (!existsSync(file)) continue
    const base = file.slice(file.lastIndexOf('/') + 1)
    test(`${base} branches visibility on the registry, not on the surface`, () => {
      const found = surfaceGatedBlocks(read(file))
      if (ROW_BRANCH_EXEMPT[base]) {
        expect(
          found.length,
          `${base} is exempt but no longer has a surface-gated block — delete the exemption`,
        ).toBeGreaterThan(0)
        return
      }
      expect(
        found,
        `${base} hides or shows a row from the surface itself. Tab-level visibility belongs to ` +
          `DESKTOP_ONLY_SETTINGS_TABS (lib/integrations.ts); a row that genuinely cannot be ` +
          `expressed there needs an entry in ROW_BRANCH_EXEMPT saying why. Blocks: ${found.join(' | ')}`,
      ).toEqual([])
    })
  }

  test('every row exemption names a reason', () => {
    for (const [file, reason] of Object.entries(ROW_BRANCH_EXEMPT)) {
      expect(reason?.trim(), `exemption for '${file}' must say why`).toBeTruthy()
    }
  })
})
