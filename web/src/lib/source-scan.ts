/*
 * One stripper for every source-scanning gate in both trees. This file is
 * the single owner (GDK-1878); mobile/src/lib/source-scan.ts re-exports it
 * so the phone's six importers keep their relative paths.
 *
 * The phone got here first, on 2026-09-14 (GDK-1872 part 2): five of its
 * scanners (contract, vocabulary, template-copy, back, terminal/gdk-908)
 * each carried their own copy of the same three replaces — "prose about a
 * ban is not the ban" — and a copy is where a scanner quietly stops seeing
 * the file it is scanning.
 *
 * Measured, 2026-09-14 (GDK-1872 part 2): the composer's picker markup is
 * `accept="image/*"`. Those two characters open a block comment as far as
 * `/\/\*[\s\S]*?\*\//` is concerned, so every one of the five scanners threw
 * away 34 KB of Detail.svelte — everything from that attribute to the next
 * comment terminator in the stylesheet. contract.test.ts said so out loud (four GDK-1497
 * pins went red looking for markup that was no longer in the string they were
 * handed), and that is the only reason it was noticed: vocabulary.test.ts and
 * template-copy.test.ts stayed GREEN while scanning a truncated file. A gate
 * that passes because it cannot see is the failure mode this file closes.
 *
 * The web tree still carried three naive copies until 2026-09-15 (GDK-1878):
 * terminal/gdk-944.test.ts's paneSrc() and the CSS_CODE constants in
 * layout-tokens.test.ts and theme-links.test.ts. They now call these
 * functions too.
 *
 * The fix is the lookbehind below, not a wider strip: a real block comment
 * never opens directly after a word character, a quote or a slash, and
 * `image/*` always does. Everything else about the three replaces is
 * unchanged, so no scanner sees LESS than it did — several now see more.
 */

/**
 * Source text with HTML comments, block comments and whole-line `//`
 * comments removed. Never removes a `/*` that sits inside a word or a quoted
 * value (`accept="image/*"`, a `url(…/*)`, a regex literal).
 */
export function stripComments(text: string): string {
  return text
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/(?<![\w"'/])\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
}

/**
 * Block comments (recognised by the same lookbehind regex as stripComments)
 * blanked in place: every non-newline character of the comment becomes a
 * space, so offsets, lengths and line structure stay real — the line numbers
 * CSS assertions quote remain the file's own.
 */
export function blankBlockComments(text: string): string {
  return text.replace(/(?<![\w"'/])\/\*[\s\S]*?\*\//g, (comment) =>
    comment.replace(/[^\n]/g, ' '),
  )
}
