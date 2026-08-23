# Changelog

<sub><a href="CHANGELOG.md">English</a> · 한국어 — 영문이 원본이며, 번역은 영문과 함께 갱신됩니다(마지막 동기화 2026-08-23).</sub>

## Unreleased

에이전트의 쓰기가 성숙해진 사이클입니다. 이제 이슈가 자신을 구현하는 PR과
커밋을 보여주고, 쓰기 동사들이 코딩 에이전트가 실제로 보내는 어휘를 배웠고,
워크스페이스를 매 명령마다 다시 고르지 않아도 됩니다. 마지막은 네트워크 이음새·
MCP 표면·웹 UI를 훑는 릴리스 전 감사로 닫았습니다.

### 개발 패널

- 이슈가 자신의 PR·커밋·배포·빌드와 그 사람들을 압니다 — connected
  워크스페이스는 미러로, standalone은 쓰기까지 ([GDK-496], [GDK-497],
  [GDK-592], [GDK-589]).
- `gadak dev scan`이 저장소의 PR을 한 번에 개발 링크로 쓸어 담고,
  `gadak dev link`가 하나를 씁니다 ([GDK-531], [GDK-538], [GDK-539]).
- 웹은 미러된 PR을 링크된 PR로 그리고, GitHub 링크를 앱 안 패널에서 열고,
  패널이 비었으면 왜 비었는지 말합니다 ([GDK-495], [GDK-527], [GDK-555],
  [GDK-540]).
- 개발 링크가 다음 sync·fetch 오류·페어링된 원격의 증분 동기화를 살아남습니다
  ([GDK-536], [GDK-541], [GDK-562], [GDK-537]).
- 링크를 걸려고 앱을 떠날 이유가 없어졌습니다: `gadak link A B --type blocks`와
  상세 패널이 같은 경로로 이슈 링크를 씁니다 ([GDK-19], [GDK-85]).

### 에이전트가 믿을 수 있는 쓰기 동사

- `create`·`edit`가 프로젝트가 요구하는 커스텀 필드를 `--field alias=value`로
  받고, 생성 다이얼로그가 이 프로젝트·유형이 실제로 요구하는 것을 배우고,
  `issue --editmeta`가 origin에게 이 이슈에서 무엇을 편집할 수 있는지 묻습니다
  ([GDK-513], [GDK-254], [GDK-514]).
- `transition`이 `--resolution`·`--field`·`-m`을 싣고, `edit`가 fix version과
  컴포넌트를 이름으로 쓰면서 id를 지킵니다 ([GDK-509], [GDK-516], [GDK-517]).
- `assign`이 이메일 외에 이름·accountId도 받습니다 ([GDK-515]).
- 이슈 대량 읽기가 다중 키와 `--keys -`를 받고 조용히 빠뜨리지 않으며, REST가
  CLI에 이미 있던 parent 쌍을 갖게 됐습니다 ([GDK-425], [GDK-328]).
- 거절된 parent가 고를 수 있었던 에픽 이름을 댑니다 — 두 표면 모두
  ([GDK-330], [GDK-635]).
- `gadak claim KEY`가 이슈를 한 번에 내 것으로 만들고, `gadak issue`가 작업이
  얼마나 머물렀는지 보여줍니다 — `wait 3d · progress 5h`, 읽는 시점 계산
  ([GDK-591]).
- 타입이 틀린 쓰기 필드는 빈 문자열이 아니라 거절입니다 ([GDK-643]).
- 그 동사들 뒤의 정리: 공유 재시도 기본값, 삭제 전에 미참조를 증명한 데드코드,
  쓰기 후 재읽기의 단일 소유자, 못 하는 것을 스텁으로 때우지 않고 거절하는
  어댑터 ([GDK-644], [GDK-647], [GDK-642], [GDK-641]).
- 두 표면의 갈라짐을 멈췄습니다: CLI가 리치 페이지 본문을 문단 하나로 덮거나
  미러 밖에 이슈를 만드는 일이 없어지고, "이 본문이 리치인가"를 한 함수가
  답합니다 ([GDK-682], [GDK-666]).

### standalone, 그리고 origin 쓰기의 단일 어휘

- standalone 워크스페이스가 당신의 언어로 말하고, 제한된 이슈가 공개 이슈와
  구분됩니다 ([GDK-597], [GDK-519]).
- 쓰기가 저자를 싣습니다: `GADAK_ACTOR`, 이름 없는 에이전트의 읽을 수 있는
  이름, 웹에서 봇 작업을 표시하는 배지 ([GDK-586], [GDK-588], [GDK-593],
  [GDK-590]).
- `origin.Writer`가 자기 시그니처에서 Jira를 말하지 않게 되어, 한 표면에 동사를
  더하는 것이 같은 가드의 세 구현이 되지 않습니다 ([GDK-665]).
- 쓰기는 그 행을 소유한 origin으로 갑니다: 무조건 Jira 클라이언트를 만들던 HTTP
  핸들러 두 개가 라우팅되고, 다음 우회를 막는 AST 게이트가 생겼습니다
  ([GDK-681]).

### 워크스페이스와 페어링

- `gadak workspace use <name>`이 기본값을 저장해 매 명령마다 워크스페이스를 다시
  고르지 않아도 됩니다. `--workspace`가 이름이고 `--profile`은 별칭으로 남습니다
  ([GDK-490]).
- 페어링이 사실을 말합니다: 토큰이 origin 패스스루를 게이트하고, 상태 동사가
  paired가 무엇인지 배우고, 실패가 Jira 401로 재라벨되지 않습니다 ([GDK-433],
  [GDK-449], [GDK-453]).
- 묶인 워크스페이스는 조용히 다른 사이트로 재결속되지 않고, origin을 교체하면
  그 파생 행들이 함께 갑니다 ([GDK-452], [GDK-561], [GDK-418]).
- 마운트된 standalone 워크스페이스가 다시 이슈를 만들 수 있습니다 ([GDK-677],
  [GDK-678]).

### 미러의 스키마

- fix version이 id를 지키고 프로젝트의 릴리스 카탈로그가 미러에 들어옵니다.
  스프린트는 에이전트가 쿼리할 수 있는 컬럼입니다 ([GDK-532], [GDK-518]).
- 한국어 복합어 중간 검색이 됩니다: 네 번째 FTS 컬럼 `cjk_bigram`이 복합어
  안의 단어를 찾습니다 ([GDK-259], [GDK-444]).
- JQL `parent =` / `parent IN`이 미러 자신의 `parent_key`로 걸리고, 계층이
  미러 밖으로 나가는 길에서 살아남습니다 ([GDK-521], [GDK-329]).
- 개인 상태가 미러 밖으로 나가서, `rm gadak.db`가 뷰·방문·검색 기록을 건드리지
  않습니다 ([GDK-105]).

### 일관성을 갖춘 웹 UI

- 서로 맞춰야 했던 세 표면 대신 커맨드 레지스트리 하나, UI 문자열 하나당 모든
  로케일이 든 객체 하나 — 전체 `ja` 카탈로그 포함 ([GDK-674], [GDK-668],
  [GDK-626]).
- Esc가 겨눈 표면만 닫고 다른 것은 닫지 않습니다. 같은 부류의 모달 여섯 개는
  소유자가 하나입니다 ([GDK-617], [GDK-604], [GDK-316]).
- 타입 크기는 넷이고 그 사이는 없습니다. "여기 있다"는 칠한 색이 아니라
  속성입니다. 빈 것은 공백이 아니라 상태입니다 ([GDK-129], [GDK-613],
  [GDK-130]).
- 화면이 필요한 transition은 그 자리에서 묻고, 컴포넌트와 parent는 인라인으로
  편집되고, 스토리가 자식 이슈를 보여줍니다 ([GDK-83], [GDK-86], [GDK-121]).
- 저장 뷰는 한 종류입니다 — 팀이 없으니까요. Enter가 저장이고, 서버가 있으면
  서버가 소유하고, 이미 브라우저에 있던 뷰는 거기로 옮겨갑니다 ([GDK-437]).
- 빈 Documents 본문이 사이드바와 같은 여섯 원인을 읽어, 이미 켜져 있는 스위치를
  켜라고 하지 않습니다 ([GDK-738]).
- 즐겨찾기의 시각 칸에는 시각이 있거나 비어 있고, "All"은 동사가 아니라 탭
  필터입니다 ([GDK-739]).
- 데모 fixture가 스키마보다 조용히 뒤처질 수 없습니다 ([GDK-671]).

### MCP와 CLI 표면

- 대량 쓰기가 키별로 정직한 봉투를 답하고, 이슈를 닫는 것은 왕복 한 번이며
  재시도가 공짜입니다 ([GDK-501], [GDK-500]).
- 일을 고르는 것이 동사가 됐습니다: `gadak pick` ([GDK-503]).
- CLI 읽기가 흔적을 남기고 `gadak recents`가 그것을 되짚습니다 ([GDK-502]).
- MCP가 자기 미러를 신선하게 유지하고, `gadak sync --if-stale 15m`은 에이전트가
  묻지 않고 부를 수 있는 세션 오프너입니다 ([GDK-599], [GDK-598]).

### 데스크톱

- 두 번째 실행이 경쟁 프로세스를 띄우는 대신 첫 창을 올립니다 — standalone도
  포함 ([GDK-658]).
