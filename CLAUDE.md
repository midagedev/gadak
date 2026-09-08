# gadak — 세션·에이전트 공통 계약

현재 유효한 규칙만 담는다. 히스토리·결정 경위는 `docs/decisions/`와
CHANGELOG의 몫. 여기 없는 도메인 지식은 `docs/project/STATE_OF_PLAY.md`(현황·
hard-won 목록)와 `AGENTS.md`(스키마·쿼리)가 원본이다.

## 제품 불변 조건 (깨면 제품이 아니다)

- **미러는 버려도 되는 캐시.** 원본은 항상 Jira — 그 Jira가 Atlassian
  Cloud든, gadak이 함께 들고 다니는 아주 미니멀한 셀프호스트 Jira
  (`issuetap`, gadak origin)든. 어느 쪽이든 미러는 origin에서 다시
  만들 수 있고, **gadak 자신은 원본을 보관하지 않는다.** gadak에만 존재하는
  원본 데이터를 만드는 변경은 금지 — 예외는 `local.db`(방문·검색 기록)와
  저장된 뷰이고, 그것들은 export 가능해야 한다.
- **영속은 origin의 몫이다.** gadak origin 의 영속 공간은
  issuetap의 persist 파일이며 미러가 아니다. 그래서 백업 대상은 `gadak.db`가
  아니라 그 파일이고, **워크스페이스는 origin 하나에 묶인다** — origin을
  바꾸는 것은 설정 편집이 아니라 새 워크스페이스다. 이 조항을 어기는 것이
  "다른 트래커를 조용히 가리키게 하는" 부류의 결함이다.
- **쓰기는 전부 origin(Jira)을 통과**한 뒤 미러 갱신. 미러에 직접 쓰는 API를
  열지 않는다. 위키도 같은 규칙이다 — 페이지 쓰기(생성·편집·코멘트,
  GDK-380/381/382)는 origin.Wiki를 통과한다: connected는 Confluence REST,
  gadak origin 은 issuetap 의 Confluence API(미러 직접 쓰기가 아니다).
- **아웃바운드 없음.** 텔레메트리 금지. 나가는 요청은 사용자가 설정한
  origin(Atlassian 사이트·Linear: api.linear.app/uploads.linear.app), 페어링한
  home serve, 사용자가 직접 실행한 `gh`·라이브러리 다운로드, loopback뿐
  (업데이트 체크는 GDK-1626으로 제거됐다 — 부재를 카피로 되살리지 않는다)
  (`SECURITY.md`가 정본).
- **계정·서버·포트 강제 없음.** loopback 단일 사용자 모델 (`docs/decisions/0003`).

## 스키마·계약

- 0.x에서 약속된 것은 **셋뿐**: `issues_full`+RECIPES 쿼리 / `gadak sql`
  stdout 형식 / `views open --keys -` 의미. 원본:
  `specs/000-product/data-model.md` 상단. 문서에서 "스키마 전체가 계약"
  이라고 쓰지 마라.
- **상태·우선순위·이슈 유형은 display name으로 키하지 않는다.**
  `status = 'In Progress'`는 한국어 계정에서 소리 없이 0행이다. 항상
  `status_category` (new|inprogress|done) 또는 `status_id`; 우선순위는
  `priority_rank` (또는 `priority_id`); 유형은 `issue_type_id`. 이 함정은
  코드·문서·툴 설명 어디서든 재발 금지.
- `time-in-status`는 저장 컬럼이 아니다 — `status_changed_at`에서 계산.
  (`data-model.md`가 "deliberately absent"로 명시.)
- decisions/ 문서는 **개정하지 않는다. Addendum만 추가.**
- CHANGELOG는 히스토리 — 소급 수정 금지. **예외: 주장을 바꾸지 않는
  링크화**(사용자 결정 2026-08-20). `GDK-nnn`을 공개 백로그
  (`…/gadak/backlog/#/?ks=<KEY>`)로 참조 스타일 링크로 잇는 것은 히스토리
  수정이 아니라 해석 가능성 부여다 — 링크는 광고판이기도 하다. 문장·날짜·
  주장을 고치는 것은 여전히 금지.

## 빌드·게이트

