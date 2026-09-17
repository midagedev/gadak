import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/*
 * Recurrence layer for GDK-1925: a phone screen grows without seams.
 *
 * The v0.23 audit measured Detail.svelte at 2,319 lines and 52 pieces of
 * component state, having taken on 919 lines that cycle without gaining a
 * single child component. Nothing failed while that happened, because size
 * is the one property no gate here reads — svelte-check reads types, the
 * viewport gate reads geometry, vitest reads behaviour.
 *
 * This is a ratchet, not a budget. Each ceiling is that screen's measurement
 * on the date below, so today's tree is exactly green and tomorrow's growth
 * is red; a screen that gets smaller must lower its own row in the same
 * commit, which is what keeps the ceiling from drifting into permission. It
 * deliberately says nothing about what good structure looks like — a 2,000
 * line screen split into two 1,000 line halves is not obviously better, and
 * this file is not the place to have that argument. It only refuses the
 * growth that happens without anyone deciding to.
 *
 * Both numbers matter and they fail for different reasons. Lines is the
 * reading cost. State count is the coupling: 52 pieces of mutable state in
 * one component is 52 things any handler in it can reach, which is what
 * makes such a file risky to change rather than merely long.
 */

const screensDir = join(dirname(fileURLToPath(import.meta.url)), '..', 'screens')

/** Measured 2026-09-16 on `main` at the v0.23 audit's fix cycle. Lower a row
 *  when a screen shrinks; raising one is a decision, not a fix. */
const CEILINGS: Record<string, { lines: number; state: number }> = {
  // Lowered 2026-09-16 in the extraction round (GDK-1925): the five write
  // sheets moved to ui/detail/, 2,319→1,793 lines and 52→38 $state.
  // Raised 2026-09-16 for GDK-1964/1965: every landed write now announces
  // itself (announceWrite + onWritten's field param) and the transition
  // sheet takes the current status as a prop. 1,793 → 1,817; $state 38.
  'Detail.svelte': { lines: 1817, state: 38 },
  // Raised 2026-09-17 for GDK-1972 (the owner's shell): the hosted refusal
  // dead end — a pairing/scope refusal on a hosted page renders the
  // tailscale-serve sentence instead of the pairing road it cannot take.
  // 1611 → 1619; $state unchanged (the branch reads app.hosted, not state).
  // Raised 2026-09-17 for GDK-1988 (한글 arrived as 자모): the field stops
  // being a write-only sink and becomes a buffer — the hold/flush calls, the
  // Backspace and key-bar boundaries, and the compose wrapper that rides the
  // keyboard band so WebKit is not asked to compose under the keyboard, plus
  // that strip's CSS. The rule itself is pure and lives in
  // lib/terminal/hangul-hold.ts; what is here is the wiring. 1619 → 1686;
  // $state unchanged (the strip is the field, so its content is the state).
  'Shell.svelte': { lines: 1686, state: 18 },
  // Raised 2026-09-16 for GDK-1966 (hosted mode): the hosted connection
  // section (page host + the viewer sentence from GET viewer/) plus hosted
  // guards on the roster read, the scan entries, the terminal endpoint line
  // and both unpair controls. 765 → 827; $state 14 → 15 (the viewer doc).
  // Raised 2026-09-17 (GDK-1970 install hint, 2026-09-17): the home-screen
  // install sentence + its standalone guard. 827 → 837; $state unchanged
  // (display-mode is read once, not state).
  // Raised 2026-09-17 (GDK-1973 declared name): the identity snippet
  // (title, field, hint, Save) rendered in both the paired and hosted
  // branches, the hostedDeclared arm beside the no-viewer sentence, the
  // local save verb, and the field's control CSS. 837 → 924; $state
  // 15 → 16 (the field's draft).
  'Settings.svelte': { lines: 924, state: 16 },
  'PageDetail.svelte': { lines: 522, state: 8 },
  // 2026-09-16, GDK-1936: raised to 497 when the header learned to wrap,
  // then lowered when the revision moved the measurement out to
  // lib/fit-heading.ts (an action, because an $effect writing $state is what
  // GDK-692 forbids) and the wrap came back out. 482 → 497 → 496.
  // Raised 2026-09-17 (GDK-1974 search door, 2026-09-17): the header's
  // magnifier button — template, glyph, and its 44pt rule — beside the
  // create control, plus the two doors (showPalette/showSearch over one
  // preparePalette core). 498 → 538.
  // 2026-09-17, GDK-1974: the stamp words and lib/fit-heading left the
  // header; re-pinned to the measured count. 538 → 528; $state unchanged
  // at 3 (paletteFocus lives in the store, not here).
  // 2026-09-17, GDK-1974: the name steps down through lib/fit-heading
  // before it ellipsizes — the action is back on .head and the two step
  // rules ([data-fit='1'/'2']) joined .name. 528 → 542; $state unchanged.
  // 2026-09-17, GDK-1985: one door — the header's magnifier button and
  // showSearch leave, the chevron becomes the field's own magnifier glyph,
  // and the heading toggles (GDK-1984). 548 → 529; $state unchanged at 3.
  // Raised 2026-09-18 for GDK-1989 (the header's hierarchy, third pass on
  // GDK-1974/1985): the door draws a rule and takes the row's leftover room,
  // the `.spacer` element goes, and the three actions become one set in
  // their own wrapper. Mostly comment and CSS — the round's own recurrence
  // layer is in e2e/heading.spec.ts, not here. Then 559 → 579 for the vision
  // round's two axes (the rule's ink is the strong border, not the row
  // separator's; the set's last gap is 8px, not 2), both comment. $state
  // unchanged. 529 → 579.
  'Issues.svelte': { lines: 579, state: 3 },
  // Measured 2026-09-16 at birth (GDK-1827): the sprint list is the first
  // screen with no local state at all — every row is derived from the store.
  'Sprints.svelte': { lines: 262, state: 0 },
  // Raised 2026-09-16 for GDK-1966 (hosted mode): the gate's hosted branch —
  // the unreachable sentence replaces the pairing form a hosted page cannot
  // complete, and the scan entry is unreachable at its guard. 247 → 260.
  'PairGate.svelte': { lines: 260, state: 4 },
}

