/*
 * Shared format helpers for the detail panel ([detail]).
 * Pure functions only: relative time, browse URL, etc.
 */

import { jiraBrowseUrl } from '../../lib/config'
import { absTime as i18nAbsTime, relativeTime as i18nRelativeTime } from '../../lib/i18n'

/** ISO8601 → relative time (long style: "3m ago"). On parse fail, return raw. */
export function relativeTime(iso: string | null | undefined): string {
  return i18nRelativeTime(iso, 'long')
}

/** ISO8601 → absolute time label (for tooltips). */
export function absoluteTime(iso: string | null | undefined): string {
  return i18nAbsTime(iso)
}

/** Canonical Jira issue URL. null if no site configured (omit href). */
export function jiraUrl(issueKey: string): string | null {
  return jiraBrowseUrl(issueKey)
}
