import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/*
 * Recurrence layer for GDK-1150: "the phone ships an English string no
 * catalog owns" must fail here, not in a Korean reader's eye.
 *
 * vocabulary.test.ts (GDK-884) catches invented nouns; this catches the
 * quieter class — copy that was simply never routed through t(). The
 * 2026-09-10 census that motivated the gate: eleven-plus strings across
 * five screens (Back ×2 pairs, "Comment…", "Key, summary, comment…",
 * 'Searching…', 'Creating…', the transition sheet's ' · needs fields — use
 * desktop', 'Unpair this phone', TabBar's two aria-labels, …), every one
 * invisible to an English-reading reviewer because the screen around them
 * reads fine.
 *
 * What it scans, in every mobile/src .svelte template:
 *   1. copy attributes (aria-label, placeholder, title) with literal
 *      values — `attr={t('…')}` is an expression and passes,
 *   2. text nodes, and inside a text `{…}` expression its quoted string
 *      literals ({sending ? 'Sending…' : t('write.commentButton')} — the first arm
 *      is copy and is scanned; the t() call's own argument is not).
 * "English words" = /[A-Za-z]{2,}/. Structure is never scanned: control
 * blocks ({#if}, {/each}, {:else}, {@render}), event handlers and other
 * attributes, <style> blocks, and the script section.
 *
 * Scanning is a real tokenizer walk, not a split on /<[^>]*>/: a handler
 * attribute like onclick={() => (open = false)} contains both quotes and a
 * `>`, and t('key', {n: x}) spans braces — a naive split reads those as
 * text nodes and buries the real findings under a hundred false ones.
 *
 * The second `it` closes the other half of the same class: an English
 * *sentence* returned from a .ts module. That seam is where the copy the
 * template gate cannot see lives (api.ts errorMessage was a five-arm table
 * of them, reaching the reader on the very screens the cataloged strings
 * do). The rule is deliberately narrow so it stays true: a quoted literal
 * that reads as a sentence — three or more words and sentence-ending
 * punctuation — is copy, and copy comes from the catalog.
 *
 * Honest limits (documented here, not in the allowlist):
 *   - single words and fragments in .ts are not scanned: the
 *     §8-documented tab labels ('Search', 'Pairing') live in TabBar's
 *     script array and read as identifiers as often as copy. The template
 *     gate covers their rendered form.
 *   - a sentence assembled from pieces at runtime is invisible to both.
 */

const srcDir = join(dirname(fileURLToPath(import.meta.url)), '..')

function svelteFiles(): string[] {
  const out: string[] = []
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name)
      if (entry.isDirectory()) walk(path)
      else if (entry.name.endsWith('.svelte')) out.push(path)
    }
  }
  walk(srcDir)
  return out
}

