/*
 * buildViewLink's three answers, and the one it used to give twice
 * (GDK-1861).
 *
 * The clipboard for a view is an origin address when the origin has one and
 * the JQL carries the whole view; the app lines alone when it does not; and
 * both when a clause could not become JQL (GDK-1858). The failure of the
 * emit is a fourth state and had no spelling: `catch` returned the same
 * `{ origin: false, omitted: [] }` a built-in tracker gets, so the toast said
 * "Copied" and the fact that Jira could not be addressed disappeared.
 *
 * FAIL-first (2026-09-18, with the catch returning `originFailed: false`
 * again — the answer it used to give):
 *
 *	× an emit that throws says so, instead of looking like a built-in
 *	  AssertionError: expected false to be true
 *
 * which is the defect in one line: the failure and the built-in tracker gave
 * the same answer, and the toast can only read the answer.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

const emitJql = vi.fn()
vi.mock('./api', () => ({ emitJql: (...a: unknown[]) => emitJql(...a) }))

const cfg = {
  originType: 'jira',
  profile: 'default',
  hostedDemo: false,
  siteUrl: 'https://example.atlassian.net',
}
vi.mock('./config', async (importOriginal) => {
  const real = (await importOriginal()) as Record<string, unknown>
  return {
    ...real,
    config: () => cfg,
    isDesktop: () => true,
    isHostedDemo: () => cfg.hostedDemo,
    appMountPath: () => '/',
    profileName: (p: string) => p,
    jiraFilterUrl: (_: string, jql: string) =>
      cfg.siteUrl ? `${cfg.siteUrl}/issues/?jql=${encodeURIComponent(jql)}` : '',
  }
})

// The suite runs in node, so the address bar has to be supplied. Only the
// two fields view-link reads.
vi.stubGlobal('location', { hash: '#/?sc=inprogress', origin: 'http://127.0.0.1:7777' })

const { buildViewLink } = await import('./view-link')

const VIEW = { filters: {}, display: {} } as never

describe('buildViewLink (GDK-1858, GDK-1861)', () => {
  beforeEach(() => {
    emitJql.mockReset()
    cfg.originType = 'jira'
    cfg.hostedDemo = false
    cfg.siteUrl = 'https://example.atlassian.net'
    vi.stubGlobal('location', { hash: '#/?sc=inprogress', origin: 'http://127.0.0.1:7777' })
  })

  it('a Jira view whose JQL carries everything copies the origin address alone', async () => {
    emitJql.mockResolvedValue({ jql: 'statusCategory in ("In Progress")', omitted: [] })
    const link = await buildViewLink(VIEW)
    expect(link.origin).toBe(true)
    expect(link.originFailed).toBe(false)
    expect(link.text.split('\n')).toHaveLength(1)
  })

  it('a clause the JQL cannot carry brings the app link along and names it', async () => {
    emitJql.mockResolvedValue({ jql: 'project = STD', omitted: ['reopened'] })
    const link = await buildViewLink(VIEW)
    expect(link.origin).toBe(true)
    expect(link.originFailed).toBe(false)
    expect(link.omitted).toEqual(['reopened'])
    expect(link.text.split('\n')).toHaveLength(2)
  })

  it('a workspace with no origin address copies the app link and calls it ordinary', async () => {
    cfg.originType = 'gadak'
    const link = await buildViewLink(VIEW)
    expect(link.origin).toBe(false)
    // Not a failure: the built-in tracker has no site, so the app link is
    // the whole truth and the toast that says "Copied" is right.
    expect(link.originFailed).toBe(false)
    expect(emitJql).not.toHaveBeenCalled()
  })

  it('an emit that throws says so, instead of looking like a built-in (GDK-1861)', async () => {
    emitJql.mockRejectedValue(new Error('502'))
    const link = await buildViewLink(VIEW)
    expect(link.origin).toBe(false)
    expect(link.originFailed).toBe(true)
    // The payload is the same as the built-in case above; the difference is
    // the one bit the toast reads, and that bit is the whole issue.
    expect(link.text).not.toContain('atlassian.net')
  })
})
