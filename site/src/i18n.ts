import { TAGLINE } from './tagline.js'

// The site's locales in one place: en at the root, the rest under their own
// prefix (pathFor below). Every locale-aware surface — layout, sitemap,
// switcher, banners — iterates this list, so a fourth locale is a list entry
// plus copy, not another ternary.
export const LOCALES = ['en', 'ko', 'ja'] as const
export type Locale = (typeof LOCALES)[number]

/** One landing section. Each locale lists the sections it shows, in its own order (`layout`). */
export type Section = 'hero' | 'compare' | 'speed' | 'search' | 'agent' | 'origin' | 'install' | 'ask'

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
  /** en only: the question every HN/Reddit thread opens with, answered before it is asked. */
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
    skillLead: string
    mcpLead: string
    setupLink: string
    driveCaption: string
    showcaseLink: string
  }
  origin: {
    label: string
    heading: string
    points: readonly string[]
    /** Optional pointer under the list (ja links SECURITY.md here). */
    link?: { href: string; label: string }
  }
  /** ko only: the one sentence the maintainer is asking for. */
  ask?: {
    label: string
    heading: string
    body: string
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
    heading: string
    macosApp: string
    cliOnly: string
    windowsBefore: string
    windowsAfter: string
    firstRun: string
  }
  landing: { flagshipSlot: string; searchSlot: string; agentSlot: string; allPlatforms: string }
  footer: { builtBy: string; whereBytes: string }
}

const GITHUB = 'https://github.com/midagedev/gadak'

