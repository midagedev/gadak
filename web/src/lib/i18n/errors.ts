import type { MessageKey } from './catalog'

/**
 * `fail()` / `failJira` / `failCreate` codes from internal/server/write.go →
 * catalog keys. Only codes whose sentence differs from the operation generic
 * (or is reused from an existing write.* key). Unknown snake_case codes must
 * not appear here — writeErrorMessage falls those back to the caller's generic
 * sentence. failCreate's codes are derived from write.go by catalog.test.ts.
 */
export const WRITE_ERROR_KEYS = {
  // Same latch for pulls and writes (GDK-507): the sentence carries the
  // unfreeze path, so it serves both.
  workspace_frozen: 'sync.frozen',
  credential_required: 'write.needToken',
  credential_rejected: 'write.tokenRejected',
  workspace_busy: 'write.workspaceBusy',
  jira_unavailable: 'write.jiraUnavailable',
  write_applied_mirror_stale: 'write.mirrorStale',
  not_found: 'write.notFound',
  summary_required: 'write.titleRequired',
  summary_too_long: 'write.summaryTooLong',
  project_issue_type_and_summary_required: 'write.requiredFields',
  project_required: 'write.projectRequired',
  issue_type_required: 'write.issueTypeRequired',
  priority_required: 'write.priorityRequired',
  project_not_mirrored: 'write.projectNotMirrored',
  field_not_editable: 'write.fieldNotEditable',
  site_required: 'write.siteRequired',
  invalid_token_expires: 'onboarding.errExpires',
} as const satisfies Record<string, MessageKey>

const GADAK_ERROR_CODE = /^[a-z][a-z0-9_]*$/

/**
 * Map a write-endpoint `error` body to a user-facing sentence.
 * Known codes → catalog. Unknown snake_case codes → fallback (never the raw
 * code). Anything else is Jira prose from failJira's APIError.Message() and
 * is returned as-is.
 */
export function writeErrorMessage(
  code: string | null | undefined,
  fallback: string,
  translate: (key: MessageKey) => string,
): string {
  if (!code) return fallback
  if (Object.prototype.hasOwnProperty.call(WRITE_ERROR_KEYS, code)) {
    return translate(WRITE_ERROR_KEYS[code as keyof typeof WRITE_ERROR_KEYS])
  }
  if (GADAK_ERROR_CODE.test(code)) return fallback
  return code
}
