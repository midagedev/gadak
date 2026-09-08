<p align="center">
  <img src="docs/media/wordmark-dark.svg#gh-dark-mode-only" width="380" alt="gadak">
  <img src="docs/media/wordmark-light.svg#gh-light-mode-only" width="380" alt="gadak">
</p>

<p align="center">
  <a href="https://github.com/midagedev/gadak/releases"><img src="https://img.shields.io/github/v/release/midagedev/gadak" alt="Latest Release"></a>
  <a href="https://github.com/midagedev/gadak/actions/workflows/ci.yml"><img src="https://github.com/midagedev/gadak/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="License"></a>
</p>

<p align="center"><b>Find the thread in your backlog.</b></p>

<p align="center"><sub><a href="README.md">English</a> · 한국어 · <a href="README.ja.md">日本語</a></sub></p>

지라를 쓰기 싫은데 어쩔 수 없이 써야 해서 만들었습니다. 묵은 이슈들을
클로드로 뒤지다가 한참 걸리고 결국 rate limit에 걸려 중단된 적이 있는데, 그때
만들기 시작했습니다. 크롬에 지라 탭이 잔뜩 쌓여 피곤해지는 것도 겸사겸사 없애고
싶었고요.

gadak은 필요한 Jira 프로젝트와 Confluence 스페이스만 골라 이 컴퓨터에 캐시합니다.
이슈와 댓글, 변경 이력, 위키 문서를 한 검색창에서 찾고, SQL로 집계합니다. 검색과
이슈 조회는 캐시에서 끝나고, 쓰기는 Jira가 먼저 받은 뒤 캐시가 따라 바뀝니다.
데스크톱 앱, `gadak serve`가 여는 브라우저 탭, CLI가 같은 캐시를 보고, Claude
Code에는 스킬 하나로 넘깁니다. 바이너리 하나로 돌고, gadak 계정은 없습니다.

## 먼저 눌러 보기

