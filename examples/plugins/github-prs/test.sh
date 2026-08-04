#!/usr/bin/env bash
# Self-test for the github-prs example plugin.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
python3 "$DIR/github_prs.py" --self-test
python3 "$DIR/github_prs.py" --help >/dev/null
echo "github-prs test.sh ok"
