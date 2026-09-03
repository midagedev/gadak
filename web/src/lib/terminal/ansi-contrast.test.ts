/*
 * The paper-tuned ANSI sixteen exist for one property (GDK-1358): on the
 * light palette the normal eight clear 4.5:1 and the bright eight 3.0:1,
 * over both grounds a terminal can sit on (bg-base, bg-panel). Nothing
 * gated that — the only pins were two hex literals in an e2e, which held a
 * value rather than the property, and ansi-white slipped to 4.43 on
 * bg-panel unseen (GDK-1375). This reads app.css and measures.
 */
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { TERMINAL_ANSI_VARS } from './protocol'

const HERE = dirname(fileURLToPath(import.meta.url))
const css = readFileSync(resolve(HERE, '../../app.css'), 'utf8')

/** The @theme block: the light palette's values, one per token. */
function lightValue(name: string): string {
  const theme = css.slice(css.indexOf('@theme'), css.indexOf('@layer base'))
  const m = theme.match(new RegExp(`^\\s*${name}:\\s*(#[0-9a-fA-F]{6})`, 'm'))
  if (!m) throw new Error(`${name} has no hex value in the @theme block`)
  return m[1]
}

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
  const lin = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
}

function contrast(a: string, b: string): number {
  const la = luminance(a)
  const lb = luminance(b)
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

describe('paper ANSI palette contrast (GDK-1358)', () => {
  const grounds = ['--color-bg-base', '--color-bg-panel'].map(lightValue)
  const slots = Object.entries(TERMINAL_ANSI_VARS) as [string, string][]
  expect(slots).toHaveLength(16)

  for (const [slot, name] of slots) {
    const floor = slot.startsWith('bright') ? 3.0 : 4.5
    test(`${slot} clears ${floor}:1 on bg-base and bg-panel`, () => {
      const value = lightValue(name)
      for (const ground of grounds) {
        expect(contrast(value, ground), `${slot} ${value} on ${ground}`).toBeGreaterThanOrEqual(floor)
      }
    })
  }
})
