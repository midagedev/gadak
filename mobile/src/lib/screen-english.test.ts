import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { blankBlockComments } from './source-scan'

/*
 * Recurrence layer for GDK-1935: the phone authored `${n} comment${n===1?'':'s'}`
 * while the desk's catalog already owned the sentence (`list.commentCount`),
 * and every gate stayed green. The rule this gate enforces is
 * mobile/src/lib/i18n.ts's own: "the phone never authors a word the desk
 * already owns" — user-facing English in mobile/src is a word the desk owns.
 *
 * The scan is textual, same stance as web-boundary.test.ts and
 * vocabulary.test.ts: no bundler, no type information, just the sources. It
 * flags string and template literals that carry an English plural or an
 * English sentence fragment — shapes that only make sense as words headed
 * for the screen:
 *
 *   ① a run of two or more pure-alphabetic words ("this machine", "This Mac")
 *   ② a word standing beside a `${…}` interpolation ("${n} comment"), when
 *     the literal's static text is not a path or dotted key ("a/${b}/c",
 *     "fields.${x}" — the interpolation itself may carry code, colons
 *     included: GDK-1935's own literal hid its ternary's `:` inside `${…}`,
 *     and this gate's first draft missed it by testing the whole literal)
 *
 * Three classes are structurally out of scope, and why:
 *   - lines that throw, log or reject (developer strings — the message the
 *     *developer* reads, never the phone's reader);
 *   - `class="…"` attribute values (CSS class lists read as two English
 *     words — "sub mono", "paper short" — but name styles, not sentences);
 *   - whole files named in EXEMPT_FILES, whose strings are refusal
 *     reasons, dev-rig commands or a crash overlay (each entry says which).
 *
 * Literals that carry escapes or span lines are skipped by the literal
 * matcher — conservative on purpose: a gate that false-positives is a gate
 * people route around, and both shapes are rare in screen copy.
 */

const srcDir = join(dirname(fileURLToPath(import.meta.url)), '..')

/** Files whose every literal is a refusal reason or a dev-rig string. */
const EXEMPT_FILES = [
  // Refusal `why:` reasons for deep links. Not screen copy by decision:
  // App.svelte leaves onRefused unwired because "a refusal a user should
  // read needs catalog keys in all three locales, which this round does not
  // author" (App.svelte, deeplink wiring comment).
  'lib/deeplink.ts',
  // The dev-only capture tour (armed by ?demo-tour in dev): it types shell
  // commands and narrates the take to the console. No user copy exists here.
  'lib/demo-tour.ts',
  // The DEV-gated crash overlay: a <pre> dump of e.message/e.reason for a
  // developer whose webview has no console (the file's own header). "unhandled
  // rejection" is that dump's prefix line — console-class, never localized.
  'main.ts',
]

/** A literal the gate flags but the tree deliberately keeps, with its why. */
const OPT_OUT: { file: string; needle: string; why: string }[] = [
  {
    file: 'screens/Settings.svelte',
    needle: 'inner ',
    why: 'dev-only viewport telemetry (DEV-gated $effect): axis labels for the geometry probe, not locale copy',
  },
  {
    file: 'screens/Settings.svelte',
    needle: ' · screen',
    why: 'dev-only viewport telemetry (same probe block)',
  },
  {
    file: 'screens/Settings.svelte',
    needle: 'this machine (dev proxy)',
    why: 'label for the dev-proxy host row (endpoint ""), a surface a packaged phone never shows; the catalog has no key for it yet',
  },
  {
    file: 'screens/Shell.svelte',
    needle: 'this machine (dev proxy)',
    why: 'same dev-proxy host label, same reason (Settings.svelte entry)',
  },
  {
    file: 'lib/store.svelte.ts',
    needle: 'This Mac (dev)',
    why: 'the dev-proxy adoption meta label, shown only in dev-paired sessions; the catalog has no key for it yet',
  },
  {
    file: 'screens/Settings.svelte',
    needle: 'This offer is for the issue mirror',
    why: 'genuine user-facing English (terminalProbeCopy, scope_rejected) — GDK-1935-class, needs desk catalog keys; reported to the lead, opted out until the desk owns the words',
  },
  {
    file: 'screens/Settings.svelte',
    needle: 'This code is expired or revoked',
    why: 'genuine user-facing English (terminalProbeCopy, pairing_rejected) — same class, same report',
  },
  {
    file: 'screens/PairGate.svelte',
    needle: 'dev: ',
    why: 'dev diagnostics (IS_DEV-gated keychain probe), presence-only and never localized',
  },
  {
    file: 'lib/terminal/transport.ts',
    needle: 'shell websocket is not the paired origin',
    why: 'thrown refusal reason (ORIGIN_MISMATCH), never rendered',
  },
  {
    file: 'lib/terminal/transport.ts',
    needle: 'shell endpoint is outside the app dialling scope',
    why: 'thrown refusal reason (ENDPOINT_OUT_OF_SCOPE), never rendered',
  },
  {
    file: 'ui/KeyBar.svelte',
    needle: 'No Mods',
    why: 'keycap legend, a deliberate naming the file documents (look verdict 2026-08-27: Clear/Reset are terminal commands, No Mods states what is true); the keycap cluster is unlocalized keyboard vocabulary — reported for the desk-catalog decision',
  },
]

