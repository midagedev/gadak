<p align="center">
  <img src="docs/media/wordmark-dark.svg#gh-dark-mode-only" width="380" alt="gadak">
  <img src="docs/media/wordmark-light.svg#gh-light-mode-only" width="380" alt="gadak">
</p>

<p align="center">
  <a href="https://github.com/midagedev/gadak/releases"><img src="https://img.shields.io/github/v/release/midagedev/gadak" alt="Latest Release"></a>
  <a href="https://github.com/midagedev/gadak/actions/workflows/ci.yml"><img src="https://github.com/midagedev/gadak/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="License"></a>
</p>

<p align="center"><b>Follow the thread.</b></p>

<p align="center"><sub><a href="README.md">English</a> · 한국어 — 영문이 원본이며, 이 문서는 v0.15.2 기준 번역입니다.</sub></p>

내 Jira를 로컬 SQLite 파일 하나로 — "어느 에픽이 막혀 있지?"가 물을 수
없는 질문이 아니라 쿼리 한 줄이 됩니다.

gadak은 Jira *그리고* Confluence를 로컬 SQLite 파일 하나로 미러링합니다 —
이슈, 코멘트, 히스토리, 위키 문서가 한 인덱스에 들어가고, 검색은 네트워크
없이 로컬에서 끝납니다. 그 데이터가 내 머신에서 사는 자리가 이 창입니다:
[데스크톱 앱](docs/DESKTOP.md)이나 브라우저 탭에서 트리아지하고, 코딩
에이전트가 SQL로 묻고 같은 창에 답을 띄우게 하세요. 바이너리 하나,
앱 하나, gadak 계정은 없습니다.

**미러는 버려도 되는 캐시입니다.** 이 프로젝트가 내일 멈춰도 디렉터리
하나를 지우면 끝 — 잃는 것이 없습니다. 원본은 언제나 Jira입니다.

<p align="center">
  <a href="https://midagedev.github.io/gadak/"><b>▶&nbsp; 라이브 데모 열기</b></a>
  &nbsp;—&nbsp; 이슈 534개, 지금 바로 브라우저에서.
</p>

```bash
gadak sql "select epic_key, count(*) from issues_full where resolved_at is null
           and epic_key <> '' group by epic_key order by 2 desc"
```

위 쿼리가 핵심입니다: JQL에는 `GROUP BY`가 없습니다. "어느 에픽이
실제로 막혀 있나"는 어려운 질문이 아니라 **물을 수 없는** 질문입니다 —
데이터가 파일이 되기 전까지는. 나머지 레시피는
[`docs/RECIPES.md`](docs/RECIPES.md)에 있습니다.

설치 없이 지금 바로 이 쿼리를 실행해 보세요: [Datasette Lite가 이 탭에 데모
스냅샷을
불러오고](<https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc>)
SQL은 브라우저 안에서 돌아갑니다.

실제 Cloud 사이트(이슈 2,853개)에서 실측한 값입니다 (중앙값, CLI 기동 포함 —
[방법론과 gadak이 지는 행](docs/BENCHMARKS.md)):

| 질문 | REST API | `gadak` | |
| --- | ---: | ---: | ---: |
| 단순 필터 100건 | 706 ms | 17 ms | 42× |
| 이슈 하나 + 전체 히스토리 | 1,055 ms | 54 ms | 20× |
| 에픽별 미해결 수 (`GROUP BY`) | 3,924 ms · API 7페이지 | 24 ms · 쿼리 1개 | 162× |
| 변경 이력을 걸치는 모든 질문 | ≈ 20분 (전 changelog 순회) | 쿼리 1개 | — |

반대편도: 첫 full sync는 실측 534이슈 26.4초 · 2,865이슈 7.2분
([방법론과 gadak이 지는 행](docs/BENCHMARKS.md)), 조용한 사이트의 watch
틱은 ~6.7초를 쓰고, 미러는 동기화 주기만큼 Jira보다 늦습니다.

