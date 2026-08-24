/*
 * GDK-786 dataColors lookups. The snapshot is written only through the
 * listener hook user-tokens.ts exposes (applyUserTokens → setUiTokenSnapshot),
 * so these tests drive the same path the app does and pin the wiring the
 * Playwright live-reflect spec asserts end to end: reads go through $state,
 * so chips re-tint when a settings write lands with no reload.
 */
import { describe, expect, it } from 'vitest'

import { applyUserTokens, type UiTokenDoc } from '../lib/user-tokens'
import {
  labelChipTint,
  labelColor,
  setUiTokenSnapshot,
  statusCategoryColor,
  typeChipTint,
  typeColor,
} from './ui-tokens.svelte'

function docWith(dataColors: UiTokenDoc['dataColors']): UiTokenDoc {
  return { vars: {}, dataColors }
}

describe('ui-tokens store', () => {
  it('receives applyUserTokens output through the listener hook', () => {
    applyUserTokens(null)
    expect(labelColor('urgent')).toBeUndefined()
    applyUserTokens(docWith({ label: { urgent: '#c03030' } }))
    expect(labelColor('urgent')).toBe('#c03030')
    applyUserTokens(null)
  })

  it('looks up by the stable key kind per family', () => {
    setUiTokenSnapshot(docWith({ type: { '10007': '#d07020' }, status: { inprogress: '#7e5904' } }))
    expect(typeColor('10007')).toBe('#d07020')
    // Display names are never keys — the lookup must not "helpfully" find
    // anything for them.
    expect(typeColor('Task')).toBeUndefined()
    expect(statusCategoryColor('inprogress')).toBe('#7e5904')
    expect(statusCategoryColor('In Progress')).toBeUndefined()
    expect(typeColor(null)).toBeUndefined()
    setUiTokenSnapshot(null)
  })

  it('tints chips at ~18% alpha, expanding 3-digit hex first', () => {
    setUiTokenSnapshot(docWith({ label: { urgent: '#c03030', tiny: '#abc' }, type: { '10007': '#d07020' } }))
    expect(labelChipTint('urgent')).toBe('#c030302e')
    expect(labelChipTint('tiny')).toBe('#aabbcc2e')
    expect(labelChipTint('unset')).toBeUndefined()
    expect(typeChipTint('10007')).toBe('#d070202e')
    expect(typeChipTint(undefined)).toBeUndefined()
    setUiTokenSnapshot(null)
  })
})