/** Same stripper as vocabulary.test.ts: prose about the ban is not the ban. */
function code(path: string): string {
  return readFileSync(path, 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
}

/** The template: after the last </script>, without <style> blocks. */
function template(path: string): string {
  const src = code(path)
  const cut = src.lastIndexOf('</script>')
  return (cut === -1 ? src : src.slice(cut)).replace(/<style[\s\S]*?<\/style>/g, '')
}

const ENGLISH_WORD = /[A-Za-z]{2,}/

/** Balanced-paren removal of `t(…)` calls, so their keys are not "copy". */
function withoutTCalls(expr: string): string {
  let out = expr
  let guard = 0
  while (guard++ < 50) {
    const start = out.search(/\bt\s*\(/)
    if (start === -1) break
    let depth = 0
    let end = -1
    for (let i = start + out.slice(start).indexOf('('); i < out.length; i++) {
      if (out[i] === '(') depth++
      else if (out[i] === ')') {
        depth--
        if (depth === 0) {
          end = i + 1
          break
        }
      }
    }
    if (end === -1) break
    out = out.slice(0, start) + out.slice(end)
  }
  return out
}

/** Equality against a string literal is a discriminant check (mode enums),
 *  never display copy — copy compared by string equality would itself be
 *  the defect this gate exists for. */
const COMPARE_LITERAL = /[A-Za-z_$][\w$.]*\s*(?:===|!==)\s*(?:'[^']*'|"[^"]*")/g

/** One template → copy findings. Text expressions contribute their quoted
 *  literals (outside t() calls); control blocks are skipped entirely. */
function copyLiterals(path: string): { kind: 'attr' | 'text'; value: string }[] {
  const out: { kind: 'attr' | 'text'; value: string }[] = []
  const src = template(path)
  let i = 0
  const at = (offset: number): string => src[i + offset] ?? ''
  while (i < src.length) {
    if (src[i] === '<' && /[a-zA-Z/!]/.test(at(1))) {
      // Tag: consume to the `>` that sits outside quotes and {…} braces.
      let quote = ''
      let braces = 0
      let j = i + 1
      for (; j < src.length; j++) {
        const c = src[j]
        if (quote) {
          if (c === quote) quote = ''
        } else if (c === '"' || c === "'") quote = c
        else if (c === '{') braces++
        else if (c === '}') braces--
        else if (c === '>' && braces <= 0) break
      }
      const tag = src.slice(i, j + 1)
      for (const m of tag.matchAll(
        /\b(?:aria-label|placeholder|title)\s*=\s*(?:"([^"{]*)"|'([^'{}]*)')/g,
      )) {
        const value = m[1] ?? m[2] ?? ''
        if (ENGLISH_WORD.test(value)) out.push({ kind: 'attr', value })
      }
      i = j + 1
      continue
    }
    if (src[i] === '{' && /[#/:@]/.test(at(1))) {
      // Control block {#if …} {/if} {:else} {@render …} — structure, skipped.
      const close = src.indexOf('}', i)
      i = close === -1 ? src.length : close + 1
      continue
    }
    if (src[i] === '{') {
      // Text expression: consume to the matching } (expressions nest braces).
      let depth = 0
      let j = i
      for (; j < src.length; j++) {
        if (src[j] === '{') depth++
        else if (src[j] === '}') {
          depth--
          if (depth === 0) break
        }
      }
      const expr = withoutTCalls(src.slice(i + 1, j)).replace(COMPARE_LITERAL, '')
      for (const q of expr.matchAll(/'([^']*)'|"([^"]*)"/g)) {
        const value = q[1] ?? q[2] ?? ''
        if (ENGLISH_WORD.test(value)) out.push({ kind: 'text', value })
      }
      i = j + 1
      continue
    }
    // Text run up to the next < or {.
    let j = i
    while (j < src.length && src[j] !== '<' && src[j] !== '{') j++
    const text = src.slice(i, j).trim()
    if (text && ENGLISH_WORD.test(text)) out.push({ kind: 'text', value: text })
    i = j
  }
  return out
}

/**
 * Documented non-catalog literals — brand tokens, CLI commands shown
 * verbatim, and dev-only instrumentation. `file :: value` → reason a
 * reviewer can check. The GDK-1150 round cataloged everything §8 had left
 * as "connective prose" (its list is now history — see DESIGN.md §8), so
 * this list is short and should stay that way: a user-facing sentence
 * never belongs here, it belongs in the catalog.
 */
const ALLOWED = new Map<string, string>([
  ['screens/PairGate.svelte :: gadak', 'brand token — the product names itself'],
  [
    'screens/PairGate.svelte :: gadak pairing mint',
    'CLI command shown verbatim in the hint (mono .cmd span)',
  ],
  [
    'screens/PairingTab.svelte :: gadak pairing mint --scope terminal',
    'CLI command shown verbatim in the hint (mono span)',
  ],
  [
    'screens/PairingTab.svelte :: gadak mobile 0.1.0',
    'brand + version footer, locale-neutral by construction',
  ],
  [
    'screens/PairingTab.svelte :: DEV',
    'dev-only viewport probe prefix (import.meta.env.DEV-gated, hidden attr)',
  ],
])

/** Files whose sentence literals are not copy, with the reason. */
const ALLOWED_SCRIPT = new Map<string, string>([])

/** Three-plus words ending in . … ? or ! — the shape of a sentence a reader reads. */
function sentenceLiterals(path: string): string[] {
  const src = code(path)
  const out: string[] = []
  for (const m of src.matchAll(/'([^'\\\n]{12,})'|"([^"\\\n]{12,})"/g)) {
    const value = m[1] ?? m[2]
    if (!/[.…?!]$/.test(value)) continue
    if (value.split(/\s+/).filter((w) => /[A-Za-z]{2,}/.test(w)).length < 3) continue
    out.push(value)
  }
  return out
}

function tsFiles(): string[] {
  const out: string[] = []
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name)
      if (entry.isDirectory()) walk(path)
      else if (entry.name.endsWith('.ts') && !entry.name.endsWith('.test.ts')) out.push(path)
    }
  }
  walk(srcDir)
  return out
}

describe('GDK-1150 template copy comes from the catalog', () => {
  it('no .ts module returns an English sentence of its own', () => {
    const hits: string[] = []
    for (const path of tsFiles()) {
      const file = relative(srcDir, path)
      for (const value of sentenceLiterals(path)) {
        const id = `${file} :: ${value}`
        if (ALLOWED_SCRIPT.has(id)) continue
        hits.push(id)
      }
    }
    expect(hits, hits.join('\n')).toEqual([])
  })

  it('no .svelte template ships an English literal outside the allowlist', () => {
    const hits: string[] = []
    for (const path of svelteFiles()) {
      const file = relative(srcDir, path)
      for (const { kind, value } of copyLiterals(path)) {
        const id = `${file} :: ${value}`
        if (ALLOWED.has(id)) continue
        hits.push(`${id} (${kind})`)
      }
    }
    expect(hits, hits.join('\n')).toEqual([])
  })
})
