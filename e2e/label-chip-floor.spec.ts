import { type Page } from '@playwright/test'
import { test, expect } from './helpers'
import { forceLocale, gotoApp, searchInput } from './helpers'
import { fields } from '../web/src/lib/i18n/messages/fields'
import { list } from '../web/src/lib/i18n/messages/list'
import type { Message } from '../web/src/lib/i18n/types'

/*
 * GDK-1744: a label chip is either readable or absent.
 *
 * With the detail panel open at 1440x900 the list column narrows, the labels
 * slot steps to its 64/76px rung, and the GDK-1050 counter reserve took the
 * difference out of the chip — leaving about 23px of text, which rendered as
 * "pa…", "da…", "cu…". Two characters and an ellipsis identify nothing, so
 * the chip was costing space to convey zero.
 *
 * The contract: a chip that is *clipped* has at least CHIP_MIN_TEXT_CH
 * characters' worth of text box. Below that the chips do not render at all and
 * the slot carries one "+N" collapse badge whose title lists every label —
 * strictly more information than the truncated chip it replaces.
 *
 * The clipped qualifier is the whole point and not a loophole: a short label
 * showing in full ("docs" in a 36px box) is doing its job, and a floor on the
 * box alone would fail it for no reader-visible reason. What must never render
 * is an ellipsis with fewer than six characters in front of it.
 *
 * This measures the shipped DOM rather than the CSS text, so it holds however
 * the fold is expressed. It is width-driven, not data-driven: it walks the
 * rows the fixture happens to show and asserts the invariant on each.
 */

const CHIP_MIN_TEXT_CH = 6

type ChipReport = {
  key: string
  label: string
  textPx: number
  minPx: number
  clipped: boolean
}

/** Every label chip on screen, with its text box and the 6ch floor resolved
 *  in that element's own font — `ch` is font-relative, so it is read from the
 *  element rather than assumed. */
async function chipReports(page: Page): Promise<ChipReport[]> {
  return page.evaluate((minCh) => {
    const out: ChipReport[] = []
    const probe = document.createElement('span')
    probe.style.cssText = 'position:absolute;visibility:hidden;white-space:pre'
    document.body.appendChild(probe)
    const rows = document.querySelectorAll('[data-issue-key]')
    for (const row of rows) {
      const key = row.getAttribute('data-issue-key') ?? '?'
      const slot = row.querySelector('[data-col="labels"]')
      if (!slot) continue
      for (const chip of slot.querySelectorAll('button')) {
        // A folded-away chip is display:none — that is the fix, not a finding.
        if (!(chip as HTMLElement).offsetParent) continue
        const cs = getComputedStyle(chip)
        // Content box: the chip's own horizontal padding is not text room.
        const textPx =
          chip.getBoundingClientRect().width -
          parseFloat(cs.paddingLeft) -
          parseFloat(cs.paddingRight)
        probe.style.font = cs.font
        probe.textContent = '0'.repeat(minCh)
        const minPx = probe.getBoundingClientRect().width
        out.push({
          key,
          label: chip.textContent?.trim() ?? '',
          textPx: Math.round(textPx * 10) / 10,
          minPx: Math.round(minPx * 10) / 10,
          clipped: chip.scrollWidth > chip.clientWidth + 1,
        })
      }
    }
    probe.remove()
    return out
  }, CHIP_MIN_TEXT_CH)
}

async function assertNoStarvedChips(page: Page, where: string) {
  const chips = await chipReports(page)
  const starved = chips.filter((c) => c.clipped && c.textPx < c.minPx)
  expect(
    starved.map((c) => `${c.key} "${c.label}" ${c.textPx}px < ${c.minPx}px`).join('\n') || '(none)',
    `${where}: a chip under ${CHIP_MIN_TEXT_CH}ch must fold into the +N badge (GDK-1744)`,
  ).toBe('(none)')
  return chips
}

