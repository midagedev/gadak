/*
 * First-view decision at boot.
 *
 * Priority: URL view params > last-used view (localStorage) > own group preset
 * > first-run Epic breakdown > all-open. Hosted demo lands on the built-in
 * Epic breakdown before even the last-used view. A first run on one's own
 * site (no saved view — onboarding just ended) gets the same Epic breakdown
 * instead of a bare Jira replica (GDK-100); from the second run the last
 * view wins. The decision is pure; App supplies URL/storage/identity and
 * applies the resulting config.
 */

import { emptyConfig, parseConfig, type ViewConfig } from './view-config'

export interface StartupViewInput {
  urlHasViewParam: boolean
  hostedDemo: boolean
  epicBreakdown: ViewConfig | undefined
  lastViewKey: string | null
  teamGroupEnabled: boolean
  group: string | null
}

export type StartupDecision = { kind: 'keep-url' } | { kind: 'apply'; config: ViewConfig }

function allOpenConfig(): ViewConfig {
  const c = emptyConfig()
  c.filters.status_category = ['new', 'inprogress']
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

  if (input.hostedDemo && input.epicBreakdown) {
    return { kind: 'apply', config: input.epicBreakdown }
  }

  if (input.lastViewKey) {
    return { kind: 'apply', config: parseConfig(new URLSearchParams(input.lastViewKey)) }
  }

  if (input.teamGroupEnabled && input.group) {
    return { kind: 'apply', config: groupPresetConfig(input.group) }
  }

  // First run (nothing above matched): show the product's question, not a
  // Jira replica. The group preset stays above this — personalization beats
  // the generic default (GDK-100).
  if (input.epicBreakdown) {
    return { kind: 'apply', config: input.epicBreakdown }
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