export const strings: Record<Locale, Strings> = {
  en: {
    htmlLang: 'en',
    ogLocale: 'en_US',
    title: 'gadak — Same Jira. No waiting.',
    description:
      'Query Jira with SQL, search it offline, and hand your coding agent a local MCP server. gadak mirrors Jira and Confluence into one SQLite file on your machine. Reads never touch the network.',
    nav: { demo: 'Live demo', changelog: 'Changelog', install: 'Install', github: 'GitHub' },
    copy: { label: 'Copy', copied: 'Copied' },
    ogImageAlt:
      'gadak — Same Jira. No waiting. Your team’s Jira and its Confluence wiki, mirrored into one local SQLite file.',
    langName: 'English',
    langBanner: {
      offer: 'This page is also available in English.',
      cta: 'View in English →',
      dismiss: 'Dismiss',
    },
    // The HN reader decides in this order: the pain, the objection they were
    // about to type, the measurement, the daily loop, the agent, trust, install.
    layout: ['hero', 'compare', 'speed', 'search', 'agent', 'origin', 'install'],
    hero: {
      eyebrow: 'gadak',
      heading: TAGLINE.en.heading,
      lede:
        'JQL has no GROUP BY. Past one page of results the API hands you rows and leaves the counting to you. gadak keeps your Jira and Confluence in one SQLite file on your machine, so the question is one query and the answer is milliseconds. Offline search, real SQL, an MCP server for your agent. Jira stays the source of truth; the file is a cache.',
      videoCaption: 'A 20,000-issue mirror. Search as fast as you can type. Recorded, not animated.',
      doors: {
        installTitle: 'Install',
        installSub: 'Homebrew on macOS, the Microsoft Store on Windows, a CLI for Linux.',
        demoTitle: 'Live demo',
        demoSub: '534 issues in your browser. No install, no account.',
      },
    },
    compare: {
      label: 'The first question in every thread',
      heading: 'Why not the official Atlassian MCP server?',
      body:
        'Use it when the agent has to act on Jira right now. It is hosted by Atlassian, searches Jira and Confluence together, and needs nothing installed. The difference is what happens after the search. A hosted server answers one question per round trip, cannot aggregate, and does not work offline. gadak is the other half: the same data as a file on your disk, so a count over the whole backlog, a join across the change history, or a search on a plane is one query.',
      colA: 'Rovo MCP (hosted)',
      colB: 'gadak (local file)',
      rows: [
        { what: 'Where the query runs', a: 'Atlassian’s cloud', b: 'your disk' },
        { what: 'Open issues per epic', a: 'no such tool', b: 'one GROUP BY, 22 ms' },
        { what: 'Search across 20,000 issues', a: 'one round trip per question', b: 'milliseconds, offline' },
        { what: 'Confluence in the same index', a: 'yes', b: 'yes' },
        { what: 'Writes: comment, transition, assign', a: 'yes', b: 'yes, through Jira first' },
        { what: 'Freshness', a: 'live', b: 'one sync interval behind' },
        { what: 'Acting on Jira in real time', a: 'the right tool', b: 'not what this is for' },
      ],
      note: 'jira-cli, Linear and Jira’s own UI, compared the same way:',
      noteLink: 'docs/FAQ.md#how-it-compares',
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
        { what: 'Rate limit', value: '429 + Retry-After', alt: 'none — your disk', ratio: '—' },
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
      // Renders only on locales whose changelog falls back to the English
      // file (changelogIsFallback); never on en itself.
      fallbackNote: 'This changelog is published in English.',
    },
    install: {
      heading: 'Install',
      macosApp: 'The desktop app, CLI included:',
      cliOnly: 'CLI only:',
      windowsBefore: 'On Windows, the desktop app is on the',
      windowsAfter: '.',
      firstRun: 'Connect to your team\'s Jira (asks for site, email, token, projects):',
    },
    // The landing's locale-varying fragments (MediaSlot labels, the
    // all-platforms link) — kept here so the component holds no copy.
    landing: {
      flagshipSlot: 'flagship · 20k mirror',
      searchSlot: 'search',
      agentSlot: 'agent in the window',
      allPlatforms: 'All platforms →',
    },
    footer: {
      builtBy: 'Built by',
      whereBytes: 'Where the bytes go',
    },
  },
  ko: {
    htmlLang: 'ko',
    ogLocale: 'ko_KR',
    title: 'gadak — 같은 Jira, 기다림 없이.',
    description:
      '지라를 쓰기 싫은데 써야 하는 사람을 위한 앱입니다. Jira와 Confluence를 통째로 캐시해서 검색은 밀리초 안에 끝나고, 읽기는 네트워크를 타지 않습니다. 코딩 에이전트에게 그대로 넘길 수 있습니다.',
    nav: { demo: '라이브 데모', changelog: '체인지로그', install: '설치', github: 'GitHub' },
    copy: { label: '복사', copied: '복사됨' },
    ogImageAlt: 'gadak — 같은 Jira, 기다림 없이. 팀의 Jira와 Confluence 위키를 통째로 캐시합니다.',
    langName: '한국어',
    langBanner: {
      offer: '이 페이지는 한국어로도 볼 수 있습니다.',
      cta: '한국어로 보기 →',
      dismiss: '닫기',
    },
    // 트위터에서 온 독자: 왜 만들었나 → 숫자 → 매일 쓰는 장면 → 에이전트 →
    // 안심 → 설치, 그리고 마지막에 한 줄 부탁. 비교표는 없다 — 이 독자는
    // 공식 MCP와 비교하러 오지 않는다.
    layout: ['hero', 'speed', 'search', 'agent', 'origin', 'install', 'ask'],
    hero: {
      eyebrow: 'gadak',
      heading: TAGLINE.ko.heading,
      lede:
        '지라를 쓰기 싫은데 어쩔 수 없이 써야 해서 만들었습니다. 회사 Jira를 Confluence 위키까지 통째로 캐시해 두고, 검색과 읽기는 캐시에서 끝냅니다. 그래서 스피너가 없습니다. 쓰기는 Jira로 갑니다.',
      videoCaption: '이슈 2만 건에서 타이핑하는 속도로 검색합니다. 실제 화면 녹화입니다.',
      doors: {
        installTitle: '설치',
        installSub: 'macOS는 Homebrew, Windows는 Microsoft Store, Linux는 CLI.',
        demoTitle: '라이브 데모',
        demoSub: '이슈 534건을 브라우저에서 바로. 설치도 계정도 없습니다.',
      },
    },
    speed: {
      label: '빠르다는 말 대신 숫자로',
      heading: '같은 질문을 두 가지 방법으로',
      note: '2026-08-26에 실제 Atlassian Cloud 사이트(실제 업무 프로젝트, 이슈 3,296건)에서 잰 값입니다. 합성 데이터가 아닙니다. gadak 쪽 수치에는 CLI 프로세스 시작 시간까지 들어 있습니다. 측정 방법과 재측정 이력, gadak이 더 느린 경우까지 정리한 표:',
      rows: [
        { what: '단순 필터, 100건', value: '583 ms', alt: '19 ms', ratio: '31×' },
        { what: '이슈 1건 + 체인지로그 전체', value: '710 ms', alt: '28 ms', ratio: '25×' },
        { what: '전문 검색', value: '543 ms', alt: '41 ms', ratio: '13×' },
        { what: '에픽별 열린 이슈 (GROUP BY)', value: '4,761 ms, API 호출 8페이지', alt: '22 ms, 쿼리 한 번', ratio: '214×' },
        { what: '변경 이력 집계', value: 'JQL로는 표현 불가', alt: '14 ms', ratio: '—' },
        { what: '요청 제한', value: '429 + Retry-After', alt: '없음', ratio: '—' },
      ],
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    ux: {
      label: '매일 반복하는 일',
      search: {
        heading: '타이핑을 따라오는 검색',
        body:
          '제목, 본문, 코멘트, 위키까지 한 팔레트에서 찾습니다. 단어를 다 치기 전에 결과가 뜹니다. 스피너도 서버 왕복도 없습니다.',
      },
    },
    agent: {
      label: '에이전트와 함께 일하는 사람에게',
      heading: '사람과 에이전트가 같은 명령을 씁니다',
      body:
        'CLI가 그대로 에이전트 인터페이스입니다. 에이전트가 create, claim, transition을 실행하면 같은 보드가 눈앞에서 바뀝니다. 셸이 없는 클라이언트는 MCP 서버로 붙습니다. 읽기는 캐시에서, 쓰기는 Jira를 거쳐서. 에이전트가 쓴 것에는 에이전트 이름이 남아서, 팀이 읽는 스레드에 그대로 보입니다.',
      skillLead: '같은 캐시를 코딩 에이전트에게 넘기려면:',
      mcpLead: '셸이 없는 MCP 클라이언트(Claude Desktop)에는:',
      setupLink: '도구별로 붙여 넣을 설정 블록 → docs/AGENT_SETUP.md',
      driveCaption:
        '같은 창 안의 실제 Claude Code 세션입니다. 한국어 한 문장이 리스트가 되고, 다음 문장이 대시보드를 저장해 엽니다. 에이전트와 에이전트가 움직이는 보드가 한 창에 있습니다.',
      showcaseLink: '녹화본 더 보기: 대시보드, 팀 테마, 런처, 라이브 MCP 세션 → docs/SHOWCASE.md',
    },
    origin: {
      label: '안심하고 써도 되는 이유',
      heading: '캐시일 뿐입니다',
      points: [
        '쓰기는 먼저 Jira로 가고, Jira가 받아들인 뒤에야 캐시가 갱신됩니다.',
        '캐시는 이 컴퓨터 안의 SQLite 파일 하나입니다. 지워도 되고, 다시 동기화하면 그대로 다시 만들어집니다.',
        '텔레메트리는 없습니다. 밖으로 나가는 요청은 직접 설정한 것뿐입니다.',
        '자격 증명은 캐시에도, 로그에도, 스냅샷에도 남지 않습니다.',
      ],
    },
    ask: {
      label: '한 줄 남겨 주세요',
      heading: '써 보셨으면 한 줄만',
      body:
        '텔레메트리가 없어서 누가 쓰는지 저는 숫자로 모릅니다. 버그 제보와 UI 지적은 릴리스마다 덕을 봤습니다. 아직 없는 건 "이슈 N개 넣었더니 이렇게 됐다" 같은 한 줄입니다. 느려졌다거나 틀렸다는 쪽이면 더 좋습니다.',
      links: [
        { href: `${GITHUB}/issues`, label: 'GitHub 이슈로' },
        { href: 'https://x.com/midagedev', label: 'X @midagedev 멘션으로' },
      ],
    },
    changelog: {
      heading: '체인지로그',
      lede:
        '릴리스마다 직접 내보낸 사람이 자기 말로 씁니다. 이슈 키는 공개 백로그로 이어져서, ' +
        '여기 한 줄에서 그 일을 요청한 이슈까지 거슬러 읽을 수 있습니다.',
      source: '저장소의 CHANGELOG.ko.md를 그대로 렌더링합니다. 영문판이 원본입니다.',
      jumpLabel: '버전으로 이동',
      // Renders only on locales whose changelog falls back to the English
      // file (changelogIsFallback) — never here.
      fallbackNote: 'This changelog is published in English.',
    },
    install: {
      heading: '설치',
      macosApp: '데스크톱 앱, CLI 포함:',
      cliOnly: 'CLI만:',
      windowsBefore: 'Windows 데스크톱 앱은',
      windowsAfter: '에 있습니다.',
      firstRun: '회사 Jira에 연결합니다. 사이트, 이메일, 토큰, 프로젝트를 차례로 묻습니다:',
    },
    landing: {
      flagshipSlot: '플래그십 · 이슈 2만 건',
      searchSlot: '검색',
      agentSlot: '창 안의 에이전트',
      allPlatforms: '모든 플랫폼 →',
    },
    footer: {
      builtBy: '만든 사람',
      whereBytes: '데이터가 어디로 가는지',
    },
  },
  ja: {
    htmlLang: 'ja',
    ogLocale: 'ja_JP',
    title: 'gadak — 同じJira。待ち時間なし。',
    description:
      'JQLにGROUP BYはありません。gadakはJiraとConfluenceをまるごとキャッシュし、SQLで集計し、ミリ秒で全文検索します。コーディングエージェントにはMCPで渡せます。テレメトリはありません。',
    nav: { demo: 'ライブデモ', changelog: '変更履歴', install: 'インストール', github: 'GitHub' },
    copy: { label: 'コピー', copied: 'コピーしました' },
    ogImageAlt:
      'gadak — 同じJira。待ち時間なし。チームのJiraとConfluenceのWikiを、まるごとキャッシュします。',
    langName: '日本語',
    langBanner: {
      offer: 'このページは日本語でも読めます。',
      cta: '日本語で表示 →',
      dismiss: '閉じる',
    },
    // Qiita・Zenn の検索から来る読者: 問題 → 数字 → 導入前に確認したいこと
    // (この市場では購買条件) → エージェント → 検索 → インストール。
    layout: ['hero', 'speed', 'origin', 'agent', 'search', 'install'],
    hero: {
      eyebrow: 'gadak',
      heading: TAGLINE.ja.heading,
      lede:
        'JQLにGROUP BYはありません。「未完了の課題が多いエピックはどれか」を数えるには、APIを8ページめくって自分で集計することになります。実測で4,761 ms。gadakはJiraとConfluenceをまるごとキャッシュしておくので、同じ答えがSQL 1本、22 msで返ります。検索はミリ秒、読み取りはネットワークに出ません。書き込みはJiraへ通します。',
      videoCaption: '課題2万件のキャッシュ。打つ速さのまま検索が返ります。録画で、アニメーションではありません。',
      doors: {
        installTitle: 'インストール',
        installSub: 'macOSはHomebrew、WindowsはMicrosoft Store、LinuxはCLI。',
        demoTitle: 'ライブデモ',
        demoSub: 'ブラウザの中に534件の課題。インストールもアカウントも不要。',
      },
    },
    speed: {
      label: '速さは形容詞ではなく、測った数字',
      heading: '同じ質問を、2つの経路で',
      note: '2026-08-26に、本番のAtlassian Cloudサイト（実際の業務プロジェクト、課題3,296件）で計測。合成データではありません。gadakの数字にはCLIプロセスの起動時間を含みます。計測方法、再計測の履歴、そしてgadakが負ける場面を正直に並べた表はこちら:',
      rows: [
        { what: '単純なフィルタ、課題100件', value: '583 ms', alt: '19 ms', ratio: '31×' },
        { what: '課題1件 + 変更履歴すべて', value: '710 ms', alt: '28 ms', ratio: '25×' },
        { what: '全文検索', value: '543 ms', alt: '41 ms', ratio: '13×' },
        { what: 'エピック別の未完了（GROUP BY）', value: '4,761 ms — APIページ8回', alt: '22 ms — クエリ1回', ratio: '214×' },
        { what: '変更履歴を数える', value: '表現できない', alt: '14 ms', ratio: '—' },
        { what: 'レート制限', value: '429 + Retry-After', alt: 'なし', ratio: '—' },
      ],
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    ux: {
      label: '毎日のループ',
      search: {
        heading: '入力に追いつく検索',
        body:
          'タイトル、本文、コメント、Wikiまで、パレット1つで横断します。前方一致は単語を打ち終える前に返り、全文一致がそのすぐ後に続きます。スピナーも往復もありません。',
      },
    },
    agent: {
      label: 'エージェントと一緒に作る人へ',
      heading: 'あなたとエージェントの語彙を1つに',
      body:
        'CLIはそのままエージェントのインターフェースです。作成、担当、遷移。エージェントが実行する動詞を、あなたは同じボードで見ています。シェルのないクライアントにはMCPサーバーがあります。書き込みはJiraへ通し、読み取りはキャッシュから。そしてエージェントの書き込みには必ず名前が付きます。コメントも紐づけたPRも、チームが読むそのスレッドにボットの名前で残ります。',
      skillLead: '同じキャッシュをコーディングエージェントに渡す:',
      mcpLead: 'シェルのないMCPクライアント（Claude Desktop）には:',
      setupLink: 'ツールごとに貼るだけの設定ブロック → docs/AGENT_SETUP.md',
      driveCaption:
        '同じペインで動くClaude Codeのライブセッション。韓国語の一文がそのまま一覧になり、次の一文で保存してダッシュボードを開きます。エージェントと、それが動かすボードが、1つの窓の中に。',
      showcaseLink: 'ほかの録画 — ダッシュボード、チームのテーマ、ランチャー、MCPのライブセッション → docs/SHOWCASE.md',
    },
    origin: {
      label: '導入前に確認したいこと',
      heading: '外に出る通信は、自分で設定したものだけ',
      points: [
        'テレメトリはありません。gadakから出る通信は、自分のAtlassianサイトと、自分で有効にしたものだけです。一覧と切り方はSECURITY.mdにあります。',
        'APIトークンは ~/.gadak/config.json にモード0600で置かれ、自分のサイトへのAuthorizationヘッダーにしか使われません。ログにも、キャッシュにも、スナップショットにも入りません。',
        'キャッシュの実体は、この端末の中のSQLiteファイル1つです。いつ消しても構いません。もう一度同期すれば、そのまま作り直せます。',
        '書き込みは先にJiraへ通します。Jiraが受け付けてから、キャッシュが更新されます。届かなかった書き込みをキャッシュに溜めることはありません。',
        'キャッシュを読むエージェントは、読んだ内容をそのエージェントのモデルへ送ります。gadak自身は何も送りません。見せてよいプロジェクトとスペースだけをキャッシュしてください。',
        '対応しているのはJira Cloudです。Server / Data Centerは未検証なので、対応をうたっていません。',
        'アカウント登録もgadakのサーバーもありません。個人の端末1台で完結します。',
      ],
      link: { href: `${GITHUB}/blob/main/SECURITY.md`, label: 'SECURITY.md — 通信先の一覧と、それぞれの切り方 →' },
    },
    changelog: {
      heading: '変更履歴',
      lede:
        'すべてのリリースを、出荷した本人の言葉で。課題キーは公開バックログにリンクしているので、ここの一行から、それを求めた課題までさかのぼれます。',
      source: 'リポジトリのCHANGELOG.mdから描画しています。',
      jumpLabel: 'バージョンへ移動',
      // Not a placeholder like the rest: this one already renders (the ja
      // page reads the English changelog), so the sentence is real copy.
      fallbackNote: 'この変更履歴は英語で公開しています。',
    },
    install: {
      heading: 'インストール',
      macosApp: 'デスクトップアプリ（CLI同梱）:',
      cliOnly: 'CLIのみ:',
      windowsBefore: 'Windowsでは、デスクトップアプリは',
      windowsAfter: 'にあります。',
      firstRun: 'チームのJiraに接続します（サイト、メール、トークン、プロジェクトを聞かれます）:',
    },
    landing: {
      flagshipSlot: 'フラッグシップ · 課題2万件',
      searchSlot: '検索',
      agentSlot: '窓の中のエージェント',
      allPlatforms: 'すべてのプラットフォーム →',
    },
    footer: {
      builtBy: '作者:',
      whereBytes: 'バイトの行き先',
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
