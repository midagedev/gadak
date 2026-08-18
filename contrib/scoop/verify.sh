#!/usr/bin/env bash
# Check the Scoop manifest against the latest tag and that tag's checksums.txt.
#
# Usage: contrib/scoop/verify.sh [gadak.json]
#
# What it asserts (offline-checkable; this is not `scoop install`):
#   1. gadak.json is valid JSON and validates against Scoop's published schema
#      (required fields, additionalProperties, hash/version patterns)
#   2. version equals `git describe --tags --abbrev=0` with the leading v
#      stripped — same tag source as tools/doc-checks.sh. Skipped (not
#      failed) when no tag is reachable, same as that script.
#   3. architecture.64bit / arm64 hashes match that tag's checksums.txt
#      (checksums.txt is the owner; this script does not compute hashes)
#   4. url filenames are gadak_<ver>_windows_{amd64,arm64}.zip
#   5. bin is gadak.exe (v0.15.2 zip members, unzip -l: no directory prefix)
#   6. checkver's GitHub regex matches the latest release
#
# Exit 64 = usage / bad arguments
#      69 = a required tool is missing
#       1 = a verification above failed
#       0 = all of them passed
#
# Requires: curl, python3, git, awk, mktemp, rm.
set -euo pipefail

usage() {
  echo "usage: contrib/scoop/verify.sh [gadak.json]" >&2
  exit 64
}

need() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "verify.sh: missing ${name}" >&2
    exit 69
  fi
}

