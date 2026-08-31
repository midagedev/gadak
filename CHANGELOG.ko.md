# Changelog

<sub><a href="CHANGELOG.md">English</a> · 한국어 — 영문이 원본이며, 번역은 영문과 함께 갱신됩니다(마지막 동기화 2026-09-01).</sub>

## v0.19.0 — 2026-09-01

이슈가 보드로 일어서고, 터미널이 베타 딱지를 떼는 릴리스입니다.

**보드.** 늘 필터하던 그 리스트가 이제 보드이기도 합니다: 토글 하나로 같은
이슈들이 컬럼으로 눕습니다 — 같은 필터, 같은 13개 그룹 축, 같은 검색.
카드를 끌면 진짜 전환이고(대상 상태가 여럿이면 메뉴가 묻습니다), 다른
곳에서 일어난 이동은 — 다른 창이든, 에이전트의 `gadak transition`이든 —
착지 링과 함께 보드를 가로질러 날아옵니다. 이 화면에서는 움직임이 그 일이
일어났다는 유일한 증거이기 때문입니다. 상태 컬럼 셋은 항상 셋입니다:
"Done이 비어 있다"는 없어진 컬럼이 아니라 대답입니다 ([GDK-1175],
[GDK-1176], [GDK-1190]). 그리고 뷰는 자신이 보드임을 말할 수 있습니다:
보드 레이아웃으로 저장한 뷰는 보드로 다시 열립니다 — 앱에서도, CLI에서도:
`gadak views save "Sprint board" --jql '…' --layout board`, 딥링크도
레이아웃을 싣습니다 ([GDK-1248]).

**터미널이 베타 딱지를 뗍니다** ([GDK-1024]) — 그리고 세션은 이슈의
것입니다. 패널의 셸에서 `gadak claim`을 치면 그 세션이 이슈에 묶이고, 탭이
이슈 키를 답니다 ([GDK-1158]). 이슈 본문의 명령에는 ▶가 붙어 그 이슈의 셸
프롬프트에 명령을 놓아 주고, 보드 카드와 ⌘K 팔레트 둘 다 이슈의 세션을
엽니다 ([GDK-1196], [GDK-1197]). 패널은 행 전체 아래 독으로 눕고, 크롬은 한
줄이며, 탭에서 세션을 끝낼 수 있습니다 ([GDK-1194], [GDK-1199],
[GDK-1200]). 포커스된 터미널이 더는 키보드의 함정이 아닙니다:
`Ctrl+Shift+[` 와 `]` 로 세션을 오가고, `` Ctrl+Shift+` `` 는 아무것도 닫지
않고 빠져나오며, `Ctrl+Shift+O` 는 그 세션의 이슈를 엽니다 ([GDK-1250],
[GDK-1251]).

**폰이 집을 여러 개 가집니다.** 호스트마다 캐시를 가진 로스터로 페어링된
머신들을 오가고, 워크스페이스는 웹과 CLI에서 만들고 지웁니다. 페어링 없이
모든 화면을 보여주는 내장 데모 워크스페이스가 실렸고, "자리 비운 사이 뭐가
움직였나"는 글랜스 스트립이 답하며, 터미널은 손가락으로 스크롤됩니다
([GDK-1097], [GDK-1096], [GDK-1098], [GDK-1051], [GDK-871], [GDK-899]).

**CLI가 세션들이 실제로 치는 동사를 배웠습니다.** `gadak list`, `next`,
`show`, `done`, `recent`, `pick` — 블라인드 세션들이 뻗은 손을 실측해서
고른 동사들 — 에 더해 에이전트 노트용 `memory add`/`memory search`, 잘못
분류된 이슈를 옮기는 `edit --type`, `link`에 늘 없던 역동사 `unlink`,
그리고 `workspaces rm`. 쓰기는 스스로를 확인해 주고, `edit -m`은 서식 있는
본문을 소리 없이 평문으로 뭉개기를 거부합니다 ([GDK-992], [GDK-1030],
[GDK-1205], [GDK-1098], [GDK-1001]).

**거절은 시끄러워지고, 쓰기는 안전해졌습니다.** 미러의 부분집합이 표현할 수
없는 JQL 절은 목록을 조용히 넓히는 대신 소리 내어 거절됩니다 ([GDK-1234]).
설정·자격증명 저장은 하나의 원자적 stage-then-rename 소유자를 거치므로 두
저장이 서로를 태워 먹을 수 없습니다 ([GDK-1233], [GDK-1244]). 창이 뜨기
전에 실패한 데스크톱 부팅은 조용히 종료하는 대신 대화상자로 말하고
([GDK-1243]), Linear 담당자 수정은 UI가 광고하는 것과 같은 쓰기 표면을
지납니다 ([GDK-1235]).

**조용한 미러에서 동기화가 빨라졌습니다.** Jira 증분은 겹침 창의 메아리를
다시 받아오는 대신 미러에서 답하고, Confluence 증분은 틱마다 CQL 한 쌍만
물으며 코멘트가 움직이지 않은 본문을 다시 읽지 않습니다 ([GDK-1075],
[GDK-1074]).

그밖에: 페어링 제안이 스캔 가능한 QR로 그려지고 데스크톱에 기기 탭이
생겼습니다 ([GDK-1047]); 업데이트 대화상자는 마크다운 원문을 쏟아붓는 대신
버전을 알리고 릴리스 페이지를 가리킵니다 ([GDK-1246]); 스탠드얼론
워크스페이스는 도움이 될 수 없는 Jira 자격증명 대화상자를 팔지 않습니다
([GDK-1122]); 설치된 에이전트 스킬은 하루 한 번 바이너리를 따라갑니다
([GDK-996]).

이 태그 전에 전체 코드베이스 감사가 한 번 돌았습니다 — 부모 [GDK-1128].

## v0.18.1 — 2026-08-26

터미널이 뒷정리를 배운 패치입니다 — 0.18.0이 터미널을 내놓은 바로 그날
썼습니다.

**터미널을 닫으면 그 안에서 시작한 것까지 전부 닫힙니다.** 셸은 백그라운드
작업을 자기만의 프로세스 그룹에 두기 때문에, 세션을 닫아도 `sleep 999 &` —
또는 잊고 둔 에이전트 — 는 영원히 남았습니다. 이제 닫기는 그 세션의 터미널
위에 있는 모든 프로세스를 훑습니다. 단, 딱 한 번 — 훑기를 신뢰할 수 있는
동안에만. 두 번째 훑기가 *다른* 세션의 셸을 찾아낸 리눅스 실측 실패가 "한
번"의 이유입니다 ([GDK-950]).

**`gadak views open`이 열려 있는 모든 창에 닿습니다** — 예전에는 먼저 폴링한
창이 포커스를 소모해 버려서, 정작 보고 있던 창은 그대로였습니다. 그리고 같은
초에 실행한 두 번의 `views open`도 이제 두 번째를 잃지 않습니다: 중복 판정이
쓰인 시각만이 아니라 내용까지 봅니다 ([GDK-960], [GDK-981]).

**터미널 패널이 왜 못 여는지 말해 줍니다** — Windows에는 PTY가 없다, 토큰에
terminal 스코프가 없다, 네트워크가 끊겼다 — 문장으로, 재시도와 함께, 웹과
폰이 같은 원본에서 같은 문장을 냅니다 ([GDK-944]).

**gadak이 로그 파일을 남기고, doctor가 그것을 건네줍니다.** Finder에서 실행한
앱은 진단 한 줄도 남기지 않고 버렸습니다. 이제 `gadak doctor`가 파일 위치를
알려 주고 최근 에러를 인용합니다. 한 줄이 쓰이기 전에 자격증명은
지워집니다 ([GDK-967]).

그 외: 에이전트 스킬이 부딪힌 벽을 스스로 진단하고, 마찰을 조용히 우회하는
대신 보고하도록 가르칩니다 ([GDK-968]); 낡은 "open" 마커를 남기던 CLI 종료
경로가 마커를 지웁니다 ([GDK-971]); 호스티드 데모의 미러를 내려받아 자기
gadak에서 열 수 있는 파일로 공개합니다 ([GDK-975]); 벤치마크 표는 조용한
머신에서, 데모가 실제로 싣는 코퍼스로 다시 측정했습니다. 태그 전에 이 델타의
릴리스 감사를 돌렸습니다 — 부모 이슈 [GDK-980].

## v0.18.0 — 2026-08-26

**gadak 안에 터미널이 생겼습니다.** ⌘K → 터미널, 또는 `Ctrl+\``. 이슈와 같은
창에 있는 진짜 셸이라, 거기서 코딩 에이전트를 띄우고 그 옆에서 보드가 움직이는
걸 볼 수 있습니다. 웹 탭, macOS 앱, 페어링한 폰 모두 같은 터미널입니다. 한글
조합은 커서 자리에 붙습니다 — 캔버스 위에서는 저절로 되는 일이 아닙니다.
**Beta**로 나갑니다 — 쓸 만하되, 거친 구석은 숨기기보다 표시하는 쪽을
택했습니다 ([GDK-862], [GDK-864], [GDK-865], [GDK-892], [GDK-895],
[GDK-956]).

