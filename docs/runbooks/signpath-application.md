# SignPath Foundation application (GDK-211)

Lead-only runbook. This file is the draft and the checklist. **Do not submit
the form, comment on GDK-211, or edit `.github/workflows/` from a delegated
round** — those writes are the lead’s.

User-facing status (what to tell someone who hit SmartScreen) lives in
[`docs/WINDOWS-SIGNING.md`](../WINDOWS-SIGNING.md). Requirements below are
from SignPath Foundation’s own pages, fetched 2026-08-23.

## Official sources

| What | URL |
| --- | --- |
| Program home | https://signpath.org/ |
| Conditions (OSS subscription + Foundation certificate) | https://signpath.org/terms.html |
| Apply form | https://signpath.org/apply.html |
| Listed OSS projects | https://signpath.org/projects |
| Origin verification (required for OSS code signing) | https://docs.signpath.io/origin-verification/ |
| Project / signing-policy settings (OSS edition flags) | https://docs.signpath.io/projects |
| GitHub as trusted build system | https://docs.signpath.io/trusted-build-systems/github |
| Submit-signing-request Action | https://github.com/SignPath/github-action-submit-signing-request |
| PE metadata restrictions | https://docs.signpath.io/artifact-configuration/reference#metadata-restrictions |

The apply page is a form (little public HTML). Field names below follow the
public conditions page plus a 2026-02 write-up of an actual submission
(https://zenn.dev/shm_7ec/articles/signpath-oss-code-signing) — **treat the
zenn field list as unverified against today’s form**; paste into whatever
the form currently asks, and do not invent extra claims.

## Requirements vs gadak (gap table)

From https://signpath.org/terms.html unless noted.

### Free OSS SignPath.io subscription

| Requirement | Status | Evidence |
| --- | --- | --- |
| No malware / PUP | Met as far as this tree can show | Product is a local SQLite mirror; no installer adware. SignPath will judge. |
| OSI-approved license, no commercial dual-license | **Met** | `LICENSE` is Apache License 2.0. GitHub API `license.spdx_id` = `Apache-2.0`. OSI: https://opensource.org/licenses/Apache-2.0. No second license in-tree. |
| No proprietary components (system libraries allowed) | **Met** | Application code is this repo + OSS Go modules (`go.mod`, `desktop/go.mod`). Windows UI uses the Evergreen WebView2 **runtime** (not bundled; system library). wails v3 is MIT. |
| Actively maintained | **Met** | Public repo, tags through v0.16.1 (2026-08-20), `pushed_at` 2026-08-23 (GitHub API). |
| Already released in the form that should be signed | **Met** | Windows portable zip ships from 0.16 (`Gadak-<ver>-windows-x64.zip` / `arm64`). CLI zip has shipped on every goreleaser tag. |
| Functionality described on the download page | **Met** | README + GitHub Releases + [INSTALL.md](../INSTALL.md). |

### Extra conditions for a *SignPath Foundation* certificate

| Requirement | Status | Evidence |
| --- | --- | --- |
| Sign own projects only (same team owns repo + signing) | **Met** | Owner is GitHub user `midagedev` (type `User`). No separate signing vendor. |
| Sign own binaries only | **Met** | Release PE files are built from this repo. WebView2 is not signed by us (system). |
| No hacking / exploit / security-circumvention tools | **Met** | Not that class of software. |
| Respect user privacy; announce system changes; provide uninstall | **Met, with a wording caveat** | Portable zip (delete the folder). `install-service` is refused on Windows (`cmd/gadak/service.go`). Network policy is [SECURITY.md](../../SECURITY.md) — do **not** paste SignPath’s canned “no transfer unless requested” line as if the default version check did not exist. |
| MFA on GitHub and SignPath | **Unknown (lead)** | Cannot be verified from the public repo. Confirm GitHub 2FA on `midagedev` before submit. |
| Roles: Authors / Reviewers / Approvers named | **Met on paper** | Single maintainer; [WINDOWS-SIGNING.md](../WINDOWS-SIGNING.md) names `midagedev` for all three. Outside PRs are reviewed by that user. Direct pushes by the author are the “Authors” case SignPath describes. |
| Public **Code signing policy** page | **Met by this round’s doc** | [WINDOWS-SIGNING.md](../WINDOWS-SIGNING.md) uses that heading. Attribution sentence is marked *planned*, not current. README Windows install sections must keep linking here. |
| Artifact metadata restrictions (product name + version on every signed PE) | **Not met** | Repo grep: no `VERSIONINFO` / `winres` / `.syso`. Go ldflags set `main.version` (CLI) / `appVersion` (desktop) as strings, not PE resources. **Must land before the first signed tag**, not in the application form. |
| Trusted build system = GitHub.com; origin verification | **Met for runners; integration not wired** | Both release jobs use GitHub-hosted runners (`ubuntu-latest`, `windows-latest`, `macos-14`). OSS condition: *all jobs leading up to the signing request on GitHub-hosted agents* (https://docs.signpath.io/trusted-build-systems/github). No SignPath Action in workflows yet (this round does not add one). |
| Binaries built from source in a verifiable way; every release manually approved | **Met for “CI from this repo”; not bit-reproducible** | Workflows and scripts are in the repo. SignPath’s “reproducability” check is CI-config-in-git / no manual job overrides / GitHub-hosted agents — **not** Debian-style bit-identical rebuilds. Manual approval is a SignPath policy setting after the grant. |
| Executable programs need “verifiable reputation” (soft; SignPath’s own “common misunderstandings”) | **Unknown / risk** | Repo created 2026-08-04; 19 stars, 2 forks (GitHub API 2026-08-23). Foundation says they will not sign “binaries based on source code that nobody knows”. Application can still be refused with no arbitration. |

## Similar accepted projects

Listed at https://signpath.org/projects (fetched 2026-08-23):

- **MCPProxy** — https://signpath.org/projects/mcpproxy — Go desktop + CLI for AI agents (`github.com/smart-mcp-proxy/mcpproxy-go`, MIT). Closest product shape (single binary, embedded UI, Windows zip/installer).
- **dnsmonster** — https://signpath.org/projects/dnsmonster — Go CLI toolkit (`github.com/mosajjal/dnsmonster`, GPL-3.0).

Use them as “the program already signs Go Windows binaries”, not as a promise gadak will be accepted.

## Alternatives (lead decision)

| Path | Cost (cited) | Identity / geography | What the user sees | Fit for gadak 0.x |
| --- | --- | --- | --- | --- |
| **SignPath Foundation OSS** | Free for OSS (https://signpath.org/) | No personal CA identity. Publisher on the cert is “SignPath Foundation”. | Authenticode; reputation still accrues over downloads | **Recommended first try.** Matches the OSS constraint, GitHub Actions, and “no $700/year USB token”. |
| **Stay unsigned + this page** | $0 | n/a | SmartScreen override; Smart App Control has no per-app bypass (Microsoft FAQ above) | **Current state.** Honest. CLI zip + `checksums.txt` remains the reliable Windows route. |
| **Azure Artifact Signing** (rebrand of Trusted Signing) | Basic **$9.99/month** / 5,000 signatures; Premium $99.99/month (https://azure.microsoft.com/en-us/products/artifact-signing/). Paid Azure subscription required (https://learn.microsoft.com/en-us/azure/artifact-signing/faq). | **Public Trust, individual:** United States or Canada only. **Public Trust, organization:** US, Canada, EU, UK, Australia, New Zealand, Japan, **South Korea**, Singapore, Switzerland, Norway, Israel (https://learn.microsoft.com/en-us/azure/artifact-signing/quickstart, dated 2026-05-21). Private Trust: no geo restriction, **not** SmartScreen-trusted. | Publisher is the validated individual or legal entity | Fallback **if** SignPath refuses **and** a matching identity path exists. `midagedev` is a GitHub User, not an org; individual Public Trust is US/CA. Korea is on the **organization** list only. |
| **Commercial CA (e.g. DigiCert)** | DigiCert list price 2026-08-21: Code Signing own-token / HSM **$44/month** (12-month **$696**); USB-token **$54/month** (**$840**); KeyLocker **$65/month** (**$996**) — https://www.digicert.com/signing/compare-code-signing-certificates | Organization or individual vetting; USB token or HSM | Immediate publisher name is yours | Overkill for a one-person 0.x OSS. Spec background “$200–400/year” is **stale** against this list. |

**Recommendation:** submit SignPath. Keep WINDOWS-SIGNING.md as the SmartScreen answer in the meantime. Do not buy a CA cert. Do not open an Azure Artifact Signing account unless SignPath is refused and the identity-validation geography is actually satisfiable.

## Application form — draft answers (English)

Replace the contact email. Do not paste secrets.

**Project / repository URL**

```
https://github.com/midagedev/gadak
```

**License**

```
Apache License 2.0 (OSI-approved). LICENSE at the repository root.
https://github.com/midagedev/gadak/blob/main/LICENSE
```

**Download / release URL**

```
https://github.com/midagedev/gadak/releases
Latest: https://github.com/midagedev/gadak/releases/latest
Windows files: gadak_<version>_windows_amd64.zip, gadak_<version>_windows_arm64.zip
(CLI), and Gadak-<version>-windows-x64.zip, Gadak-<version>-windows-arm64.zip
(portable desktop directory; not an installer).
```

**Project description** (pasteable)

```
gadak is a local-first issue tracker for people and coding agents. It mirrors
Jira, Confluence, and optionally Linear into one SQLite file on the user's
machine (or runs gadak's own in-process tracker with no Atlassian account).
Reads never use the network. Writes go through the origin the user configured.
Surfaces: a desktop window (Windows portable zip, macOS signed+notarized dmg),
a web UI served from the same binary, a CLI, and MCP.

We want Authenticode signatures on the Windows PE files we already publish
from GitHub Releases (gadak.exe, gadak-desktop.exe). Builds are GitHub Actions
on GitHub-hosted runners: GoReleaser on ubuntu-latest for the CLI zip, and
desktop/build-windows.ps1 on windows-latest for the portable desktop zip.
There is no MSI. The private key would stay on SignPath's HSM.

Homepage: https://github.com/midagedev/gadak
Code signing policy (unsigned today; planned SignPath attribution is marked
as planned): https://github.com/midagedev/gadak/blob/main/docs/WINDOWS-SIGNING.md
```

**Contact**

```
Maintainer GitHub: https://github.com/midagedev
Email: <lead fills in a mailbox that receives external mail>
```

If the form asks whether binaries are already released: **yes**, from v0.16.0
(first desktop zip) and on every later tag.

If it asks for a build-pipeline URL:

```
CLI: https://github.com/midagedev/gadak/blob/main/.github/workflows/release.yml
Desktop: https://github.com/midagedev/gadak/blob/main/.github/workflows/desktop-release.yml
(job desktop-windows)
```

## After a grant — CI outline (do not implement in this round)

SignPath’s GitHub path (https://docs.signpath.io/trusted-build-systems/github):

1. Install the [SignPath GitHub App](https://github.com/apps/signpath) on `midagedev/gadak`.
2. In SignPath: organization, project slug `gadak`, repository URL
   `https://github.com/midagedev/gadak`, trusted build system GitHub.com,
   **release-signing** policy with origin verification, allowed branches
   matching tags / `main` as SignPath advises, **one** required approval
   (`midagedev`), submitter = CI user (not an interactive user — OSS
   trusted-build policies forbid interactive submitters).
3. Artifact configurations, two slugs (or one zip that contains both PE
   names):
   - CLI: GitHub Actions artifact is the unsigned `gadak.exe` (or the zip
     with `<zip-file>` / `<pe-file path="gadak.exe">`).
   - Desktop: zip whose root contains `gadak-desktop.exe` and `gadak.exe`;
     deep-sign both PE files.
   - Enforce `product-name="gadak"` (or `"Gadak"`) and a single
     `product-version` per build — **blocked today** until VERSIONINFO is
     embedded.
4. Repo secrets/variables (names from SignPath’s Action README; do not
   commit values): `SIGNPATH_API_TOKEN`, `SIGNPATH_ORGANIZATION_ID`,
   `SIGNPATH_PROJECT_SLUG`, `SIGNPATH_SIGNING_POLICY_SLUG`, artifact
   configuration slugs.
5. Workflow change (later round): on `desktop-windows` and on the
   goreleaser Windows build, `actions/upload-artifact` the unsigned files,
   then `signpath/github-action-submit-signing-request@v2` with
   `wait-for-completion: true`, then attach the signed bytes to the GitHub
   Release. OSS rule: every job **before** that step stays on
   `windows-latest` / `ubuntu-latest` (already true). Do not introduce a
   self-hosted runner on that path.
6. Timeout: the Action default wait is 600s. A single-maintainer approval
   email may need a longer `wait-for-completion-timeout-in-seconds` or a
   two-stage job (submit, then a follow-up download after approval).
7. After the first signed tag: flip WINDOWS-SIGNING.md and README from
   “unsigned” to the required SignPath sentence; keep the SHA256 procedures.

## Lead checklist before submit

- [ ] GitHub 2FA on `midagedev` (and SignPath, once the account exists).
- [ ] Contact email that accepts external mail.
- [ ] [WINDOWS-SIGNING.md](../WINDOWS-SIGNING.md) is on `main` and README
      Windows install sections link it (term **Code signing policy** visible).
- [ ] Form answers above still match HEAD (artifact names, unsigned status).
- [ ] Do **not** claim SignPath signing, Azure signing, or “safe”.
- [ ] Do **not** promise bit-reproducible builds.
- [ ] Reputation risk (19 stars, repo younger than a month) is accepted;
      a refusal is possible and not a product defect.
- [ ] VERSIONINFO work is queued as a follow-up issue, not silently assumed
      done.
- [ ] Submit at https://signpath.org/apply.html (lead). Comment on GDK-211
      with the submission date (lead). Do not edit workflows until granted.

[GDK-211]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-211