- Windows에서 `gadak://` 링크가 동작하고, `install-cli`가 Windows를 말하고,
  Ctrl+W가 못 하는 것을 약속하지 않고, Windows가 알림을 보냈다고 거짓말하지
  않습니다 ([GDK-350], [GDK-353], [GDK-351], [GDK-349]).
- wails 핀이 `v3.0.0-beta.12`로 올라갔습니다 ([GDK-639]).

### 감사한 네트워크 이음새

- Linear 프록시는 리다이렉트까지 포함해 `uploads.linear.app`만 가져오고, API
  키는 그곳으로만 갑니다 ([GDK-427], [GDK-558], [GDK-560]).
- 빈 호스트는 loopback이 아닌 바인드이므로, `serve`가 다른 노출과 똑같이
  `--allow-remote`를 요구합니다 ([GDK-542]).
- 부트 시퀀스의 소유자가 하나이고, `gadak serve`와 데스크톱 앱이 그것을
  공유합니다 ([GDK-664]).
- Linear 전용 워크스페이스도 설정된 워크스페이스이고, Linear의 rate limit은
  죽음이 아니라 재시도입니다 ([GDK-654], [GDK-263]).

### 공개 백로그, 그리고 gadak.dev의 현관

- gadak의 백로그가 공개되고, 한 파일로 배포되고, 각 이슈가 실제로 말하는 것을
  게시합니다 ([GDK-389], [GDK-669], [GDK-430]).
- 공개 표면의 GDK 키는 절대 끊어진 포인터가 아니고, 그 링크의 대상도 공개되어야
  하며, 게이트는 추적된 파일만이 아니라 디스크의 파일을 읽습니다 ([GDK-269],
  [GDK-675], [GDK-683]).
- 사이트에 apex가 생기고 그 뒤에 문이 셋입니다 — 랜딩·데모·백로그 ([GDK-676]).
- Windows 경고를 설명하는 페이지가 생겼습니다 ([GDK-211]).

### 게이트 아래에서

- e2e 리스닝 포트의 소유자가 하나가 되어, 워크트리 두 개가 브라우저 스위트를
  동시에 돌릴 수 있습니다 ([GDK-672]).
- hosted 사이트가 SPA를 한 번만 빌드합니다: `basePath()`가 컴파일타임 상수가
  아니라 런타임 마운트를 읽고, 게이트가 양쪽 에셋 트리를 해시합니다
  ([GDK-673]).
- e2e fixture는 신선하거나 아니거나입니다 — `local.db`도 미러와 함께 시드되어,
  저장 뷰가 다음 런까지 살아남지 않습니다 ([GDK-742]).
- 에이전트 온보딩은 skill-first이고, 이 저장소는 Claude Code 플러그인
  마켓플레이스입니다 ([GDK-8], [GDK-93]).

## v0.16.1 — 2026-08-20

0.16이 시작한 것을 끝내는 릴리스입니다. standalone은 동작하는 origin으로
출하됐고, 그 뒤 하루 동안 "누가 이 파일의 주인인가"를 두 프로세스가 다르게
답할 수 있는 경로를 차례로 드러냈습니다. Linear는 꽂을 자리가 없는 읽기 전용
클라이언트로 도착했고, 에이전트가 실제로 읽는 문서는 "standalone"이라는
단어를 한 번도 배우지 못했습니다. 세 가지 모두 여기서 닫힙니다.

### 세 번째 트래커, 그리고 쓰기까지

- Linear는 계획이 아니라 소스입니다: `"linear"` 블록과 `gadak sync --source
  linear`가 이슈·코멘트·라벨·첨부를 미러하고, 쓰기는 그 행의 origin으로
  라우팅되며, Linear에서 아직 못 하는 것은 정직하게 거절합니다
  ([GDK-263], [GDK-360], [GDK-361]).
- Jira, standalone, Linear가 하나의 쓰기 어휘를 말하고 ([GDK-359]), Linear만
  있는 프로필도 sync됩니다. 소스를 가로지르는 키 충돌이 Jira 쓰기를 거절하지
  않습니다 ([GDK-361], [GDK-263]).

### 위키가 읽기 전용을 벗습니다

- 페이지 편집·코멘트·생성이 origin을 통과하고, 페이지 id도 미러 id와 같은
  네임스페이스를 받습니다 ([GDK-380], [GDK-381], [GDK-382], [GDK-344]).

### standalone origin의 소유자는 하나

- 데스크톱 앱도 origin을 공표해서, 앱과 CLI가 persist 파일을 동시에 들 수
  없습니다 ([GDK-340], [GDK-333]).
- persist lock이 누가 임베드할 수 있는지의 단일 진실이고, 응답 전에 쓰기가
  디스크에 도달하며, 영속 실패는 쓰기를 실패시키고, 치명 경로에서도
  flush합니다 ([GDK-343], [GDK-342], [GDK-346], [GDK-348]).
- standalone 실패가 `credential_required`로 위장하지 않고, 전환 문구는 전환이
  실제로 무엇을 하는지 말합니다 ([GDK-345], [GDK-347]).
- 미러 id가 자기 네임스페이스를 갖고, 전환은 옛 미러를 통째로 버리며 watch와
  즐겨찾기를 함께 가져갑니다 ([GDK-241], [GDK-344]).

### 에이전트 표면이 standalone을 배웁니다

- 임베디드 스킬이 standalone의 존재를 알고, CLI가 어느 origin을 말하는지
  밝히며, `transition`이 각 target의 `status_id`를 알려 주고 읽기 경로가 주는
  값도 받습니다 ([GDK-239], [GDK-363], [GDK-364], [GDK-366], [GDK-371],
  [GDK-365], [GDK-313]).
- kind와 persist가 에이전트의 프리플라이트에 올라오고, standalone `init`이
  미러를 채우며, `issues_full`이 `description_text`를 꺼냅니다 ([GDK-368],
  [GDK-376], [GDK-367], [GDK-312]).

### 제품과 모순되기를 멈춘 문서

- `AGENTS.md`는 이제 이 저장소의 개발 계약이고, 제품 매뉴얼이 아닙니다
  ([GDK-8]).
- 네트워크가 자기 페이지를 얻고, 낡은 stale 경고 동사 목록이 `sql`도
  경고한다는 것을 배웁니다 ([GDK-601], [GDK-598]).
- 설치 앞문이 standalone을 인정하고, CONCEPT는 origin을 가르치며, FAQ는
  `rm -rf ~/.gadak`을 권하기를 멈추고, PROMISES는 아홉 번째 약속을 얻고,
  MAINTENANCE는 이미 출하한 Windows 셸을 거절하기를 멈추며, export/import는
  드디어 문단을 얻습니다. Go 테스트 스위트가 픽스처 자격증명을 실제
  Atlassian 호스트로 향하게 하던 것을 멈춥니다 ([GDK-271], [GDK-372],
  [GDK-373], [GDK-374], [GDK-375], [GDK-304]).

### 2차 감사, 그리고 이미 고친 것

- 태그 전에 코드베이스 전체를 6축으로 다시 훑었고, 감사가 잡은 결함 2건은
  당일 출고됐습니다 ([GDK-603], [GDK-602], [GDK-604]).
- SQL 주석 제거, 개인 디렉터리, 뷰 이름, `ray develop` 출력, 치명 오류의
  소유자가 각각 하나입니다 ([GDK-605], [GDK-606], [GDK-612], [GDK-615],
  [GDK-611]).
- 테스트가 거부당한 다이얼에 잠들지 않고, `gofmt`가 CI 게이트가 되고, 잘못된
  커서는 정체성이며, 이슈를 세는 일이 이슈를 다 불러오지 않고, 죽은 코드는
  호출자를 센 뒤에 지우며, 작은 헬퍼 사본 여섯이 소유자 하나로 접힙니다
  ([GDK-608], [GDK-607], [GDK-609], [GDK-610], [GDK-616], [GDK-619]).

### 태그 전에 닫은 릴리스 감사

- 이 릴리스 자신의 델타를 대상으로 한 감사 3라운드가 발견 30건을 등록했고,
  검증에서 살아남은 것은 여기서 고쳤습니다 ([GDK-393]).
- Linear 미러가 origin이 가진 것을 담고, Linear 쓰기가 Linear의 어휘로
  말합니다 — 두 소스가 같이 만드는 키는 명시적 `key_ambiguous` 거절입니다
  ([GDK-394], [GDK-396], [GDK-400]).
- 위키 쓰기가 실패 앞에서 정직합니다 ([GDK-408]).
- standalone 변환의 소유자는 하나입니다: busy 락이 소유자 pid를 지목하고,
  `gadak project create`가 origin을 통과해 standalone 워크스페이스에
  프로젝트를 더합니다 ([GDK-415], [GDK-421], [GDK-391]).
- `gadak_status`가 워크스페이스 kind를 알려 주고, 최상위 usage가 모든
  커맨드를 소유하고, 거절된 `create --parent`가 계층 규칙을 설명하며, ko
  문서가 위키 쓰기의 존재를 인정합니다 ([GDK-420], [GDK-426], [GDK-424],
  [GDK-409]).

## v0.16.0 — 2026-08-19

gadak이 쓸모 있으려면 Atlassian 계정이 있어야 하고, 돌리려면 Mac이 있어야
하고, 사람들이 실제로 트리아지하는 필드는 읽기만 되던 — 그 세 가지를 끝내는
릴리스입니다. 워크스페이스는 이제 standalone가 될 수 있습니다 — origin은
앱과 함께 다니는 최소 셀프호스트 Jira입니다 — 그리고 connected든
standalone든, 어떤 워크스페이스의 이슈든 읽은 자리에서 고칠 수 있습니다:
기한, 설명, 우선순위, 담당자, 그리고 사이트에서 실제로 쓰는 커스텀 필드.

