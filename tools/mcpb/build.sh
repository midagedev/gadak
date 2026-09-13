#!/usr/bin/env bash
# Build gadak's Claude Desktop extension (.mcpb) and its MCP Registry
# server.json from a published release tag.
#
#   bash tools/mcpb/build.sh <tag> <outdir>
#
# Outputs:
#   <outdir>/gadak-<ver>.mcpb   the extension (darwin universal + win32 amd64)
#   <outdir>/server.json        the registry document pointing at that file
#
# The inputs are the release's own archives, verified against its
# checksums.txt — nothing is compiled here, so the bundled binaries are the
# signed/notarized ones goreleaser published. macOS only: lipo, codesign and
# sips have no portable stand-ins. See tools/mcpb/README.md.
#
# Every execution of a gadak binary goes through run_gadak, which runs it
# with an empty environment and a fresh HOME. A release binary pointed at a
# real ~/.gadak migrates that workspace's mirror to its own schema, and an
# older installed app can then no longer open it (measured 2026-09-13).
#
# The .mcpb is not byte-reproducible: mcpb pack stamps every ZIP entry with
# the current time (dist/cli/pack.js, `mtime: new Date()` in 2.1.2). Entry
# contents and modes are stable; the archive sha256 is not. server.json always
# carries the sha256 of the file this run produced — upload that file.

set -euo pipefail

MCPB_VERSION=2.1.2
REPO=midagedev/gadak
ASSET_BASE="https://github.com/${REPO}/releases/download"

die() {
  echo "build.sh: $*" >&2
  exit 1
}

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "build.sh: macOS only (needs lipo, codesign and sips); this host is $(uname -s)" >&2
  exit 2
fi
if [[ $# -ne 2 ]]; then
  echo "usage: bash tools/mcpb/build.sh <tag> <outdir>   e.g. v0.22.0 out" >&2
  exit 2
fi

tag=$1
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || die "tag must look like v1.2.3 (got '$tag')"
ver=${tag#v}

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
here="$root/tools/mcpb"
mkdir -p "$2"
outdir="$(cd "$2" && pwd)"

for tool in curl shasum lipo codesign sips unzip python3 npx; do
  command -v "$tool" >/dev/null 2>&1 || die "missing required tool: $tool"
done

work="$(mktemp -d "${TMPDIR:-/tmp}/gadak-mcpb.XXXXXX")"
trap 'rm -rf "$work"' EXIT

# run_gadak <binary> <args…> — the only way this script executes gadak.
# env -i drops GADAK_HOME, GADAK_WORKSPACE, GADAK_PROFILE and everything else;
# HOME and TMPDIR point at a directory created for this one call.
run_gadak() {
  local sandbox
  sandbox="$(mktemp -d "$work/home.XXXXXX")"
  env -i HOME="$sandbox" TMPDIR="$sandbox" PATH=/usr/bin:/bin "$@"
}

# The three JSON-RPC lines a host sends before it knows the tool list.
mcp_requests() {
  printf '%s\n' \
    '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"gadak-mcpb-build","version":"0"}}}' \
    '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
    '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
}

step() { echo "==> $*"; }

# ── 1. download the release inputs and verify them ──────────────────────────
step "1/8 download ${tag} archives"
dl="$work/dl"
mkdir -p "$dl"
assets=("gadak_${ver}_darwin_arm64.tar.gz" "gadak_${ver}_darwin_amd64.tar.gz" "gadak_${ver}_windows_amd64.zip")
for name in checksums.txt "${assets[@]}"; do
  curl -fsSL --retry 3 -o "$dl/$name" "$ASSET_BASE/$tag/$name" || die "download failed: $ASSET_BASE/$tag/$name"
done
: > "$work/checksums.subset"
for name in "${assets[@]}"; do
  awk -v n="$name" '$2 == n' "$dl/checksums.txt" >> "$work/checksums.subset"
done
[[ "$(wc -l < "$work/checksums.subset" | tr -d ' ')" == "3" ]] || die "checksums.txt does not list exactly the three archives"
(cd "$dl" && shasum -a 256 -c "$work/checksums.subset")

# ── 2. universal darwin binary + windows exe ────────────────────────────────
step "2/8 lipo darwin slices, copy windows exe"
bundle="$work/bundle"
mkdir -p "$bundle/server" "$work/arm64" "$work/amd64" "$work/win"
tar -xzf "$dl/gadak_${ver}_darwin_arm64.tar.gz" -C "$work/arm64" gadak
tar -xzf "$dl/gadak_${ver}_darwin_amd64.tar.gz" -C "$work/amd64" gadak
unzip -q -o "$dl/gadak_${ver}_windows_amd64.zip" gadak.exe -d "$work/win"
lipo -create -output "$bundle/server/gadak" "$work/arm64/gadak" "$work/amd64/gadak"
archs="$(lipo -archs "$bundle/server/gadak")"
[[ " $archs " == *" x86_64 "* && " $archs " == *" arm64 "* ]] || die "universal binary archs are '$archs'"
echo "lipo archs: $archs"
cp "$work/win/gadak.exe" "$bundle/server/gadak.exe"
chmod 0755 "$bundle/server/gadak" "$bundle/server/gadak.exe"

# goreleaser signs darwin only when the signing secrets exist, so a fork's
# release is not Developer ID signed. "Signed" here means the signature names
# an Authority: Go's linker ad-hoc signs darwin/arm64 output and leaves amd64
# unsigned, so `codesign -dv` alone succeeds on one fork slice and not the
# other (measured with go build, 2026-09-13).
# Both slices have an Authority → the universal file must verify.
# Neither → say so and continue. One of each → something is wrong.
# No pipe: `codesign … | grep -q` under pipefail is false on a signed file,
# because grep exits at the first match and codesign's next stderr write
# fails (measured 2026-09-13 on the v0.22.0 slices).
has_authority() {
  local info
  info="$(codesign -dvv "$1" 2>&1)" || return 1
  [[ $'\n'"$info" == *$'\n'Authority=* ]]
}
signed=0
has_authority "$work/arm64/gadak" && signed=$((signed + 1))
has_authority "$work/amd64/gadak" && signed=$((signed + 1))
case $signed in
  2) codesign --verify --verbose=2 "$bundle/server/gadak" ;;
  0) echo "codesign: release slices are unsigned or ad-hoc (fork build?) — verify skipped" ;;
  *) die "only one darwin slice carries a signing authority" ;;
