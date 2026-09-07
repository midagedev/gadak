/*
 * GDK-1047 gates for the Devices tab (desktop only). Same story as
 * FeaturesTab.test.ts: the unit vitest project is environment 'node' with
 * no svelte plugin, so mounting the component is not a thing this project
 * can do — the render contract is asserted against the source the
 * compiler emits, and everything with real logic (tab visibility) is a
 * genuine unit test against the lib.
 *
 * The one behavior this file owns outright: the Devices tab must NOT exist
 * under gadak serve — neither in the header nor via a pasted
 * `settings=devices` URL. No e2e spec asserts the tab set (settings.spec
 * walks tabs by name), so this is where that absence is pinned.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse } from 'svelte/compiler'
import { describe, expect, test } from 'vitest'
import { SETTINGS_TABS } from '../../lib/settings-tabs'
import { isVisibleSettingsTab, visibleSettingsTabs } from '../../lib/integrations'

const HERE = dirname(fileURLToPath(import.meta.url))
const DEVICES_TAB = join(HERE, 'DevicesTab.svelte')
const SETTINGS_DIALOG = join(HERE, 'SettingsDialog.svelte')
const MESSAGES = join(HERE, '../../lib/i18n/messages')

/*
 * GDK-1474: the render contract below used to be markup literals —
 * /<img\s[^>]*src=\{minted\.qr_png\}/, /<option value="serve">/,
 * /let revealed = \$state\(false\)/. Those pin one spelling of the markup:
 * an attribute reordered, a line wrapped, or a space added breaks the test
 * while the component still does the same thing, and a match inside a
 * comment passes while nothing is wired. The wiring is one tier down —
 * the AST the compiler builds — so it is asserted there, the way
 * IssueRow.test.ts and SearchBox.test.ts already do.
 *
 * Still deliberately regex, further down: the offer-masking boundaries
 * (slice(0, 6) / slice(-4)) and the absence checks (no localStorage, no QR
 * library import). An absence is about the source text, not about a node
 * that exists to be walked to.
 */
type AnyNode = { type: string } & Record<string, unknown>

function isNode(value: unknown): value is AnyNode {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { type?: unknown }).type === 'string'
  )
}

// Template children only, and `attributes` is not among them — an attribute
// is reached from the element that owns it, never walked into blindly.
const CHILD_KEYS = [
  'fragment',
  'nodes',
  'consequent',
  'alternate',
  'body',
  'pending',
  'then',
  'catch',
  'fallback',
] as const

function walkTemplate(node: unknown, visit: (n: AnyNode) => void): void {
  if (Array.isArray(node)) {
    for (const child of node) walkTemplate(child, visit)
    return
  }
  if (!isNode(node)) return
  visit(node)
  for (const key of CHILD_KEYS) walkTemplate(node[key], visit)
}

/** `minted.qr_png` for a non-computed member chain; undefined for anything else. */
function memberPath(node: unknown): string | undefined {
  if (!isNode(node)) return undefined
  if (node.type === 'Identifier') return String(node.name)
  if (node.type === 'MemberExpression' && node.computed === false) {
    const object = memberPath(node.object)
    const property = memberPath(node.property)
    return object && property ? `${object}.${property}` : undefined
  }
  return undefined
}

function attribute(element: AnyNode, name: string): AnyNode | undefined {
  const attrs = element.attributes
  if (!Array.isArray(attrs)) return undefined
  return (attrs as AnyNode[]).find((a) => a.type === 'Attribute' && a.name === name)
}

/** `value="serve"` — a static attribute is Text nodes. Undefined if it interpolates. */
function staticAttribute(element: AnyNode, name: string): string | undefined {
  const value = attribute(element, name)?.value
  if (!Array.isArray(value)) return undefined
  const parts = value as AnyNode[]
  if (!parts.every((n) => n.type === 'Text')) return undefined
  return parts.map((n) => String(n.data)).join('')
}

/** `src={minted.qr_png}` — a single ExpressionTag, returned as its member path. */
function expressionAttribute(element: AnyNode, name: string): string | undefined {
  const value = attribute(element, name)?.value
  if (!isNode(value) || value.type !== 'ExpressionTag') return undefined
  return memberPath(value.expression)
}

