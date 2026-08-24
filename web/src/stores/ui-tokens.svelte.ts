/*
 * Reactive snapshot of the user's color overrides (GDK-786/791).
 *
 * lib/user-tokens.ts applies the stylesheet; this file holds the dataColors
 * half in a $state snapshot so every chip that reads labelColor() /
 * typeColor() / statusCategoryColor() re-renders when a settings write lands
 * — no reload. The snapshot is written from exactly one place
 * (applyUserTokens, after its defensive parse), so lookups can trust the
 * values; the startsWith('#') guard is only a belt against a stale snapshot
 * shape from an older bundle in the same tab.
 */

import { setUiTokensListener, type UiTokenDoc } from '../lib/user-tokens'

let snapshot: UiTokenDoc | null = $state(null)

/** Single writer — user-tokens.ts applyUserTokens(), via the listener hook. */
export function setUiTokenSnapshot(doc: UiTokenDoc | null): void {
  snapshot = doc
}

setUiTokensListener(setUiTokenSnapshot)

function lookup(family: 'label' | 'type' | 'status', key: string): string | undefined {
  const hex = snapshot?.dataColors?.[family]?.[key]
  return typeof hex === 'string' && hex.startsWith('#') ? hex : undefined
}

/** User ink for one label (the label text is the key). */
export function labelColor(label: string): string | undefined {
  return lookup('label', label)
}

/** User ink for one issue type, keyed by issue_type_id — never the display name. */
export function typeColor(issueTypeId: string | null | undefined): string | undefined {
  if (!issueTypeId) return undefined
  return lookup('type', issueTypeId)
}

/** User ink for a status category (new|inprogress|done). */
export function statusCategoryColor(category: string): string | undefined {
  return lookup('status', category)
}

function tint(hex: string | undefined): string | undefined {
  // Chip tint ≈18% alpha: the user's color reads at a glance while the theme
  // text on top keeps its own contrast. 3-digit hex is expanded first.
  if (!hex) return undefined
  const v = hex.length === 4 ? `#${hex[1]}${hex[1]}${hex[2]}${hex[2]}${hex[3]}${hex[3]}` : hex
  return `${v}2e`
}

/** Inline background for a label chip, undefined = default chip style. */
export function labelChipTint(label: string): string | undefined {
  return tint(labelColor(label))
}

/** Inline background for a type chip, undefined = default chip style. */
export function typeChipTint(issueTypeId: string | null | undefined): string | undefined {
  return tint(typeColor(issueTypeId))
}
