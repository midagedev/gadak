#!/usr/bin/env bash
# Self-test for the deploy-status example plugin.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
python3 "$DIR/deploy_status.py" --self-test
python3 "$DIR/deploy_status.py" --help >/dev/null
echo "deploy-status test.sh ok"