test.describe('GDK-1744 label chips are readable or absent', () => {
  test('list alone and with the detail panel open, 1440x900', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()

    // 1) List alone — the wide rung, where chips are expected to render.
    const wide = await assertNoStarvedChips(page, 'list alone')
    expect(wide.length, 'the fixture must show some label chips at 1440 wide').toBeGreaterThan(0)

    // 2) Detail panel open — the narrow rung that produced "pa…".
    await searchInput(page).fill('NMB-110')
    await page
      .locator('[data-testid="issue-list-scroller"] [data-issue-key="NMB-110"]')
      .first()
      .click()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()
    await assertNoStarvedChips(page, 'detail panel open')
  })

  test('a folded slot still names every label in its badge title', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)
    await searchInput(page).fill('NMB-110')
    await page
      .locator('[data-testid="issue-list-scroller"] [data-issue-key="NMB-110"]')
      .first()
      .click()
    await expect(page.getByTestId('issue-detail-panel')).toBeVisible()

    // Whatever the labels slot shows at this width, it is not silent: either
    // chips (checked above) or a badge carrying the full list.
    const titled = await page.evaluate(() => {
      const rows = document.querySelectorAll('[data-issue-key]')
      let withBadge = 0
      let emptyTitle = 0
      for (const row of rows) {
        const slot = row.querySelector('[data-col="labels"]')
        if (!slot) continue
        for (const badge of slot.querySelectorAll('span[title]')) {
          if (!(badge as HTMLElement).offsetParent) continue
          withBadge++
          if (!(badge.getAttribute('title') ?? '').trim()) emptyTitle++
        }
      }
      return { withBadge, emptyTitle }
    })
    expect(titled.emptyTitle, 'every visible +N badge lists its labels').toBe(0)
  })
})

/*
 * GDK-1744 follow-up (web-fix round, 2026-09-10): the deploy chip hard-cut.
 *
 * The vision verdict on 1744-detail-open-{light,dark}.png: a chip reading
 * "QA r" cut at four glyphs with no ellipsis, next to a labels chip that
 * truncated normally. Measured, it is the deploy-stage badge
 * ([data-col="deploy"], state 'qa' → "QA ready"):
 *
 *   1. MECHANISM — the chip is `flex … truncate`, and text-overflow never
 *      applies to flex content: the label was an anonymous flex item with
 *      min-width:auto, so it did not shrink, and the chip's own
 *      overflow:hidden cut it mid-glyph (scrollWidth 71 > clientWidth 40,
 *      computed text-overflow: ellipsis — configured, never painted).
 *   2. STARVATION — the column is w-10 (40px, fixed at every row width;
 *      trail-fold-3 only hides it below 400), and no label of the set is
 *      both fittable and readable: measured in the chip's 11px face the 6ch
 *      floor is 42.9px of text; the shortest chip ("미배포") is 40.5px
 *      carrying 28.5px of text — under the floor even where it fits — and
 *      the shortest floor-meeting chip is 59px. Every deploy chip shipped as
 *      a 2–4-glyph cut.
 *
 * The column cannot simply widen: the full-catalog rows the ladder pins
 * (list-row-overflow.spec.ts) hold 1.6 / 87.6 / 19.6px of title slack above
 * the 13ch floor at row widths 1008 / 1168 / 1360 (measured, same round) —
 * the ~60px a label-sized column needs does not exist at the 1360 cap, and
 * the set-aware generator that could re-price the rungs is not in this
 * round's file list. So the GDK-1744 trade lands instead: where text cannot
 * reach the 6ch floor it does not render. The 'qa' chip keeps its designed
 * teal dot; the dotless states hide (their words live in the chip title and
 * the detail panel, and absence is the column's existing rendering for
 * state 'none'). The label span keeps a real `truncate`: as a block flex
 * item it ellipsizes for real, where the old flex button never did.
 */

/** The deploy chip's label set (IssueRow deployMeta), per locale. */
const DEPLOY_LABELS: Record<string, Message> = {
  'deploy.qa': fields['deploy.qa'],
  'deploy.dev': fields['deploy.dev'],
  'deploy.prod': fields['deploy.prod'],
  'deploy.qa_preview(list.qaPending)': list['list.qaPending'],
  'deploy.notDeployed': fields['deploy.notDeployed'],
}

type FlexTextCut = {
  key: string
  text: string
  sw: number
  cw: number
  cls: string
  rect: string
}

