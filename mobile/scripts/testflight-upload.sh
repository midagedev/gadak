#!/usr/bin/env bash
#
# gadak mobile → TestFlight, in one command.
#
#   scripts/testflight-upload.sh --bump          # next build number, then ship
#   scripts/testflight-upload.sh                 # ship the build number in tauri.conf.json
#   scripts/testflight-upload.sh --dry-run       # build + verify + validate, no upload
#   scripts/testflight-upload.sh --status        # what does App Store Connect hold?
#
# In order: fast gates -> build number -> `tauri ios build` (release, device,
# app-store-connect export) -> **verify the .ipa against the submission
# contract** -> `altool --validate-app` -> `altool --upload-app` -> poll App
# Store Connect until the build stops processing -> write a record under
# artifacts/app-store/.
#
# The verification step is the point: every item is one Apple would only tell
# you about after a full upload, or would not tell you about at all.
#
# Ported from ~/repo/naru-remote/scripts/testflight-upload.sh (same Apple
# account, same API-key credentials). What changed for gadak: the archive is
# produced by `tauri ios build` rather than xcodebuild, so the contract is
# checked against the exported .ipa instead of an .xcarchive, and the
# must-not-ship check looks for the DEV capture tour instead of Naru's test
# hooks.
#
# Credentials (never in the repo, never echoed):
#
#   ~/.appstoreconnect/credentials.env          ASC_KEY_ID, ASC_ISSUER_ID (0600)
#   ~/.appstoreconnect/private_keys/AuthKey_$ASC_KEY_ID.p8
#
# Override with ASC_CREDENTIALS_FILE / ASC_PRIVATE_KEY on another machine.
#
# Runbook, including the account-owner web steps this script cannot do:
# docs/runbooks/testflight-release.md

set -euo pipefail

BUNDLE_ID="dev.gadak.mobile"

bump_build=0
dry_run=0
allow_dirty=0
run_gates=1
status_only=0

while [ $# -gt 0 ]; do
    case "$1" in
        --bump) bump_build=1 ;;
        --dry-run) dry_run=1 ;;
        --allow-dirty) allow_dirty=1 ;;
        --no-gates) run_gates=0 ;;
        --status) status_only=1 ;;
        -h|--help) sed -n '2,33p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "unknown option: $1" >&2; exit 64 ;;
    esac
    shift
done

mobile_root="$(cd "$(dirname "$0")/.." && pwd)"
repo_root="$(cd "$mobile_root/.." && pwd)"
conf="$mobile_root/src-tauri/tauri.conf.json"
cd "$mobile_root"

# Every log this script writes goes here, outside the repo. Not tidiness: an
# xcodebuild log is hundreds of lines of absolute home paths, and a copy inside
# the tree is picked up by scripts/scan-internal.sh — the CI gate that exists to
# keep exactly those strings out of a public repo (2026-08-26: it went red on
# mobile/.testflight-build.log). Keeping the logs out of the tree removes the
# hit at its source instead of teaching the scanner to look away.
stamp="$(date +%Y%m%d-%H%M%S)"
work_dir="${TMPDIR:-/tmp/}gadak-testflight-$stamp"
mkdir -p "$work_dir"