- Go: `go build ./...` · `go test ./... -count=1` · `go vet ./...` ·
  **`gofmt -l` 빈 출력**(CI에 gofmt 잡이 있다 — 2026-08-24 tokencheck 정렬로
  로컬 전부 초록인데 CI만 빨간 사고. 신규 .go 파일이 있는 커밋은 필수)
- **루트 Go 코드에 서드파티 import가 새로 생기면 `desktop/`(별도 go.mod)도
  게이트다**: `cd desktop && go mod tidy && go build ./...`. 2026-08-23
  GDK-635에서 internal/ 신규 패키지의 runewidth import가 desktop go.sum에
  없어 데스크톱 CI 3개 잡이 빨갛게 됐다 — 로컬 go 전체는 초록이었다.
- **Go 정적 분석은 `bash tools/staticcheck.sh`이고 반드시 GOOS 매트릭스다**
  (GDK-1463). darwin 단독 실행은 `//go:build` 반대편에서만 호출되는 함수를
  U1000 죽은 코드로 오탐한다(`parseProcStartTime`·`protocolDefaultIcon`
  실측). 세 GOOS가 모두 동의한 것만 실패이고 나머지는 informational로
  인쇄된다. ST1005는 제외(한국어 에러 문장·의도된 다중행 프로토콜 에코).
  CI 잡 `Staticcheck (warning-only)`은 `--warn-only`라 아직 게이트가 아니다
  — 남은 cross-platform 12건을 정리한 뒤 플래그를 떼는 것이 게이트화다.
- **OS별 경로·카탈로그를 만드는 Go 코드는 `goos`를 인자로 받고 세 GOOS를
  테스트로 잰다** (2026-09-08, 런 34233540027). `integrations.listFor(goos)`
  가 goos 카탈로그를 약속하는데 Claude Desktop 행만 런타임 바인딩 helper를
  불러 **호스트의 경로**를 답했다 — darwin 에서는 요청과 답이 우연히 같아서
  로컬 go 전체가 초록이고, Linux CI 에서만 `~/.config/Claude` 로 죽는다.
  `…For(goos, …)` 씨임을 만들어 놓고 정작 그 행이 안 쓰는 것이 실제 형태였다.
  테스트는 한 GOOS 만 재지 말고 darwin·linux·windows 셋을 같은 호스트에서
  전부 재고, 실패 메시지에 **어느 goos 를 물었는지** 적는다(그게 없으면 CI
  실패가 경로 형식 문제처럼 읽힌다).
- 웹: `make typecheck` (svelte-check). e2e: Playwright, CI 세트는
  `e2e/*.spec.ts`(demo/·hosted/·perf/ 제외 — `e2e/playwright.config.ts`).
- **`mobile/`은 루트 게이트가 보지 않는다** — 자기 tsconfig·자기 lockfile을
  갖고 있으면서 `web/src`의 일부 모듈(i18n 카탈로그, `lib/terminal/protocol`)을
  직접 import한다. **그 둘 중 하나를 건드렸으면** `cd mobile && npm test &&
  npm run check && npm run lint:ios`도 게이트다 (2026-08-26: 웹에서 지운
  i18n 키를 `mobile/src/screens/Shell.svelte`가 계속 써서, 로컬 go·typecheck·
  vitest·e2e 326·doc-checks가 전부 초록인 채로 Mobile 잡만 빨갰다).
  **`mobile/src`의 화면·스토어를 건드렸으면 `npm run viewport-gate`까지가
  게이트다** — `mobile/e2e/`(Playwright, 402×874 + 셸 6종)는 위 세 명령이
  전혀 보지 않는데 CI Mobile 잡은 돌린다 (2026-08-29: dev 셸 자동 채택이
  "페어링 없으면 탭 3개" 계약을 깼고, 로컬 go·web e2e 378·mobile vitest·
  check·lint:ios가 전부 초록인 채로 CI에서 8개가 빨갰다). 반대 방향도 있다:
  **폰에서 i18n 키의 마지막 사용처를 지우면 웹 `npm run test:unit`이
  게이트다** — `web/src/lib/i18n/catalog.test.ts`가 미사용 키를 빨강으로
  만든다 (2026-09-07 GDK-1542: mobile만 건드린 커밋이 CI Frontend unit만
  적색, 로컬 mobile 게이트 넷은 초록). GDK-1540부터 이
  게이트는 dev 서버가 아니라 **빌드된 번들**(`vite preview`)을 찍는다 — 다른
  라운드의 편집이 스펙 중간에 리로드를 일으키지 않고, 포트는
  `GADAK_MOBILE_E2E_PORT`/`GADAK_MOBILE_API_PORT`(기본 5182/7899)로 라운드마다
  달리 줄 수 있으며, 다른 워크트리가 띄운 서버는 스탬프 대조가 거절한다.
  약 1분(빌드 포함) — 병렬 라운드에는 스펙에 포트 쌍을 명시 배정.
