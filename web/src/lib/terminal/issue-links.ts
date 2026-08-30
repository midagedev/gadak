/*
 * Issue keys in terminal output are links (GDK-1160).
 *
 * `git log`, a build log and an agent's own report already print keys into
 * this pane; until now they were dead text. The feature is not the underline
 * — it is the false-positive control. `ABC-123` is a shape a version string,
 * a part number and a diff hunk all take, so a provider that underlines the
 * shape turns every third line into a link and the pane into noise. Only
 * keys whose project the mirror actually covers are offered.
 *
 * Pure, and single-line by design: xterm asks a link provider one buffer
 * line at a time, so a key split across a wrap is not offered rather than
 * being guessed at. That is the same cut @xterm/addon-web-links makes.
 */

/**
 * The candidate shape. Anchored on word boundaries so `xGDK-1` and
 * `GDK-1x` are not candidates; the project half must start with a letter
 * because a Jira project key does.
 */
export const ISSUE_KEY_RE = /\b[A-Z][A-Z0-9]+-\d+\b/g

/** One offered link: the key, and where it sits in the line (0-based, end
 *  exclusive — the half-open convention the rest of this codebase uses). */
export interface IssueKeyMatch {
  key: string
  start: number
  end: number
}

/**
 * Every issue key in `line` whose project is in `projects`.
 *
 * `projects` is matched case-sensitively against the uppercase half of the
 * key: mirror project keys are uppercase, and lowercasing here would let
 * `abc-1` in prose become a link.
 */
export function findIssueKeyMatches(
  line: string,
  projects: Iterable<string>,
): IssueKeyMatch[] {
  const known = projects instanceof Set ? projects : new Set(projects)
  if (known.size === 0) return []
  const out: IssueKeyMatch[] = []
  // A fresh regex per call: ISSUE_KEY_RE is global, and a shared lastIndex
  // across calls would skip matches in every other line.
  const re = new RegExp(ISSUE_KEY_RE.source, 'g')
  for (const m of line.matchAll(re)) {
    const key = m[0]
    const project = key.slice(0, key.lastIndexOf('-'))
    if (!known.has(project)) continue
    out.push({ key, start: m.index, end: m.index + key.length })
  }
  return out
}

/**
 * The project keys this workspace can offer links for: the mirror's own
 * `projects` setting when an install narrows it, otherwise the distinct
 * project halves of the keys already in the pool.
 *
 * GDK-1177: the fallback used to read `source_project`, which is the
 * *cloned-from* project (store/read.go), not the issue's own — null on every
 * issue that was not cloned, so a standalone workspace (which also leaves
 * `projects` empty, meaning "mirror everything") offered no links at all.
 * The issue's own key is the thing the mirror actually covers.
 */
export function knownProjectKeys(
  configured: Iterable<string> | undefined,
  issues: Iterable<{ issue_key: string }>,
): Set<string> {
  const keys = new Set<string>(configured ?? [])
  if (keys.size > 0) return keys
  for (const issue of issues) {
    const i = issue.issue_key.lastIndexOf('-')
    if (i > 0) keys.add(issue.issue_key.slice(0, i))
  }
  return keys
}
