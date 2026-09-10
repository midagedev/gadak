/*
 * GDK-1092 B-6: a settled empty state that owns a whole surface centres in it.
 *
 * The issue panel's "not found" sentence rendered at `px-5 py-16` — the top of
 * a 830px scroller, about 8% down it — with the rest of the panel blank. Every
 * other empty state in the app is EmptyState.svelte, which is
 * `h-full … justify-center`, so the same kind of message sat in two different
 * places depending on which surface drew it. The document panel had a second
 * copy of the same top-anchored block.
 *
 * The fix is not a padding number: both panels render EmptyState, which is the
 * one owner of what a settled empty surface looks like. This gate keeps the
 * copies from coming back — it fails on a detail-panel error/empty branch that
 * lays out its own centred-ish block instead of calling EmptyState.
 *
 * FAIL-first output on the source before this round is in the round report.
 */
import { readFileSync } from 'node:fs'
import { describe, expect, test } from 'vitest'

/** The panels that own a whole scroller and can settle on "nothing here". */
const PANELS = ['DetailPanel.svelte', 'DocumentPanel.svelte'] as const

/** A block that pads a message down from the top of its scroller instead of
 *  centring in it. `py-16` is the exact literal both copies used; the class is
 *  "vertical padding as a stand-in for centring". */
const TOP_ANCHORED = /class="[^"]*\bflex flex-col items-center[^"]*\bpy-\d+\b[^"]*"/g

describe('GDK-1092 B-6: panel empty/error states are centred by EmptyState', () => {
  test.each(PANELS)('%s has no hand-laid top-anchored message block', (file) => {
    const src = readFileSync(new URL(`./${file}`, import.meta.url), 'utf8')
    const hits = src.match(TOP_ANCHORED) ?? []
    expect(
      hits,
      `${file}: a settled empty/error state must render <EmptyState> (h-full, justify-center), ` +
        `not a padded column of its own — otherwise the same sentence lands at 8% of one ` +
        `surface and at 50% of another`,
    ).toEqual([])
  })

  test.each(PANELS)('%s renders EmptyState', (file) => {
    const src = readFileSync(new URL(`./${file}`, import.meta.url), 'utf8')
    expect(src).toMatch(/import EmptyState from '\.\.\/list\/EmptyState\.svelte'/)
    expect(src).toMatch(/<EmptyState/)
  })
})