esac

# ── 3. icon ─────────────────────────────────────────────────────────────────
step "3/8 icon"
sips -Z 512 "$root/docs/media/logo.png" --out "$bundle/icon.png" >/dev/null

# ── 4. manifest.json, tools taken from the bundled binary ───────────────────
step "4/8 manifest.json (tools/list from the bundled binary)"
mcp_requests | run_gadak "$bundle/server/gadak" --workspace default mcp > "$work/tools-list.jsonl"

cat > "$work/manifest.py" <<'PY'
import json, re, sys

template, responses, version, out = sys.argv[1:5]

def tools_from(path):
    for line in open(path, encoding="utf-8"):
        line = line.strip()
        if not line:
            continue
        msg = json.loads(line)
        if msg.get("id") == 2:
            if "error" in msg:
                sys.exit(f"tools/list returned an error: {msg['error']}")
            return msg["result"]["tools"]
    sys.exit("no tools/list response on stdout")

def first_sentence(text, limit=200):
    text = " ".join(text.split())
    # A sentence ends at . ! ? plus whitespace, whatever case follows
    # (gadak_status's second sentence starts with a lowercase field name).
    m = re.search(r"(?<!e\.g\.)(?<!i\.e\.)(?<=[.!?])\s+", text)
    s = text[: m.start()] if m else text
    if len(s) > limit:
        s = s[: limit - 1].rsplit(" ", 1)[0].rstrip(",;:") + "…"
    return s

tools = tools_from(responses)
if not tools:
    sys.exit("tools/list is empty")
manifest = json.load(open(template, encoding="utf-8"))
manifest["version"] = version
manifest["tools"] = [{"name": t["name"], "description": first_sentence(t.get("description", ""))} for t in tools]
for t in manifest["tools"]:
    assert t["description"] and len(t["description"]) <= 200, t
with open(out, "w", encoding="utf-8") as f:
    json.dump(manifest, f, indent=2, ensure_ascii=False)
    f.write("\n")
