/*
 * The settings dialog's form model — one editable object between GET settings/
 * and PUT settings/.
 *
 * Most of it is `GadakSettings` verbatim, and the tabs bind straight into it.
 * The exceptions are the shapes a form can edit and a config file cannot:
 *  - records become ordered row arrays (a key being typed is briefly empty, and
 *    two rows may hold the same key until one of them is finished),
 *  - list fields that render as one line keep their comma-separated text,
 *  - an interval is a preset plus a custom seconds entry.
 *
 * Every normalization the PUT depends on lives in `toSettings` — trimming,
 * dropping empty keys, and the two conditionally present fields. Keeping it in
 * one pure function is what makes the payload checkable without a browser.
 */

import type { GadakSettings, SettingsFieldSpec, UITokens } from '../../lib/api'
import type { GadakFeatures } from '../../lib/config'
import type { MessageKey } from '../../lib/i18n'

export interface GroupRow {
  key: string
  label: string
  color: string
}
export interface ProductRow {
  group: string
  key: string
  label: string
}
export interface RuleRow {
  group: string
  projects: string
  labels: string
  components: string
}
export interface MemberRow {
  email: string
  name: string
  display_name: string
  group: string
  department: string
  job_role: string
  jira_account_id: string
  avatar_url: string
}

export type FeatureFlags = Record<keyof GadakFeatures, boolean>

export interface SettingsDraft {
  /** Picker selection; the field of record once the site's project list arrives. */
  projects: string[]
  /** Manual entry, used only while the site's project list is unreachable. */
  projectsText: string
  /**
   * The `confluence` key is absent from the response unless the source is
   * configured, and PUTting it while it is off is rejected — so its presence is
   * the section's on/off switch.
   */
  confluenceConfigured: boolean
  /** Pending on/off, applied on Save like every other field on that tab. */
  confluenceOn: boolean
  spaces: string[]
  staleText: string
  qaDashboardUrl: string
  features: FeatureFlags
  syncPreset: number
  syncCustomText: string
  reconcilePreset: number
  reconcileCustomText: string
  groups: GroupRow[]
  products: ProductRow[]
  rules: RuleRow[]
  groupQuery: string
  members: MemberRow[]
  bodyFieldsText: string
  /** Discovered field specs. */
  specs: SettingsFieldSpec[]
  /**
   * specsTouched gates the PUT: absence preserves discovery output on the
   * server, so an untouched section costs nothing.
   */
  specsTouched: boolean
  specsSupported: boolean
  /** Terminal tab (GDK-1357). Scrollback as typed; '' = the server default. */
  terminalScrollbackText: string
  terminalCursorBlink: boolean
  /** Font size in px as typed ('' = default) and the family stack — the two
   *  `ui.tokens` leaves the tab edits (type.terminal, fonts.mono-terminal). */
  terminalFontSizeText: string
  terminalFontFamily: string
  /**
   * The `ui` block as GET sent it. PUT replaces that block whole, so the tab
   * changes two leaves on this copy and sends the copy — only when touched:
   * absence keeps the stored overrides (the omit-to-preserve rule).
   */
  ui: GadakSettings['ui']
  uiTouched: boolean
}

/** Preset values in seconds. 0 = server default; -1 = custom number entry. */
export const SYNC_PRESETS: { value: number; labelKey: MessageKey }[] = [
  { value: 0, labelKey: 'settings.intervalDefault' },
  { value: 30, labelKey: 'settings.intervalPreset30s' },
  { value: 60, labelKey: 'settings.intervalPreset1m' },
  { value: 300, labelKey: 'settings.intervalPreset5m' },
  { value: 900, labelKey: 'settings.intervalPreset15m' },
  { value: -1, labelKey: 'settings.intervalCustom' },
]
export const RECONCILE_PRESETS: { value: number; labelKey: MessageKey }[] = [
  { value: 0, labelKey: 'settings.intervalDefault' },
  { value: 3600, labelKey: 'settings.intervalPreset1h' },
  { value: 21600, labelKey: 'settings.intervalPreset6h' },
  { value: 86400, labelKey: 'settings.intervalPreset24h' },
  { value: -1, labelKey: 'settings.intervalCustom' },
]

const NO_FEATURES: FeatureFlags = {
  feed: false,
  deploy: false,
  qa: false,
  teamGroups: false,
}

