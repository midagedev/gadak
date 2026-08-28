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
 * GDK-842 (dim wave) adds the dimension axis: the server also merges a
 * palette-agnostic `dims` map (--spacing-row → 44px) that lands in ONE :root
 * rule after the palette cascade — lengths have no palette variants. The
 * same apply also recouples the JS geometry owners (rowMetrics cache,
 * viewport-regime's docked floor) so a dims change repaints and re-windows
 * in the same tick, not just the next reload.
 *
 * GDK-896 R4 adds the fonts axis the same way: a palette-agnostic `fonts`
 * map (--font-mono-terminal → "Menlo, monospace") that joins the same single
 * :root rule. The grammar mirror (isFontStack) is load-bearing, not belt
 * polish: the value is spliced into this element as text, and the grammar is
 * what makes the splice inject-proof. The style half is CSS-only — an open
 * xterm canvas does not re-read font CSS, so a change lands on the next
 * terminal open (see renderer.ts fontFamily).
 *
 * The server already refuses bad values at write time; the hex/var and dim
 * filters here are the client-side belt for hand-edited config.json files
 * and downgrades (a newer config naming tokens this build locks or renamed).
 * Those degrade to advisories, never a broken boot.
 */

import { invalidateRowMetrics } from './row-metrics'
import { THEME_STORAGE_KEY, themeStorageKeyFromPath } from './storage'
import { applyLayoutDimOverrides } from './viewport-regime'

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
  /** CSS variable → length/line-height. Palette-agnostic (--spacing-row →
   * "44px"); the server's UIDimensionVars already filtered unknown/locked
   * tokens. Optional because documents predate the dim wave; parseUiDoc
   * always sets it (empty when the user overrides nothing). */
  dims?: Record<string, string>
  /** CSS variable → font stack. Palette-agnostic (--font-mono-terminal →
   * "Menlo, monospace"); the server's UIFontVars already dropped unknown
   * names and grammar failures. Optional like dims for the same reason. */
  fonts?: Record<string, string>
  /** Load-time advisories (unknown token after a downgrade, …). */
  warnings?: UiTokenWarning[]
}

const HEX_RE = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/
const CSS_VAR_RE = /^--color-[a-z0-9-]+$/i
const PALETTE_RE = /^[a-z0-9-]{1,32}$/
// Dim names: the three overridable axes' prefixes. Deliberately a separate
// branch from CSS_VAR_RE — a --color-* name inside dims is dropped, and a
// --spacing-* name inside vars is dropped, so the two maps can't launder
// each other's shapes past the value filters.
const DIM_NAME_RE = /^--(?:spacing|layout|text)-[a-z0-9-]+$/i
// Dim values, mirroring the server's dimcheck.ParseValue exactly: px is a
// positive integer-or-one-decimal length; a *--line-height name instead takes
// a unitless one-or-two-decimal value. The name owns the unit — a bare "42"
// can never pass as a spacing, and "14px" never as a line-height.
const DIM_PX_RE = /^[0-9]+(\.[0-9])?px$/
const DIM_UNITLESS_RE = /^[0-9]+\.[0-9]{1,2}$/
// Font names own their kind exactly like the dim prefixes: a --font-* name
// inside dims is dropped, a --spacing-*/--color-* name inside fonts is
// dropped, so neither map can launder the other's shape.
const FONT_NAME_RE = /^--font-[a-z0-9-]+$/i
// Font values, mirroring the server's validFontStack exactly (GDK-896 R4):
// comma-separated families, 1..8 of them, at most 256 characters in total,
// each a bare CSS identifier or a quoted name whose inner text is
// [A-Za-z0-9 _-]. Everything that could carry CSS structure is outside
// these alphabets by construction.
const FONT_IDENT_RE = /^[A-Za-z][A-Za-z0-9-]{0,63}$/
const FONT_QUOTED_INNER_RE = /^[A-Za-z0-9 _-]{1,64}$/

/** #rgb/#rrggbb only — same rule the server's write gate applies. */
export function isHexColor(v: unknown): v is string {
  return typeof v === 'string' && HEX_RE.test(v)
}

/** A dim value for this name — the unit is owned by the name (see DIM_*_RE). */
export function isDimValue(name: string, value: unknown): value is string {
  if (typeof value !== 'string') return false
  if (/--line-height$/.test(name)) {
    return DIM_UNITLESS_RE.test(value) && Number.parseFloat(value) > 0
  }
  return DIM_PX_RE.test(value) && Number.parseFloat(value) > 0
}

/** A fonts-axis value — the exact grammar the server write gate refuses on
 *  (see FONT_*_RE). Quote characters must pair; commas split families. */
