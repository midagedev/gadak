/*
 * Navigation state — the single owner (A-nav, GDK-800/801/802 마감).
 *
 * One value decides what is on screen; App.svelte holds it and every
 * transition goes through these pure steps so the landing rules are
 * testable without a DOM (the web's column-union precedent, web/src/
 * App.svelte "one address, one column": each branch is a show onto the
 * union, so two screens can never stack).
 *
 * Grammar (ux-report 결론 요약 8, Q5, Q7):
 *  - tabs: Queue / Search / More — the only three; Pair is not a tab
 *    (onboarding, not navigation). The unpaired first screen is Queue
 *    ("첫 화면을 Pair 탭으로 강제하지 말 것").
 *  - pair: the Pair screen pushed over a tab (queue-empty CTA, or More's
 *    pair-management row). The tab bar stays — bailing out is a tab tap,
 *    never a dead end.
 *  - detail: an issue pushed over queue/search. Back (button or left
 *    edge swipe) lands on the tab it was opened over; a notification tap
 *    goes straight to Detail and its back is the Queue (Q5 — "백 제스처는
 *    큐", there is no notification inbox to return to).
 */
import type { QueueRow } from './api'

export type NavTab = 'queue' | 'search' | 'more'

/** Where a pushed Detail returns to — More never opens one. */
export type DetailBack = 'queue' | 'search'

export type NavState =
  | { view: 'tabs'; tab: NavTab }
  | { view: 'pair'; back: NavTab }
  | { view: 'detail'; issueKey: string; back: DetailBack }

/** The app always opens here — paired or not (ux-report Q6). */
export const NAV_HOME: NavState = { view: 'tabs', tab: 'queue' }

/** Tab tap: shows a tab, wherever we were (from `pair` this is the bail-out). */
export function openTab(_s: NavState, tab: NavTab): NavState {
  return { view: 'tabs', tab }
}

/** The Pair screen over the current tab (queue-empty CTA, More's row). */
export function openPair(s: NavState): NavState {
  const back: NavTab = s.view === 'tabs' ? s.tab : 'queue'
  return { view: 'pair', back }
}

/**
 * An issue opened from a list: back is the tab under it. Search rows keep
 * Search (the query is still there); anything else lands on Queue. A
 * second open from inside Detail replaces the pushed screen — Detail has
 * no row list of its own, so this arm only exists to keep the step total.
 */
export function pushDetail(s: NavState, issueKey: string): NavState {
  const back: DetailBack = s.view === 'tabs' && s.tab === 'search' ? 'search' : 'queue'
  return { view: 'detail', issueKey, back }
}

/**
 * A notification/banner tap: straight to the issue, back to the Queue
 * (ux-report Q5). A tap with no issue_key lands the Queue — there is no
 * notification inbox screen to restore.
 */
export function openDetailFromNotification(issueKey: string | null): NavState {
  // '' is "no target" too — bindNotificationTap already folds it to null;
  // this guard keeps the step safe for any future caller.
  return issueKey === null || issueKey === ''
    ? NAV_HOME
    : { view: 'detail', issueKey, back: 'queue' }
}

/** Back from Detail: land on the tab it was opened over. No-op elsewhere. */
export function popDetail(s: NavState): NavState {
  return s.view === 'detail' ? { view: 'tabs', tab: s.back } : s
}

/**
 * The list row a pushed Detail should read its status/priority from — the
 * row the caller's tab holds, null when the pool does not know the key
 * (a done issue, or a notification for something outside the queue).
 */
export function rowFor(pool: QueueRow[], issueKey: string): QueueRow | null {
  return pool.find((r) => r.issue_key === issueKey) ?? null
}