log() { printf '\033[1m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[31mFAIL\033[0m %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------- credentials

credentials_file="${ASC_CREDENTIALS_FILE:-$HOME/.appstoreconnect/credentials.env}"
[ -f "$credentials_file" ] || fail "no credentials at $credentials_file (see docs/runbooks/testflight-release.md)"

# shellcheck disable=SC1090
set +u
. "$credentials_file"
set -u

[ -n "${ASC_KEY_ID:-}" ] || fail "ASC_KEY_ID missing from $credentials_file"
[ -n "${ASC_ISSUER_ID:-}" ] || fail "ASC_ISSUER_ID missing from $credentials_file"

private_key="${ASC_PRIVATE_KEY:-$HOME/.appstoreconnect/private_keys/AuthKey_$ASC_KEY_ID.p8}"
[ -f "$private_key" ] || fail "no API private key at the expected path (AuthKey_<key id>.p8)"

# `altool` takes the key id and issuer as arguments, so they are visible in
# this machine's process list while an upload runs. That is Apple's interface,
# not a choice — but nothing here writes them to a file, a log or the record.
asc_api() { # asc_api <path-with-query>
    local token
    token="$(python3 "$mobile_root/scripts/asc-jwt.py" "$private_key" "$ASC_KEY_ID" "$ASC_ISSUER_ID")"
    # `--globoff`: App Store Connect filters are `filter[app]=...`, and curl
    # would otherwise read those brackets as a glob range and refuse the URL.
    curl --silent --show-error --fail-with-body --globoff \
        -H "Authorization: Bearer $token" \
        "https://api.appstoreconnect.apple.com/v1/$1"
}

json_value() { # json_value <python-expression-on-`d`>
    python3 -c 'import json,sys; d=json.load(sys.stdin); print(eval(sys.argv[1]))' "$1"
}

# The version and the build number live in tauri.conf.json, so both stay
# reviewable in git instead of being rewritten at export time.
conf_get() { # conf_get <python-expression-on-`c`>
    python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(eval(sys.argv[2]))' "$conf" "$1"
}

marketing_version="$(conf_get 'c["version"]')"
build_number="$(conf_get 'c["bundle"]["iOS"]["bundleVersion"]')"
[ -n "$marketing_version" ] && [ -n "$build_number" ] || fail "could not read version/bundleVersion out of tauri.conf.json"

# The signing team has exactly one owner: the tauri config, which is what
# actually signs the build. Reading it here rather than repeating the value
# means the archive check below cannot drift away from what was signed.
TEAM_ID="$(conf_get 'c["bundle"]["iOS"]["developmentTeam"]')"
[ -n "$TEAM_ID" ] || fail "bundle.iOS.developmentTeam is not set in tauri.conf.json"

# --------------------------------------------------------------- status only

resolve_app_id() {
    asc_api "apps?filter[bundleId]=$BUNDLE_ID&fields[apps]=bundleId" \
        | json_value 'd["data"][0]["id"] if d["data"] else ""'
}

if [ "$status_only" -eq 1 ]; then
    app="$(resolve_app_id)"
    if [ -z "$app" ]; then
        echo "no App Store Connect app record for $BUNDLE_ID yet."
        echo "Creating one is a web step — see docs/runbooks/testflight-release.md."
        exit 1
    fi
    asc_api "builds?filter[app]=$app&filter[preReleaseVersion.version]=$marketing_version&limit=5&fields[builds]=version,processingState,uploadedDate,expired" \
        | python3 -c '
import json, sys
data = json.load(sys.stdin)["data"]
if not data:
    print("no builds uploaded for this version yet")
for b in data:
    a = b["attributes"]
    print("build %s: %s  uploaded %s  expired=%s" % (
        a["version"], a["processingState"], a.get("uploadedDate"), a.get("expired")))
'
    echo "(tauri.conf.json currently points at build $build_number of $marketing_version)"
    exit 0
fi

# ----------------------------------------------------------------- work tree

if [ "$allow_dirty" -eq 0 ] && [ -n "$(git -C "$repo_root" status --porcelain --untracked-files=no)" ]; then
    fail "work tree is dirty — a TestFlight build should be reproducible from a commit (--allow-dirty to override)"
fi

command -v xcodegen >/dev/null || fail "xcodegen is required (tauri regenerates the Xcode project with it)"

# ------------------------------------------------------------ rust toolchain
#
# The Xcode run phase shells out to whatever `cargo` is first on PATH. On a
# machine carrying both Homebrew's standalone `rust` formula and rustup,
# /opt/homebrew/bin/cargo wins — and that toolchain ships only the host std.
# The build then dies a few hundred lines deep in cargo with "can't find crate
# for `std`", naming a target that `rustup target list --installed` swears is
# present (2026-08-26: it was — for the rustup toolchain the build never
# reached). So resolve the toolchain here and put it in front instead of
# trusting PATH, and let the Xcode script inherit that.
if command -v rustup >/dev/null 2>&1; then
    toolchain_cargo="$(rustup which cargo 2>/dev/null || true)"
    if [ -n "$toolchain_cargo" ] && [ -x "$toolchain_cargo" ]; then
        PATH="$(dirname "$toolchain_cargo"):$PATH"
        export PATH
    fi
fi

# Ask the rustc that will actually run — not rustup, which answers for its own
# toolchain regardless of who is first on PATH. `--print target-libdir` prints
# a path whether or not anything is there, so the existence check is the gate.
ios_libdir="$(rustc --print target-libdir --target aarch64-apple-ios 2>/dev/null || true)"
if [ -z "$ios_libdir" ] || [ ! -d "$ios_libdir" ]; then
    printf '\033[31mFAIL\033[0m %s\n' "the rust toolchain on PATH has no std for aarch64-apple-ios" >&2
    printf '  cargo:  %s\n' "$(command -v cargo || echo '<none>')" >&2
    printf '  rustc:  %s\n' "$(command -v rustc || echo '<none>')" >&2
    printf '  looked: %s\n' "${ios_libdir:-<rustc could not answer>}" >&2
    printf '\nInstall the target for THAT toolchain (rustup target add aarch64-apple-ios),\n' >&2
    printf 'or put rustup ahead of a standalone install on PATH. Beware:\n' >&2
    printf '`rustup target list --installed` answers for rustup even when a different\n' >&2
    printf 'cargo is first on PATH — that mismatch is exactly this failure.\n' >&2
    exit 1
fi
echo "  ok   rust std for aarch64-apple-ios: $ios_libdir"

if [ "$run_gates" -eq 1 ]; then
    log "Fast gates (svelte-check, vitest, ios-contract)"
    npm run check > "$work_dir/gates.log" 2>&1 \
        || { tail -40 "$work_dir/gates.log"; fail "svelte-check failed — see $work_dir/gates.log"; }
    npm test >> "$work_dir/gates.log" 2>&1 \
        || { tail -40 "$work_dir/gates.log"; fail "vitest failed — see $work_dir/gates.log"; }
    npm run lint:ios >> "$work_dir/gates.log" 2>&1 \
        || { tail -40 "$work_dir/gates.log"; fail "ios-contract failed — see $work_dir/gates.log"; }
        echo "  ok   gates"
fi

# -------------------------------------------------------------- build number

if [ "$bump_build" -eq 1 ]; then
    build_number=$((build_number + 1))
    log "Build number -> $build_number"
    python3 - "$conf" "$build_number" <<'PY'
import json, re, sys
path, build = sys.argv[1], sys.argv[2]
s = open(path).read()
s2, n = re.subn(r'("bundleVersion"\s*:\s*)"[^"]*"', r'\g<1>"%s"' % build, s, count=1)
if n != 1:
    raise SystemExit("could not find bundle.iOS.bundleVersion in %s" % path)
open(path, "w").write(s2)
assert json.load(open(path))["bundle"]["iOS"]["bundleVersion"] == build
PY
    [ "$(conf_get 'c["bundle"]["iOS"]["bundleVersion"]')" = "$build_number" ] || fail "the build-number edit did not take"
fi

log "Shipping $marketing_version (build $build_number)"

# App Store Connect refuses a duplicate build number for a version, and it does
# so *after* a full build and upload. Ask first.
app_id="$(resolve_app_id)"
[ -n "$app_id" ] || fail "no App Store Connect app record for $BUNDLE_ID — that is a web step, see docs/runbooks/testflight-release.md"
existing="$(asc_api "builds?filter[app]=$app_id&filter[preReleaseVersion.version]=$marketing_version&filter[version]=$build_number&fields[builds]=version,processingState" \
    | json_value '",".join(b["attributes"]["processingState"] for b in d["data"])')"
if [ -n "$existing" ]; then
    fail "build $build_number of $marketing_version already exists upstream (state: $existing) — run with --bump"
fi

# --------------------------------------------------------------------- build

log "tauri ios build (release, device, app-store-connect export) — takes a few minutes"
build_log="$work_dir/build.log"
./node_modules/.bin/tauri ios build \
    --target aarch64 \
    --export-method app-store-connect \
    --ci \
    > "$build_log" 2>&1 \
    || { tail -60 "$build_log"; fail "tauri ios build failed — full log at $build_log"; }

ipa_path="$(/usr/bin/find "$mobile_root/src-tauri/gen/apple/build" -name '*.ipa' -newer "$conf" 2>/dev/null | head -1)"
[ -n "$ipa_path" ] || ipa_path="$(/usr/bin/find "$mobile_root/src-tauri/gen/apple/build" -name '*.ipa' 2>/dev/null | head -1)"
[ -n "$ipa_path" ] || { tail -40 "$build_log"; fail "the build produced no .ipa — full log at $build_log"; }
log "Built $(basename "$ipa_path") ($(du -h "$ipa_path" | cut -f1))"

# ---------------------------------------------- verify the .ipa contract

mkdir -p "$work_dir/unzipped"
/usr/bin/unzip -q "$ipa_path" -d "$work_dir/unzipped"

app_bundle="$(/usr/bin/find "$work_dir/unzipped/Payload" -maxdepth 1 -name '*.app' | head -1)"
[ -n "$app_bundle" ] || fail "the .ipa has no app bundle where one is expected"
info_plist="$app_bundle/Info.plist"
[ -f "$info_plist" ] || fail "the app bundle has no Info.plist"

plist_get() { /usr/libexec/PlistBuddy -c "Print :$1" "$info_plist" 2>/dev/null || true; }

verify_failed=0
expect() { # expect <label> <actual> <wanted>
    if [ "$2" = "$3" ]; then
        printf '  ok   %-30s %s\n' "$1" "$2"
    else
        printf '  FAIL %-30s %s (expected %s)\n' "$1" "$2" "$3"
        verify_failed=1
    fi
}

log "Verifying the .ipa against the submission contract"
expect "CFBundleShortVersionString" "$(plist_get CFBundleShortVersionString)" "$marketing_version"
expect "CFBundleVersion" "$(plist_get CFBundleVersion)" "$build_number"
expect "CFBundleIdentifier" "$(plist_get CFBundleIdentifier)" "$BUNDLE_ID"
# Without this key every upload parks in "Missing Compliance" and never
# reaches a tester until someone answers the export question by hand.
expect "NonExemptEncryption=false" "$(plist_get ITSAppUsesNonExemptEncryption)" "false"
# The camera string is required before the pairing scanner may run; iOS kills
# the app on the first scan without it, and Apple rejects the binary.
if [ -n "$(plist_get NSCameraUsageDescription)" ]; then
    printf '  ok   %-30s present\n' "NSCameraUsageDescription"
else
    printf '  FAIL %-30s missing (the QR pairing scanner needs it)\n' "NSCameraUsageDescription"
    verify_failed=1
fi

signing_team="$(security cms -D -i "$app_bundle/embedded.mobileprovision" 2>/dev/null \
    | /usr/bin/plutil -extract TeamIdentifier.0 raw - 2>/dev/null || true)"
expect "signing team" "${signing_team:-<none>}" "$TEAM_ID"

# TestFlight rejects a build whose bundle carries no app icon, and it does so
# after the upload. Assets.car is opaque, so assert the manifest key xcodegen
# writes when the appiconset is compiled in.
if [ -n "$(plist_get 'CFBundleIcons:CFBundlePrimaryIcon:CFBundleIconName')" ] || [ -f "$app_bundle/Assets.car" ]; then
    printf '  ok   %-30s compiled into the bundle\n' "app icon"
else
    printf '  FAIL %-30s no Assets.car and no CFBundleIconName\n' "app icon"
    verify_failed=1
fi

# The self-driving capture tour (src/lib/demo-tour.ts) is DEV-only: it drives
# the store and the DOM with no user input. Vite is expected to drop it from a
# production bundle because the dynamic import sits behind `import.meta.env.DEV`
# — expected is not verified, so this greps the shipped JS for its arming
# token. One leaking into a tester's build would let the app drive itself.
if /usr/bin/grep -rql "demo-tour" "$app_bundle" 2>/dev/null; then
    printf '  FAIL %-30s the DEV capture tour reached the shipped bundle\n' "demo-tour"
    verify_failed=1
else
    printf '  ok   %-30s absent from the shipped bundle\n' "demo-tour"
fi

[ "$verify_failed" -eq 0 ] || fail "the .ipa does not satisfy the submission contract (nothing was uploaded)"

# ------------------------------------------------------ validate and upload

log "altool --validate-app"
xcrun altool --validate-app -f "$ipa_path" -t ios \
    --apiKey "$ASC_KEY_ID" --apiIssuer "$ASC_ISSUER_ID" \
    > "$work_dir/validate.log" 2>&1 \
    || { tail -30 "$work_dir/validate.log"; fail "validation failed — full log at $work_dir/validate.log"; }
grep -q "VERIFY SUCCEEDED\|No errors validating" "$work_dir/validate.log" \
    || { tail -30 "$work_dir/validate.log"; fail "validation did not report success"; }
echo "  VERIFY SUCCEEDED"

if [ "$dry_run" -eq 1 ]; then
    log "--dry-run: stopping before upload. Artifacts in $work_dir"
    exit 0
fi

log "altool --upload-app"
xcrun altool --upload-app -f "$ipa_path" -t ios \
    --apiKey "$ASC_KEY_ID" --apiIssuer "$ASC_ISSUER_ID" \
    > "$work_dir/upload.log" 2>&1 \
    || { tail -30 "$work_dir/upload.log"; fail "upload failed — full log at $work_dir/upload.log"; }
grep -q "UPLOAD SUCCEEDED\|No errors uploading" "$work_dir/upload.log" \
    || { tail -30 "$work_dir/upload.log"; fail "upload did not report success"; }
echo "  UPLOAD SUCCEEDED"

# ------------------------------------------------------------ wait for Apple

log "Waiting for Apple to finish processing build $build_number"
processing_state="UNKNOWN"
deadline=$(( $(date +%s) + 1800 ))
while [ "$(date +%s)" -lt "$deadline" ]; do
    sleep 30
    state="$(asc_api "builds?filter[app]=$app_id&filter[preReleaseVersion.version]=$marketing_version&filter[version]=$build_number&fields[builds]=version,processingState" \
        | json_value 'd["data"][0]["attributes"]["processingState"] if d["data"] else ""' 2>/dev/null || true)"
    [ -n "$state" ] || continue
    if [ "$state" != "$processing_state" ]; then
        processing_state="$state"
        echo "  processingState=$processing_state"
    fi
    case "$processing_state" in
        VALID|FAILED|INVALID) break ;;
    esac
