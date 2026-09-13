# tools/mcpb

Builds gadak's Claude Desktop extension and its MCP Registry entry from a
published release tag.

```bash
bash tools/mcpb/build.sh v0.22.0 out
```

Outputs:

- `out/gadak-<ver>.mcpb`: the extension. A ZIP holding `manifest.json`,
  `icon.png`, `server/gadak` (darwin) and `server/gadak.exe` (win32).
- `out/server.json`: the [MCP Registry](https://github.com/modelcontextprotocol/registry)
  document (`io.github.midagedev/gadak`, `registryType: "mcpb"`). It points at
  the `.mcpb` as a release asset and carries its sha256.

The release workflow's `mcpb` job runs the same script after goreleaser,
attaches the `.mcpb` to the release, and publishes `server.json` only when the
repository variable `MCP_REGISTRY_PUBLISH` is `true`.

## What the script does

1. Downloads the darwin arm64/amd64 archives, the windows amd64 archive and
   `checksums.txt` from the release, and checks the three archives against it.
   Nothing is compiled, so the bundle carries the binaries goreleaser signed.
2. Joins the two darwin slices with `lipo -create` and runs `codesign --verify`
   on the result. When neither slice carries a signing authority (a fork's
   release is unsigned or only ad-hoc signed), the check is skipped with a
   message.
3. Resizes `docs/media/logo.png` to 512 px.
4. Fills `manifest.template.json`. `version` comes from the tag; `tools` comes
   from `tools/list` on the bundled binary (name plus first sentence, at most
   200 characters).
5. `mcpb validate` and `mcpb pack`, pinned to `@anthropic-ai/mcpb@2.1.2`.
6. Asserts the ZIP-stored mode of `server/gadak` has owner-exec.
7. Installs the file the way the host does and asks it for `tools/list`. The
   names must match the manifest.
8. Writes `server.json`, checks the registry's constraints, and validates it
   against the published schema when python `jsonschema` is importable.

macOS only: `lipo`, `codesign` and `sips` have no portable equivalents.

## Why a universal binary

An MCPB manifest overrides `mcp_config` per platform (`darwin`, `win32`,
`linux`), not per architecture. One `darwin` entry has to run on both Apple
silicon and Intel, so the two slices go into one file. Windows gets the amd64
exe, which arm64 Windows runs under emulation. Linux is left out because Claude
Desktop has no Linux build.

## Why every gadak run gets a fresh HOME

A release binary opened against a real workspace migrates that mirror to its
own schema, and an older installed app can no longer read it afterwards.
`run_gadak` in `build.sh` is the only place the script executes a gadak binary.
It uses `env -i` with `HOME` and `TMPDIR` set to a new temporary directory, so
`~/.gadak`, `GADAK_HOME` and `GADAK_WORKSPACE` never reach the process.

## The exec-bit assertion

`mcpb pack` stores the `entry_point` with the exec bits set. Claude Desktop
extracts every file at 0600 and makes it 0700 only when the stored mode has
owner-exec (0o100). An older Claude Desktop dropped stored modes entirely
([mcpb#294](https://github.com/modelcontextprotocol/mcpb/issues/294)), and the
extension then could not start. Step 6 fails the build if that bit is missing,
and step 7 repeats the host's extraction and runs the result.

## Reproducibility

Rebuilding the same tag gives the same entries (contents and modes) but a
different archive sha256. `mcpb pack` stamps each ZIP entry with the current
time. `server.json` always carries the sha256 of the file that run produced,
so upload that exact file. Don't rerun the job after the registry has the
version: `--clobber` would replace the asset with one whose sha256 no longer
matches the registry entry.