### Atlassian 계정 없는 워크스페이스

- 워크스페이스는 standalone가 될 수 있습니다: origin이 프로세스 안에서 돌고,
  미러는 버려도 되는 캐시이며, 쓰기는 전부 origin을 통과합니다 ([GDK-183]).
- 워크스페이스는 origin 하나에 묶입니다 — 자격증명을 연결한다고 대상이
  조용히 바뀌지 않습니다 ([GDK-238], [GDK-247]).
- standalone 위키는 origin의 Confluence API를 통과해 쓰고, UI는 standalone에
  토큰을 묻지 않습니다 ([GDK-267], [GDK-237]).

### Windows와 Linux

- Windows는 포터블 팩과 설치 프로그램 경로, 거기서 동작하는 `install-cli`,
  두 번 적용되던 `gadak://`의 첫 실행 수정을 얻습니다. Scoop 매니페스트는
  Windows 없이도 검증할 수 있습니다 ([GDK-209], [GDK-293], [GDK-246]).
- Linux는 dmg와 대칭인 팩 스크립트, brew 옆의 tarball 설치, 버전 드리프트를
  막는 AUR 패키징 키트를 얻습니다 ([GDK-208], [GDK-229], [GDK-115]).
- Omarchy는 *내* 미러에서 무엇이 바뀌었는지 답하는 바 위젯과, 실제 게스트에서
  검증한 설치 레시피를 얻습니다 ([GDK-116], [GDK-225]).

### 읽은 자리에서 바로 고치는 이슈

- 필드 에디터가 QA 우리를 벗어납니다: 편집 가능 여부는 이슈 자신의
  editmeta이고, 허용 값은 한곳에서 옵니다 ([GDK-322], [GDK-323]).
- 기한은 상세 패널에서 설정하고 지웁니다 ([GDK-223]). 날짜는 먼저 읽기
  표면을 얻었고, Jira가 잘못된 기한을 거절하면 읽을 수 있는 문장이 됩니다
  ([GDK-249], [GDK-250], [GDK-251]).
- 설명은 평문으로 편집되고, 리치 본문을 파괴하기 전에 서식 손실 가드가
  있습니다 ([GDK-82]).
- `s`/`a`/`l`이 이미 동작하는 곳에서 `p`가 카탈로그 우선순위 메뉴를 열고,
  리스트의 담당자 메뉴가 상세 피커와 같은 사람을 찾습니다 ([GDK-331],
  [GDK-332]).
- 팔레트는 입력한 텍스트로 이슈를 만들 수 있고, 답이 뻔한 필수 생성 필드는
  질문이 아니며, 액션 이름은 모든 로케일에서 이기고, 코멘트를 올리면 도착했다고
  말합니다 ([GDK-217], [GDK-218], [GDK-302], [GDK-300], [GDK-301]).

### 데모, 그리고 문

- 호스티드 데모가 제품으로 바로 열리고, 설정 About 탭과 macOS Help 메뉴가
  같은 피드백 채널 4종을 담습니다 ([GDK-335], [GDK-336]).

### 업데이터 없이, 업데이트

- 업데이트 감지가 UI에 도달하고 플랫폼마다 맞는 말을 합니다 — 알림만, 자기
  업데이트는 없습니다. 릴리스 노트가 앱 안에서 렌더되고, 업그레이드 안내의
  소유자는 하나입니다 ([GDK-213], [GDK-214], [GDK-215], [GDK-216]).

### 그룹

- `team_group`은 파생 뷰가 다시 빌드될 때 미러 위 읽기 전용 쿼리 하나로
  분류되고, 키스트로크마다 돌지 않습니다. 빈 그룹은 미분류이고, `groupRules`는
  세 리스트 형식 그대로입니다.

### Linear, 연결하기 전에 먼저 재다

- 읽기 전용 Linear GraphQL 클라이언트가 기반 작업으로 들어왔고, 아직
  워크스페이스에 연결하지 않습니다 ([GDK-263], [GDK-274], [GDK-258],
  [GDK-261]).

### 감사의 파도

- 로컬라이즈된 이름은 키가 되지 않습니다: 상태·우선순위·유형은 어디서든 id와
  카테고리로 키합니다 ([GDK-275], [GDK-272], [GDK-248], [GDK-161]).
- 콜드 오픈 하나가 모두를 직렬화하던 일을 멈추고, 경합하는 쓰기는
  `busy_timeout`이 보지도 못해 즉시 죽었으며, 백그라운드 sync는 그것을 시작한
  서버보다 오래 살지 않습니다 ([GDK-282], [GDK-305], [GDK-270]).
- 다이얼로그 여섯이 셸 계약 하나를 공유하고, 온보딩이 자기 패인을 소유하며,
  한국어 카탈로그가 헤더 행 하나 안에서 자기와 싸우지 않습니다 ([GDK-297],
  [GDK-299], [GDK-298]).
- 미러의 다운그레이드 안내에 상한과 조언이 붙고, 만들어 놓고 연결하지 않았던
  위키 쓰기 표면이 연결되며, Linear 클라이언트가 죽은 자격증명을 감지합니다
  ([GDK-310], [GDK-267], [GDK-263], [GDK-274]).
- CI가 인프라에 대해 거짓말하는 일을 멈춥니다: 멈춘 apt 미러, configure
  중간에 apt를 죽이던 재시도, 락을 붙잡고 있던 고아 root apt-get — 각각 빨리
  실패하고 어느 쪽이 실패했는지 말합니다 ([GDK-308], [GDK-317], [GDK-319]).

### 에이전트를 위해

- 도그푸딩 마찰은 우회할 것이 아니라 백로그 항목이고, 에이전트는 미러에 쓸 수
  없다는 FAQ의 주장이 코드와 다시 맞으며, CJK 복합어 중간 검색은 앱 레이어
  바이그램입니다 ([GDK-312], [GDK-313], [GDK-314], [GDK-315], [GDK-306],
  [GDK-259]).

## v0.15.2 — 2026-08-17

설정이 화면이기를 멈추는 릴리스입니다. 다이얼로그가 고치는 필드마다 CLI
동사가 있어서, 에이전트가 워크스페이스를 처음부터 끝까지 차릴 수 있습니다 —
그리고 그 길로 먼저 다니는 것이 룩입니다.

### 설정이 에이전트 표면이 되다

- `gadak config list | get | set`은 CLI와 설정 PUT 뒤에 경로→검증 테이블이
  하나라서, 둘이 절대 어긋날 수 없습니다 ([GDK-193]).
- 테마는 워크스페이스별 `config.json`에 삽니다: UI에서 고르는 것과 터미널에서
  설정하는 것은 같은 행위입니다 ([GDK-190]).

### 다크 셋, 그중 하나는 원래 쓰던 것

- `dark`는 이제 뉴트럴-쿨 차콜이고, `ink`는 새 블루-블랙 팔레트이며, `ember`는
  이전의 따뜻한 다크를 바이트 단위로 보존합니다 ([GDK-190]).

### 길을 막고 있던 작은 것들

- 숫자만 쳐도 어느 프로젝트의 이슈든 찾습니다 ([GDK-186]).
- 설정 → 연동(데스크톱만)이 gadak이 설치하는 표면을 네 방향의 진실로 나열하고,
  메뉴가 설치를 멈춥니다 ([GDK-185], [GDK-189]).
- ⌘K 팔레트는 절대 비지 않습니다: 빈 질의는 최근 본 항목 아래에 최근 갱신
  이슈와 저장된 뷰를 보여 줍니다 ([GDK-184], [GDK-191]).
- 설정 다이얼로그가 모든 탭 위에 이 미러 블록을 반복하지 않습니다
  ([GDK-188]).

## v0.15.1 — 2026-08-17

- `gadak raycast install`이 Raycast 확장을 내장하고 등록해서, brew나 앱 번들
  설치가 체크아웃을 필요로 하지 않습니다 ([GDK-182]).
- ⌘K 팔레트 홈은 절대 비지 않습니다: 빈 질의는 최근 본 항목 아래에 최근 갱신
  이슈와 저장된 뷰를 보여 줍니다 ([GDK-184]).
- 설정 → 연동(데스크톱만)이 gadak이 설치하는 에이전트 표면을 정직한 감지와
  라이브 로그로 나열하고, 평결은 스트림의 마지막 `exit=` 줄입니다 ([GDK-185]).

## v0.15.0 — 2026-08-17

gadak을 바깥으로 여는 릴리스입니다. 뷰나 이슈는 이제 어떤 앱이든 넘겨 줄 수
있는 링크이고, 검색은 남의 UI를 키스트로크마다 굴릴 만큼 빠르며, Raycast에는
문서화된 입구가 생깁니다. 안으로는, 라이트와 같은 종이와 먹 기준으로 지은
다크 테마 — 그리고 새 의식의 첫 실행: 마이너마다 전체 코드베이스 감사.

### gadak이 주소가 되다

- `gadak://` 딥링크: gadak의 한 조각이 셸 명령이 아니라 링크로 다니고, 문법에는
  동사도 페이로드도 없습니다 ([GDK-119]).
- 모든 자리에 URL 안의 이름이 있습니다 — 검토된 레지스트리 하나 안의 자리
  파라미터 아홉 개 ([GDK-124]).
