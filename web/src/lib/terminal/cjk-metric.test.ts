/*
 * GDK-1597 — the CJK cell fit, unit half.
 *
 * The perceptual half is tools/cjk-cell-fit.mjs, which renders a real pane
 * and measures pixels; it cannot run here (no fonts, no canvas, and the
 * verdicts are about this platform's font resolution). What *is* pinnable
 * in node is the arithmetic and the shape of what gets written to <head> —
 * the parts that decide whether the measured correction is the right one.
 *
 * The numbers in these cases are the measured ones (2026-09-09, macOS,
 * Chromium and WebKit agreeing): Menlo advances 'W' 0.60205em, Apple SD
 * Gothic Neo advances '가' 0.865em, Hiragino Sans advances 'あ' 1.0em.
 */
import { describe, expect, it } from 'vitest'
import {
  CJK_METRIC_MAX_ADJUST,
  CJK_METRIC_SCRIPTS,
  CJK_METRIC_UNICODE_RANGE,
  cjkMetricUnicodeRange,
  cjkMetricAdjust,
  cjkMetricFaceCss,
  cjkMetricOrder,
  installCjkMetricFaces,
  pickCjkMetricFace,
  withCjkMetricFamilies,
} from './cjk-metric'

const MENLO_W = 0.60205
const HANGUL = 0.865
const KANA = 1.0

describe('cjkMetricAdjust', () => {
  it('scales a Hangul face onto exactly two Menlo cells', () => {
    const pct = cjkMetricAdjust(MENLO_W, HANGUL)
    expect(pct).toBe(139.202)
    // The point of the number: the scaled advance is two cells, so xterm has
    // nothing left to pad with.
    expect((HANGUL * pct!) / 100).toBeCloseTo(MENLO_W * 2, 5)
  })

  it('scales a kana face onto the same two cells with its own factor', () => {
    const pct = cjkMetricAdjust(MENLO_W, KANA)
    expect(pct).toBe(120.41)
    expect((KANA * pct!) / 100).toBeCloseTo(MENLO_W * 2, 5)
  })

  it('is a ratio, so a narrower Latin lead gets a smaller factor', () => {
    // Consolas, where ui-monospace resolves to it and Menlo is absent. A
    // constant tuned to Menlo would overshoot here — this is the whole
    // reason the factor is measured at runtime instead of written down.
    const consolas = 0.5498
    const pct = cjkMetricAdjust(consolas, HANGUL)!
    expect(pct).toBeLessThan(cjkMetricAdjust(MENLO_W, HANGUL)!)
    expect((HANGUL * pct) / 100).toBeCloseTo(consolas * 2, 5)
  })

  it('declares nothing for a face that already fills two cells', () => {
    // A 1:2 design (Sarasa Mono, Noto Sans Mono CJK, Osaka-Mono): correcting
    // it could only add a rounding error.
    expect(cjkMetricAdjust(0.5, 1.0)).toBeNull()
  })

  it('declares nothing when there was nothing to measure', () => {
    expect(cjkMetricAdjust(0, HANGUL)).toBeNull()
    expect(cjkMetricAdjust(MENLO_W, 0)).toBeNull()
    expect(cjkMetricAdjust(Number.NaN, HANGUL)).toBeNull()
    expect(cjkMetricAdjust(MENLO_W, Number.POSITIVE_INFINITY)).toBeNull()
  })
})

