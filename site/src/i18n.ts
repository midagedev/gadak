export type Locale = 'en' | 'ko'

export const strings = {
  en: {
    htmlLang: 'en',
    title: 'gadak — Same Jira. No waiting.',
    description:
      'The tracker your company already runs, mirrored into one local SQLite file. Search lands in milliseconds on 20,000 issues. Reads never touch the network.',
    nav: { demo: 'Live demo', backlog: 'Backlog', install: 'Install', github: 'GitHub' },
    hero: {
      eyebrow: 'gadak',
      heading: 'Same Jira. No waiting.',
      lede:
        'Your team\u2019s tracker, mirrored into one local SQLite file on this machine. Search lands in milliseconds, history reads like a document, and the page never spins. Jira stays the source of truth — you just stop waiting on it.',
      videoCaption: 'A 20,000-issue mirror. Search as fast as you can type. Recorded, not animated.',
      doors: {
        demoTitle: '▶ Open the live demo',
        demoSub: 'The real UI over a scrubbed sample mirror. No install, no account.',
        installTitle: 'Install',
        installSub: 'One Homebrew line. macOS app, CLI for Linux and Windows.',
      },
    },
    speed: {
      label: 'Fast is a measurement, not an adjective',
      heading: 'Numbers you can check yourself',
      note: 'Measured on a 20,000-issue mirror, M4 Pro. Full table in docs/BENCHMARKS.md — every query on this page runs against the same SQLite file the demo uses.',
      rows: [
        { what: 'Full-text search, 20k issues', value: '~2 ms' },
        { what: 'Key lookup → issue open', value: '110 ms' },
        { what: 'Full sync of a 534-issue site', value: '~5 s' },
        { what: 'Incremental re-run, nothing changed', value: '0 writes' },
      ],
    },
    ux: {
      label: 'The daily loop, reconsidered',
      search: {
        heading: 'Search that keeps up with typing',
        body:
          'One palette over everything — titles, bodies, comments, even the wiki. Prefix matches land locally before you finish the word; full-text lands right behind them. No spinner, no round trip.',
      },
      group: {
        heading: 'Any axis, one menu',
        body:
          'Regroup by assignee, priority, or epic and the breakdown bar follows. Saved views are one URL — share the exact slice, not a screenshot.',
      },
      history: {
        heading: 'History reads like a document',
        body:
          'Every status change, comment, and linked PR in one scroll — with time-in-status computed as you read (waited 3d, in progress 5h). The changelog is the interface.',
      },
    },
    agent: {
      label: 'For the people building with agents',
      heading: 'One vocabulary between you and the agent',
      body:
        'The CLI doubles as the agent interface: create, claim, transition — verbs an agent can run while you watch the same board. An MCP server covers clients without a shell. Writes go through to the origin; reads come off the local mirror.',
      snippetCaption: 'Point an MCP client at gadak:',
      videoCaption: 'A question JQL cannot ask, answered from the local mirror.',
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
      '회사가 쓰는 그 트래커를 로컬 SQLite 파일 하나로 미러링합니다. 2만 건에서도 검색은 밀리초 단위. 읽기는 네트워크를 건드리지 않습니다.',
    nav: { demo: '라이브 데모', backlog: '공개 백로그', install: '설치', github: 'GitHub' },
    hero: {
      eyebrow: 'gadak',
      heading: '같은 지라, 기다림 없이.',
      lede:
        '회사가 이미 쓰는 트래커를 이 머신의 SQLite 파일 하나로 미러링합니다. 검색은 밀리초 안에 끝나고, 히스토리는 문서처럼 읽히고, 스피너는 사라집니다. 원본은 여전히 Jira — 기다리는 것만 없어집니다.',
      videoCaption: '2만 건 미러에서 타이핑 속도로 검색. 녹화본이지 애니메이션이 아닙니다.',
      doors: {
        demoTitle: '▶ 라이브 데모 열기',
        demoSub: '새로 씻어낸 샘플 미러 위의 실제 UI. 설치도 계정도 없이.',
        installTitle: '설치',
        installSub: 'Homebrew 한 줄. macOS 앱, Linux·Windows CLI.',
      },
    },
    speed: {
      label: '빠르다는 말 대신 측정값',
      heading: '직접 확인할 수 있는 숫자',
      note: '2만 건 미러, M4 Pro 실측. 전체 표는 docs/BENCHMARKS.md — 이 페이지의 모든 쿼리는 데모가 쓰는 것과 같은 SQLite 파일에서 돌아갑니다.',
      rows: [
        { what: '전문 검색, 2만 건', value: '약 2ms' },
        { what: '키 조회 → 이슈 열기', value: '110ms' },
        { what: '534건 사이트 풀 싱크', value: '약 5초' },
        { what: '변화 없는 증분 재실행', value: '쓰기 0건' },
      ],
    },
    ux: {
      label: '매일의 루프를 다시 설계',
      search: {
        heading: '타이핑을 따라오는 검색',
        body:
          '하나의 팔레트가 전부를 덮습니다 — 제목, 본문, 코멘트, 위키까지. 접두 매치는 단어를 끝내기 전에 로컬에서 뜨고, 전문 검색이 바로 뒤따릅니다. 스피너도 왕복도 없습니다.',
      },
      group: {
        heading: '어떤 축이든, 메뉴 하나',
        body:
          '담당자·우선순위·에픽으로 다시 묶으면 분포 막대가 따라옵니다. 저장된 뷰는 URL 하나 — 스크린샷 대신 정확한 조각을 공유하세요.',
      },
      history: {
        heading: '히스토리가 문서처럼 읽힌다',
        body:
          '상태 변화·코멘트·연결된 PR이 한 번의 스크롤에 — 읽는 시점에 체류 시간이 계산됩니다(대기 3일, 진행 5시간). 체인지로그가 곧 인터페이스입니다.',
      },
    },
    agent: {
      label: '에이전트와 함께 만드는 사람에게',
      heading: '나와 에이전트가 나누는 하나의 어휘',
      body:
        'CLI가 곧 에이전트 인터페이스입니다 — create, claim, transition, 보드는 눈앞에서 같이 움직입니다. MCP 서버가 셸 없는 클라이언트를 맡습니다. 쓰기는 origin까지 통과하고, 읽기는 로컬 미러에서 나옵니다.',
      snippetCaption: 'MCP 클라이언트를 gadak에 연결:',
      videoCaption: 'JQL이 표현 못 하는 질문 하나를, 로컬 미러에서 답하기.',
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
