# gadak — 세션·에이전트 공통 계약

현재 유효한 규칙만 담는다. 히스토리·결정 경위는 `docs/decisions/`와
CHANGELOG의 몫. 여기 없는 도메인 지식은 `docs/project/STATE_OF_PLAY.md`(현황·
hard-won 목록)와 `AGENTS.md`(스키마·쿼리)가 원본이다.

## 제품 불변 조건 (깨면 제품이 아니다)

- **미러는 버려도 되는 캐시.** 원본은 항상 Jira — 그 Jira가 Atlassian
  Cloud든, gadak이 함께 들고 다니는 아주 미니멀한 셀프호스트 Jira
  (`issuetap`, standalone 워크스페이스)든. 어느 쪽이든 미러는 origin에서 다시
  만들 수 있고, **gadak 자신은 원본을 보관하지 않는다.** gadak에만 존재하는
  원본 데이터를 만드는 변경은 금지 — 예외는 `local.db`(방문·검색 기록)와
  저장된 뷰이고, 그것들은 export 가능해야 한다.
- **영속은 origin의 몫이다.** standalone 워크스페이스의 영속 공간은
  issuetap의 persist 파일이며 미러가 아니다. 그래서 백업 대상은 `gadak.db`가
  아니라 그 파일이고, **워크스페이스는 origin 하나에 묶인다** — origin을
  바꾸는 것은 설정 편집이 아니라 새 워크스페이스다. 이 조항을 어기는 것이
  "다른 트래커를 조용히 가리키게 하는" 부류의 결함이다.
- **쓰기는 전부 origin(Jira)을 통과**한 뒤 미러 갱신. 미러에 직접 쓰는 API를
  열지 않는다. 위키도 같은 규칙이다 — 페이지 쓰기(생성·편집·코멘트,
  GDK-380/381/382)는 origin.Wiki를 통과한다: connected는 Confluence REST,
  standalone은 issuetap의 Confluence API(미러 직접 쓰기가 아니다).
- **아웃바운드 없음.** 텔레메트리 금지. 나가는 요청은 사용자가 설정한
  origin(Atlassian 사이트·Linear: api.linear.app/uploads.linear.app), GitHub
  릴리스 버전 체크(설정으로 끔), 페어링한 home serve, loopback뿐
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
  check·lint:ios가 전부 초록인 채로 CI에서 8개가 빨갰다). 느리다(~12분)
  — 커밋 직전 1회.
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
- 문서 사실성 가드: `tools/doc-checks.sh` (있으면 커밋 전 실행).
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

## 배포·이름

- 커밋·태그·푸시·릴리스는 **리드 세션 전용**. **main 푸시는 매번 묻지
  않는다** (사용자 지시 2026-08-26 "묻지말고 푸시해도 괜찮아") — 게이트가
  전부 초록이면 그대로 올리고 `tools/ci-status.sh`로 확인한다. 승인이
  여전히 필요한 것은 **태그·릴리스 게시**와 공개 스토어 제출이다.
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
- 에이전트 온보딩은 **skill-first**: 셸 있는 호스트는 `gadak skill install`,
  MCP(`gadak mcp install claude`)는 셸 없는 호스트(Claude Desktop)용.
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
  전에 `gadak --profile oss sql`로 우선순위 상위를 확인하고 거기서 고른다
  (`priority_rank` — display name으로 키하지 말 것). 방금 대화에서 나왔다는
  이유로 순번을 앞당기지 않는다.
- **백로그 원본은 Jira `GDK` 프로젝트** (midagedev 개인 사이트, `gadak
  --profile oss`). 세션 태스크 리스트는 이번 세션의 실행 단위일 뿐 —
  세션을 넘길 백로그는 GDK에 이슈로 등록한다. 조회는 gadak으로(도그푸딩):
  `gadak --profile oss sql "..."`.
- 홍보 전략·멘토 보고서·벤치 원자료는 **Confluence `GDK` 스페이스**.
  비공개 전략 문서를 공개 레포(scratch/ 포함)에 새로 만들지 않는다.
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

- **영문 README가 원본**, `README.ko.md`는 번역(헤더에 기준 버전 명시) —
  영문을 고치면 ko 갱신 여부를 확인.
- MCP 툴 서술: `gadak_search`의 주 인자는 `query`(별칭 `text`/`q`).
  `{text: string}`을 주 인자로 쓰는 서술을 새로 만들지 마라.
- 마케팅 수치 주장(속도 등)은 공개 벤치 근거가 생기기 전에는 리드에 두지
  않는다.