<details>
<summary>▶ 종이 리스트 90초 투어 (GIF, 7 MB)</summary>

<p align="center">
  <img src="docs/media/web-demo.gif" alt="타이핑할수록 종이 리스트가 좁혀지고, 이슈가 라벨·우선순위·리오픈 배지와 함께 열리며, 문서와 에픽이 같은 창에 있다" width="900">
  <br>
  <sub><a href="e2e/demo/web-demo.spec.ts">e2e/demo/web-demo.spec.ts</a>가 데모 스냅샷에 대해 생성.</sub>
</p>

</details>

macOS: [`Gadak-<version>-arm64.dmg`](https://github.com/midagedev/gadak/releases/latest)를
받아 창을 엽니다.

Windows (0.16부터): [`Gadak-<version>-windows-x64.zip`](https://github.com/midagedev/gadak/releases/latest)
(또는 `windows-arm64`)을 받아 압축을 풀고 `gadak-desktop.exe`를 실행합니다.
이 빌드는 서명되어 있지 않습니다 — Windows가 막으면 아래 CLI 경로를
쓰세요. 자세한 내용:
[`docs/INSTALL.md`](docs/INSTALL.md#desktop-app-windows).

또는 터미널에서:

```bash
brew install midagedev/tap/gadak        # macOS 앱 — 번들된 CLI도 PATH에 올라갑니다
# CLI만 (macOS + Linux는 brew; Windows는 릴리스 zip):
brew install midagedev/tap/gadak-cli
gadak init && gadak sync    # Jira (그리고 Confluence) -> ~/.gadak/gadak.db
gadak serve                # http://gadak.localhost:7777
```

> **상태: 0.16, 아직 0.x입니다.** 동기화, 읽기 API, 쓰기 통과(write-through),
> 데스크톱, 웹, CLI, MCP가 실제 사이트에 대해 검증되어 있습니다. 숨김 없는
> 현황은 [`docs/STATE_OF_PLAY.md`](docs/STATE_OF_PLAY.md)에.

## 왜 만들었나

Jira 검색은 네트워크 왕복이고, 위키는 두 번째 검색입니다. "우리가 뭘 이미
고쳤고, 뭘 결정했지?"라고 묻는 에이전트는 REST API 두 개를 페이징합니다.
원인은 같습니다: 데이터가 파일이 아니라서.
[`docs/CONCEPT.md`](docs/CONCEPT.md) · [`docs/PAIN_POINTS.md`](docs/PAIN_POINTS.md).

인덱스는 ⌘K 하나입니다 — 제목, 본문, 코멘트, 이슈와 문서가 전부 거기
들어갑니다. 리스트에 걸린 칩은 여기 적용되지 않습니다. 코멘트에만 나온
단어로도 그 행이 잡히는 이유입니다.

<p align="center">
  <img src="docs/media/search.gif" alt="리스트에 Project 칩이 걸린 상태에서 ⌘K로 팔레트를 열고 코멘트에만 있는 단어를 입력하면, 다른 프로젝트의 행들이 Comment match 라벨과 스니펫을 달고 전체 검색을 채운다" width="900">
  <br>
  <sub><a href="e2e/demo/search-demo.spec.ts">e2e/demo/search-demo.spec.ts</a>가 데모 스냅샷에 대해 생성.</sub>
</p>

## 표면은 둘, 저장소는 하나

| | 용도 | 모습 |
| --- | --- | --- |
| **앱 + 웹 UI** | 종일 트리아지 | [데스크톱 앱](docs/DESKTOP.md)(포트 없음) 또는 `gadak serve`. `j`/`k`로 이동, `x`로 선택, `s`/`a`/`l`/`c`로 리스트에서 바로 상태·담당자·라벨·코멘트 변경 |
| **CLI + SQL** | 에이전트, 스크립트 | `gadak issue`, `gadak search`(FTS, `--jql`, Jira URL), `gadak sql`, 그리고 파일 그 자체 |

쓰기는 Jira로 통과된 뒤 미러가 갱신됩니다. 앱·웹: 코멘트, 상태 전이,
담당자, 라벨, 우선순위, 제목. CLI: `create`(단건 또는 `--batch`),
`attach`, `edit`, `comment`, `transition`, `assign`.
위키 미러는 읽기 전용입니다. 계층 구조, `item_refs`, 첨부:
[`docs/CONCEPT.md`](docs/CONCEPT.md#two-surfaces).
창은 네 팔레트에서 같은 종이 메타포를 유지합니다 — `light`, 중립-쿨
`dark`, 블루-블랙 `ink`, 웜 `ember`. 테마는 고르지 않으면 시스템을 따르고,
브라우저가 아니라 **워크스페이스**에 속합니다:
`gadak config set appearance.theme ink`.

그리고 두 표면은 닫힌 목록이 아닙니다. 미러를 읽는 것은 바이너리 호출
하나(`gadak search --json`, ~20ms)이고, 앱에서 무언가를 여는 것은 URL
하나(`gadak://view?issue=…` — [스킴](docs/DESKTOP.md))입니다. 그 둘을 할
수 있는 것이면 무엇이든 표면이 됩니다. 예컨대 런처:

<p align="center">
  <img src="docs/media/raycast.gif" alt="Raycast가 타이핑마다 로컬 gadak 미러를 검색한다 — 텍스트 질의는 매치된 스니펫을 볼드와 필드 태그로 보여주고, 이슈 키를 그대로 치면 그 이슈가 나오며, Enter는 gadak:// 딥링크로 Gadak 앱에서 연다" width="800">
  <br>
  <sub>키스트로크 하나가 <code>gadak search --json</code> 한 번이고, Enter가 딥링크입니다. 저장된 뷰도 같은 길로 갑니다 — <code>gadak views open</code>이 링크를 출력합니다.</sub>
</p>

그 런처는 이미 있습니다: 타이핑마다 이슈와 위키 문서를 검색하는 Raycast
확장이고, [Raycast Store에 제출되어](https://github.com/raycast/extensions/pull/30297)
심사 중입니다. 심사가 끝나기 전에도, 이미 설치된 바이너리에서 한 줄로
설치됩니다(확장이 내장되어 있어 체크아웃이 필요 없습니다):

```sh
gadak raycast install
```

macOS 앱에서는 같은 설치가 버튼입니다 — **설정 → 연동**이 Raycast·에이전트
스킬·MCP를 한 목록으로 보여주고, 무엇이 설치되어 있는지 표시하며, 화면에
적힌 바로 그 명령을 실행합니다. 확장 자체를 만지려면:
[`contrib/raycast/`](contrib/raycast/). 확장 없이 가려면, 키로 여는 쪽
절반은 `gadak://view?issue={argument}`를 넣은 Raycast Quicklink로 됩니다.

## 에이전트를 위해

gadak이 존재하는 이유의 절반입니다. 레퍼런스: **[AGENTS.md](AGENTS.md)**.
호스트별 붙여넣기 한 번: [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md).

```bash
gadak skill install         # 스키마 + 쿼리 패턴, 별도 프로세스 없음
# 또는, 셸이 없는 호스트(Claude Desktop)라면:
gadak mcp install claude    # 이 바이너리와 프로필을 등록에 고정
```

두 설치(그리고 Raycast까지)는 macOS 앱에서는 버튼이기도 합니다 — 설치
상태를 정직하게 보여주는 **설정 → 연동**.

설정도 에이전트가 화면을 눌러야 하는 일이 아닙니다. 설정 다이얼로그가
편집하는 모든 필드가 같은 검증을 지나는 CLI 동사입니다:

```bash
gadak config list                          # 편집 가능한 전체 경로와 현재값
gadak config set appearance.theme ink      # 워크스페이스별, 즉시 적용
```

SQL이 답하고, 창이 보여 줍니다. 이미 JQL이 있다면 SQL을 건너뛰세요 —
절이 그대로 칩이 됩니다:

```bash
gadak sql --no-header "select key from issues_full where status_category = 'inprogress'
                       order by status_changed_at asc limit 5" | gadak views open --keys -
gadak views open --jql 'project = NMA AND priority = High AND resolution is EMPTY'
```

<p align="center">
  <img src="docs/media/agent.gif" alt="터미널이 gadak sql을 gadak views open --keys - 로 파이프하자 실행 중인 앱이 그 다섯 키로 즉시 이동하고, 이어서 gadak views open --jql이 같은 창을 프로젝트·우선순위·미해결 칩 위에 내려놓는다" width="800">
  <br>
  <sub><code>gadak views open</code>은 일회성 해시를 쓰고, 실행 중인 앱 또는 serve 탭이 그것을 적용합니다. <a href="e2e/demo/agent-demo.spec.ts">e2e/demo/agent-demo.spec.ts</a>가 생성.</sub>
</p>

셸이 없는 호스트(Claude Desktop)에서는 같은 미러가 MCP 서버가 됩니다.
Jira가 아예 답할 수 없는 것을 물어보세요 — 위키는 두 번째 검색이니까요.
"X에 대해 우리가 아는 게 뭐지?" 한 인덱스가 둘을 다 담고 있으므로, 답은
티켓과 그 티켓을 만든 설계 문서를 한 문장에 담을 수 있습니다.

<p align="center">
  <img src="docs/media/mcp.gif" alt="Claude Code가 gadak을 MCP 서버로 등록하고, Jira와 위키에서 idempotency를 검색해 달라는 요청에 gadak을 호출하여, 이슈와 그 이슈를 만든 Confluence 문서로 답한다" width="800">
  <br>
  <sub>도구 다섯 개. 미러에도 Jira에도 쓰지 않습니다. 셸이 있는 호스트는 <code>gadak sql</code>을 쓰면 됩니다. 설정: <a href="docs/MCP.md">docs/MCP.md</a>.</sub>
</p>

`gadak views open`은 "gadak에서 열기" 동사이고, `gadak open KEY`는 Jira로
나가는 문입니다. 리스트 박스는 `gadak search --jql`과 같은 JQL 붙여넣기를
받습니다. gadak이 표현하지 못하는 절은 조용히 버려지지 않고 나열됩니다.
JQL로 여전히 물을 수 없는 것은 `gadak sql`과
[`docs/RECIPES.md`](docs/RECIPES.md)의 몫입니다. `gadak sql`은 파일을
`mode=ro`로 열고, MCP의 `gadak_query`는 SELECT가 아닌 것을 전부
거부합니다. [`gadak api`](docs/AGENT_ACCESS.md)는 미러가 모델링하지 않는
엔드포인트로의 통로입니다 — `--write` 없이는 읽기 전용, MCP에는 절대
없습니다.

**미러를 읽는 에이전트는 읽은 것을 자기가 쓰는 모델로 보냅니다.** gadak
자신은 아무것도 보내지 않습니다([`SECURITY.md`](SECURITY.md)). 에이전트가
봐도 되는 범위로 미러를 좁히세요.

## 설치

Atlassian Cloud 전용입니다. [API 토큰](https://id.atlassian.com/manage-profile/security/api-tokens)
하나로 같은 사이트의 Jira와 Confluence를 모두 커버합니다.

**1. [데스크톱 앱](docs/DESKTOP.md).**

macOS: [최신 릴리스](https://github.com/midagedev/gadak/releases/latest)에서
`Gadak-<version>-arm64.dmg`를 받아 Applications로 드래그하세요. 서명·공증
완료. 첫 실행이 사이트, 이메일, 토큰, 프로젝트 선택을 안내합니다. CLI는
번들 안에 있습니다 — macOS는 앱을 `PATH`에 올려 주지 않으므로:

```bash
/Applications/Gadak.app/Contents/Resources/bin/gadak install-cli
```

Windows (0.16부터): 같은 릴리스에서 `Gadak-<version>-windows-x64.zip`(또는
`windows-arm64`)을 받아 압축을 풀고 `gadak-desktop.exe`를 실행하세요. 서명은
없습니다(GDK-211). Windows가 **Windows protected your
PC** 또는 **Smart App Control blocked an app that may be unsafe**를 보여
주면 바이러스 탐지가 아닙니다 — 아래 CLI 경로를 쓰세요. Smart App Control을
끄지 마세요.
[`docs/INSTALL.md`](docs/INSTALL.md#desktop-app-windows).

**2. CLI** — 리눅스, Windows, 또는 같은 UI를 브라우저 탭으로:

```bash
brew install midagedev/tap/gadak-cli     # macOS + Linux
gadak init && gadak sync
gadak serve      # http://gadak.localhost:7777
```

Homebrew 없이 Windows에 설치하려면
[최신 릴리스](https://github.com/midagedev/gadak/releases/latest)에서
`gadak_<version>_windows_amd64.zip`(또는 `windows_arm64`)과 `checksums.txt`를
받으세요. 압축을 풀고 `gadak.exe`를 `PATH`에 둔 뒤
`gadak init && gadak sync && gadak serve`. 서명되지 않은 데스크톱 exe가
막히면 0.16에서 믿을 수 있는 Windows 경로입니다.

Scoop 매니페스트는 [`contrib/scoop`](contrib/scoop)에 있습니다. 버킷은
아직 게시되지 않았고, Windows 머신에서 `scoop install`을 돌린 적도 없습니다
([`docs/INSTALL.md`](docs/INSTALL.md#scoop-windows-cli)).

Homebrew 없이 리눅스에 설치하려면
[최신 릴리스](https://github.com/midagedev/gadak/releases/latest)에서
`gadak_<version>_linux_amd64.tar.gz`(또는 `linux_arm64`)와 `checksums.txt`를
받으세요. 아카이브 하나가 설치 전부입니다 — 웹 UI는 바이너리 안에 있습니다.

```bash
sha256sum --ignore-missing -c checksums.txt
tar -xzf gadak_<version>_linux_amd64.tar.gz
# `gadak`을 PATH에 두세요
gadak serve             # http://gadak.localhost:7777
gadak install-service   # 선택: systemd --user, 재부팅 후에도 serve 유지
```

Arch 리눅스: 검증된 `PKGBUILD`가
[`contrib/aur/gadak-bin`](contrib/aur/gadak-bin)에 있습니다 — 거기서
`makepkg -si`. 아직 AUR에는 없습니다. 업스트림 등록이 닫혀 있습니다
([`docs/INSTALL.md`](docs/INSTALL.md#arch-linux)).

설치 스크립트, 릴리스 아카이브, 소스 빌드, Docker, 위키 미러링, 프로필,
업그레이드: **[`docs/INSTALL.md`](docs/INSTALL.md)**.

## 나머지

**내 것으로 만들기.** 포크 없이 두 축으로: [`docs/EXTENDING.md`](docs/EXTENDING.md).
설정: [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md). 확장 데이터:
[`docs/PLUGINS.md`](docs/PLUGINS.md).

**동작 원리.** 바이너리 하나, SQLite 파일 하나. 증분 동기화 + 정합성
보정 패스. [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). 왜 확장 프로그램이나
Forge 앱이 아닌가: [`docs/decisions/0003-local-process.md`](docs/decisions/0003-local-process.md).

**맞는 곳 / 안 맞는 곳.** 매일의 검색 지연, 트래커 *와* 위키를 함께 보는
에이전트, 오프라인 읽기 — 맞습니다. 보드, 어드민, 위키 저작, 그리고 1분의
지연도 안 되는 일 — Jira에 남기세요.
[`docs/CONCEPT.md`](docs/CONCEPT.md#good-fit-bad-fit).

**비교.** jira-cli는 커맨드마다 라이브 API를 호출합니다. Linear는 다른
트래커입니다. Rovo MCP도 두 소스를 함께 검색하지만 호스팅형입니다 — 집계
불가, 오프라인 불가, 호출마다 토큰이 나갑니다.
[`docs/FAQ.md`](docs/FAQ.md#how-it-compares).

**다음 소스.** Confluence가 뼈대가 소스 중립임을 증명했습니다. 다음
소스는 수요 순으로: [`docs/ROADMAP.md`](docs/ROADMAP.md#more-sources-later).

## 문서

- [`docs/INSTALL.md`](docs/INSTALL.md) · [`docs/DESKTOP.md`](docs/DESKTOP.md) — 설치, 첫 실행, 데스크톱 앱
- [`AGENTS.md`](AGENTS.md) · [`docs/MCP.md`](docs/MCP.md) · [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md) — SQL, CLI, REST, MCP, 호스트별 붙여넣기 한 번
- [`docs/RECIPES.md`](docs/RECIPES.md) — JQL이 못 묻는 질문들, SQL로
- [`SECURITY.md`](SECURITY.md) · [`docs/FAQ.md`](docs/FAQ.md) · [`MAINTENANCE.md`](MAINTENANCE.md) — 위협 모델, 사이트 부하, 누가 유지하는가
- [`docs/EXTENDING.md`](docs/EXTENDING.md) · [`docs/PLUGINS.md`](docs/PLUGINS.md) — 팀에 맞추기
- [`docs/STATE_OF_PLAY.md`](docs/STATE_OF_PLAY.md) · [`docs/CONCEPT.md`](docs/CONCEPT.md) · [`docs/PAIN_POINTS.md`](docs/PAIN_POINTS.md)
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) · [`docs/UX_PRINCIPLES.md`](docs/UX_PRINCIPLES.md)
- [`docs/decisions/`](docs/decisions/) · [`specs/000-product/`](specs/000-product/) — 왜 그런가, 그리고 계약들

## 누가 만드나

현재는 한 사람입니다. 그 사실을 저울에 올리세요 — 그리고 반대편도:
미러는 내 Jira의 버려도 되는 캐시이고, 0.x가 약속하는 것은
[data-model.md](specs/000-product/data-model.md)의 세 가지(`issues_full`과
RECIPES 쿼리들, `gadak sql`의 stdout, `gadak views open --keys -`)뿐이며,
라이선스는 Apache-2.0이고, 파일은 무엇으로든 읽히는 평범한 SQLite입니다.
어려운 질문들: [`docs/FAQ.md`](docs/FAQ.md). 믿지 않아도 되는 것들, 각
항목마다 확인 명령과 함께: [`PROMISES.md`](PROMISES.md).

## 기여와 피드백

[`CONTRIBUTING.md`](CONTRIBUTING.md) — 시작은
[`docs/GOOD_FIRST_ISSUES.md`](docs/GOOD_FIRST_ISSUES.md)에서. 버그 리포트에는
Jira 배포 유형(Cloud), gadak 커밋, 실행한 명령이 필요합니다. 실제 이슈
데이터, 토큰, 사이트 URL은 공개 이슈에 절대 붙여넣지 마세요.

에이전트와 함께 gadak을 쓰다 걸리는 게 있나요?
[이슈를 열고](https://github.com/midagedev/gadak/issues) 무엇을 물었고
에이전트가 무엇을 했는지 적어 주세요.

## 라이선스

Apache-2.0. `LICENSE`와 `NOTICE`를 보세요.
