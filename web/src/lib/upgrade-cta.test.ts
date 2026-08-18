import { describe, expect, test } from 'vitest'
import { upgradeCta } from './upgrade-cta'

/*
 * GDK-216: one owner answers "what does this install run to upgrade".
 * darwin is the only published package path (brew cask). linux/windows
 * have none yet (AUR unpublished, Scoop out of scope) — a command here
 * would be a lie. Empty os is static export / hosted demo / unknown.
 */

describe('upgradeCta', () => {
  test('darwin is brew cask', () => {
    expect(upgradeCta('darwin').command).toBe('brew upgrade --cask gadak')
  })

  test('linux has no command', () => {
    expect(upgradeCta('linux').command).toBeNull()
  })

  test('windows has no command', () => {
    expect(upgradeCta('windows').command).toBeNull()
  })

  test('empty os has no command', () => {
    expect(upgradeCta('').command).toBeNull()
  })

  test('unknown os has no command', () => {
    expect(upgradeCta('freebsd').command).toBeNull()
  })
})