**셸만 열고 그 외에는 아무것도 열지 않는 토큰.** `gadak pairing mint --scope
terminal`만 셸을 엽니다. `serve`나 `origin` 토큰은 셸을 열지 못합니다 — 대신
`serve` 토큰은 이제 13개 경로 화이트리스트가 아니라 미러 REST 전체에 닿습니다.
터미널 토큰을 폐기하면 그 토큰이 연 셸들이 몇 초 안에 닫히고, 이유를 듣습니다.
루프백은 여전히 토큰이 필요 없습니다 ([GDK-863], [GDK-883]).

**폰 앱이 쓸 수 있는 물건이 됐습니다.** 내가 저장한 뷰 그대로의 이슈, 그 옆의
위키 문서, 페이지 히트까지 보여주는 검색, 스레드에 바로 착지하는 이슈 상세,
그리고 진짜 내 것에만 오는 알림 — 담당·멘션·리오픈. 두 번째 토큰으로 터미널에도
닿고, 페어링을 풀면 그 셸도 함께 잊습니다. 내부 TestFlight 배포는 명령 한
줄입니다 ([GDK-805], [GDK-867], [GDK-870], [GDK-879], [GDK-884], [GDK-885],
[GDK-886], [GDK-887], [GDK-888], [GDK-905], [GDK-906], [GDK-907], [GDK-908],
[GDK-910]).

**한국어 Jira에서도 `gadak create --type Bug`이 됩니다.** 예전에는 현지화된
사이트마다 한 번씩 죽어서 타입 id를 찾아봐야 했습니다. 이제 이름, id,
`epic`/`subtask`, 그리고 실측한 작은 로케일 표가 모두 해석됩니다. 둘 이상
걸리면 추측 대신 충돌을 밝히는 에러입니다 ([GDK-741]).

**위키를 얼마나 받을지가 설치 전에 적혀 있습니다.** 위키는 원래도 켜야 켜지고
스페이스 단위였습니다 — `gadak init --spaces ENG,PROD`, 또는 설정 → 소스 —
그런데 그 말이 설치하고 sync를 돌린 뒤에야 나왔습니다. 전체 Confluence를
받기엔 양이 너무 많아 보여서 시도를 못 했다는 분이 계셨는데, 애초에 그럴
필요가 없었습니다 ([GDK-964]).

**지금 읽고 계신 이 체인지로그가 사이트에도 있습니다.** 두 언어 모두,
복사본이 아니라 저장소에서 렌더한 것입니다. 함께 이 사이트에 없던 검색엔진
기본기도 — canonical 링크, 사이트맵, 릴리스 구조화 데이터.

**작은 것들.** 대시보드에서 이슈를 여는 링크가 벽을 잃지 않습니다
([GDK-880]). `gadak open`이 실행 중인 serve를 찾으려고 포트를 훑지 않습니다
([GDK-859]). WebSocket 업그레이드의 origin을 검사합니다 ([GDK-860]). 낡은
캐시 행 하나가 UI 전체를 날리지 못합니다 ([GDK-835]). `gadak dev --help`가
가진 verb를 전부 적고, `gadak pairing`은 다른 list 명령들처럼 목록을 냅니다
([GDK-946], [GDK-947]). 에이전트 스킬은 설치되지도 않는 파일을 가리키는 대신
동작하는 대시보드 예제를 직접 싣습니다 ([GDK-963]).

## v0.17.3 — 2026-08-25

**폰이 뼈대에서 벗어났습니다.** 페어링한 아이폰이 자기 몫의 `serve` 페어링
스코프로 미러를 읽습니다 — 한쪽으로만 열린 문이라, 그 토큰은 origin
패스스루를 타지 못하고 origin 토큰은 미러를 통째로 못 가져갑니다. connected
워크스페이스에서도 `pairing mint`가 됩니다. 폰 쪽은: 진짜 상태 이름을 그대로
쓰는 조용한 큐가 첫 화면이고, 페어링은 자기가 주장하는 연결을 실제로 증명하며
실패마다 다음 수를 알려줍니다. 검색은 치기 전에 이미 답을 내놓고(최근 검색,
내 저장된 뷰가 칩으로), 이슈 상세의 코멘트 초안은 전송이 실패해도 살아남고,
알림은 담당·멘션·리오픈만 옵니다 — 앱을 보고 있는 동안은 조용합니다
([GDK-796], [GDK-797], [GDK-798], [GDK-799], [GDK-800], [GDK-801],
[GDK-802], [GDK-837]).

**색뿐 아니라 간격·레이아웃·타이포도 내 것입니다.** `ui.tokens`에 축 세 개가
더 붙었고, 하나를 설정하는 것이 나머지를 위태롭게 하지 않습니다 — 각각이
키 단위 병합이라, 거절된 쓰기는 설정을 건드리지 않습니다. 토큰 하나는 그
자체로 경로입니다: `gadak config set ui.tokens.type.terminal 15px`, JSON도
따옴표도 없이. 모르는 이름은 오타로 저장되는 대신 거절됩니다. `gadak config
get ui.tokens.dim-catalog`가 이름마다 기본값·범위·함께 움직여야 하는 것을
적어 줍니다 ([GDK-842], [GDK-849], [GDK-850], [GDK-852], [GDK-853]).

**내 룩은 내 것입니다: 검증이 거절 대신 경고하고 저장합니다.** 대비, 색차,
색각 이상, 토큰 간 관계는 전부 그대로 돌면서 어떤 결과가 될지 말해 줍니다 —
다만 기계가 정말로 이행할 수 없는 것만 거절됩니다. 경고는 진단만이 아니라
다음 수를 함께 줍니다: 대비 줄은 어느 팔레트가 걸리는지와 어디서 고치는지를
짚고, 타입 줄은 함께 움직여야 하는 사다리 전체를 찍습니다. 어떤 룩에서든
빠져나오는 길은 언제나 CLI 한 줄입니다 ([GDK-856], [GDK-857], [GDK-858]).

**대시보드의 링크가 이슈를 열 수 있습니다.** 벽에서 앱의 어느 경로로든 —
이슈, 저장된 뷰, 필터된 목록, 검색 — 이동할 수 있고, 외부 링크는 벽을
대체하는 대신 새 탭으로 열립니다 ([GDK-854]).

## v0.17.2 — 2026-08-25

**에이전트가 대시보드를 만들어 줄 수 있습니다.** HTML 문서 하나 + 이름 붙인
쿼리들, 뷰처럼 저장되고 웹 UI에서 탭 전체로 렌더됩니다. SQL(또는 JQL)은
호스트가 돌려 행만 넘겨주고, 페이지 자신은 네트워크에 닿지 않습니다. 저장하면
열린 탭이 1초쯤 만에 다시 그려지고, 새 미러 데이터는 알아서 밀려 들어옵니다.
차트는 오프라인에서 됩니다 — uPlot이 gadak 안에 들어 있어서 CDN도, 정책
완화도 없습니다 ([GDK-781], [GDK-782], [GDK-792], [GDK-793]).

**원하는 차트 라이브러리는 한 번 받아서 씁니다.** `gadak dashboards lib add
<url>`이 받아서 해시를 고정하고 로컬에서 서빙합니다 — 요청마다 다시
해시하므로, 나중에 변조된 파일은 실행되는 대신 닫힙니다. 대시보드는 자기가
쓰는 라이브러리를 이름으로 적고, 안 쓰는 쪽은 영향이 없습니다. three.js는
바이너리에서 빠져 문서화된 예제가 됐고, 그만큼(750KB) 모든 다운로드가
가벼워졌습니다 ([GDK-808]).

