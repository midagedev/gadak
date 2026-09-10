/*
 * GDK-138: the close chrome of an overlay is a rule, not a per-file coin
 * toss. `docs/project/UX_PRINCIPLES.md` §15 states it; this measures it.
 *
 * The three clauses, and what each one is guarding against:
 *
 *  (1) One name. Every dismiss control is `common.closeEsc` on BOTH
 *      aria-label and title, so the screen reader and the tooltip say the
 *      same thing and an e2e spec can find any of them by one accessible
 *      name. A second phrasing ("Close", "Dismiss") is a second vocabulary
 *      for one action (§8).
 *  (2) One glyph. The mark inside that control is `<Icon name="x" />` —
 *      never a hand-rolled <svg>. Four panels carried byte-identical copies
 *      of the same 14x14 path, which is exactly the drift Icon.svelte exists
 *      to prevent (its header comment: an icon set is only a set if every
 *      member shares a grid, a weight and a color contract).
 *  (3) One owner for the modal class. A component with aria-modal="true"
 *      either IS DialogShell or renders one — nothing else may hand-roll a
 *      modal, which is GDK-316's claim restated on the a11y axis rather than
 *      the visual one (DialogShell.test.ts owns the visual half).
 *
 * Source scanning, not mounting: vitest unit is environment:'node' with no
 * svelte plugin (DialogShell.test.ts, dom-actions.test.ts). Rendered
 * behaviour — Esc, backdrop click, focus return — is Playwright's job in
 * e2e/dialog-shell.spec.ts and e2e/esc-negotiate.spec.ts.
 *
 * Deliberately outside the sweep, each with its reason:
 *  - CommandPalette: a different class (DialogShell.test.ts says the same) —
 *    it has the backdrop but no header X, because it closes on Esc, on
 *    outside click, and on running the thing you came for.
 *  - Anchored popovers and menus (AssigneePicker, HostedLinks, ScopePicker,
 *    FieldEditor, BulkBar): §15 gives them no X on purpose. They have no
 *    header to put one in, and outside-click is the dismissal a person
 *    already reaches for. Clause (1) still applies if one grows a control.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const COMPONENTS = join(HERE, '..')
const SHELL = join(HERE, 'DialogShell.svelte')

/** The one accessible name for dismissing an overlay. */
const CLOSE_ESC = "t('common.closeEsc')"

function svelteFiles(dir: string): string[] {
  const out: string[] = []
  for (const name of readdirSync(dir).sort()) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...svelteFiles(p))
    else if (name.endsWith('.svelte')) out.push(p)
  }
  return out
}

const rel = (p: string) => relative(COMPONENTS, p)
const FILES = svelteFiles(COMPONENTS)

/** The `<button …>…</button>` spans that carry the close name. Text slicing
 *  rather than the svelte AST: this asks about one attribute and the element
 *  between two literal tags, and dom-actions.test.ts already owns the walker
 *  for questions that need one. */
function closeButtons(src: string): string[] {
  const out: string[] = []
  let from = 0
  for (;;) {
    const open = src.indexOf('<button', from)
    if (open < 0) return out
    const close = src.indexOf('</button>', open)
    if (close < 0) return out
    const span = src.slice(open, close + '</button>'.length)
    if (span.includes(`aria-label={${CLOSE_ESC}}`)) out.push(span)
    from = close + 1
  }
}

describe('GDK-138 §15 every overlay closes the same way', () => {
  test('a close control names itself once — aria-label and title agree', () => {
    const bad: string[] = []
    for (const p of FILES) {
      for (const span of closeButtons(readFileSync(p, 'utf8'))) {
        if (!span.includes(`title={${CLOSE_ESC}}`)) bad.push(`${rel(p)}: close button has no matching title`)
      }
    }
    expect(bad, bad.join('\n') || '(none)').toEqual([])
  })

  test('a close control draws its × with Icon, never a hand-rolled <svg>', () => {
    const bad: string[] = []
    for (const p of FILES) {
      for (const span of closeButtons(readFileSync(p, 'utf8'))) {
        if (span.includes('<svg')) bad.push(`${rel(p)}: close button hand-rolls its glyph`)
        else if (!span.includes('<Icon name="x"'))
          bad.push(`${rel(p)}: close button draws no Icon name="x"`)
      }
    }
    expect(bad, bad.join('\n') || '(none)').toEqual([])
  })

  test('DialogShell is the only hand-rolled modal', () => {
    const bad = FILES.filter((p) => {
      if (p === SHELL) return false
      const src = readFileSync(p, 'utf8')
      return src.includes('aria-modal="true"') && !src.includes('DialogShell')
    })
      .map(rel)
      // The two sanctioned exceptions, both already named as a different
      // class by DialogShell.test.ts. MediaViewer is a full-bleed bg-black/90
      // lightbox with no panel chrome — it carries the §15 close control,
      // which clauses (1) and (2) above measure. CommandPalette has the
      // backdrop but no header X on purpose (§15): it closes on Esc, on
      // outside click, and on running the thing you came for.
      .filter((f) => f !== 'detail/MediaViewer.svelte' && f !== 'palette/CommandPalette.svelte')
    expect(bad, `modal hand-rolled outside DialogShell: ${bad.join(', ') || '(none)'}`).toEqual(
      [],
    )
  })

  test('the settings tabs are a tablist, not nine buttons', () => {
    // (b) of GDK-138: aria-selected was null, so a screen reader was told
    // which tab is *current* (aria-current, the sidebar's vocabulary) but
    // never which is *selected*, which is the one a tablist promises.
    const src = readFileSync(join(COMPONENTS, 'settings/SettingsDialog.svelte'), 'utf8')
    for (const token of ['role="tablist"', 'role="tab"', 'aria-selected', 'aria-controls', 'role="tabpanel"']) {
      expect(src, `SettingsDialog must carry ${token}`).toContain(token)
    }
  })
})
