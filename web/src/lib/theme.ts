/*
 * Theme registry and tri-state preference (light / dark / system).
 *
 * Adding a palette: a token block in app.css (both the :root[data-theme] rule
 * and the color-scheme rule outside @layer), an entry in THEMES, a label in
 * the i18n catalog (messages/), and the name + --boot-* shell in index.html's boot script.
 * boot-theme.test.ts and tools/theme-check.mjs hold every one of those; the
 * picker iterates THEME_MODES and must not be edited for a new theme.
 *
 * data-theme is the override; prefers-color-scheme is the default.
 * system → no data-theme attribute (CSS media query applies dark tokens).
 * light / dark → data-theme wins over the OS.
 */

import { getSettings, putSettings } from './api'
import { isHostedDemo } from './config'
import type { MessageKey } from './i18n'
import { themeStorageKey } from './storage'

export const THEMES = [
  { name: 'light', labelKey: 'theme.light' satisfies MessageKey },
  { name: 'dark', labelKey: 'theme.dark' satisfies MessageKey },
  { name: 'ink', labelKey: 'theme.ink' satisfies MessageKey },
  { name: 'ember', labelKey: 'theme.ember' satisfies MessageKey },
] as const

export const THEME_MODES = [
  { name: 'system', labelKey: 'theme.system' satisfies MessageKey },
  ...THEMES,
] as const

export type ThemeName = (typeof THEMES)[number]['name']
export type ThemePreference = (typeof THEME_MODES)[number]['name']

const PREFERENCE_SET: ReadonlySet<string> = new Set(THEME_MODES.map((m) => m.name))

export function isThemePreference(value: unknown): value is ThemePreference {
  return typeof value === 'string' && PREFERENCE_SET.has(value)
}

/** Missing or unknown stored values are system (the product default). */
export function parseThemePreference(raw: string | null | undefined): ThemePreference {
  return isThemePreference(raw) ? raw : 'system'
}

/**
 * Attribute written on <html>. null means remove it so the media query
 * can see :root:not([data-theme='light']).
 */
export function dataThemeAttr(pref: ThemePreference): ThemeName | null {
  return pref === 'system' ? null : pref
}

export function readThemePreference(): ThemePreference {
  try {
    return parseThemePreference(localStorage.getItem(themeStorageKey()))
  } catch {
    return 'system'
  }
}

function writeThemePreference(pref: ThemePreference): void {
  try {
    localStorage.setItem(themeStorageKey(), pref)
  } catch {
    /* private mode / unavailable */
  }
}

export function applyThemePreference(pref: ThemePreference): void {
  if (typeof document === 'undefined') return
  const attr = dataThemeAttr(pref)
  if (attr === null) document.documentElement.removeAttribute('data-theme')
  else document.documentElement.setAttribute('data-theme', attr)
}

export function setThemePreference(pref: ThemePreference): void {
  writeThemePreference(pref)
  applyThemePreference(pref)
}

/** Blocking boot: must run before first paint of the app shell. */
export function applyThemeAtBoot(): void {
  applyThemePreference(readThemePreference())
}

/**
 * Instant local apply, then read-modify-write this workspace's settings
 * document. Shared by the settings picker and the ⌘K action. Does not
 * touch the settings-dialog draft — GET is the server's current state.
 * Hosted demo has no settings server; local only.
 */
export function persistThemePreference(pref: ThemePreference): Promise<void> {
  setThemePreference(pref)
  const run = persistChain.then(
    () => writeThroughTheme(pref),
    () => writeThroughTheme(pref),
  )
  persistChain = run.catch(() => {
    /* keep the chain alive after a failure */
  })
  return run
}

let persistChain: Promise<void> = Promise.resolve()

async function writeThroughTheme(pref: ThemePreference): Promise<void> {
  if (isHostedDemo()) return
  try {
    const current = await getSettings()
    await putSettings({ ...current, appearance: { theme: pref } })
  } catch {
    try {
      const { write } = await import('../stores/write.svelte')
      const { t } = await import('./i18n')
      write.toast(t('theme.savedLocally'), 'info')
    } catch {
      console.warn('gadak: theme saved locally only')
    }
  }
}

/**
 * After first paint: server appearance wins, including "system". Missing
 * `appearance` (old server) or a failed GET leaves the local mirror alone.
 */
export async function hydrateThemeFromServer(): Promise<void> {
  if (isHostedDemo()) return
  const localBefore = readThemePreference()
  try {
    const settings = await getSettings()
    if (!settings.appearance) return
    const remote = parseThemePreference(settings.appearance.theme)
    if (readThemePreference() !== localBefore) return
    if (remote !== localBefore) setThemePreference(remote)
  } catch {
    console.warn('gadak: theme settings unavailable; keeping local preference')
  }
}
