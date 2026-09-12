/**
 * Terminal-pane hero, live-Claude cut — docs/media/terminal-hero.mp4.
 *
 * Every earlier agent clip — agent.gif, claude-drive, claude-dashboards —
 * draws a *paper terminal* beside an app iframe, because gadak had no
 * terminal of its own and the agent in the story lived in some other window.
 * This take retires that composite: Claude Code runs in gadak's own pane, in
 * gadak's own window, and the board it moves is the one next to it.
 *
 * Nothing here is mocked and nothing is scripted output. The only things this
 * spec types are the two prompts a person would type; every command on screen
 * is Claude's own choice, and the list and the dashboard beside them are the
 * app reacting to what Claude actually wrote.
 *
 * **The claim is "the agent in the window, in your language."** Through
 * 2026-09-07 this clip was one global take — English UI, English mirror,
 * Korean prompts — and its second claim was the mismatch itself: a Korean
 * sentence landing on an English board. That is not the claim any more. Each
 * locale gets its own take and each take is one language end to end: the
 * chrome from that locale's catalog, the mirror translated on the drive copy
 * (tools/demo-i18n/apply.py, GDK-1556), and the prompts written in it. The
 * Korean viewer is not shown a translation of someone else's demo.
 *
 * What stays English in every take, because it is not ours to translate: the
 * gadak CLI has no i18n (`bound to session` is its own output) and Claude
 * Code's TUI is English (`Welcome to Claude Code`). Those two are asserted as
 * literals; everything the *app* renders is read out of the catalog.
 *
 * Beats:
 *   1. The list at rest — Epics, on a real mirror, in this take's language
 *   2. ⌘K → the Terminal action, named in this locale, opens the pane
 *      `gadak claim NMA-140` — the row moves, the roster tab takes the key
 *   3. `claude` boots inside it
 *   4. the first prompt — the list becomes that answer
 *   5. the second prompt — and the same pane paints a wall
 *
 * Gated by GADAK_MEDIA=1, and driven by record-terminal-claude.sh — which
 * owns the serve, the isolated agent HOME, the frozen GADAK_HOME and the
 * translation applied to it. Running this config on its own attaches to
 * whatever is on the port and will record the operator's real home directory
 * into the frame.
 *
 * Viewport and video size must stay 1440×900 (terminal-claude.config.ts) or
 * Playwright letterboxes the capture.
 */