/** Every visible-row element that is a flex box, DIRECTLY carries text, and
 *  clips it — the "QA r" mechanism. text-overflow does not apply to flex
 *  content, so any hit is a mid-glyph cut by construction, whatever chip
 *  wears it. Widths, classes and the rect ride along so a failure reads as
 *  a measurement, not a pointer chase. */
async function flexTextCuts(page: Page): Promise<FlexTextCut[]> {
  return page.evaluate(() => {
    const out: FlexTextCut[] = []
    const scroller = document.querySelector('[data-testid="issue-list-scroller"]')
    if (!scroller) return out
    for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
      const r = row.getBoundingClientRect()
      if (r.bottom < 0 || r.top > innerHeight) continue
      for (const el of row.querySelectorAll<HTMLElement>('*')) {
        const cs = getComputedStyle(el)
        if (cs.display !== 'flex' && cs.display !== 'inline-flex') continue
        const ownText = [...el.childNodes].some(
          (n) => n.nodeType === Node.TEXT_NODE && (n.textContent ?? '').trim() !== '',
        )
        if (!ownText) continue
        if (el.scrollWidth <= el.clientWidth + 1) continue
        const b = el.getBoundingClientRect()
        out.push({
          key: row.dataset.issueKey ?? '?',
          text: (el.textContent ?? '').trim().slice(0, 24),
          sw: el.scrollWidth,
          cw: el.clientWidth,
          cls: el.className.toString().split(' ').slice(0, 4).join('·'),
          rect: `${Math.round(b.left)},${Math.round(b.top)} ${Math.round(b.width)}x${Math.round(b.height)}`,
        })
      }
    }
    return out
  })
}

type DeployChipReport = {
  key: string
  chipVisible: boolean
  dot: boolean
  labelVisible: boolean
  labelClippedTextPx: number | null
  labelMinPx: number | null
  slotW: number
  chipRight: number | null
  slotRight: number
}

/** Every rendered deploy chip with the axes of its contract: the chip stays
 *  inside its slot, and any label text that renders meets the same 6ch floor
 *  the labels chips answer to (measured in the label's own font).
 *
 *  The chip is found structurally (the deploy slot's button / stale span), and
 *  the label carrier falls back to the chip itself when it directly carries
 *  text — which is the pre-fix DOM — so the starvation numbers below are real
 *  on the unfixed tree too, not just a missing-class liveness failure. */
async function deployChipReports(page: Page): Promise<DeployChipReport[]> {
  return page.evaluate((minCh) => {
    const round = (n: number) => Math.round(n * 10) / 10
    const out: DeployChipReport[] = []
    const probe = document.createElement('span')
    probe.style.cssText = 'position:absolute;visibility:hidden;white-space:pre'
    document.body.appendChild(probe)
    // Chrome returns '' for the computed .font shorthand (measured, the probe
    // round of this same fix) — set the longhands, which always serialize.
    const setProbeFont = (cs: CSSStyleDeclaration) => {
      probe.style.fontStyle = cs.fontStyle
      probe.style.fontWeight = cs.fontWeight
      probe.style.fontSize = cs.fontSize
      probe.style.fontFamily = cs.fontFamily
      probe.style.letterSpacing = cs.letterSpacing
    }
    const scroller = document.querySelector('[data-testid="issue-list-scroller"]')
    if (!scroller) return out
    for (const row of scroller.querySelectorAll<HTMLElement>('[data-issue-key]')) {
      const slot = row.querySelector<HTMLElement>('[data-col="deploy"]')
      if (!slot) continue
      const slotRight = slot.getBoundingClientRect().right
      for (const chip of slot.querySelectorAll<HTMLElement>('button, .deploy-stale')) {
        const cs = getComputedStyle(chip)
        const chipVisible = cs.display !== 'none' && !!chip.offsetParent
        const labelEl = chip.querySelector<HTMLElement>('.deploy-chip-label')
        const ownText = [...chip.childNodes].some(
          (n) => n.nodeType === Node.TEXT_NODE && (n.textContent ?? '').trim() !== '',
        )
        const label = labelEl ?? (ownText ? chip : null)
        let labelVisible = false
        let labelClippedTextPx: number | null = null
        let labelMinPx: number | null = null
        if (label) {
          const lcs = getComputedStyle(label)
          labelVisible = lcs.display !== 'none' && !!label.offsetParent
          if (labelVisible && label.scrollWidth > label.clientWidth + 1) {
            labelClippedTextPx =
              round(label.getBoundingClientRect().width) -
              parseFloat(lcs.paddingLeft) -
              parseFloat(lcs.paddingRight)
            setProbeFont(lcs)
            probe.textContent = '0'.repeat(minCh)
            labelMinPx = round(probe.getBoundingClientRect().width)
          }
        }
        out.push({
          key: row.dataset.issueKey ?? '?',
          chipVisible,
          dot: chip.classList.contains('deploy-chip-dot'),
          labelVisible,
          labelClippedTextPx,
          labelMinPx,
          slotW: round(slot.getBoundingClientRect().width),
          chipRight: chipVisible ? round(chip.getBoundingClientRect().right) : null,
          slotRight: round(slotRight),
        })
      }
    }
    probe.remove()
    return out
  }, CHIP_MIN_TEXT_CH)
}

