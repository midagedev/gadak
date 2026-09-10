import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

import {
  coerceDroppedReason,
  DROPPED_REASONS,
  TERMINAL_ANSI_VARS,
  TERMINAL_CHROME_VARS,
  TERMINAL_FONT_VARS,
  terminalFontFamily,
  watchChromeVars,
} from './protocol'

// GDK-932: the wire vocabulary is the serve's own (internal/term/session.go),
// and protocol.ts promises "neither side re-spells them." An unknown reason is
// coerced at runtime, so a Go rename or addition that TS never followed would
// silently degrade to 'closed' instead of failing anything. This pins the two
// sets against each other, both directions, so the divergence is caught here.
describe('GDK-932 dropped-reason parity (Go ⟷ TS)', () => {
  // The Go source is the source of truth; scrape its Reason* string literals.
  const goReasons = (): Set<string> => {
    const src = readFileSync(
      resolve(dirname(fileURLToPath(import.meta.url)), '../../../../internal/term/session.go'),
      'utf8',
    )
    const out = new Set<string>()
    // Matches:  ReasonSlow = "slow_client"
    const re = /^\s*Reason\w+\s*=\s*"([^"]+)"/gm
    for (let m = re.exec(src); m !== null; m = re.exec(src)) out.add(m[1])
    return out
  }

  test('the scrape finds the constants (guards the regex, not just the data)', () => {
    // If the constant style in session.go ever changes shape, this fails loudly
    // rather than letting an empty set pass the parity checks below.
    expect(goReasons().size).toBeGreaterThanOrEqual(5)
  })

  test('every Go reason is a known TS DroppedReason', () => {
    for (const r of goReasons()) {
      expect(DROPPED_REASONS.has(r), `Go reason ${r} missing from protocol.ts`).toBe(true)
      // And it survives coercion as itself, not degraded to 'closed'.
      expect(coerceDroppedReason(r)).toBe(r)
    }
  })

  test('every TS DroppedReason is a Go Reason constant', () => {
    const go = goReasons()
    for (const r of DROPPED_REASONS) {
      expect(go.has(r), `TS reason ${r} has no Go Reason* constant`).toBe(true)
    }
  })

  test('an unknown reason coerces to closed, not through', () => {
    expect(coerceDroppedReason('reboot')).toBe('closed')
    expect(coerceDroppedReason(undefined)).toBe('closed')
    expect(coerceDroppedReason(42)).toBe('closed')
  })
})

/*
 * GDK-1109: TERMINAL_CHROME_VARS is the single owner of the chrome variable
 * *names* because two renderers read them — this one and the phone's, which
 * imports this module directly. Before the list had an owner, each renderer
 * spelled its own copy, so a rename in app.css (or in the web renderer) left
 * the phone's chrome silently on fallbacks. Two pins close both directions:
 * the list against app.css, and both renderers against the list.
 */
describe('GDK-1109 chrome-variable parity (protocol ⟷ app.css ⟷ renderers)', () => {
  const HERE = dirname(fileURLToPath(import.meta.url))

  // Every custom property app.css declares, @theme and the theme overrides
  // alike (same names, different values). Anchored to a declaration shape,
  // not a block structure a CSS refactor could move.
  const declaredTokens = (): Set<string> => {
    const css = readFileSync(resolve(HERE, '../../app.css'), 'utf8')
    const out = new Set<string>()
    const re = /^[ \t]*(--[a-z0-9-]+)\s*:/gm
    for (let m = re.exec(css); m !== null; m = re.exec(css)) out.add(m[1])
    return out
  }

  test('the scrape finds the custom properties (guards the regex, not the data)', () => {
    expect(declaredTokens().size).toBeGreaterThanOrEqual(20)
  })

  test('every chrome variable the list names is declared in app.css', () => {
    const declared = declaredTokens()
    for (const [slot, name] of Object.entries(TERMINAL_CHROME_VARS)) {
      expect(
        declared.has(name),
        `${slot} reads ${name}, which app.css no longer declares — update TERMINAL_CHROME_VARS (protocol.ts is the list's one owner)`,
      ).toBe(true)
    }
  })

  // GDK-1358: the sixteen ANSI slots are tokens too — one per xterm ITheme
  // key, declared in app.css under every palette (theme-check holds the
  // per-palette parity; this holds the list against the stylesheet).
  test('every ANSI variable the list names is declared in app.css', () => {
    const declared = declaredTokens()
    const slots = Object.keys(TERMINAL_ANSI_VARS)
    expect(slots).toHaveLength(16)
    for (const [slot, name] of Object.entries(TERMINAL_ANSI_VARS)) {
      expect(declared.has(name), `${slot} reads ${name}, which app.css does not declare`).toBe(true)
    }
    const src = readFileSync(resolve(HERE, 'renderer.ts'), 'utf8')
    expect(src, 'renderer.ts must read the ANSI slots through TERMINAL_ANSI_VARS').toContain(
      'TERMINAL_ANSI_VARS',
    )
    // The property, not the loop's spelling: no slot is named by hand.
    expect(src, 'renderer.ts must not spell an --color-ansi-* token itself').not.toContain(
      "'--color-ansi-",
    )
  })

  test('the web renderer reads the names through the list, not a re-spelled copy', () => {
    const src = readFileSync(resolve(HERE, 'renderer.ts'), 'utf8')
    for (const slot of Object.keys(TERMINAL_CHROME_VARS)) {
      expect(src, `renderer.ts must read its ${slot} via TERMINAL_CHROME_VARS`).toContain(
        `TERMINAL_CHROME_VARS.${slot}`,
      )
    }
    // Chrome names re-spelled as literals are the drift this gate exists to
    // catch. Font and size tokens (--font-*, --text-*) are not chrome and
    // stay as literals.
    expect(src.includes("'--color-")).toBe(false)
  })

  /*
   * GDK-1156: both renderers must be wired to the shared watcher, not just
   * able to be. The behaviour is held in a real browser by
   * e2e/terminal-theme.spec.ts — but that spec drives the WEB pane, and the
   * phone has no equivalent harness (its e2e runs the unpaired three-tab
   * shell, where no terminal is constructed at all). So the phone's half is
   * pinned the same way its variable names already are: by source, here,
   * next to the list it shares. A renderer that goes back to reading the
   * chrome once fails this before anyone flips a phone to dark at sunset.
   */
  test('both renderers subscribe to the shared watcher', () => {
    for (const path of ['renderer.ts', '../../../../mobile/src/lib/terminal/renderer.ts']) {
      const src = readFileSync(resolve(HERE, path), 'utf8')
      expect(src, `${path} must import the shared watcher`).toContain('watchChromeVars')
      // GDK-1357: the web renderer reads the chrome off the pane's host
      // (`chromeTheme(scope)`), so the argument list is open here.
      expect(src, `${path} must re-apply the chrome, not only read it once`).toMatch(
        /term\.options\.theme\s*=\s*chromeTheme\([^)]*\)/,
      )
    }
  })
})

