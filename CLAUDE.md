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
- **상태는 display name으로 필터하지 않는다.** `status = 'In Progress'`는
  한국어 계정에서 소리 없이 0행이다. 항상 `status_category`
  (new|inprogress|done) 또는 `status_id`. 이 함정은 코드·문서·툴 설명
  어디서든 재발 금지.
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

## 문서

- **영문 README가 원본**, `README.ko.md`는 번역(헤더에 기준 버전 명시) —
  영문을 고치면 ko 갱신 여부를 확인.
- MCP 툴 서술: `gadak_search`의 주 인자는 `query`(별칭 `text`/`q`).
  `{text: string}`을 주 인자로 쓰는 서술을 새로 만들지 마라.
- 마케팅 수치 주장(속도 등)은 공개 벤치 근거가 생기기 전에는 리드에 두지
  않는다.
