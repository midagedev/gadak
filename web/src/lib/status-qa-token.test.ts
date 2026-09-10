/*
 * GDK-160: the deploy-QA teal is a palette token, not raw hex.
 *
 * The QA state painted arbitrary Tailwind teals (#2dd4bf/#5eead4) that no
 * palette carries — neon values whose ink-on-tint contrast was 1.23:1 where
 * the house rule for chip micro text is 4.5:1 (app.css). The fix is not a
 * new hex: avatar-5 is the palette's own teal family, so QA wears it as
 * --color-status-qa (light #2a5c54 = 5.32:1 on its own 15% tint, ≥5.45 on
 * every ground; dark #6c9e95 = 5.26:1 on the tint), full ink on an alpha
 * tint like every other status chip (bg-status-X/15 text-status-X).
 *
 * Source sweep, not render: the palette parity itself is theme-check's
 * (app.css blocks); what only this file catches is a *new* raw teal hex
 * arriving in a component — the class of thing theme-check cannot see
 * because it never names the arbitrary value.
 */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB = join(HERE, '..')
const APP_CSS = readFileSync(join(WEB, 'app.css'), 'utf8')
const DEPLOY = readFileSync(join(WEB, 'components/detail/DeployTimeline.svelte'), 'utf8')
const ISSUE_ROW = readFileSync(join(WEB, 'components/list/IssueRow.svelte'), 'utf8')

function sourceFiles(dir: string): string[] {
  const out: string[] = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...sourceFiles(p))
    else if (/\.(svelte|ts|css)$/.test(name)) out.push(p)
  }
  return out
}

describe('QA teal is a token, not raw hex (GDK-160)', () => {
  test('app.css defines --color-status-qa in every palette block', () => {
    // Five blocks: light @theme, dark media, dark explicit, ink, ember.
    // A definition count is the cheap parity floor — theme-check's own
    // parity assertion only covers the tokens its tables name.
    const defs = APP_CSS.match(/--color-status-qa:\s*#[0-9a-f]{6}/g) ?? []
    expect(defs).toHaveLength(5)
    expect(APP_CSS).toMatch(/--color-status-qa:\s*#2a5c54/)
    // One value per family, avatar-5's translation, not five inventions.
    expect(APP_CSS.match(/--color-status-qa:\s*#6c9e95/g)).toHaveLength(4)
  })

  test('the deploy timeline consumes the token', () => {
    expect(DEPLOY).toContain('border-status-qa bg-status-qa')
    expect(DEPLOY).toContain('text-status-qa')
  })

  test('the list chips consume the token like every other status chip', () => {
    // House idiom: full ink on an alpha tint (IssueRow qaImpact chips).
    expect(ISSUE_ROW).toContain("'bg-status-qa/15 text-status-qa'")
    expect(ISSUE_ROW).toContain("'bg-status-qa/8 text-status-qa'")
    expect(ISSUE_ROW).toMatch(/rounded-full bg-status-qa["' ]/)
  })

  test('no arbitrary teal hex survives anywhere in web/src', () => {
    // The two hexes the issue named, plus the teal-950 inner-dot value the
    // neon disc carried — a fix that leaves any of the three keeps a color
    // no palette can translate. This file is exempt: naming them is its job.
    const self = fileURLToPath(import.meta.url)
    const banned = /#(2dd4bf|5eead4|083344)/i
    const offenders = sourceFiles(WEB)
      .filter((p) => p !== self)
      .filter((p) => banned.test(readFileSync(p, 'utf8')))
    expect(offenders, `raw teal hex in: ${offenders.join(', ')}`).toEqual([])
  })
})