- Raycast, 문 둘: MCP install이 폼에 넣을 값을 찍고, 확장이 앉을 로컬 검색은
  "즉시처럼 느껴지는" 예산 안입니다 ([GDK-117]).
- 제품이 자기가 소비하는 링크를 만듭니다: 링크 복사 액션, `gadak issue KEY
  --link`, 그리고 `#/` 없이 붙여넣던 쿼리스트링이 가리킨 자리에 착륙합니다
  ([GDK-163], [GDK-164]).
- 이슈가 부모를 이름 댈 수 있습니다: `gadak create --parent`와 `gadak edit
  --parent`가 하위 이슈 관계를 Jira로 씁니다 ([GDK-19], [GDK-86]).
- 이슈 키를 치면 그 이슈가 나오고, 검색이 남의 키스트로크 아래에 앉을 만큼
  빠릅니다 — 아이템 2만 개 미러 최악 1.6s → 110ms ([GDK-170], [GDK-166]).

### 다크 테마, 그리고 다음 테마가 들어갈 자리

- Dark: 따뜻한 바닥, 먹 전경, 라이트와 같은 종이 은유, 첫 페인트 플래시 없음,
  세 번째 테마는 정의 블록 하나입니다 ([GDK-154], [GDK-156], [GDK-162]).
- 성공과 실패가 색만으로 말해지지 않습니다 ([GDK-158]).
- 두 팔레트가 같은 측정 하한을 통과합니다: 상태 잉크가 정상과 제2색각에서
  분리되고, 검색 하이라이트가 자기 토큰을 갖습니다 ([GDK-157], [GDK-159],
  [GDK-171]).

### 리스트가 AAA 리스트처럼 동작한다

- 행의 오른쪽은 훑을 수 있는 컬럼이고, 마지막 행이 반으로 잘리지 않습니다
  ([GDK-128], [GDK-131]).
- Esc는 보고 있는 것을 닫고, 1440 px 아래에서 덮는 패널이 스스로를 선언하며,
  한 개념은 한국어 한 단어입니다 ([GDK-132], [GDK-133], [GDK-127], [GDK-135]).
- 반만 조합된 음절은 질의가 아닙니다 ([GDK-169]).

### 가장자리에서의 정직

- 호스티드 스냅샷이 답할 수 없는 동사를 광고하지 않습니다 ([GDK-52]).
- 레거시 필드 매핑이 스스로 은퇴하고, 읽기 전용 홈에서는 그 재쓰기가 시작
  거절이 아니라 경고입니다 ([GDK-149], [GDK-173]).
- 복사면 복사된 것이고, 첨부는 최대 한 번만 가져오며, 데스크톱 앱이 런타임을
  두 번 로드하지 않습니다 ([GDK-178], [GDK-177], [GDK-150]).

### 감사, 그리고 그것이 지운 것

- 마이너마다 도는 전체 코드베이스 감사의 첫 실행: 발견 열여덟을 고쳤고,
  나머지는 `carryover-v0.15` 라벨을 달고 갑니다 ([GDK-125]).
- 타임스탬프의 소유자가 하나이고, 뷰 파라미터 키가 타입이 되며, Svelte 위생이
  죽은 export를 떨어냅니다 ([GDK-148], [GDK-147], [GDK-152]).
- 브라우저 테스트 열여섯이 유닛이 되고, Go 스위트가 벽시계에서 잠들지 않으며,
  테스트 없던 순수 모듈이 진짜 케이스를 얻습니다 ([GDK-145], [GDK-144],
  [GDK-146]).
- 파생 필드 의미의 단일한 집이 생기고 그 SQL 예제는 테스트가 실행하며, 초성
  검색이 제품 전역에서 은퇴합니다 ([GDK-88], [GDK-89], [GDK-168]).

## v0.14.2 — 2026-08-16

첫 10분과, 토큰이 죽는 날에 관한 릴리스입니다. 여기 있는 것은 새 능력이라기보다,
드디어 자기가 무엇을 하는지 말하는 기존 능력입니다.

- 모든 토큰 덫을 붙여넣기 전에 이름 댑니다. 401 다음이 아닙니다 ([GDK-69],
  [GDK-98]).
- 거절된 토큰은 쓰기 없이 복구할 수 있습니다 ([GDK-68]).
- 프로젝트를 고르지 않는 것도 선택입니다 ([GDK-99]).
- `gadak skill install`은 업그레이드를 업그레이드로 다루고, 내장 스킬이 CLI가
  가진 동사를 압니다 ([GDK-92], [GDK-91]).
- 조용한 Confluence 틱은 페이지 본문을 0건 읽습니다 ([GDK-113]).
- `gadak issue <KEY> --derive`가 파생 컬럼이 어떻게 계산됐는지 찍습니다
  ([GDK-111]).
- 히스토리가 순서를 지킵니다 ([GDK-26]).
- 토큰 만료가 sync가 죽기 전에 경고하고, browse 패인이 Escape를 양보하고,
  `gadak sql`이 오래된 미러에 경고하고, `Open`이 이 빌드가 쓸 수 없는
  `items_fts`를 고치고, 검색 도움 `?`가 터치에서 동작하고, `examples/compose`가
  순수 셸로 들어오고, Datasette Lite 딥링크가 핀되고, `PROMISES.md`가
  `SECURITY.md`에 대해 게이트됩니다 ([GDK-67], [GDK-78], [GDK-90], [GDK-112],
  [GDK-53], [GDK-109], [GDK-101], [GDK-104]).
- Node 버전의 소유자가 셸이 읽을 수 있는 하나이고, `tools/ci-status.sh`가
  방금 푸시한 것이 통과했는지 답합니다 ([GDK-57]).

## v0.14.1 — 2026-08-15

gadak의 백로그를 gadak으로 하루 도그푸딩하고, 착륙하는 대로 실은 날입니다:
첫 CLI 쓰기 동사, 사람들이 실제로 탭하는 곳에서 드디어 동작하는 데모, 그리고
신뢰를 한 번도 얻지 못한 업데이터의 제거.

- macOS 앱이 다시 알림만 합니다: 한 번도 돌려 보지 않은 인앱 자기 업데이터를
  제거했고, v0.14.1은 `gadak-desktop-darwin-<arch>.zip`을 싣지 않습니다
  ([GDK-58], [GDK-61]).
- 첫 쓰기 동사: `gadak create`, `gadak attach`, `gadak edit`.
- 호스티드 데모가 인앱 브라우저 안에서 동작하고, 첫 페인트가 폰 폭에서 읽히는
  정적 프레임입니다 ([GDK-23], [GDK-51]).
- browse 패인이 양보하고, 부트 키스트로크는 버려지지 않고 붙잡힙니다
  ([GDK-76], [GDK-46]).
- 실패가 무슨 일이 있었는지 말합니다: 잘린 키 목록은 몇 개인지 말하고, 거절된
  자격증명은 모든 소스의 watch 루프를 멈춥니다 ([GDK-35], [GDK-24],
  [GDK-48]).
- 우선순위 색이 계정의 언어가 아니라 rank를 읽습니다.
- MCP 도구가 호출마다 미러 전체를 스캔하지 않고, 웹 유닛 티어 스펙 100+개가
  푸시마다 ~300ms에 돕니다.

## v0.14.0 — 2026-08-15

유지자 리뷰 릴리스입니다: 사랑받는 개발자 도구를 만든 일곱 명에게, 렌즈별로,
gadak이 사랑받을지 아닐지 물었고 — 확인된 발견은 실리거나 막대가 적혔습니다.
테마는 신뢰입니다: 조용히 실패하지 않고 크게 실패하는 표면, 코드와 맞는 문서,
형용사가 아니라 잰 숫자.

- 첫 에이전트 호출이 성공하거나, 왜 아닌지를 말합니다: `gadak_search`의 주
  인자는 `query`이고, 모든 도구 오류는 `ERROR:`로 시작하며 받은 키를
  메아리치고, 응답 한도를 넘는 `gadak_issue`는 가장 오래된 코멘트를 버리고
  `truncated`라고 말합니다.
- 파이프가 이제 약속입니다: `issues_full` + RECIPES 쿼리, `gadak sql` stdout,
  `views open --keys -`.
- `gadak export` / `gadak import`가 정말로 아쉬울 행 — 저장된 뷰, 워치,
  즐겨찾기 — 를 자격증명 없이, 사이트 URL 없이 라운드트립합니다.
- 재고, 지는 행과 함께: 이슈 2,853개의 Cloud 프로젝트에 대한 라이브 사이트
  벤치 (단순 필터 42×, 에픽 GROUP BY 162×, 재오픈 횟수 REST ~20분에 맞서 로컬
  14.5 ms).
- 설정 다이얼로그가 빈 프로젝트 선택, 자른 웹 푸시 토글, 이미 다음 틱에 다시
  로드되는 config에 대해 거짓말을 멈춥니다.
- 호스티드 데모가 에픽별 보기에 착륙합니다.
- `brew install midagedev/tap/gadak`이 이제 앱이고, `gadak-cli`는 CLI만의
  formula입니다.
- 문서가 다시 진실을 말하고, 한국어 README와 모든 세션이 같은 계약에서
  시작하도록 레포 `CLAUDE.md`가 생깁니다.

## v0.13.0 — 2026-08-14

검색과 히스토리와 에이전트의 창을 한자리에 두는 릴리스입니다. ⌘K가 미러
전체를 검색하고, 무엇을 열었는가는 미러가 가져갈 수 없는 파일에 살며, `gadak
views open --keys -`가 에이전트의 답을 실행 중인 창에 올립니다.