done

# ------------------------------------------------------------------- record

record_dir="$repo_root/artifacts/app-store/$(date +%Y%m%d)-build$build_number"
mkdir -p "$record_dir"
{
    echo "# TestFlight upload — gadak mobile $marketing_version (build $build_number)"
    echo
    echo "- Uploaded: $(date '+%Y-%m-%d %H:%M %Z')"
    echo "- Commit: $(git -C "$repo_root" rev-parse --short HEAD)$([ -n "$(git -C "$repo_root" status --porcelain --untracked-files=no)" ] && echo ' (dirty tree)')"
    echo "- Bundle: $BUNDLE_ID, team $TEAM_ID"
    echo "- .ipa contract: version/build, bundle id, signing team,"
    echo "  ITSAppUsesNonExemptEncryption=false, NSCameraUsageDescription present,"
    echo "  app icon compiled in, no demo-tour in the shipped bundle — all verified"
    echo "  pre-upload."
    echo "- altool: VERIFY SUCCEEDED, UPLOAD SUCCEEDED."
    echo "- App Store Connect processingState: $processing_state"
    echo
    echo "Produced by \`mobile/scripts/testflight-upload.sh\`. Credentials were read"
    echo "from ~/.appstoreconnect and are not recorded here."
} > "$record_dir/upload.md"

log "Done. processingState=$processing_state"
log "Record: ${record_dir#$repo_root/}/upload.md"

case "$processing_state" in
    VALID)
        cat <<'EOF'

The build is processed and available in TestFlight. What this script cannot do
(account-owner web steps): assign it to the internal tester group, and answer
any first-time export-compliance prompt. See docs/runbooks/testflight-release.md.
EOF
        ;;
    FAILED|INVALID)
        fail "Apple rejected the build (processingState=$processing_state) — check App Store Connect for the reason"
        ;;
    *)
        echo
        echo "Still processing after 30 minutes; that is normal for a first upload."
        echo "Re-check with: scripts/testflight-upload.sh --status"
        ;;
esac