if [[ $# -gt 1 ]]; then
  usage
fi
case "${1:-}" in
  -h|--help) usage ;;
esac

need curl
need python3
need git
need awk
need mktemp
need rm

self="${BASH_SOURCE[0]}"
if [[ "$self" != /* ]]; then
  self="$(pwd)/$self"
fi
here="${self%/*}"
repo="$(cd "${here}/../.." && pwd)"
manifest="${1:-${here}/gadak.json}"
if [[ "$manifest" != /* ]]; then
  manifest="$(pwd)/$manifest"
fi
if [[ ! -f "$manifest" ]]; then
  echo "verify.sh: gadak.json not found at ${manifest}" >&2
  exit 1
fi

schema_url="https://raw.githubusercontent.com/ScoopInstaller/Scoop/master/schema.json"
checkver_regex='/releases/tag/(?:v|V)?([\d.]+)'
# Scoop bin/checkver.ps1 default for checkver.github (field-read 2026-08-18).

work="$(mktemp -d "${TMPDIR:-/tmp}/gadak-scoop-verify.XXXXXX")"
cleanup() { rm -rf "$work"; }
trap cleanup EXIT INT HUP TERM

if ! curl -fsSL -o "${work}/schema.json" "$schema_url"; then
  echo "verify.sh: download failed: ${schema_url}" >&2
  exit 1
fi

# Schema walk is stdlib-only. jsonschema is not a repo dependency.
GADAK_SCOOP_MANIFEST="$manifest" GADAK_SCOOP_SCHEMA="${work}/schema.json" \
python3 - <<'PY'
import json
import os
import re
import sys
from pathlib import Path
from urllib.parse import urlparse

manifest_path = Path(os.environ["GADAK_SCOOP_MANIFEST"])
schema_path = Path(os.environ["GADAK_SCOOP_SCHEMA"])
root = json.loads(schema_path.read_text())
instance = json.loads(manifest_path.read_text())
errors = []


def resolve(schema):
    if not isinstance(schema, dict):
        return schema
    ref = schema.get("$ref")
    if not ref:
        return schema
    if not ref.startswith("#/"):
        errors.append(f"unsupported $ref {ref}")
        return {}
    node = root
    for part in ref[2:].split("/"):
        node = node[part]
    return node


def check(inst, schema, path="$"):
    schema = resolve(schema)
    if schema is True:
        return
    if schema is False:
        errors.append(f"{path}: schema is false")
        return
    if not isinstance(schema, dict):
        return

    if "anyOf" in schema:
        before = len(errors)
        for i, sub in enumerate(schema["anyOf"]):
            sub_errs = []
            saved = errors
            # isolate
            globals_errors_holder = []
            # use a nested collector
            # (re-bind via a local list)
            # Implemented below with a helper that returns errs.
            sub_list = _validate(inst, sub)
            if not sub_list:
                errors[:] = saved
                return
            globals_errors_holder.append(sub_list)
        errors[:] = saved
        errors.append(f"{path}: matches no anyOf branch: {globals_errors_holder[0][:3]}")
        return

    expected = schema.get("type")
    if expected:
        types = expected if isinstance(expected, list) else [expected]
        ok_type = False
        for t in types:
            if t == "object" and isinstance(inst, dict):
                ok_type = True
            elif t == "array" and isinstance(inst, list):
                ok_type = True
            elif t == "string" and isinstance(inst, str):
                ok_type = True
            elif t == "boolean" and isinstance(inst, bool):
                ok_type = True
            elif t == "number" and isinstance(inst, (int, float)) and not isinstance(inst, bool):
                ok_type = True
            elif t == "integer" and isinstance(inst, int) and not isinstance(inst, bool):
                ok_type = True
            elif t == "null" and inst is None:
                ok_type = True
        if not ok_type:
            errors.append(f"{path}: want type {expected}, got {type(inst).__name__}")
            return

    if "pattern" in schema and isinstance(inst, str):
        if not re.search(schema["pattern"], inst):
            errors.append(f"{path}: does not match {schema['pattern']}")

    if schema.get("format") == "uri" and isinstance(inst, str):
        parsed = urlparse(inst)
        if parsed.scheme not in ("http", "https") or not parsed.netloc:
            errors.append(f"{path}: not an http(s) URI")

    if schema.get("format") == "regex" and isinstance(inst, str):
        try:
            re.compile(inst)
        except re.error as exc:
            errors.append(f"{path}: not a regex ({exc})")

    if isinstance(inst, dict):
        required = schema.get("required") or []
        for key in required:
            if key not in inst:
                errors.append(f"{path}: missing required {key}")
        props = schema.get("properties") or {}
        additional = schema.get("additionalProperties", True)
        for key, value in inst.items():
            if key in props:
                check(value, props[key], f"{path}.{key}")
            elif additional is False:
                errors.append(f"{path}: additional property {key}")
            elif isinstance(additional, dict):
                check(value, additional, f"{path}.{key}")

    if isinstance(inst, list) and "items" in schema:
        item_schema = schema["items"]
        for i, value in enumerate(inst):
            check(value, item_schema, f"{path}[{i}]")


def _validate(inst, schema):
    global errors
    saved = errors
    errors = []
    check(inst, schema, "$")
    got = errors
    errors = saved
    return got


check(instance, root)
if errors:
    print("FAIL: schema:", file=sys.stderr)
    for err in errors:
        print(f"  {err}", file=sys.stderr)
    raise SystemExit(1)
print("ok: schema (Scoop schema.json required/additionalProperties/patterns)")
PY

tag="$(git -C "$repo" describe --tags --abbrev=0 2>/dev/null || true)"
ver="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$manifest")"

if [[ -z "$tag" ]]; then
  echo "ok: no tag reachable — version-vs-tag guard skipped (same as tools/doc-checks.sh)"
  rel_tag="v${ver}"
else
  want="${tag#v}"
  if [[ "$ver" != "$want" ]]; then
    echo "FAIL: version: manifest ${ver} != latest tag ${tag}" >&2
    exit 1
  fi
  echo "ok: version ${ver} matches latest tag ${tag}"
  rel_tag="$tag"
fi

checksums_url="https://github.com/midagedev/gadak/releases/download/${rel_tag}/checksums.txt"
if ! curl -fsSL -o "${work}/checksums.txt" "$checksums_url"; then
  echo "verify.sh: download failed: ${checksums_url}" >&2
  exit 1
fi

GADAK_SCOOP_MANIFEST="$manifest" GADAK_SCOOP_CHECKSUMS="${work}/checksums.txt" \
GADAK_SCOOP_VER="$ver" python3 - <<'PY'
import json
import os
import sys
from pathlib import Path

manifest = json.loads(Path(os.environ["GADAK_SCOOP_MANIFEST"]).read_text())
ver = os.environ["GADAK_SCOOP_VER"]
sums = {}
for line in Path(os.environ["GADAK_SCOOP_CHECKSUMS"]).read_text().splitlines():
    parts = line.split()
    if len(parts) >= 2:
        sums[parts[1]] = parts[0]

want = {
    "64bit": f"gadak_{ver}_windows_amd64.zip",
    "arm64": f"gadak_{ver}_windows_arm64.zip",
}
arch = manifest.get("architecture") or {}
failed = False
for key, filename in want.items():
    block = arch.get(key) or {}
    url = block.get("url") or ""
    got_hash = (block.get("hash") or "").lower()
    if not url.endswith("/" + filename):
        print(f"FAIL: {key} url does not end with {filename}: {url}", file=sys.stderr)
        failed = True
    if filename not in sums:
        print(f"FAIL: checksums.txt has no entry for {filename}", file=sys.stderr)
        failed = True
        continue
    want_hash = sums[filename].lower()
    if got_hash != want_hash:
        print(
            f"FAIL: hash {key}: manifest {got_hash} != checksums.txt {want_hash}",
            file=sys.stderr,
        )
        failed = True
    else:
        print(f"ok: hash {key} {got_hash} == checksums.txt {filename}")

bin_name = manifest.get("bin")
if bin_name != "gadak.exe":
    print(f"FAIL: bin is {bin_name!r}, want 'gadak.exe' (zip member, no prefix)", file=sys.stderr)
    failed = True
else:
    print("ok: bin gadak.exe")

hash_url = ((manifest.get("autoupdate") or {}).get("hash") or {}).get("url") or ""
if "checksums.txt" not in hash_url:
    print(f"FAIL: autoupdate.hash.url does not name checksums.txt: {hash_url!r}", file=sys.stderr)
    failed = True
else:
    print("ok: autoupdate.hash.url reads checksums.txt")

if failed:
    raise SystemExit(1)
PY

# Scoop checkver.github default regex, against the live latest-release page.
html="${work}/latest.html"
if ! curl -fsSL -A "gadak-scoop-verify" -o "$html" \
  "https://github.com/midagedev/gadak/releases/latest"; then
  echo "verify.sh: download failed: https://github.com/midagedev/gadak/releases/latest" >&2
  exit 1
fi
GADAK_SCOOP_HTML="$html" GADAK_SCOOP_VER="$ver" GADAK_SCOOP_RE="$checkver_regex" \
python3 - <<'PY'
import os
import re
import sys
from pathlib import Path

html = Path(os.environ["GADAK_SCOOP_HTML"]).read_text(errors="replace")
regex = re.compile(os.environ["GADAK_SCOOP_RE"])
match = regex.search(html)
if not match:
    print("FAIL: checkver regex matched nothing on /releases/latest", file=sys.stderr)
    raise SystemExit(1)
got = match.group(1)
want = os.environ["GADAK_SCOOP_VER"]
if got != want:
    print(f"FAIL: checkver regex captured {got}, manifest version is {want}", file=sys.stderr)
    raise SystemExit(1)
print(f"ok: checkver regex {os.environ['GADAK_SCOOP_RE']!s} -> {got}")
PY

echo "ok: ${manifest}"
echo "verify.sh: offline checks passed (not a Windows scoop install)"