const splitCsv = (s: string): string[] => s.split(',').map((v) => v.trim()).filter(Boolean)
const joinCsv = (a: string[] | undefined): string => (a ?? []).join(', ')

function pickPreset(sec: number, presets: { value: number }[]): number {
  if (!sec || sec <= 0) return 0
  return presets.some((p) => p.value === sec) ? sec : -1
}

function resolveInterval(preset: number, customText: string): number {
  if (preset === 0) return 0
  if (preset > 0) return preset
  const n = Number(customText)
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : 0
}

/**
 * Server response (or the JSON textarea) → form model.
 *
 * `features` carries the flags forward: a hand-written JSON that omits one must
 * not silently switch it off, which is what merging into the current set buys.
 */
export function toDraft(s: GadakSettings, features: FeatureFlags = NO_FEATURES): SettingsDraft {
  const syncSec = s.syncIntervalSec ?? 0
  const syncPreset = pickPreset(syncSec, SYNC_PRESETS)
  const recSec = s.reconcileIntervalSec ?? 0
  const reconcilePreset = pickPreset(recSec, RECONCILE_PRESETS)
  const groupKeys = [
    ...new Set([...Object.keys(s.groupLabels ?? {}), ...Object.keys(s.groupColors ?? {})]),
  ]
  return {
    projects: [...(s.projects ?? [])],
    projectsText: joinCsv(s.projects),
    confluenceConfigured: s.confluence !== undefined,
    confluenceOn: s.confluence !== undefined,
    spaces: [...(s.confluence?.spaces ?? [])],
    staleText: String(s.staleThresholdHours ?? 72),
    qaDashboardUrl: s.qaDashboardUrl ?? '',
    features: { ...features, ...(s.features ?? {}) },
    syncPreset,
    syncCustomText: syncPreset === -1 ? String(syncSec) : '',
    reconcilePreset,
    reconcileCustomText: reconcilePreset === -1 ? String(recSec) : '',
    groups: groupKeys.map((key) => ({
      key,
      label: s.groupLabels?.[key] ?? '',
      color: s.groupColors?.[key] ?? '',
    })),
    products: Object.entries(s.productByGroup ?? {}).map(([group, p]) => ({
      group,
      key: p?.key ?? '',
      label: p?.label ?? '',
    })),
    rules: (s.groupRules ?? []).map((r) => ({
      group: r.group ?? '',
      projects: joinCsv(r.projects),
      labels: joinCsv(r.labels),
      components: joinCsv(r.components),
    })),
    groupQuery: s.groupQuery ?? '',
    members: (s.members ?? []).map((m) => ({
      email: m.email ?? '',
      name: m.name ?? '',
      display_name: m.display_name ?? '',
      group: m.group ?? '',
      department: m.department ?? '',
      job_role: m.job_role ?? '',
      jira_account_id: m.jira_account_id ?? '',
      avatar_url: m.avatar_url ?? '',
    })),
    bodyFieldsText: joinCsv(s.bodyFields),
    specs: (s.fieldSpecs ?? []).map((sp) => ({ ...sp, ids: [...sp.ids] })),
    specsTouched: false,
    specsSupported: s.fieldSpecs !== undefined,
    terminalScrollbackText: s.terminal?.scrollback ? String(s.terminal.scrollback) : '',
    terminalCursorBlink: s.terminal?.cursorBlink ?? false,
    terminalFontSizeText: (s.ui?.tokens?.type?.terminal ?? '').replace(/px$/, ''),
    terminalFontFamily: s.ui?.tokens?.fonts?.['mono-terminal'] ?? '',
    ui: s.ui,
    uiTouched: false,
  }
}

/**
 * The GET `ui` block with the Terminal tab's two leaves written onto a copy.
 * An empty entry deletes its leaf; an axis left empty is dropped so the
 * document never carries `type: {}`. Anything else in the block is carried
 * verbatim — PUT replaces the block, so a partial send would erase it.
 */