- 모든 것을 검색하는 검색 상자 하나: ⌘K가 모든 이슈와 문서를 FTS 인덱스
  하나에서, 리스트의 필터 칩을 무시하고 질의하고, 리스트 위 상자는 옛 일을
  유지합니다 ("이 목록에서 좁히기").
- 히스토리는 미러 옆의 `~/.gadak/local.db`에 삽니다: 이슈·문서·검색이 하나의
  타임라인이고, 에이전트가 `gadak sql` 한 줄로 방문을 이슈에 조인합니다.
- 이슈 리스트가 문서 화면에 지지 않습니다: "리스트를 보여 줘"의 소유자가
  하나입니다.
- 창이 에이전트를 따릅니다: `keys` 축이 임의의 이슈 키 집합을 일급 뷰로
  만듭니다. `gadak views open`은 *gadak 안에서* 열고, `gadak open`은 Jira로
  떠납니다.
- MCP에 다섯 번째 도구 `gadak_show`가 생겨, 셸이 없는 호스트도 제시할 수
  있습니다.
- Confluence 스페이스 스코프가 이제 진짜입니다: 각 스페이스가 자기 워터마크를
  들고, 새로 선택한 스페이스는 전부 백필하며, 성공한 패스마다 스코프를 떠난
  스페이스를 제거합니다.
- account-id 버그 부류가 패치가 아니라 닫힙니다: 사람은 JQL·저장 뷰·필터·멤버
  디렉터리에서 account id로 해석되고, changelog와 첨부 작성자가 `author_id`를
  얻습니다.
- 프로필 이름이 홈 디렉터리를 탈출할 수 없고, 브라우저 가드가 mux 전체를
  감쌉니다.