**색은 내 것입니다.** `ui.tokens`, `ui.tokensByTheme`, `ui.dataColors`로
창을 다시 칠하고, 이름은 `ui.tokens.catalog`로 찾습니다. 쓰기는 기본 팔레트가
통과해야 했던 것과 같은 대비 규칙으로 검사하고, 열린 탭은 새로고침 없이 다시
칠해지며, 부팅 캐시 덕에 커스텀한 설치본이 기본 팔레트를 먼저 번쩍이지
않습니다 ([GDK-785], [GDK-786], [GDK-787], [GDK-791]).

**`gadak sql`이 낡은 사본으로 답하는 일이 없어졌습니다.** 접두사 없는 테이블
이름은 마이그레이션 시점에 얼어붙은 스냅샷이 아니라 살아 있는 `local.db`로
떨어집니다 — 그전까지는 조용히, 틀리게 답하고 있었습니다 ([GDK-824]).

**오래됨 경고가 어느 소스가 낡았는지 말합니다.** `mirror last synced 154h
ago`는 모든 소스를 통틀어 가장 오래된 행에서 나온 값이라, 조용한 Confluence
스페이스 하나가 미러 전체를 6일 된 것처럼 보이게 했습니다 — 그동안 `status`는
10분 전 워터마크를 보여주고 있었고요. 이제 소스 이름을 대고 `status`와 같은
시각을 찍습니다 ([GDK-810]).

**읽어서 얻은 페이지 id로 쓸 수 있습니다.** 검색이 그 id를 찍고, JSON 도움말이
페이지의 존재를 인정하며, 에이전트 레시피가 origin이 실제로 받는 id를
냅니다 ([GDK-816]).

**CLI가 삼키던 오타 둘.** `gadak create GDK "…"`는 제목이 프로젝트 키로
시작하는 이슈를 만들어 버렸습니다 — 이제 `--project` 철자를 알려주며
거절하고, 첫 단어가 정말로 이 워크스페이스가 아는 키일 때만 그렇게 합니다.
`config set projects`는 아무 문자열이나 받았습니다 — 이제 모양을 검사하고,
사이트에 존재 여부를 물으며, 어긋난 스코프는 `status`와 `sync`가 양쪽을 다
짚어 줍니다 ([GDK-594], [GDK-809]).

**작은 것들.** 대시보드가 이슈 목록을 덮어 칠하지 못하고 ([GDK-815],
[GDK-821]), 다른 것들처럼 Esc에 답합니다 ([GDK-827]). 접어 둔 문서 트리는
sync 후에도 접힌 채입니다 ([GDK-817]). 피드는 컬럼을 덮는 대신 차지하므로,
피드에서 Esc를 누르면 목록에 착지합니다. 세 언어에 걸친 에러 메시지 14개가
문제만이 아니라 다음 수를 말합니다 ([GDK-828], [GDK-829], [GDK-831]).
목록이 보여주는 발췌와 텍스트 검색이 색인하는 것이 한 곳에서 나오므로 서로
어긋날 수 없습니다 ([GDK-814]).

## v0.17.1 — 2026-08-24

미러가 나눠 쓰는 법을 배운 패치. 2만 개짜리 미러로 하루를 써 보니 gadak
프로세스 둘이 파일 하나를 두고 기다리게 되는 경로가 전부 드러났습니다.

**standalone 워크스페이스의 원본이 SQLite로 옮겨졌습니다.** 내장 트래커가
타이머마다 YAML 파일을 통째로 다시 쓰는 대신 `origin/issuetap.db`에 쓰기마다
한 트랜잭션으로 씁니다. 기존 YAML은 한 번 씨앗이 된 뒤 롤백용으로 그대로
남고, export는 여전히 YAML입니다. 백업 대상은 그 파일입니다 — 앱을 끄고
복사하거나, 켠 채로 `sqlite3 .backup` ([GDK-202]).

**"Database is busy"가 누가 잡고 있는지 말합니다.** Jira에 도달한 쓰기가
로컬 재조회 충돌 때문에 실패하지 않고, 진짜 거절일 때는 에러 코드 대신 이웃
이름을 댑니다 — 다른 앱인지, `serve`인지, CLI인지. `gadak doctor`가 목록을
냅니다. 방문 기록이 자기 연결을 따로 가져가서 읽기가 sync 뒤에 줄서지 않고,
에이전트의 읽기는 즉시 실패하는 대신 점잖게 기다립니다 ([GDK-740],
[GDK-753], [GDK-754], [GDK-757], [GDK-755]).

**가장 느리던 곳이 빨라졌습니다 — 2만 이슈에서 실측.** `gadak issue KEY`가
미러를 통째로 읽는 대신 그 키만 읽습니다. `search --jql`은 사람을 좁게
해석합니다(CLI·서버 양쪽). `doctor`는 모든 문서를 훑는 대신 표본을 봅니다
([GDK-747], [GDK-748], [GDK-749], [GDK-756]).

**제외가 모든 필터에서 됩니다.** 어느 피커의 어느 값에든 ⊘(Alt-클릭도) —
일부 메뉴에만 있던 모달 토글을 대체합니다. Copy JQL은 JQL이 말할 수 있는
곳에 `not in`을 쓰고 못 담은 축을 알려주며, `search --jql`도 같은 부정을
이해합니다 ([GDK-771]).

**좁은 창에서 잘리지 않습니다.** 1100px 아래의 모든 이음매를 감사해 셋을
닫았습니다: 숨지 않던 칩, 글자 중간에서 잘리던 행 컬럼, 그리고 자기가 설명해야
할 레이아웃과 어긋나 있던 최소 너비. CI 검사가 셋을 닫힌 채로 유지합니다
([GDK-758], [GDK-766]).

**gadak.dev에서는:** 한국어 브라우저에 한국어 페이지를 권합니다 — 리다이렉트가
아니라 제안이고, 답을 기억합니다 ([GDK-770]). 사이트를 읽는 에이전트를 위한
`llms.txt`, 그리고 전체화면 영상 대신 읽을 수 있는 크기로 제품을 보여주는 랜딩
미디어 ([GDK-751], [GDK-752]).

## v0.17.0 — 2026-08-23

에이전트의 쓰기가 어른이 된 사이클. 이슈가 자기를 구현한 코드를 보여주고,
쓰기 동사들이 코딩 에이전트가 실제로 보내는 것을 배웠고, 워크스페이스가 명령마다
다시 고르는 물건이기를 그만뒀습니다.

**이슈가 자기 PR을 압니다.** PR·커밋·배포·빌드와 거기 붙은 사람들이, connected
사이트에서는 미러로 오고 standalone에서는 쓰기까지 됩니다. `gadak dev scan`이
저장소의 PR을 한 번에 이슈로 쓸어 담고, `gadak dev link`가 하나를 쓰며, 웹은
GitHub 링크를 앱 안에서 엽니다. 그 링크들은 다음 sync에도 살아남습니다. 패널이
비어 있으면 왜 비었는지 말합니다 ([GDK-495], [GDK-496], [GDK-497], [GDK-527],
[GDK-531], [GDK-536], [GDK-537], [GDK-538], [GDK-539], [GDK-540], [GDK-541],
[GDK-555], [GDK-562], [GDK-589], [GDK-592]). 이슈 링크 때문에 앱을 나갈
일도 없어졌습니다: `gadak link A B --type blocks`, 또는 상세 패널
([GDK-19], [GDK-85]).

**프로젝트가 요구하는 것을 실제로 해내는 쓰기 동사들.** `create`와 `edit`가
필수 커스텀 필드용 `--field alias=value`를 받고, 생성 대화상자는 이 프로젝트와
이 이슈 유형이 실제로 무엇을 요구하는지 배웁니다. `transition`은
`--resolution`·`--field`·코멘트를 함께 나릅니다. `edit`는 fix version과
컴포넌트를 이름으로 씁니다. `assign`은 이메일뿐 아니라 이름이나 account id도
받습니다. 타입이 틀린 필드는 빈 문자열로 써지는 대신 거절됩니다. 거절된
parent는 고를 수 있었던 에픽 목록을 알려줍니다 ([GDK-254], [GDK-330],
[GDK-509], [GDK-513], [GDK-514], [GDK-515], [GDK-516], [GDK-517], [GDK-635],
[GDK-643]).