- **브랜드 마크(`docs/media/logo.png`)를 건드렸으면 `make brand`가 게이트다.**
  데스크톱은 빌드 때 그 로고를 리사이즈하니 새 마크를 자동으로 집지만, 폰
  아이콘은 생성해 커밋하는 파일이라 따라오지 않는다 — 2026-08-27까지 폰은
  스캐폴드 기본 아이콘을 달고 있었다. 재생성 없이 커밋하면
  `tools/check-brand-icons.sh`(CI Mobile 잡)가 빨강이다. iOS 세트는 트리에
  사본이 둘(`mobile/src-tauri/icons/ios/`와
  `mobile/src-tauri/gen/apple/Assets.xcassets/`)이고 `tauri icon`은 후자만
  쓴다 — `tools/brand/mobile-icons.sh`가 앞쪽을 미러링하고, 게이트가 둘의
  일치를 잰다.
- **e2e 직렬화의 실체는 락 파일이 아니라 포트다** — 그리고 포트의 단일
  소유자는 `GADAK_E2E_PORT`(기본 7877, `e2e/helpers.ts` `e2eServePort()`)다.
  홈은 포트별로 격리된다(`e2e/.tmp/home-<port>`) — GDK-672 랜딩으로 **서로
  다른 포트를 준 스위트 두 개는 병렬 가능**하다(병렬 라운드에는 스펙에
  포트를 명시 배정). 같은 포트 위의 충돌·낡은 서버 재사용은 여전히 경쟁
  신호이고(스탬프 불일치는 `assertServedArtifact`가 잡는다), 한 스위트
  안은 `workers: 1`이다.
- **터미널 e2e(`e2e/terminal*.spec.ts`·`e2e/issue-command.spec.ts`)를
  건드렸으면 `npm run test:e2e:wide-prompt`도 게이트다.** 그 스위트는 pane의
  셸을 `e2e/ci-shell.sh`로 바꿔 리눅스 CI 러너의 환경 셋을 재현한다 —
  24열 프롬프트(로컬 macOS는 훨씬 짧아 줄이 안 접힌다), Ubuntu 기본
  `.bashrc`의 창 제목 OSC(종결자가 BEL이다), 그리고 **배너 없음**(macOS
  bash 3.2의 zsh 안내 3줄이 모든 출력을 세 행 아래로 밀어 결함 하나를
  가리고 있었다). 2026-08-30: 새 터미널 e2e 5건이
  로컬 397 전부 초록인 채 CI에서만 죽었고, 원인은 접힌 줄을 읽는 방식·BEL
  판정·xterm 링크 캐시 셋이었다. 버퍼를 읽을 때는 스펙마다 `readTerm`을 다시
  쓰지 말고 `e2e/helpers.ts`의 것을 쓴다(접힘의 단일 소유자).
- 게이트 단언 완화는 ①귀속 주석 ②정당한 파생 ③FAIL-first 증거 셋 모두
  있을 때만.
- **`web/src/app.css` 의 `@theme` 토큰(색·모션·간격)을 건드렸으면 `npm run theme-check`
  와 `node tools/token-catalog.mjs`(catalog.json 재생성·커밋)도 게이트다** (2026-09-02:
  bg-hover 를 잉크 토큰으로 바꾼 뒤 로컬 go·typecheck·vitest·Playwright 432 전부
  초록인 채로 CI 만 두 번 연속 빨갔다 — 첫 번째는 hex 전제, 두 번째는 카탈로그
  불일치. 둘 다 CI 의 Theme check 잡만 본다).