/** Developer-string lines: the message never reaches the phone's reader. */
const DEV_LINE =
  /\bthrow\b|new\s+\w*Error\b|console\.(log|info|warn|error|debug)\b|\.reject\(/

/** Single-line literals without escapes: '…', "…", `…`. */
const LITERAL = /'([^'\\\n]*)'|"([^"\\\n]*)"|`([^`\n]*)`/g

/** A run of 2+ pure-alphabetic words, hyphen-glued tokens excluded. */
const FRAGMENT =
  /(^|[^A-Za-z0-9-])[A-Za-z][a-z]{1,}(?:\s+[A-Za-z][a-z]{1,})+([^A-Za-z0-9-]|$)/

/** A standalone lowercase word (≥3 letters) with the same boundaries. */
const WORD = /(^|[^A-Za-z0-9-])[a-z]{3,}([^A-Za-z0-9-]|$)/

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) walk(path, out)
    else if (/\.(ts|svelte)$/.test(entry.name) && !/\.test\.ts$/.test(entry.name)) {
      out.push(path)
    }
  }
  return out
}

/** Comments blanked in place, so the line numbers reported stay the file's
 *  own (blankBlockComments' own header explains why that matters). The
 *  `//` stage must not use the seam's `^\s*` shape: \s crosses the newline
 *  of a blank line above the comment, and blanking the match then merges
 *  the two lines — every hit below that point reports a line that does not
 *  exist (measured: Settings −6, Shell −7). `[^\S\n]` keeps the count. */
function code(text: string): string {
  return blankBlockComments(text)
    .replace(/<!--[\s\S]*?-->/g, (c) => c.replace(/[^\n]/g, ' '))
    .replace(/^[^\S\n]*\/\/.*$/gm, (m) => ' '.repeat(m.length))
}

function violations(): string[] {
  const hits: string[] = []
  for (const path of walk(srcDir)) {
    const rel = relative(srcDir, path)
    if (EXEMPT_FILES.includes(rel)) continue
    const raw = readFileSync(path, 'utf8').split('\n')
    const lines = code(raw.join('\n')).split('\n')
    lines.forEach((line, i) => {
      if (DEV_LINE.test(raw[i])) return
      for (const m of line.matchAll(LITERAL)) {
        const lit = m[1] ?? m[2] ?? m[3] ?? ''
        if (lit === '') continue
        // A `class="…"` value names styles, not words ("sub mono"). The
        // match starts at the value's opening quote, so the prefix ends
        // at the `=`.
        if (line.slice(0, m.index).endsWith('class=')) continue
        const isTemplate = m[3] !== undefined
        // What the reader would see: interpolations resolve to values, so
        // only the static text around them can carry authored words. Paths
        // and dotted key prefixes stay out by their own punctuation.
        const staticText = lit.replace(/\$\{[^}]*\}/g, ' ')
        const flagged =
          FRAGMENT.test(staticText) ||
          (isTemplate && !/[/:.]/.test(staticText) && WORD.test(staticText))
        if (!flagged) continue
        const opt = OPT_OUT.find(
          (o) => o.file === rel && lit.includes(o.needle),
        )
        if (opt) continue
        hits.push(`${rel}:${i + 1}: ${JSON.stringify(lit)}`)
      }
    })
  }
  return hits
}

describe('GDK-1935 the phone does not author English the desk owns', () => {
  it('no user-facing English literal in mobile/src', () => {
    // "the phone never authors a word the desk already owns"
    // (mobile/src/lib/i18n.ts). A hit here means a sentence the catalog
    // should own — fix it by calling t(), never by widening this gate.
    expect(violations()).toEqual([])
  })
})