[라이브 데모](https://gadak.dev/demo/)에 이슈 534건이 들어 있습니다. 설치도
계정도 없이 브라우저에서 열립니다. 바로 가기: [연결 전에 확인할 것](#연결-전에-확인할-것)
· [설치](#설치와-첫-실행) · [Claude Code](#claude-code와-다른-에이전트) ·
[지원 범위](#지원-범위와-다른-사용-방식).

## 연결 전에 확인할 것

- **연결되는 Jira는 Atlassian Cloud입니다.**
  [API 토큰](https://id.atlassian.com/manage-profile/security/api-tokens)
  하나로 같은 사이트의 Jira와 Confluence에 붙습니다. 토큰은 스코프 없이 만든
  사용자 토큰(`ATATT…`)이어야 합니다. 스코프가 붙은 토큰이나 admin.atlassian.com의
  조직 키(`ATCTT…`)로는 사이트에 로그인할 수 없습니다. Jira Server와 Data
  Center는 `gadak init --server`에 Personal Access Token 하나로 붙습니다.
  위키는 거기서는 꺼져 있습니다(Confluence Server 클라이언트가 없습니다).
- **가져올 범위는 직접 정합니다.** `--projects`로 Jira 프로젝트를, `--spaces`로
  위키 스페이스를 고릅니다. 스페이스를 지정하기 전에는 위키를 가져오지 않습니다.
- **캐시는 이 컴퓨터 안의 SQLite 파일 하나입니다.** 처음 한 번은 전체 동기화가
  필요합니다(측정한 사이트에서 10.6분). 그 뒤로는 `gadak serve`가 기본 60초
  간격으로 증분 동기화를 돌리고, 한 시간에 한 번 대조를 해서 삭제됐거나 더 볼 수
  없게 된 이슈를 캐시에서도 지웁니다. 캐시는 지워도 됩니다. 다시 동기화하면
  그대로 돌아옵니다.
- **API 토큰은 캐시에도, 로그에도, 스냅샷에도 남지 않습니다.** 토큰은
  `~/.gadak/config.json`에 0600 권한으로 저장되고, 내 사이트로 보내는 요청의
  Authorization 헤더에만 쓰입니다.
- **텔레메트리는 없습니다.** gadak이 연결하는 곳은 직접 설정한 곳뿐입니다. 전체
  목록과 조건은 [`SECURITY.md`](SECURITY.md)에, 연결마다 끄는 방법은
  [`docs/NETWORK.md`](docs/NETWORK.md)에 있습니다.
- **쓰기는 Jira로 먼저 갑니다.** Jira가 받아들이면 캐시를 갱신하고, 거절하면 그
  자리에서 실패합니다. 캐시에 쌓아 두고 나중에 보내는 일은 없습니다.
- **몇 가지 읽기는 Jira에 직접 묻습니다.** 첨부파일 보기, `gadak issue
  --editmeta`, `gadak fields`, `gadak api`가 그렇습니다.
- **캐시를 읽는 에이전트는 읽은 내용을 자기 모델로 보냅니다.** gadak 자신은
  아무 데도 보내지 않습니다. 에이전트가 봐도 되는 프로젝트와 스페이스만
  캐시하세요.

## 설치와 첫 실행

macOS 앱(CLI 포함):

```bash
brew install --cask midagedev/tap/gadak
```

CLI만:

```bash
brew install midagedev/tap/gadak-cli
```

첫 실행. 사이트 주소, 이메일, API 토큰, 캐시할 프로젝트를 차례로 묻습니다. 끝나면
`gadak serve`가 `http://gadak.localhost:7777`을 찍습니다:

```bash
gadak init && gadak sync && gadak serve
```

처음부터 범위를 좁혀 시작하려면 프로젝트와 스페이스를 함께 줍니다:

```bash
gadak init --projects ENG,PROD --spaces ENG
```

화면은 한국어로 뜹니다. 브라우저와 OS 언어를 따르고, 설정에서 바꿀 수 있습니다.
dmg, 리눅스 tarball, Docker, 업그레이드는 [`docs/INSTALL.md`](docs/INSTALL.md)에,
캐시 안에 무엇이 어떤 모양으로 들어 있는지는 [`docs/MIRROR.md`](docs/MIRROR.md)에
있습니다.

**Windows.** 데스크톱 앱은 [Microsoft Store](https://apps.microsoft.com/detail/9NZW91TXH36G)에
있습니다. Store가 서명하니 SmartScreen도 Smart App Control도 막지 않고,
0.20.2부터는 Store로 설치하면 `gadak`이 `PATH`에 올라갑니다. Store 없이 CLI만
쓰려면 [최신 릴리스](https://github.com/midagedev/gadak/releases/latest)의
`gadak_<version>_windows_amd64.zip`(또는 `arm64`)을 풉니다. 릴리스에 있는
데스크톱 zip(`Gadak-<version>-windows-x64.zip`)은 아직 서명이 없어서 SmartScreen이
막습니다. 바이러스가 아니라 서명이 없다는 경고입니다
([`docs/WINDOWS-SIGNING.md`](docs/WINDOWS-SIGNING.md)). 막히면 Store로 가고, Smart
App Control은 끄지 마세요.

## Claude Code와 다른 에이전트

```bash
gadak skill install
```

Claude Code용 스킬을 설치합니다. 스키마와 쿼리 패턴이 담긴 파일 하나이고, 별도
프로세스는 띄우지 않습니다. `gadak skill install codex`처럼 이름을 붙이면
cursor·gemini·opencode·grok에도 같은 파일이 들어갑니다. 셸이 없는 Claude
Desktop에는 `gadak mcp install claude-desktop`으로 MCP 서버를 등록합니다.

<p align="center">
  <img src="docs/media/terminal-hero.ko.gif" alt="gadak 앱의 터미널에서 Claude Code로 이슈 목록을 바꾸고 라벨 비율 대시보드를 저장해 여는 한국어 세션" width="900">
  <br>
  <sub>앱 창 안의 셸(⌘K → 터미널, 또는 Ctrl+`)에서 <code>gadak claim NMA-140</code>을 치면 이슈가 진행 중으로 바뀌고 셸 탭 이름이 그 키로 바뀝니다. 그 셸에서 시작한 Claude Code 세션이 옆의 보드를 움직입니다. 화면, 트래커, 프롬프트 전부 한국어 세션이고, 프롬프트 두 줄 외에는 대본이 없습니다. 에이전트가 일하는 구간은 빨리 감았습니다. 녹화: <a href="e2e/demo/terminal-claude-demo.spec.ts">e2e/demo/terminal-claude-demo.spec.ts</a>, <a href="e2e/demo/record-terminal-claude.sh">record-terminal-claude.sh</a>.</sub>
</p>

한국어 계정에서 에이전트가 가장 자주 빠지는 함정이 하나 있습니다. Jira는 상태와
우선순위 이름을 계정 언어로 번역합니다. 그래서 `priority = High`는 한국어
계정에서 오류 없이 0행이고, 에이전트는 그걸 "그런 이슈는 없다"로 읽습니다. 필터는
`status_category`와 `priority_rank`로 걸어야 하고, 스킬이 그렇게 가르칩니다. SQL로
찾은 이슈는 `gadak sql --no-header "…" | gadak views open --keys -`로 앱에 띄울 수
있고, `gadak views open --jql '…'`은 붙여 넣은 JQL을 필터 칩으로 내려놓습니다.

쓰기(`create`, `edit`, `comment`, `transition`, `claim`, `link`, 위키 `page`)는
Jira를 거친 뒤 캐시가 갱신됩니다. 에이전트가 남긴 댓글과 만든 이슈에는
에이전트 이름이 붙습니다. SQL 레퍼런스는 [`docs/MIRROR.md`](docs/MIRROR.md),
도구별 연결 방법은 [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md)에 있습니다.

## SQL로 집계하기

JQL에는 `GROUP BY`가 없습니다. 이번 측정에서는 에픽별 열린 이슈를 세려고 API 결과
8페이지를 받아 프로그램에서 합산했습니다. gadak에서는 쿼리 한 번입니다:

```bash
gadak sql "select epic_key, count(*) from issues_full where resolved_at is null
           and epic_key <> '' group by epic_key order by 2 desc"
```

같은 쿼리를 [Datasette Lite가 데모 스냅샷에서 브라우저 안에서 돌려
줍니다](<https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc>).
SQL을 고쳐서 바로 다시 돌릴 수 있습니다. 나머지 쿼리는
[`docs/RECIPES.md`](docs/RECIPES.md).

## 성능 측정

2026-08-26, 실제 Atlassian Cloud 업무 프로젝트(이슈 3,296건)에서 잰 중앙값입니다.
gadak 쪽은 CLI 프로세스가 뜨는 시간까지 넣은 값입니다.

| 질문 | REST API | gadak | |
| --- | ---: | ---: | ---: |
| 단순 필터 100건 | 583 ms | 19 ms | 31× |
| 이슈 1건과 전체 변경 이력 | 710 ms | 28 ms | 25× |
| 텍스트 검색 | 543 ms | 41 ms | 13× |
| 에픽별 열린 이슈 (`GROUP BY`) | 4,761 ms, API 8페이지를 받아 합산 | 22 ms | 214× |
| 변경 이력 집계 | JQL로 표현 불가, 순회하면 약 28분 | 14 ms | |

첫 전체 동기화에는 시간이 걸리고(위 사이트에서 10.6분), 캐시는 동기화 주기만큼
늦습니다. 측정 방법과 재측정 이력은 [`docs/BENCHMARKS.md`](docs/BENCHMARKS.md).

## 지원 범위와 다른 사용 방식

Atlassian Cloud, Jira Server/Data Center, Linear, 내장 트래커에서 같은 명령을
씁니다. Server는 `gadak init --server`에 PAT 하나이고, Linear 동기화는
`gadak sync --source linear`입니다. 읽기, 쓰기, 계층, 첨부, 이력, 보드 배치가
넷 다에서 되고(위키는 Confluence Cloud와 내장 위키에서), 서비스별로 무엇을
거절하는지는 셀마다 코드를 인용한
[`docs/SUPPORT_MATRIX.md`](docs/SUPPORT_MATRIX.md)에 있습니다.

어느 서비스에도 없는 것이 셋 있습니다. 화면으로서의 스프린트, Jira 대시보드, Jira
알림함. 스프린트 계획, 관리자 작업, 화면 안에서 페이지 편집, 1분의 지연도 안 되는
일은 Jira에서 계속 합니다([`docs/CONCEPT.md`](docs/CONCEPT.md#good-fit--bad-fit)).

Atlassian 계정 없이 시작하려면 `gadak init --local`로 내장 트래커를 씁니다.
워크스페이스를 옮기는 명령은 `gadak --workspace <new> migrate --from <old>`이고,
Linear로 옮길 때는 `--to linear`를 붙입니다. 다른 컴퓨터와 페어링은
`gadak --workspace laptop init --pairing-code-stdin`입니다.

## 상태와 호환성

**상태: 0.21, 아직 0.x입니다.** 동기화와 읽기 API, Jira를 먼저 거치는 쓰기,
데스크톱·웹·CLI·MCP를 실제 사이트에서 확인했습니다. 지금은 한 사람이 만들고,
라이선스는 Apache-2.0입니다. 이름은 엉킨 실에서 뽑아낸 한 줄기, '가닥'에서
왔습니다.

0.x에서 바꾸지 않기로 약속한 것은 [data-model.md](specs/000-product/data-model.md)의
셋입니다. `issues_full`과 RECIPES 쿼리, `gadak sql`의 출력 형식, `gadak views
open --keys -`의 의미. 항목별 확인 명령은 [`docs/PROMISES.md`](docs/PROMISES.md)에,
무엇이 나왔는지는 [`CHANGELOG.ko.md`](CHANGELOG.ko.md)에 있습니다.

## 한 줄 남겨 주세요

텔레메트리가 없어서 누가 쓰는지 저는 모릅니다. 써 보셨다면 어떤 일에 썼고
어땠는지 알려 주세요. 불편했거나 결과가 틀렸다면 그 이야기도요. 이슈 수는
공개해도 괜찮을 때만 적어 주세요.
[GitHub 이슈](https://github.com/midagedev/gadak/issues), X
[@midagedev](https://x.com/midagedev) 멘션, 메일
[midagedev@gmail.com](mailto:midagedev@gmail.com), 어느 쪽이든 됩니다.

공개된 곳에는 실제 이슈 데이터나 토큰, 사이트 URL을 붙이지 마세요. 에이전트와
쓰다 걸린 일은 무엇을 물었고 에이전트가 무엇을 했는지만, 민감한 내용을 빼고 적어
주세요.

버그 리포트에는 Jira 배포 유형(Cloud), gadak 커밋, 실행한 명령이 있어야 합니다.
GitHub 이슈는 백로그에도 옮겨 두고, 커밋의 `GDK-nnn` 키는
[공개 백로그](https://gadak.dev/backlog/)로 이어집니다. 코드로 오시려면
[`CONTRIBUTING.md`](.github/CONTRIBUTING.md)와
[`docs/project/GOOD_FIRST_ISSUES.md`](docs/project/GOOD_FIRST_ISSUES.md).

## 문서

- [`CHANGELOG.ko.md`](CHANGELOG.ko.md) · 무엇이 나왔는지
- [`docs/INSTALL.md`](docs/INSTALL.md) · [`docs/DESKTOP.md`](docs/DESKTOP.md) · 설치와 데스크톱 앱
- [`docs/SHOWCASE.md`](docs/SHOWCASE.md) · 에이전트가 만든 대시보드·런처 녹화
- [`docs/MIRROR.md`](docs/MIRROR.md) · [`docs/MCP.md`](docs/MCP.md) · [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md) · SQL, CLI, REST, MCP, 도구별 연결 설정
- [`docs/RECIPES.md`](docs/RECIPES.md) · [`docs/DASHBOARDS.md`](docs/DASHBOARDS.md) · JQL이 못 묻는 질문
- [`SECURITY.md`](SECURITY.md) · [`docs/NETWORK.md`](docs/NETWORK.md) · [`docs/FAQ.md`](docs/FAQ.md) · [`docs/MAINTENANCE.md`](docs/MAINTENANCE.md) · 위협 모델, 연결 전체, 누가 유지하는가
- [`docs/BENCHMARKS.md`](docs/BENCHMARKS.md) · [`docs/SUPPORT_MATRIX.md`](docs/SUPPORT_MATRIX.md) · 측정 방법, 서비스별 지원 범위
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) · [`docs/EXTENDING.md`](docs/EXTENDING.md) · 동작 원리, 포크 없이 내 것으로
- [`docs/project/THEORY.md`](docs/project/THEORY.md) · 다음 기능이 왜 그것들인지(영문)
- [`docs/README.md`](docs/README.md) · 나머지 문서