- 문서 사실성 가드: `tools/doc-checks.sh` (있으면 커밋 전 실행).
- **origin 표면을 바꿨으면 `docs/SUPPORT_MATRIX.md`도 같은 커밋이다** (GDK-1300,
  2026-09-02). Jira·Linear·Built-in 세 열의 단일 소유자이고 README 둘은 링크만
  한다. `internal/origin/writer.go`·`linearwriter.go`·`internal/linear/`·
  `internal/sync/`·issuetap 의존성 범프·CLI 동사의 origin 분기(`gadak open`
  같은)를 건드린 커밋은 해당 셀과 `[^n]` 각주의 `path:line`을 갱신한다.
  `tools/doc-checks.sh` #39 는 구조(파일·헤더·셀 형식·각주 정의·README 링크)만
  잡고 셀의 참·거짓은 리뷰의 몫이다 — 코드에서 생성하는 것은 GDK-1301.
- **어휘 일괄 치환은 계약 문자열을 삼킨다 — 치환 뒤에 계약을 따로 세어라**
  (2026-09-02 GDK-1278 어휘 리네임, 5회 발생). 저장 값·에러 코드·요청 필드·
  라우트·데이터 id 접두사·DOM testid 는 어휘가 아니라 wire 계약인데, `\bword\b`
  치환은 주석과 구분하지 못한다. 실측으로 삼킨 것: 플래그 이름
  (`--replace-standalone` → 없는 플래그), 미러 데이터 id
  (`standalone-jira:`), API 라우트 정규식(`onboarding/standalone/`),
  식별자 자리에 들어간 산문(`Local-origin bool`), 그리고 **MCP 툴 서술의
  enum**. 앞의 넷은 게이트가 잡았고 다섯째는 아무 게이트도 안 봤다 —
  **`internal/mcp/tools.go` 의 서술은 게이트가 없는 표면이다**: 서버가 내지
  않는 값을 가르쳐도 go·doc-checks·e2e 전부 초록이고, 그것을 읽는 것은
  셸 없는 에이전트뿐이다. 리네임 커밋은 게이트 전부를 뒤에 세운 한 덩어리로
  하고, 커밋 전에 `grep` 으로 wire 계약 목록을 눈으로 확인한다.
- **IP 리터럴·호스트명·홈 경로가 들어가는 커밋은 `bash scripts/scan-internal.sh`도
  게이트다** — CI의 secret/internal-string 스캔과 같은 스크립트이고, 로컬
  게이트 목록에 없어서 두 번 연속 CI만 빨갰다(2026-08-25: 테스트 표의 CGNAT
  대역 주소 하나, 그리고 그것을 인용해 이 규칙을 적은 문장 자체). 픽스처의
  "원격 주소"는 TEST-NET(`192.0.2.x`/`198.51.100.x`)을 쓰고, 걸린 주소를
  문서에 다시 적지 마라 — **스캐너는 편집이 끝난 뒤에 돌린다.**
- **`web/`·`e2e/`·i18n 카탈로그·`examples/demo.db`를 건드렸으면 Playwright는
  선택이 아니다.** "영향 게이트만" 판단이 e2e를 건너뛰는 것이 실제 사고
  경로였다 (2026-08-16: 온보딩 카피 변경이 e2e 기대값을 낡게 만들었고,
  로컬은 go·typecheck·vitest·doc-checks 전부 초록이었다. 2026-08-21:
  fixture 재생성이 item_refs를 비웠는데 go 전체·doc-checks가 초록이라
  cross-links e2e 빨강을 CI에서야 봤다 — e2e가 읽는 fixture도 e2e의 일부다).
- **푸시는 끝이 아니다 — CI 초록이 끝이다.** 푸시 직후
  `tools/ci-status.sh`(HEAD의 런을 기다려 결론을 내고, 빨간 상태 위에
  쌓았으면 그것도 알려준다). 라운드 완료 보고에 그 결과를 쓴다.