describe('CJK_METRIC_UNICODE_RANGE', () => {
  it('leaves the box-drawing block to the Latin lead (GDK-1043)', () => {
    // U+2500-257F is what the Menlo-first order was tuned for. A scaled
    // face reaching those glyphs breaks the grid the way a sans-class
    // fallback did, so the range must not cover them.
    expect(CJK_METRIC_UNICODE_RANGE).not.toMatch(/U\+25/)
    expect(CJK_METRIC_UNICODE_RANGE).not.toMatch(/U\+2580/)
  })

  it('covers the scripts the recordings actually show', () => {
    expect(CJK_METRIC_UNICODE_RANGE).toContain('U+AC00-D7AF') // Hangul syllables
    expect(CJK_METRIC_UNICODE_RANGE).toContain('U+3040-30FF') // kana
    expect(CJK_METRIC_UNICODE_RANGE).toContain('U+4E00-9FFF') // ideographs
  })

  it('gives each script its own blocks and shares only Han', () => {
    // The split is what stops a Korean face from drawing the kana on a
    // Korean page: with one shared range the first family drew both, and
    // the corrected kana row measured worse than the uncorrected one.
    const ko = cjkMetricUnicodeRange('hangul')
    const ja = cjkMetricUnicodeRange('kana')
    expect(ko).toContain('U+AC00-D7AF')
    expect(ko).not.toContain('U+3040-30FF')
    expect(ja).toContain('U+3040-30FF')
    expect(ja).not.toContain('U+AC00-D7AF')
    // Han is contested on purpose — the family order is what resolves it.
    expect(ko).toContain('U+4E00-9FFF')
    expect(ja).toContain('U+4E00-9FFF')
    // And neither reaches the box-drawing block.
    for (const r of [ko, ja]) expect(r).not.toMatch(/U\+25/)
  })
})

describe('CJK_METRIC_SCRIPTS', () => {
  it('names every face twice, PostScript name first', () => {
    // Chromium's local() matches only the PostScript/full name; WebKit
    // matches either (measured on both). A list with only family names
    // resolves nothing on Chromium and the correction silently does not
    // happen — no error, no gate, just the defect still there.
    for (const script of Object.values(CJK_METRIC_SCRIPTS)) {
      for (const [label, ...names] of script.candidates) {
        expect(names.length, `${label} has no local() name`).toBeGreaterThan(0)
        // A face whose PostScript name differs from its family name must
        // carry both; one whose names coincide (AppleGothic, Meiryo,
        // NanumGothic) legitimately carries one.
        if (label.includes(' ')) expect(names).toContain(label)
      }
    }
    expect(CJK_METRIC_SCRIPTS.hangul.candidates.flat()).toContain('AppleSDGothicNeo-Regular')
    expect(CJK_METRIC_SCRIPTS.kana.candidates.flat()).toContain('HiraginoSans-W3')
  })

  it('keeps each script’s list to that script’s own faces', () => {
    // Arial Unicode MS measures 1.0em and would fit beautifully; it is a
    // pan-Unicode face and would draw Korean hanja and Japanese kanji with
    // one set of shapes, which is the defect GDK-1532 closed.
    const all = Object.values(CJK_METRIC_SCRIPTS).flatMap((s) => s.candidates.flat())
    expect(all).not.toContain('Arial Unicode MS')
    expect(all).not.toContain('ArialUnicodeMS')
    const hangul = CJK_METRIC_SCRIPTS.hangul.candidates.flat().join(' ')
    expect(hangul).not.toMatch(/Hiragino|Yu ?Gothic|Meiryo|CJK JP/)
    const kana = CJK_METRIC_SCRIPTS.kana.candidates.flat().join(' ')
    expect(kana).not.toMatch(/Gothic A1|Nanum|Malgun|CJK KR|Apple ?SD/)
  })
})

