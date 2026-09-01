export type Locale = 'en' | 'ko'

export const strings = {
  en: {
    htmlLang: 'en',
    title: 'gadak — Same Jira. No waiting.',
    description:
      'The Jira your company already runs — issues and the Confluence wiki — mirrored into one local SQLite file. Search lands in milliseconds on 20,000 issues. Reads never touch the network.',
    nav: { demo: 'Live demo', changelog: 'Changelog', essays: 'Essays', install: 'Install', github: 'GitHub' },
    copy: { label: 'Copy', copied: 'Copied' },
    ogImageAlt:
      'gadak — Same Jira. No waiting. Your team’s Jira and its Confluence wiki, mirrored into one local SQLite file.',
    langName: 'English',
    langBanner: {
      offer: 'This page is also available in English.',
      cta: 'View in English →',
      dismiss: 'Dismiss',
    },
    hero: {
      eyebrow: 'gadak',
      heading: 'Same Jira. No waiting.',
      lede:
        'Your team\u2019s Jira — and its Confluence wiki — mirrored into one local SQLite file on this machine. Search lands in milliseconds, history reads like a document, and the page never spins. Jira stays the source of truth — you just stop waiting on it.',
      videoCaption: 'A 20,000-issue mirror. Search as fast as you can type. Recorded, not animated.',
      doors: {
        installTitle: 'Install',
        installSub: 'One Homebrew line. macOS app, CLI for Linux and Windows.',
      },
    },
    speed: {
      label: 'Fast is a measurement, not an adjective',
      heading: 'The same question, asked two ways',
      note: 'Measured 2026-08-26 against a live Atlassian Cloud site (a real work project, 3,296 issues), not a synthetic fixture. gadak numbers include full CLI process startup. Method, re-measurement history, and the honest where-gadak-loses table:',
      rows: [
        { what: 'Simple filter, 100 issues', value: '583 ms', alt: '19 ms', ratio: '31×' },
        { what: 'One issue + full changelog', value: '710 ms', alt: '28 ms', ratio: '25×' },
        { what: 'Free-text search', value: '543 ms', alt: '41 ms', ratio: '13×' },
        { what: 'Open issues per epic (GROUP BY)', value: '4,761 ms — 8 API pages', alt: '22 ms — one query', ratio: '214×' },
        { what: 'A count over the change history', value: 'not expressible', alt: '14 ms', ratio: '—' },
        { what: 'Rate limit', value: '429 + Retry-After', alt: 'none — your own disk', ratio: '—' },
      ],
      localHeading: 'On the local mirror',
      localNote: 'The same SQLite file the demo uses, 20,000 issues: palette search (HTTP round trip included) 0.5–2 ms, full sync of a 534-issue site ~5 s, incremental re-run with nothing changed 0 writes.',
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    demo: {
      heading: 'Or just open the live demo',
      body: 'The real UI over a scrubbed 534-issue sample mirror — no install, no account. The videos on this page were recorded from this same app.',
      cta: '▶ Open the live demo',
    },
    ux: {
      label: 'The daily loop, reconsidered',
      search: {
        heading: 'Search that keeps up with typing',
        body:
          'One palette over everything — titles, bodies, comments, even the wiki. Prefix matches land locally before you finish the word; full-text lands right behind them. No spinner, no round trip.',
      },
      group: {
        heading: 'Any axis, one menu — with the counts on it',
        body:
          'Regroup by assignee, priority, or epic and the breakdown bar carries a live count per segment. Filter menus count every option against the current view, so the next pick is informed before you make it — the menu already knows. Saved views stay one URL away.',
        alt:
          'The assignee filter menu, open over an epic-grouped backlog: every option carries its live count, and the breakdown bar above counts each epic.',
      },
      history: {
        heading: 'History reads like a document',
        body:
          'Every status change, comment, and linked PR in one scroll — with time-in-status computed as you read (waited 6m, in progress 34d). The changelog is the interface.',
        alt:
          'One issue read top to bottom: a Reopened ×2 badge, a waited/in-progress duration chip, a bot comment labelled Bot, and the status-change history with a red Reopened marker.',
      },
    },
    agent: {
      label: 'For the people building with agents',
      heading: 'One vocabulary between you and the agent',
      body:
        'The CLI doubles as the agent interface: create, claim, transition — verbs an agent can run while you watch the same board. An MCP server covers clients without a shell. Writes go through to the origin; reads come off the local mirror. And every agent write is attributed: its comments and linked PRs carry the bot’s name in the same thread your team reads.',
      skillLead: 'Hand the same mirror to your coding agent:',
      mcpLead: 'For MCP clients without a shell (Claude Desktop):',
      setupLink: 'Pasteable setup blocks for every tool → docs/AGENT_SETUP.md',
      heroCaption:
        'One serve, one terminal session, twenty-six seconds. A ticket is handed to an agent and the pane is closed; the work keeps running with nobody watching. A phone closes another issue from the same board. The desk comes back to the scrollback it left and a count that already moved — including what the phone did.',
      videoCaption: 'A question JQL cannot ask, answered from the local mirror.',
      mcpCaption:
        'A live Claude Code session on the mirror: issues and wiki pages in one index — the join Jira and Confluence never make for you.',
      driveCaption:
        'The skill in motion: asked for new colors and a chart dashboard, the agent runs gadak and both land in the open tab — no reload.',
      dashboardsCaption:
        'It builds the wall, and the wall links back. One HTML document plus named queries — and the keys the agent puts on it are real links, so a click lands on the issue instead of leaving the page.',
      tokensCaption:
        'It changes the look, and keeps what you asked for. Token writes apply and then say how they will read; here the warning prints the whole type ladder, and the session repairs the step on its own.',
    },
    origin: {
      label: 'Why this is safe to try',
      heading: 'Jira stays the source of truth',
      points: [
        'Writes pass through to Jira first; the mirror refreshes after the origin accepts.',
        'The mirror is disposable — delete it and re-sync to rebuild it from the origin.',
        'No telemetry. The only network calls are the ones you configured.',
        'Credentials never reach SQLite, a log, or a snapshot.',
      ],
    },
    workspace: {
      heading: 'Work and your own work, side by side',
      body:
        'A second workspace keeps private tasks off the company tracker — same machine, separate file, separate credentials. Local-origin needs no tracker at all.',
      standaloneLead: 'No tracker at all — issues live on this machine:',
      secondLead: 'A second workspace: separate file, separate credentials:',
    },
    changelog: {
      heading: 'Changelog',
      lede:
        'Every release, in the words of the person who shipped it. Issue keys link into the ' +
        'public backlog, so a line here can be read all the way back to what asked for it.',
      source: 'Rendered from CHANGELOG.md in the repository.',
      jumpLabel: 'Jump to a version',
    },
    install: {
      heading: 'Install',
      macosApp: 'The desktop app, CLI included:',
      cliOnly: 'CLI only:',
      firstRun: 'Connect to your team\'s Jira (asks for site, email, token, projects):',
    },
    footer: {
      builtBy: 'Built by',
      whereBytes: 'Where the bytes go',
    },
  },
  ko: {
    htmlLang: 'ko',
    title: 'gadak — 같은 지라, 기다림 없이.',
    description:
      '회사가 쓰는 그 Jira를 — 이슈도 Confluence 위키도 — 로컬 SQLite 파일 하나로 미러링합니다. 2만 건에서도 검색은 밀리초 단위. 읽기는 네트워크를 건드리지 않습니다.',
    nav: { demo: '라이브 데모', changelog: '체인지로그', essays: '에세이', install: '설치', github: 'GitHub' },
    copy: { label: '복사', copied: '복사됨' },
    ogImageAlt: 'gadak — 같은 Jira, 기다림 없이. 팀의 Jira와 Confluence 위키를 로컬 SQLite 파일 하나로 미러링.',
    langName: '한국어',
    langBanner: {
      offer: '이 페이지는 한국어로도 제공됩니다.',
      cta: '한국어로 보기 →',
      dismiss: '닫기',
    },
    hero: {
      eyebrow: 'gadak',
      heading: '같은 지라, 기다림 없이.',
      lede:
        '회사가 이미 쓰는 Jira를 — Confluence 위키까지 — 이 머신의 SQLite 파일 하나로 미러링합니다. 검색은 밀리초 안에 끝나고, 히스토리는 문서처럼 읽히고, 스피너는 사라집니다. 원본은 여전히 Jira — 기다리는 것만 없어집니다.',
      videoCaption: '2만 건 미러에서 타이핑 속도로 검색. 녹화본이지 애니메이션이 아닙니다.',
      doors: {
        installTitle: '설치',
        installSub: 'Homebrew 한 줄. macOS 앱, Linux·Windows CLI.',
      },
    },
    speed: {
      label: '빠르다는 말 대신 측정값',
      heading: '같은 질문, 두 가지 방식',
      note: '2026-08-26 실제 Atlassian Cloud 사이트(실제 업무 프로젝트, 3,296건) 대상 실측 — 합성 fixture가 아닙니다. gadak 수치에는 CLI 프로세스 시작까지 포함. 측정 방법과 gadak이 지는 지점 표 전체는',
      rows: [
        { what: '단순 필터, 100건', value: '583 ms', alt: '19 ms', ratio: '31×' },
        { what: '이슈 1건 + 체인지로그 전체', value: '710 ms', alt: '28 ms', ratio: '25×' },
        { what: '전문 검색', value: '543 ms', alt: '41 ms', ratio: '13×' },
        { what: '에픽별 열린 이슈 (GROUP BY)', value: '4,761 ms — API 8페이지', alt: '22 ms — 쿼리 하나', ratio: '214×' },
        { what: '변경 이력에 대한 집계', value: '표현 불가', alt: '14 ms', ratio: '—' },
        { what: '레이트 리밋', value: '429 + Retry-After', alt: '없음 — 내 디스크니까', ratio: '—' },
      ],
      localHeading: '로컬 미러에서는',
      localNote: '데모가 쓰는 것과 같은 SQLite 파일, 2만 건: 팔레트 검색(HTTP 왕복 포함) 0.5~2ms, 534건 사이트 풀 싱크 약 5초, 변화 없는 증분 재실행 쓰기 0건.',
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    demo: {
      heading: '아니면 라이브 데모를 열어보세요',
      body: '새로 씻어낸 534건 샘플 미러 위의 실제 UI — 설치도 계정도 없습니다. 이 페이지의 영상도 같은 앱에서 녹화했습니다.',
      cta: '▶ 라이브 데모 열기',
    },
    ux: {
      label: '매일의 루프를 다시 설계',
      search: {
        heading: '타이핑을 따라오는 검색',
        body:
          '하나의 팔레트가 전부를 덮습니다 — 제목, 본문, 코멘트, 위키까지. 접두 매치는 단어를 끝내기 전에 로컬에서 뜨고, 전문 검색이 바로 뒤따릅니다. 스피너도 왕복도 없습니다.',
      },
      group: {
        heading: '어떤 축이든, 메뉴 하나 — 숫자까지 붙어서',
        body:
          '담당자·우선순위·에픽으로 다시 묶으면 분포 막대가 구간별 개수를 실시간으로 셉니다. 필터 메뉴의 모든 옵션도 현재 뷰 기준으로 카운트되어, 고르기 전에 이미 답을 압니다 — 메뉴가 먼저 알고 있습니다. 저장된 뷰는 URL 하나.',
        alt:
          '에픽으로 묶인 백로그 위에 열린 담당자 필터 메뉴: 모든 옵션에 실시간 카운트가 붙고, 위의 분포 막대는 에픽별 개수를 셉니다.',
      },
      history: {
        heading: '히스토리가 문서처럼 읽힌다',
        body:
          '상태 변화·코멘트·연결된 PR이 한 번의 스크롤에 — 읽는 시점에 체류 시간이 계산됩니다(대기 6분, 진행 34일). 체인지로그가 곧 인터페이스입니다.',
        alt:
          '이슈 하나를 위에서 아래로: Reopened ×2 배지, 대기·진행 체류 시간 칩, Bot 라벨이 붙은 에이전트 코멘트, 붉은 Reopened 마커가 있는 상태 변경 히스토리.',
      },
    },
    agent: {
      label: '에이전트와 함께 만드는 사람에게',
      heading: '나와 에이전트가 나누는 하나의 어휘',
      body:
        'CLI가 곧 에이전트 인터페이스입니다 — create, claim, transition, 보드는 눈앞에서 같이 움직입니다. MCP 서버가 셸 없는 클라이언트를 맡습니다. 쓰기는 origin까지 통과하고, 읽기는 로컬 미러에서 나옵니다. 그리고 에이전트의 모든 쓰기에는 이름이 남습니다 — 코멘트와 연결된 PR에 봇의 이름이, 팀이 읽는 그 스레드에 그대로 붙습니다.',
      skillLead: '같은 미러를 코딩 에이전트에게 넘기려면:',
      mcpLead: '셸이 없는 MCP 클라이언트(Claude Desktop)에는:',
      setupLink: '도구별 붙여넣기 블록 → docs/AGENT_SETUP.md',
      heroCaption:
        'serve 하나, 터미널 세션 하나, 26초. 티켓을 에이전트에게 넘기고 패널을 닫습니다 — 보는 사람이 없어도 일은 계속됩니다. 폰이 같은 보드에서 다른 이슈를 닫습니다. 자리로 돌아오면 두고 간 스크롤백이 그대로 있고, 카운트는 이미 움직여 있습니다 — 폰이 한 것까지 포함해서.',
      videoCaption: 'JQL이 표현 못 하는 질문 하나를, 로컬 미러에서 답하기.',
      mcpCaption:
        '미러 위의 실제 Claude Code 세션: 이슈와 위키 페이지가 하나의 인덱스 — Jira와 Confluence가 대신 만들어 주지 않는 조인.',
      driveCaption:
        '스킬이 움직이는 모습: 새 색과 차트 대시보드를 부탁하자 에이전트가 gadak을 실행하고, 둘 다 리로드 없이 열린 탭에 내려앉습니다.',
      dashboardsCaption:
        '벽을 세우고, 그 벽이 앱으로 되돌아옵니다. HTML 문서 하나에 이름 붙은 쿼리들 — 에이전트가 벽에 올린 키는 진짜 링크라, 누르면 페이지를 떠나는 대신 그 이슈가 열립니다.',
      tokensCaption:
        '룩을 바꾸되, 당신이 요청한 값을 지킵니다. 토큰 쓰기는 적용된 뒤 어떻게 보일지를 말해 줍니다 — 여기서는 경고가 타입 사다리 전체를 찍어 주고, 세션이 스스로 단을 복구합니다.',
    },
    origin: {
      label: '왜 안심하고 써도 되는가',
      heading: '원본은 여전히 Jira입니다',
      points: [
        '쓰기는 먼저 Jira를 통과하고, origin이 받아들인 뒤에 미러가 갱신됩니다.',
        '미러는 버려도 되는 캐시 — 지우고 다시 동기화하면 origin에서 재구성됩니다.',
        '텔레메트리 없음. 나가는 요청은 당신이 설정한 것뿐입니다.',
        '자격증명은 SQLite도, 로그도, 스냅샷도 만지지 않습니다.',
      ],
    },
    workspace: {
      heading: '회사 일과 내 일, 나란히',
      body:
        '두 번째 워크스페이스가 개인 작업을 회사 트래커 밖에 둡니다 — 같은 머신, 다른 파일, 다른 자격증명. 스탠드얼론은 트래커 없이도 시작합니다.',
      standaloneLead: '스탠드얼론은 트래커 없이도 시작합니다:',
      secondLead: '두 번째 워크스페이스: 같은 머신, 다른 파일, 다른 자격증명:',
    },
    changelog: {
      heading: '체인지로그',
      lede:
        '릴리스마다, 그것을 내보낸 사람의 말로. 이슈 키는 공개 백로그로 이어지므로 ' +
        '여기 한 줄에서 그것을 요청한 자리까지 되짚어 읽을 수 있습니다.',
      source: '저장소의 CHANGELOG.ko.md를 그대로 렌더링합니다. 영문이 원본입니다.',
      jumpLabel: '버전으로 이동',
    },
    install: {
      heading: '설치',
      macosApp: '데스크톱 앱, CLI 포함:',
      cliOnly: 'CLI만:',
      firstRun: '회사 Jira에 연결 (사이트·이메일·토큰·프로젝트를 차례로 묻습니다):',
    },
    footer: {
      builtBy: '만든 사람',
      whereBytes: '바이트가 가는 곳',
    },
  },
} satisfies Record<Locale, Record<string, unknown>>

export type Strings = (typeof strings)['en']
