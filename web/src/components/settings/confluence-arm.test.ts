/*
 * GDK-1134: the Confluence turn-on arm, as behaviour rather than as the
 * $effect that used to clear the latch.
 *
 * vitest unit is environment:'node' with no svelte plugin, so SourcesTab
 * cannot be mounted (FeaturesTab.test.ts). The point of moving the rule into
 * draft.ts is that the interesting part no longer needs a mount: these walk
 * the click sequences a person can perform and assert the label the button
 * shows and the draft the save path would send.
 *
 * The sequences are the ones the $effect existed for — arm, then let a space
 * arrive; arm, turn the source off again — because those are exactly where a
 * derivation and a synchronization can disagree.
 */
import { describe, expect, test } from 'vitest'
import {
  confluenceTurnOnClick,
  emptyDraft,
  isConfluenceArmed,
  isConfluenceEffective,
  toSettings,
  type SettingsDraft,
} from './draft'

/** The two inputs SourcesTab binds, in a draft the save path also accepts. */
function scope(confluenceOn: boolean, spaces: string[]): SettingsDraft {
  return { ...emptyDraft(), confluenceOn, spaces }
}

/** What SourcesTab does on a click: the latch is the only state it keeps. */
function click(latch: boolean, d: SettingsDraft): boolean {
  const next = confluenceTurnOnClick(latch, d)
  if (next.turnOn) d.confluenceOn = true
  return next.latch
}

describe('GDK-476 two-click arm for "mirror every team space"', () => {
  test('an empty scope needs two clicks, and the first one only arms', () => {
    const d = scope(false, [])
    let latch = false

    latch = click(latch, d)
    expect(isConfluenceArmed(latch, d), 'first click arms the confirm label').toBe(true)
    expect(d.confluenceOn, 'and commits nothing').toBe(false)
    expect(isConfluenceEffective(d)).toBe(false)

    latch = click(latch, d)
    expect(d.confluenceOn, 'second click turns the source on').toBe(true)
    expect(isConfluenceArmed(latch, d), 'and the arm is spent').toBe(false)
    expect(toSettings(d, true).confluence).toEqual({ enabled: true, spaces: [] })
  })

  test('a picked space turns it on in one click — there is nothing to confirm', () => {
    const d = scope(false, ['ENG'])
    const latch = click(false, d)
    expect(d.confluenceOn).toBe(true)
    expect(isConfluenceArmed(latch, d)).toBe(false)
    expect(toSettings(d, true).confluence).toEqual({ enabled: true, spaces: ['ENG'] })
  })

  test('a space arriving after the arm disarms it — the old $effect’s job', () => {
    const d = scope(false, [])
    const latch = click(false, d)
    expect(isConfluenceArmed(latch, d)).toBe(true)

    // The ScopePicker writes the draft directly; nothing re-runs the click.
    d.spaces = ['ENG']
    expect(isConfluenceArmed(latch, d), 'the confirm label is gone').toBe(false)
    expect(isConfluenceEffective(d), 'and the button has swapped to turn-off').toBe(true)
  })

  test('turning the source off after arming leaves the arm spent, not stuck', () => {
    const d = scope(false, [])
    let latch = click(false, d)
    d.spaces = ['ENG']
    expect(isConfluenceArmed(latch, d)).toBe(false)

    // The turn-off control clears both inputs (SourcesTab).
    d.confluenceOn = false
    d.spaces = []
    expect(
      isConfluenceArmed(latch, d),
      'a latch left true would make the next turn-on commit on one click',
    ).toBe(true)

    // Which is why the click that armed it is the one that must be spent:
    // the arm survives only because nothing consumed it. Consume it.
    latch = click(latch, d)
    expect(d.confluenceOn).toBe(true)
  })

  test('the source is on if either input says so — one rule, two readers', () => {
    for (const [on, spaces] of [
      [false, []],
      [true, []],
      [false, ['ENG']],
      [true, ['ENG']],
    ] as const) {
      const d = scope(on, [...spaces])
      expect(isConfluenceEffective(d), `confluenceOn=${on} spaces=${spaces.length}`).toBe(
        toSettings(d, true).confluence?.enabled ?? false,
      )
    }
  })
})