describe('pickCjkMetricFace', () => {
  // The measured advances, 2026-09-09 macOS. AppleGothic and Apple SD
  // Gothic Neo are both installed; only one of them can be corrected
  // without collapsing the side bearings.
  const ADV: Record<string, number> = {
    AppleGothic: 1.0,
    'AppleSDGothicNeo-Regular': 0.865,
    NanumGothic: 0.94,
  }
  const measure = (family: string, _t: string) => {
    for (const [name, adv] of Object.entries(ADV)) if (family.includes(name)) return adv
    return 0.7 // the absent baseline, and what an uninstalled family answers
  }
  const CANDS = CJK_METRIC_SCRIPTS.hangul.candidates

  it('takes the face that needs the least correction, not the first listed', () => {
    const got = pickCjkMetricFace(CANDS, MENLO_W, '가', measure)
    expect(got?.label).toBe('AppleGothic')
    expect(got?.adjustPct).toBe(120.41)
  })

  it('refuses a correction past the cap rather than collide the strokes', () => {
    // Apple SD Gothic Neo alone: 139.202% is what it needs and 139.202% is
    // what makes 라벨별 read as 라멜멸.
    const only = CANDS.filter(([l]) => l === 'Apple SD Gothic Neo')
    expect(pickCjkMetricFace(only, MENLO_W, '가', measure)).toBeNull()
    // ... and it is admitted when the cap is lifted, so the cap is what
    // rejected it and not an absence.
    expect(pickCjkMetricFace(only, MENLO_W, '가', measure, 200)?.adjustPct).toBe(139.202)
  })

  it('is null when nothing is installed', () => {
    expect(pickCjkMetricFace(CANDS, MENLO_W, '가', () => 0.7)).toBeNull()
  })

  it('has a cap above the widest good measurement and below the bad one', () => {
    expect(CJK_METRIC_MAX_ADJUST).toBeGreaterThan(128.096) // NanumGothic: clean
    expect(CJK_METRIC_MAX_ADJUST).toBeLessThan(139.202) // Apple SD Gothic Neo: collides
  })
})

