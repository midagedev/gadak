# gadak — 세션·에이전트 공통 계약

현재 유효한 규칙만 담는다. 히스토리·결정 경위는 `docs/decisions/`와
CHANGELOG의 몫. 여기 없는 도메인 지식은 `docs/STATE_OF_PLAY.md`(현황·
hard-won 목록)와 `AGENTS.md`(스키마·쿼리)가 원본이다.

## 제품 불변 조건 (깨면 제품이 아니다)

- **미러는 버려도 되는 캐시.** 원본은 항상 Jira. 로컬에만 존재하는 원본
  데이터를 만드는 변경은 금지 — 유일한 예외는 `local.db`(방문·검색 기록)와
  저장된 뷰이고, 그것들은 export 가능해야 한다.
- **쓰기는 전부 Jira를 통과**한 뒤 미러 갱신. 미러에 직접 쓰는 API를 열지
  않는다. 위키 미러는 읽기 전용.
- **아웃바운드 없음.** 텔레메트리 금지. 나가는 요청은 사용자의 Atlassian
  사이트, GitHub 릴리스 버전 체크(설정으로 끔), loopback뿐 (`SECURITY.md`).
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
- CHANGELOG는 히스토리 — 소급 수정 금지.

## 빌드·게이트

- Go: `go build ./...` · `go test ./... -count=1` · `go vet ./...`
- 웹: `make typecheck` (svelte-check). e2e: Playwright, CI 세트는
  `e2e/*.spec.ts`(demo/·hosted/·perf/ 제외 — `e2e/playwright.config.ts`).
- **브라우저 검증 락은 저장소 전역 파일 하나**(타임아웃 600s).
  `lock timed out`은 경쟁 신호이지 게이트 실패가 아니다.
- 게이트 단언 완화는 ①귀속 주석 ②정당한 파생 ③FAIL-first 증거 셋 모두
  있을 때만.
- 문서 사실성 가드: `tools/doc-checks.sh` (있으면 커밋 전 실행).
- **`web/`·`e2e/`·i18n 카탈로그를 건드렸으면 Playwright는 선택이 아니다.**
  "영향 게이트만" 판단이 e2e를 건너뛰는 것이 실제 사고 경로였다
  (2026-08-16: 온보딩 카피 변경이 e2e 기대값을 낡게 만들었고, 로컬은
  go·typecheck·vitest·doc-checks 전부 초록이었다).
- **푸시는 끝이 아니다 — CI 초록이 끝이다.** 푸시 직후
  `tools/ci-status.sh`(HEAD의 런을 기다려 결론을 내고, 빨간 상태 위에
  쌓았으면 그것도 알려준다). 라운드 완료 보고에 그 결과를 쓴다.
- 로컬 Node는 CI와 같아야 한다 — 버전의 단일 소유자는 `.nvmrc`(`nvm use`).
  로컬 24/CI 20 격차가 결함 하나를 여러 푸시 동안 숨긴 적이 있다(GDK-57).
- 데모 fixture는 `examples/demo.db`(이슈 534). 수치를 문서에 박을 때는
  실측 후, 가능하면 숫자 자체를 빼라.

## 배포·이름

- 커밋·태그·푸시·릴리스는 **리드 세션 전용**, main 푸시는 사용자 승인.
- brew: `gadak` = **macOS 앱 cask**(CLI 포함, v0.14부터 tap에 게시),
  `gadak-cli` = CLI formula(리눅스 포함). 문서의 설치 명령은 태그와 동시
  교체.
- 에이전트 온보딩은 **skill-first**: 셸 있는 호스트는 `gadak skill install`,
  MCP(`gadak mcp install claude`)는 셸 없는 호스트(Claude Desktop)용.
- `make media`는 `media-mcp`를 포함하지 않는다 — mcp 클립은 Claude 로그인과
  실모델 호출이 필요해서 기여자에게 강제하지 않는다 (`docs/MEDIA.md`).

## 백로그·전략 문서 (도그푸딩)

- **새 요청은 바로 구현하지 않는다 — 조사 → 우선순위 → GDK 등록이 먼저다**
  (사용자 지시 2026-08-15). 사용자가 아이디어·불만·"이거 어떨까"를 꺼내면
  그 턴의 산출물은 코드가 아니라 ① 근거 조사(기본값 crush — 사용자 지시
  2026-08-16) ② 비용·효과
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

## 문서

- **영문 README가 원본**, `README.ko.md`는 번역(헤더에 기준 버전 명시) —
  영문을 고치면 ko 갱신 여부를 확인.
- MCP 툴 서술: `gadak_search`의 주 인자는 `query`(별칭 `text`/`q`).
  `{text: string}`을 주 인자로 쓰는 서술을 새로 만들지 마라.
- 마케팅 수치 주장(속도 등)은 공개 벤치 근거가 생기기 전에는 리드에 두지
  않는다.