function parseComponent(path: string, filename: string) {
  const source = readFileSync(path, 'utf8')
  const ast = parse(source, { modern: true, filename }) as unknown as {
    fragment: unknown
    instance: { content: { body: AnyNode[] } } | null
  }
  const elements: AnyNode[] = []
  walkTemplate(ast.fragment, (n) => {
    if (n.type === 'RegularElement') elements.push(n)
  })
  return { source, elements, script: ast.instance?.content.body ?? [] }
}

/** The `$state(...)` call behind `let <name>`, or undefined if <name> is not rune state. */
function stateCall(script: AnyNode[], name: string): AnyNode | undefined {
  for (const node of script) {
    if (node.type !== 'VariableDeclaration' || !Array.isArray(node.declarations)) continue
    for (const declarator of node.declarations as AnyNode[]) {
      const id = declarator.id
      if (!isNode(id) || id.name !== name) continue
      const init = declarator.init
      if (isNode(init) && init.type === 'CallExpression') {
        const callee = init.callee
        if (isNode(callee) && callee.name === '$state') return init
      }
    }
  }
  return undefined
}

function catalogSource(): string {
  // Copy lives in messages/*.ts ({en,ko,ja} per key), not en.ts/ko.ts/ja.ts.
  return readdirSync(MESSAGES)
    .filter((n) => n.endsWith('.ts'))
    .map((n) => readFileSync(join(MESSAGES, n), 'utf8'))
    .join('\n')
}

describe('GDK-1047 devices tab is desktop-only', () => {
  test('serve hides it from the header (visibleSettingsTabs)', () => {
    expect(visibleSettingsTabs(SETTINGS_TABS, false)).not.toContain('devices')
    expect(visibleSettingsTabs(SETTINGS_TABS, true)).toContain('devices')
  })

  test('serve refuses a pasted settings=devices URL (isVisibleSettingsTab)', () => {
    expect(isVisibleSettingsTab('devices', SETTINGS_TABS, false)).toBe(false)
    expect(isVisibleSettingsTab('devices', SETTINGS_TABS, true)).toBe(true)
  })

  test('the tab sits between integrations and about', () => {
    const tabs = [...SETTINGS_TABS]
    expect(tabs.indexOf('devices')).toBe(tabs.indexOf('integrations') + 1)
    expect(tabs.indexOf('about')).toBe(tabs.indexOf('devices') + 1)
  })
})

