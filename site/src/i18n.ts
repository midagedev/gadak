import { TAGLINE } from './tagline.js'

// The site's locales in one place: en at the root, the rest under their own
// prefix (pathFor below). Every locale-aware surface — layout, sitemap,
// switcher, banners — iterates this list, so a fourth locale is a list entry
// plus copy, not another ternary.
export const LOCALES = ['en', 'ko', 'ja'] as const
export type Locale = (typeof LOCALES)[number]

/**
 * One landing section. Each locale lists the sections it shows, in its own
 * order (`layout`). The set is the shape the 2026-09-08 review round
 * (GDK-1632) settled on: say what it is (hero), prove it before asking for a
 * token (query, speed, search, agent), put every fact a reader needs before
 * connecting work data in one place (connect), then install, then where the
 * project stands (status), then ask what happened (ask). `compare` is the
 * one en-only section — the objection an HN reader will type.
 */
export type Section =
  | 'hero'
  | 'query'
  | 'speed'
  | 'search'
  | 'agent'
  | 'compare'
  | 'connect'
  | 'install'
  | 'status'
  | 'ask'

/**
 * The canonical query and the link that runs it on the demo snapshot in the
 * browser. Both are contract strings (docs/project/FACT_LEDGER.md §7): the SQL
 * is what the README prints, and the Datasette Lite URL carries that same SQL
 * URL-encoded — change one and the other is a lie.
 */
export const CANONICAL_SQL = `gadak sql "select epic_key, count(*) from issues_full where resolved_at is null
           and epic_key <> '' group by epic_key order by 2 desc"`
export const DATASETTE_DEMO_URL =
  "https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc"

/**
 * The strings one locale carries. The three locales are parallel editions,
 * not translations (docs/project/FACT_LEDGER.md): they share facts, not
 * sentences or structure, so the optional sections below exist in some
 * locales and not others, and `layout` orders them per reader.
 */
export interface Strings {
  htmlLang: string
  ogLocale: string
  title: string
  description: string
  nav: { demo: string; changelog: string; install: string; github: string }
  copy: { label: string; copied: string }
  ogImageAlt: string
  langName: string
  langBanner: { offer: string; cta: string; dismiss: string }
  layout: readonly Section[]
  hero: {
    eyebrow: string
    heading: string
    lede: string
    videoCaption: string
    doors: { installTitle: string; installSub: string; demoTitle: string; demoSub: string }
  }
  /** The runnable proof: the canonical GROUP BY and Datasette Lite on the demo snapshot. */
  query?: {
    label: string
    heading: string
    lead: string
    datasetteLabel: string
    result: string
  }
  speed: {
    label: string
    heading: string
    note: string
    rows: ReadonlyArray<{ what: string; value: string; alt: string; ratio: string }>
    colRest: string
    colGadak: string
  }
  ux: { label: string; search: { heading: string; body: string } }
  agent: {
    label: string
    heading: string
    body: string
    setupLink: string
    driveCaption: string
    showcaseLink: string
    /** The skill + MCP commands, when this locale keeps them in the agent section (ko moves them under install). */
    setup?: { skillLead: string; mcpLead: string }
  }
  /** en only: the official hosted server, compared on capability and operating cost. */
  compare?: {
    label: string
    heading: string
    body: string
    colA: string
    colB: string
    rows: ReadonlyArray<{ what: string; a: string; b: string }>
    note: string
    noteLink: string
  }
  /** Before connecting work data: which Jira, how much is copied, where it lives, what leaves the machine. */
  connect: {
    label: string
    heading: string
    points: readonly string[]
    links: ReadonlyArray<{ href: string; label: string }>
  }
  changelog: {
    heading: string
    lede: string
    source: string
    jumpLabel: string
    fallbackNote: string
  }
  install: {
    label: string
    heading: string
    macosApp: string
    cliOnly: string
    windowsBefore: string
    windowsAfter: string
    firstRun: string
    /** The skill + MCP commands, when this locale places them after the first run (ko). */
    setup?: { skillLead: string; mcpLead: string }
  }
  /** Where the project stands: 0.x, one maintainer, license, what stays in Jira. */
  status: {
    label: string
    heading: string
    points: readonly string[]
    links: ReadonlyArray<{ href: string; label: string }>
  }
  /** The closing ask — what happened when you used it — and what must stay out of a public report. */
  ask: {
    label: string
    heading: string
    body: string
    caution: string
    links: ReadonlyArray<{ href: string; label: string }>
  }
  landing: { flagshipSlot: string; searchSlot: string; agentSlot: string; allPlatforms: string }
  /** `nameNote` is the one sentence the name gets, and the only place it is explained (fact ledger §1). */
  footer: { builtBy: string; whereBytes: string; nameNote: string }
  /**
   * Which hero door comes first, and which one is the primary button.
   * Default (omitted) is install then demo. `ja` reverses it: a Japanese
   * corporate machine often cannot run `brew`, so the browser demo is the
   * only trial that reader has (GDK-1623).
   */
  heroDoorOrder?: readonly ('install' | 'demo')[]
}

const GITHUB = 'https://github.com/midagedev/gadak'
const DOCS = `${GITHUB}/blob/main/`

