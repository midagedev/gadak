/*
 * First-view decision at boot.
 *
 * Priority: URL view params > last-used view (localStorage) > own group preset
 * > all-open. Hosted demo lands on the built-in Epic breakdown when that
 * preset exists. The decision is pure; App supplies URL/storage/identity and
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
