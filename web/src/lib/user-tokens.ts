/*
 * User color overrides (GDK-786/791): CSS-variable injection + the FOUC cache.
 *
 * The server merges config.json's `ui` block into a final per-palette CSS
 * variable map (`vars`) plus per-data-key inks (`dataColors`) — see
 * internal/server settings.go webConfig(). This module owns the browser half:
 *
 *   - applyUserTokens(): one <style data-gadak-user-tokens> element whose
 *     rules mirror app.css's palette cascade (:root, :root[data-theme],
 *     prefers-color-scheme), so palette switches and OS-scheme changes need
 *     no MutationObserver — CSS re-resolves on its own. Unlayered rules
 *     outrank app.css's @layer blocks, which is what makes an override win
 *     without touching app.css.
 *   - the localStorage cache under gadak:user-tokens (workspace-scoped like
 *     the theme key), written on every server apply and read by the blocking
 *     boot script in index.html so a customized user never sees the default
 *     palette flash before the bundle lands.
 *
 * The server already refuses non-hex values at write time; the hex/var
 * filters here are the client-side belt for hand-edited config.json files
 * and downgrades (a newer config naming tokens this build locks or renamed).
 * Those degrade to advisories, never a broken boot.
 */

import { THEME_STORAGE_KEY, themeStorageKeyFromPath } from './storage'

/** Boot-mirror prefix, workspace-scoped exactly like THEME_STORAGE_KEY. */
export const USER_TOKENS_STORAGE_KEY = 'gadak:user-tokens'

/** Marks the style element both the boot script and this module install. */
export const USER_TOKENS_STYLE_ATTR = 'data-gadak-user-tokens'

export type UiDataFamily = 'label' | 'type' | 'status'

export interface UiTokenWarning {
  token: string
  rule: string
  message: string
}

/** The `ui` slice of config.json, as the server merged it. */
export interface UiTokenDoc {
  /** palette → CSS variable → hex. Overrides only; bases stay in app.css. */
  vars: Record<string, Record<string, string>>
  /** family → key → hex. label.* is the label text, type.* an issue_type_id,
   * status.* a status_category (new|inprogress|done). */
  dataColors: Partial<Record<UiDataFamily, Record<string, string>>>
  /** Load-time advisories (unknown token after a downgrade, …). */
  warnings?: UiTokenWarning[]
}

const HEX_RE = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/
const CSS_VAR_RE = /^--color-[a-z0-9-]+$/i
const PALETTE_RE = /^[a-z0-9-]{1,32}$/

/** #rgb/#rrggbb only — same rule the server's write gate applies. */
export function isHexColor(v: unknown): v is string {
  return typeof v === 'string' && HEX_RE.test(v)
}

/** Derive the boot-mirror key from a pathname. `/` and `/w/oss` stay distinct. */
export function userTokensStorageKeyFromPath(pathname: string): string {
  return USER_TOKENS_STORAGE_KEY + themeStorageKeyFromPath(pathname).slice(THEME_STORAGE_KEY.length)
}

/** Active boot-mirror key for this page. Safe when `window` is missing (tests). */
export function userTokensStorageKey(): string {
  const path =
    typeof window !== 'undefined' && window.location ? window.location.pathname : '/'
  return userTokensStorageKeyFromPath(path)
}

function declarations(map: unknown): string {
  if (!map || typeof map !== 'object') return ''
  let out = ''
  for (const [name, value] of Object.entries(map as Record<string, unknown>)) {
    if (CSS_VAR_RE.test(name) && isHexColor(value)) out += `${name}:${value};`
  }
  return out
}

/**
 * The CSS text for one vars map, mirroring app.css's cascade: light at :root,
 * every other palette behind its data-theme attribute (dark included — an
 * explicit dark preference must not depend on the media query), and dark again
 * inside prefers-color-scheme for system users. Empty input → '' (no style).
 */
export function buildUserTokenStyles(vars: unknown): string {
  if (!vars || typeof vars !== 'object') return ''
  const map = vars as Record<string, unknown>
  let css = ''
  const light = declarations(map.light)
  if (light) css += `:root{${light}}`
  for (const palette of Object.keys(map).sort()) {
    if (palette === 'light' || !PALETTE_RE.test(palette)) continue
    const d = declarations(map[palette])
    if (d) css += `:root[data-theme='${palette}']{${d}}`
  }
  const dark = declarations(map.dark)
  if (dark) css += `@media (prefers-color-scheme: dark){:root:not([data-theme='light']){${dark}}}`
  return css
}

