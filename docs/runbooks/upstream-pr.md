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

## Shipping a PR the upstream has not merged

A good PR can sit for weeks on a maintainer who is busy, and gadak does not
have to wait behind it (user decision 2026-08-30: *"업스트림 머지 안된건
포크해서라도 나가자"*). Ship it from a fork branch, pinned, with an
expiry note.

Do this only when the defect is **reachable in gadak** — say how, in the
`replace` comment. wails#6006's reachability is one line of upstream source
(`webview_window_windows.go` registers a `"*"` filter, so every Windows
request runs the handler that called `log.Fatal`). A PR that is merely good
is an upstream contribution, not a dependency change.

Go, measured 2026-08-30 on `desktop/` (wails v3):

```bash
# 1. A distribution branch: the tag we are on, plus the PR's commits only.
git -C ~/repo/<fork> branch -f gadak/<upstream-tag> <upstream-tag>
git -C ~/repo/<fork> checkout gadak/<upstream-tag>
git -C ~/repo/<fork> cherry-pick <pr-sha>...            # the PR's commits
git -C ~/repo/<fork> tag <upstream-tag>-gadak.1
git -C ~/repo/<fork> push fork gadak/<upstream-tag> <upstream-tag>-gadak.1

# 2. Pin it.
go mod edit -replace <module>=<fork-module>@<upstream-tag>-gadak.1
go mod tidy && go build ./... && GOOS=windows go build ./...
```

Two things that are not obvious and cost a round if guessed:

- **The fork's `go.mod` module line stays as upstream's.** Go resolves the
  package path from the *original* module path and only checks that the
  replacement's declared path matches the *replacement requirement* — so a
  fork works with no import rewriting at all. (Rewriting the module path is
  the folklore answer; it would break every internal import.)
- **Tag the fork branch.** Without a tag, `go mod tidy` derives a
  pseudo-version from the nearest tag reachable *in the fork*, which was
  `v3.0.0-beta.9.0.2026…` for a branch cut from beta.12 — a `go.mod` line
  that reads like a downgrade. `<upstream-tag>-gadak.N` reads like what it
  is and sorts as a prerelease.

- **A pin gate that reads the build has to be taught about the replace.**
  `debug.ReadBuildInfo()` reports the *replacement's* version, so
  `desktop.TestWailsModuleVersionMatchesGoMod` went red on the very commit
  that added the fork. The fix is not to relax it: the gate now reads the
  `replace` line too and asserts the fork version is *prefixed by* the
  required upstream version — so `v3.0.0-beta.12-gadak.1` passes while a
  fork silently cut from a different base still fails. Name the fork tag
  `<upstream-tag>-gadak.N` and that holds for free.

Then, in the repo: the `replace` carries a comment saying which PR, why the
defect is reachable, and **"delete this when it merges"**. A `replace` with
no expiry is how a fork becomes permanent by accident. Every upstream bump
redoes the branch (new tag → cherry-pick → `-gadak.N+1`) until the merge.

A `desktop/` change needs a PR — the Windows and Linux desktop jobs are the
ones local gates cannot run (CLAUDE.md).

## Lead-only boundary

Fork, push, `gh pr create`, replies, and any tracker write are lead actions.
Delegated rounds research, patch, and measure; they never touch GitHub.
