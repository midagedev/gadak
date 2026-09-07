// Thin typed wrappers over request() for every write the phone sends
// (GDK-1497 A2). Screens hold no request bodies — the method, path, and
// JSON body for each verb live here once, mirroring web/src/lib/api.ts's
// write wrappers so the two companions cannot drift apart silently: the
// assignee PUT means {account_id} on both, null clearing on both.

import { request, ApiError, type FetchLike, type ApiSession, type Envelope } from './api'
import type {
  CreateIssuePayload,
  CreateMetaResponse,
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
