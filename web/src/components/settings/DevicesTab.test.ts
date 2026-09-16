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
import { afterEach, describe, expect, test, vi } from 'vitest'
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
  const ifBlocks: AnyNode[] = []
  walkTemplate(ast.fragment, (n) => {
    if (n.type === 'IfBlock') ifBlocks.push(n)
  })
  return { source, elements, ifBlocks, script: ast.instance?.content.body ?? [] }
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
    // GDK-1966: the phone-open card adds a second <img> (its QR is fetched
    // from the serve route, not client-encoded), so "the only img in the
    // tab" stopped pinning the pairing QR — it is pinned by its own testid
    // now. FAIL-first confirmed on the pre-GDK-1966 source: this count
    // reads 1.
    const images = elements.filter((e) => e.name === 'img')
    expect(images, 'the tab renders the pairing QR and the phone QR').toHaveLength(2)
    const pairing = images.find((e) => staticAttribute(e, 'data-testid') === 'devices-qr')
    expect(pairing, 'the pairing QR keeps its devices-qr testid').toBeDefined()
    expect(expressionAttribute(pairing as AnyNode, 'src')).toBe('minted.qr_png')
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

/*
 * GDK-1966: the "Open on your phone" card. Same node-environment story as
 * above — the render contract is asserted on the AST, and the data contract
 * (`phone_urls` in config.json) is driven through loadConfig with fetch
 * stubbed, the seam config.test.ts already uses.
 */
describe('GDK-1966 phone-open card', () => {
  const { source: src, elements, ifBlocks } = parseComponent(DEVICES_TAB, 'DevicesTab.svelte')

  async function loadConfigWith(body: unknown, ok = true) {
    vi.resetModules()
    // runtimeBase() reads window.location.pathname; the unit project runs in node.
    vi.stubGlobal('window', { location: { pathname: '/' } })
    vi.stubGlobal('fetch', async () =>
      ok
        ? new Response(JSON.stringify(body), { status: 200 })
        : new Response('missing', { status: 404 }),
    )
    const mod = await import('../../lib/config')
    await mod.loadConfig()
    return mod
  }

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  /** Source text of an AST node (modern parse carries start/end offsets). */
  function nodeText(node: AnyNode | undefined): string {
    if (!node || typeof node.start !== 'number' || typeof node.end !== 'number') return ''
    return src.slice(node.start, node.end)
  }

  test('two urls: first carries the QR and selectable text, the rest a plain list', async () => {
    // Config side: the wire array arrives intact under the parsed name.
    const urls = ['https://phone-a.example.ts.net/m/', 'https://phone-b.example.ts.net:7777/m/']
    const mod = await loadConfigWith({ phone_urls: urls })
    expect(mod.config().phoneUrls).toEqual(urls)

    // Render side: the card, the QR fetched from the serve route (GDK-1966
    // — the encoder that used to live in the component is gone; the serve
    // mints the PNG over phone_urls[0]), the first url as text with the
    // tab's copy affordance, and exactly the remaining urls under it.
    const card = elements.find((e) => staticAttribute(e, 'data-testid') === 'phone-open')
    expect(card, 'the card carries data-testid="phone-open"').toBeDefined()
    const qr = elements.find((e) => staticAttribute(e, 'data-testid') === 'phone-open-qr')
    expect(qr?.name).toBe('img')
    expect(staticAttribute(qr as AnyNode, 'src')).toBe('/phone-qr.png?i=0')
    expect(src).toMatch(/data-testid="phone-open-url"[^>]*>\{phoneUrls\[0\]\}/s)
    expect(src).toMatch(/copyText\(phoneUrls\[0\]\)/)
    expect(src).toMatch(/\{#each phoneUrls\.slice\(1\) as/)
  })

  test('empty list: the none sentence replaces the body and no QR is drawn', async () => {
    const mod = await loadConfigWith({ phone_urls: [] })
    expect(mod.config().phoneUrls).toEqual([])

    // The empty branch renders the none sentence… (`test` is the modern
    // AST's name for an IfBlock's condition.)
    const emptyIf = ifBlocks.find((n) => nodeText((n.test ?? n.expression) as AnyNode) === 'phoneUrls.length === 0')
    expect(emptyIf, 'the card branches on phoneUrls.length === 0').toBeDefined()
    expect(nodeText(emptyIf as AnyNode)).toMatch(/settings\.phoneOpen\.none/)
    // …with no <img> inside it, and the QR/URL sit in the other branch.
    const emptyEls: string[] = []
    walkTemplate((emptyIf as AnyNode).consequent, (n) => {
      if (n.type === 'RegularElement') emptyEls.push(String(n.name))
    })
    expect(emptyEls, 'the empty branch draws no QR').not.toContain('img')
    const fullEls: string[] = []
    walkTemplate((emptyIf as AnyNode).alternate, (n) => {
      if (n.type === 'RegularElement') fullEls.push(String(n.name))
    })
    expect(fullEls).toContain('img')
    expect(nodeText(emptyIf as AnyNode)).toMatch(/settings\.phoneOpen\.body/)
  })

  test('absent phone_urls key reads the same as empty', async () => {
    // An older serve sends nothing — DEFAULTS' empty list must survive the
    // merge rather than read `undefined` and throw on .length.
    const mod = await loadConfigWith({})
    expect(mod.config().phoneUrls).toEqual([])
    // Garbage is not guessed into urls either.
    const garbage = await loadConfigWith({ phone_urls: 'https://phone-a.example.ts.net/m/' })
    expect(garbage.config().phoneUrls).toEqual([])
  })

  test('phoneOpen copy exists in the catalog with all three locales', () => {
    const catalog = readFileSync(join(MESSAGES, 'settings.ts'), 'utf8')
    const keys = ['settings.phoneOpen.title', 'settings.phoneOpen.body', 'settings.phoneOpen.none']
    for (const key of keys) {
      const block = catalog.match(new RegExp(`'${key.replace(/\./g, '\\.')}':\\s*\\{[\\s\\S]*?\\},`))?.[0]
      expect(block, `${key} missing from the catalog`).toBeDefined()
      expect(block, `${key} misses a locale`).toMatch(/en:/)
      expect(block).toMatch(/ko:/)
      expect(block).toMatch(/ja:/)
    }
  })
})
