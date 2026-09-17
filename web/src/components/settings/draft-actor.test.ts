/*
 * The declared-name field's half of the settings form model (GDK-1973).
 *
 * The field has exactly three input situations — a person block stored, an
 * agent block stored, no block — and exactly three verbs on the wire: set
 * ({name, kind:'person'}), clear ({}), preserve (key absent). Which verb an
 * emptied field means is decided by what GET carried: only a person's block
 * is cleared by an empty name; an agent's (or no block) is preserved by
 * omitting the key, so a person emptying the field can never knock an
 * agent's identity off the tracker.
 *
 * FAIL-first: before this change neither toDraft nor toSettings knew the
 * actor key at all — GET carried no block (server side of this round) and
 * every PUT omitted it, so the field could not round-trip anything.
 */
import { describe, expect, test } from 'vitest'
import { actorPayload, toDraft, toSettings } from './draft'

describe('declared-name draft: GET → form', () => {
  test('a person block fills the field, an agent block leaves it empty, no block stays empty', () => {
    expect(toDraft({ actor: { slug: 'person:dana', name: 'Dana', kind: 'person' } })).toMatchObject({
      actorName: 'Dana',
      actorKind: 'person',
    })
    expect(toDraft({ actor: { kind: 'agent', name: 'bot' } })).toMatchObject({
      actorName: '',
      actorKind: 'agent',
    })
    expect(toDraft({})).toMatchObject({ actorName: '', actorKind: '' })
  })
})

describe('declared-name draft: form → PUT', () => {
  test('a typed name sends the set verb', () => {
    const d = toDraft({})
    d.actorName = '  Dana Kim  '
    expect(actorPayload(d, true)).toEqual({ name: 'Dana Kim', kind: 'person' })
    expect(toSettings(d, false, true).actor).toEqual({ name: 'Dana Kim', kind: 'person' })
  })

  test('an emptied field clears a person block', () => {
    const d = toDraft({ actor: { slug: 'person:dana', name: 'Dana', kind: 'person' } })
    d.actorName = '   '
    expect(actorPayload(d, true)).toEqual({})
    expect(toSettings(d, false, true).actor).toEqual({})
  })

  test('an emptied field preserves an agent block — the key is absent, not {}', () => {
    const d = toDraft({ actor: { kind: 'agent', name: 'bot' } })
    d.actorName = ''
    expect(actorPayload(d, true)).toBeUndefined()
    expect('actor' in toSettings(d, false, true)).toBe(false)
  })

  test('an untouched empty field sends no actor key at all', () => {
    const d = toDraft({})
    expect(actorPayload(d, true)).toBeUndefined()
    expect('actor' in toSettings(d, false, true)).toBe(false)
  })

  test('a non-gadak origin never sends the key, even with a typed name', () => {
    const d = toDraft({ actor: { slug: 'person:dana', name: 'Dana', kind: 'person' } })
    d.actorName = 'Renamed'
    expect(actorPayload(d, false)).toBeUndefined()
    expect('actor' in toSettings(d, false, false)).toBe(false)
  })

  test('round-trip: a set verb stores a block the next read can clear', () => {
    const set = toSettings(toDraft({ actor: { slug: 'person:dana', name: 'Dana', kind: 'person' } }), false, true)
    expect(set.actor).toEqual({ name: 'Dana', kind: 'person' })
    // The saved document re-reads as a person block, so the next save can
    // clear it by emptying the field.
    const red = toDraft(set)
    expect(red).toMatchObject({ actorName: 'Dana', actorKind: 'person' })
    red.actorName = ''
    expect(toSettings(red, false, true).actor).toEqual({})
  })
})
