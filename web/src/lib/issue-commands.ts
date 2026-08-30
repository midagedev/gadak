/*
 * Command blocks in an issue body (GDK-1162).
 *
 * An issue body reaches this app in two shapes and both carry code:
 *
 *   ADF     — Jira. `codeBlock` nodes, with the language in attrs.
 *   Markdown — Linear (internal/sync stores the body as text and leaves
 *              description_adf empty on purpose: "markdown must not pose as
 *              ADF"), and anything else that arrives as prose.
 *
 * One owner for both, so "what is a command in this body" cannot be answered
 * two different ways by two renderers.
 *
 * `runnable` is the narrow part, and it is narrow deliberately. The serve's
 * input route refuses \n and \r (internal/server/terminal.go): the button
 * exists to put a line in front of a person, not to run a script. A block
 * that is not one line is still shown as a code block — it just has no ▶,
 * because there is no honest single thing to place.
 */

import type { AdfNode } from './types'

/** Byte cap on one placed line — terminalInputMax in internal/server. */
export const COMMAND_MAX_BYTES = 4096

export interface CommandBlock {
  /** The block's text, verbatim (trailing newline stripped). */
  text: string
  /** Fence info string / ADF `language` attr. Empty when unset. */
  lang: string
  /** Whether this can be placed in a shell: exactly one non-empty line. */
  runnable: boolean
}

/**
 * One line, non-empty, and small enough for the serve to accept.
 *
 * The byte count is TextEncoder's, not `.length`: the server measures Go
 * bytes, and a body of Korean or emoji is three to four bytes a character. A
 * client that counted UTF-16 units would offer a ▶ the serve then refuses.
 */
export function isRunnableCommand(text: string): boolean {
  const line = text.replace(/[\r\n]+$/, '')
  if (line.trim() === '') return false
  if (/[\r\n]/.test(line)) return false
  return new TextEncoder().encode(line).length <= COMMAND_MAX_BYTES
}

/** The text a ▶ would place: the block minus its trailing newline. */
export function commandTextOf(raw: string): string {
  return raw.replace(/[\r\n]+$/, '')
}

function block(raw: string, lang: string): CommandBlock {
  const text = commandTextOf(raw)
  return { text, lang: lang.trim(), runnable: isRunnableCommand(text) }
}

/** A markdown body, cut into the prose between its fences and the fences. */
export type BodySegment =
  | { kind: 'text'; text: string }
  | { kind: 'code'; block: CommandBlock }

/**
 * Cut markdown text at its fenced code blocks. One owner of the fence rules,
 * so the parser that decides what gets a ▶ and the renderer that draws the
 * block can never disagree about where a block starts.
 *
 * CommonMark's rules, kept to the parts a body actually uses:
 *  - a fence opens on ``` or ~~~ (three or more), optionally indented up to
 *    three spaces, with an optional info string after it;
 *  - it closes on a line of the *same character*, at least as long, with
 *    nothing else on it. A shorter run inside is content, which is how a
 *    ```` block quoting a ``` block survives;
 *  - a fence that never closes runs to the end of the text, rather than
 *    swallowing the document into nothing.
 */
export function splitFencedBody(text: string | null | undefined): BodySegment[] {
  if (!text) return []
  const out: BodySegment[] = []
  let prose: string[] = []
  let open: { char: string; len: number; lang: string; body: string[] } | null = null

  const flushProse = () => {
    if (prose.length === 0) return
    const joined = prose.join('\n')
    prose = []
    if (joined.trim() !== '') out.push({ kind: 'text', text: joined })
  }

  for (const line of text.split('\n')) {
    const fence = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(line)
    if (open) {
      const closes =
        fence !== null &&
        fence[1][0] === open.char &&
        fence[1].length >= open.len &&
        fence[2].trim() === ''
      if (closes) {
        out.push({ kind: 'code', block: block(open.body.join('\n'), open.lang) })
        open = null
      } else {
        open.body.push(line)
      }
      continue
    }
    // A backtick fence's info string may not contain a backtick (CommonMark):
    // ``a`b`` is inline code, not a fence.
    if (fence && !(fence[1][0] === '`' && fence[2].includes('`'))) {
      flushProse()
      open = { char: fence[1][0], len: fence[1].length, lang: fence[2], body: [] }
      continue
    }
    prose.push(line)
  }
  if (open) out.push({ kind: 'code', block: block(open.body.join('\n'), open.lang) })
  flushProse()
  return out
}

/** Fenced code blocks in markdown text, in document order. */
export function fencedCommandBlocks(text: string | null | undefined): CommandBlock[] {
  return splitFencedBody(text)
    .filter((s): s is { kind: 'code'; block: CommandBlock } => s.kind === 'code')
    .map((s) => s.block)
}

/** `codeBlock` nodes anywhere in an ADF tree, in document order. */
export function adfCommandBlocks(node: AdfNode | null | undefined): CommandBlock[] {
  const out: CommandBlock[] = []
  walk(node, out)
  return out
}

function walk(node: AdfNode | null | undefined, out: CommandBlock[]): void {
  if (!node) return
  if (node.type === 'codeBlock') {
    const lang = node.attrs?.language
    const text = (node.content ?? []).map((c) => c.text ?? '').join('')
    out.push(block(text, typeof lang === 'string' ? lang : ''))
    return
  }
  for (const child of node.content ?? []) walk(child, out)
}

/**
 * The body's command blocks, whichever shape it arrived in. ADF wins when
 * there is one: `fallback` is the flattened text of the same document
 * (description_text), so reading both would count every block twice.
 */
export function issueCommandBlocks(
  node: AdfNode | null | undefined,
  fallback: string | null | undefined,
): CommandBlock[] {
  const fromAdf = adfCommandBlocks(node)
  if (fromAdf.length > 0) return fromAdf
  if (node && (node.content?.length ?? 0) > 0) return []
  return fencedCommandBlocks(fallback)
}
