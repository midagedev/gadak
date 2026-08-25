import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/*
 * Recurrence layer for GDK-908 (track F4): the shell must admit it is
 * attaching, unavailable must offer a new session, the host must be a
 * named control, and a bar key must not overtake an open composition.
 * PairingTab: the destructive unpair is a different weight from unpairing
 * the shell, the viewport probe is not product chrome, and unpair copy
 * matches store.unpair() (serve slot only).
 *
 * Source scan, same family as lib/contract.test.ts — vitest is node and
 * these screens are not imported here (terminal/{keys,ime,api,transport}
 * stay the unit-tested cores). Comments that name a ban are not the ban.
 */

const src = join(dirname(fileURLToPath(import.meta.url)), '../..')

function read(rel: string): string {
  return readFileSync(join(src, rel), 'utf8')
}

function markup(rel: string): string {
  return read(rel)
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
}

const shell = markup('screens/Shell.svelte')
const shellRaw = read('screens/Shell.svelte')
const pairing = markup('screens/PairingTab.svelte')
const pairingRaw = read('screens/PairingTab.svelte')

describe('GDK-908 first-attach connecting state', () => {
  it('the Status union includes connecting', () => {
    expect(shell).toMatch(/kind:\s*'connecting'/)
  })

  it('first paint is connecting, not a bare none', () => {
    expect(shell).toMatch(/status = \$state<Status>\(\{\s*kind:\s*'connecting'\s*\}\)/)
  })

  it('clears connecting when the socket opens', () => {
    const onOpen = shell.slice(shell.indexOf('onOpen('), shell.indexOf('onBytes('))
    expect(onOpen).toMatch(/'connecting'/)
    expect(onOpen).toMatch(/kind:\s*'none'/)
  })

  it('says loading in shapes, not a spinner', () => {
    expect(shell).toMatch(/class="paper"|class="connecting"/)
    expect(shell).not.toMatch(/\bspinner\b/)
    expect(shell).not.toMatch(/class:spin/)
  })
})

describe('GDK-908 unavailable offers a new session', () => {
  it('unavailable is restartable with the same Enter/tap path as exited and dropped', () => {
    const activate = shell.slice(
      shell.indexOf('function onStatusActivate'),
      shell.indexOf('function focusIme'),
    )
    expect(activate).toMatch(/unavailable/)
    expect(activate).toMatch(/sendBytes/)
  })

  it('Enter on an unavailable session starts a new one (sendBytes restart)', () => {
    const send = shell.slice(shell.indexOf('function sendBytes'), shell.indexOf('function sendText'))
    expect(send).toMatch(/unavailable/)
    expect(send).toMatch(/startNew/)
  })

  it('unavailable keeps the host and shows the restart hint, not a dead-end pane', () => {
    expect(shell).not.toMatch(/status\.kind === 'unavailable'[\s\S]{0,200}class="ended"/)
    expect(shell).toMatch(/terminal\.restartHint/)
    const restartable = shell.match(/kind === 'unavailable'/g) ?? []
    expect(restartable.length).toBeGreaterThan(0)
  })
})

describe('GDK-908 host is a named control', () => {
  it('does not suppress the a11y rules that would have caught a nameless host', () => {
    expect(shellRaw).not.toMatch(/svelte-ignore a11y_no_static_element_interactions/)
    expect(shellRaw).not.toMatch(/svelte-ignore a11y_click_events_have_key_events/)
  })

  it('gives the host a role and the catalog name, and still focuses the IME', () => {
    const host = shell.slice(shell.indexOf('class="host"'), shell.indexOf('class="host"') + 500)
    expect(host).toMatch(/role="textbox"/)
    expect(host).toMatch(/aria-label=\{t\('terminal\.title'\)\}/)
    expect(host).toMatch(/tabindex="0"/)
    expect(shell).toMatch(/onpointerdown=\{focusIme\}/)
  })
})

describe('GDK-908 bar-key flush barrier', () => {
  it('commits an open composition before sending the bar key', () => {
    const fn = shell.slice(shell.indexOf('function onBarKey'), shell.indexOf('function flushIme'))
    expect(fn).toMatch(/ime\.composing/)
    expect(fn).toMatch(/compositionend/)
    expect(fn.indexOf('composing')).toBeLessThan(fn.indexOf('bytesForBarKey'))
    expect(fn.indexOf('compositionend')).toBeLessThan(fn.indexOf('bytesForBarKey'))
  })
})