test.describe('GDK-1744 follow-up: the deploy chip is readable or absent', () => {
  test('no flex box clips its own text — the "QA r" mechanism (all rows)', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)
    await searchInput(page).fill('NMB-110')
    await expect(
      page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first(),
    ).toBeVisible()

    const cuts = await flexTextCuts(page)
    expect(
      cuts.map((c) => `${c.key} "${c.text}" sw=${c.sw} cw=${c.cw} [${c.cls}] @${c.rect}`).join('\n') ||
        '(none)',
      'a flex box that clips its own text renders a mid-glyph cut (text-overflow never applies to flex content); wrap the text in a min-w-0 truncate span',
    ).toBe('(none)')
  })

  for (const locale of ['en', 'ko', 'ja'] as const) {
    test(`deploy chip: label never renders below the 6ch floor (${locale})`, async ({ page }) => {
      await page.setViewportSize({ width: 1440, height: 900 })
      // gotoApp forces 'en' and waits on the English pool count, so ko/ja
      // boot by hand: forceLocale seeds only when unset, and the steering
      // below is gotoApp's own (the first-run rule lands on Dana's issues;
      // NMB-110 is not hers).
      await forceLocale(page, locale)
      if (locale === 'en') {
        await gotoApp(page)
      } else {
        await page.goto('/')
        await expect(page.locator('[data-issue-key]').first()).toBeVisible({ timeout: 30_000 })
        if (/[#?&]fl=mine(&|$)/.test(page.url())) {
          await page.goto('/#/?sc=new%2Cinprogress&g=epic')
          await expect(page.locator('[data-issue-key]').first()).toBeVisible({ timeout: 30_000 })
        }
      }
      await searchInput(page).fill('NMB-110')
      await expect(
        page.locator('[data-testid="issue-list-scroller"] [data-issue-key="NMB-110"]'),
      ).toBeVisible()

      const reports = await deployChipReports(page)
      // Liveness: NMB-110 is the fixture's only deploy-enriched row — the
      // 'qa' chip must be on screen in every locale, or the axes below pass
      // vacuously.
      const chip = reports.find((r) => r.key === 'NMB-110' && r.chipVisible)
      expect(chip, 'NMB-110 renders a visible deploy chip').toBeTruthy()

      // Starvation first: on a tree that renders a clipped label, this is the
      // axis that says so in pixels.
      const starved = reports.filter(
        (r) => r.labelVisible && r.labelClippedTextPx !== null && r.labelClippedTextPx < r.labelMinPx!,
      )
      expect(
        starved
          .map(
            (r) =>
              `${r.key} label ${r.labelClippedTextPx}px < ${r.labelMinPx}px (slot ${r.slotW}px)`,
          )
          .join('\n') || '(none)',
        'a clipped deploy label under 6ch must not render (GDK-1744 floor)',
      ).toBe('(none)')

      expect(chip!.dot, "the 'qa' chip keeps its designed dot").toBe(true)

      const pokes = reports.filter(
        (r) => r.chipVisible && r.chipRight !== null && r.chipRight > r.slotRight + 0.5,
      )
      expect(
        pokes.map((r) => `${r.key} chip right ${r.chipRight} > slot right ${r.slotRight}`).join('\n') ||
          '(none)',
        'the deploy chip stays inside its column',
      ).toBe('(none)')
    })
  }

  test('the 40px column cannot afford any deploy label — the fold is forced, not styled', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await gotoApp(page)
    await searchInput(page).fill('NMB-110')
    await expect(
      page.locator('[data-testid="issue-list-scroller"] [data-issue-key="NMB-110"]'),
    ).toBeVisible()

    // Re-derive the requirement from the live catalog (all locales, all
    // states) against the live column: if someone widens the column to
    // afford labels, this axis goes red until the label un-hides; if someone
    // un-hides labels without widening, the 6ch axis above goes red. Both
    // directions are held by measurement, not by a CSS comment.
    //
    // "Affords" means a label that would MEET THE FLOOR fits — chip inside
    // the column AND its own text ≥ 6ch — not merely that the string fits:
    // the shortest label ("미배포") measures 40.5px of chip against the 40px
    // column, and a binary fits/doesn't would flip on half a pixel of font
    // drift. Readable needs 42.9px of text + 12px of padding; that is the
    // boundary with margin.
    const m = await page.evaluate(
      (args) => {
        const { labels, minCh } = args as { labels: Record<string, { en: string; ko: string; ja: string }>; minCh: number }
        const row = document.querySelector<HTMLElement>('[data-issue-key="NMB-110"]')
        const slot = row?.querySelector<HTMLElement>('[data-col="deploy"]')
        const chip = slot?.querySelector<HTMLElement>('button, .deploy-stale')
        if (!slot || !chip) return null
        const cs = getComputedStyle(chip)
        const probe = document.createElement('span')
        probe.style.cssText = 'position:absolute;visibility:hidden;white-space:pre'
        // Longhands, not the .font shorthand — Chrome returns '' for the
        // shorthand (measured) and the probe would measure in 16px defaults.
        probe.style.fontStyle = cs.fontStyle
        probe.style.fontWeight = cs.fontWeight
        probe.style.fontSize = cs.fontSize
        probe.style.fontFamily = cs.fontFamily
        probe.style.letterSpacing = cs.letterSpacing
        document.body.appendChild(probe)
        probe.textContent = '0'.repeat(minCh)
        const minPx = Math.round(probe.getBoundingClientRect().width * 10) / 10
        const req: Record<string, { text: number; chip: number }> = {}
        for (const [k, v] of Object.entries(labels)) {
          for (const loc of ['en', 'ko', 'ja'] as const) {
            probe.textContent = v[loc]
            const text = probe.getBoundingClientRect().width
            // px-1.5 both sides; the dot-bearing 'qa' chip adds dot 6 + gap 4.
            const dotCost = k === 'deploy.qa' ? 10 : 0
            req[`${k}.${loc}`] = {
              text: Math.round(text * 10) / 10,
              chip: Math.round((text + 12 + dotCost) * 10) / 10,
            }
          }
        }
        probe.remove()
        return { slotW: Math.round(slot.getBoundingClientRect().width * 10) / 10, minPx, req }
      },
      { labels: DEPLOY_LABELS, minCh: CHIP_MIN_TEXT_CH },
    )
    expect(m, 'the deploy slot and chip must render on NMB-110').toBeTruthy()
    const affordable = Object.entries(m!.req).filter(
      ([, v]) => v.text >= m!.minPx && v.chip <= m!.slotW,
    )
    expect(
      affordable
        .map(([k, v]) => `${k} text ${v.text}px ≥ ${m!.minPx}px and chip ${v.chip}px ≤ column ${m!.slotW}px`)
        .join('\n') || '(none)',
      'the column cannot host any floor-meeting deploy label (if it can, un-hide labels and re-derive)',
    ).toBe('(none)')
    // The full requirement table, so the margins are numbers in the log:
    const rows = Object.entries(m!.req)
      .sort((a, b) => a[1].chip - b[1].chip)
      .map(([k, v]) => `${k} text=${v.text} chip=${v.chip}`)
    console.log(
      `[deploy-chip] column=${m!.slotW}px floor(6ch)=${m!.minPx}px\n  ${rows.join('\n  ')}`,
    )
  })
})
