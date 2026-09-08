# Jira Server / Data Center 검증 환경

gadak 의 `jira-server` origin 을 실측으로 검증하는 로컬 인스턴스를 세우는
절차다. 두 종류가 필요하다 — **Core** 는 위키 마크업·첨부·사용자 축을,
**Software** 는 보드·스프린트·에픽을 재현한다. 하나로는 못 덮는다:
`atlas-run-standalone --product jira` 가 주는 것은 Core 뿐이고, Software 는
별도 앱이라 `--bundled-plugins` 로 붙지 않는다 (9 kB 서술자 jar 만 받아지고
`jira-software` 롤은 `defined: false` 로 남는다. 진짜 아티팩트는 61 MB `.obr`
인데 amps 가 classifier 를 받지 않는다: `Invalid artifact pattern: …:obr`).

## 라이선스

**둘 다 Atlassian Plugin SDK 에서 나온다.** SDK 는 인증 없이 받을 수 있고,
`atlas-run-standalone --product jira` 로 뜬 인스턴스는 만료일이 있는
Developer / dataCenter / 무제한 시트 라이선스를 갖고 있다. 그 원문을 꺼내:

```bash
curl -s -u admin:admin \
  'http://localhost:2990/jira/rest/plugins/applications/1.0/installed/jira-software/license' \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['rawLicense'])"
```

**이 값은 레포에 들어가지 않는다.** 필요할 때 위 명령으로 다시 꺼낸다.

## Core — SDK 로

```bash
# Jira 11 은 Java 21 이다. Java 17 로 띄우면 임베디드 Tomcat 이 컨텍스트
# 실패를 삼키고 아무 스택도 남기지 않은 채 404 만 응답한다.
export JAVA_HOME=$(/usr/libexec/java_home -v 21)

# nohup 만 쓰면 stdin EOF 를 amps 가 종료 신호로 읽는다. tail 로 붙잡는다.
( tail -f /dev/null | nohup atlas-run-standalone --product jira > /tmp/jira-core.log 2>&1 & )
```

`http://localhost:2990/jira`, admin/admin.

## Software — Docker 로

```bash
docker network create jiranet
docker run -d --name jiradb --network jiranet \
  -e POSTGRES_PASSWORD=jira -e POSTGRES_USER=jira -e POSTGRES_DB=jira postgres:16
docker run -d --name jirasw --network jiranet -p 8088:8080 \
  -e ATL_JDBC_URL='jdbc:postgresql://jiradb:5432/jira' \
  -e ATL_JDBC_USER=jira -e ATL_JDBC_PASSWORD=jira \
  -e ATL_DB_DRIVER=org.postgresql.Driver -e ATL_DB_TYPE=postgres72 \
  atlassian/jira-software:11.3.11
```

Jira 11 은 내장 H2 를 지원하지 않으므로 외부 DB 가 필수다. `ATL_JDBC_*` 를
주면 설정 마법사의 DB 단계는 건너뛴다. 준비되면 `/status` 가 `FIRST_RUN`.

**주의: 랩 컨테이너가 떠 있는 동안 `docker container prune` 을 돌리지 마라.**
방금 만든 postgres 가 지워지고, Jira 는 그 뒤 "startup check failed" 로
잠긴다 — 로그에 남는 것은 `PSQLException: The connection attempt failed` 한
줄뿐이라 원인이 컨테이너 소멸이라는 것이 안 보인다.

### 마법사 통과 (네 번의 POST)

`atl_token` 은 `atlassian.xsrf.token` **쿠키 값**이다. 폼 HTML 에서 긁으려
하면 빈 문자열이 나오고 403 이 돌아온다.

