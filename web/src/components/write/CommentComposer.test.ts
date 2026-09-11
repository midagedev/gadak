/*
 * GDK-528: the composer's restriction controls. There is no component-mount
 * harness (vitest is node, no svelte plugin), so this reads the source the
 * way NewIssueDialog.test.ts does. The request shape itself is pinned in
 * lib/api.comment-restriction.test.ts; the journey (controls → POST body →
 * badge) is Playwright's job (e2e/comment-visibility.spec.ts).
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(join(HERE, 'CommentComposer.svelte'), 'utf8')

describe('CommentComposer restriction (GDK-528)', () => {
  test('controls are keyed on the origin type the server stated, never identity or kind', () => {
    // GDK-1152: the branch asks isJiraFamily(config().originType). The
    // capability gate scans for the banned vocabulary globally; this pins
    // the positive side — the expression that owns the branch.
    expect(SRC).toContain('isJiraFamily(config().originType)')
  })

  test('controls exist with stable testids and live inside the canRestrict branch', () => {
    const ifAt = SRC.indexOf('{#if canRestrict}')
    expect(ifAt).toBeGreaterThan(-1)
    const block = SRC.slice(ifAt)
    for (const tid of ['comment-restriction', 'comment-visibility-kind', 'comment-visibility-value', 'comment-internal']) {
      expect(SRC.includes(`data-testid="${tid}"`), tid).toBe(true)
      expect(block.includes(`data-testid="${tid}"`), `${tid} must sit inside the branch`).toBe(true)
    }
  })

  test('a chosen kind with no name blocks submit instead of posting public', () => {
    // Two seats, both required: the footer button and submit()'s own guard —
    // the footer alone would still let ⌘↵ through.
    expect(SRC).toContain('!canSubmit || restrictionIncomplete')
    expect(SRC).toContain('if (restrictionIncomplete) return')
    // And the block is derivable only while a kind is chosen with an empty
    // name — an untouched composer must never be blocked.
    expect(SRC).toContain("visibilityKind !== '' && !visibilityValue.trim()")
  })

  test('an empty visibility value never rides the request', () => {
    // opts carries visibility only when a kind is chosen AND a name is
    // filled; the api layer would send whatever it is given, so the guard
    // lives here.
    expect(SRC).toContain("visibilityKind !== '' && value")
  })

  test('restriction resets with the issue draft, not with each submit', () => {
    // The key-change effect resets text/mentions/attachments and the
    // restriction together; submit() clears only text/mentions/attachments —
    // an internal triage run posts several internal comments in a row.
    const effect = SRC.slice(SRC.indexOf('Per-issue draft'), SRC.indexOf('queueMicrotask'))
    for (const reset of ['visibilityKind = \'\'', 'visibilityValue = \'\'', 'internal = false']) {
      expect(effect.includes(reset), reset).toBe(true)
    }
    const submitBody = SRC.slice(SRC.indexOf('async function submit'), SRC.indexOf('function onKeydown'))
    expect(submitBody.includes('visibilityKind =')).toBe(false)
  })

  test('submit hands the opts to the store', () => {
    expect(SRC).toMatch(/write\.submitComment\(issueKey, body, used, prev\.attachments, opts\)/)
  })
})
