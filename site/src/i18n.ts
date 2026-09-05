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
      videoCaption:
        'The shell is in the window: a claim names the tab after the issue, then a pipe and a JQL each become the list above it.',
      mcpCaption:
        'A live Claude Code session on the mirror: issues and wiki pages in one index — the join Jira and Confluence never make for you.',
      driveCaption:
        'A live Claude Code session in that same pane: a Korean sentence becomes the list, the next one saves and opens a dashboard — the agent and the board it moves, in one window.',
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
        'A second workspace keeps private tasks off the company tracker — same machine, separate file, separate credentials. The built-in tracker needs no account at all.',
      localOriginLead: 'The built-in tracker — no account, issues live on this machine:',
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
    title: 'gadak — 같은 Jira, 기다림 없이.',
    description:
      '회사에서 쓰는 Jira의 이슈와 Confluence 위키를 로컬 SQLite 파일 하나에 미러링합니다. 이슈 2만 건에서도 검색은 밀리초 안에 끝나고, 읽기는 네트워크를 타지 않습니다.',
    nav: { demo: '라이브 데모', changelog: '체인지로그', essays: '에세이', install: '설치', github: 'GitHub' },
    copy: { label: '복사', copied: '복사됨' },
    ogImageAlt: 'gadak — 같은 Jira, 기다림 없이. 팀의 Jira와 Confluence 위키를 로컬 SQLite 파일 하나에 미러링합니다.',
    langName: '한국어',
    langBanner: {
      offer: '이 페이지는 한국어로도 볼 수 있습니다.',
      cta: '한국어로 보기 →',
      dismiss: '닫기',
    },
    hero: {
      eyebrow: 'gadak',
      heading: '같은 Jira, 기다림 없이.',
      lede:
        '회사에서 이미 쓰는 Jira를 Confluence 위키까지 이 컴퓨터의 SQLite 파일 하나에 미러링합니다. 검색은 밀리초 안에 끝나고, 히스토리는 문서처럼 읽히고, 로딩 스피너는 보이지 않습니다. 원본은 여전히 Jira입니다. 기다리는 시간만 사라집니다.',
      videoCaption: '이슈 2만 건 미러에서 타이핑하는 속도로 검색합니다. 애니메이션이 아니라 실제 화면을 녹화한 것입니다.',
      doors: {
        installTitle: '설치',
        installSub: 'Homebrew 한 줄. macOS는 앱, Linux·Windows는 CLI.',
      },
    },
    speed: {
      label: '빠르다는 말 대신 측정값으로',
      heading: '같은 질문, 두 가지 방법으로',
      note: '2026-08-26에 실제 Atlassian Cloud 사이트(실제 업무 프로젝트, 이슈 3,296건)에서 측정했습니다. 합성 데이터가 아닙니다. gadak 쪽 수치에는 CLI 프로세스 시작 시간까지 들어 있습니다. 측정 방법과 재측정 이력, gadak이 더 느린 경우까지 정리한 표:',
      rows: [
        { what: '단순 필터, 100건', value: '583 ms', alt: '19 ms', ratio: '31×' },
        { what: '이슈 1건 + 체인지로그 전체', value: '710 ms', alt: '28 ms', ratio: '25×' },
        { what: '전문 검색', value: '543 ms', alt: '41 ms', ratio: '13×' },
        { what: '에픽별 열린 이슈 (GROUP BY)', value: '4,761 ms, API 호출 8페이지', alt: '22 ms, 쿼리 한 번', ratio: '214×' },
        { what: '변경 이력 집계', value: 'JQL로는 표현 불가', alt: '14 ms', ratio: '—' },
        { what: '요청 제한', value: '429 + Retry-After', alt: '없음, 내 디스크니까', ratio: '—' },
      ],
      localHeading: '로컬 미러에서는',
      localNote: '데모와 같은 SQLite 파일, 이슈 2만 건 기준입니다. 팔레트 검색은 HTTP 왕복을 포함해 0.5~2 ms, 이슈 534건 사이트의 전체 동기화는 약 5초, 변경이 없을 때 증분 동기화의 쓰기는 0건입니다.',
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    demo: {
      heading: '아니면 라이브 데모를 먼저 열어 보세요',
      body: '민감한 내용을 지운 이슈 534건짜리 샘플 미러 위에서 실제 UI가 그대로 돌아갑니다. 설치도 계정도 필요 없습니다. 이 페이지의 영상도 모두 이 앱에서 녹화했습니다.',
      cta: '▶ 라이브 데모 열기',
    },
    ux: {
      label: '매일 반복하는 일, 다시 설계',
      search: {
        heading: '타이핑을 따라오는 검색',
        body:
          '팔레트 하나로 제목, 본문, 코멘트, 위키까지 전부 찾습니다. 단어를 다 치기 전에 접두어가 일치하는 결과가 로컬에서 먼저 뜨고, 전문 검색 결과가 바로 뒤따릅니다. 스피너도, 서버 왕복도 없습니다.',
      },
      group: {
        heading: '어떤 축으로든 메뉴 하나로, 개수까지 함께',
        body:
          '담당자, 우선순위, 에픽으로 다시 묶으면 분포 막대가 구간별 개수를 바로 보여 줍니다. 필터 메뉴의 옵션마다 현재 뷰 기준 개수가 붙어 있어서, 고르기 전에 결과가 몇 건일지 알 수 있습니다. 저장한 뷰는 URL 하나로 다시 열립니다.',
        alt:
          '에픽으로 묶은 백로그 위에 담당자 필터 메뉴가 열려 있고, 옵션마다 현재 개수가 붙어 있다. 위쪽 분포 막대는 에픽별 개수를 보여 준다.',
      },
      history: {
        heading: '문서처럼 읽히는 히스토리',
        body:
          '상태 변경, 코멘트, 연결된 PR을 한 번의 스크롤로 읽습니다. 각 상태에 머문 시간은 읽는 시점에 계산됩니다(대기 6분, 진행 34일). 변경 이력이 곧 인터페이스입니다.',
        alt:
          '이슈 하나를 위에서 아래로 읽은 화면. Reopened ×2 배지, 대기·진행 시간 칩, Bot 라벨이 붙은 에이전트 코멘트, 빨간 Reopened 표시가 있는 상태 변경 이력이 보인다.',
      },
    },
    agent: {
      label: '에이전트와 함께 일하는 사람에게',
      heading: '사람과 에이전트가 같은 말을 씁니다',
      body:
        'CLI가 그대로 에이전트 인터페이스입니다. create, claim, transition 같은 동사를 에이전트가 실행하면 같은 보드가 눈앞에서 바뀝니다. 셸이 없는 클라이언트는 MCP 서버가 맡습니다. 쓰기는 원본 Jira를 거치고, 읽기는 로컬 미러에서 처리합니다. 에이전트가 쓴 것에는 이름이 남습니다. 코멘트와 연결된 PR에 봇 이름이 붙어서, 팀이 읽는 그 스레드에 그대로 보입니다.',
      skillLead: '같은 미러를 코딩 에이전트에게 넘기려면:',
      mcpLead: '셸이 없는 MCP 클라이언트(Claude Desktop)에는:',
      setupLink: '도구별로 붙여 넣을 설정 블록 → docs/AGENT_SETUP.md',
      videoCaption:
        '앱 창 안에서 셸이 열립니다. claim 한 번에 탭 이름이 이슈 키로 바뀌고, 파이프 한 줄과 JQL 한 줄이 차례로 위쪽 리스트가 됩니다.',
      mcpCaption:
        '미러 위에서 돌아가는 실제 Claude Code 세션입니다. 이슈와 위키 페이지가 한 인덱스에 있어서, Jira와 Confluence가 해 주지 않는 조인이 됩니다.',
      driveCaption:
        '같은 창 안의 실제 Claude Code 세션입니다. 한국어 한 문장이 리스트가 되고, 다음 문장이 대시보드를 저장해 엽니다. 에이전트와 에이전트가 움직이는 보드가 한 창에 있습니다.',
      dashboardsCaption:
        '에이전트가 대시보드를 만들면 그 대시보드가 앱 안으로 돌아옵니다. HTML 문서 하나에 이름 붙인 쿼리를 담고, 에이전트가 거기 적은 이슈 키는 진짜 링크라서 누르면 페이지를 떠나지 않고 그 이슈가 열립니다.',
      tokensCaption:
        '테마를 바꾸되 요청한 값은 지킵니다. 토큰을 쓰면 적용한 뒤 어떻게 읽힐지 알려 줍니다. 여기서는 경고가 글자 크기 단계 전체를 출력하고, 세션이 어긋난 단계를 스스로 고칩니다.',
    },
    origin: {
      label: '안심하고 쓸 수 있는 이유',
      heading: '원본은 여전히 Jira입니다',
      points: [
        '쓰기는 먼저 Jira로 가고, Jira가 받아들인 뒤에야 미러가 갱신됩니다.',
        '미러는 버려도 되는 캐시입니다. 지우고 다시 동기화하면 원본에서 그대로 다시 만들어집니다.',
        '텔레메트리는 없습니다. 밖으로 나가는 요청은 직접 설정한 것뿐입니다.',
        '자격 증명은 SQLite 파일에도, 로그에도, 스냅샷에도 남지 않습니다.',
      ],
    },
    workspace: {
      heading: '회사 일과 내 일, 나란히',
      body:
        '워크스페이스를 하나 더 만들면 개인 작업이 회사 트래커 밖에 남습니다. 같은 컴퓨터, 다른 파일, 다른 자격 증명입니다. 내장 트래커는 계정 없이 바로 시작합니다.',
      localOriginLead: '내장 트래커는 계정 없이 시작하고, 이슈는 이 컴퓨터에만 둡니다:',
      secondLead: '두 번째 워크스페이스는 같은 컴퓨터에 다른 파일과 다른 자격 증명으로 만듭니다:',
    },
    changelog: {
      heading: '체인지로그',
      lede:
        '릴리스마다 직접 내보낸 사람이 자기 말로 씁니다. 이슈 키는 공개 백로그로 이어져서, ' +
        '여기 한 줄에서 그 일을 요청한 이슈까지 거슬러 읽을 수 있습니다.',
      source: '저장소의 CHANGELOG.ko.md를 그대로 렌더링합니다. 영문판이 원본입니다.',
      jumpLabel: '버전으로 이동',
    },
    install: {
      heading: '설치',
      macosApp: '데스크톱 앱, CLI 포함:',
      cliOnly: 'CLI만:',
      firstRun: '회사 Jira에 연결합니다 (사이트, 이메일, 토큰, 프로젝트를 차례로 묻습니다):',
    },
    footer: {
      builtBy: '만든 사람',
      whereBytes: '데이터가 어디로 가는지',
    },
  },
} satisfies Record<Locale, Record<string, unknown>>

export type Strings = (typeof strings)['en']