print(f"{len(tools)} tools: " + ", ".join(t["name"] for t in manifest["tools"]))
PY
python3 "$work/manifest.py" "$here/manifest.template.json" "$work/tools-list.jsonl" "$ver" "$bundle/manifest.json"

# ── 5. validate + pack ──────────────────────────────────────────────────────
step "5/8 mcpb validate + pack (@anthropic-ai/mcpb@${MCPB_VERSION})"
mcpb="$outdir/gadak-${ver}.mcpb"
rm -f "$mcpb"
npx -y "@anthropic-ai/mcpb@${MCPB_VERSION}" validate "$bundle/manifest.json"
npx -y "@anthropic-ai/mcpb@${MCPB_VERSION}" pack "$bundle" "$mcpb"
[[ -s "$mcpb" ]] || die "mcpb pack produced no file"

# ── 6. the stored mode of the entry point carries owner-exec ────────────────
# Claude Desktop extracts at 0600 and chmods 0700 only when the ZIP-stored mode
# has 0o100; an older build dropped modes entirely (mcpb issue #294).
step "6/8 ZIP entry modes"
cat > "$work/zipcheck.py" <<'PY'
import sys, zipfile

z = zipfile.ZipFile(sys.argv[1])
names = sorted(i.filename for i in z.infolist() if not i.is_dir())
want = sorted(["manifest.json", "icon.png", "server/gadak", "server/gadak.exe"])
if names != want:
    sys.exit(f"unexpected entries: {names}")
e = z.getinfo("server/gadak")
mode = e.external_attr >> 16
print(f"server/gadak stored mode {oct(mode & 0o7777)} create_system {e.create_system}")
if e.create_system != 3:
    sys.exit("server/gadak: create_system is not 3 (unix); modes would be ignored")
if not mode & 0o100:
    sys.exit("server/gadak: stored mode lacks owner-exec (0o100)")
PY
python3 "$work/zipcheck.py" "$mcpb"

# ── 7. install it the way the host does and ask it for tools ────────────────
step "7/8 host emulation (extract 0600, chmod 0700 on owner-exec, substitute, tools/list)"
host="$work/host"
cat > "$work/hostemu.py" <<'PY'
import json, os, sys, zipfile

mcpb, dest, argv_out, names_out = sys.argv[1:5]
os.makedirs(dest)
with zipfile.ZipFile(mcpb) as z:
    for info in z.infolist():
        if info.is_dir():
            continue
        path = os.path.join(dest, info.filename)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with z.open(info) as src, open(path, "wb") as dst:
            dst.write(src.read())
        os.chmod(path, 0o600)
        if (info.external_attr >> 16) & 64:
            os.chmod(path, 0o700)
manifest = json.load(open(os.path.join(dest, "manifest.json"), encoding="utf-8"))
workspace = manifest["user_config"]["workspace"]["default"]
cfg = manifest["server"]["mcp_config"]

def sub(s):
    return s.replace("${__dirname}", dest).replace("${user_config.workspace}", workspace)

argv = [sub(cfg["command"])] + [sub(a) for a in cfg["args"]]
with open(argv_out, "w", encoding="utf-8") as f:
    f.write("\0".join(argv) + "\0")
with open(names_out, "w", encoding="utf-8") as f:
    f.write("\n".join(sorted(t["name"] for t in manifest["tools"])) + "\n")
print("host argv:", " ".join(a.replace(dest, "${__dirname}") for a in argv))
PY
python3 "$work/hostemu.py" "$mcpb" "$host" "$work/host-argv" "$work/manifest-tools.txt"
host_argv=()
while IFS= read -r -d '' arg; do
  host_argv+=("$arg")
done < "$work/host-argv"
mcp_requests | run_gadak "${host_argv[@]}" > "$work/host-tools.jsonl"
cat > "$work/names.py" <<'PY'
import json, sys
for line in open(sys.argv[1], encoding="utf-8"):
    if line.strip():
        msg = json.loads(line)
        if msg.get("id") == 2:
            print("\n".join(sorted(t["name"] for t in msg["result"]["tools"])))
            break
else:
    sys.exit("no tools/list response from the installed extension")
