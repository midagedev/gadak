<!--
.outsource/overlay.md — gadak 프로젝트 오버레이 (outsource 스킬이 spec 조립 때 포함).
레포 계약의 원본은 CLAUDE.md — 여기는 위임 라운드에 필요한 발췌와 위임 고유 규칙만.
-->

# Project overlay — gadak

## 레포 좌표

- base 브랜치 = `main`. 위임 전 `git fetch`, spec에 기준 SHA를 박는다.
- 워크트리: `git worktree add` + **`node_modules`와 `web/node_modules` 심링크**(재설치 금지).
- Node 버전의 단일 소유자는 `.nvmrc` — `npm install`을 돌리게 하지 말 것.

## 게이트 레시피 (완료 기준에 넣을 실행 명령, 전부 레포 루트에서)

| 대상 | 명령 |
|---|---|
| Go | `go build ./... && go vet ./... && go test ./... -count=1` && `gofmt -l`(빈 출력 — CI에 gofmt 잡) |
| 웹 타입체크 | `make typecheck` (svelte-check) |
| 웹 유닛 | `npx vitest run` |
| **mobile 모듈** | **i18n 카탈로그나 `web/src/lib/terminal/`을 건드렸으면** `cd mobile && npm test && npm run check && npm run lint:ios` — 폰 앱은 자기 tsconfig·자기 lockfile을 갖고 `web/src`에서 일부 모듈을 직접 import한다. 루트 `make typecheck`는 폰을 보지 않는다 (2026-08-26: 웹에서 지운 i18n 키를 `mobile/src/screens/Shell.svelte`가 여전히 써서, 로컬 전부 초록인데 Mobile 잡만 빨갰다) |
| e2e (CI 세트) | `npx playwright test --config e2e/playwright.config.ts` — **web/·e2e/·i18n 카탈로그를 건드렸으면 선택이 아니다** |
| 문서 사실성 | `bash tools/doc-checks.sh` |
| 내부 문자열 | `bash scripts/scan-internal.sh` — IP 리터럴·호스트명·홈 경로가 들어가는 커밋이면 게이트다(CI와 같은 스크립트). **마지막 편집이 끝난 뒤에 돌린다** |
| desktop 모듈 | 루트 Go에 **서드파티 import가 새로 생겼으면** `cd desktop && go mod tidy && go build ./...` (별도 go.mod — GDK-635에서 desktop CI 3잡이 빨갰다) |
| lockfile 플랫폼 | 의존성을 하나라도 건드렸으면 `bash tools/check-lockfile-platforms.sh` |

- **게이트 출력을 `tail`/`head`로 파이프하지 말 것** — 파이프라인 exit이 pager 것이 된다(실사고 2회).
  파일로 캡처하고 exit을 직접 확인한다.
- **e2e 직렬화의 실체는 락 파일이 아니라 포트다** (2026-08-23 실측 교정 — 종전 이 문단에 있던
  "저장소 전역 락 파일·타임아웃 600s"는 타 레포 규칙의 오복사였고, 그런 파일은 이 레포에
  존재하지 않는다). serve가 `127.0.0.1:7877`에 붙고(`e2e/helpers.ts`), 홈은 단일
  `e2e/.tmp/home`, Playwright는 `workers: 1`. 그래서 트리 둘이 동시에 e2e를 못 돌리고,
  **경쟁 신호는 포트 충돌과 낡은 서버 재사용**이다(스탬프 불일치는 `assertServedArtifact`가 잡는다).
  포트는 `GADAK_E2E_PORT`로 옮길 수 있지만 홈 격리는 아직 아니다(GDK-672).
  포트 충돌을 만나면 그것은 다른 프로세스와의 경쟁이지 네 변경의 실패가 아니다 — 보고하고 1회 재시도.
