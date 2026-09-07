// The site's locales in one place: en at the root, the rest under their own
// prefix (pathFor below). Every locale-aware surface — layout, sitemap,
// switcher, banners — iterates this list, so a fourth locale is a list entry
// plus copy, not another ternary.
export const LOCALES = ['en', 'ko', 'ja'] as const
export type Locale = (typeof LOCALES)[number]

export const strings = {
  en: {
    htmlLang: 'en',
    ogLocale: 'en_US',
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
      firstRun: '회사 Jira에 연결합니다 (사이트, 이메일, 토큰, 프로젝트를 차례로 묻습니다):',
    },
    landing: {
      flagshipSlot: '플래그십 · 2만 건 미러',
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
      '会社で使っているそのJira（課題とConfluenceのWiki）を、ローカルのSQLiteファイル1つにミラーします。2万件の課題でも検索はミリ秒で返り、読み取りはネットワークに触れません。',
    nav: { demo: 'ライブデモ', changelog: '変更履歴', essays: 'エッセイ', install: 'インストール', github: 'GitHub' },
    copy: { label: 'コピー', copied: 'コピーしました' },
    ogImageAlt:
      'gadak — 同じJira。待ち時間なし。チームのJiraとConfluenceのWikiを、ローカルのSQLiteファイル1つにミラー。',
    langName: '日本語',
    langBanner: {
      offer: 'このページは日本語でも読めます。',
      cta: '日本語で表示 →',
      dismiss: '閉じる',
    },
    hero: {
      eyebrow: 'gadak',
      heading: '同じJira。待ち時間なし。',
      lede:
        'チームのJiraとそのConfluence Wikiを、この端末のSQLiteファイル1つにミラーします。検索はミリ秒で返り、履歴は文書のように読め、ページが回り続けることはありません。正本はJiraのまま。ただ、待たなくなるだけです。',
      videoCaption: '2万件の課題のミラー。打つ速さのまま検索が返ります。録画です。アニメーションではありません。',
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
        { what: '単純なフィルタ、課題100件', value: '583 ms', alt: '19 ms', ratio: '31×' },
        { what: '課題1件 + 変更履歴すべて', value: '710 ms', alt: '28 ms', ratio: '25×' },
        { what: '全文検索', value: '543 ms', alt: '41 ms', ratio: '13×' },
        { what: 'エピック別の未完了（GROUP BY）', value: '4,761 ms — APIページ8回', alt: '22 ms — クエリ1回', ratio: '214×' },
        { what: '変更履歴を数える', value: '表現できない', alt: '14 ms', ratio: '—' },
        { what: 'レート制限', value: '429 + Retry-After', alt: 'なし — 自分のディスク', ratio: '—' },
      ],
      colRest: 'Jira REST API',
      colGadak: 'gadak',
    },
    ux: {
      label: '毎日のループ',
      search: {
        heading: '入力に追いつく検索',
        body:
          'タイトル、本文、コメント、Wikiまで、パレット1つで横断します。前方一致は単語を打ち終える前にローカルで返り、全文一致がそのすぐ後に続きます。スピナーも往復もありません。',
      },
    },
    agent: {
      label: 'エージェントと一緒に作る人へ',
      heading: 'あなたとエージェントの語彙を1つに',
      body:
        'CLIはそのままエージェントのインターフェースです。作成、担当、遷移。エージェントが実行する動詞を、あなたは同じボードで見ています。シェルのないクライアントにはMCPサーバーがあります。書き込みは正本へ通し、読み取りはローカルのミラーから。そしてエージェントの書き込みには必ず名前が付きます。コメントも紐づけたPRも、チームが読むそのスレッドにボットの名前で残ります。',
      skillLead: '同じミラーをコーディングエージェントに渡す:',
      mcpLead: 'シェルのないMCPクライアント（Claude Desktop）には:',
      setupLink: 'ツールごとに貼るだけの設定ブロック → docs/AGENT_SETUP.md',
      driveCaption:
        '同じペインで動くClaude Codeのライブセッション。韓国語の一文がそのまま一覧になり、次の一文で保存してダッシュボードを開きます。エージェントと、それが動かすボードが、1つの窓の中に。',
      showcaseLink: 'ほかの録画 — ダッシュボード、チームのテーマ、ランチャー、MCPのライブセッション → docs/SHOWCASE.md',
    },
    origin: {
      label: '安心して試せる理由',
      heading: '正本はJiraのまま',
      points: [
        '書き込みは先にJiraへ通します。正本が受け付けてから、ミラーが更新されます。',
        'ミラーは捨てられます。削除して再同期すれば、正本から作り直せます。',
        'テレメトリはありません。ネットワークに出るのは、あなたが設定した通信だけです。',
        '認証情報はSQLiteにも、ログにも、スナップショットにも入りません。',
      ],
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
      flagshipSlot: 'フラッグシップ · 2万件のミラー',
      searchSlot: '検索',
      agentSlot: '窓の中のエージェント',
      allPlatforms: 'すべてのプラットフォーム →',
    },
    footer: {
      builtBy: '作者:',
      whereBytes: 'バイトの行き先',
    },
  },
} satisfies Record<Locale, Record<string, unknown>>

export type Strings = (typeof strings)['en']

/** '' for the default locale, '/ko' / '/ja' for the prefixed ones. */
export function localePrefix(l: Locale): string {
  return l === 'en' ? '' : `/${l}`
}

/** The same page in another locale: pathFor('ko', '/') is '/ko/', pathFor('ko', '/install/') is '/ko/install/', pathFor('en', x) is x. */
export function pathFor(l: Locale, enPath: string): string {
  if (l === 'en') return enPath
  return `${localePrefix(l)}${enPath === '/' ? '/' : enPath}`
}

// Matches a locale prefix only as a full first segment, so /essays/ or a
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