PY
python3 "$work/names.py" "$work/host-tools.jsonl" > "$work/host-tools.txt"
if ! diff -u "$work/manifest-tools.txt" "$work/host-tools.txt"; then
  die "installed extension's tools/list differs from manifest tools"
fi
echo "installed extension answers $(wc -l < "$work/host-tools.txt" | tr -d ' ') tools, same names as the manifest"

# ── 8. server.json ──────────────────────────────────────────────────────────
step "8/8 server.json"
sha="$(shasum -a 256 "$mcpb" | awk '{print $1}')"
size="$(stat -f %z "$mcpb")"
server="$outdir/server.json"
cat > "$work/server.py" <<'PY'
import json, re, sys

template, tag, version, sha, out = sys.argv[1:6]
raw = open(template, encoding="utf-8").read()
for k, v in (("@TAG@", tag), ("@VERSION@", version), ("@SHA256@", sha)):
    raw = raw.replace(k, v)
doc = json.loads(raw)
with open(out, "w", encoding="utf-8") as f:
    json.dump(doc, f, indent=2, ensure_ascii=False)
    f.write("\n")

# Partial check, always run: the constraints the registry enforces on this
# document (schema required fields and lengths, and the mcpb validator in
# modelcontextprotocol/registry internal/validators/registries/mcpb.go).
errs = []
for k in ("name", "description", "version"):
    if not doc.get(k):
        errs.append(f"missing {k}")
if not re.fullmatch(r"[a-zA-Z0-9.-]+/[a-zA-Z0-9._-]+", doc.get("name", "")):
    errs.append("name does not match the registry pattern")
for k in ("description", "title"):
    if k in doc and not 1 <= len(doc[k]) <= 100:
        errs.append(f"{k} is {len(doc[k])} chars (1..100)")
pkgs = doc.get("packages") or []
if len(pkgs) != 1:
    errs.append("expected exactly one package")
for p in pkgs:
    if p.get("registryType") != "mcpb":
        errs.append("registryType is not mcpb")
    if "registryBaseUrl" in p:
        errs.append("mcpb packages must not carry registryBaseUrl")
    if not re.fullmatch(r"[a-f0-9]{64}", p.get("fileSha256", "")):
        errs.append("fileSha256 is not 64 lowercase hex")
    ident = p.get("identifier", "")
    if not re.fullmatch(r"https://github\.com/([a-zA-Z0-9]([a-zA-Z0-9\-]{0,37}[a-zA-Z0-9])?)/([a-zA-Z0-9._\-]+)/releases/download/([^/]+)/([^/]+)", ident):
        errs.append(f"identifier is not a GitHub release asset URL: {ident}")
    if "mcp" not in ident.lower():
        errs.append("identifier does not contain 'mcp'")
    if (p.get("transport") or {}).get("type") != "stdio":
        errs.append("transport.type is not stdio")
if doc.get("version") != version:
    errs.append("version mismatch")
if errs:
    sys.exit("server.json: " + "; ".join(errs))
print("server.json registry constraints: ok (partial check)")
print(doc["$schema"])
PY
schema_url="$(python3 "$work/server.py" "$here/server.template.json" "$tag" "$ver" "$sha" "$server" | tee /dev/stderr | tail -n 1)"

cat > "$work/schema.py" <<'PY'
import json, sys
try:
    import jsonschema
except ImportError:
    print("server.json full schema validation: NOT RUN (python jsonschema not importable)")
    sys.exit(0)
schema = json.load(open(sys.argv[1], encoding="utf-8"))
doc = json.load(open(sys.argv[2], encoding="utf-8"))
jsonschema.validate(doc, schema)
from importlib.metadata import version
print(f"server.json full schema validation: ok (jsonschema {version('jsonschema')})")
PY
curl -fsSL --retry 3 -o "$work/server.schema.json" "$schema_url" || die "could not fetch $schema_url"
python3 "$work/schema.py" "$work/server.schema.json" "$server"

echo
echo "mcpb:        $mcpb"
echo "size:        $size bytes ($(awk -v b="$size" 'BEGIN { printf "%.1f MiB", b / 1048576 }'))"
echo "sha256:      $sha"
echo "server.json: $server"