describe('GDK-1047 devices tab render contract', () => {
  const { source: src, elements, script } = parseComponent(DEVICES_TAB, 'DevicesTab.svelte')

  test('the QR is an <img> fed by the server data URI, not a client-side renderer', () => {
    const images = elements.filter((e) => e.name === 'img')
    expect(images, 'the QR must render as a plain <img>').toHaveLength(1)
    expect(expressionAttribute(images[0], 'src')).toBe('minted.qr_png')
    // No new npm dependency: nothing imports a QR library.
    expect(src).not.toMatch(/from '[^']*qr/i)
    expect(src).not.toMatch(/import\s+qrcode/)
  })

  test('the offer string is masked until explicitly revealed', () => {
    // Masked rendering: head…tail, never the full string by default.
    expect(src).toMatch(/minted\.offer\.slice\(0, 6\)/)
    expect(src).toMatch(/minted\.offer\.slice\(-4\)/)
    // The full offer renders only behind the revealed flag.
    expect(src).toMatch(/\{revealed \? minted\.offer :/)
    // Reveal is explicit UI state, default hidden.
    const revealed = stateCall(script, 'revealed')
    expect(revealed, 'revealed must be rune-backed component state').toBeDefined()
    expect((revealed?.arguments as AnyNode[]).map((a) => a.value)).toEqual([false])
  })

  test('the credential lives in component state only — no persistence', () => {
    expect(src).not.toMatch(/localStorage/)
    expect(src).not.toMatch(/sessionStorage/)
    expect(src).not.toMatch(/indexedDB/)
  })

  test('the scope select offers serve and origin only — terminal is not minted from a form', () => {
    // TODO(GDK-1474): the better home for this is an exported const in
    // DevicesTab.svelte (`export const MINTABLE_SCOPES = ['serve','origin']`)
    // that the markup maps over and this file imports — the shape
    // SETTINGS_TABS already has above. That needs an edit to the component,
    // which this round does not own; until then the list is read off the AST,
    // which at least survives reformatting.
    const options = elements.filter((e) => e.name === 'option')
    expect(options.map((o) => staticAttribute(o, 'value'))).toEqual(['serve', 'origin'])
  })

  test('the local-routing row (_home) carries no revoke button', () => {
    expect(src).toMatch(/\{#if !isHome\}/)
  })

  test('mint errors map server codes to catalog keys, with a fallback', () => {
    expect(src).toMatch(/MINT_ERROR\[doc\.error \?\? ''\]/)
    expect(src).toMatch(/'settings\.devicesErrFailed'/)
  })
})

describe('GDK-1047 devices tab is wired into the dialog', () => {
  const dialog = readFileSync(SETTINGS_DIALOG, 'utf8')

  test('labeled, shown, and mounted', () => {
    expect(dialog).toMatch(/devices: t\('settings\.tabDevices'\)/)
    expect(dialog).toMatch(/const showDevices = TABS\.some/)
    expect(dialog).toMatch(/tab === 'devices' && showDevices/)
    expect(dialog).toMatch(/<DevicesTab \/>/)
  })
})

describe('GDK-1047 devices copy is complete in every locale', () => {
  const catalog = catalogSource()

  // Every key the component or dialog references, pinned here so a rename
  // fails loudly instead of rendering a raw key.
  const keys = [
    'settings.tabDevices',
    'settings.devicesIntro',
    'settings.devicesLoadFailed',
    'settings.devicesLoading',
    'settings.devicesEmpty',
    'settings.devicesUnavailableNotConfigured',
    'settings.devicesUnavailablePairedAway',
    'settings.devicesColLabel',
    'settings.devicesColScope',
    'settings.devicesColExpires',
    'settings.devicesColState',
    'settings.devicesScopeServe',
    'settings.devicesScopeOrigin',
    'settings.devicesScopeLocalRouting',
    'settings.devicesLabelLabel',
    'settings.devicesScopeLabel',
    'settings.devicesEndpointLabel',
    'settings.devicesEndpointHint',
    'settings.devicesTtlLabel',
    'settings.devicesMint',
    'settings.devicesMintBusy',
    'settings.devicesMinted',
    'settings.devicesLoopbackWarning',
    'settings.devicesOfferLabel',
    'settings.devicesOfferShow',
    'settings.devicesOfferHide',
    'settings.devicesCopyOffer',
    'settings.devicesQrAlt',
    'settings.devicesErrLabelRequired',
    'settings.devicesErrReservedLabel',
    'settings.devicesErrBadScope',
    'settings.devicesErrBadEndpoint',
    'settings.devicesErrBadTtl',
    'settings.devicesErrNoServe',
    'settings.devicesErrLabelExists',
    'settings.devicesErrFailed',
    'settings.devicesRevoke',
    // settings.devicesRevoked was pinned here with no caller — removed from
    // the catalog in the same round (GDK-1474).
    'settings.devicesHomeRowHint',
    'settings.devicesStateActive',
    'settings.devicesStateExpired',
    'settings.devicesStateRevoked',
  ]

  test.each(keys)('%s exists in the catalog', (key) => {
    expect(catalog).toMatch(new RegExp(`'${key.replace(/\./g, '\\.')}':`))
  })

  test('the params the copy interpolates are named, not positional', () => {
    expect(catalog).toMatch(/'settings\.devicesMinted':[\s\S]*?\{label\}[\s\S]*?\{expires\}/)
    expect(catalog).toMatch(/'settings\.devicesErrLabelExists':[\s\S]*?\{label\}/)
  })

  test('every devices entry carries all three locales', () => {
    const src = readFileSync(join(MESSAGES, 'settings.ts'), 'utf8')
    const blocks = src.match(/'settings\.devices[A-Za-z]*':\s*\{[\s\S]*?\},/g) ?? []
    const tab = src.match(/'settings\.tabDevices':\s*\{[\s\S]*?\},/)?.[0] ?? ''
    for (const block of [...blocks, tab]) {
      expect(block, `${block.slice(0, 40)}… misses a locale`).toMatch(/en:/)
      expect(block).toMatch(/ko:/)
      expect(block).toMatch(/ja:/)
    }
  })
})