```bash
J=http://localhost:8088
curl -s -c cj -b cj -L "$J/" -o /dev/null            # 쿠키 발급
T=$(grep atlassian.xsrf.token cj | awk '{print $7}')

curl -s -b cj -c cj -L "$J/secure/SetupApplicationProperties.jspa" -o /dev/null \
  --data-urlencode "atl_token=$T" --data-urlencode "title=gadak DC test" \
  --data-urlencode "mode=private" --data-urlencode "baseURL=$J" \
  --data-urlencode "nextStep=true" --data-urlencode "next=Next"

curl -s -b cj -c cj -L "$J/secure/SetupLicense.jspa" -o /dev/null \
  --data-urlencode "atl_token=$T" --data-urlencode "setupLicenseKey=$(cat jira-software.lic)" \
  --data-urlencode "next=Next"

curl -s -b cj -c cj -L "$J/secure/SetupAdminAccount.jspa" -o /dev/null \
  --data-urlencode "atl_token=$T" --data-urlencode "username=admin" \
  --data-urlencode "password=admin" --data-urlencode "confirm=admin" \
  --data-urlencode "fullname=Admin" --data-urlencode "email=admin@example.com" \
  --data-urlencode "next=Next"

curl -s -b cj -c cj -L "$J/secure/SetupMailNotifications.jspa" -o /dev/null \
  --data-urlencode "atl_token=$T" --data-urlencode "noemail=true" --data-urlencode "finish=Finish"
```

`/status` 가 `RUNNING` 이면 끝이다.

### PAT — basic auth 는 꺼져 있다

Jira 11 DC 는 basic 인증이 기본으로 꺼져 있다
(`{"message":"Basic Authentication has been disabled on this instance."}`).
gadak 의 Server 지원이 PAT 를 쓰는 것이 선택이 아니라 필수인 이유다.

```bash
curl -s -b cj -X POST "$J/rest/pat/latest/tokens" \
  -H 'Content-Type: application/json' -H 'X-Atlassian-Token: no-check' \
  -d '{"name":"gadak","expirationDuration":90}'
```

필드 이름은 `expirationDuration` 이다 (`expiring…` 은 400).

### 스크럼 프로젝트와 스프린트 시드

```bash
H="Authorization: Bearer $PAT"
# 템플릿 키는 gh-scrum-template 이다. Cloud 의 gh-simplified-agility-scrum 은 없다.
curl -s -H "$H" -H 'Content-Type: application/json' -X POST "$J/rest/api/2/project" \
  -d '{"key":"SCR","name":"Scrum Test","projectTypeKey":"software",
       "projectTemplateKey":"com.pyxis.greenhopper.jira:gh-scrum-template","lead":"admin"}'

curl -s -H "$H" -H 'Content-Type: application/json' -X POST "$J/rest/agile/1.0/sprint" \
  -d '{"name":"Sprint 1","originBoardId":1}'
curl -s -H "$H" -H 'Content-Type: application/json' -X POST "$J/rest/agile/1.0/sprint/1/issue" \
  -d '{"issues":["SCR-1","SCR-2","SCR-3"]}'
curl -s -H "$H" -H 'Content-Type: application/json' -X POST "$J/rest/agile/1.0/sprint/1" \
  -d '{"state":"active","startDate":"…","endDate":"…"}'
```

Epic 을 만들려면 Epic Name(`gh-epic-label`) 이 필수다 — 없으면 400 이다.

## gadak 붙이기

```bash
gadak --workspace scr init --server --site http://localhost:8088 --token-stdin <<< "$PAT"
gadak --workspace scr sync --full
```

**미러가 이미 있으면 `sync --full` 이 필드 추가를 반영하지 않는다** —
`0 changed` 로 끝난다. 동기화 코드의 필드 목록을 바꿨으면
`rm -f ~/.gadak/profiles/scr/gadak.db*` 뒤에 다시 돌린다.

## Jira 자신의 스키마를 참고 자료로

컨테이너의 postgres 에는 Jira 의 실제 테이블이 다 있다. 미러 설계의 참고가
된다 (특히 greenhopper):

```bash
docker exec jiradb psql -U jira -d jira -c '\dt "AO_60DB71"*'
docker exec jiradb psql -U jira -d jira -c '\d "AO_60DB71_SPRINT"'
```

`AO_60DB71_SPRINT` 는 상태를 `state` 문자열이 아니라 `CLOSED`/`STARTED`
불리언 둘로 들고, `RAPID_VIEW_ID` 로 보드에 붙는다. `AO_60DB71_RAPIDVIEW`
가 보드, `AO_60DB71_ISSUERANKING`/`LEXORANK` 가 백로그 순서다.
