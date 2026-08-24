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
| e2e (CI 세트) | `npx playwright test --config e2e/playwright.config.ts` — **web/·e2e/·i18n 카탈로그를 건드렸으면 선택이 아니다** |
| 문서 사실성 | `bash tools/doc-checks.sh` |

- **게이트 출력을 `tail`/`head`로 파이프하지 말 것** — 파이프라인 exit이 pager 것이 된다(실사고 2회).
  파일로 캡처하고 exit을 직접 확인한다.
- 브라우저 검증 락은 저장소 전역 파일 하나(타임아웃 600s). `lock timed out`은 경쟁 신호이지
  게이트 실패가 아니다 — 재시도한다.

## 도메인 함정 (위임 결과를 믿기 전에)

- **상태·우선순위·이슈 유형을 display name으로 키하지 않는다.** `status='In Progress'`는 한국어
  계정에서 소리 없이 0행. 항상 `status_category`/`status_id`, `priority_rank`/`priority_id`,
  `issue_type_id`. 이 함정은 코드·문서·툴 설명 어디서든 재발 금지.
- **쓰기는 전부 origin을 통과** 후 미러 갱신. 미러에 직접 쓰는 API를 열지 않는다.
- `time-in-status`는 저장 컬럼이 아니다 — `status_changed_at`에서 계산.
- decisions/ 문서는 개정하지 않는다(Addendum만). CHANGELOG는 소급 수정 금지.
- Go 1.19+ gofmt가 doc comment의 `''`를 `”`로 정규화한다 — 회피는 문구 재작성.

## 보안 (위임 라운드 절대 규칙)

- **LINEAR_API_KEY 값을 spec·로그·커밋·픽스처에 넣지 않는다.** 경로(`~/.config/gadak-dev/linear.env`)와
  변수명만 언급 가능. 라이브 Linear/Jira 호출은 리드 전용 — 위임 라운드는 httptest로 고정한다.
- 아웃바운드 없음: 새 외부 요청을 추가하는 변경은 그 자체로 스펙 위반.

## 커밋·푸시

- 커밋·푸시는 리드 전용. 위임 라운드는 워크트리에 편집만 남긴다(strict 프로파일).
- 이슈당 커밋 하나가 기본. 커밋 메시지에 GDK 키.