describe('cjkMetricFaceCss', () => {
  it('writes one local()-only @font-face — no network', () => {
    const css = cjkMetricFaceCss('Test Face', ['A-Regular', 'A'], 139.202, 'U+AC00-D7AF')
    expect(css).toContain("font-family:'Test Face'")
    expect(css).toContain("src:local('A-Regular'), local('A')")
    expect(css).toContain('size-adjust:139.202%')
    expect(css).toContain('unicode-range:U+AC00-D7AF')
    expect(css).not.toMatch(/url\(|https?:/)
  })
})

describe('cjkMetricOrder', () => {
  it('puts the reader’s own script first so shared Han follows lang', () => {
    expect(cjkMetricOrder('ja-JP')).toEqual(['kana', 'hangul'])
    expect(cjkMetricOrder('ja')).toEqual(['kana', 'hangul'])
    expect(cjkMetricOrder('ko-KR')).toEqual(['hangul', 'kana'])
    expect(cjkMetricOrder('en-US')).toEqual(['hangul', 'kana'])
    expect(cjkMetricOrder(null)).toEqual(['hangul', 'kana'])
    // 'jav' is Javanese, not Japanese.
    expect(cjkMetricOrder('jav')).toEqual(['hangul', 'kana'])
  })
})

describe('withCjkMetricFamilies', () => {
  it('puts the corrected families ahead of the stack', () => {
    expect(withCjkMetricFamilies('Menlo, monospace', ['A', 'B'])).toBe("'A', 'B', Menlo, monospace")
  })

  it('does not stack up on a second call', () => {
    const once = withCjkMetricFamilies('Menlo, monospace', ['A'])
    expect(withCjkMetricFamilies(once, ['A'])).toBe(once)
  })

  it('returns the stack untouched when there is nothing to add', () => {
    expect(withCjkMetricFamilies('Menlo, monospace', [])).toBe('Menlo, monospace')
  })
})

/*
 * A Document stand-in. The unit projects run in node with no DOM (there is
 * no jsdom in this repo and adding one to assert five DOM calls would put a
 * native dependency in the lockfile), and installCjkMetricFaces touches
 * exactly five things: createElement, getElementById, head.appendChild,
 * documentElement.lang, and a canvas 2d context it is allowed not to find.
 */
function fakeDoc(lang = 'ko-KR') {
  const byId = new Map<string, { id: string; textContent: string }>()
  const appended: unknown[] = []
  return {
    documentElement: { lang },
    head: { appendChild: (el: { id: string }) => { appended.push(el); byId.set(el.id, el as never) } },
    getElementById: (id: string) => byId.get(id) ?? null,
    createElement: (tag: string) =>
      // No canvas here, so canvasAdvanceMeasure() finds nothing and the
      // injected measure() is what runs — which is the point of injecting it.
      tag === 'canvas' ? {} : { id: '', textContent: '' },
    appended,
  } as unknown as Document & { appended: unknown[] }
}

describe('installCjkMetricFaces', () => {
  const STACK = "Menlo, 'Apple SD Gothic Neo', monospace"

  /** A measure() stand-in modelling this machine: Menlo leads, AppleGothic
   *  and Hiragino are the faces that need the least correction, everything
   *  else answers the absent baseline. */
  const measure = (family: string, text: string) => {
    if (text === 'W') return MENLO_W
    if (family.includes('AppleGothic')) return 1.0
    if (family.includes('AppleSDGothicNeo')) return HANGUL
    if (family.includes('Hiragino')) return KANA
    return 0.7
  }

  it('declares both faces and puts them in front', () => {
    const doc = fakeDoc('ko-KR')
    const out = installCjkMetricFaces({ stack: STACK, doc, measure })
    expect(out).toBe(`'Gadak Terminal Hangul', 'Gadak Terminal Kana', ${STACK}`)
    const css = doc.getElementById('gadak-cjk-metric-faces')!.textContent!
    // Both at 120.41%: AppleGothic and Hiragino each advance 1.0em, which
    // is the least-correction pick for their script.
    expect(css).toContain("src:local('AppleGothic')")
    expect(css).toContain("local('HiraginoSans-W3')")
    expect(css.match(/size-adjust:120\.41%/g)).toHaveLength(2)
    expect(css).not.toContain('139.202')
    expect(css).not.toMatch(/url\(|https?:/)
  })

  it('follows the document language for the shared Han block', () => {
    const out = installCjkMetricFaces({ stack: STACK, doc: fakeDoc('ja-JP'), measure })
    expect(out.indexOf('Gadak Terminal Kana')).toBeLessThan(out.indexOf('Gadak Terminal Hangul'))
  })

  it('takes an explicit lang over the document’s', () => {
    const out = installCjkMetricFaces({ stack: STACK, doc: fakeDoc('ko-KR'), lang: 'ja-JP', measure })
    expect(out.indexOf('Gadak Terminal Kana')).toBeLessThan(out.indexOf('Gadak Terminal Hangul'))
  })

  it('rewrites one <style> in place rather than stacking rules', () => {
    const doc = fakeDoc()
    installCjkMetricFaces({ stack: STACK, doc, measure })
    installCjkMetricFaces({ stack: STACK, doc, measure })
    expect(doc.appended).toHaveLength(1)
    expect(
      doc.getElementById('gadak-cjk-metric-faces')!.textContent!.match(/@font-face/g),
    ).toHaveLength(2)
  })

  it('hands back the stack untouched when no CJK face is installed', () => {
    // Every probe answers what the absent baseline answers: the machine has
    // none of these faces, so there is no advance to correct against.
    const absent = (_f: string, text: string) => (text === 'W' ? MENLO_W : 0.9)
    const doc = fakeDoc()
    expect(installCjkMetricFaces({ stack: STACK, doc, measure: absent })).toBe(STACK)
    expect(doc.getElementById('gadak-cjk-metric-faces')).toBeNull()
  })

  it('hands back the stack untouched when the cell cannot be measured', () => {
    expect(installCjkMetricFaces({ stack: STACK, doc: fakeDoc(), measure: () => 0 })).toBe(STACK)
  })

  it('hands back the stack untouched with no canvas and no document', () => {
    // The pane must still get a usable stack rather than an exception.
    expect(installCjkMetricFaces({ stack: STACK, doc: fakeDoc() })).toBe(STACK)
    expect(installCjkMetricFaces({ stack: STACK, doc: null })).toBe(STACK)
  })

  it('keeps a user’s own terminal font as the lead', () => {
    // GDK-1043's contract (renderer.test.ts): --font-mono-terminal wins.
    // The correction is measured *against* whatever leads, so a user stack
    // stays in front and only the CJK faces are added.
    const user = "'JetBrains Mono', monospace"
    const out = installCjkMetricFaces({ stack: user, doc: fakeDoc(), measure })
    expect(out.endsWith(user)).toBe(true)
  })
})
