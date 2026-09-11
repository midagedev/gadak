import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

/*
 * GDK-1152 source gate: components must not re-derive "can this origin do X"
 * from the signals that only look like it — auth/me's identity, the workspace
 * kind, or an empty jiraBaseUrl. A surface with that question asks the origin's
 * capability statement instead: `can(...)`, `credentialRequired()`, or the
 * legacy alias `originWritable()` from lib/config.
 *
 * A source scan, not a render test, on purpose: the defect class lives in
 * which *expression* a component branches on, and every one of these tokens in
 * a component is either a migrated-away affordance (this gate's red) or a
 * consciously reviewed use (the allowlist below). The scan cannot tell an
 * affordance from copy, so each allowlisted file carries its reason and its
 * exact hit count — a NEW hit in an allowlisted file is still red (the count
 * drifts), and a file whose hits drop must shed its entry. Migrating an
 * entry means decrementing here in the same change; the ideal end state is an
 * empty allowlist.
 *
 * Allowed homes for these tokens (not scanned): lib/config.ts (the selector
 * vocabulary itself), lib/workspace.ts (the parsers), stores/ (identity state),
 * and *.test.ts. mobile/ imports lib/config and reads the same block.
 */

const COMPONENTS_ROOT = join(fileURLToPath(new URL('.', import.meta.url)), '..', 'components')

// The guessing vocabulary. me.identified answers "who is reading" — empty on
// built-in, paired, and Linear workspaces that write fine, which is exactly
// why it must never gate a write affordance. workspaceKind / WORKSPACE_KIND_*
// / isBuiltIn(Workspace)?() say which shell built the workspace, not what the
// origin accepts. jiraBaseUrl-emptiness is also the hosted demo's shape.
const GUESS =
  /me\.identified|\bworkspaceKind\b|\bWORKSPACE_KIND_(?:STANDALONE|CONNECTED)\b|\bisBuiltIn(?:Workspace)?\s*\(|\bjiraBaseUrl\b/g

type AllowEntry = { hits: number; reason: string }

// file (relative to web/src) → reviewed residual use.
const ALLOWLIST: Record<string, AllowEntry> = {
  'components/write/TitleEditor.svelte': {
    hits: 1,
    reason:
      'title edit affordance still keyed on identity — same class DescriptionEditor/FieldEditor left; write/ is outside this round\'s file boundary',
  },
  'components/write/PriorityPicker.svelte': {
    hits: 1,
    reason:
      'priority edit affordance still keyed on identity — same class; write/ is outside this round\'s file boundary',
  },
  'components/write/StatusTransition.svelte': {
    hits: 2,
    reason:
      'status edit affordance and its counter still keyed on identity — same class; write/ is outside this round\'s file boundary',
  },
  'components/write/CommentComposer.svelte': {
    hits: 2,
    reason:
      'issue-comment placeholder keeps an identity term beside the GDK-1148 originWritable() one, and the attach button disables on identity — write/ is outside this round\'s file boundary',
  },
  'components/write/AssigneePicker.svelte': {
    hits: 2,
    reason:
      'meMember resolves which member row is "me" (identity by definition — assignee search needs the email, not the origin), and the tooltip branches on the same identity to say whose reassignment it is',
  },
  'components/palette/CommandPalette.svelte': {
    hits: 2,
    reason:
      'personal commands (assigned-to-me, watched-by-me) are filtered by identity because their data is per-person — identity is the question there, not a proxy for origin ability',
  },
  'components/sidebar/SidebarNav.svelte': {
    hits: 5,
    reason:
      'identity-native navigation: the my-work view pack and the account row render who is reading (email display, "no mine while anonymous" per GDK-1342); the footer CTA itself already asks originWritable() (GDK-1148), and one hit is the comment documenting why',
  },
  'components/personal/MyIssuesNav.svelte': {
    hits: 3,
    reason:
      'the personal surfaces are keyed on identity by definition — whose issues these are is the question, not a proxy for what the origin accepts',
  },
  'components/personal/WatchButton.svelte': {
    hits: 2,
    reason: 'watch state belongs to a person; identity is the datum, not a capability proxy',
  },
  'components/personal/PersonalFeed.svelte': {
    hits: 1,
    reason: 'the feed is per-person; identity is the datum, not a capability proxy',
  },
  'components/list/BulkBar.svelte': {
    hits: 1,
    reason:
      'meMember resolves which member row is "me" for bulk assignment — identity is the datum, not a capability proxy',
  },
  'components/list/ListView.svelte': {
    hits: 1,
    reason:
      'empty-state copy names where issues come from on a workspace that has none yet (onboarding copy, not an affordance; no capability axis states mirroring coverage)',
  },
  'components/detail/DetailPanel.svelte': {
    hits: 1,
    reason:
      'PR empty-state copy distinguishes "no PRs" (built-in holds them natively) from "PRs not mirrored" (connected) — mirroring coverage, which no capability axis states',
  },
  'components/detail/FieldEditor.svelte': {
    hits: 1,
    reason:
      'meMember resolves which member row is "me" for assignment — identity is the datum; the edit affordance itself asks can(\'issueWrite\')',
  },
  'components/detail/LinkedIssues.svelte': {
    hits: 1,
    reason: 'comment only: documents why link types are NOT gated on identity (GDK-1032)',
  },
  'components/shell/FreshnessChip.svelte': {
    hits: 1,
    reason:
      'sync-freshness copy names what the timestamp is on a workspace that syncs vs one that is its own origin (GDK-1325) — a copy axis with no capability counterpart',
  },
  'components/settings/RuntimeMirror.svelte': {
    hits: 1,
    reason:
      'another round\'s file (read-only here); the branch picks mirror-path copy from the origin type, not an affordance',
  },
}

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir).sort()) {
    const full = join(dir, name)
    const st = statSync(full)
    if (st.isDirectory()) {
      walk(full, out)
    } else if (name.endsWith('.svelte') || (name.endsWith('.ts') && !name.endsWith('.test.ts'))) {
      out.push(full)
    }
  }
  return out
}

