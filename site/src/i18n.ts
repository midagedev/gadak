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
        installSub: 'Homebrew on macOS, the Microsoft Store on Windows, a CLI for Linux.',
        demoTitle: 'Live demo',
        demoSub: '534 issues in your browser. No install, no account.',
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
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    ux: {
      label: 'The daily loop',
      search: {
        heading: 'Search that keeps up with typing',
        body:
          'One palette over everything — titles, bodies, comments, even the wiki. Prefix matches land locally before you finish the word; full-text lands right behind them. No spinner, no round trip.',
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
      driveCaption:
        'A live Claude Code session in that same pane: a Korean sentence becomes the list, the next one saves and opens a dashboard — the agent and the board it moves, in one window.',
      showcaseLink: 'More recordings — dashboards, a team theme, a launcher, a live MCP session → docs/SHOWCASE.md',
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
      windowsBefore: 'On Windows, the desktop app is on the',
      windowsAfter: '.',
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
        installSub: 'macOS는 Homebrew, Windows는 Microsoft Store, Linux는 CLI.',
        demoTitle: '라이브 데모',
        demoSub: '이슈 534건을 브라우저에서 바로. 설치도 계정도 없습니다.',
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
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    ux: {
      label: '매일 반복하는 일',
      search: {
        heading: '타이핑을 따라오는 검색',
        body:
          '팔레트 하나로 제목, 본문, 코멘트, 위키까지 전부 찾습니다. 단어를 다 치기 전에 접두어가 일치하는 결과가 로컬에서 먼저 뜨고, 전문 검색 결과가 바로 뒤따릅니다. 스피너도, 서버 왕복도 없습니다.',
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
      driveCaption:
        '같은 창 안의 실제 Claude Code 세션입니다. 한국어 한 문장이 리스트가 되고, 다음 문장이 대시보드를 저장해 엽니다. 에이전트와 에이전트가 움직이는 보드가 한 창에 있습니다.',
      showcaseLink: '녹화본 더 보기: 대시보드, 팀 테마, 런처, 라이브 MCP 세션 → docs/SHOWCASE.md',
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
      windowsBefore: 'Windows 데스크톱 앱은',
      windowsAfter: '에 있습니다.',
      firstRun: '회사 Jira에 연결합니다 (사이트, 이메일, 토큰, 프로젝트를 차례로 묻습니다):',
    },
    footer: {
      builtBy: '만든 사람',
      whereBytes: '데이터가 어디로 가는지',
    },
  },
} satisfies Record<Locale, Record<string, unknown>>

export type Strings = (typeof strings)['en']