- **PR은 로컬에서 못 도는 잡에 걸리는 변경만** (사용자 결정 2026-08-19).
  리뷰어가 없으므로 PR이 사는 것은 리뷰가 아니라 "CI 평결이 main이 아니라
  브랜치에 떨어진다" 하나뿐이다. 그 값이 실제로 나오는 것은 로컬 게이트가
  대신할 수 없는 잡뿐이다 — `desktop/`, `.github/workflows/`, 팩 스크립트
  (Desktop Windows build는 `windows-latest`, Desktop Linux build는 GTK4·
  WebKitGTK·AppImage). **그 외에는 로컬 게이트가 전부 초록이면 main에 직접
  푸시**하고 `tools/ci-status.sh`로 확인한다. 기본값은 직접이다.
- **PR 두 개 이상이 동시에 열려 있으면 리베이스는 손으로 하지 않는다** —
  `tools/rebase-pr.sh <branch>`. main에 뭘 올릴 때마다 열린 PR 전부가 뒤로
  밀리고, 충돌은 매번 같은 두 곳이다: CHANGELOG 참조 링크 꼬리(양쪽이 다
  맞으니 둘 다 유지)와 `examples/backlog-snapshot.tar.gz`(바이트는 머지하지
  않고 재생성). 그 둘 밖의 충돌은 진짜 충돌이라 스크립트가 exit 2로 멈춘다.
  푸시는 스크립트가 하지 않는다.
- 로컬 Node는 CI와 같아야 한다 — 버전의 단일 소유자는 `.nvmrc`(`nvm use`).
  로컬 24/CI 20 격차가 결함 하나를 여러 푸시 동안 숨긴 적이 있다(GDK-57).
- **의존성을 하나라도 건드렸으면 `bash tools/check-lockfile-platforms.sh`도
  게이트다.** macOS에서 `npm install`이 lockfile을 재해결하면 npm 10은
  darwin-arm64 것만 남기고 다른 플랫폼의 optional 네이티브 바이너리를 전부
  지운다(2026-08-26: `@rollup/rollup-*` 75→26, `@esbuild/*` 78→27). 로컬은
  전부 초록이다 — 없어진 게 이 머신이 안 쓰는 것들이라서. CI는 linux-x64라
  `vite build`가 두 잡에서 죽었다. **고칠 때 lockfile을 지우고 재설치하지
  마라** — 그게 애초에 잘라먹은 경로다. 플랫폼이 살아 있던 마지막 lockfile을
  복원한 뒤(`git show <ref>:package-lock.json > package-lock.json`)
  `npm install --package-lock-only`로 바뀐 의존성만 갱신한다.
- 데모 fixture는 `examples/demo.db`(이슈 534). 수치를 문서에 박을 때는
  실측 후, 가능하면 숫자 자체를 빼라.
- **미러 스키마 마이그레이션(schemaVNN)을 추가한 커밋은 `make demo-fixture`가
  게이트다** — e2e serve가 fixture의 user_version 불일치를 거절해서, go 전체
  초록인 채 CI e2e만 빨갛다(2026-08-31: schemaV40이 정확히 이 경로로 074c9dcd
  를 적색으로 만들었다 — 로컬에서 스키마 커밋에 Playwright를 건너뛴 것이 원인).
  재생성은 항상 make 타깃으로(스크럽 생략 금지), 재생성 뒤 Playwright 전체.

## 배포·이름

- 커밋·태그·푸시·릴리스는 **리드 세션 전용**. **main 푸시는 매번 묻지
  않는다** (사용자 지시 2026-08-26 "묻지말고 푸시해도 괜찮아") — 게이트가
  전부 초록이면 그대로 올리고 `tools/ci-status.sh`로 확인한다. 승인이
  여전히 필요한 것은 **태그·릴리스 게시**와 공개 스토어 제출이다.