**Jira에 도달한 쓰기는 성공입니다** — 직후의 로컬 재조회가 실패했더라도
([GDK-740]). 벌크 읽기는 여러 키나 `--keys -`를 받고, 조용히 버려지는 것이
없습니다 ([GDK-328], [GDK-425]). `gadak claim KEY`가 이슈를 한 수에 가져오고,
`gadak issue`는 일이 얼마나 묵었는지 보여줍니다 — `wait 3d · progress 5h`
([GDK-591]).

**쓰기가 누가 했는지를 나릅니다.** `GADAK_ACTOR`가 에이전트의 이름을 대고,
웹은 봇의 작업에 배지를 붙입니다 — 기계의 편집이 내 것과 구분되지 않는 일이
없도록. standalone 워크스페이스가 내 언어를 쓰고, 제한된 이슈는 공개 이슈와
다르게 보입니다 ([GDK-519], [GDK-586], [GDK-588], [GDK-590], [GDK-593],
[GDK-597]).

**워크스페이스가 고른 채로 있습니다.** `gadak workspace use <name>`이 기본값을
저장합니다. 페어링은 무엇인지·무엇이 실패했는지 사실대로 말하고, 묶인
워크스페이스가 조용히 다른 사이트로 향할 수 없으며, origin을 교체하면 거기서
파생된 행들도 함께 갑니다 ([GDK-418], [GDK-433], [GDK-449], [GDK-452],
[GDK-453], [GDK-490], [GDK-561], [GDK-677], [GDK-678]).

**한국어 검색이 복합어 안의 단어를 찾습니다.** 그리고: fix version이 id를
간직하고 프로젝트의 릴리스 카탈로그가 미러에 오며, sprint는 쿼리할 수 있는
컬럼이고, JQL `parent =` / `parent IN`이 로컬에서 걸립니다 ([GDK-259],
[GDK-444], [GDK-518], [GDK-521], [GDK-532], [GDK-329]).

**`rm gadak.db`가 더 이상 아무것도 앗아가지 않습니다.** 저장된 뷰·방문·검색
기록이 미러 밖으로 나왔습니다 — 미러는 버려도 되는 쪽이니까요 ([GDK-105]).

**에이전트용.** `gadak pick`이 할 일을 고릅니다. `gadak recents`가 CLI가 읽은
것을 되짚습니다. `gadak sync --if-stale 15m`은 에이전트가 눈감고 부를 수 있는
세션 오프너입니다. 배치 쓰기는 키마다 정직하게 답합니다. 이슈를 닫는 것은
재시도해도 안전한 왕복 한 번입니다 ([GDK-500], [GDK-501], [GDK-502],
[GDK-503], [GDK-598], [GDK-599]).

**창의 일관성.** Esc는 겨눈 것을 닫고 다른 것은 닫지 않습니다. 타입 크기는
넷이고 그 사이는 없습니다. 비어 있음은 공백이 아니라 말이 있는 상태입니다.
스크린이 필요한 전이는 인라인으로 묻고, 컴포넌트와 parent는 제자리에서
고쳐지며, 스토리는 자식을 보여줍니다. 저장된 뷰는 한 종류입니다. 빨리 끝난
읽기는 스켈레톤을 아예 그리지 않습니다. 그리고 일본어 카탈로그 전체
([GDK-83], [GDK-86], [GDK-121], [GDK-129], [GDK-130], [GDK-316], [GDK-437],
[GDK-604], [GDK-613], [GDK-617], [GDK-626], [GDK-737], [GDK-738],
[GDK-739]).

**데스크톱.** 두 번째 실행이 경쟁자를 띄우는 대신 이미 있는 창을 올립니다.
Windows에서는: `gadak://` 링크가 동작하고, `install-cli`가 Windows 말을 하며,
알리지 않았으면서 알렸다고 하지 않습니다 ([GDK-349], [GDK-350], [GDK-351],
[GDK-353], [GDK-658], [GDK-700]).

**네트워크 감사.** 빈 호스트는 루프백이 아닌 바인드로 셉니다 — 그래서
`serve`가 다른 노출과 똑같이 `--allow-remote`를 요구합니다 ([GDK-542]).
Linear의 rate limit은 죽음이 아니라 재시도이고, Linear만 있는 워크스페이스도
설정된 워크스페이스입니다 ([GDK-263], [GDK-654]). Web Push 클라이언트는
사라졌습니다: 서버가 404로 답하는 엔드포인트를 부르고 있었고, 벤더 푸시
서비스는 이 프로젝트가 하지 않는 아웃바운드입니다 ([GDK-711]).

**gadak 자신의 백로그가 공개됐습니다** — gadak.dev에, 앞문과 데모를 곁에
두고. Windows 경고를 설명하는 페이지도 함께 ([GDK-211], [GDK-389],
[GDK-676]).

## v0.16.1 — 2026-08-20

0.16이 시작한 것을 끝내는 릴리스.

**Linear가 세 번째 트래커가 됐고, gadak이 거기에 씁니다.** 워크스페이스 설정의
`"linear"` 블록과 `gadak sync --source linear`가 이슈·코멘트·라벨·첨부를
미러링합니다. 쓰기는 그 행을 소유한 origin으로 라우팅되고, Linear가 아직 못
하는 것은 반쯤 적용하는 대신 정직하게 거절합니다. Jira·standalone·Linear가
모두 같은 쓰기 동사에 답합니다 ([GDK-263], [GDK-359], [GDK-360], [GDK-361]).

**위키가 읽기 전용이기를 그만뒀습니다.** 페이지 생성, 제목·본문 편집, 코멘트 —
전부 origin을 통과해서, CLI에서도 REST API에서도 ([GDK-344], [GDK-380],
[GDK-381], [GDK-382]).

**gadak 프로세스 둘이 standalone 워크스페이스를 두고 다투지 않습니다.**
데스크톱 앱이 `serve`처럼 자기 origin을 알리므로, 앱과 CLI가 원본 파일을 함께
붙들 수 없습니다. 승인된 쓰기는 답을 받기 전에 디스크에 있고, 영속시키지 못한
쓰기는 그런 척하는 대신 실패합니다. standalone의 실패가 자신을 "자격증명 없음"
으로 보고하지 않고, 워크스페이스를 변환할 때 내 로컬 전용 이슈들에 무슨 일이
일어나는지 말해 줍니다 ([GDK-241], [GDK-333], [GDK-340], [GDK-342],
[GDK-343], [GDK-345], [GDK-346], [GDK-347], [GDK-348]).

**에이전트가 standalone의 존재를 배웁니다.** 내장 스킬이 그 단어를 알고, CLI가
어느 origin을 말하는지 밝히며, `transition`이 대상마다 `status_id`를 대고 —
읽기 경로가 방금 건네준 그 값을 받습니다. 에이전트가 계속 실패하던 고리가
그것이었습니다. `issues_full`에 `description_text`가 생겼고, standalone
`init`이 미러를 채워 아무것도 빈 채로 시작하지 않습니다 ([GDK-239],
[GDK-312], [GDK-313], [GDK-363], [GDK-364], [GDK-365], [GDK-366], [GDK-367],
[GDK-368], [GDK-371], [GDK-376]).

**제품과 어긋나지 않는 문서.** 설치 페이지가 standalone의 존재를 인정하고,
FAQ가 `rm -rf ~/.gadak`을 권하기를 그만두며, 네트워크가 자기 페이지를 갖고,
export/import에 드디어 문단이 생겼습니다 ([GDK-271], [GDK-372], [GDK-373],
[GDK-374], [GDK-375], [GDK-601]).

## v0.16.0 — 2026-08-19

gadak이 쓸모 있기 위해 Atlassian 계정을 필요로 하지 않게 되고, 돌기 위해 Mac을
필요로 하지 않게 되고, 사람들이 실제로 분류에 쓰는 필드에 대해 읽기 전용이기를
그만둔 릴리스.

