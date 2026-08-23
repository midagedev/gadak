export type Locale = 'en' | 'ko'

export const strings = {
  en: {
    htmlLang: 'en',
    title: 'gadak — Follow the thread.',
    description:
      'Your team’s tracker, mirrored into one local SQLite file. Search it, query it in SQL, point your coding agent at it. Reads never touch the network.',
    nav: { demo: 'Live demo', backlog: 'Backlog', install: 'Install', github: 'GitHub' },
    heroTagline: 'Follow the thread.',
    heroLede:
      'The tracker your company already runs, mirrored into one local SQLite file on this machine. Search lands in milliseconds, history reads like a document, and your coding agent works the same list you do.',
    doors: {
      demoTitle: '▶ Open the live demo',
      demoSub: 'The real UI over a scrubbed sample mirror. No install, no account.',
      backlogTitle: 'Public backlog',
      backlogSub: 'gadak’s own issues, published from gadak — every commit cites one.',
      sourceTitle: 'Source and releases',
      sourceSub: 'Apache-2.0 on GitHub. macOS app, and a CLI for Linux and Windows.',
    },
    audiences: {
      label: 'Built for two kinds of people',
      teamTitle: 'Teams that live in a slow tracker',
      teamLede:
        'Keep writing where your company writes. Read from a copy that opens instantly and never waits on a page load.',
      teamPoints: [
        'Search across every issue in about 100 ms, at 20k issues',
        'Group by assignee, priority, epic — saved views stay one URL away',
        'Time on each issue, computed as you read: wait 3d · progress 5h',
      ],
      agentTitle: 'Builders who code with agents',
      agentLede:
        'Give your agent a task queue it can actually write to — create, claim, transition — while you watch the same board update.',
      agentPoints: [
        'CLI verbs an agent can trust: create, edit, transition, assign',
        'MCP server for clients without a shell',
        'Start with no tracker at all — one command, local first',
      ],
    },
    speed: {
      heading: 'Fast is a measurement, not an adjective',
      note: 'Measured on a 20k-issue mirror, M4 Pro. Full table in docs/BENCHMARKS.md.',
      rows: [
        { what: 'Key lookup → issue open', value: '110 ms' },
        { what: 'Full sync of a 534-issue site', value: '~5 s' },
        { what: 'Incremental sync re-run (nothing changed)', value: '0 writes' },
      ],
    },
    agentSection: {
      heading: 'One vocabulary between you and the agent',
      body:
        'The CLI doubles as the agent interface, and an MCP server covers clients without a shell. Writes go through to the origin; reads come off the local mirror.',
      snippetCaption: 'Point an MCP client at gadak:',
    },
    workspace: {
      heading: 'Work and your own work, side by side',
      body:
        'A second workspace keeps private tasks off the company tracker — same machine, separate file, separate credentials. Standalone needs no tracker at all.',
    },
    install: {
      heading: 'Install',
    },
    mediaPlaceholder: { slot: 'Media slot' },
    footer: {
      builtBy: 'Built by',
      whereBytes: 'Where the bytes go',
    },
  },
  ko: {
    htmlLang: 'ko',
    title: 'gadak — 실을 따라가다',
    description:
      '회사가 쓰는 그 트래커를 로컬 SQLite 파일 하나로 미러링합니다. 검색하고, SQL로 묻고, 코딩 에이전트를 붙이세요. 읽기는 네트워크를 건드리지 않습니다.',
    nav: { demo: '라이브 데모', backlog: '공개 백로그', install: '설치', github: 'GitHub' },
    heroTagline: '실을 따라가다.',
    heroLede:
      '회사가 이미 쓰는 트래커를 이 머신의 SQLite 파일 하나로 미러링합니다. 검색은 밀리초 안에 끝나고, 히스토리는 문서처럼 읽히고, 코딩 에이전트도 같은 목록에서 일합니다.',
    doors: {
      demoTitle: '▶ 라이브 데모 열기',
      demoSub: '새로 씻어낸 샘플 미러 위의 실제 UI. 설치도 계정도 없이.',
      backlogTitle: '공개 백로그',
      backlogSub: 'gadak의 이슈를 gadak으로 공개합니다 — 모든 커밋이 하나를 인용합니다.',
      sourceTitle: '소스와 릴리즈',
      sourceSub: 'GitHub에서 Apache-2.0. macOS 앱, Linux·Windows용 CLI.',
    },
    audiences: {
      label: '두 종류의 사람을 위해',
      teamTitle: '느린 트래커 속에서 일하는 팀',
      teamLede:
        '쓰는 곳은 회사가 정한 곳 그대로. 읽는 것은 즉시 열리는 사본에서 — 페이지 로딩을 기다리는 일이 없습니다.',
      teamPoints: [
        '2만 건 기준 검색 약 100ms',
        '담당자·우선순위·에픽으로 묶기 — 저장된 뷰는 URL 하나',
        '이슈별 시간 회계를 읽을 때 계산: 대기 3일 · 진행 5시간',
      ],
      agentTitle: '에이전트와 함께 만드는 사람',
      agentLede:
        '에이전트가 실제로 쓸 수 있는 태스크 큐를 주세요 — 생성, 클레임, 상태 전환까지. 보드는 당신 눈앞에서 같이 움직입니다.',
      agentPoints: [
        '에이전트가 믿을 수 있는 CLI 동사: create, edit, transition, assign',
        '셸 없는 클라이언트를 위한 MCP 서버',
        '트래커 없이 시작 — 명령 한 줄, 로컬 우선',
      ],
    },
    speed: {
      heading: '빠르다는 말 대신 측정값',
      note: '2만 건 미러, M4 Pro 실측. 전체 표는 docs/BENCHMARKS.md.',
      rows: [
        { what: '키 조회 → 이슈 열기', value: '110ms' },
        { what: '534건 사이트 풀 싱크', value: '약 5초' },
        { what: '변화 없는 증분 재실행', value: '쓰기 0건' },
      ],
    },
    agentSection: {
      heading: '나와 에이전트가 나누는 하나의 어휘',
      body:
        'CLI가 곧 에이전트 인터페이스고, MCP 서버가 셸 없는 클라이언트를 맡습니다. 쓰기는 origin까지 통과하고, 읽기는 로컬 미러에서 나옵니다.',
      snippetCaption: 'MCP 클라이언트를 gadak에 연결:',
    },
    workspace: {
      heading: '회사 일과 내 일, 나란히',
      body:
        '두 번째 워크스페이스가 개인 작업을 회사 트래커 밖에 둡니다 — 같은 머신, 다른 파일, 다른 자격증명. 스탠드얼론은 트래커 없이도 시작합니다.',
    },
    install: {
      heading: '설치',
    },
    mediaPlaceholder: { slot: '영상 자리' },
    footer: {
      builtBy: '만든 사람',
      whereBytes: '바이트가 가는 곳',
    },
  },
} satisfies Record<Locale, Record<string, unknown>>

export type Strings = (typeof strings)['en']