- **`~/repo/issuetap` 도 같은 권한이다** (사용자 지시 2026-09-09 "이수탭도
  편하게 작업해"). gadak origin 을 고치는 일은 gadak 작업의 일부이고, 그
  레포의 게이트(`go vet` · `go test ./...` · `scripts/secretscan.sh` ·
  건드린 파일의 `gofmt`)가 초록이면 묻지 않고 main 에 올린다. 커밋 신원은
  `~/repo` 아래라 midagedev 로 자동이지만, 공개 푸시 전에 한 번 확인한다
  (`git log -1 --format='%ae'`). 그 뒤 gadak 의 `go.mod` 핀을 올리는 것은
  **별개 커밋**이다 — 핀 범프는 `desktop/go.mod` 까지 게이트다.
- brew: `gadak` = **macOS 앱 cask**(CLI 포함, v0.14부터 tap에 게시),
  `gadak-cli` = CLI formula(리눅스 포함). 문서의 설치 명령은 태그와 동시
  교체.
- **폰 앱(mobile/)의 TestFlight 내부 배포는 한 줄이다**:
  `cd mobile && scripts/testflight-upload.sh --bump` — 게이트 → `tauri ios
  build` → **`.ipa` 계약 검증 8항목** → `altool` 업로드 → 처리 대기 →
  `artifacts/app-store/`에 영수증. 계정 소유자 웹 단계(그룹 배정·계약 갱신
  동의)와 rust 툴체인 함정은 `docs/runbooks/testflight-release.md`가 정본.
  자격증명은 `~/.appstoreconnect`(레포 밖). 공개 스토어 제출은 여전히 별개
  판단이고 리뷰어 데모 경로가 선행 조건이다(GDK-805).
- 에이전트 온보딩은 **skill-first**: 셸 있는 호스트는 `gadak skill install
  [client]` — claude(기본)·codex·agents(`~/.agents/skills`, agentskills 계열
  전부)·cursor·gemini·opencode·grok, 전부 같은 SKILL.md가 호스트별 경로에
  들어간다(GDK-1508; Codex는 그 파일을 그대로 읽는 것을 실측). MCP(`gadak
  mcp install <client>`)는 셸 없는 호스트(Claude Desktop)용.
- **리드 세션도 skill-first다**: gadak CLI를 만지기 전에 `Skill(gadak)`을
  로드한다 — 동사 추측 금지 (2026-08-27 실측: 스킬이 current로 설치돼
  있는데 리드가 안 읽고 view→show→get을 연속 추측, 셋 다 실패. 정답은
  `gadak issue`. 에러 개선은 GDK-1015).
- `make media`는 `media-mcp`를 포함하지 않는다 — mcp 클립은 Claude 로그인과
  실모델 호출이 필요해서 기여자에게 강제하지 않는다 (`docs/project/MEDIA.md`).
- **업스트림 PR은 `docs/runbooks/upstream-pr.md`의 파이프라인을 통과한 뒤에만 제출한다**
  — 사전조사(중복·분류·머지 선례) → 실측 FAIL-first(코드 리딩만으로 확정한
  결함은 가설이다) → 형제 구현 패리티 패치 → 제출 전 적대적 리뷰(예상 지적을
  고치거나 본문 한 문장으로 선제) → 정직한 검증 경계. 근거 사례: wails#6000
  (봇 지적 1건, 답변 1회로 철회) / dock-reopen (실측이 비버그를 제출 전에 잡음).
- **마이너 버전 태그 전에 전체 코드 감사를 1회 돈다** (사용자 지시
  2026-08-16). 절차·축·이슈 등록 방식은 `docs/runbooks/release-audit.md`.
  결과는 GDK에 부모 이슈(`quality` 라벨) + 하위 이슈로 등록한다.

## 백로그·전략 문서 (도그푸딩)

- **새 요청은 바로 구현하지 않는다 — 조사 → 우선순위 → GDK 등록이 먼저다**
  (사용자 지시 2026-08-15). 사용자가 아이디어·불만·"이거 어떨까"를 꺼내면
  그 턴의 산출물은 코드가 아니라 ① 근거 조사(기본값 grok — 사용자 지시
  2026-08-17) ② 비용·효과
  우선순위 판정 ③ GDK 이슈 등록이다. 예외는 둘 — 사용자가 "지금 해"라고
  명시하거나, 조사할 것이 없는 자명한 한 줄 수정. 예외로 건너뛸 때는 그
  이유를 보고에 한 줄 적는다.
- **버그 수정이 항상 최우선이다** (사용자 지시 2026-08-15). 결함은 기능·
  문서·마케팅·리팩토링보다 먼저 잡는다. 마감이 걸린 비결함 작업도 결함을
  앞서지 않는다. 새 결함은 발견 즉시 GDK에 등록하고 Highest로 연다.
- **버그 수정은 모아 두지 않고 바로 내보낸다** (사용자 지시 2026-08-15).
  게이트가 초록이면 그 자리에서 main에 올린다 — 기능 작업이 끝나기를
  기다리거나 다음 릴리스까지 묶어 두지 않는다. 릴리스 태그는 별개 결정이다.
- **착수 순서는 대화 순서가 아니라 GDK 우선순위 순서다.** 라운드를 열기
  전에 `gadak --workspace gdk sql`로 우선순위 상위를 확인하고 거기서 고른다
  (`priority_rank` — display name으로 키하지 말 것). 방금 대화에서 나왔다는
  이유로 순번을 앞당기지 않는다.
- **백로그 원본은 셀프호스트 트래커의 `GDK` 프로젝트**다 (2026-09-01
  컷오버, GDK-1262). 집 serve에 페어링된 `gdk` 워크스페이스가 그 origin이고,
  읽기·쓰기 모두 `gadak --workspace gdk`로 간다. **Jira(`--profile oss`)는
  컷오버 이전 기록의 읽기 전용 보존본**이다 — `frozen: true`라 동기화되지
  않고, 거기에 새로 쓰지 않는다. 세션 태스크 리스트는 이번 세션의 실행
  단위일 뿐 — 세션을 넘길 백로그는 GDK에 이슈로 등록한다. 조회는
  gadak으로(도그푸딩): `gadak --workspace gdk sql "..."`.
- 홍보 전략·멘토 보고서·벤치 원자료는 컷오버 때 위키 페이지로 함께
  이전됐고, `gdk` 워크스페이스 미러에도 들어온다 (홈 origin의 `GDK`
  스페이스 — 페어링 위키 동기화는 GDK-1276으로 고쳤다, 2026-09-02).
  Confluence `GDK` 스페이스는 컷오버 이전 기록의 보존본이고 `oss`는
  frozen이라 그 미러는 멈춰 있다 — 컷오버 이전 Confluence 원본이 꼭
  필요할 때만 `gadak --profile oss config set frozen false`로 잠시 풀고
  동기화한 뒤 다시 얼린다.
  비공개 전략 문서를 공개 레포(scratch/ 포함)에 새로 만들지 않는다.
- **ko·ja 클립 녹화는 다국어 검수 라운드다** (사용자 지시 2026-09-07 "영상찍을때
  이런 부분 잘 제품보완을 병행해줘"). 번역된 픽스처 위에서 제품 전체가 한 언어로
  보이는 첫 자리가 녹화라서, 거기서 드러난 것 — 미번역 잔존, CJK 잘림·줄바꿈,
  날짜·기간 단위, 번역투 UI 문구, 복제 데이터의 반복 — 은 클립을 게시하고
  메모로 남기는 것이 아니라 그 세션에서 GDK 등록·수정·재녹화까지 간다. 리드가
  프레임 시트를 직접 보고 opus 비전 판정 1회를 같은 축으로 병행한다.
  **"의도된 것이라 결함 아님"으로 분류된 항목은 통과시키지 않고 사용자에게
  목록으로 보고한다** — 근거가 upstream 결정·과거 실측·"Jira도 그렇다"류이면
  더욱. 2026-09-08 GDK-1596: 첫 ko 히어로 라운드가 영어 우선순위 이름을 보고
  issuetap의 Cloud 충실도 주석을 인용해 "on purpose"로 적었고 리드도 받아들여,
  같은 날 재녹화 비전 판정에서 다시 나왔다. 분류를 바꾸는 권한은 사용자에게 있다.
- GitHub Issues는 **사용자 인바운드 전용** — 들어오면 GDK로 미러.
- 백로그 작업 중 걸리는 write 격차는 그 자리에서 GDK에 `write-gap`
  라벨로 등록한다.
- **도그푸딩 중 걸린 마찰은 전부 제품 개선 기회다** (사용자 지시
  2026-08-19). gadak으로 gadak을 운영하다 손이 한 번 더 가면 — 컬럼이 없어
  파서를 손으로 쓰게 되거나, 읽기가 준 식별자를 쓰기가 거절하거나, 같은
  줄이 두 번 찍히거나 — 그것을 우회하고 넘어가지 말고 GDK에 등록한다.
  결함이 아니어도 등록한다: 그 마찰은 에이전트 사용자가 매번 겪는 것이고,
  이 레포는 그것을 관측할 수 있는 유일한 자리다. 등록 시 **실측 재현**(내가
  실제로 실행한 명령과 출력)과 이미 존재하는 자산(예: 데이터는 있고 뷰가
  안 꺼내는 경우)을 본문에 넣는다 — 그것이 비용 판정을 바꾼다.

## 문서

- **README 셋은 번역이 아니라 병렬 판본이다** (사용자 지시 2026-09-08, GDK-1601).
  공유하는 것은 **사실**이고 그 원본은 `docs/project/FACT_LEDGER.md` 하나다 —
  문장·문단 순서·헤딩·분량은 판본마다 다르다. 판본을 고칠 때 다른 판본을
  보고 맞추지 말고 원장을 보고 쓴다. 원장의 수치·명령·링크·버전 문자열은
  계약이고, 어느 조항이 `tools/doc-checks.sh`에 걸리는지는 원장이 절마다
  표시한다. **사실이 바뀌면 원장을 먼저 고치고 세 판본을 각각 갱신한다.**
  판본별 독자·목표는 GDK-1601 의 하위 이슈 본문에 있다: ko는 트위터 유입에
  1인칭, en은 HN·GitHub 에 제품 화자, ja는 Qiita·Zenn 검색 유입에 보안 섹션
  독립.
- 세 판본이 문단 수·헤딩 수열까지 같아지면 그게 번역으로 읽히는 원인이다
  (2026-09-08 실측: 셋 다 문단 53·헤딩 9). 같아지지 않게 유지한다.
- **한국어·일본어 산문을 GLM에 위임하지 않는다** (사용자 지시 2026-09-05
  "한글은 GLM이 정말 못 써"). 근거는 GLM의 한국어 품질이므로 **Claude 계열
  서브에이전트(Fable 등)는 이 금지에 걸리지 않는다** — 2026-09-08 세 판본
  재작성이 그렇게 돌았다. 어느 쪽이든 리드가 diff를 직접 읽고, 원장에 없는
  주장이 새로 들어왔으면 코드·docs로 하나씩 검증한다(그 라운드에서 일본어판이
  아웃바운드를 6곳에서 4종으로 줄여 쓴 것을 이 검증이 잡았다).
- 일본어는 리드가 최종 판정자가 아니다 — **네이티브 검수 1회**가 남은 관문이고
  섭외 경로는 미정이다.
- **로컬 사본의 이름은 "캐시"다 — "미러"가 아니다** (사용자 결정 2026-09-08).
  사용자가 읽는 ko·ja 산문(README·사이트 카피·설치 페이지)에 미러/ミラー를
  쓰지 않는다. 캐시라는 말이 빠른 이유와 지워도 되는 이유를 다 설명하므로
  "원본은 Jira다"를 반복하지 않고(있어도 한 번), 구현 상세(SQLite 파일 하나·
  이 컴퓨터·디스크)는 문서마다 **한 자리에서 한 번**만 말한다. 영어판의
  `mirror`, `docs/MIRROR.md` 같은 파일명, 코드 식별자, CLI 출력은 그대로다.
- **사용자가 읽는 한국어는 짧게, 목소리 흉내 없이, 일화는 사용자가 준 것만**
  (사용자 지시 2026-09-08). 길이를 줄이면 어색함도 같이 줄었다. 트윗 문체를
  모사하거나 리드가 저자의 경험을 지어내지 않는다 — README.ko 도입부의
  rate-limit 일화는 사용자가 구술한 것이다.
- **사이트에 에세이는 없다** (사용자 결정 2026-09-08, `site/essays/` 전부 제거).
  저자가 손으로 쓰지 않은 장문은 AI 티가 나서 신뢰를 깎는다. 장문 산문을 새로
  만들자고 제안하지 않는다 — 사용자가 직접 쓴 글이 오면 그때 게재 경로를 다시
  만든다.
- MCP 툴 서술: `gadak_search`의 주 인자는 `query`(별칭 `text`/`q`).
  `{text: string}`을 주 인자로 쓰는 서술을 새로 만들지 마라.
- 마케팅 수치 주장(속도 등)은 공개 벤치 근거가 생기기 전에는 리드에 두지
  않는다.