export function isFontStack(v: unknown): v is string {
  if (typeof v !== 'string' || v.length === 0 || v.length > 256) return false
  const families = v.split(',')
  if (families.length > 8) return false
  for (const raw of families) {
    const f = raw.trim()
    if (f.length >= 2 && (f[0] === "'" || f[0] === '"') && f[f.length - 1] === f[0]) {
      if (!FONT_QUOTED_INNER_RE.test(f.slice(1, -1))) return false
      continue
    }
    if (!FONT_IDENT_RE.test(f)) return false
  }
  return true
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

function dimDeclarations(map: unknown): string {
  if (!map || typeof map !== 'object') return ''
  let out = ''
  for (const [name, value] of Object.entries(map as Record<string, unknown>)) {
    if (DIM_NAME_RE.test(name) && isDimValue(name, value)) out += `${name}:${value};`
  }
  return out
}

function fontDeclarations(map: unknown): string {
  if (!map || typeof map !== 'object') return ''
  let out = ''
  for (const [name, value] of Object.entries(map as Record<string, unknown>)) {
    if (FONT_NAME_RE.test(name) && isFontStack(value)) out += `${name}:${value};`
  }
  return out
}

/**
 * The CSS text for one vars map, mirroring app.css's cascade: light at :root,
 * every other palette behind its data-theme attribute (dark included — an
 * explicit dark preference must not depend on the media query), and dark again
 * inside prefers-color-scheme for system users. Empty input → '' (no style).
 *
 * The dims map (GDK-842) appends ONE palette-agnostic :root rule after the
 * cascade — lengths have no palette variants, and it must not sit inside a
 * data-theme block or a user with an explicit theme would lose the override.
 * The fonts map (GDK-896 R4) joins that same trailing rule: stacks have no
 * palette variants either.
 */
export function buildUserTokenStyles(vars: unknown, dims?: unknown, fonts?: unknown): string {
  let css = ''
  if (vars && typeof vars === 'object') {
    const map = vars as Record<string, unknown>
    const light = declarations(map.light)
    if (light) css += `:root{${light}}`
    for (const palette of Object.keys(map).sort()) {
      if (palette === 'light' || !PALETTE_RE.test(palette)) continue
      const d = declarations(map[palette])
      if (d) css += `:root[data-theme='${palette}']{${d}}`
    }
    const dark = declarations(map.dark)
    if (dark) css += `@media (prefers-color-scheme: dark){:root:not([data-theme='light']){${dark}}}`
  }
  const tail = dimDeclarations(dims) + fontDeclarations(fonts)
  if (tail) css += `:root{${tail}}`
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

/** Dim map, sanitized with the same name/value filters the CSS build applies. */
function parseDims(raw: unknown): UiTokenDoc['dims'] {
  const clean: Record<string, string> = {}
  if (!raw || typeof raw !== 'object') return clean
  for (const [name, value] of Object.entries(raw as Record<string, unknown>)) {
    if (DIM_NAME_RE.test(name) && isDimValue(name, value)) clean[name] = value
  }
  return clean
}

/** Font map, sanitized the same way — names own the kind (FONT_NAME_RE). */
function parseFonts(raw: unknown): UiTokenDoc['fonts'] {
  const clean: Record<string, string> = {}
  if (!raw || typeof raw !== 'object') return clean
  for (const [name, value] of Object.entries(raw as Record<string, unknown>)) {
    if (FONT_NAME_RE.test(name) && isFontStack(value)) clean[name] = value
  }
  return clean
}

/** Defensive read of an untrusted `ui` document (cache, older servers). */
export function parseUiDoc(raw: unknown): UiTokenDoc | null {
  if (!raw || typeof raw !== 'object') return null
  const doc: UiTokenDoc = {
    vars: {},
    dataColors: parseDataColors((raw as Record<string, unknown>).dataColors),
    dims: parseDims((raw as Record<string, unknown>).dims),
    fonts: parseFonts((raw as Record<string, unknown>).fonts),
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
    if (
      doc &&
      (Object.keys(doc.vars).length > 0 ||
        Object.keys(doc.dims ?? {}).length > 0 ||
        Object.keys(doc.fonts ?? {}).length > 0)
    ) {
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
  // GDK-842: the geometry owners must see the change in the same call —
  // rowMetrics caches heights for every scroll frame, and the docked floor
  // lives inside a matchMedia threshold. Both re-derive from the dims map
  // here, before the style install, so a synchronous reader right after
  // this call sees the new geometry. (Runs for color-only changes too: one
  // extra computed-style read is cheaper than tracking which axis moved.)
  invalidateRowMetrics()
  applyLayoutDimOverrides(clean?.dims ?? {})
  if (typeof document === 'undefined') return
  const css = clean ? buildUserTokenStyles(clean.vars, clean.dims, clean.fonts) : ''
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