**Atlassian 계정 없는 워크스페이스.** standalone: origin이 gadak 안에서 돌고
gadak과 함께 다니는 미니멀 트래커입니다. 미러는 여전히 버려도 되는 캐시고
모든 쓰기는 여전히 origin을 통과합니다 — 바뀐 것은 origin이 누구인가뿐입니다.
워크스페이스는 origin 하나에 묶이므로, 자격증명을 연결한다고 해서 조용히 다른
곳을 가리키게 되지 않습니다. standalone 위키도 같은 경로로 씁니다 ([GDK-183],
[GDK-237], [GDK-238], [GDK-247], [GDK-267]).

**Windows와 Linux.** Windows에는 포터블 팩, 설치 경로, 동작하는
`install-cli`, Scoop 매니페스트, 그리고 첫 실행에도 살아남는 `gadak://`
링크가 생겼습니다. Linux에는 brew 옆의 tarball 설치와 AUR 패키징 킷이.
Omarchy에는 *내* 미러에서 무엇이 바뀌었는지 보여주는 바 위젯이 ([GDK-115],
[GDK-116], [GDK-208], [GDK-209], [GDK-225], [GDK-229], [GDK-246],
[GDK-293]).

**읽는 자리에서 이슈를 고칩니다.** 마감일을 상세 패널에서 설정하고 지우며,
Jira의 거절은 읽을 수 있는 문장으로 보여줍니다. 설명은 평문으로 편집하되
서식이 파괴되기 전에 막습니다. `s`/`a`/`l`이 되는 곳이면 `p`가 우선순위
메뉴를 엽니다. 무엇이 편집 가능한지는 고정 목록이 아니라 그 이슈 자신의
메타데이터에서 나오므로, 내 사이트의 커스텀 필드도 포함됩니다 ([GDK-82],
[GDK-223], [GDK-249], [GDK-250], [GDK-251], [GDK-322], [GDK-323],
[GDK-331], [GDK-332]).

**팔레트에서 방금 친 내용으로 이슈를 만들 수 있고** ([GDK-217], [GDK-218]),
답이 뻔한 필수 필드는 질문이기를 그만두며, 코멘트를 달면 드디어 도착했다고
말해 줍니다 ([GDK-300], [GDK-301], [GDK-302]).

**영어가 아닌 Jira가 조용히 빈손으로 돌아오지 않습니다.** 상태·우선순위·이슈
유형이 표시 이름 대신 id와 카테고리로 걸립니다 — `status = 'In Progress'`는
한국어 계정에서 0행이고, 그 부류의 조용히 틀린 답이 닫혔습니다 ([GDK-161],
[GDK-248], [GDK-272], [GDK-275]). 한국어 복합어 중간 검색도 됩니다
([GDK-259]).

**알리기만 하고, 행동하지 않는 업데이트.** gadak은 새 릴리스를 알아채고, 내
플랫폼에 맞는 말을 하고, 릴리스 노트를 앱 안에서 렌더합니다. 자기를 스스로
업데이트하지는 않습니다 ([GDK-213], [GDK-214], [GDK-215], [GDK-216]).

**작은 것들.** 차가운 첫 실행이 모두를 자기 뒤에 줄 세우지 않고, 경합하는
쓰기는 즉사 대신 기다리며, 백그라운드 sync가 자기를 시작한 서버보다 오래
살지 않습니다 ([GDK-270], [GDK-282], [GDK-305]). 호스티드 데모가 제품
화면으로 열리고, 피드백 채널이 설정과 macOS 도움말 메뉴에 있습니다
([GDK-335], [GDK-336]). 읽기 전용 Linear 클라이언트가 기초 공사로
들어왔습니다 — 워크스페이스에는 일부러 아직 연결하지 않았고, 그건 0.16.1의
몫입니다 ([GDK-258], [GDK-261], [GDK-263], [GDK-274]).

## v0.15.2 — 2026-08-17

설정이 화면이기를 그만둔 릴리스.

**설정 대화상자가 고치는 모든 필드가 CLI 동사이기도 합니다.** `gadak config
list | get | set`과 설정 API가 한 테이블을 통과하므로 서로 어긋날 수
없습니다 — 즉 에이전트가 워크스페이스를 처음부터 끝까지 세팅할 수 있습니다.
테마는 워크스페이스 설정 파일에 있어서, UI에서 고르는 것과 터미널에서
설정하는 것이 같은 행위입니다 ([GDK-190], [GDK-193]).

**다크 셋, 그중 하나가 내 것.** `dark`는 중성-차가운 차콜, `ink`는 새로 만든
푸른 검정, `ember`는 이전의 따뜻한 다크를 그대로 지킵니다 ([GDK-190]).

**작은 것들.** 숫자만 쳐도 어느 프로젝트에서든 그 이슈를 찾습니다 — 모든 검색
표면에서 ([GDK-186]). 설정 대화상자가 탭마다 미러 블록을 되풀이하지 않습니다
([GDK-188]). 메뉴가 등 뒤에서 뭘 설치하지 않습니다 — 그건 설정 → 통합이
하고, 이미 설치된 것을 말해 줍니다 ([GDK-189], [GDK-191]).

## v0.15.1 — 2026-08-17

- `gadak raycast install`이 Raycast 확장을 바이너리 안에 담고 다니므로, brew나
  앱으로 설치했다면 체크아웃이 필요 없습니다 ([GDK-182]).
- ⌘K 팔레트가 비지 않습니다: 빈 질의에 최근 본 것 아래로 최근 갱신된 이슈와
  내 저장된 뷰가 나옵니다 ([GDK-184]).
- 설정 → 통합(데스크톱)이 gadak이 설치하는 에이전트 표면들을 나열하고, 이미
  있는 것을 정직하게 감지하며, 실시간 로그를 보여줍니다 ([GDK-185]).

## v0.15.0 — 2026-08-17

gadak을 바깥으로 여는 릴리스. 뷰나 이슈는 어느 앱에나 건넬 수 있는 링크가 되고,
검색은 남의 키 입력 아래에 앉을 만큼 빨라졌으며, 라이트와 같은 기준으로 만든
다크 테마가 생겼습니다.

**gadak의 한 조각이 링크로 이동합니다.** `gadak://` 딥링크로 앱의 모든 자리에
주소가 붙었고, gadak은 자기가 소비하는 링크를 만들기도 합니다: UI의 링크 복사
액션, CLI의 `gadak issue KEY --link` ([GDK-119], [GDK-124], [GDK-163],
[GDK-164]).

**남의 앱 UI를 굴릴 만큼 빠른 검색.** 이슈 키를 치면 그 이슈가 나옵니다 —
2만 개짜리 미러에서 최악의 경우가 1.6초에서 110ms로. 런처 확장이 로컬처럼
느껴지게 만드는 것이 그것입니다 ([GDK-117], [GDK-166], [GDK-170]).

**이슈가 자기 parent를 댈 수 있습니다.** `gadak create --parent`와 `gadak
edit --parent`가 하위 이슈 관계를 Jira를 통해 씁니다 ([GDK-19], [GDK-86]).

**제대로 만든 다크 테마.** 따뜻한 바탕, 먹빛 전경, 라이트와 같은 종이 은유,
그리고 첫 페인트에 번쩍임 없음. 두 팔레트 모두 같은 실측 하한을 통과합니다:
상태 색은 정상 시각과 색각 이상에서 모두 구분되고, 성공과 실패를 색만으로
말하지 않습니다 ([GDK-154], [GDK-156], [GDK-157], [GDK-158], [GDK-159],
[GDK-162], [GDK-171]).

**목록이 목록처럼 굽니다.** 행의 오른쪽은 훑을 수 있는 컬럼이고, 마지막 행이
반으로 잘리지 않으며, Esc는 보고 있는 것을 닫고, 목록을 덮는 패널은 덮는다고
말합니다 ([GDK-128], [GDK-131], [GDK-132], [GDK-133]).

**한국어 입력이 검색창과 싸우기를 그만뒀습니다.** 조합 중인 음절은 질의가
아니고, 초성 매칭은 제품 전체에서 사라졌습니다 — 의도하지 않은 것을 물어오고
있었습니다 ([GDK-168], [GDK-169]).