import { type Page, type TestInfo } from '@playwright/test'
import { test, expect } from '../helpers'
import { writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { catalogFor, forceLocale, mediaLocale, MEDIA_LOCALE_STAMP } from '../helpers'
import { readTerm } from '../term-read'
import type { Locale } from '../../web/src/lib/i18n/types'

const isMedia = !!process.env.GADAK_MEDIA

/**
 * The UI language this take records in (GADAK_MEDIA_LOCALE, default en).
 *
 * The chrome below is read out of that locale's catalog rather than restated
 * as an English literal, and the mirror under it is the translated copy
 * record-terminal-claude.sh applied to the drive's gadak.db for this locale.
 */
const LOCALE = mediaLocale()
const t = catalogFor(LOCALE)

/**
 * Stamp the locale beside the take, the way search-demo.spec.ts does.
 *
 * Measured 2026-09-07 on the sibling clip: running an export by hand with
 * GADAK_MEDIA_LOCALE=ja over a results directory left by an earlier English
 * take produced an mp4 full of English pixels under a Japanese name, and
 * nothing said so. The take now carries its own language, and
 * export-terminal.sh refuses a mismatch.
 */
async function stampLocale(testInfo: TestInfo): Promise<void> {
  await writeFile(join(testInfo.project.outputDir, MEDIA_LOCALE_STAMP), `${LOCALE}\n`, 'utf8')
}

/** Pause between beats so a human can read the frame. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

/**
 * Focus the pane the way the e2e suite does: click the host, then focus the
 * helper textarea explicitly. The renderer paints on a canvas, so the click
 * alone lands on something that cannot hold a caret.
 */
async function focusPane(page: Page): Promise<void> {
  const pane = page.getByTestId('terminal-pane')
  const host = pane.locator('[data-gadak-editable]')
  if (await host.count()) {
    await host.first().click({ position: { x: 24, y: 24 } })
  } else {
    await pane.click({ position: { x: 24, y: 24 } })
  }
  await page.evaluate(() => {
    document
      .querySelector<HTMLTextAreaElement>('[data-testid="terminal-pane"] textarea')
      ?.focus()
  })
}

/**
 * Type a prompt into Claude's TUI and submit it.
 *
 * Two deliberate pauses. The delay is a reading speed, not a throughput test.
 * The gap before Enter is what the tapes learned the hard way: Claude's input
 * box re-renders as it grows, and an Enter that lands mid-render is swallowed.
 */
async function ask(page: Page, prompt: string): Promise<void> {
  await focusPane(page)
  // A Korean or Japanese sentence is about half the characters of the
  // English one, and keyboard.type has no IME — every character lands
  // finished. 55ms per character reads as typing in English and as a paste
  // in ko/ja; 90ms puts the three takes' typing time within a second of each
  // other and lets a native viewer read the sentence as it forms.
  await page.keyboard.type(prompt, { delay: LOCALE === 'en' ? 55 : 90 })
  await beat(page, 900)
  await page.keyboard.press('Enter')
}

/**
 * The two prompts, per locale. Phrased the way someone actually asks — no
 * key, no JQL, no column names — and each one written in its own language
 * rather than translated from the English line.
 *
 * The name is spelled in full on purpose, and that holds in all three. Take 1
 * asked for "다나" and Claude stopped to ask which one it meant: the demo
 * config's identity is dana@example.com, the assignee in the mirror is Dana
 * Whitfield, and the changelog entries on those issues were written by Alex
 * Kim. It was right to ask, and a clarifying question is the one thing a
 * 40-second clip has no room for. Ambiguity in the prompt is the recorder's
 * bug, not the model's. The name stays Latin in the ko and ja takes because
 * that is what the mirror's assignee field says — the fixture translation
 * covers the prose, not the people.
 *
 * "open it" is load-bearing in all three, not politeness — "열어줘",
 * "開いて". Take 1 authored and saved a dashboard and stopped there, which is
 * the correct reading of "만들어줘" / "make one" and leaves the clip ending on
 * a terminal. `dashboards open` is what takes the column, and it is a
 * separate verb the skill teaches.
 */
const PROMPTS: Record<Locale, { activity: string; dashboard: string }> = {
  en: {
    activity: "Show me Dana Whitfield's issues that moved recently",
    // "ratios" alone read as a *count* chart three takes out of three
    // (2026-09-12: "12 labels · 640 issues", no percent sign anywhere), the
    // same way Korean 비율 did on 2026-09-08 — so the English line now pins
    // the unit the way the Korean one does. The contract below is what
    // caught it; the prompt is what was ambiguous.
    dashboard: 'Make a dashboard of issue label ratios as percentages and open it',
  },
  ko: {
    activity: 'Dana Whitfield이 담당한 이슈 중에 최근에 움직인 것 보여줘',
    // "비율 대시보드" alone read as a *count* chart three takes out of three
    // (2026-09-08: 12개 라벨 · 이슈 640건, no percentages), while the en and
    // ja lines got ratios first time. Korean 비율 is looser than "ratio" —
    // 백분율 is the word that pins it, and the contract below insists on it.
    dashboard: '이슈 라벨별 비율을 백분율로 보여주는 대시보드 만들어서 열어줘',
  },
  ja: {
    activity: 'Dana Whitfield が担当している課題のうち、最近動いたものを見せて',
    // Pinned by parity with en and ko rather than by its own measurement:
    // 比率 is as loose as 비율, and a rejected live take costs three rounds.
    dashboard: '課題ラベルの比率をパーセントでダッシュボードにして開いて',
  },
}
const ASK_ACTIVITY = PROMPTS[LOCALE].activity
const ASK_DASHBOARD = PROMPTS[LOCALE].dashboard

test.describe('terminal claude demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('the agent is in the window: two prompts in this locale move the board', async ({
    page,
  }, testInfo) => {
    await stampLocale(testInfo)
    // The pane's stored width is per-browser, and a fresh recording context
    // has none — the default ratio would open it at 634px here, and the take
    // wants the width to be a decision rather than a ratio. 640 is also the
    // first frame where the pane gets what it asks for: under the 1080-wide
    // cut the split clamped to ~428 (the list keeps a 390px minimum), which
    // is about 55 columns and wraps Claude's TUI into a column of stubs.
    await page.addInitScript(() => {
      try {
        localStorage.setItem('gadak.terminal.height', '340')
      } catch {
        /* private mode */
      }
    })
    await forceLocale(page, LOCALE)

    // Beat 1 — the list at rest.
    await page.goto('/#/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible({ timeout: 30_000 })
    const atRest = await page.getByTestId('list-count').textContent()
    // Into the take log: beat 4's contract is "the count changed", and a
    // rejected take is unreadable without knowing what it was at rest.
    console.log(`terminal-claude: locale ${LOCALE}, list at rest = ${JSON.stringify(atRest)}`)
    // 2000, not 1600: the first thing a ko/ja viewer checks is whether the
    // chips and headers are really their language. The head is time-lapsed
    // by dense-cut.py anyway, so the hold costs the cut little.
    await beat(page, 2000)

    // Beat 2 — ⌘K, the Terminal action, Enter. The chord (Ctrl+`) is the
    // shortcut a regular carries; the palette is how the pane is *discovered*,
    // and it says on camera that this is a first-class command, not a hidden
    // key.
    //
    // What gets typed is the action's own label in this locale
    // (palette.actionTerminal — en "Terminal", ko "터미널", ja "ターミナル"),
    // because the palette matches action rows by case-insensitive substring
    // over the rendered label (CommandPalette.svelte `matches`). Typing the
    // whole label also makes it an *exact* action match
    // (`isExactActionMatch`), which hoists the action block above docs and
    // issues so Enter is locale-stable (GDK-300). A hard-coded "terminal"
    // matches nothing under ko/ja and the take would stall at ⌘K.
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: t['palette.title'] })
    await expect(palette).toBeVisible()
    await beat(page, 700)
    // "Terminal" is 8 characters, "터미널" 3, "ターミナル" 5 — at one delay
    // the discovery beat would be a third as long in Korean. The per-character
    // delay is derived from the label so the typing lasts ~480ms everywhere.
    const label = t['palette.actionTerminal']
    await page.keyboard.type(label, { delay: Math.max(60, Math.round(480 / label.length)) })
    await expect(palette.getByTestId('palette-action-terminal')).toBeVisible()
    await beat(page, 900)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const pane = page.getByTestId('terminal-pane')
    await expect(pane).toBeVisible()
    await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
    // The beta mark came off in 0.19 (GDK-1024) — asserted absent, so a
    // revert cannot quietly put it back into a recording.
    await expect(page.getByTestId('terminal-beta')).toHaveCount(0)
    await beat(page, 1200)

    // Beat 2b — `gadak claim NMA-140`, typed by the person. Two things land
    // at once and both are on camera: the row moves to In Progress with an
    // assignee, and the roster tab stops being a session id and becomes the
    // issue's key (GDK-1158) — this shell is now NMA-140's shell, and the
    // agent about to start in it inherits that. Real write: the take runs
    // on the built-in tracker (record-terminal-claude.sh), not the fixture's
    // fake Jira.
    //
    // The command, the key and `bound to session` are English in all three
    // takes: the CLI has no i18n, and pretending otherwise in a recording
    // would be the one false frame in the clip.
    await focusPane(page)
    await page.keyboard.type('gadak claim NMA-140', { delay: 60 })
    await beat(page, 500)
    await page.keyboard.press('Enter')
    await expect.poll(async () => readTerm(page), { timeout: 30_000 }).toContain('bound to session')
    // Two facts land here and they get a hold each: first the row moving to
    // In Progress on the board (the payoff a ko/ja viewer can read — the CLI
    // line above it is English in every take), then the roster tab taking
    // the key.
    await beat(page, 900)
    await expect(page.getByTestId('terminal-strip-name').first()).toHaveText('NMA-140', {
      timeout: 15_000,
    })
    await beat(page, 1200)

    // Beat 3 — `claude`, in gadak's own shell. The TUI boot is the slow part;
    // the input box is what tells us it is ready to be typed into. Claude
    // Code's own chrome is English wherever the take is recorded.
    await focusPane(page)
    await page.keyboard.type('claude', { delay: 90 })
    await beat(page, 500)
    await page.keyboard.press('Enter')
    await expect
      .poll(async () => readTerm(page), { timeout: 90_000, intervals: [1000] })
      .toMatch(/Welcome to Claude Code|for shortcuts|\? for shortcuts/i)
    await beat(page, 2000)

    // Beat 4 — a sentence in the viewer's own language becomes the list. What
    // Claude runs to get there is its own; the skill teaches `views open`,
    // and the app is watching that handoff.
    await ask(page, ASK_ACTIVITY)
    await expect(page.getByTestId('list-count')).not.toHaveText(atRest ?? '', {
      timeout: 300_000,
    })
    await beat(page, 3000)

    // Beat 5 — and the same pane can paint a wall. dashboards open takes the
    // whole column, so this is the frame the clip ends on.
    await ask(page, ASK_DASHBOARD)
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 420_000 })
    await expect(page.getByTestId('dashboard-frame')).toBeVisible({ timeout: 60_000 })

    // The wall has to *render*, not just exist. The take before this one
    // saved and opened a dashboard whose every card read "undefined" and
    // "NaN%" — the datasource SQL was right and the page never got the rows.
    // A contract that only asks "is a dashboard open" passes that take, so
    // this asks what the eye asks. Both patterns hold in a translated take:
    // they come from the renderer, not from the fixture's prose.
    const wall = page.frameLocator('[data-testid="dashboard-frame"]').locator('body')
    await expect(wall).toContainText(/\d/, { timeout: 60_000 })
    await expect(wall).not.toContainText(/undefined|NaN/i)
    // The prompt asked for *ratios*. The first ko take (2026-09-08) saved and
    // opened a chart of bare counts under the title "12개 라벨 · 이슈 640건" —
    // every gate above passed, and the poster showed a Korean visitor an
    // answer to a different question. A ratio chart carries a percent sign;
    // a take without one is rejected here and retried, in every locale.
    await expect(wall).toContainText('%')
    await beat(page, 4000)
  })
})