function parseDataColors(raw: unknown): UiTokenDoc['dataColors'] {
  const out: UiTokenDoc['dataColors'] = {}
  if (!raw || typeof raw !== 'object') return out
  for (const family of ['label', 'type', 'status'] as const) {
    const m = (raw as Record<string, unknown>)[family]
    if (!m || typeof m !== 'object') continue
    const clean: Record<string, string> = {}
    for (const [k, v] of Object.entries(m as Record<string, unknown>)) {
      if (isHexColor(v)) clean[k] = v
    }
    out[family] = clean
  }
  return out
}

/** Defensive read of an untrusted `ui` document (cache, older servers). */
export function parseUiDoc(raw: unknown): UiTokenDoc | null {
  if (!raw || typeof raw !== 'object') return null
  const doc: UiTokenDoc = {
    vars: {},
    dataColors: parseDataColors((raw as Record<string, unknown>).dataColors),
  }
  const vars = (raw as Record<string, unknown>).vars
  if (vars && typeof vars === 'object') {
    for (const [palette, m] of Object.entries(vars as Record<string, unknown>)) {
      if (!PALETTE_RE.test(palette)) continue
      const clean: Record<string, string> = {}
      for (const [name, value] of Object.entries(
        (m ?? {}) as Record<string, unknown>,
      )) {
        if (CSS_VAR_RE.test(name) && isHexColor(value)) clean[name] = value
      }
      doc.vars[palette] = clean
    }
  }
  const warnings = (raw as Record<string, unknown>).warnings
  if (Array.isArray(warnings)) {
    doc.warnings = warnings.filter(
      (w): w is UiTokenWarning =>
        !!w && typeof w === 'object' && typeof (w as UiTokenWarning).message === 'string',
    )
  }
  return doc
}

/** Blocking-boot half: what index.html's hand-spelled script re-derives. */
export function readCachedUserTokens(): UiTokenDoc | null {
  try {
    return parseUiDoc(JSON.parse(localStorage.getItem(userTokensStorageKey()) ?? 'null'))
  } catch {
    return null
  }
}

function writeUserTokensCache(doc: UiTokenDoc | null): void {
  try {
    if (doc && Object.keys(doc.vars).length > 0) {
      localStorage.setItem(userTokensStorageKey(), JSON.stringify(doc))
    } else {
      localStorage.removeItem(userTokensStorageKey())
    }
  } catch {
    /* private mode / unavailable */
  }
}

/**
 * Snapshot sink for the dataColors lookups (stores/ui-tokens.svelte). This
 * module stays runes-free so the plain unit project can import it — the
 * store registers itself here at module init, which keeps the dependency
 * one-directional (store → lib).
 */
export type UiTokensListener = (doc: UiTokenDoc | null) => void

let listener: UiTokensListener | null = null

export function setUiTokensListener(cb: UiTokensListener): void {
  listener = cb
}

/**
 * Install (or replace) the override stylesheet, publish the snapshot for the
 * dataColors lookups, and refresh the boot cache. Called with the server doc
 * after loadConfig() and again whenever the ui-focus poll sees configVersion
 * move — never a reload, CSS variables only.
 */
export function applyUserTokens(doc: UiTokenDoc | null | undefined): void {
  const clean = doc ? parseUiDoc(doc) : null
  listener?.(clean)
  writeUserTokensCache(clean)
  for (const w of clean?.warnings ?? []) console.warn(`gadak: ui: ${w.message}`)
  if (typeof document === 'undefined') return
  const css = clean ? buildUserTokenStyles(clean.vars) : ''
  let el = document.querySelector<HTMLStyleElement>(`style[${USER_TOKENS_STYLE_ATTR}]`)
  if (!el) {
    el = document.createElement('style')
    el.setAttribute(USER_TOKENS_STYLE_ATTR, '')
    document.head.appendChild(el)
  }
  // The boot script may have installed the element before the bundle; either
  // way there is exactly one, and this is the only writer after boot.
  el.textContent = css
}
