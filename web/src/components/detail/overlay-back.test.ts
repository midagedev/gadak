import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

/*
 * GDK-729: panels open and close symmetrically. When the right-hand column
 * covers the list (overlay regime, narrow windows) every panel in it offers
 * the same way back — the issue panel had the feed.backToList arrow while
 * documents and people offered only a dialog X, a different closing grammar
 * on the same surface. This pins that all three carry the arrow, keyed off
 * the one regime owner (viewport-regime.svelte module state — a component
 * subscribing for itself is the duplication GDK-696 banned).
 *
 * Source sweep: the gate is the overlay-gated button in the source; the
 * runtime half (the button appears at a narrow viewport and closes the
 * panel) is e2e's to measure.
 */
const HERE = dirname(fileURLToPath(import.meta.url))

const PANELS = {
  issue: {
    source: readFileSync(join(HERE, 'DetailPanel.svelte'), 'utf8'),
    testid: 'issue-detail-back',
    clear: "selection.clear('detail-back')",
  },
  doc: {
    source: readFileSync(join(HERE, 'DocumentPanel.svelte'), 'utf8'),
    testid: 'doc-panel-back',
    clear: 'pages.clear()',
  },
  person: {
    source: readFileSync(join(HERE, 'PersonPanel.svelte'), 'utf8'),
    testid: 'person-panel-back',
    clear: 'person.clear()',
  },
} as const

test.each(Object.entries(PANELS))('%s panel carries the overlay way back (GDK-729)', (_name, p) => {
  const at = p.source.indexOf(`data-testid="${p.testid}"`)
  expect(at, `${p.testid} must exist`).toBeGreaterThan(-1)
  const tag = p.source.slice(p.source.lastIndexOf('<', at), p.source.indexOf('>', at) + 1)
  // Same grammar as the issue panel: the shared label, the left arrow,
  // closing the panel through the store's clear. The icon renders in the
  // button body, just past the open tag.
  expect(tag).toContain("t('feed.backToList')")
  expect(p.source.slice(at, at + 500)).toContain('arrow-left')
  expect(tag).toContain(p.clear)
})

test('the panels key the arrow off the one regime owner, not a private copy', () => {
  for (const p of Object.values(PANELS)) {
    expect(p.source, 'module state is the single live regime (GDK-696)').toContain(
      "from '../../lib/viewport-regime.svelte'",
    )
    expect(p.source).toMatch(/viewport\.regime === 'overlay'/)
    // Composed so this file does not itself trip the GDK-696 sweep, which
    // scans for the same literal.
    const privateSub = 'subscribeViewport' + 'Regime'
    expect(p.source, 'no private subscription (GDK-696 sweep)').not.toContain(privateSub)
  }
})