export function withTerminalTokens(
  ui: GadakSettings['ui'],
  fontSizeText: string,
  fontFamily: string,
): NonNullable<GadakSettings['ui']> {
  const tokens: UITokens = { ...(ui?.tokens ?? {}) }
  const type = { ...(tokens.type ?? {}) }
  const fonts = { ...(tokens.fonts ?? {}) }
  // Both arrive from <input type="number"> bindings, which hand back a
  // number once the field holds one — so coerce before trimming.
  const size = String(fontSizeText ?? '').trim()
  if (size) type.terminal = /^\d+(\.\d+)?$/.test(size) ? `${size}px` : size
  else delete type.terminal
  const family = String(fontFamily ?? '').trim()
  if (family) fonts['mono-terminal'] = family
  else delete fonts['mono-terminal']
  if (Object.keys(type).length) tokens.type = type
  else delete tokens.type
  if (Object.keys(fonts).length) tokens.fonts = fonts
  else delete tokens.fonts
  const out = { ...(ui ?? {}) }
  if (Object.keys(tokens).length) out.tokens = tokens
  else delete out.tokens
  return out
}

/** The form's state before the first response lands. */
export function emptyDraft(): SettingsDraft {
  return toDraft({})
}

/**
 * Form model → PUT payload (full replace). Do not send runtime/site.
 *
 * `projectsPickerReady` decides which of the two project fields is the record:
 * the picker once the site answered, the manual text box while it has not.
 */
export function toSettings(d: SettingsDraft, projectsPickerReady: boolean): GadakSettings {
  const groupLabels: Record<string, string> = {}
  const groupColors: Record<string, string> = {}
  for (const row of d.groups) {
    const key = row.key.trim()
    if (!key) continue
    if (row.label.trim()) groupLabels[key] = row.label.trim()
    if (row.color.trim()) groupColors[key] = row.color.trim()
  }
  const productByGroup: Record<string, { key: string; label: string }> = {}
  for (const row of d.products) {
    const group = row.group.trim()
    if (group) productByGroup[group] = { key: row.key.trim(), label: row.label.trim() }
  }
  const hours = Number(d.staleText)
  const scrollback = Number(d.terminalScrollbackText)
  return {
    projects: projectsPickerReady ? [...d.projects] : splitCsv(d.projectsText),
    // `enabled` is what lets the Sources tab turn the source on at all; the
    // server rejects a bare `spaces` while it is off. Sent whenever the section
    // was reachable, so switching it off is a save like any other.
    //
    // Picking a space is the request to mirror it — same rule the dialog used
    // to enforce with an $effect that wrote confluenceOn (GDK-692). Spaces
    // ride along: enabled-from-spaces with spaces:[] would mean "every team
    // space", which is the silent-discard's opposite foot-gun.
    confluence: {
      enabled: d.confluenceOn || d.spaces.length > 0,
      spaces: d.confluenceOn || d.spaces.length > 0 ? [...d.spaces] : [],
    },
    staleThresholdHours: Number.isFinite(hours) && hours > 0 ? hours : 72,
    syncIntervalSec: resolveInterval(d.syncPreset, d.syncCustomText),
    reconcileIntervalSec: resolveInterval(d.reconcilePreset, d.reconcileCustomText),
    qaDashboardUrl: d.qaDashboardUrl.trim(),
    features: { ...d.features },
    groupLabels,
    groupColors,
    productByGroup,
    groupRules: d.rules
      .filter((r) => r.group.trim())
      .map((r) => ({
        group: r.group.trim(),
        projects: splitCsv(r.projects),
        labels: splitCsv(r.labels),
        components: splitCsv(r.components),
      })),
    groupQuery: d.groupQuery,
    members: d.members
      .filter((m) => m.email.trim())
      .map((m) => ({ ...m, email: m.email.trim() })),
    bodyFields: splitCsv(d.bodyFieldsText),
    // Only when touched: the server treats absence as "keep discovery output".
    ...(d.specsTouched && d.specsSupported
      ? { fields: d.specs.filter((r) => r.alias.trim() && r.ids.length > 0) }
      : {}),
    // Display fields only — the server merges them onto the stored block, so
    // shell and workingDir (not in this document, GDK-1069) are untouched.
    terminal: {
      scrollback: Number.isFinite(scrollback) && scrollback > 0 ? Math.floor(scrollback) : 0,
      cursorBlink: d.terminalCursorBlink,
    },
    ...(d.uiTouched ? { ui: withTerminalTokens(d.ui, d.terminalFontSizeText, d.terminalFontFamily) } : {}),
  }
}
