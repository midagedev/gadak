import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-1815 (CI run 34664828627, both attempts): a live terminal pane owns the
 * keyboard. TerminalPane's `onAttached` calls `renderer.focus()`, and from
 * xterm's helper textarea the VT keeps every Ctrl chord except the Backquote
 * family (web/src/lib/terminal/renderer.ts `isAppChord`) — Ctrl+K is the
 * shell's kill-line and staying the shell's is correct.
 *
 * What this lint refuses is one spelling, not one chord. `ControlOrMeta` is
 * Meta on macOS, which xterm has no binding for and which reaches the app,
 * and Control on the Linux runner, which xterm consumes. So a spec that opens
 * the dock and then presses `ControlOrMeta+<key>` passes on the machine it was
 * written on and fails only in CI. Measured here with the dock focused:
 * Control+k -> 0 palette rows, Meta+k -> 1.
 *
 * The literal `Control+<key>` spelling is deliberately NOT flagged: it is
 * eaten on every platform alike, so a spec that gets it wrong is red for its
 * author too — and it is also how a spec legitimately sends SIGINT to the VT
 * (terminal-modes.spec.ts Control+c after focusTerm, which is the shell's
 * chord and no business of this lint's).
 *
 * The recourse is the chord the product ships: Ctrl+Shift+` (commands.ts
 * `terminal-focus-strip` — "leave the VT without closing the pane"), pressed
 * after the pane reports `data-attached`, since `onAttached` fires late
 * enough to undo an earlier escape.
 *
 * Ordered by line, because a chord pressed BEFORE the dock is ever opened is
 * not in the VT's reach — demo/terminal-demo.spec.ts opens the palette to
 * *discover* the pane, which is the opposite shape and fine. It is a lint,
 * not a proof: it reads position in a file, not the run.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const ROOT = join(HERE, '..')
const SKIP_DIRS = new Set(['.tmp', 'node_modules', 'test-results'])

/** Toggling the dock open — the act that hands the keyboard to xterm. */
const OPENS_DOCK = "press('Control+Backquote')"
/** Leaving the VT with the pane still open — terminal-focus-strip. */
const LEAVES_VT = "press('Control+Shift+Backquote')"
/** The platform-divergent spelling: Meta locally, Control on the runner. */
const PORTABLE_CHORD = /press\('ControlOrMeta\+/

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (name.endsWith('.spec.ts')) out.push(p)
  }
  return out
}

function isComment(line: string): boolean {
  const t = line.trim()
  return t.startsWith('//') || t.startsWith('*') || t.startsWith('/*')
}

/**
 * "path:line" for every `ControlOrMeta+` chord that follows a dock-open with
 * no escape between the two.
 */
export function chordsInsideTheVT(roots: string[]): string[] {
  const hits: string[] = []
  for (const root of roots) {
    for (const file of walk(root)) {
      let dockOpen = false
      readFileSync(file, 'utf8')
        .split('\n')
        .forEach((line, i) => {
          if (isComment(line)) return
          if (line.includes(LEAVES_VT)) dockOpen = false
          else if (line.includes(OPENS_DOCK)) dockOpen = true
          else if (dockOpen && PORTABLE_CHORD.test(line)) {
            hits.push(`${relative(ROOT, file)}:${i + 1}`)
          }
        })
    }
  }
  return hits.sort()
}

test('a ControlOrMeta chord is never pressed while the terminal dock holds the keyboard', () => {
  const offenders = chordsInsideTheVT([join(ROOT, 'e2e'), join(ROOT, 'mobile/e2e')])
  expect(
    offenders,
    'ControlOrMeta resolves to Control on the Linux runner, where xterm consumes it — ' +
      'green on macOS, red only in CI. Leave the VT first with ' +
      "page.keyboard.press('Control+Shift+Backquote') (commands.ts terminal-focus-strip), " +
      `after the pane reports data-attached:\n${offenders.join('\n')}`,
  ).toEqual([])
})
