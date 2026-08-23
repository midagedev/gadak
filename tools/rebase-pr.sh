#!/usr/bin/env bash
# Rebase a PR branch onto origin/main, resolving the two conflicts every PR in
# this repository hits, and nothing else.
#
# Why this exists: PRs here are rare by policy (only changes touching desktop/,
# .github/workflows/ or the pack scripts — everything else goes straight to
# main), so when two or three are open at once, every push to main puts each of
# them behind and they conflict in exactly two places:
#
#   CHANGELOG.md / CHANGELOG.ko.md   the reference-link tail — both sides are
#                                    correct, so keep both, deduped by line
#   examples/backlog-snapshot.tar.gz a packed archive — bytes never merge, so
#                                    regenerate it from the mirror instead
#
# Anything else conflicting is a real conflict: this script stops with exit 2
# and names the file rather than guessing.
#
# Usage:  bash tools/rebase-pr.sh <branch>      # then push --force-with-lease
#
# It works in a throwaway detached worktree under ../gadak-wt/rb-<branch>, so
# your own tree — including uncommitted work — is never touched. It does not
# push: reviewing the result and pushing stay with the lead.
set -uo pipefail

BR="${1:-}"
if [[ -z "$BR" ]]; then
  echo "usage: bash tools/rebase-pr.sh <branch>" >&2
  exit 64
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WT="$ROOT/../gadak-wt/rb-$BR"

git -C "$ROOT" fetch origin --quiet
git -C "$ROOT" worktree remove --force "$WT" 2>/dev/null
if ! git -C "$ROOT" worktree add -f --detach "$WT" "origin/$BR" >/dev/null 2>&1; then
  echo "rebase-pr: no origin/$BR" >&2
  exit 65
fi
cd "$WT" || exit 1

BEFORE="$(git rev-parse HEAD)"
git rebase origin/main
while [[ -d "$(git rev-parse --git-path rebase-merge)" || -d "$(git rev-parse --git-path rebase-apply)" ]]; do
  conflicted=$(git diff --name-only --diff-filter=U)
  if [[ -z "$conflicted" ]]; then
    git -c core.editor=true rebase --continue </dev/null
    continue
  fi
  for f in $conflicted; do
    case "$f" in
      examples/backlog-snapshot.tar.gz)
        # Either side is a throwaway: the archive is regenerated below from the
        # mirror, which is the only thing that knows the current backlog.
        git checkout --theirs -- "$f" 2>/dev/null || git checkout --ours -- "$f"
        git add "$f"
        ;;
      CHANGELOG.md|CHANGELOG.ko.md)
        python3 - "$f" <<'PY'
import re, sys
path = sys.argv[1]
src = open(path, encoding='utf-8').read()

def keep_both(m):
    lines = []
    for block in (m.group(1), m.group(2)):
        for line in block.split('\n'):
            if line not in lines:
                lines.append(line)
    return '\n'.join(lines) + '\n'

src = re.sub(
    r'<<<<<<< [^\n]*\n(.*?)=======\n(.*?)>>>>>>> [^\n]*\n',
    keep_both, src, flags=re.S)
open(path, 'w', encoding='utf-8').write(src)
PY
        git add "$f"
        ;;
      *)
        echo "rebase-pr: unexpected conflict in $f — resolve it yourself" >&2
        exit 2
        ;;
    esac
  done
  git -c core.editor=true rebase --continue
done

if [[ "$(git rev-parse HEAD)" == "$BEFORE" ]]; then
  # Already on top of origin/main. Amending here would only churn the SHA and
  # cancel the CI run already going on it, so stop instead.
  echo "rebase-pr: $BR was already up to date — nothing rewritten ($(git rev-parse --short HEAD))"
  exit 0
fi

bash tools/backlog-snapshot.sh >/dev/null 2>&1 && git add examples/backlog-snapshot.tar.gz
if ! git diff --cached --quiet; then
  git -c core.editor=true commit --amend --no-edit >/dev/null
fi

echo "rebase-pr: $BR is now $(git rev-parse --short HEAD)"
git log --oneline origin/main..HEAD
echo "rebase-pr: worktree $WT — push with:"
echo "  git -C $WT push --force-with-lease origin HEAD:$BR"
