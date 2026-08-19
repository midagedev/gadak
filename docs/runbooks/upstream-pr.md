# Upstream PR quality pipeline

How this repository sends changes to upstream projects (wails etc.) so a PR
arrives with zero predictable review comments. Every step below is here
because skipping it cost — or nearly cost — a real submission. Reference
case: wailsapp/wails#6000 (GTK4 launch events, 2026-08-19), which passed
CodeRabbit/semgrep/snyk/license with one bot nit that was resolved with a
single reply and no code change.

## The pipeline (each step is a separate delegated round)

### 1. Prior art — before writing any code

One research round in a fresh clone of the target repo. Deliverables:

- Duplicate search: issues AND PRs AND discussions, queries listed verbatim
  in the report so absence is checkable. (GitHub's REST discussion search
  returns false zeros — use GraphQL.)
- Classification against the target's own contribution rules, quoted: bug
  fix vs enhancement vs proposal (for wails: bug fix = plain PR, new public
  behaviour = WEP draft first).
- **Merged sibling precedents** — the two or three closest merged PRs, who
  reviewed them, what evidence they carried. The new PR copies that shape;
  taste arguments lose to "same class as #X, already merged".
- What NOT to touch (open PRs that look adjacent but are different files or
  platforms — stacking on them is how a clean fix inherits someone else's
  review debt).

### 2. Measured FAIL-first — a code-read bug is a hypothesis, not a bug

Never submit a fix whose defect was established only by reading source.
One probe round must observe the bad behaviour on the target's master, on a
real environment, before the PR is drafted.

This is not ceremony. Case: the dock-reopen "bug" (candidate 4, 2026-08-19)
— code reading and AppKit docs both said the handler's gate never fires, and
that was true, but a measured probe showed AppKit's default reopen handling
restores the window anyway (delegate returns TRUE). The user-visible bug did
not exist on macOS 26; the PR was killed before it embarrassed us. The GTK4
PR survived the same test: a headless container A/B showed the event firing
on the patch and absent on the parent commit, for the positive rows, the
negative rows, and the non-regressed sibling tag.

### 3. The patch — minimal, single-purpose, sibling-parity

- One fix per PR, byte-for-byte parity with the sibling implementation the
  target already merged, including log strings. Divergence is what reviewers
  flag; parity is what makes review instant.
- Changelog entry in the target's own format, author filled, PR number
  placeholder swapped for the real number immediately after opening (amend +
  force-push before anyone reviews).
- Build and test both variants (default and tagged sibling) in a container
  pinned to versions the report names.

### 4. Pre-flight adversarial review — predict the bots

Before submitting, run a fresh-eyes review of the final diff + PR body,
asking one question: **what will an automated reviewer or a tired
maintainer flag?** The predictable classes:

- new code logging raw user data (URLs, paths, argv) — the one nit #6000
  got, and it was 100% predictable;
- missing tests / unchecked "tested" boxes without an explanation;
- magic numbers, swallowed errors, TODOs;
- claims in the body the diff does not support.

For each anticipated flag: either fix it, or **pre-empt it in the PR body
with a one-sentence rationale** ("the debug lines are byte-for-byte copies
of the GTK3/Windows block; redacting only this copy would break parity —
follow-up welcome for all three hosts"). A pre-empted objection costs one
sentence; an unanswered one costs a review round-trip and makes the PR look
unconsidered.

### 5. The body — evidence, honestly bounded

- "How Has This Been Tested" states exactly what was and was not run. If the
  test was headless, say headless and say what a desktop test would add. An
  overclaim discovered by a maintainer poisons the whole PR; an honest
  boundary invites them to run the one step you could not.
- Cite the sibling precedents and the commit that introduced the regression.
  "Fixes #" only when the issue actually exists; do not open a tracker issue
  just to have one (the target's template says "where one exists").
- Provenance sentence when code is adapted from the target's own tree.

### 6. After submission — reply protocol

- Answer bot findings with verifiable facts (file paths, PR numbers the bot
  can check itself), not opinions. CodeRabbit re-verified the parity claim
  with its own script and withdrew the finding; that outcome is available to
  any reply that gives it something checkable.
- Offer the follow-up ("happy to do all three hosts together") instead of
  arguing scope. Do not push new commits to answer a comment a sentence can
  answer — every push re-triggers review.
- Do not open follow-up issues in the target's tracker on a bot's
  suggestion; that is the maintainers' call.

## Lead-only boundary

Fork, push, `gh pr create`, replies, and any tracker write are lead actions.
Delegated rounds research, patch, and measure; they never touch GitHub.