**가장자리에서의 정직함.** 호스티드 데모가 답할 수 없는 동사를 광고하지
않습니다 ([GDK-52]). 읽기 전용 홈은 시작 거부가 아니라 경고입니다
([GDK-149], [GDK-173]). 복사는 복사됐다는 뜻이고, 첨부는 많아야 한 번
받아오며, 데스크톱 앱이 런타임을 두 번 로드하지 않습니다 ([GDK-150],
[GDK-177], [GDK-178]).

## v0.14.2 — 2026-08-16

첫 10분에 대한, 그리고 토큰이 죽는 날에 대한 릴리스. 여기 있는 것은 새 능력이
아니라, 이미 있던 능력이 드디어 자기가 뭘 하는지 말하게 된 것입니다.

**모든 토큰 함정을 401 이후가 아니라 붙여넣기 전에 이름으로 말합니다** —
거절된 토큰은 뭘 먼저 쓰지 않고도 복구됩니다 ([GDK-68], [GDK-69],
[GDK-98]). 만료는 sync가 죽기 전에 경고합니다 ([GDK-67]).

**프로젝트를 하나도 안 고르는 것은 미완성 양식이 아니라 선택입니다**
([GDK-99]).

**`gadak skill install`이 업그레이드를 업그레이드로 다룹니다**, 그리고 내장
스킬이 CLI에 실제로 있는 동사를 압니다 ([GDK-91], [GDK-92]).

**조용한 위키는 sync 비용이 거의 없습니다** — 변하지 않은 Confluence에 대한
한 틱은 페이지 본문을 하나도 읽지 않습니다 ([GDK-113]).

**`gadak issue <KEY> --derive`가 파생 컬럼이 어디서 왔는지 보여줍니다**
([GDK-111]).

**작은 것들.** 기록이 순서를 지킵니다 ([GDK-26]). `gadak sql`이 낡은 미러를
경고합니다 ([GDK-90]). 미러를 열 때 이 빌드가 쓸 수 없는 검색 색인을
고칩니다 ([GDK-112]). 브라우즈 창이 Escape에 양보합니다 ([GDK-78]). 검색
도움말이 터치에서 동작합니다 ([GDK-53]).

## v0.14.1 — 2026-08-15

gadak 자신의 백로그 위에서 gadak을 쓴 하루를, 나온 그대로 내보냅니다.

**첫 CLI 쓰기 동사들:** `gadak create`, `gadak attach`, `gadak edit`.

**macOS 앱은 알림 전용입니다.** 한 번도 실행되지 않았고 한 번도 신뢰를 얻지
못한 인앱 자가 업데이터는 사라졌고, 이 릴리스는 일부러 데스크톱 zip을 내지
않습니다 ([GDK-58], [GDK-61]).

**호스티드 데모가 사람들이 실제로 두드리는 곳에서 동작합니다:** 인앱 브라우저
안에서, 폰 너비에서 읽히는 첫 페인트로 ([GDK-23], [GDK-51]).

**실패가 무슨 일이 있었는지 말합니다.** 잘린 키 목록은 몇 개가 빠졌는지 말하고,
거절된 자격증명은 한 소스가 아니라 모든 소스의 watch 루프를 멈춥니다
([GDK-24], [GDK-35], [GDK-48]). 우선순위 색은 계정의 언어가 아니라 rank를
읽습니다. 부팅 중의 키 입력은 버려지는 대신 잡아 둡니다 ([GDK-46],
[GDK-76]).

## v0.14.0 — 2026-08-15

신뢰에 대한 릴리스: 조용히 대신 크게 실패하는 표면들, 코드와 맞는 문서, 형용사
대신 실측한 숫자.

**에이전트의 첫 호출은 성공하거나, 왜 아닌지 말합니다.** `gadak_search`는
`query`를 받고, 모든 툴 에러는 `ERROR:`로 시작하며 받은 키를 되읊고, 크기 상한을
넘는 응답은 오래된 코멘트를 덜어내고 `truncated`라고 말합니다 — 온전한 척하는
대신.

**세 가지는 그 위에 지어도 되는 약속입니다:** `issues_full`과 RECIPES 쿼리,
`gadak sql` stdout, `views open --keys -`.

**`gadak export` / `gadak import`가 진짜 아쉬울 것들을 왕복시킵니다** — 저장된
뷰, watch, 즐겨찾기 — 자격증명도 사이트 URL도 담지 않고.

**숫자, gadak이 지는 행까지 함께.** 살아 있는 2,853 이슈 Cloud 프로젝트 대비
실측: 단순 필터 42배, 에픽 `GROUP BY` 162배, 그리고 REST로 20분쯤 걸리는
리오픈 카운트가 로컬에서 14.5ms.

**`brew install midagedev/tap/gadak`이 이제 앱입니다**; `gadak-cli`가 CLI 전용
formula입니다. 그리고 한국어 README, 프로젝트 선택에 대해 사실이 아닌 것을
주장하기를 그만둔 설정 대화상자.

## v0.13.0 — 2026-08-14

검색·기록·에이전트의 창을 한자리에 놓는 릴리스.

**모든 것을 검색하는 검색창 하나.** ⌘K가 모든 이슈와 문서를 한 색인에서
질의합니다 — 목록의 필터 칩은 무시하고. 목록 위의 상자는 원래 하던 일을 계속
합니다: 이미 있는 것을 좁히기.

**기록은 미러 옆의 파일입니다.** 이슈·문서·검색이 `~/.gadak/local.db`의 한
타임라인에 있으므로, 미러를 버려도 그것은 버려지지 않고, 에이전트는 내가 방문한
것과 내가 가진 것을 `gadak sql` 한 번으로 조인할 수 있습니다.

**창이 에이전트를 따라갑니다.** 임의의 이슈 키 집합이 일급 뷰이므로 `gadak
views open --keys -`가 에이전트의 답을 내 창에 띄웁니다. `gadak views open`은
gadak에서 열고, `gadak open`은 Jira로 나갑니다. 셸이 없는 호스트는 새 MCP
툴로 같은 것을 합니다.

**Jira URL을 붙여넣으면 필터가 됩니다.** 네비게이터 URL이나 `jql=` 절이
해당하는 칩을 적용하고, Copy JQL이 돌아가는 길이며, 지원하지 않는 부분집합은
조용히 버려지는 대신 나열됩니다. 내 Jira 저장 필터가 사이드바에 나오고,
`gadak views`가 그것을 나열·표시·열기·저장합니다.

**위키 스코프가 진짜가 됐습니다.** Confluence 스페이스마다 자기 워터마크를
갖고, 새로 고른 스페이스는 전체를 채우며, 스코프를 떠난 스페이스는 제거됩니다.

