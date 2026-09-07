/*
 * First-view decision at boot.
 *
 * Priority: URL view params > hosted-demo landing > last-used view
 * (localStorage) > own group preset > first-run my-work (identified, with
 * assigned work) > all-open. A first run on one's own site (no saved view —
 * onboarding just ended) opens on "my work" when there is a signed-in account
 * with open issues — the contributor's first screen before any sentence is
 * added (G4, THEORY.md "Two stances") — and on the open pool otherwise; from
 * the second run the last view wins. Until 2026-09-07 the anonymous fallback
 * was the Epic breakdown built-in (GDK-100); that view left the sidebar with
 * GDK-1493 (a layout of the open pool that only means something on a site
 * with a hierarchy), so the fallback is the pool itself. The hosted demo
 * keeps an epic-grouped landing as its own config — the fixture has epics
 * and the grouped list is the demo's first frame — which is a demo choice,
 * not a built-in view. The decision is pure; App supplies URL/storage/
 * identity and applies the resulting config.
 */

import { emptyConfig, parseConfig, type ViewConfig } from './view-config'

export interface StartupViewInput {
  urlHasViewParam: boolean
  hostedDemo: boolean
  /** The hosted demo's landing config (demoStartupConfig); undefined = fall through. */
  demoView: ViewConfig | undefined
  lastViewKey: string | null
  teamGroupEnabled: boolean
  group: string | null
  /** auth/me answered with an identity (App waits for authChecked first). */
  identified: boolean
  /** The my-work built-in's config, when the catalog has one. */
  myWork: ViewConfig | undefined
  /** Open issues assigned to this identity (person-match), computed by App. */
  myWorkCount: number
}

export type StartupDecision = { kind: 'keep-url' } | { kind: 'apply'; config: ViewConfig }

function allOpenConfig(): ViewConfig {
  const c = emptyConfig()
  c.filters.status_category = ['new', 'inprogress']
  return c
}

/**
 * The hosted demo's first frame: the open pool grouped by epic. Not a
 * built-in view (GDK-1493) — the demo fixture has epics and the sectioned
 * list is the richest first frame, so the demo opens on it; a real site
 * falls through to its own rules below.
 */
export function demoStartupConfig(): ViewConfig {
  const c = allOpenConfig()
  c.display.group_by = 'epic'
  return c
}

function groupPresetConfig(group: string): ViewConfig {
  const c = emptyConfig()
  c.filters.team_group = [group]
  c.filters.status_category = ['new', 'inprogress']
  return c
}

export function decideStartupView(input: StartupViewInput): StartupDecision {
  if (input.urlHasViewParam) return { kind: 'keep-url' }

  if (input.hostedDemo && input.demoView) {
    return { kind: 'apply', config: input.demoView }
  }

  if (input.lastViewKey) {
    return { kind: 'apply', config: parseConfig(new URLSearchParams(input.lastViewKey)) }
  }

  if (input.teamGroupEnabled && input.group) {
    return { kind: 'apply', config: groupPresetConfig(input.group) }
  }

  // First run (nothing above matched): the contributor's own work is the
  // question a first screen should answer — urgent-first, before any
  // sentence is added (G4). Zero assigned work has nothing to show, and an
  // anonymous reader has no "mine", so both fall through to the open pool.
  // The group preset stays above this — personalization beats the generic
  // default (GDK-100).
  if (input.identified && input.myWork && input.myWorkCount > 0) {
    return { kind: 'apply', config: input.myWork }
  }

  return { kind: 'apply', config: allOpenConfig() }
}

export function readLastViewKey(storageKey: string): string | null {
  try {
    return localStorage.getItem(storageKey)
  } catch {
    return null
  }
}

export function applyStartupView(
  input: StartupViewInput,
  applyConfig: (config: ViewConfig) => void,
): void {
  const decision = decideStartupView(input)
  if (decision.kind === 'apply') applyConfig(decision.config)
}

/**
 * What the list's viewKey effect does on this tick.
 *
 * wait:         boot view not applied yet — do not reset, do not accept keys
 * mark-ready:   this tick *is* the boot commit — flip keysReady, replay holds
 * same-view:    keysReady, but viewKey did not change (scroller bind, etc.)
 * reset-cursor: a user (or post-boot) view change — intended reset
 *
 * Owned here so a future view source that writes the hash during boot cannot
 * invent another meaning; IssueList is the only caller.
 */
export type StartupViewTick = 'wait' | 'mark-ready' | 'same-view' | 'reset-cursor'

export function startupViewTick(
  startupViewApplied: boolean,
  keysReady: boolean,
  viewKey = '',
  lastHandledViewKey = '',
): StartupViewTick {
  if (!startupViewApplied) return 'wait'
  if (!keysReady) return 'mark-ready'
  if (viewKey === lastHandledViewKey) return 'same-view'
  return 'reset-cursor'
}