/** `let x = $state(...)`, including the typed form `$state<T>(...)` — the
 *  form a plain `$state(` grep misses, which is how a first reading of this
 *  file counted 28 where there are 52. */
const STATE_DECL = /^[ \t]*let[ \t]+\w+[ \t]*(?::[^=]+)?=[ \t]*\$state\b/gm

function screens(): string[] {
  return readdirSync(screensDir).filter((n) => n.endsWith('.svelte')).sort()
}

describe('phone screen size ratchet (GDK-1925)', () => {
  it('every screen has a ceiling, and every ceiling names a screen', () => {
    // A new screen with no row would be unbounded, and a row left behind by a
    // deleted or renamed screen is an exemption nobody is checking.
    expect(screens()).toEqual(Object.keys(CEILINGS).sort())
  })

  it('no screen is larger than its ceiling', () => {
    const over: string[] = []
    for (const name of screens()) {
      const src = readFileSync(join(screensDir, name), 'utf8')
      const lines = src.split('\n').length - (src.endsWith('\n') ? 1 : 0)
      const state = (src.match(STATE_DECL) ?? []).length
      const cap = CEILINGS[name]
      if (lines > cap.lines) over.push(`${name}: ${lines} lines > ceiling ${cap.lines}`)
      if (state > cap.state) over.push(`${name}: ${state} $state > ceiling ${cap.state}`)
    }
    expect(over, over.join('\n')).toEqual([])
  })

  it('a ceiling that has grown past its screen is lowered', () => {
    // The ratchet half. Without this the numbers only ever go up: a round
    // that extracts half of Detail leaves a ceiling with room for it to come
    // back, and the next round's growth is invisible again.
    const slack: string[] = []
    for (const name of screens()) {
      const src = readFileSync(join(screensDir, name), 'utf8')
      const lines = src.split('\n').length - (src.endsWith('\n') ? 1 : 0)
      const state = (src.match(STATE_DECL) ?? []).length
      const cap = CEILINGS[name]
      if (lines < cap.lines) slack.push(`${name}: ${lines} lines, ceiling still ${cap.lines} — lower it`)
      if (state < cap.state) slack.push(`${name}: ${state} $state, ceiling still ${cap.state} — lower it`)
    }
    expect(slack, slack.join('\n')).toEqual([])
  })
})