/*
 * GDK-1528: the pane's font stack used to live as two whole copies — the
 * resolution order, the token names, and the fallback literal — one in the
 * web renderer, one in the phone's, byte-identical the day they were split
 * and free to drift the next. The phone imports this module directly, so the
 * chrome variable names already had their one owner here (GDK-1109) and the
 * dropped-reason vocabulary before that (GDK-932); the font stack is the
 * same shape of thing and now has the same home. These pins hold the owner
 * against app.css and both renderers against the owner, exactly as the
 * chrome list is held.
 */
describe('GDK-1528 font-stack ownership (protocol ⟷ app.css ⟷ renderers)', () => {
  const HERE = dirname(fileURLToPath(import.meta.url))

  // Same declaration-shaped scrape as the chrome parity above: names as
  // app.css declares them, @theme and palette overrides alike.
  const declared = (): Set<string> => {
    const css = readFileSync(resolve(HERE, '../../app.css'), 'utf8')
    const out = new Set<string>()
    const re = /^[ \t]*(--[a-z0-9-]+)\s*:/gm
    for (let m = re.exec(css); m !== null; m = re.exec(css)) out.add(m[1])
    return out
  }

  test('every font variable the owner names is declared in app.css', () => {
    const names = declared()
    for (const [slot, name] of Object.entries(TERMINAL_FONT_VARS)) {
      expect(
        names.has(name),
        `${slot} reads ${name}, which app.css no longer declares — update TERMINAL_FONT_VARS (protocol.ts is the list's one owner)`,
      ).toBe(true)
    }
  })

  test('the owner resolves terminal token → app face → literal, in that order', () => {
    const both = terminalFontFamily(
      (n) => (n === '--font-mono-terminal' ? 'Menlo, monospace' : 'ui-monospace, monospace'),
    )
    expect(both).toBe('Menlo, monospace')
    const appOnly = terminalFontFamily(
      (n) => (n === '--font-mono' ? 'ui-monospace, monospace' : ''),
    )
    expect(appOnly).toBe('ui-monospace, monospace')
    // Units and jsdom ship no stylesheet: the literal is the whole stack.
    expect(terminalFontFamily(() => '')).toBe(
      'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
    )
  })

  test('both renderers resolve through the owner, not a re-spelled copy', () => {
    for (const path of ['renderer.ts', '../../../../mobile/src/lib/terminal/renderer.ts']) {
      const src = readFileSync(resolve(HERE, path), 'utf8')
      expect(src, `${path} must resolve its font stack via terminalFontFamily`).toContain(
        'terminalFontFamily',
      )
      // A token literal or the fallback stack re-spelled in a renderer is the
      // drift this gate exists to catch — the two copies had already grown
      // different doc comments when GDK-1528 found them.
      expect(src, `${path} must not spell the font tokens itself`).not.toContain(
        "'--font-mono-terminal'",
      )
      expect(src, `${path} must not re-spell the fallback stack`).not.toContain(
        'SFMono-Regular, Menlo, Consolas',
      )
    }
  })
})

/*
 * GDK-1156: the watcher runs inside a renderer that is also constructed in
 * the plain unit project (environment 'node' — vite.config.ts), so it has to
 * be a no-op without a DOM rather than throw on `document`. The behaviour
 * that matters — noticing a stylesheet swap, an attribute, a media flip —
 * is held by e2e/terminal-theme.spec.ts in a real browser; this pins only
 * the guard, which is the half a node test can actually see.
 */
describe('GDK-1156 chrome watcher, without a document', () => {
  test('returns a stoppable no-op and never reads the tokens', () => {
    let reads = 0
    const w = watchChromeVars(
      () => {
        reads += 1
        return 'x'
      },
      () => {
        throw new Error('onChange fired with no DOM')
      },
    )
    expect(reads).toBe(0)
    expect(reads).toBe(0)
    expect(() => w.stop()).not.toThrow()
  })
})