export const strings: Record<Locale, Strings> = {
  en: {
    htmlLang: 'en',
    ogLocale: 'en_US',
    title: 'gadak — Query your Jira backlog with SQL',
    description:
      'gadak mirrors the Jira Cloud projects and Confluence spaces you choose into SQLite on your machine. Search with no network, query with SQL, and hand a coding agent the same file through a skill or an MCP server.',
    nav: { demo: 'Live demo', changelog: 'Changelog', install: 'Install', github: 'GitHub' },
    copy: { label: 'Copy', copied: 'Copied' },
    ogImageAlt:
      'gadak. Query your Jira backlog with SQL. Selected Jira Cloud projects and Confluence spaces, mirrored into SQLite on your machine.',
    langName: 'English',
    langBanner: {
      offer: 'This page is also available in English.',
      cta: 'View in English →',
      dismiss: 'Dismiss',
    },
    // The HN reader: what it is, the query they can run right now, the
    // measurement, the agent, the objection they were about to type, what to
    // check before connecting work data, install, status, and a way to say
    // what happened. No `search` section: daily search is table stakes for
    // this reader, the hero recording already shows it, and its one useful
    // claim now sits in the lede and the agent section (GDK-1623).
    layout: ['hero', 'query', 'speed', 'agent', 'compare', 'connect', 'install', 'status', 'ask'],
    hero: {
      eyebrow: 'gadak',
      heading: TAGLINE.en.heading,
      lede:
        'gadak mirrors the Jira Cloud projects and Confluence spaces you choose into one SQLite file on your machine. Issue titles, bodies, comments and wiki pages land in one search index, and searching it opens no connection. SQL over the same file answers the counts JQL cannot express. The desktop app, the browser tab, the CLI and a coding agent read that file; writes go to Jira first, and the file is a cache you can delete.',
      videoCaption:
        'Recording: search over a 20,000-issue mirror built from the demo snapshot. Screen capture, not an animation.',
      doors: {
        installTitle: 'Install',
        installSub: 'Homebrew on macOS, the Microsoft Store on Windows, a CLI for Linux that opens the same UI in a browser tab.',
        demoTitle: 'Live demo',
        demoSub: '534 issues in your browser. No install, no account.',
      },
    },
    query: {
      label: 'Count unresolved issues by epic',
      heading: 'JQL has no GROUP BY.',
      lead: 'The REST API returns issues a page at a time, and no totals. Once the data is a local file, the count is one query:',
      datasetteLabel: 'Run this query on the demo snapshot in your browser (Datasette Lite, nothing installed) →',
      result:
        'On a live Cloud site with 3,296 issues (2026-08-26, medians): 4,761 ms over 8 API pages aggregated client-side, against 22 ms for this query.',
    },
    speed: {
      label: 'Local reads compared with the Jira REST API',
      heading: 'The same questions, measured',
      note: 'Measured 2026-08-26 against a live Atlassian Cloud site: a real work project, 3,296 issues. Medians. The gadak numbers include full CLI process startup. In this measurement the epic count took 8 API pages, aggregated client-side, against one query on the mirror. The history count has no native aggregate; crawling and counting it client-side takes about 28 minutes. The rate-limit row is about reads, which stay local. A first full sync of that site took 10.6 minutes, and the mirror trails Jira by one sync interval. Method, re-measurements, and the rows where gadak loses: ',
      rows: [
        { what: 'Simple filter, 100 issues', value: '583 ms', alt: '19 ms', ratio: '31×' },
        { what: 'One issue + full changelog', value: '710 ms', alt: '28 ms', ratio: '25×' },
        { what: 'Free-text search', value: '543 ms', alt: '41 ms', ratio: '13×' },
        { what: 'Open issues per epic (GROUP BY)', value: '4,761 ms', alt: '22 ms', ratio: '214×' },
        { what: 'A count over the change history', value: 'no native aggregate', alt: '14 ms', ratio: '—' },
        { what: 'Rate limit on mirror reads', value: '429 + Retry-After', alt: 'none', ratio: '—' },
      ],
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    // en does not list `search` in its layout (see the comment on `layout`),
    // so these strings render only if that changes. ko and ja keep the
    // section and carry their own.
    ux: {
      label: 'Search',
      search: {
        heading: 'Search that keeps up with typing',
        body:
          'Issue titles, bodies, comments and wiki pages in one search, read from the local mirror. Results land before you finish the word; nothing waits on a round trip.',
      },
    },
    agent: {
      label: 'Coding agents on the same mirror',
      heading: 'Use the CLI from your coding agent',
      body:
        'You and your agent read the same file. The CLI is the agent interface: create, claim, transition, and SQL over the mirror, while the board in front of you updates. Writes go through Jira first, and an agent’s comments and the issues it creates carry its name. An agent that reads your mirror sends what it reads to whatever model it talks to; scope the mirror to what the agent should see.',
      setupLink: 'One paste per host → docs/AGENT_SETUP.md',
      driveCaption:
        'A Claude Code session in gadak’s own terminal pane filters the issue list, then saves and opens a dashboard in the same window. Time-lapsed while the agent works.',
      showcaseLink: 'More recordings: dashboards, a team theme, a launcher, a live MCP session → docs/SHOWCASE.md',
      setup: {
        skillLead: 'Install the skill for Claude Code:',
        mcpLead: 'For Claude Desktop, register the MCP server:',
      },
    },
    compare: {
      label: 'Compared with the official Atlassian MCP server',
      heading: 'Why not Rovo MCP?',
      body:
        'Rovo MCP is hosted by Atlassian and needs no local MCP server installation. It searches Jira and Confluence and has write tools, but no native aggregation tool and no offline reads. gadak runs SQL against a synchronized local mirror, so a count over the whole backlog or a join across the change history is one query. The costs are a local binary, an initial sync, and reads that trail Jira by one sync interval.',
      colA: 'Rovo MCP (hosted)',
      colB: 'gadak (local mirror)',
      rows: [
        { what: 'Where the query runs', a: 'Atlassian’s cloud', b: 'your machine, in SQLite' },
        { what: 'Open issues per epic', a: 'no native aggregation tool', b: 'one GROUP BY; 22 ms on 3,296 issues (2026-08-26)' },
        { what: 'Network required for search', a: 'yes', b: 'no, once the selected data is synced' },
        { what: 'Searches Jira and Confluence', a: 'yes', b: 'yes, for the spaces you select' },
        { what: 'Writes: comment, transition, assign', a: 'yes', b: 'yes, through Jira first' },
        { what: 'Freshness', a: 'Atlassian’s hosted data', b: 'the last synced state, one interval behind' },
        { what: 'Local setup', a: 'none', b: 'a local binary and an initial sync' },
      ],
      note: 'The same comparison for jira-cli, Linear and Jira’s own UI:',
      noteLink: 'docs/FAQ.md#how-it-compares',
    },
    connect: {
      label: 'Before you connect work data',
      heading: 'What gets copied, where it lives, and what leaves your machine',
      points: [
        'gadak connects to Jira Cloud with one API token; it covers Jira and Confluence on the same site. Server and Data Center are untested and not claimed.',
        'You choose the scope: <code>--projects</code> for Jira, <code>--spaces</code> for the wiki. The wiki stays off until you name spaces.',
        'The mirror is one SQLite file on your machine. It needs a first full sync, trails Jira by one sync interval, and can be deleted and rebuilt at any time.',
        'Credentials never reach SQLite, a log, or a snapshot.',
        'No telemetry, no analytics, no gadak account. The only connections gadak opens are the ones you configured; the complete list is in SECURITY.md.',
        'Writes go to Jira first and the mirror refreshes after Jira accepts. A write Jira did not accept fails then and there; nothing is queued locally.',
        'Four reads still open a connection: viewing an attachment, <code>gadak issue --editmeta</code>, <code>gadak fields</code>, and <code>gadak api</code>.',
        'An agent that reads the mirror sends what it reads to whatever model it talks to. gadak itself sends nothing. Scope the mirror to what the agent should see.',
      ],
      links: [
        { href: `${DOCS}SECURITY.md`, label: 'Every outbound destination and its condition → SECURITY.md' },
        { href: `${DOCS}docs/NETWORK.md`, label: 'Every connection and its off switch → docs/NETWORK.md' },
      ],
    },
    changelog: {
      heading: 'Changelog',
      lede:
        'Every release, in the words of the person who shipped it. Issue keys link into the ' +
        'public backlog, so a line here leads to the issue that asked for it.',
      source: 'Rendered from CHANGELOG.md in the repository.',
      jumpLabel: 'Jump to a version',
      // Renders only on locales whose changelog falls back to the English
      // file (changelogIsFallback); never on en itself.
      fallbackNote: 'This changelog is published in English.',
    },
    install: {
      label: 'Install and first sync',
      heading: 'Install',
      macosApp: 'The desktop app, CLI included:',
      cliOnly: 'CLI only, on macOS or Linux. The same UI opens in a browser tab via gadak serve:',
      windowsBefore: 'On Windows, the desktop app is on the',
      windowsAfter: '.',
      firstRun:
        'Connect to your team’s Jira Cloud site. It asks for the site, your email, an API token and the projects to mirror; add --spaces to include the wiki:',
    },
    status: {
      label: 'Where the project stands',
      heading: 'Status and limits',
      points: [
        'Status: 0.21, still 0.x. Sync, reads, write-through, desktop, web, CLI and MCP are verified against a live site.',
        'One maintainer, currently. Apache-2.0. The mirror is ordinary SQLite, and the 0.x contract is three promises: <code>issues_full</code> and the RECIPES queries, <code>gadak sql</code> stdout, and <code>gadak views open --keys -</code>.',
        'Keep sprint planning, administration, page editing in a UI, and anything that cannot tolerate a sync interval of delay in Jira.',
        'Three origins, one set of verbs: Atlassian Cloud, Linear, and the built-in tracker. What each one refuses is in one table.',
      ],
      links: [
        { href: `${DOCS}docs/SUPPORT_MATRIX.md`, label: 'What each origin supports' },
        { href: `${DOCS}README.md#alternatives-and-prior-work`, label: 'Earlier projects in this area' },
        { href: `${DOCS}docs/FAQ.md`, label: 'Hard questions' },
        { href: `${DOCS}docs/MAINTENANCE.md`, label: 'Who maintains this' },
        { href: GITHUB, label: 'Source' },
      ],
    },
    ask: {
      label: 'Report what happened',
      heading: 'If you used it on your own project, say so',
      body:
        'Tell us what question gadak answered, and whether you used it again.',
      caution: 'Keep real issue data, tokens, and site URLs out of public reports.',
      links: [{ href: `${GITHUB}/issues`, label: 'Open a GitHub issue' }],
    },
    // The landing's locale-varying fragments (MediaSlot labels, the
    // all-platforms link), kept here so the component holds no copy.
    landing: {
      flagshipSlot: 'recording · 20k mirror',
      searchSlot: 'search',
      agentSlot: 'agent in the window',
      allPlatforms: 'All platforms →',
    },
    footer: {
      builtBy: 'Built by',
      whereBytes: 'Where the bytes go',
      nameNote: 'gadak is Korean for a strand, a thread drawn from a tangle.',
    },
  },
  ko: {
    htmlLang: 'ko',
    ogLocale: 'ko_KR',
    title: 'gadak — 묵은 Jira 이슈를 Claude Code로 찾아봅니다',
    description:
      '필요한 Jira 프로젝트와 Confluence 스페이스만 골라 이 컴퓨터에 캐시합니다. 검색과 이슈 조회는 캐시에서 하고, Claude Code에는 스킬 하나로 넘깁니다. 텔레메트리는 없습니다.',
    nav: { demo: '라이브 데모', changelog: '체인지로그', install: '설치', github: 'GitHub' },
    copy: { label: '복사', copied: '복사됨' },
    ogImageAlt: 'gadak — 묵은 Jira 이슈를 Claude Code로 찾아봅니다. 필요한 Jira 프로젝트와 Confluence 스페이스만 골라 캐시합니다.',
    langName: '한국어',
    langBanner: {
      offer: '이 페이지는 한국어로도 볼 수 있습니다.',
      cta: '한국어로 보기 →',
      dismiss: '닫기',
    },
    // 트위터에서 온 독자. 무엇을 왜 만들었는지(1인칭) → 무엇을 찾아볼 수
    // 있는지(검색, Claude Code) → 잰 값 → 회사 데이터를 두고 쓰는 도구라서
    // 먼저 알아야 할 것 → 설치(동기화 뒤에 스킬·MCP) → 어디까지 왔는지 →
    // 한 줄 부탁. 비교표는 없다. 이 독자는 공식 MCP와 비교하러 오지 않는다.
    // 절 이름은 이 독자에게 맞춰 따로 지었다. 다른 두 판본과 절 대 절로
    // 맞추지 않는다(GDK-1623).
    layout: ['hero', 'search', 'agent', 'speed', 'connect', 'install', 'status', 'ask'],
    hero: {
      eyebrow: 'gadak',
      heading: TAGLINE.ko.heading,
      lede:
        '지라를 쓰기 싫은데 어쩔 수 없이 써야 해서 만들었습니다. 묵은 이슈들을 클로드로 뒤지다가 한참 걸리고 결국 rate limit에 걸려 중단된 적이 있는데, 그때 시작했습니다. 크롬에 지라 탭이 잔뜩 쌓여 피곤해지는 것도 겸사겸사 없애고 싶었고요. 필요한 Jira 프로젝트와 Confluence 스페이스만 골라 캐시해 두고, 검색과 이슈 조회는 거기서 합니다. 직접 찾아도 되고, Claude Code에 맡겨도 됩니다.',
      videoCaption: '검색 녹화입니다. 데모 데이터를 이슈 2만 건으로 늘린 캐시에서 찍었고, 글자를 치는 대로 결과가 바뀝니다.',
      doors: {
        installTitle: '설치',
        installSub: 'macOS 앱은 Homebrew로, Windows 앱은 Microsoft Store에서. Linux는 CLI를 설치하고 gadak serve로 브라우저에서 씁니다.',
        demoTitle: '라이브 데모',
        demoSub: '이슈 534건이 들어 있는 데모를 브라우저에서 바로 엽니다. 설치도 계정도 필요 없습니다.',
      },
    },
    speed: {
      label: '잰 값',
      heading: 'Jira API에 물을 때와 캐시에 물을 때',
      note: '2026-08-26에 실제 Atlassian Cloud 업무 프로젝트(이슈 3,296건)에서 잰 중앙값입니다. 에픽별 열린 이슈는 REST API 쪽이 응답 8페이지를 받아 합산한 값이고, gadak 쪽은 쿼리 한 번에 CLI 프로세스가 뜨는 시간까지 넣은 값입니다. 첫 전체 동기화는 그 사이트에서 10.6분 걸렸고, 그 뒤로 캐시는 동기화 주기만큼 늦습니다. 측정 방법과 나머지 행: ',
      rows: [
        { what: '텍스트 검색', value: '543 ms', alt: '41 ms', ratio: '13×' },
        { what: '에픽별 열린 이슈 (GROUP BY)', value: '4,761 ms', alt: '22 ms', ratio: '214×' },
      ],
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    ux: {
      label: '검색',
      search: {
        heading: '이슈, 댓글, 위키를 한 검색창에서',
        body:
          '이슈 제목과 본문, 댓글, 위키 문서를 한 검색창에서 찾습니다. 단어를 다 치기 전에 결과가 나옵니다. 캐시에서 찾기 때문에 서버 응답을 기다리지 않습니다.',
      },
    },
    agent: {
      label: 'Claude Code에 맡기기',
      heading: '스킬 하나면 Claude Code가 같은 캐시를 읽습니다',
      body:
        '스킬을 설치하면 Claude Code가 gadak CLI로 이슈를 찾고, 만들고, 상태를 옮깁니다. 스킬에는 한국어 계정에서 에이전트가 자주 빠지는 함정도 적어 두었습니다. Jira는 상태와 우선순위 이름을 계정 언어로 번역합니다. 그래서 영어 이름으로 물으면 오류 없이 빈 결과가 돌아오고, 에이전트는 그걸 "그런 이슈는 없다"로 읽습니다. 에이전트가 남긴 댓글과 만든 이슈에는 에이전트 이름이 붙습니다. 에이전트는 읽은 내용을 자기 모델로 보냅니다. 캐시 범위는 그걸 감안해 정하세요.',
      setupLink: '도구별 연결 방법 → docs/AGENT_SETUP.md',
      driveCaption:
        'gadak 앱 안의 터미널에서 Claude Code에 한국어로 부탁해 이슈 목록을 바꾸고, 라벨 비율 대시보드를 저장해 여는 장면입니다. 에이전트가 일하는 구간은 빨리 감았습니다.',
      showcaseLink: '녹화 더 보기: 대시보드, 팀 테마, 런처, MCP 세션 → docs/SHOWCASE.md',
    },
    connect: {
      label: '연결하기 전에',
      heading: '회사 데이터를 두고 쓰는 도구라서, 먼저 알아야 할 것',
      points: [
        '연결되는 Jira는 Atlassian Cloud입니다. API 토큰 하나로 같은 사이트의 Jira와 Confluence에 붙습니다. Server와 Data Center는 아직 확인하지 않아서, 된다고 말하지 않습니다.',
        '가져올 범위는 직접 정합니다. <code>--projects</code>로 Jira 프로젝트를, <code>--spaces</code>로 위키 스페이스를 고릅니다. 스페이스를 지정하기 전에는 위키를 가져오지 않습니다.',
        '캐시는 이 컴퓨터 안의 SQLite 파일 하나입니다. 처음 한 번은 전체 동기화가 필요하고, 그 뒤로는 동기화 주기만큼 늦습니다. 지워도 되고, 다시 동기화하면 그대로 돌아옵니다.',
        'API 토큰은 캐시에도, 로그에도, 스냅샷에도 남지 않습니다.',
        '텔레메트리는 없습니다. gadak이 연결하는 곳은 직접 설정한 곳뿐이고, 전체 목록은 SECURITY.md에 있습니다.',
        '쓰기는 Jira로 먼저 갑니다. Jira가 받아들이면 캐시를 갱신하고, 거절하면 그 자리에서 실패합니다. 캐시에 쌓아 두고 나중에 보내는 일은 없습니다.',
        '첨부파일 보기와 편집 가능 필드 조회처럼 몇 가지 읽기는 Jira에 직접 묻습니다.',
        '캐시를 읽는 에이전트는 읽은 내용을 자기 모델로 보냅니다. gadak 자신은 아무 데도 보내지 않습니다.',
      ],
      links: [
        { href: `${DOCS}SECURITY.md`, label: '밖으로 나가는 연결 전체와 그 조건 → SECURITY.md' },
        { href: `${DOCS}docs/NETWORK.md`, label: '연결마다 끄는 방법 → docs/NETWORK.md' },
      ],
    },
    changelog: {
      heading: '체인지로그',
      lede: '릴리스마다 만든 사람이 직접 씁니다. 이슈 키를 누르면 공개 백로그의 그 이슈로 갑니다.',
      source: '저장소의 CHANGELOG.ko.md를 그대로 보여 줍니다. 영문 CHANGELOG.md를 한국어로 옮긴 것입니다.',
      jumpLabel: '버전으로 이동',
      // Renders only on locales whose changelog falls back to the English
      // file (changelogIsFallback) — never here.
      fallbackNote: 'This changelog is published in English.',
    },
    install: {
      label: '설치',
      heading: '설치와 첫 동기화',
      macosApp: '데스크톱 앱(CLI 포함):',
      cliOnly: 'CLI만:',
      windowsBefore: 'Windows 데스크톱 앱은',
      windowsAfter: '에 있습니다.',
      firstRun:
        '회사 Jira Cloud에 연결합니다. 사이트 주소, 이메일, API 토큰, 캐시할 프로젝트를 차례로 묻습니다. 위키까지 캐시하려면 --spaces를 붙입니다:',
      setup: {
        skillLead: '동기화가 끝나면 Claude Code용 스킬을 설치합니다:',
        mcpLead: 'Claude Desktop에는 MCP 서버를 등록합니다:',
      },
    },
    status: {
      label: '어디까지 왔는지',
      heading: '0.21, 만드는 사람 한 명, Apache-2.0',
      points: [
        '버전 0.21, 아직 0.x입니다. 동기화, 읽기 API, Jira를 먼저 거치는 쓰기, 데스크톱·웹·CLI·MCP를 실제 사이트에서 확인했습니다.',
        '0.x에서 바꾸지 않기로 약속한 것은 셋입니다. <code>issues_full</code>과 RECIPES 쿼리, <code>gadak sql</code>의 출력 형식, <code>gadak views open --keys -</code>의 의미.',
        '스프린트 계획, Jira 대시보드와 알림, 관리자 작업, 1분의 지연도 안 되는 일은 Jira에서 계속 합니다.',
        'Atlassian Cloud, Linear, 내장 트래커에서 같은 명령을 씁니다. 서비스별로 되는 것과 안 되는 것은 표 하나에 정리해 두었습니다.',
      ],
      links: [
        { href: `${DOCS}docs/SUPPORT_MATRIX.md`, label: '서비스별 지원 범위' },
        { href: `${DOCS}docs/FAQ.md`, label: '자주 묻는 질문' },
        { href: `${DOCS}CHANGELOG.ko.md`, label: '무엇이 나왔는지' },
        { href: GITHUB, label: 'GitHub' },
      ],
    },
    ask: {
      label: '써 보셨다면',
      heading: '한 줄 남겨 주세요',
      body:
        '텔레메트리가 없어서 누가 쓰는지 저는 모릅니다. 써 보셨다면 어떤 일에 썼고 어땠는지 한 줄 알려 주세요. 불편했거나 결과가 틀렸다면 그 이야기도요. 이슈 수는 공개해도 괜찮을 때만 적어 주세요.',
      caution: '공개된 곳에는 실제 이슈 데이터나 토큰, 사이트 URL을 붙이지 마세요.',
      links: [
        { href: `${GITHUB}/issues`, label: 'GitHub 이슈로' },
        { href: 'https://x.com/midagedev', label: 'X @midagedev 멘션으로' },
      ],
    },
    landing: {
      flagshipSlot: '검색 녹화 · 이슈 2만 건',
      searchSlot: '검색',
      agentSlot: '앱 안의 Claude Code',
      allPlatforms: '다른 플랫폼 →',
    },
    footer: {
      builtBy: '만든 사람',
      whereBytes: '데이터가 어디로 가는지',
      nameNote: "이름은 엉킨 실에서 뽑아낸 한 줄기, '가닥'에서 왔습니다.",
    },
  },
  ja: {
    htmlLang: 'ja',
    ogLocale: 'ja_JP',
    title: 'gadak｜Jira の課題を SQL で集計する',
    description:
      'gadak は、指定した範囲の Jira と Confluence をキャッシュするツールです。課題・コメント・変更履歴・wiki ページをまとめて検索でき、SQL で集計できます。テレメトリはありません。',
    nav: { demo: 'ライブデモ', changelog: '変更履歴', install: 'インストール', github: 'GitHub' },
    copy: { label: 'コピー', copied: 'コピーしました' },
    ogImageAlt:
      'gadak｜Jira の課題を SQL で集計する。指定した範囲の Jira と Confluence をキャッシュし、まとめて検索できます。',
    langName: '日本語',
    langBanner: {
      offer: 'このページは日本語でも読めます。',
      cta: '日本語で表示 →',
      dismiss: '閉じる',
    },
    // Qiita・Zenn の検索から来る読者: 何をする道具か → いま試せる SQL → 計測値
    // → 検索 → 導入前に確認したいこと（この市場では購買条件）→ インストール
    // → エージェント → 対応範囲と開発状況 → 試した結果を教えてください。
    layout: ['hero', 'query', 'speed', 'search', 'connect', 'install', 'agent', 'status', 'ask'],
    // 会社の PC は brew を止められていることが多く、ブラウザーで試すのが唯一の
    // 入口になる読者がいる。だからデモのカードを先に置く (GDK-1623)。
    heroDoorOrder: ['demo', 'install'],
    hero: {
      eyebrow: 'gadak',
      heading: TAGLINE.ja.heading,
      lede:
        'gadak は、指定した範囲の Jira と Confluence をキャッシュするツールです。課題・コメント・変更履歴・wiki ページをまとめて検索でき、SQL で集計できます。デスクトップアプリ、ブラウザー、CLI から使えて、画面は日本語表示に対応しています。接続できるのは Jira Cloud です。',
      videoCaption: '検索の録画です。デモのスナップショットを課題 2 万件に増やしたキャッシュを検索していて、文字を打つ速さに結果が追いつきます。',
      doors: {
        installTitle: 'インストール',
        installSub: 'macOS は Homebrew、Windows は Microsoft Store から入れます。Linux は CLI を入れて、gadak serve でブラウザーから使います。',
        demoTitle: 'ライブデモ',
        demoSub: 'ブラウザーの中で 534 件の課題をそのまま開けます。インストールもアカウントも要りません。',
      },
    },
    query: {
      label: 'エピックごとの未完了件数を数える',
      heading: 'JQL に GROUP BY はありません。',
      lead: 'ページサイズを超えると、API が返すのは行だけで、集計は自分で書くことになります。キャッシュがあれば、同じ答えを SQL 1 本で出せます:',
      datasetteLabel: 'デモデータでこの SQL を試す（Datasette Lite、インストール不要）→',
      result:
        '2026-08-26 に課題 3,296 件のサイトで計測したところ、REST API の結果を 8 ページ取得して集計する処理は 4,761 ms、同期済みのキャッシュに対する gadak の SQL は 22 ms でした。数値は中央値で、gadak 側は CLI の起動時間を含みます。',
    },
    speed: {
      label: '計測条件と制約',
      heading: 'REST API とキャッシュの応答時間',
      note: '上と同じ、2026-08-26 の計測です。エピックごとの未完了件数は、REST API 側が 8 ページを取得して集計した値、gadak 側はクエリ 1 本の値です。gadak が負ける行もあります。このサイトの初回フル同期には 10.6 分かかり、キャッシュは同期間隔 1 回ぶん遅れます。計測方法と再計測の履歴は次にまとめています: ',
      rows: [
        { what: '全文検索', value: '543 ms', alt: '41 ms', ratio: '13×' },
        { what: 'エピックごとの未完了件数（GROUP BY）', value: '4,761 ms', alt: '22 ms', ratio: '214×' },
      ],
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    ux: {
      label: '課題と wiki をまとめて検索する',
      search: {
        heading: '入力に追いつく検索',
        body:
          '課題のタイトル・本文・コメントと wiki ページを、まとめて全文検索できます。検索はキャッシュの中だけで行うので、単語を打ち終える前に結果が返り、通信を待つ時間もありません。',
      },
    },
    agent: {
      label: 'コーディングエージェントから使う',
      heading: 'エージェントから課題を操作する',
      body:
        'スキルを 1 つ入れると、Claude Code が gadak の CLI で課題を検索し、作成し、ステータスを変更します。その結果は同じボードで確認できます。スキルには、日本語のアカウントで引っかかりやすい落とし穴も書いてあります。ステータスと優先度の表示名は、Jira がアカウントの言語ごとに翻訳するので、英語の名前で問い合わせるとエラーも出ないまま 0 行が返り、エージェントはそれを「該当する課題はない」と読みます。エージェントが書いたコメントと作成した課題には、そのエージェントの名前が付きます。',
      setupLink: 'ホストごとの設定 → docs/AGENT_SETUP.md',
      driveCaption:
        '画面もプロンプトも日本語で収録した、Claude Code のライブセッションです。gadak のターミナルペインで動かしていて、1 文目で課題の一覧が変わり、2 文目で同じウィンドウにダッシュボードが開きます。エージェントが作業している区間は早送りです。',
      showcaseLink: 'ほかの録画: ダッシュボード、チームのテーマ、ランチャー、MCP のライブセッション → docs/SHOWCASE.md',
      setup: {
        skillLead: 'Claude Code にスキルを入れます:',
        mcpLead: 'Claude Desktop には MCP サーバーを登録します:',
      },
    },
    connect: {
      label: '導入前に確認したいこと',
      heading: '何を写し、どこに置き、何が外に出るのか',
      points: [
        '対応しているのは Jira Cloud です。API トークン 1 つで、同じサイトの Jira と Confluence の両方に接続します。Server / Data Center は検証していないため、対応対象にはしていません。',
        '写す範囲は自分で決めます。Jira は <code>--projects</code>、wiki は <code>--spaces</code> で絞ります。スペースを指定するまで wiki は同期されません。',
        'キャッシュの実体は、使っているマシンの中の SQLite ファイル 1 つです。使い始める前に初回のフル同期が必要で、その後の読み取りは直近の同期時点の内容になります。ディレクトリごと消しても失うものはなく、もう一度同期すれば作り直せます。',
        'API トークンの置き場所は <code>~/.gadak/config.json</code> で、パスはどの OS でも同じです。パーミッション 0600 で書き込まれ、送られる先は自分のサイトへの <code>Authorization</code> ヘッダーだけです。キャッシュ・ログ・スナップショットのどこにも書き込まれません。',
        'テレメトリも、解析も、gadak のアカウントもありません。外に出る通信は、自分で設定した接続先と、自分で実行したコマンドのぶんだけです。一覧とその条件は SECURITY.md にまとめています。',
        '書き込みは先に Jira へ届き、Jira が受け付けてからキャッシュが更新されます。届かなかった書き込みはその場で失敗として返り、キャッシュに溜まることはありません。',
        '同期済みの課題や wiki ページを開くときは、キャッシュだけを読みます。例外は接続先に訊く必要がある操作で、添付ファイルの表示、編集できる項目の問い合わせ、素通しのリクエストがそれにあたります。',
        'コーディングエージェントにキャッシュを読ませると、読んだ内容はそのエージェントの背後にあるモデルへ送られます。gadak 自体が課題データを外部のモデルへ送ることはありません。エージェントに見せてよいプロジェクトとスペースだけを写してください。',
      ],
      links: [
        { href: `${DOCS}SECURITY.md`, label: '外に出る通信の一覧と条件 → SECURITY.md' },
        { href: `${DOCS}docs/NETWORK.md`, label: '各接続を無効にする方法 → docs/NETWORK.md' },
      ],
    },
    changelog: {
      heading: '変更履歴',
      lede:
        'リリースの内容は、出した本人が書いています。課題キーは公開バックログにリンクしているので、ここの 1 行から、それを求めた課題までさかのぼれます。',
      source: 'リポジトリの CHANGELOG.md から描画しています。',
      jumpLabel: 'バージョンへ移動',
      // Not a placeholder like the rest: this one already renders (the ja
      // page reads the English changelog), so the sentence is real copy.
      fallbackNote: 'この変更履歴は英語で公開しています。',
    },
    install: {
      label: 'インストールと初回同期',
      heading: 'インストール',
      macosApp: 'デスクトップアプリ（CLI 同梱）:',
      cliOnly: 'CLI のみ:',
      windowsBefore: 'Windows のデスクトップアプリは',
      windowsAfter: 'からインストールできます。',
      firstRun:
        'チームの Jira Cloud サイトに接続します。サイト、メールアドレス、API トークン、写すプロジェクトを順に聞かれます。wiki も同期するなら --spaces を付けます:',
    },
    status: {
      label: '開発状況',
      heading: '対応範囲と開発状況',
      points: [
        '状態: 0.21、まだ 0.x です。同期、読み取り API、書き込み、デスクトップアプリ、ウェブ、CLI、MCP は、実際のサイトで検証しています。',
        'メンテナーは現在 1 人です。ライセンスは Apache-2.0 で、0.x の間に互換性を約束しているのは、<code>issues_full</code> と RECIPES のクエリ、<code>gadak sql</code> の標準出力の形式、<code>gadak views open --keys -</code> の意味の 3 つです。',
        'スプリント計画、管理作業、アプリの画面でのページ編集、1 分の遅れが問題になる作業は、Jira 側で続けてください。',
        'Atlassian Cloud、Linear、内蔵トラッカーを共通のコマンドで操作できます。接続先ごとの対応状況は 1 枚の表にまとめています。',
      ],
      links: [
        { href: `${DOCS}docs/SUPPORT_MATRIX.md`, label: '接続先ごとの対応状況' },
        { href: `${DOCS}docs/FAQ.md`, label: 'よくある質問' },
        { href: `${DOCS}docs/MAINTENANCE.md`, label: '保守の方針' },
        { href: GITHUB, label: 'GitHub' },
      ],
    },
    ask: {
      label: 'フィードバック',
      heading: '試した結果を教えてください',
      body:
        '検索や集計で試した結果、困った点、導入を見送った理由も GitHub issue で教えてください。どんな質問に gadak で答えたか、その後も使ったかが分かると助かります。遅かった、間違っていたという場合も、そのまま書いてください。',
      caution: '実際の課題データ、API トークン、サイト URL は載せないでください。',
      links: [{ href: `${GITHUB}/issues`, label: 'GitHub issue を開く' }],
    },
    landing: {
      flagshipSlot: '録画 · 課題 2 万件',
      searchSlot: '検索',
      agentSlot: 'ウィンドウの中のエージェント',
      allPlatforms: 'すべてのプラットフォーム →',
    },
    footer: {
      builtBy: '作者:',
      whereBytes: 'バイトの行き先',
      nameNote: '名前は、絡まった糸の一本を表す韓国語「가닥」に由来します。',
    },
  },
}

/** '' for the default locale, '/ko' / '/ja' for the prefixed ones. */
export function localePrefix(l: Locale): string {
  return l === 'en' ? '' : `/${l}`
}

/** The same page in another locale: pathFor('ko', '/') is '/ko/', pathFor('ko', '/install/') is '/ko/install/', pathFor('en', x) is x. */
export function pathFor(l: Locale, enPath: string): string {
  if (l === 'en') return enPath
  return `${localePrefix(l)}${enPath === '/' ? '/' : enPath}`
}

/**
 * Which media files have a cut of their own per locale, and in which
 * locales that cut exists.
 *
 * Declared, never probed. `site/public/media` is a symlink the build creates
 * (Makefile `site:`), so at authoring time there is no directory to look in,
 * and a build that silently fell back because a file was missing is exactly
 * the failure mode this map exists to make loud: `tools/doc-checks.sh` reads
 * it and fails when a listed locale has no file in `docs/media/`.
 *
 * `en` is never listed — it is the bare filename every entry derives from.
 * Adding a locale here is what turns a recorded variant on; recording it and
 * forgetting this map ships the English clip, and listing it here without the
 * recording turns the doc-check red. Either way, nothing silent.
 *
 * Keys are the exact path the page asks for, so the entry greps.
 */
export const MEDIA_LOCALES: Record<string, readonly Exclude<Locale, 'en'>[]> = {
  // The share card is a Node render, not a recording — all three exist.
  '/media/og.png': ['ko', 'ja'],
  // The three landing clips and their posters. Every one of them carries the
  // product's own UI — and, since GDK-1556, a mirror translated into the same
  // language — in its pixels, so each is a per-language asset. The hero
  // joined this list on 2026-09-07 (the user reversed "terminal-hero has no
  // variants"): its ko and ja takes are one language end to end, prompts
  // included. An entry without a locale serves that locale the English take;
  // doc-checks #40/#41 keep a listed locale, its files and its fixture
  // translation together.
  '/media/scale.mp4': ['ko', 'ja'],
  '/media/scale-poster.png': ['ko', 'ja'],
  '/media/search.mp4': ['ko', 'ja'],
  '/media/search-poster.png': ['ko', 'ja'],
  '/media/terminal-hero.mp4': ['ko', 'ja'],
  '/media/terminal-hero-poster.png': ['ko', 'ja'],
}

/**
 * The path to `file` for `lang`: the locale tag goes in as the last segment
 * before the extension, so `/media/scale.mp4` becomes `/media/scale.ja.mp4`
 * and `/media/scale-poster.png` becomes `/media/scale-poster.ja.png`. The
 * export scripts write exactly that shape (`e2e/demo/export-scale.sh`).
 *
 * A path with no cut in this locale — and every path in `en` — comes back
 * unchanged, so a caller never has to know which is which. A path missing
 * from MEDIA_LOCALES entirely is also returned unchanged: an asset that is
 * one file in every language (the app stills, the CLI shots) needs no entry.
 */
export function mediaFor(lang: Locale, path: string): string {
  if (lang === 'en') return path
  if (!MEDIA_LOCALES[path]?.includes(lang)) return path
  const dot = path.lastIndexOf('.')
  if (dot <= path.lastIndexOf('/')) return path
  return `${path.slice(0, dot)}.${lang}${path.slice(dot)}`
}

// Matches a locale prefix only as a full first segment, so /changelog/ or a
// hypothetical /kotlin/ page is never stripped.
const NON_DEFAULT_PREFIX = new RegExp(`^/(?:${LOCALES.filter((l) => l !== 'en').join('|')})(?=/|$)`)

/** Strips any locale prefix: '/ja/install/' → '/install/', '/ko/' → '/'. */
export function enPathOf(pathname: string): string {
  return pathname.replace(NON_DEFAULT_PREFIX, '') || '/'
}

/** LOCALES minus `l`, in LOCALES order. */
export function otherLocales(l: Locale): Locale[] {
  return LOCALES.filter((x) => x !== l)
}