**사람은 이메일이 아니라 account id로 맞춥니다.** 사람 필터가 내 사이트가
이메일 주소를 보여주는지에 더 이상 기대지 않습니다 (#1, @elppaaa 감사합니다) —
JQL·저장된 뷰·필터·멤버 디렉터리 전반에서.

**macOS 창을 드래그할 수 있습니다** (#2, @wafe 감사합니다).

**작은 것들.** 코멘트만 바뀐 위키 편집도 미러에 도달하고, 변하지 않은 페이지는
버전을 올리지 않으며, 삭제된 이슈는 단일 항목 sync로 묘비가 세워집니다. 모르는
`--profile`은 진짜 목록과 함께 에러를 냅니다. 업로드 후 미러 재조회가 실패하면
그런 척하는 대신 문서화된 에러를 돌려줍니다.

## v0.12.0 — 2026-08-13

룩과 개명의 릴리스. gadak은 가닥입니다: 코팅 없는 종이, 먹, 쪽빛 실 한 올.

**어두운 대시보드가 아니라 종이.** 마크는 두 획으로 그린 가입니다. 수정구
대시보드와 TUI는 사라졌습니다.

**gadak으로 개명** — 바이너리, 홈 디렉터리(`~/.gadak`), 환경변수
접두사(`GADAK_*`), MCP 툴, 모듈 경로, 데스크톱 번들 id. 기존 `~/.scry` 트리는
첫 실행에 이름이 바뀌고, `GADAK_*` 쪽이 비어 있는 자리에서는 `SCRY_*`도 여전히
읽습니다.

**라벨과 우선순위가 바꿀 수 있는 것이 됐습니다.** 라벨은 목록에서 보이고,
이슈에서 편집되며, 벌크 바에서 선택 전체에 적용됩니다(`s`·`a` 옆의 `l`).
우선순위는 사이트 자신의 카탈로그에서 id로 씁니다. 제목도 편집됩니다.

**데스크톱 앱에서 워크스페이스가 동작하고**, 자격증명이 있는 워크스페이스마다
자기 sync 루프를 갖습니다.

**큰 미러에서 문서 목록이 얼어붙지 않습니다** — 1만 페이지 창에서 4,433ms가
68ms로.

**작은 것들.** 네이티브 타이틀바가 사라지고 창 컨트롤이 사이드바로
들어갔습니다. `gadak skill install`이 MCP 없이 Claude Code 스킬을 심고,
`gadak install-cli`가 돌고 있는 바이너리를 PATH에 놓습니다. `gadak doctor`는
버그 리포트에 붙여넣을 수 있는, 가려진 진단을 찍습니다. `gadak api`는 원시
Atlassian REST 탈출구이고, 낯선 호스트에서는 거절됩니다.

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
[GDK-90]: https://gadak.dev/backlog/#/?ks=GDK-90
[GDK-91]: https://gadak.dev/backlog/#/?ks=GDK-91
[GDK-92]: https://gadak.dev/backlog/#/?ks=GDK-92
[GDK-98]: https://gadak.dev/backlog/#/?ks=GDK-98
[GDK-99]: https://gadak.dev/backlog/#/?ks=GDK-99
[GDK-105]: https://gadak.dev/backlog/#/?ks=GDK-105
[GDK-111]: https://gadak.dev/backlog/#/?ks=GDK-111
[GDK-112]: https://gadak.dev/backlog/#/?ks=GDK-112
[GDK-113]: https://gadak.dev/backlog/#/?ks=GDK-113
[GDK-115]: https://gadak.dev/backlog/#/?ks=GDK-115
[GDK-116]: https://gadak.dev/backlog/#/?ks=GDK-116
[GDK-117]: https://gadak.dev/backlog/#/?ks=GDK-117
[GDK-119]: https://gadak.dev/backlog/#/?ks=GDK-119
[GDK-121]: https://gadak.dev/backlog/#/?ks=GDK-121
[GDK-124]: https://gadak.dev/backlog/#/?ks=GDK-124
[GDK-128]: https://gadak.dev/backlog/#/?ks=GDK-128
[GDK-129]: https://gadak.dev/backlog/#/?ks=GDK-129
[GDK-130]: https://gadak.dev/backlog/#/?ks=GDK-130
[GDK-131]: https://gadak.dev/backlog/#/?ks=GDK-131
[GDK-132]: https://gadak.dev/backlog/#/?ks=GDK-132
[GDK-133]: https://gadak.dev/backlog/#/?ks=GDK-133
[GDK-149]: https://gadak.dev/backlog/#/?ks=GDK-149
[GDK-150]: https://gadak.dev/backlog/#/?ks=GDK-150
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
[GDK-270]: https://gadak.dev/backlog/#/?ks=GDK-270
[GDK-271]: https://gadak.dev/backlog/#/?ks=GDK-271
[GDK-272]: https://gadak.dev/backlog/#/?ks=GDK-272
[GDK-274]: https://gadak.dev/backlog/#/?ks=GDK-274
[GDK-275]: https://gadak.dev/backlog/#/?ks=GDK-275
[GDK-282]: https://gadak.dev/backlog/#/?ks=GDK-282
[GDK-293]: https://gadak.dev/backlog/#/?ks=GDK-293
[GDK-300]: https://gadak.dev/backlog/#/?ks=GDK-300
[GDK-301]: https://gadak.dev/backlog/#/?ks=GDK-301
[GDK-302]: https://gadak.dev/backlog/#/?ks=GDK-302
[GDK-305]: https://gadak.dev/backlog/#/?ks=GDK-305
[GDK-312]: https://gadak.dev/backlog/#/?ks=GDK-312
[GDK-313]: https://gadak.dev/backlog/#/?ks=GDK-313
[GDK-316]: https://gadak.dev/backlog/#/?ks=GDK-316
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
[GDK-418]: https://gadak.dev/backlog/#/?ks=GDK-418
[GDK-425]: https://gadak.dev/backlog/#/?ks=GDK-425
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
[GDK-604]: https://gadak.dev/backlog/#/?ks=GDK-604
[GDK-613]: https://gadak.dev/backlog/#/?ks=GDK-613
[GDK-617]: https://gadak.dev/backlog/#/?ks=GDK-617
[GDK-626]: https://gadak.dev/backlog/#/?ks=GDK-626
[GDK-635]: https://gadak.dev/backlog/#/?ks=GDK-635
[GDK-643]: https://gadak.dev/backlog/#/?ks=GDK-643
[GDK-654]: https://gadak.dev/backlog/#/?ks=GDK-654
[GDK-658]: https://gadak.dev/backlog/#/?ks=GDK-658
[GDK-676]: https://gadak.dev/backlog/#/?ks=GDK-676
[GDK-677]: https://gadak.dev/backlog/#/?ks=GDK-677
[GDK-678]: https://gadak.dev/backlog/#/?ks=GDK-678
[GDK-711]: https://gadak.dev/backlog/#/?ks=GDK-711
[GDK-738]: https://gadak.dev/backlog/#/?ks=GDK-738
[GDK-739]: https://gadak.dev/backlog/#/?ks=GDK-739
[GDK-740]: https://gadak.dev/backlog/#/?ks=GDK-740
[GDK-737]: https://gadak.dev/backlog/#/?ks=GDK-737
[GDK-700]: https://gadak.dev/backlog/#/?ks=GDK-700
[GDK-771]: https://gadak.dev/backlog/#/?ks=GDK-771
[GDK-202]: https://gadak.dev/backlog/#/?ks=GDK-202
[GDK-747]: https://gadak.dev/backlog/#/?ks=GDK-747
[GDK-748]: https://gadak.dev/backlog/#/?ks=GDK-748
[GDK-749]: https://gadak.dev/backlog/#/?ks=GDK-749
[GDK-751]: https://gadak.dev/backlog/#/?ks=GDK-751
[GDK-752]: https://gadak.dev/backlog/#/?ks=GDK-752
[GDK-753]: https://gadak.dev/backlog/#/?ks=GDK-753
[GDK-754]: https://gadak.dev/backlog/#/?ks=GDK-754
[GDK-755]: https://gadak.dev/backlog/#/?ks=GDK-755
[GDK-756]: https://gadak.dev/backlog/#/?ks=GDK-756
[GDK-757]: https://gadak.dev/backlog/#/?ks=GDK-757
[GDK-758]: https://gadak.dev/backlog/#/?ks=GDK-758
[GDK-766]: https://gadak.dev/backlog/#/?ks=GDK-766
[GDK-770]: https://gadak.dev/backlog/#/?ks=GDK-770
[GDK-785]: https://gadak.dev/backlog/#/?ks=GDK-785
[GDK-786]: https://gadak.dev/backlog/#/?ks=GDK-786
[GDK-787]: https://gadak.dev/backlog/#/?ks=GDK-787
[GDK-791]: https://gadak.dev/backlog/#/?ks=GDK-791
[GDK-781]: https://gadak.dev/backlog/#/?ks=GDK-781
[GDK-782]: https://gadak.dev/backlog/#/?ks=GDK-782
[GDK-792]: https://gadak.dev/backlog/#/?ks=GDK-792
[GDK-793]: https://gadak.dev/backlog/#/?ks=GDK-793
[GDK-808]: https://gadak.dev/backlog/#/?ks=GDK-808
[GDK-797]: https://gadak.dev/backlog/#/?ks=GDK-797
[GDK-798]: https://gadak.dev/backlog/#/?ks=GDK-798
[GDK-800]: https://gadak.dev/backlog/#/?ks=GDK-800
[GDK-594]: https://gadak.dev/backlog/#/?ks=GDK-594
[GDK-809]: https://gadak.dev/backlog/#/?ks=GDK-809
[GDK-810]: https://gadak.dev/backlog/#/?ks=GDK-810
[GDK-796]: https://gadak.dev/backlog/#/?ks=GDK-796
[GDK-799]: https://gadak.dev/backlog/#/?ks=GDK-799
[GDK-801]: https://gadak.dev/backlog/#/?ks=GDK-801
[GDK-802]: https://gadak.dev/backlog/#/?ks=GDK-802
[GDK-837]: https://gadak.dev/backlog/#/?ks=GDK-837
[GDK-824]: https://gadak.dev/backlog/#/?ks=GDK-824
[GDK-814]: https://gadak.dev/backlog/#/?ks=GDK-814
[GDK-815]: https://gadak.dev/backlog/#/?ks=GDK-815
[GDK-816]: https://gadak.dev/backlog/#/?ks=GDK-816
[GDK-817]: https://gadak.dev/backlog/#/?ks=GDK-817
[GDK-821]: https://gadak.dev/backlog/#/?ks=GDK-821
[GDK-827]: https://gadak.dev/backlog/#/?ks=GDK-827
[GDK-828]: https://gadak.dev/backlog/#/?ks=GDK-828
[GDK-829]: https://gadak.dev/backlog/#/?ks=GDK-829
[GDK-831]: https://gadak.dev/backlog/#/?ks=GDK-831
[GDK-842]: https://gadak.dev/backlog/#/?ks=GDK-842
[GDK-849]: https://gadak.dev/backlog/#/?ks=GDK-849
[GDK-850]: https://gadak.dev/backlog/#/?ks=GDK-850
[GDK-852]: https://gadak.dev/backlog/#/?ks=GDK-852
[GDK-853]: https://gadak.dev/backlog/#/?ks=GDK-853
[GDK-854]: https://gadak.dev/backlog/#/?ks=GDK-854
[GDK-856]: https://gadak.dev/backlog/#/?ks=GDK-856
[GDK-857]: https://gadak.dev/backlog/#/?ks=GDK-857
[GDK-858]: https://gadak.dev/backlog/#/?ks=GDK-858
[GDK-862]: https://gadak.dev/backlog/#/?ks=GDK-862
[GDK-863]: https://gadak.dev/backlog/#/?ks=GDK-863
[GDK-883]: https://gadak.dev/backlog/#/?ks=GDK-883
[GDK-864]: https://gadak.dev/backlog/#/?ks=GDK-864
[GDK-835]: https://gadak.dev/backlog/#/?ks=GDK-835
[GDK-892]: https://gadak.dev/backlog/#/?ks=GDK-892
[GDK-895]: https://gadak.dev/backlog/#/?ks=GDK-895
[GDK-865]: https://gadak.dev/backlog/#/?ks=GDK-865
[GDK-805]: https://gadak.dev/backlog/#/?ks=GDK-805
[GDK-964]: https://gadak.dev/backlog/#/?ks=GDK-964
[GDK-956]: https://gadak.dev/backlog/#/?ks=GDK-956
[GDK-950]: https://gadak.dev/backlog/#/?ks=GDK-950
[GDK-960]: https://gadak.dev/backlog/#/?ks=GDK-960
[GDK-981]: https://gadak.dev/backlog/#/?ks=GDK-981
[GDK-944]: https://gadak.dev/backlog/#/?ks=GDK-944
[GDK-967]: https://gadak.dev/backlog/#/?ks=GDK-967
[GDK-968]: https://gadak.dev/backlog/#/?ks=GDK-968
[GDK-971]: https://gadak.dev/backlog/#/?ks=GDK-971
[GDK-975]: https://gadak.dev/backlog/#/?ks=GDK-975
[GDK-980]: https://gadak.dev/backlog/#/?ks=GDK-980
[GDK-963]: https://gadak.dev/backlog/#/?ks=GDK-963
[GDK-741]: https://gadak.dev/backlog/#/?ks=GDK-741
[GDK-946]: https://gadak.dev/backlog/#/?ks=GDK-946
[GDK-947]: https://gadak.dev/backlog/#/?ks=GDK-947
[GDK-859]: https://gadak.dev/backlog/#/?ks=GDK-859
[GDK-860]: https://gadak.dev/backlog/#/?ks=GDK-860
[GDK-880]: https://gadak.dev/backlog/#/?ks=GDK-880
[GDK-884]: https://gadak.dev/backlog/#/?ks=GDK-884
[GDK-885]: https://gadak.dev/backlog/#/?ks=GDK-885
[GDK-886]: https://gadak.dev/backlog/#/?ks=GDK-886
[GDK-887]: https://gadak.dev/backlog/#/?ks=GDK-887
[GDK-888]: https://gadak.dev/backlog/#/?ks=GDK-888
[GDK-905]: https://gadak.dev/backlog/#/?ks=GDK-905
[GDK-906]: https://gadak.dev/backlog/#/?ks=GDK-906
[GDK-907]: https://gadak.dev/backlog/#/?ks=GDK-907
[GDK-908]: https://gadak.dev/backlog/#/?ks=GDK-908
[GDK-910]: https://gadak.dev/backlog/#/?ks=GDK-910
[GDK-867]: https://gadak.dev/backlog/#/?ks=GDK-867
[GDK-870]: https://gadak.dev/backlog/#/?ks=GDK-870
[GDK-879]: https://gadak.dev/backlog/#/?ks=GDK-879
[GDK-1175]: https://gadak.dev/backlog/#/?ks=GDK-1175
[GDK-1176]: https://gadak.dev/backlog/#/?ks=GDK-1176
[GDK-1190]: https://gadak.dev/backlog/#/?ks=GDK-1190
[GDK-1248]: https://gadak.dev/backlog/#/?ks=GDK-1248
[GDK-1024]: https://gadak.dev/backlog/#/?ks=GDK-1024
[GDK-1158]: https://gadak.dev/backlog/#/?ks=GDK-1158
[GDK-1196]: https://gadak.dev/backlog/#/?ks=GDK-1196
[GDK-1197]: https://gadak.dev/backlog/#/?ks=GDK-1197
[GDK-1194]: https://gadak.dev/backlog/#/?ks=GDK-1194
[GDK-1199]: https://gadak.dev/backlog/#/?ks=GDK-1199
[GDK-1200]: https://gadak.dev/backlog/#/?ks=GDK-1200
[GDK-1250]: https://gadak.dev/backlog/#/?ks=GDK-1250
[GDK-1251]: https://gadak.dev/backlog/#/?ks=GDK-1251
[GDK-1097]: https://gadak.dev/backlog/#/?ks=GDK-1097
[GDK-1096]: https://gadak.dev/backlog/#/?ks=GDK-1096
[GDK-1098]: https://gadak.dev/backlog/#/?ks=GDK-1098
[GDK-1051]: https://gadak.dev/backlog/#/?ks=GDK-1051
[GDK-871]: https://gadak.dev/backlog/#/?ks=GDK-871
[GDK-899]: https://gadak.dev/backlog/#/?ks=GDK-899
[GDK-992]: https://gadak.dev/backlog/#/?ks=GDK-992
[GDK-1030]: https://gadak.dev/backlog/#/?ks=GDK-1030
[GDK-1205]: https://gadak.dev/backlog/#/?ks=GDK-1205
[GDK-1001]: https://gadak.dev/backlog/#/?ks=GDK-1001
[GDK-1234]: https://gadak.dev/backlog/#/?ks=GDK-1234
[GDK-1233]: https://gadak.dev/backlog/#/?ks=GDK-1233
[GDK-1244]: https://gadak.dev/backlog/#/?ks=GDK-1244
[GDK-1243]: https://gadak.dev/backlog/#/?ks=GDK-1243
[GDK-1235]: https://gadak.dev/backlog/#/?ks=GDK-1235
[GDK-1075]: https://gadak.dev/backlog/#/?ks=GDK-1075
[GDK-1074]: https://gadak.dev/backlog/#/?ks=GDK-1074
[GDK-1047]: https://gadak.dev/backlog/#/?ks=GDK-1047
[GDK-1246]: https://gadak.dev/backlog/#/?ks=GDK-1246
[GDK-1122]: https://gadak.dev/backlog/#/?ks=GDK-1122
[GDK-996]: https://gadak.dev/backlog/#/?ks=GDK-996
[GDK-1128]: https://gadak.dev/backlog/#/?ks=GDK-1128
