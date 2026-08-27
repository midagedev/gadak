/*
 * GDK-1055: the feed's "Field change" detail must show field display
 * names, not raw changelog ids ("Field change issueparentassociation").
 *
 * No component-mount harness: vitest is environment:'node' and the unit
 * project loads no svelte plugin, so importing .svelte fails outright
 * (favorites-labels.test.ts documents the same limit). What this file
 * proves: the catalog owner (fieldLabel + field.*) resolves the ids the
 * feed ships, and the PersonalFeed source routes payload.fields through
 * that owner — no raw passthrough, no second mapping table.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { fieldLabel } from '../../lib/i18n'
import { en, ja, ko } from '../../lib/i18n/catalog'

const HERE = dirname(fileURLToPath(import.meta.url))
const FEED = join(HERE, 'PersonalFeed.svelte')

describe('GDK-1055 feed field names go through the catalog owner', () => {
  test('fieldLabel resolves the raw changelog ids the feed ships', () => {
    // Node has no localStorage and navigator.language is en*, so the
    // active locale here is en.
    expect(fieldLabel('issueparentassociation')).toBe('Parent')
    expect(fieldLabel('labels')).toBe('Labels')
    // Composed the way the feed composes payload.fields.
    expect(['issueparentassociation', 'labels'].map(fieldLabel).join(', ')).toBe('Parent, Labels')
  })

  test('field.issueparentassociation is present in every locale', () => {
    expect(en['field.issueparentassociation']).toBe('Parent')
    expect(ko['field.issueparentassociation']).toBe('상위 항목')
    expect(ja['field.issueparentassociation']).toBe('親課題')
  })

  test('PersonalFeed maps payload.fields through fieldLabel, never raw', () => {
    const src = readFileSync(FEED, 'utf8')
    expect(src).toMatch(/import \{ t, fieldLabel \} from '\.\.\/\.\.\/lib\/i18n'/)
    // Both branches (payload.fields[] and legacy changes[].label) map.
    expect(src).toMatch(/typeof f === 'string' \? fieldLabel\(f\) : ''/)
    expect(src).toMatch(/typeof label === 'string' \? fieldLabel\(label\) : ''/)
    // The pre-GDK-1055 raw passthrough must stay gone.
    expect(src).not.toMatch(/typeof f === 'string' \? f : ''/)
  })
})
