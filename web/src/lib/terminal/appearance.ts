/*
 * The terminal dock's own appearance (GDK-1357): dark or follow.
 *
 * The dock paints the dark palette under every app theme by default. xterm's
 * ANSI palette is a dark-ground palette — measured on paper (#f4efe4), 9 of
 * its 16 colours fall under 3.0:1 (yellow 2.19, bright yellow 1.08, bright
 * white 1.01) — and a band of ink under a paper page is also the seam the
 * dock otherwise lacks. "follow" hands the dock the app's palette instead.
 *
 * Same three-part shape as the theme preference: a localStorage mirror so
 * the attribute is on <html> before the pane opens, an attribute the CSS
 * keys on (app.css scopes the dark block under
 * `:root:not([data-terminal-theme='follow']) .terminal-dock`, so an unset
 * attribute is dark), and a write-through to appearance.terminal in the
 * settings document, which the server owns.
 */

import type { GadakSettings } from '../api'
import type { MessageKey } from '../i18n'
import { queueSettingsPatch } from '../settings-write'
import { themeStorageKey } from '../storage'

export const TERMINAL_APPEARANCES = [
  { name: 'dark', labelKey: 'settings.terminalAppearanceDark' satisfies MessageKey },
  { name: 'follow', labelKey: 'settings.terminalAppearanceFollow' satisfies MessageKey },
] as const

export type TerminalAppearance = (typeof TERMINAL_APPEARANCES)[number]['name']

/** The attribute on <html>. The stylesheet keys on its absence == dark. */
export const TERMINAL_THEME_ATTR = 'data-terminal-theme'

/** Missing or unknown values are dark (the product default). */
export function parseTerminalAppearance(raw: unknown): TerminalAppearance {
  return raw === 'follow' ? 'follow' : 'dark'
}

function storageKey(): string {
  return `${themeStorageKey()}:terminal`
}

export function readTerminalAppearance(): TerminalAppearance {
  try {
    return parseTerminalAppearance(localStorage.getItem(storageKey()))
  } catch {
    return 'dark'
  }
}

function writeTerminalAppearance(pref: TerminalAppearance): void {
  try {
    localStorage.setItem(storageKey(), pref)
  } catch {
    /* private mode / unavailable */
  }
}

export function applyTerminalAppearance(pref: TerminalAppearance): void {
  if (typeof document === 'undefined') return
  document.documentElement.setAttribute(TERMINAL_THEME_ATTR, pref)
}

export function setTerminalAppearance(pref: TerminalAppearance): void {
  writeTerminalAppearance(pref)
  applyTerminalAppearance(pref)
}

/** Boot: before the app mounts, so the pane never opens under the wrong ground. */
export function applyTerminalAppearanceAtBoot(): void {
  applyTerminalAppearance(readTerminalAppearance())
}

/** Instant local apply, then write appearance.terminal through — keeping the
 *  sibling theme field, which the same block carries. */
export async function persistTerminalAppearance(pref: TerminalAppearance): Promise<void> {
  setTerminalAppearance(pref)
  try {
    await queueSettingsPatch((current) => ({
      ...current,
      appearance: { ...current.appearance, terminal: pref },
    }))
  } catch {
    // Same road as the theme picker: the look already applied locally,
    // the server did not take it — say so once instead of failing silently.
    try {
      const { write } = await import('../../stores/write.svelte')
      const { t } = await import('../i18n')
      write.toast(t('theme.savedLocally'), 'info')
    } catch {
      console.warn('gadak: terminal appearance saved locally only')
    }
  }
}

/** After first paint, from the settings GET the theme hydrate already made:
 *  the server's value wins over the local mirror. A document without the key
 *  (older server) leaves the mirror alone. */
export function hydrateTerminalAppearance(settings: GadakSettings): void {
  const remote = settings.appearance?.terminal
  if (remote === undefined) return
  const want = parseTerminalAppearance(remote)
  if (want !== readTerminalAppearance()) setTerminalAppearance(want)
}
