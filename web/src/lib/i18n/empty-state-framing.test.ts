/*
 * GDK-1189 recurrence gate: an empty state must not open by naming a lack.
 *
 * The sidebar said, permanently, in every account-free workspace:
 *
 *   "The personal feed needs an identity — this workspace runs without an
 *    account"
 *
 * The second clause is a thing this product is *for*. The first clause
 * reframes it as something missing, and because it comes first the whole
 * sentence reads as a warning. All three blind reviewers of the 0.19 release
 * video (GDK-1159) flagged it unprompted — "something isn't set up" — on a
 * film shot to show an account-free workspace off. The sidebar was arguing
 * against the footage.
 *
 * It cannot be turned off, either: the gate is a credential, not an actor
 * (stores/me.svelte.ts — `identified` is email !== null), so removing the
 * sentence would mean giving the workspace an account, which is the opposite
 * of the thing being demonstrated. So it is a copy contract, and this is it:
 * a strings-level rule, because it holds for strings nobody has written yet.
 *
 * The rule: an account-free empty state states the property first. It may
 * still say what an account would add — after.
 *
 * FAIL-first, on the strings before this round:
 *   personal.builtInNoIdentity "The personal feed needs an identity — this
 *     workspace runs without an account" opens with a lack
 *   personal.demoNoIdentity "The personal feed needs a Jira identity — not
 *     available in the demo" opens with a lack
 */
import { describe, expect, test } from 'vitest'

import { messages } from './catalog'

/** The surfaces that describe an account-free workspace to its user. Only
 *  `en` is asserted: ko and ja are the lead's to write (project CLAUDE.md —
 *  Korean and Japanese prose is not delegated), so a translated string is not
 *  this gate's business until it lands. */
const ACCOUNT_FREE_KEYS = ['personal.builtInNoIdentity', 'personal.demoNoIdentity'] as const

/** "needs an identity", "requires an account", "not available", "no identity"
 *  — the openers that make an absence the subject of the sentence. Matched
 *  only against the first clause, because saying what an account adds *after*
 *  the property is exactly the shape we want. */
const LACK_OPENER = /\b(needs?|requires?|missing|unavailable|not available|no)\b/i

/** Everything before the first em dash, period, or comma. */
function firstClause(s: string): string {
  return s.split(/\s+—\s+|[.,]/)[0]
}

describe('GDK-1189 account-free empty states lead with the property', () => {
  test.each(ACCOUNT_FREE_KEYS)('%s does not open by naming a lack', (key) => {
    const en = (messages as Record<string, Record<string, string>>)[key].en
    const opener = firstClause(en)
    expect(
      LACK_OPENER.test(opener) ? `"${opener}"` : '(none)',
      `${key}: running without an account is what this product is for — say that first, ` +
        `then what an account would add. Full string: "${en}"`,
    ).toBe('(none)')
  })

  // The other half of the contract: the sentence must still carry the fact,
  // or "don't lead with a lack" would be satisfied by saying nothing at all.
  test.each(ACCOUNT_FREE_KEYS)('%s still says the workspace has no account', (key) => {
    const en = (messages as Record<string, Record<string, string>>)[key].en
    expect(en.toLowerCase()).toMatch(/account|identity/)
  })
})