- macOS 창을 드래그할 수 있습니다 (#2, @wafe 감사합니다).
- sync와 캐시 일관성: 위키 페이지의 코멘트만의 편집이 미러에 닿고, 바뀌지 않은
  페이지가 버전을 올리지 않고, 이슈→페이지 링크가 raw ADF에서 읽히고, 삭제된
  이슈가 단건 sync로 톰스톤되며, changelog 필드가 로컬라이즈된 이름이 아니라
  id로 식별됩니다.
- CLI와 서버의 정직: 모르는 `--profile`은 실제 목록과 함께 오류이고, 빈
  `GADAK_*`가 `SCRY_*` 폴백을 가리지 않고, 첨부 캐시는 사이트와 이슈로 키하며,
  업로드 뒤 실패한 미러 재읽기는 계약이 정한 502를 돌려줍니다.
- 사람 필터가 Jira 이메일 가시성에 의존하지 않습니다 (#1, @elppaaa
  감사합니다).
- JQL in, JQL out: Jira 내비게이터 URL이나 `jql=` 절을 붙여넣으면 맞는 칩이
  적용되고, JQL 복사가 돌아가는 길이며, 지원하지 않는 부분집합은 나열되고
  조용히 버려지지 않습니다.
- Jira 저장 필터가 사이드바에 착륙하고, `gadak views`가 나열·보여 주기·열기·
  저장을 합니다.
- README에 Claude 사용이 돌아왔습니다.

## v0.12.0 — 2026-08-13

룩과 개명의 릴리스입니다. gadak은 가닥입니다: 도포하지 않은 종이, 수묵, 쪽빛
실 하나 — 남은 수정 구슬 대시보드는 사라집니다. 같은 컷이 이슈에 라벨과
우선순위를 올리고, 데스크톱 앱에 워크스페이스를 넣고, 에이전트가 실제로 필요로
하는 CLI 동사를 줍니다.

- 다크 대시보드가 아니라 종이. 마크는 가를 두 획으로 그린 것입니다. TUI는
  사라집니다.
- 라벨이 리스트에 남고, 이슈에서 고치고, 벌크 바의 `l`로 선택에 적용됩니다
  (`s` / `a`와 같은 자리).
- 우선순위가 이제 동사입니다: 칩이 사이트 카탈로그를 열고 id로 씁니다. 이름은
  받지 않습니다.
- 제목을 고칠 수 있습니다.
- gadak으로 개명: 바이너리, 홈 디렉터리 (`~/.gadak`), env prefix (`GADAK_*`),
  MCP 도구, 모듈 경로, 데스크톱 번들 id. 기존 `~/.scry` 트리는 첫 실행에
  개명되고, `GADAK_*` 대응이 비어 있으면 `SCRY_*`를 여전히 읽습니다.
- `gadak profiles`가 이제 인벤토리입니다. 의도적으로 `switch`는 없습니다.
- 워크스페이스가 데스크톱 앱에서 동작하고, 자격증명이 있는 프로필마다 sync
  루프를 받습니다.
- 큰 미러에서 문서 목록이 더 이상 얼지 않습니다 (페이지 10,000장 창에서
  4,433ms → 68ms).
- 네이티브 타이틀 바가 사라지고, 창 컨트롤이 사이드바 첫 행으로 옮깁니다.
- `gadak skill install`이 MCP 없이 Claude Code 스킬을 내장하고, 데스크톱
  메뉴가 CLI를 설치하며, `gadak install-cli`가 실행 중인 바이너리를 PATH에
  올립니다.
- `gadak doctor`가 버그 리포트용으로 민감 정보를 가린 진단을 찍고, `gadak
  api`는 남의 호스트를 거절하는 raw Atlassian REST 탈출구입니다.

## v0.9.0 — 2026-08-06

사람과 시각 기초의 릴리스입니다. ⌘K에 이름을 치면 사람이 열리고, 모든 검색
히트가 왜 맞았는지 말하며, 크롬이 타입 스케일 하나와 오브 하나에 앉습니다.

- 사람 축: ⌘K에 이름을 치면 사람 패널이 열립니다. 이 버전은 웹만입니다.
- 검색이 왜 맞았는지 말하고, 맞은 필드의 스니펫을 보여 줍니다.
- 페이지 목록 발췌: 모든 페이지의 한 줄 본문 미리보기.
- 시각 기초: 진짜 타입 스케일, 6.2:1의 뮤트 텍스트, 모노크롬 아이콘 패밀리
  하나, 빨강이 의미에 예약된 아바타 팔레트.
- 어디서나 오브 하나: 워드마크의 구가 x-height에 앉고, 모든 아이콘이 같은
  그림에서 파생됩니다. 초승달 로고는 은퇴합니다.
- 색만이 아니라 기하: 두 단계 높이 그리드, 중첩을 따르는 코너 반지름, 구조로
  핀된 상세 패널 헤더, 한 작성자의 연속 코멘트가 헤더 하나 아래 그룹.
- 데모에 사람이 둘 이상 있습니다.

## v0.8.0 — 2026-08-06

- Gadak.app — macOS 데스크톱 앱: 웹 UI가 서명·공증된 자기 창에 있고, 로컬
  서버가 전혀 없습니다. 두 번째 실행은 실행 중인 창에 포커스하고, 번들이 CLI를
  듭니다.
- 인앱 온보딩 뒤에 sync가 재시작 없이 시작됩니다.

## v0.7.0 — 2026-08-06

에이전트를 올바른 미러에 핀하고 제품에 얼굴을 다는 릴리스입니다. MCP
install이 현재 프로필을 호스트에 쓰고, 로컬 API가 열린 프록시이기를 멈추며,
README가 라이브 데모로 시작합니다.

- `gadak mcp install <client>`가 현재 프로필과 절대 바이너리 경로를 MCP 호스트
  등록에 핀합니다.
- 로컬 API의 브라우저 가드: 크로스 오리진 쓰기와 DNS-rebinding 읽기를
  거절합니다.
- 스페이스 이름, 문서 UX 파도(내가 본 / 최근 갱신 / 작성자별), 에픽 기본 뷰.
- 미러 파일 권한이 열릴 때 `0600` / `0700`으로 조여집니다.
- 얼굴: 한 번도 없던 워드마크, 로고, favicon.
- 데모가 영어를 말합니다 (CJK 검색을 위해 한국어 내러티브 페이지는 남습니다).
- `docs/FAQ.md`가 어려운 질문에 영수증으로 답합니다.
- 리졸버가 루프백으로 매핑하면 `serve`가 `http://gadak.localhost`를 열고, 바쁜
  listen 포트는 실행 중인 gadak에 넘기거나 빈 포트로 폴백합니다.
- 키보드 트리아지, 신선도 칩, 웜부트 캐시, 이슈 1만 개 픽스처에 대한 인터랙션
  성능 게이트.
- 실제 사이트용으로 단단해진 Confluence sync.

## v0.6.0 — 2026-08-06

위키 릴리스입니다. Confluence 페이지가 items 척추에 합류하고, 웹 UI와 TUI에
나타나며, 이슈가 정직한 에픽 계층을 얻습니다.

- Confluence 페이지 라벨: fetch 때 모아져 페이지가 나타나는 모든 곳에 보입니다.
- 에픽 계층: 파생 `epic_key` — 가장 가까운 level-1 조상 — 이라서 서브태스크가
  스토리가 아니라 에픽 아래 묶입니다.
- Confluence 페이지 미러: items 척추의 두 번째 소스. 웹 UI의 문서와 TUI 문서
  내비게이터 (`D`).
- 웹 UI의 에픽 계층: 그룹 라벨, 행 칩, 브레드크럼, 롤업.
- 폰이 짜인 컬럼이 아니라 데스크톱 레이아웃을 렌더합니다.

## v0.5.0 — 2026-08-05

- 워크스페이스: `serve`가 모든 프로필을 `/w/<name>/` 아래에 마운트합니다.
- TUI 네온 룩: 앰비언트 애니메이션, 마우스 지원, 팔레트, 매치 하이라이트.
- 검색 prefix 매치, 활용된 한국어가 찾아집니다.

## v0.4.0 — 2026-08-05

- TUI 커스텀 필드 편집, Jira가 허용한 값만.
- 업데이트 안내: 모든 표면에서 매일 익명 확인, opt-out 있음.
- 호스티드 데모 서비스 워커 핸드셰이크: 브라우저가 데모를 돌릴 수 없으면
  깨끗이 타임아웃하고 그렇게 말합니다.

## v0.3.0 — 2026-08-05

- 필드 자동 발견: 첫 full sync가 커스텀 필드를 스스로 발견하고 설정합니다.
- 발견한 필드의 필터 축, multi-select 에디터 포함.
- sync 진행 줄이 진짜 총량을 들고, 프로젝트는 sync에서 선택입니다.
- 사이드바 타임스탬프 뒤의 sync 히스토리.

## v0.2.1 — 2026-08-05

- macOS 릴리스 바이너리를 서명하고 공증합니다.
- 호스티드 데모: 변경이 저장되지 않았다고 말하는 로컬 쓰기 시뮬레이션, 그리고
  표면을 데모로 식별하는 카피.

## v0.2.0 — 2026-08-05

팀 설정 공유, 설치 없는 호스티드 데모, 개인 워치 피드, 그리고 저장 스키마와
HTTP·sync·에이전트 계약.

- 팀 설정 공유: `gadak team export` / `import`가 팀이 합의하는 뷰, 필드 맵,
  그룹 규칙, 임계값을 쓰고, 자격증명은 절대 다니지 않으며, 자격증명 키를 담은
  파일은 import에서 거절됩니다.
- 레이트 리밋 가시성: 우리 자신의 호출량. `gadak status`와 설정 런타임 패널에
  보이고, 횟수가 0이면 숨습니다.
- `gadak fields`가 실제로 채워진 커스텀 필드가 무엇인지 보고합니다.
- `gadak snapshot`이 나눌 수 있는 사본을 짓습니다. `--spread`는 이슈의 내부
  순서를 보존한 채 창에 걸쳐 타임스탬프를 다시 말하고, `--scale`은 이슈를 새
  키에 복제하고, `--now`는 시계를 핀하며, 게시 전에 자격증명 스캔이 돕니다.
- 명령별 도움, FlagSet에서 생성되어 어긋날 수 없습니다.
- TUI 패리티: 피드 포커스 탭, 저장된 뷰의 sort/dir/group_by, `priority_rank`로
  키하는 우선순위 정렬.
- 즐겨찾기가 미러에 살아서 `gadak sql`과 에이전트가 볼 수 있고, 호스티드
  데모는 로컬 저장소로 폴백합니다.
- `presence` 클라이언트 스택이 사라집니다.
- 설치 없는 호스티드 데모: 데모 전용 서비스 워커가 서빙하는 정적 스냅샷 —
  바이너리 없이, 계정 없이.
- 리텐션 루프: 자격증명이 있으면 `gadak serve`가 기본으로 sync watch 루프를
  시작하고, `gadak install-service`가 launchd 에이전트나 systemd user unit을
  쓰며, 새 개인 피드 이벤트에 OS 데스크톱 알림이 하나 뜰 수 있습니다.
- 개인 워치 피드, 질의 때 미러에서 30일 창으로 계산됩니다.
- 데모 Jira seeder가 Python에서 Go로 옮겨지고, 웹 애플리케이션이 내부 배포에서
  이 저장소로 추출됩니다.
- 내장 뷰가 모든 Jira 사이트에서 같은 의미인 축으로 키하고, resolution과
  재오픈 감지가 로컬라이즈된 이름이 아니라 상태 *카테고리*로 키합니다.
- `gadak serve`가 빌드된 UI를 서빙하고, `--allow-remote` 없이 루프백이 아닌
  주소에 바인드하는 것을 거절합니다.
- 저장 스키마와 HTTP·sync·에이전트 계약, 그리고 WAL, FTS5, 파생 필드 계산기를
  갖춘 SQLite 구현.

[GDK-8]: https://gadak.dev/backlog/#/?ks=GDK-8
[GDK-19]: https://gadak.dev/backlog/#/?ks=GDK-19
[GDK-23]: https://gadak.dev/backlog/#/?ks=GDK-23
[GDK-24]: https://gadak.dev/backlog/#/?ks=GDK-24
[GDK-26]: https://gadak.dev/backlog/#/?ks=GDK-26
[GDK-35]: https://gadak.dev/backlog/#/?ks=GDK-35
[GDK-46]: https://gadak.dev/backlog/#/?ks=GDK-46
[GDK-48]: https://gadak.dev/backlog/#/?ks=GDK-48
[GDK-51]: https://gadak.dev/backlog/#/?ks=GDK-51
[GDK-52]: https://gadak.dev/backlog/#/?ks=GDK-52
[GDK-53]: https://gadak.dev/backlog/#/?ks=GDK-53
[GDK-57]: https://gadak.dev/backlog/#/?ks=GDK-57
[GDK-58]: https://gadak.dev/backlog/#/?ks=GDK-58
[GDK-61]: https://gadak.dev/backlog/#/?ks=GDK-61
[GDK-67]: https://gadak.dev/backlog/#/?ks=GDK-67
[GDK-68]: https://gadak.dev/backlog/#/?ks=GDK-68
[GDK-69]: https://gadak.dev/backlog/#/?ks=GDK-69
[GDK-76]: https://gadak.dev/backlog/#/?ks=GDK-76
[GDK-78]: https://gadak.dev/backlog/#/?ks=GDK-78
[GDK-82]: https://gadak.dev/backlog/#/?ks=GDK-82
[GDK-83]: https://gadak.dev/backlog/#/?ks=GDK-83
[GDK-85]: https://gadak.dev/backlog/#/?ks=GDK-85
[GDK-86]: https://gadak.dev/backlog/#/?ks=GDK-86
[GDK-88]: https://gadak.dev/backlog/#/?ks=GDK-88
[GDK-89]: https://gadak.dev/backlog/#/?ks=GDK-89
[GDK-90]: https://gadak.dev/backlog/#/?ks=GDK-90
[GDK-91]: https://gadak.dev/backlog/#/?ks=GDK-91
[GDK-92]: https://gadak.dev/backlog/#/?ks=GDK-92
[GDK-93]: https://gadak.dev/backlog/#/?ks=GDK-93
[GDK-98]: https://gadak.dev/backlog/#/?ks=GDK-98
[GDK-99]: https://gadak.dev/backlog/#/?ks=GDK-99
[GDK-101]: https://gadak.dev/backlog/#/?ks=GDK-101
[GDK-104]: https://gadak.dev/backlog/#/?ks=GDK-104
[GDK-105]: https://gadak.dev/backlog/#/?ks=GDK-105
[GDK-109]: https://gadak.dev/backlog/#/?ks=GDK-109
[GDK-111]: https://gadak.dev/backlog/#/?ks=GDK-111
[GDK-112]: https://gadak.dev/backlog/#/?ks=GDK-112
[GDK-113]: https://gadak.dev/backlog/#/?ks=GDK-113
[GDK-115]: https://gadak.dev/backlog/#/?ks=GDK-115
[GDK-116]: https://gadak.dev/backlog/#/?ks=GDK-116
[GDK-117]: https://gadak.dev/backlog/#/?ks=GDK-117
[GDK-119]: https://gadak.dev/backlog/#/?ks=GDK-119
[GDK-121]: https://gadak.dev/backlog/#/?ks=GDK-121
[GDK-124]: https://gadak.dev/backlog/#/?ks=GDK-124
[GDK-125]: https://gadak.dev/backlog/#/?ks=GDK-125
[GDK-127]: https://gadak.dev/backlog/#/?ks=GDK-127
[GDK-128]: https://gadak.dev/backlog/#/?ks=GDK-128
[GDK-129]: https://gadak.dev/backlog/#/?ks=GDK-129
[GDK-130]: https://gadak.dev/backlog/#/?ks=GDK-130
[GDK-131]: https://gadak.dev/backlog/#/?ks=GDK-131
[GDK-132]: https://gadak.dev/backlog/#/?ks=GDK-132
[GDK-133]: https://gadak.dev/backlog/#/?ks=GDK-133
[GDK-135]: https://gadak.dev/backlog/#/?ks=GDK-135
[GDK-144]: https://gadak.dev/backlog/#/?ks=GDK-144
[GDK-145]: https://gadak.dev/backlog/#/?ks=GDK-145
[GDK-146]: https://gadak.dev/backlog/#/?ks=GDK-146
[GDK-147]: https://gadak.dev/backlog/#/?ks=GDK-147
[GDK-148]: https://gadak.dev/backlog/#/?ks=GDK-148
[GDK-149]: https://gadak.dev/backlog/#/?ks=GDK-149
[GDK-150]: https://gadak.dev/backlog/#/?ks=GDK-150
[GDK-152]: https://gadak.dev/backlog/#/?ks=GDK-152
[GDK-154]: https://gadak.dev/backlog/#/?ks=GDK-154
[GDK-156]: https://gadak.dev/backlog/#/?ks=GDK-156
[GDK-157]: https://gadak.dev/backlog/#/?ks=GDK-157
[GDK-158]: https://gadak.dev/backlog/#/?ks=GDK-158
[GDK-159]: https://gadak.dev/backlog/#/?ks=GDK-159
[GDK-161]: https://gadak.dev/backlog/#/?ks=GDK-161
[GDK-162]: https://gadak.dev/backlog/#/?ks=GDK-162
[GDK-163]: https://gadak.dev/backlog/#/?ks=GDK-163
[GDK-164]: https://gadak.dev/backlog/#/?ks=GDK-164
[GDK-166]: https://gadak.dev/backlog/#/?ks=GDK-166
[GDK-168]: https://gadak.dev/backlog/#/?ks=GDK-168
[GDK-169]: https://gadak.dev/backlog/#/?ks=GDK-169
[GDK-170]: https://gadak.dev/backlog/#/?ks=GDK-170
[GDK-171]: https://gadak.dev/backlog/#/?ks=GDK-171
[GDK-173]: https://gadak.dev/backlog/#/?ks=GDK-173
[GDK-177]: https://gadak.dev/backlog/#/?ks=GDK-177
[GDK-178]: https://gadak.dev/backlog/#/?ks=GDK-178
[GDK-182]: https://gadak.dev/backlog/#/?ks=GDK-182
[GDK-183]: https://gadak.dev/backlog/#/?ks=GDK-183
[GDK-184]: https://gadak.dev/backlog/#/?ks=GDK-184
[GDK-185]: https://gadak.dev/backlog/#/?ks=GDK-185
[GDK-186]: https://gadak.dev/backlog/#/?ks=GDK-186
[GDK-188]: https://gadak.dev/backlog/#/?ks=GDK-188
[GDK-189]: https://gadak.dev/backlog/#/?ks=GDK-189
[GDK-190]: https://gadak.dev/backlog/#/?ks=GDK-190
[GDK-191]: https://gadak.dev/backlog/#/?ks=GDK-191
[GDK-193]: https://gadak.dev/backlog/#/?ks=GDK-193
[GDK-208]: https://gadak.dev/backlog/#/?ks=GDK-208
[GDK-209]: https://gadak.dev/backlog/#/?ks=GDK-209
[GDK-211]: https://gadak.dev/backlog/#/?ks=GDK-211
[GDK-213]: https://gadak.dev/backlog/#/?ks=GDK-213
[GDK-214]: https://gadak.dev/backlog/#/?ks=GDK-214
[GDK-215]: https://gadak.dev/backlog/#/?ks=GDK-215
[GDK-216]: https://gadak.dev/backlog/#/?ks=GDK-216
[GDK-217]: https://gadak.dev/backlog/#/?ks=GDK-217
[GDK-218]: https://gadak.dev/backlog/#/?ks=GDK-218
[GDK-223]: https://gadak.dev/backlog/#/?ks=GDK-223
[GDK-225]: https://gadak.dev/backlog/#/?ks=GDK-225
[GDK-229]: https://gadak.dev/backlog/#/?ks=GDK-229
[GDK-237]: https://gadak.dev/backlog/#/?ks=GDK-237
[GDK-238]: https://gadak.dev/backlog/#/?ks=GDK-238
[GDK-239]: https://gadak.dev/backlog/#/?ks=GDK-239
[GDK-241]: https://gadak.dev/backlog/#/?ks=GDK-241
[GDK-246]: https://gadak.dev/backlog/#/?ks=GDK-246
[GDK-247]: https://gadak.dev/backlog/#/?ks=GDK-247
[GDK-248]: https://gadak.dev/backlog/#/?ks=GDK-248
[GDK-249]: https://gadak.dev/backlog/#/?ks=GDK-249
[GDK-250]: https://gadak.dev/backlog/#/?ks=GDK-250
[GDK-251]: https://gadak.dev/backlog/#/?ks=GDK-251
[GDK-254]: https://gadak.dev/backlog/#/?ks=GDK-254
[GDK-258]: https://gadak.dev/backlog/#/?ks=GDK-258
[GDK-259]: https://gadak.dev/backlog/#/?ks=GDK-259
[GDK-261]: https://gadak.dev/backlog/#/?ks=GDK-261
[GDK-263]: https://gadak.dev/backlog/#/?ks=GDK-263
[GDK-267]: https://gadak.dev/backlog/#/?ks=GDK-267
[GDK-269]: https://gadak.dev/backlog/#/?ks=GDK-269
[GDK-270]: https://gadak.dev/backlog/#/?ks=GDK-270
[GDK-271]: https://gadak.dev/backlog/#/?ks=GDK-271
[GDK-272]: https://gadak.dev/backlog/#/?ks=GDK-272
[GDK-274]: https://gadak.dev/backlog/#/?ks=GDK-274
[GDK-275]: https://gadak.dev/backlog/#/?ks=GDK-275
[GDK-282]: https://gadak.dev/backlog/#/?ks=GDK-282
[GDK-293]: https://gadak.dev/backlog/#/?ks=GDK-293
[GDK-297]: https://gadak.dev/backlog/#/?ks=GDK-297
[GDK-298]: https://gadak.dev/backlog/#/?ks=GDK-298
[GDK-299]: https://gadak.dev/backlog/#/?ks=GDK-299
[GDK-300]: https://gadak.dev/backlog/#/?ks=GDK-300
[GDK-301]: https://gadak.dev/backlog/#/?ks=GDK-301
[GDK-302]: https://gadak.dev/backlog/#/?ks=GDK-302
[GDK-304]: https://gadak.dev/backlog/#/?ks=GDK-304
[GDK-305]: https://gadak.dev/backlog/#/?ks=GDK-305
[GDK-306]: https://gadak.dev/backlog/#/?ks=GDK-306
[GDK-308]: https://gadak.dev/backlog/#/?ks=GDK-308
[GDK-310]: https://gadak.dev/backlog/#/?ks=GDK-310
[GDK-312]: https://gadak.dev/backlog/#/?ks=GDK-312
[GDK-313]: https://gadak.dev/backlog/#/?ks=GDK-313
[GDK-314]: https://gadak.dev/backlog/#/?ks=GDK-314
[GDK-315]: https://gadak.dev/backlog/#/?ks=GDK-315
[GDK-316]: https://gadak.dev/backlog/#/?ks=GDK-316
[GDK-317]: https://gadak.dev/backlog/#/?ks=GDK-317
[GDK-319]: https://gadak.dev/backlog/#/?ks=GDK-319
[GDK-322]: https://gadak.dev/backlog/#/?ks=GDK-322
[GDK-323]: https://gadak.dev/backlog/#/?ks=GDK-323
[GDK-328]: https://gadak.dev/backlog/#/?ks=GDK-328
[GDK-329]: https://gadak.dev/backlog/#/?ks=GDK-329
[GDK-330]: https://gadak.dev/backlog/#/?ks=GDK-330
[GDK-331]: https://gadak.dev/backlog/#/?ks=GDK-331
[GDK-332]: https://gadak.dev/backlog/#/?ks=GDK-332
[GDK-333]: https://gadak.dev/backlog/#/?ks=GDK-333
[GDK-335]: https://gadak.dev/backlog/#/?ks=GDK-335
[GDK-336]: https://gadak.dev/backlog/#/?ks=GDK-336
[GDK-340]: https://gadak.dev/backlog/#/?ks=GDK-340
[GDK-342]: https://gadak.dev/backlog/#/?ks=GDK-342
[GDK-343]: https://gadak.dev/backlog/#/?ks=GDK-343
[GDK-344]: https://gadak.dev/backlog/#/?ks=GDK-344
[GDK-345]: https://gadak.dev/backlog/#/?ks=GDK-345
[GDK-346]: https://gadak.dev/backlog/#/?ks=GDK-346
[GDK-347]: https://gadak.dev/backlog/#/?ks=GDK-347
[GDK-348]: https://gadak.dev/backlog/#/?ks=GDK-348
[GDK-349]: https://gadak.dev/backlog/#/?ks=GDK-349
[GDK-350]: https://gadak.dev/backlog/#/?ks=GDK-350
[GDK-351]: https://gadak.dev/backlog/#/?ks=GDK-351
[GDK-353]: https://gadak.dev/backlog/#/?ks=GDK-353
[GDK-359]: https://gadak.dev/backlog/#/?ks=GDK-359
[GDK-360]: https://gadak.dev/backlog/#/?ks=GDK-360
[GDK-361]: https://gadak.dev/backlog/#/?ks=GDK-361
[GDK-363]: https://gadak.dev/backlog/#/?ks=GDK-363
[GDK-364]: https://gadak.dev/backlog/#/?ks=GDK-364
[GDK-365]: https://gadak.dev/backlog/#/?ks=GDK-365
[GDK-366]: https://gadak.dev/backlog/#/?ks=GDK-366
[GDK-367]: https://gadak.dev/backlog/#/?ks=GDK-367
[GDK-368]: https://gadak.dev/backlog/#/?ks=GDK-368
[GDK-371]: https://gadak.dev/backlog/#/?ks=GDK-371
[GDK-372]: https://gadak.dev/backlog/#/?ks=GDK-372
[GDK-373]: https://gadak.dev/backlog/#/?ks=GDK-373
[GDK-374]: https://gadak.dev/backlog/#/?ks=GDK-374
[GDK-375]: https://gadak.dev/backlog/#/?ks=GDK-375
[GDK-376]: https://gadak.dev/backlog/#/?ks=GDK-376
[GDK-380]: https://gadak.dev/backlog/#/?ks=GDK-380
[GDK-381]: https://gadak.dev/backlog/#/?ks=GDK-381
[GDK-382]: https://gadak.dev/backlog/#/?ks=GDK-382
[GDK-389]: https://gadak.dev/backlog/#/?ks=GDK-389
[GDK-391]: https://gadak.dev/backlog/#/?ks=GDK-391
[GDK-393]: https://gadak.dev/backlog/#/?ks=GDK-393
[GDK-394]: https://gadak.dev/backlog/#/?ks=GDK-394
[GDK-396]: https://gadak.dev/backlog/#/?ks=GDK-396
[GDK-400]: https://gadak.dev/backlog/#/?ks=GDK-400
[GDK-408]: https://gadak.dev/backlog/#/?ks=GDK-408
[GDK-409]: https://gadak.dev/backlog/#/?ks=GDK-409
[GDK-415]: https://gadak.dev/backlog/#/?ks=GDK-415
[GDK-418]: https://gadak.dev/backlog/#/?ks=GDK-418
[GDK-420]: https://gadak.dev/backlog/#/?ks=GDK-420
[GDK-421]: https://gadak.dev/backlog/#/?ks=GDK-421
[GDK-424]: https://gadak.dev/backlog/#/?ks=GDK-424
[GDK-425]: https://gadak.dev/backlog/#/?ks=GDK-425
[GDK-426]: https://gadak.dev/backlog/#/?ks=GDK-426
[GDK-427]: https://gadak.dev/backlog/#/?ks=GDK-427
[GDK-430]: https://gadak.dev/backlog/#/?ks=GDK-430
[GDK-433]: https://gadak.dev/backlog/#/?ks=GDK-433
[GDK-437]: https://gadak.dev/backlog/#/?ks=GDK-437
[GDK-444]: https://gadak.dev/backlog/#/?ks=GDK-444
[GDK-449]: https://gadak.dev/backlog/#/?ks=GDK-449
[GDK-452]: https://gadak.dev/backlog/#/?ks=GDK-452
[GDK-453]: https://gadak.dev/backlog/#/?ks=GDK-453
[GDK-490]: https://gadak.dev/backlog/#/?ks=GDK-490
[GDK-495]: https://gadak.dev/backlog/#/?ks=GDK-495
[GDK-496]: https://gadak.dev/backlog/#/?ks=GDK-496
[GDK-497]: https://gadak.dev/backlog/#/?ks=GDK-497
[GDK-500]: https://gadak.dev/backlog/#/?ks=GDK-500
[GDK-501]: https://gadak.dev/backlog/#/?ks=GDK-501
[GDK-502]: https://gadak.dev/backlog/#/?ks=GDK-502
[GDK-503]: https://gadak.dev/backlog/#/?ks=GDK-503
[GDK-509]: https://gadak.dev/backlog/#/?ks=GDK-509
[GDK-513]: https://gadak.dev/backlog/#/?ks=GDK-513
[GDK-514]: https://gadak.dev/backlog/#/?ks=GDK-514
[GDK-515]: https://gadak.dev/backlog/#/?ks=GDK-515
[GDK-516]: https://gadak.dev/backlog/#/?ks=GDK-516
[GDK-517]: https://gadak.dev/backlog/#/?ks=GDK-517
[GDK-518]: https://gadak.dev/backlog/#/?ks=GDK-518
[GDK-519]: https://gadak.dev/backlog/#/?ks=GDK-519
[GDK-521]: https://gadak.dev/backlog/#/?ks=GDK-521
[GDK-527]: https://gadak.dev/backlog/#/?ks=GDK-527
[GDK-531]: https://gadak.dev/backlog/#/?ks=GDK-531
[GDK-532]: https://gadak.dev/backlog/#/?ks=GDK-532
[GDK-536]: https://gadak.dev/backlog/#/?ks=GDK-536
[GDK-537]: https://gadak.dev/backlog/#/?ks=GDK-537
[GDK-538]: https://gadak.dev/backlog/#/?ks=GDK-538
[GDK-539]: https://gadak.dev/backlog/#/?ks=GDK-539
[GDK-540]: https://gadak.dev/backlog/#/?ks=GDK-540
[GDK-541]: https://gadak.dev/backlog/#/?ks=GDK-541
[GDK-542]: https://gadak.dev/backlog/#/?ks=GDK-542
[GDK-555]: https://gadak.dev/backlog/#/?ks=GDK-555
[GDK-558]: https://gadak.dev/backlog/#/?ks=GDK-558
[GDK-560]: https://gadak.dev/backlog/#/?ks=GDK-560
[GDK-561]: https://gadak.dev/backlog/#/?ks=GDK-561
[GDK-562]: https://gadak.dev/backlog/#/?ks=GDK-562
[GDK-586]: https://gadak.dev/backlog/#/?ks=GDK-586
[GDK-588]: https://gadak.dev/backlog/#/?ks=GDK-588
[GDK-589]: https://gadak.dev/backlog/#/?ks=GDK-589
[GDK-590]: https://gadak.dev/backlog/#/?ks=GDK-590
[GDK-591]: https://gadak.dev/backlog/#/?ks=GDK-591
[GDK-592]: https://gadak.dev/backlog/#/?ks=GDK-592
[GDK-593]: https://gadak.dev/backlog/#/?ks=GDK-593
[GDK-597]: https://gadak.dev/backlog/#/?ks=GDK-597
[GDK-598]: https://gadak.dev/backlog/#/?ks=GDK-598
[GDK-599]: https://gadak.dev/backlog/#/?ks=GDK-599
[GDK-601]: https://gadak.dev/backlog/#/?ks=GDK-601
[GDK-602]: https://gadak.dev/backlog/#/?ks=GDK-602
[GDK-603]: https://gadak.dev/backlog/#/?ks=GDK-603
[GDK-604]: https://gadak.dev/backlog/#/?ks=GDK-604
[GDK-605]: https://gadak.dev/backlog/#/?ks=GDK-605
[GDK-606]: https://gadak.dev/backlog/#/?ks=GDK-606
[GDK-607]: https://gadak.dev/backlog/#/?ks=GDK-607
[GDK-608]: https://gadak.dev/backlog/#/?ks=GDK-608
[GDK-609]: https://gadak.dev/backlog/#/?ks=GDK-609
[GDK-610]: https://gadak.dev/backlog/#/?ks=GDK-610
[GDK-611]: https://gadak.dev/backlog/#/?ks=GDK-611
[GDK-612]: https://gadak.dev/backlog/#/?ks=GDK-612
[GDK-613]: https://gadak.dev/backlog/#/?ks=GDK-613
[GDK-615]: https://gadak.dev/backlog/#/?ks=GDK-615
[GDK-616]: https://gadak.dev/backlog/#/?ks=GDK-616
[GDK-617]: https://gadak.dev/backlog/#/?ks=GDK-617
[GDK-619]: https://gadak.dev/backlog/#/?ks=GDK-619
[GDK-626]: https://gadak.dev/backlog/#/?ks=GDK-626
[GDK-635]: https://gadak.dev/backlog/#/?ks=GDK-635
[GDK-639]: https://gadak.dev/backlog/#/?ks=GDK-639
[GDK-641]: https://gadak.dev/backlog/#/?ks=GDK-641
[GDK-642]: https://gadak.dev/backlog/#/?ks=GDK-642
[GDK-643]: https://gadak.dev/backlog/#/?ks=GDK-643
[GDK-644]: https://gadak.dev/backlog/#/?ks=GDK-644
[GDK-647]: https://gadak.dev/backlog/#/?ks=GDK-647
[GDK-654]: https://gadak.dev/backlog/#/?ks=GDK-654
[GDK-658]: https://gadak.dev/backlog/#/?ks=GDK-658
[GDK-664]: https://gadak.dev/backlog/#/?ks=GDK-664
[GDK-665]: https://gadak.dev/backlog/#/?ks=GDK-665
[GDK-666]: https://gadak.dev/backlog/#/?ks=GDK-666
[GDK-668]: https://gadak.dev/backlog/#/?ks=GDK-668
[GDK-669]: https://gadak.dev/backlog/#/?ks=GDK-669
[GDK-671]: https://gadak.dev/backlog/#/?ks=GDK-671
[GDK-672]: https://gadak.dev/backlog/#/?ks=GDK-672
[GDK-673]: https://gadak.dev/backlog/#/?ks=GDK-673
[GDK-674]: https://gadak.dev/backlog/#/?ks=GDK-674
[GDK-675]: https://gadak.dev/backlog/#/?ks=GDK-675
[GDK-676]: https://gadak.dev/backlog/#/?ks=GDK-676
[GDK-677]: https://gadak.dev/backlog/#/?ks=GDK-677
[GDK-678]: https://gadak.dev/backlog/#/?ks=GDK-678
[GDK-681]: https://gadak.dev/backlog/#/?ks=GDK-681
[GDK-682]: https://gadak.dev/backlog/#/?ks=GDK-682
[GDK-683]: https://gadak.dev/backlog/#/?ks=GDK-683
[GDK-738]: https://gadak.dev/backlog/#/?ks=GDK-738
[GDK-739]: https://gadak.dev/backlog/#/?ks=GDK-739
[GDK-742]: https://gadak.dev/backlog/#/?ks=GDK-742
