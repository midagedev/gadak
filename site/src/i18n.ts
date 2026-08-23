export type Locale = 'en' | 'ko'

export const strings = {
  en: {
    htmlLang: 'en',
    title: 'gadak — Same Jira. No waiting.',
    description:
      'The Jira your company already runs — issues and the Confluence wiki — mirrored into one local SQLite file. Search lands in milliseconds on 20,000 issues. Reads never touch the network.',
    nav: { demo: 'Live demo', backlog: 'Backlog', install: 'Install', github: 'GitHub' },
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
      note: 'Measured 2026-08-23 against a live Atlassian Cloud site (a real work project, 7,166 issues), not a synthetic fixture. gadak numbers include full CLI process startup. Method, re-measurement history, and the honest where-gadak-loses table: docs/BENCHMARKS.md.',
      rows: [
        { what: 'Simple filter, 100 issues', value: '374 ms', alt: '17 ms', ratio: '23×' },
        { what: 'One issue + full changelog', value: '687 ms', alt: '29 ms', ratio: '24×' },
        { what: 'Free-text search', value: '504 ms', alt: '22 ms', ratio: '23×' },
        { what: 'A count over the change history', value: 'not expressible', alt: '14 ms', ratio: '—' },
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
      snippetCaption: 'Onboarding is one line — a skill for shell agents, MCP for the rest:',
      setupLink: 'Pasteable setup blocks for every tool → docs/AGENT_SETUP.md',
      videoCaption: 'A question JQL cannot ask, answered from the local mirror.',
      mcpCaption:
        'A live Claude Code session on the mirror: issues and wiki pages in one index — the join Jira and Confluence never make for you.',
      mcpAlt:
        'A terminal where Claude Code is asked to search Jira and the wiki for idempotency; it calls the gadak MCP tool once and answers citing an issue key and a Confluence page together.',
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
        'A second workspace keeps private tasks off the company tracker — same machine, separate file, separate credentials. Standalone needs no tracker at all.',
    },
    install: {
      heading: 'Install',
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
    nav: { demo: '라이브 데모', backlog: '공개 백로그', install: '설치', github: 'GitHub' },
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
      note: '2026-08-23 실제 Atlassian Cloud 사이트(실제 업무 프로젝트, 7,166건) 대상 실측 — 합성 fixture가 아닙니다. gadak 수치에는 CLI 프로세스 시작까지 포함. 측정 방법과 gadak이 지는 지점 표 전체는 docs/BENCHMARKS.md.',
      rows: [
        { what: '단순 필터, 100건', value: '374 ms', alt: '17 ms', ratio: '23×' },
        { what: '이슈 1건 + 체인지로그 전체', value: '687 ms', alt: '29 ms', ratio: '24×' },
        { what: '전문 검색', value: '504 ms', alt: '22 ms', ratio: '23×' },
        { what: '변경 이력에 대한 집계', value: '표현 불가', alt: '14 ms', ratio: '—' },
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
      snippetCaption: '온보딩은 한 줄 — 셸 에이전트는 스킬, 나머지는 MCP:',
      setupLink: '도구별 붙여넣기 블록 → docs/AGENT_SETUP.md',
      videoCaption: 'JQL이 표현 못 하는 질문 하나를, 로컬 미러에서 답하기.',
      mcpCaption:
        '미러 위의 실제 Claude Code 세션: 이슈와 위키 페이지가 하나의 인덱스 — Jira와 Confluence가 대신 만들어 주지 않는 조인.',
      mcpAlt:
        'Claude Code에 Jira와 위키에서 idempotency를 찾으라고 묻는 터미널; gadak MCP 툴을 한 번 호출해 이슈 키와 Confluence 페이지를 한 문장으로 답합니다.',
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
    },
    install: {
      heading: '설치',
    },
    footer: {
      builtBy: '만든 사람',
      whereBytes: '바이트가 가는 곳',
    },
  },
} satisfies Record<Locale, Record<string, unknown>>

export type Strings = (typeof strings)['en']