describe('GDK-908 pairing: two unpair actions, one of them destructive', () => {
  it('Unpair this phone and Unpair the shell do not share a class', () => {
    const buttonBefore = (label: string): string => {
      const i = pairing.indexOf(label)
      expect(i, label).toBeGreaterThan(-1)
      return pairing.slice(pairing.lastIndexOf('<button', i), i)
    }
    const shellBtn = buttonBefore('Unpair the shell')
    const phoneBtn = buttonBefore('Unpair this phone')
    const shellClass = shellBtn.match(/class="([^"]+)"/)?.[1] ?? ''
    const phoneClass = phoneBtn.match(/class="([^"]+)"/)?.[1] ?? ''
    expect(shellClass).not.toBe('')
    expect(phoneClass).not.toBe('')
    expect(shellClass.split(/\s+/).sort().join(' ')).not.toBe(phoneClass.split(/\s+/).sort().join(' '))
  })

  it('destructive unpair still uses ink, never a status token', () => {
    const styles = pairing.slice(pairing.indexOf('<style>'))
    expect(styles).not.toMatch(/\.unpair[^{]*\{[^}]*--color-status-/)
  })
})

describe('GDK-908 pairing: probe off the product surface', () => {
  it('the viewport probe is not a visible product line', () => {
    expect(pairingRaw).toMatch(/viewportProbe/)
    const probeTag = pairing.match(/<p[^>]*class="probe"[^>]*>/)
    expect(probeTag?.[0] ?? pairing).toMatch(/\bhidden\b|data-viewport/)
  })
})

describe('GDK-908 unpair copy matches store.unpair()', () => {
  it('does not claim the Keychain token as a singular that includes the shell', () => {
    expect(pairing).not.toMatch(/deletes the token from the Keychain/)
  })

  it('says the shell pairing is separate', () => {
    expect(pairing).toMatch(/shell pairing/i)
  })
})

describe('GDK-908 the attach effect must not depend on status', () => {
  /*
   * Found at lead review, not by this file's first draft. activate() is
   * called from the `$effect` that watches app.tab and hostEl, so every
   * $state it *reads* becomes that effect's dependency. Reading `status`
   * there subscribed the attach effect to the field attaching updates:
   * onOpen sets 'none', the effect re-runs, actSeq bumps, and attachSocket()
   * detaches the socket that had just opened — connecting → reconnecting,
   * forever. Writing a fresh object each pass made the cycle synchronous and
   * fatal (Svelte effect_update_depth_exceeded killed the pane on the first
   * tap of the Terminal tab; 5 of 6 shell e2e tests died at 90s each).
   *
   * shell.spec.ts is the real guard. This is the cheap one that runs in
   * milliseconds, because that e2e only fails after three minutes of
   * timeouts.
   */
  const activate = (() => {
    const s = read('screens/Shell.svelte')
    const start = s.indexOf('async function activate()')
    expect(start, 'activate() in Shell.svelte').toBeGreaterThan(-1)
    return s.slice(start, s.indexOf('\n  function reattachNow', start))
  })()

  it('reads status only inside untrack', () => {
    const body = activate.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
    const untracked = body.match(/untrack\(\(\) => \{[\s\S]*?\n {4}\}\)/)
    expect(untracked, 'an untrack() block in activate()').toBeTruthy()
    const outside = body.replace(untracked![0], '')
    expect(outside, 'status read outside untrack').not.toMatch(/\bstatus\b/)
  })

  it('imports untrack from svelte', () => {
    expect(read('screens/Shell.svelte')).toMatch(/import \{[^}]*\buntrack\b[^}]*\} from 'svelte'/)
  })
})

describe('GDK-908 invariants that must still hold', () => {
  it('dials the terminal session, never the serve token', () => {
    expect(shell).toContain('terminalSession()')
    expect(shell).not.toMatch(/createShellSession\([^)]*app\.meta/)
  })

  it('keeps the session id and visibility reattach', () => {
    expect(shellRaw).toContain('keptSessionId')
    expect(shell).toContain('onVisibility')
    expect(shell).toContain('visibilitychange')
  })

  it('does not print a token value', () => {
    for (const text of [shell, pairing]) {
      expect(text).not.toMatch(/token\.(slice|substring|length)/)
      expect(text).not.toMatch(/\$\{[^}]*token[^}]*\}/)
    }
  })
})