function hitsIn(source: string): { line: number; text: string }[] {
  const found: { line: number; text: string }[] = []
  for (const m of source.matchAll(GUESS)) {
    const line = source.slice(0, m.index ?? 0).split('\n').length
    found.push({ line, text: m[0] })
  }
  return found
}

describe('capability vocabulary source gate (GDK-1152)', () => {
  test('no component re-derives origin capabilities from identity, kind, or an empty site URL', () => {
    const files = walk(COMPONENTS_ROOT)
    expect(files.length).toBeGreaterThan(50) // sanity: the walk found the tree

    const failures: string[] = []
    const seen = new Set<string>()
    for (const full of files) {
      const rel = relative(join(COMPONENTS_ROOT, '..'), full) // → components/...
      seen.add(rel)
      const found = hitsIn(readFileSync(full, 'utf8'))
      const allowed = ALLOWLIST[rel]
      if (!allowed) {
        if (found.length > 0) {
          failures.push(
            `${rel}: ${found.length} unlisted capability guess(es) — ask lib/config's can()/credentialRequired()/originWritable() instead:\n` +
              found.map((f) => `  :${f.line} ${f.text}`).join('\n'),
          )
        }
        continue
      }
      if (found.length > allowed.hits) {
        failures.push(
          `${rel}: ${found.length} hits, allowlist admits ${allowed.hits} (${allowed.reason}) — the extra ones are new guesses:\n` +
            found.map((f) => `  :${f.line} ${f.text}`).join('\n'),
        )
      }
    }

    // Stale entries must go: an allowlist row whose file vanished or whose
    // hits dropped is dead weight that hides progress.
    for (const rel of Object.keys(ALLOWLIST)) {
      if (!seen.has(rel)) {
        failures.push(`${rel}: allowlisted but no such file — remove the entry`)
      } else if (ALLOWLIST[rel].hits > 0) {
        const found = hitsIn(readFileSync(join(COMPONENTS_ROOT, '..', rel), 'utf8'))
        if (found.length < ALLOWLIST[rel].hits) {
          failures.push(
            `${rel}: ${found.length} hits now, allowlist still says ${ALLOWLIST[rel].hits} — shrink or remove the entry`,
          )
        }
      }
    }

    if (failures.length > 0) {
      throw new Error(
        'Capability vocabulary gate (GDK-1152):\n' +
          failures.join('\n') +
          '\nEvery surface that branches an affordance on me.identified / workspaceKind / isBuiltIn / jiraBaseUrl is guessing what only the origin can state. Migrate to can(...) in lib/config, or — if the use is genuinely identity-native (whose data, not what the origin accepts) — add or adjust the allowlist entry with its reason and exact hit count.',
      )
    }
  })
})
