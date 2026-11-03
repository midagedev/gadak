// Thin typed wrappers over request() for every write the phone sends
// (GDK-1497 A2). Screens hold no request bodies — the method, path, and
// JSON body for each verb live here once, mirroring web/src/lib/api.ts's
// write wrappers so the two companions cannot drift apart silently: the
// assignee PUT means {account_id} on both, null clearing on both.

import { request, ApiError, type FetchLike, type ApiSession, type Envelope } from './api'
import type {
  CreateIssuePayload,
  CreateMetaResponse,
  IssueLite,
  IssueWriteResponse,
  PrioritiesResponse,
  UsersResponse,
} from './types'

/** Test seam + search abort, forwarded verbatim into request(). */
export interface WriteOpts {
  session?: ApiSession
  fetchFn?: FetchLike
  signal?: AbortSignal
}

/** Unwraps the envelope — a 2xx write always carries its issue back. */
async function unwrap<T>(pending: Promise<Envelope<T>>): Promise<T> {
  const env = await pending
  if (env.body === null) throw new ApiError('bad_response', env.status)
  return env.body
}

/** PUT `<key>/assignee/` — `null` clears (the Unassigned row). */
export function setAssignee(
  issueKey: string,
  accountId: string | null,
  opts: WriteOpts = {},
): Promise<IssueWriteResponse> {
  return unwrap(
    request(`issues/${encodeURIComponent(issueKey)}/assignee/`, {
      method: 'PUT',
      body: { account_id: accountId },
      ...opts,
    }),
  )
}

/**
 * The stale-assignee refusal (GDK-2073): what the 409 assignee_changed body
 * carried — the refreshed row naming the holder this cache did not show, and
 * the holder's display name (null when nobody holds it) — or null when err
 * is anything else. Lives here rather than in the sheet so the wire shape
 * has one owner and a test that needs no DOM mount: AssigneeSheet keeps the
 * sheet open on it and says who took the issue; picking again replaces them,
 * because the refusal refreshed the server's cache.
 */
export function assigneeStaleRefusal(err: unknown): { issue: IssueLite; holderName: string | null } | null {
  if (!(err instanceof ApiError) || err.code !== 'assignee_changed' || !err.body) return null
  const issue = err.body.issue as IssueLite | undefined
  if (!issue || !issue.issue_key) return null
  const holder = err.body.current_assignee as { display_name?: string } | null | undefined
  return { issue, holderName: holder?.display_name || null }
}

/** PUT `<key>/priority/` — `null` clears (the None row). Site id, never the name. */
export function setPriority(
  issueKey: string,
  priorityId: string | null,
  opts: WriteOpts = {},
): Promise<IssueWriteResponse> {
  return unwrap(
    request(`issues/${encodeURIComponent(issueKey)}/priority/`, {
      method: 'PUT',
      body: { priority_id: priorityId },
      ...opts,
    }),
  )
}

/** PUT `<key>/summary/` — trim before sending; empty is refused client-side. */
export function setSummary(
  issueKey: string,
  summary: string,
  opts: WriteOpts = {},
): Promise<IssueWriteResponse> {
  return unwrap(
    request(`issues/${encodeURIComponent(issueKey)}/summary/`, {
      method: 'PUT',
      body: { summary },
      ...opts,
    }),
  )
}

/**
 * PUT `<key>/description/` — markdown; null or whitespace clears. 409
 * format_loss (the phone's plain textarea would drop formatting) is
 * answered by re-calling with `force: true`. 409 placeholder carries the
 * server's sentence in ApiError.serverMessage.
 */
export function setDescription(
  issueKey: string,
  description: string | null,
  opts: WriteOpts & { force?: boolean } = {},
): Promise<IssueWriteResponse> {
  const { force, ...rest } = opts
  return unwrap(
    request(`issues/${encodeURIComponent(issueKey)}/description/`, {
      method: 'PUT',
      body: force ? { description, force: true } : { description },
      ...rest,
    }),
  )
}

/**
 * PUT `<key>/labels/` — the whole set, not a delta: the server replaces the
 * field with what it is given (write.go handleLabels), trimming and deduping
 * on the way in. A label with a space in it is a label Jira refuses, so the
 * caller splits before it gets here (lib/labels.ts).
 */
export function setLabels(
  issueKey: string,
  labels: string[],
  opts: WriteOpts = {},
): Promise<IssueWriteResponse> {
  return unwrap(
    request(`issues/${encodeURIComponent(issueKey)}/labels/`, {
      method: 'PUT',
      body: { labels },
      ...opts,
    }),
  )
}

/** PUT `<key>/duedate/` — `YYYY-MM-DD`, or `null` to clear it. */
export function setDuedate(
  issueKey: string,
  duedate: string | null,
  opts: WriteOpts = {},
): Promise<IssueWriteResponse> {
  return unwrap(
    request(`issues/${encodeURIComponent(issueKey)}/duedate/`, {
      method: 'PUT',
      body: { duedate },
      ...opts,
    }),
  )
}

/** POST `create/` — the server resolves the default issue type; only
 *  summary is required, and the phone sends just what the person filled. */
export function createIssue(
  payload: CreateIssuePayload,
  opts: WriteOpts = {},
): Promise<IssueWriteResponse> {
  return unwrap(request('issues/create/', { method: 'POST', body: payload, ...opts }))
}

/** GET `create-meta/` — more than one project means the sheet asks which. */
export function getCreateMeta(opts: WriteOpts = {}): Promise<CreateMetaResponse> {
  return unwrap(request('issues/create-meta/', opts))
}

/** GET `<key>/priorities/` — per-key: Linear rows answer 0-4, Jira rows the site list. */
export function getPriorities(issueKey: string, opts: WriteOpts = {}): Promise<PrioritiesResponse> {
  return unwrap(request(`issues/${encodeURIComponent(issueKey)}/priorities/`, opts))
}

/** GET `<key>/users/?q=` — the debounced keystrokes' search; abortable. */
export function searchUsers(
  issueKey: string,
  q: string,
  opts: WriteOpts = {},
): Promise<UsersResponse> {
  return unwrap(
    request(`issues/${encodeURIComponent(issueKey)}/users/?q=${encodeURIComponent(q)}`, opts),
  )
}