- **Playwright 스위트가 도는 동안 같은 트리에서 무거운 게이트(go test 전체 등)를 병렬로
  돌리지 말 것** — serve 프로세스가 조용히 죽어(SIGKILL형, 로그 무출력) 대량 적녹을 만든다
  (2026-08-24 dash-web 라운드 실측: 병렬 실행 203 빨강 → 직렬 재실행 317 전부 초록).

## 도메인 함정 (위임 결과를 믿기 전에)

- **상태·우선순위·이슈 유형을 display name으로 키하지 않는다.** `status='In Progress'`는 한국어
  계정에서 소리 없이 0행. 항상 `status_category`/`status_id`, `priority_rank`/`priority_id`,
  `issue_type_id`. 이 함정은 코드·문서·툴 설명 어디서든 재발 금지.
- **쓰기는 전부 origin을 통과** 후 미러 갱신. 미러에 직접 쓰는 API를 열지 않는다.
- `time-in-status`는 저장 컬럼이 아니다 — `status_changed_at`에서 계산.
- decisions/ 문서는 개정하지 않는다(Addendum만). CHANGELOG는 소급 수정 금지.
- Go 1.19+ gofmt가 doc comment의 `''`를 `”`로 정규화한다 — 회피는 문구 재작성.
- **수명주기에 훅을 걸었으면 "제품이 실제로 타는 경로"로 테스트했는지 확인하라.**
  이 레포는 같은 자원에 진입점이 여러 개다 — 워크스페이스를 여는 길은
  `origin.Client`(CLI)와 `origin.StandaloneHandler`(앱·serve) 둘이고, 닫는 길은
  `origin.Close`(프로세스 종료)와 `origin.CloseStandalone`(워크스페이스 하나)
  둘이다. 2026-08-26 GDK-971: 정리 훅을 `CloseStandalone`에만 걸었는데 CLI가
  실제로 타는 건 `Close`라, 모든 명령이 죽은 PID의 마커를 흘렸다. 유닛 테스트는
  전부 초록이었다 — 죄다 `CloseStandalone`을 직접 불렀기 때문이다. **훅마다
  "이 경로로 들어오는 실제 호출자가 누구인가"를 grep하고, 각 호출자별 게이트를
  하나씩 둬라.**

## 보안 (위임 라운드 절대 규칙)

- **LINEAR_API_KEY 값을 spec·로그·커밋·픽스처에 넣지 않는다.** 경로(`~/.config/gadak-dev/linear.env`)와
  변수명만 언급 가능. 라이브 Linear/Jira 호출은 리드 전용 — 위임 라운드는 httptest로 고정한다.
- 아웃바운드 없음: 새 외부 요청을 추가하는 변경은 그 자체로 스펙 위반. 텔레메트리 금지.
- **토큰·페어링 오퍼 값을 화면·로그·에러·보고 어디에도 찍지 않는다** — 접두사도, 길이도,
  해시도 아니다. 페어링 스코프가 생긴 뒤 이 규칙은 셸을 여는 권한에 걸려 있다.
- **픽스처의 "원격 주소"는 TEST-NET을 쓴다** (`192.0.2.x` / `198.51.100.x`).
  실제 대역 주소 하나가 `scripts/scan-internal.sh`에 걸려 CI를 두 번 연속 세웠다
  (2026-08-25 — 표에 적은 주소 하나, 그리고 그것을 인용해 규칙을 적은 문장 자체).
- **테스트 픽스처의 이슈 키에 `GDK-*`를 쓰지 마라** — `tools/doc-checks.sh`의
  공개 백로그 인용 게이트가 테스트 파일도 훑어서, 비공개 키 하나가 CI를
  빨갛게 만든다(2026-08-25 실사고 — 비공개 5번 키 하나가 CI를 세웠다).
  픽스처는 `STD-*`/`NMB-*` 같은
  스캐너가 해석하지 않는 프로젝트 키로.

## 커밋·푸시

- 커밋·푸시는 리드 전용. 위임 라운드는 워크트리에 편집만 남긴다(strict 프로파일).
- 이슈당 커밋 하나가 기본. 커밋 메시지에 GDK 키.
