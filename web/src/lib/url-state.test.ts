import { describe, expect, test } from 'vitest'
import { PLACE_PARAM_KEYS, isPlaceParam } from './url-state'
import { VIEW_PARAM_KEYS, isViewParam } from './view-config'

/*
 * The registry contract (task: "URL as state"). Place params and view params
 * are disjoint worlds with different serialization homes — the URL hash hosts
 * both, a saved view hosts only view params. A key in both would ride into
 * every saved view as a phantom axis and shift the sidebar's active-view match.
 */

/** The shape of a gadak:// view-parameter key: `keyPattern` in
 *  internal/deeplink/deeplink.go. A param that cannot survive the scheme is a
 *  param no deep link can express, and the whole point of a place param is
 *  that `gadak://view/w/oss?person=…` works the moment it is registered. */
const KEY_SHAPE = /^[a-z][a-z0-9_.]{0,63}$/

describe('url param registry', () => {
  test('no key is both a place param and a view param', () => {
    const view = new Set<string>(VIEW_PARAM_KEYS)
    const both = PLACE_PARAM_KEYS.filter((k) => view.has(k))
    // A key in both would end up inside saved views.
    expect(both).toEqual([])
  })

  test('isViewParam is false for every place param, isPlaceParam false for every view param', () => {
    for (const key of PLACE_PARAM_KEYS) {
      expect(isViewParam(key), `${key} must not be view state`).toBe(false)
    }
    for (const key of VIEW_PARAM_KEYS) {
      expect(isPlaceParam(key), `${key} must not be place state`).toBe(false)
    }
    // Dynamic field axes are view state too.
    expect(isPlaceParam('f.some_alias')).toBe(false)
  })

  test('every place param can ride a gadak:// link', () => {
    for (const key of PLACE_PARAM_KEYS) {
      expect(key, `${key} must match keyPattern (internal/deeplink)`).toMatch(KEY_SHAPE)
    }
  })

  test('excluded names stay out of the registry', () => {
    /*
     * A tripwire, not a proof: it pins the names someone would actually reach
     * for, not every conceivable bad key. The proof-shaped half of the rule is
     * the type constraint on bindParam/bindParams (lib/url-sync) — a param
     * cannot be bound without being listed in PLACE_PARAM_KEYS, and this list
     * is what a reviewer reads next to the exclusions (lib/url-state).
     */
    const excluded = [
      'newIssue', // write.newIssueOpen — compose flow, phishing surface
      'comment', // triage.commentKey — compose flow
      'writeSettings', // write.settingsOpen — credential form, not the settings place
      'palette', // paletteOpen — keystroke UI
      'shortcuts', // shortcutsOpen — keystroke UI
    ]
    for (const name of excluded) {
      expect(isPlaceParam(name), `${name} must never become a param`).toBe(false)
      expect(PLACE_PARAM_KEYS, `${name} must not be registered`).not.toContain(name)
    }
  })
})
