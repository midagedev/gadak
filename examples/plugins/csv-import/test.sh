#!/usr/bin/env bash
# Self-test for the csv-import example plugin.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
python3 "$DIR/csv_import.py" --self-test
python3 "$DIR/csv_import.py" --help >/dev/null
echo "csv-import test.sh ok"
