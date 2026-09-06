/*
 * Sidebar, shortcuts, browse, freshness, builtin views, app shell.
 * One key = {en, ko, ja}; omitting a locale is a type error.
 */
import type { Message } from '../types'

export const shell = {
  /* ── Sidebar ── */
  'sidebar.newIssueTitle': {
    en: 'New issue (c)',
    ko: '새 이슈 (c)',
    ja: '新しい課題 (c)',
  },
  'sidebar.builtinViews': {
    en: 'Views',
    ko: '뷰',
    ja: 'ビュー',
  },
  'sidebar.myViews': {
    en: 'Saved views',
    ko: '저장한 뷰',
    ja: '保存したビュー',
  },
  'sidebar.jiraFilters': {
    en: 'Jira filters',
    ko: 'Jira 필터',
    ja: 'Jira フィルター',
  },
  'sidebar.openFilterInJira': {
    en: 'Open this filter in Jira',
    ko: '이 필터를 Jira에서 열기',
    ja: 'このフィルターを Jira で開く',
  },
  'sidebar.docsNoneTitle': {
    en: 'No documents mirrored',
    ko: '미러링된 문서 없음',
    ja: 'ミラーされたドキュメントはありません',
  },
  'sidebar.docsNoneHint': {
    en: 'Turn on Confluence in Settings → Sources.',
    ko: '설정 → 소스에서 Confluence를 켜세요.',
    ja: '設定 → ソースで Confluence をオンにしてください。',
  },
  'sidebar.docsUnavailable': {
    en: 'This snapshot carries issues only.',
    ko: '이 스냅샷에는 이슈만 들어 있습니다.',
    ja: 'このスナップショットには課題だけが入っています。',
  },
  // Confluence is on and the mirror still holds no pages. Four different
  // reasons, four different sentences: the CTA above belongs to "off" alone.
  'sidebar.docsSyncing': {
    en: 'Fetching documents…',
    ko: '문서를 가져오는 중…',
    ja: 'ドキュメントを取得中…',
  },
  'sidebar.docsNotFetched': {
    en: 'Documents not fetched yet',
    ko: '아직 문서를 가져오지 않음',
    ja: 'まだドキュメントを取得していません',
  },
  'sidebar.docsNotFetchedHint': {
    en: 'Sync now to mirror the spaces you chose.',
    ko: '선택한 스페이스를 지금 동기화하세요.',
    ja: '選んだスペースを今すぐ同期してください。',
  },
  'sidebar.docsFetchFailed': {
    en: 'Could not fetch documents',
    ko: '문서를 가져오지 못했습니다',
    ja: 'ドキュメントを取得できませんでした',
  },
  // GDK-831: same recovery as the locked need-credentials family, so the same
  // noun — credentials/자격증명/資格情報, never the secret's wire name.
  'sidebar.docsFetchFailedHint': {
    en: 'Check the spaces and your credentials in Sources.',
    ko: '소스 설정에서 스페이스와 자격증명을 확인하세요.',
    ja: 'ソースでスペースと資格情報を確認してください。',
  },
  'sidebar.docsLocalEmpty': {
    en: 'No documents yet',
    ko: '아직 문서가 없습니다',
    ja: 'ドキュメントはまだありません',
  },
  'sidebar.docsEmptySpaces': {
    en: 'No documents in these spaces',
    ko: '이 스페이스에 문서가 없습니다',
    ja: 'これらのスペースにドキュメントはありません',
  },
  'sidebar.docsEmptySpacesHint': {
    en: 'Change the selection in Settings → Sources.',
    ko: '설정 → 소스에서 선택을 바꾸세요.',
    ja: '設定 → ソースで選択を変えてください。',
  },
  'sidebar.docs': {
    en: 'Documents',
    ko: '문서',
    ja: 'ドキュメント',
  },
  'sidebar.dashboards': {
    en: 'Dashboards',
    ko: '대시보드',
    ja: 'ダッシュボード',
  },
  'sidebar.terminal': {
    en: 'Terminal',
    ko: '터미널',
    ja: 'ターミナル',
  },
  'terminal.title': {
    en: 'Terminal',
    ko: '터미널',
    ja: 'ターミナル',
  },
  'terminal.reconnecting': {
    en: 'Reconnecting…',
    ko: '다시 연결 중…',
    ja: '再接続中…',
  },
  'terminal.exited': {
    en: 'Shell exited (code {code})',
    ko: '셸이 종료됨 (코드 {code})',
    ja: 'シェルが終了しました（コード {code}）',
  },
  'terminal.dropped.slow_client': {
    en: 'Disconnected: slow client',
    ko: '느린 연결로 끊겼습니다',
    ja: '通信が遅く切断されました',
  },
  'terminal.dropped.token_revoked': {
    en: 'Pairing token revoked',
    ko: '페어링 토큰이 폐기되었습니다',
    ja: 'ペアリングトークンが失効しました',
  },
  'terminal.dropped.idle_timeout': {
    en: 'Session ended (idle timeout)',
    ko: '유휴 시간이 지나 세션이 종료되었습니다',
    ja: 'アイドルのためセッションが終了しました',
  },
  'terminal.dropped.server_shutdown': {
    en: 'Server shut down',
    ko: '서버가 종료되었습니다',
    ja: 'サーバーが終了しました',
  },
  'terminal.dropped.closed': {
    en: 'Session closed',
    ko: '세션이 닫혔습니다',
    ja: 'セッションが閉じられました',
  },
  'terminal.unavailable.unsupported': {
    en: 'This machine cannot run a shell. Windows has no PTY, and that is permanent.',
    ko: '이 기기에서는 셸을 쓸 수 없습니다. Windows에는 PTY가 없으며, 이는 바뀌지 않습니다.',
    ja: 'このマシンではシェルを実行できません。Windows に PTY はなく、これは変わりません。',
  },
  'terminal.unavailable.forbidden': {
    en: 'This device is not allowed to open a shell here. Mint a terminal-scope token with `gadak pairing mint --label NAME --scope terminal`.',
    ko: '이 기기에서는 셸을 열 수 없습니다. `gadak pairing mint --label NAME --scope terminal`로 터미널 스코프 토큰을 만드세요.',
    ja: 'このデバイスではシェルを開けません。`gadak pairing mint --label NAME --scope terminal` でターミナルスコープのトークンを発行してください。',
  },
  'terminal.unavailable.failed': {
    en: 'The host could not start a shell: {message}',
    ko: '호스트가 셸을 시작하지 못했습니다: {message}',
    ja: 'ホストがシェルを起動できませんでした: {message}',
  },
  'terminal.unavailable.network': {
    en: 'The shell connection did not open.',
    ko: '셸 연결이 열리지 않았습니다.',
    ja: 'シェル接続が開きませんでした。',
  },
  'terminal.restartHint': {
    en: 'Press Enter to start a new shell',
    ko: 'Enter 키로 새 셸을 시작합니다',
    ja: 'Enter で新しいシェルを起動します',
  },
  'terminal.mintHint': {
    en: 'Mint a new token with `gadak pairing mint --label NAME --scope terminal`',
    ko: '`gadak pairing mint --label NAME --scope terminal`로 새 토큰을 만드세요',
    ja: '`gadak pairing mint --label NAME --scope terminal` で新しいトークンを発行してください',
  },
  'terminal.close': {
    en: 'Close the terminal',
    ko: '터미널 닫기',
    ja: 'ターミナルを閉じる',
  },
  // The resize grip has its own name so a screen reader does not read it as a
  // second "Terminal" — the region already carries terminal.title (GDK-948).
  'terminal.resize': {
    en: 'Resize the terminal',
    ko: '터미널 크기 조절',
    ja: 'ターミナルのサイズを変更',
  },
  'terminal.shortcut': {
    en: 'Ctrl+`',
    ko: 'Ctrl+`',
    ja: 'Ctrl+`',
  },
  // The session strip (GDK-1153 / GDK-1163): one row per live shell, named
  // after the issue it was claimed for. The four states are the terminal's
  // own signals — output, a BEL, an attachment, what is on the tty — not a
  // status field anyone types.
  'terminal.strip.list': {
    en: 'Terminal sessions',
    ko: '터미널 세션',
    ja: 'ターミナルセッション',
  },
  'terminal.strip.show': {
    en: 'Show {name}',
    ko: '{name} 보기',
    ja: '{name} を表示',
  },
  'terminal.strip.state.needs': {
    en: 'Wants you',
    ko: '사람 필요',
    ja: '要対応',
  },
  'terminal.strip.state.running': {
    en: 'Running',
    ko: '출력 중',
    ja: '実行中',
  },
  'terminal.strip.state.quiet': {
    en: 'Quiet',
    ko: '조용',
    ja: '静か',
  },
  'terminal.strip.state.ghost': {
    en: 'Unwatched',
    ko: '보는 사람 없음',
    ja: '見ていない',
  },
  'terminal.strip.start': {
    en: 'Start a shell here',
    ko: '여기서 셸을 엽니다',
    ja: 'ここでシェルを開く',
  },
  'terminal.strip.new': {
    en: 'New shell',
    ko: '새 셸',
    ja: '新しいシェル',
  },
  // A readable default for an unnamed, unclaimed shell (GDK-1387): the
  // server sends the creation ordinal, the client says it in words.
  'terminal.strip.defaultName': {
    en: 'shell {n}',
    ko: '셸 {n}',
    ja: 'シェル {n}',
  },
  // Rename (GDK-1195): double-click or F2 on a row; Enter keeps, Esc drops,
  // an empty name returns the row to its issue key or default.
  'terminal.strip.rename': {
    en: 'Rename {name}',
    ko: '{name} 이름 바꾸기',
    ja: '{name} の名前を変更',
  },
  'terminal.strip.renameHint': {
    en: 'Enter keeps · Esc cancels · empty clears',
    ko: 'Enter 확정 · Esc 취소 · 비우면 기본 이름',
    ja: 'Enter で確定 · Esc で取消 · 空で既定名',
  },
  // The tab's × (GDK-1200): ends the session itself, as opposed to
  // terminal.close, which only closes the dock and leaves every shell alive.
  'terminal.strip.kill': {
    en: 'Close session {name}',
    ko: '{name} 세션 종료',
    ja: 'セッション {name} を終了',
  },
  // The dashboard surface (GDK-782). notFound: the id arrived (link, uifocus)
  // but the row is gone — `gadak dashboards rm` or another workspace. Both
  // states name the next action (GDK-827): a dead end that only states
  // itself leaves the person on a screen whose only working control is the
  // one they have to hunt for.
  'dash.notFound': {
    en: 'This dashboard no longer exists. Pick another one in the sidebar, or list them with `gadak dashboards list`.',
    ko: '이 대시보드는 더 이상 없습니다. 사이드바에서 다른 대시보드를 고르거나 `gadak dashboards list`로 목록을 확인하세요.',
    ja: 'このダッシュボードはもう存在しません。サイドバーで別のものを選ぶか、`gadak dashboards list` で一覧を確認してください。',
  },
  'dash.loadError': {
    en: 'Could not load this dashboard. Press Esc or the back arrow to return to the list, then open it again.',
    ko: '이 대시보드를 불러올 수 없습니다. Esc 또는 뒤로 화살표로 목록으로 돌아간 뒤 다시 열어보세요.',
    ja: 'このダッシュボードを読み込めませんでした。Esc または戻る矢印で一覧に戻り、もう一度開いてください。',
  },
  'sidebar.docsSpaceTitle': {
    en: '{space} · {n} documents',
    ko: '{space} · 문서 {n}건',
    ja: '{space} · ドキュメント {n}件',
  },
  'sidebar.docsAll': {
    en: 'Documents',
    ko: '문서',
    ja: 'ドキュメント',
  },
  'sidebar.docsAllTitle': {
    en: 'Documents you viewed, what changed, and who wrote it',
    ko: '내가 본 문서, 최근 갱신, 작성자별로',
    ja: '閲覧したドキュメント、更新、作成者',
  },
  'sidebar.docsSpaces': {
    en: 'Spaces',
    ko: '스페이스',
    ja: 'スペース',
  },
  'sidebar.docsSpacesTitle': {
    en: 'Browse one space at a time',
    ko: '스페이스 하나씩 살펴보기',
    ja: 'スペースをひとつずつ見る',
  },
  'sidebar.docsToggleNode': {
    en: 'Documents under {title}',
    ko: '{title} 하위 문서',
    ja: '{title} 配下のドキュメント',
  },
  'sidebar.workspaceSwitch': {
    en: 'Switch workspace',
    ko: '워크스페이스 전환',
    ja: 'ワークスペースを切り替え',
  },
  'sidebar.workspaceNew': {
    en: 'New workspace',
    ko: '새 워크스페이스',
    ja: '新しいワークスペース',
  },
  'sidebar.workspaceUnreadable': {
    en: 'Workspace config could not be read',
    ko: '워크스페이스 설정을 읽을 수 없음',
    ja: 'ワークスペース設定を読めませんでした',
  },
  'sidebar.settings': {
    en: 'Settings',
    ko: '설정',
    ja: '設定',
  },
  'sidebar.serverSettings': {
    en: 'Server settings (projects, features, teams, fields)',
    ko: '서버 설정 (프로젝트·기능·팀·필드)',
    ja: 'サーバー設定（プロジェクト、機能、チーム、フィールド）',
  },
  'sidebar.jiraCreds': {
    en: 'Jira credentials',
    ko: 'Jira 자격증명 설정',
    ja: 'Jira 資格情報',
  },
  'sidebar.jiraCredsMissing': {
    en: 'Jira credentials not set — configure to write',
    ko: 'Jira 자격증명 미설정 — 쓰기하려면 설정하세요',
    ja: 'Jira 資格情報が未設定です — 書き込むには設定してください',
  },
  'sidebar.viewDeleteFail': {
    en: 'Could not delete view. Try again.',
    ko: '뷰를 삭제하지 못했습니다. 다시 시도하세요.',
    ja: 'ビューを削除できませんでした。再試行してください。',
  },
  'sidebar.syncOk': {
    en: 'OK',
    ko: '정상',
    ja: '正常',
  },
  'sidebar.syncing': {
    en: 'Syncing',
    ko: '동기화 중',
    ja: '同期中',
  },
  'sidebar.syncOffHours': {
    en: 'Waiting (outside business hours)',
    ko: '업무시간 외 대기',
    ja: '待機（営業時間外）',
  },
  'sidebar.syncWaiting': {
    en: 'Waiting',
    ko: '대기',
    ja: '待機',
  },
  'sidebar.syncDelayed': {
    en: 'Delayed',
    ko: '지연',
    ja: '遅延',
  },
  'sidebar.syncFailed': {
    en: 'Failed',
    ko: '실패',
    ja: '失敗',
  },
  'sidebar.syncNoRecord': {
    en: 'No record',
    ko: '기록 없음',
    ja: '記録なし',
  },
  'sidebar.syncFailTitle': {
    en: 'Sync failed',
    ko: '동기화 실패',
    ja: '同期失敗',
  },
  'sidebar.syncDelayedTitle': {
    en: 'Sync delayed',
    ko: '동기화 지연',
    ja: '同期遅延',
  },
  'sidebar.updateAvailable': {
    en: 'gadak {version} is out — release notes',
    ko: 'gadak {version} 업데이트가 나왔습니다 — 릴리스 노트 보기',
    ja: 'gadak {version} が公開されています — リリースノート',
  },
  'sidebar.syncNow': {
    en: 'Sync now',
    ko: '지금 동기화',
    ja: '今すぐ同期',
  },
  'sidebar.syncHistory': {
    en: 'Sync history',
    ko: '동기화 내역',
    ja: '同期履歴',
  },
  'sidebar.syncHistoryTitle': {
    en: 'Click for recent sync runs',
    ko: '클릭하면 최근 동기화 내역을 보여줍니다',
    ja: 'クリックすると最近の同期を表示します',
  },
  'sidebar.syncNoHistory': {
    en: 'No recorded runs yet — history keeps syncs that changed something.',
    ko: '기록된 내역이 없습니다 — 변경이 있었던 동기화만 남습니다.',
    ja: '記録された実行はまだありません — 何かが変わった同期だけが残ります。',
  },
  'sidebar.syncLastChecked': {
    en: 'Last checked {when}',
    ko: '마지막 확인 {when}',
    ja: '最終確認 {when}',
  },
  'sidebar.serverUnreachable': {
    en: 'Could not reach the server. Retry.',
    ko: '서버에 연결하지 못했습니다. 다시 시도하세요.',
    ja: 'サーバーに到達できませんでした。再試行してください。',
  },
  'sidebar.runFull': {
    en: 'Full sync',
    ko: '전체 동기화',
    ja: '全同期',
  },
  'sidebar.runIncremental': {
    en: 'Incremental',
    ko: '증분',
    ja: '増分',
  },
  'sidebar.runReconcile': {
    en: '+ deletions',
    ko: '+ 삭제 반영',
    ja: '+ 削除反映',
  },
  'sidebar.runCounts': {
    en: '{changed} changed · {deleted} deleted',
    ko: '{changed}건 변경 · {deleted}건 삭제',
    ja: '{changed}件変更 · {deleted}件削除',
  },
  'sidebar.issueCount': {
    en: '{n} issues',
    ko: '{n}건',
    ja: '{n}件',
  },
  'sidebar.sectionReorderHint': {
    en: 'Drag or Alt+↑↓ to reorder',
    ko: '드래그 또는 Alt+↑↓로 순서 변경',
    ja: 'ドラッグまたは Alt+↑↓ で並べ替え',
  },
  /* ── Freshness chip (mirror ↔ Jira leg) ── */
  'freshness.label': {
    en: 'Mirror freshness',
    ko: '미러 신선도',
    ja: 'ミラーの鮮度',
  },
  'freshness.titleFresh': {
    en: 'Mirror pulled from {tracker} {when}. Click to sync now.',
    ko: '{tracker}에서 미러를 {when} 가져왔습니다. 클릭하면 지금 동기화합니다.',
    ja: '{tracker} からミラーを {when} 取得しました。クリックで今すぐ同期します。',
  },
  'freshness.titleFreshLocal': {
    en: 'Mirror refreshed from the built-in tracker {when}. Click to sync now.',
    ko: '내장 트래커에서 미러를 {when} 갱신했습니다. 클릭하면 지금 동기화합니다.',
    ja: '内蔵トラッカーからミラーを {when} 更新しました。クリックで今すぐ同期します。',
  },
  'freshness.titleStale': {
    en: 'Mirror is behind — last successful sync {when}. Click to sync now.',
    ko: '미러가 지연됐습니다 — 마지막 성공 동기화 {when}. 클릭하면 지금 동기화합니다.',
    ja: 'ミラーが遅れています — 最後の成功同期は {when}。クリックで今すぐ同期します。',
  },
  'freshness.titleFailed': {
    en: 'Last sync failed: {message}\nClick to retry.',
    ko: '마지막 동기화 실패: {message}\n클릭하면 재시도합니다.',
    ja: '最後の同期に失敗しました: {message}\nクリックで再試行します。',
  },
  'freshness.titleNever': {
    en: 'The mirror has never synced. Click to sync now.',
    ko: '미러가 아직 한 번도 동기화되지 않았습니다. 클릭하면 지금 동기화합니다.',
    ja: 'ミラーはまだ一度も同期していません。クリックで今すぐ同期します。',
  },
  'freshness.tokenExpiring': {
    en: 'Token expires in {n} days',
    ko: '토큰이 {n}일 후 만료',
    ja: 'トークンは {n} 日後に期限切れ',
  },
  'freshness.tokenExpiringOne': {
    en: 'Token expires in 1 day',
    ko: '토큰이 1일 후 만료',
    ja: 'トークンは 1 日後に期限切れ',
  },
  'freshness.tokenExpiringToday': {
    en: 'Token expires today',
    ko: '토큰이 오늘 만료',
    ja: 'トークンは今日期限切れ',
  },
  'freshness.tokenExpired': {
    en: 'API token expired',
    ko: 'API 토큰 만료됨',
    ja: 'API token の期限切れ',
  },
  /* ── Builtin views ── */
  'view.myWork.name': {
    en: 'My issues',
    ko: '내 이슈',
    ja: '自分の課題',
  },
  'view.myWork.hint': {
    en: 'Open issues assigned to this account, most urgent first; the stale mark carries the age',
    ko: '이 계정에 배정된 미해결 이슈. 급한 것부터, 나이는 지연 표시가 말한다',
    ja: 'このアカウントに割り当てられた未解決課題。緊急度順、経過日数は滞留マークが示す',
  },
  'view.delegated.name': {
    en: 'Handed off',
    ko: '내가 맡긴 것',
    ja: '任せた課題',
  },
  'view.delegated.hint': {
    en: 'Open issues this account reported and someone else holds, quietest first',
    ko: '이 계정이 만들고 다른 사람이 들고 있는 미해결 이슈. 오래 조용한 것부터',
    ja: 'このアカウントが作成し他の人が担当する未解決課題。長く動きのないものから',
  },
  'sidebar.stanceMine': {
    en: 'My work',
    ko: '내 일',
    ja: '自分の仕事',
  },
  'sidebar.stanceTeam': {
    en: 'Team flow',
    ko: '팀의 흐름',
    ja: 'チームの流れ',
  },
  'view.allOpen.name': {
    en: 'All open',
    ko: '전체 미해결',
    ja: '未解決すべて',
  },
  'view.allOpen.hint': {
    en: 'New + in progress',
    ko: '신규 + 진행 중',
    ja: '未着手 + 進行中',
  },
  'view.unassignedNew.name': {
    en: 'Unassigned new',
    ko: '미할당 신규',
    ja: '未割り当ての未着手',
  },
  'view.unassignedNew.hint': {
    en: 'New with no assignee',
    ko: '담당자 없는 신규',
    ja: '担当者のいない未着手',
  },
  'view.reopened.name': {
    en: 'Reopened',
    ko: '재오픈 이슈',
    ja: '再オープン',
  },
  'view.reopened.hint': {
    en: 'Reopened after done',
    ko: '완료 후 다시 열린 이슈',
    ja: '完了後に再び開いた課題',
  },
  'view.epicBreakdown.name': {
    en: 'Epics',
    ko: '에픽별 보기',
    ja: 'エピック',
  },
  'view.epicBreakdown.hint': {
    en: 'Open issues grouped by epic',
    ko: '진행 중 이슈를 에픽으로 묶어 보기',
    ja: '未解決の課題をエピックでグループ化',
  },
  'view.stale.name': {
    en: 'Stale',
    ko: '정체 이슈',
    ja: '滞留',
  },
  'view.stale.hint': {
    en: 'Stuck in one status too long',
    ko: '한 상태에 오래 머문 이슈',
    ja: '同じステータスに長く留まっている',
  },
  'view.agingInProgress.name': {
    en: 'Aging in progress',
    ko: '오래된 진행 중',
    ja: '長く進行中',
  },
  'view.agingInProgress.hint': {
    // The my-work pack moved this view from the updated-at proxy to the real
    // status_changed axis — the hint follows the sort it names (2026-09-06).
    en: 'In progress · longest in status first',
    ko: '진행 중 · 한 상태에 가장 오래 머문 순',
    ja: '進行中 · 同じステータスに最も長く留まっている順',
  },
  'view.recentlyUpdated.name': {
    en: 'Recently updated',
    ko: '최근 갱신',
    ja: '最近の更新',
  },
  'view.recentlyUpdated.hint': {
    en: 'Open · newest updates',
    ko: '미해결 · 갱신 최신순',
    ja: '未解決 · 最新の更新',
  },
  'view.resolvedWeek.name': {
    en: 'Resolved this week',
    ko: '이번 주 해결됨',
    ja: '今週解決',
  },
  'view.resolvedWeek.hint': {
    en: 'Resolved since Monday',
    ko: '월요일 이후 해결',
    ja: '月曜以降に解決',
  },
  'view.unnamed': {
    en: 'Untitled view',
    ko: '이름 없는 뷰',
    ja: '無題のビュー',
  },
  /* ── my-work pack chips + sort label. These would sit beside
   * filter.flagStale / sort.* in messages/list.ts, but that file belongs to
   * a parallel round this commit must not touch — so they live here, in the
   * same terse register as their siblings ('Stale' / '정체' / '滞留'). ── */
  'filter.flagMine': {
    en: 'Mine',
    ko: '내 이슈',
    ja: '自分の課題',
  },
  'filter.flagDelegated': {
    en: 'Handed off',
    ko: '내가 맡긴 것',
    ja: '任せた',
  },
  'sort.statusChanged': {
    en: 'Time in status',
    ko: '상태 경과순',
    ja: 'ステータス経過順',
  },

  /* ── Keyboard shortcuts cheat sheet (?) ── */
  'shortcuts.title': {
    en: 'Keyboard shortcuts',
    ko: '키보드 단축키',
    ja: 'キーボードショートカット',
  },
  'shortcuts.sectionGlobal': {
    en: 'Global',
    ko: '전역',
    ja: 'グローバル',
  },
  'shortcuts.sectionList': {
    en: 'Issue list',
    ko: '이슈 목록',
    ja: '課題一覧',
  },
  'shortcuts.sectionColumnViews': {
    en: 'Documents, history, feed',
    ko: '문서·히스토리·피드',
    ja: 'ドキュメント・履歴・フィード',
  },
  'shortcuts.sectionDetail': {
    en: 'Detail panel',
    ko: '상세 패널',
    ja: '詳細パネル',
  },
  'shortcuts.sectionSearch': {
    en: 'Search',
    ko: '검색',
    ja: '検索',
  },
  'shortcuts.sectionPalette': {
    en: 'Command palette',
    ko: '커맨드 팔레트',
    ja: 'コマンドパレット',
  },
  'shortcuts.sectionCompose': {
    en: 'Composing',
    ko: '작성',
    ja: '作成中',
  },
  'shortcuts.palette': {
    en: 'Open the command palette',
    ko: '커맨드 팔레트 열기',
    ja: 'コマンドパレットを開く',
  },
  'shortcuts.settings': {
    en: 'Open settings',
    ko: '설정 열기',
    ja: '設定を開く',
  },
  'shortcuts.newIssueContext': {
    en: 'New issue (when no detail or cursor)',
    ko: '새 이슈 (상세·커서가 없을 때)',
    ja: '新しい課題（詳細もカーソルもないとき）',
  },
  'shortcuts.help': {
    en: 'Show this cheat sheet',
    ko: '이 치트시트 보기',
    ja: 'このチートシートを表示',
  },
  'shortcuts.terminal': {
    en: 'Toggle the terminal',
    ko: '터미널 열기/닫기',
    ja: 'ターミナルを開閉',
  },
  'shortcuts.terminalPrevSession': {
    en: 'Previous terminal session',
    ko: '이전 터미널 세션으로',
    ja: '前のターミナルセッションへ',
  },
  'shortcuts.terminalNextSession': {
    en: 'Next terminal session',
    ko: '다음 터미널 세션으로',
    ja: '次のターミナルセッションへ',
  },
  'shortcuts.terminalFocusTabs': {
    en: 'Move focus to the terminal tabs',
    ko: '터미널 탭으로 포커스 이동',
    ja: 'ターミナルのタブへフォーカスを移す',
  },
  'shortcuts.terminalOpenIssue': {
    en: "Open the session's issue",
    ko: '세션의 이슈 열기',
    ja: 'セッションの課題を開く',
  },
  'shortcuts.moveDown': {
    en: 'Move cursor down',
    ko: '커서 아래로',
    ja: 'カーソルを下へ',
  },
  'shortcuts.moveUp': {
    en: 'Move cursor up',
    ko: '커서 위로',
    ja: 'カーソルを上へ',
  },
  'shortcuts.openIssue': {
    en: 'Open the issue under the cursor',
    ko: '커서의 이슈 열기',
    ja: 'カーソル下の課題を開く',
  },
  'shortcuts.selectRow': {
    en: 'Select the row under the cursor',
    ko: '커서의 행 선택',
    ja: 'カーソル下の行を選択',
  },
  'shortcuts.listStatus': {
    en: 'Change status (selection, or the cursor row)',
    ko: '상태 변경 (선택 항목 또는 커서 행)',
    ja: 'ステータスを変更（選択、またはカーソル行）',
  },
  'shortcuts.listPriority': {
    en: 'Change priority (selection, or the cursor row)',
    ko: '우선순위 변경 (선택 항목 또는 커서 행)',
    ja: '優先度を変更（選択、またはカーソル行）',
  },
  'shortcuts.listAssignee': {
    en: 'Change assignee (selection, or the cursor row)',
    ko: '담당자 변경 (선택 항목 또는 커서 행)',
    ja: '担当者を変更（選択、またはカーソル行）',
  },
  'shortcuts.listLabels': {
    en: 'Change labels (selection, or the cursor row)',
    ko: '라벨 변경 (선택 항목 또는 커서 행)',
    ja: 'ラベルを変更（選択、またはカーソル行）',
  },
  'shortcuts.listComment': {
    en: 'Comment on the row under the cursor',
    ko: '커서의 이슈에 코멘트',
    ja: 'カーソル下の行にコメント',
  },
  'shortcuts.clearSelection': {
    en: 'Clear the selection, then close the detail panel',
    ko: '선택 해제 후 상세 패널 닫기',
    ja: '選択をクリアし、詳細パネルを閉じる',
  },
  'shortcuts.tabMoveRows': {
    en: 'Move through rows in documents, history, and feed',
    ko: '문서·히스토리·피드 행 이동',
    ja: 'ドキュメント・履歴・フィードの行を移動',
  },
  'shortcuts.closeColumnView': {
    en: 'Close documents, history, feed, or dashboard',
    ko: '문서·히스토리·피드·대시보드 닫기',
    ja: 'ドキュメント・履歴・フィード・ダッシュボードを閉じる',
  },
  // {tracker}: the origin's brand name (Jira/Linear), see detail.openJira.
  'shortcuts.detailOpenJira': {
    en: 'Open the issue in {tracker}',
    ko: '이슈를 {tracker}에서 열기',
    ja: '課題を {tracker} で開く',
  },
  'shortcuts.focusStatus': {
    en: 'Change status (when detail is open)',
    ko: '상태 변경 (상세 열림 시)',
    ja: 'ステータスを変更（詳細が開いているとき）',
  },
  'shortcuts.focusPriority': {
    en: 'Change priority (when detail is open)',
    ko: '우선순위 변경 (상세 열림 시)',
    ja: '優先度を変更（詳細が開いているとき）',
  },
  'shortcuts.focusAssignee': {
    en: 'Change assignee (when detail is open)',
    ko: '담당자 변경 (상세 열림 시)',
    ja: '担当者を変更（詳細が開いているとき）',
  },
  'shortcuts.focusLabels': {
    en: 'Add a label (when detail is open)',
    ko: '라벨 추가 (상세 열림 시)',
    ja: 'ラベルを追加（詳細が開いているとき）',
  },
  'shortcuts.focusComment': {
    en: 'Focus the comment box (when detail is open)',
    ko: '코멘트 입력 포커스 (상세 열림 시)',
    ja: 'コメント欄にフォーカス（詳細が開いているとき）',
  },
  'shortcuts.focusSearch': {
    en: 'Focus the search or filter box',
    ko: '검색·필터 입력 포커스',
    ja: '検索またはフィルター欄にフォーカス',
  },
  'shortcuts.suggestions': {
    en: 'Move through suggestions',
    ko: '추천 항목 이동',
    ja: '候補を移動',
  },
  'shortcuts.applySearch': {
    en: 'Search every issue and document',
    ko: '이슈와 문서 전체에서 검색',
    ja: 'すべての課題とドキュメントを検索',
  },
  'shortcuts.clearSearch': {
    en: 'Clear the search box',
    ko: '검색창 비우기',
    ja: '検索欄をクリア',
  },
  'shortcuts.paletteMove': {
    en: 'Move through results',
    ko: '결과 이동',
    ja: '結果を移動',
  },
  'shortcuts.paletteRun': {
    en: 'Run the highlighted item',
    ko: '선택 항목 실행',
    ja: 'ハイライトした項目を実行',
  },
  'shortcuts.paletteClose': {
    en: 'Close the palette',
    ko: '팔레트 닫기',
    ja: 'パレットを閉じる',
  },
  'shortcuts.submitComment': {
    en: 'Submit the comment',
    ko: '코멘트 등록',
    ja: 'コメントを送信',
  },
  /* ── In-app browser pane (desktop app only) ── */
  'browse.paneLabel': {
    en: 'In-app browser',
    ko: '인앱 브라우저',
    ja: 'アプリ内ブラウザ',
  },
  'browse.tabs': {
    en: 'Open pages',
    ko: '열린 페이지',
    ja: '開いているページ',
  },
  'browse.back': {
    en: 'Back to gadak',
    ko: 'gadak로 돌아가기',
    ja: 'gadak に戻る',
  },
  'browse.backEsc': {
    en: 'Back to gadak (Esc)',
    ko: 'gadak로 돌아가기 (Esc)',
    ja: 'gadak に戻る (Esc)',
  },
  'browse.openExternal': {
    en: 'Open in the system browser',
    ko: '시스템 브라우저에서 열기',
    ja: 'システムのブラウザで開く',
  },
  'browse.closeTab': {
    en: 'Close this page',
    ko: '이 페이지 닫기',
    ja: 'このページを閉じる',
  },
  'browse.loading': {
    en: 'Opening {host}…',
    ko: '{host} 여는 중…',
    ja: '{host} を開いています…',
  },
  'browse.resume': {
    en: 'Browser',
    ko: '브라우저',
    ja: 'ブラウザ',
  },
  'browse.resumeHint': {
    en: '{n} open',
    ko: '{n}개 열림',
    ja: '{n}件開いています',
  },
  /* ── App / shell ── */
  'app.loadFailed': {
    en: 'Could not load data. Check network/VPN.',
    ko: '데이터를 불러오지 못했습니다. 네트워크/VPN 상태를 확인하세요.',
    ja: 'データを読み込めませんでした。ネットワーク/VPN を確認してください。',
  },
  'app.authGate': {
    en: 'Cannot connect to the local server.',
    ko: '로컬 서버에 연결할 수 없습니다.',
    ja: 'ローカルサーバーに接続できません。',
  },
  'app.authGateHint': {
    en: 'Make sure it is running.',
    ko: '가 실행 중인지 확인하세요.',
    ja: '起動していることを確認してください。',
  },
  // GDK-1048: the pre-dial scope refusal in mobile lib/api.ts. Not
  // 'network' — the server may be perfectly up; this app never sent the
  // request because the endpoint is outside the http capability scope.
  'app.endpointScope': {
    en: 'This app does not send requests to that address. It only talks to tailnet HTTPS names (`*.ts.net`) or this machine over loopback. Pair again with a serve address of that shape.',
    ko: '이 앱은 그 주소로 요청을 보내지 않습니다. 테일넷 HTTPS 주소(`*.ts.net`)나 이 기기의 루프백 주소로만 통신합니다. 그 형태의 serve 주소로 다시 페어링하세요.',
    ja: 'このアプリはそのアドレスにリクエストを送信しません。テールネットの HTTPS アドレス（`*.ts.net`）かこの端末のループバックのみに接続します。その形式の serve アドレスで再度ペアリングしてください。',
  },
  'app.offlineBanner': {
    en: 'Offline — showing cached data',
    ko: '오프라인 — 캐시된 데이터를 표시 중',
    ja: 'オフライン — キャッシュを表示中',
  },
  'app.demoBadge': {
    en: 'Demo',
    ko: '데모',
    ja: 'デモ',
  },
  'app.demoBanner': {
    en: 'Fictional issues, read-only. Nothing here connects to Jira and no account is involved.',
    ko: '가상의 이슈이며 읽기 전용입니다. Jira에 연결되지 않고 계정도 필요 없습니다.',
    ja: '架空の課題で、読み取り専用です。ここは Jira に繋がらず、アカウントも使いません。',
  },
  'app.demoBannerLink': {
    en: 'Run it on your own Jira →',
    ko: '내 Jira에서 실행하기 →',
    ja: '自分の Jira で実行する →',
  },
  'app.demoNoCredentials': {
    en: 'Credentials are disabled in the demo',
    ko: '데모에서는 자격증명을 설정할 수 없습니다',
    ja: 'デモでは資格情報を設定できません',
  },
  'app.demoWriteDisabled': {
    en: 'Creating issues needs a server — try a status change or a comment instead',
    ko: '이슈 생성은 서버가 필요합니다 — 상태 변경이나 코멘트를 시도해 보세요',
    ja: '課題の作成にはサーバーが必要です — 代わりにステータス変更やコメントを試してください',
  },
  'app.demoWriteNotice': {
    en: 'Demo edit applied in this browser only — it is not sent anywhere and a reload restores it.',
    ko: '데모 수정은 이 브라우저에만 적용됩니다 — 어디로도 전송되지 않고 새로고침하면 사라집니다.',
    ja: 'デモの編集はこのブラウザにだけ適用されます — どこにも送られず、再読み込みで元に戻ります。',
  },
  'app.demoAttachDisabled': {
    en: 'Attachments need a server — not available in the demo',
    ko: '첨부는 서버가 필요해 데모에서는 지원되지 않습니다',
    ja: '添付にはサーバーが必要です — デモでは利用できません',
  },
  'app.demoEditCount': {
    en: '{n} local edit(s), not saved',
    ko: '로컬 수정 {n}건 · 저장되지 않음',
    ja: 'ローカル編集 {n}件 · 未保存',
  },
  'app.demoEditedIssue': {
    en: 'Edited in this demo — not saved',
    ko: '이 데모에서 수정됨 — 저장되지 않음',
    ja: 'このデモで編集済み — 未保存',
  },
  // GDK-1051: the phone's bundled demo workspace (mobile/src/lib/demo.ts) —
  // a pairing-free, read-only sample mirror shipped in the app bundle.
  'app.demoEnter': {
    en: 'Explore the sample workspace',
    ko: '샘플 워크스페이스 둘러보기',
    ja: 'サンプルワークスペースを見る',
  },
  'app.demoWorkspace': {
    en: 'Demo workspace',
    ko: '데모 워크스페이스',
    ja: 'デモワークスペース',
  },
  'app.demoMode': {
    en: 'Demo — sample data, read-only',
    ko: '데모 — 샘플 데이터 · 읽기 전용',
    ja: 'デモ — サンプルデータ · 読み取り専用',
  },
  'app.demoExit': {
    en: 'Exit demo',
    ko: '데모 나가기',
    ja: 'デモを終了',
  },
  'app.demoPairedNote': {
    en: 'These issues are sample data bundled with the app. Nothing is paired and nothing is sent anywhere; exiting returns to pairing.',
    ko: '표시된 이슈는 앱에 포함된 샘플 데이터입니다. 페어링된 서버가 없고 아무것도 전송되지 않습니다. 나가면 페어링 화면으로 돌아갑니다.',
    ja: '表示されている課題はアプリ同梱のサンプルデータです。ペアリングされたサーバーはなく、どこにも送信されません。終了するとペアリング画面に戻ります。',
  },
  // GDK-1097 B2: the phone's known-host roster (mobile/src/screens/PairingTab.svelte)
  // — switching, forgetting, and pairing another host from `gadak pairing mint`.
  // No endpoint or token value ever appears in this copy.
  'app.hosts.title': {
    en: 'Hosts',
    ko: '호스트',
    ja: 'ホスト',
  },
  'app.hosts.active': {
    en: 'Active',
    ko: '활성',
    ja: 'アクティブ',
  },
  'app.hosts.remove': {
    en: 'Forget this host',
    ko: '이 호스트 지우기',
    ja: 'このホストを削除',
  },
  'app.hosts.removeConfirm': {
    en: 'Tap again to forget',
    ko: '다시 눌러 지우기',
    ja: 'もう一度タップして削除',
  },
  'app.hosts.add': {
    en: 'Pair another host',
    ko: '다른 호스트 페어링',
    ja: '別のホストをペアリング',
  },
  'app.hosts.addHide': {
    en: 'Close',
    ko: '닫기',
    ja: '閉じる',
  },
  'app.hosts.repairHint': {
    en: 'The token for this host is missing on this phone. Mint a new offer on its desktop and pair again.',
    ko: '이 호스트의 토큰이 이 폰에 없습니다. 해당 데스크톱에서 새 오퍼를 만들어 다시 페어링하세요.',
    ja: 'このホストのトークンがこの端末にありません。デスクトップで新しいオファーを作成して、再度ペアリングしてください。',
  },
  'app.hosts.offerLabel': {
    en: 'Pairing offer',
    ko: '페어링 오퍼',
    ja: 'ペアリングオファー',
  },
  'app.hosts.offerPlaceholder': {
    en: 'Paste the offer line here',
    ko: '오퍼 줄을 여기에 붙여넣으세요',
    ja: 'オファー行をここに貼り付けてください',
  },
  'app.hosts.pastePair': {
    en: 'Paste & pair',
    ko: '붙여넣고 페어링',
    ja: '貼り付けてペアリング',
  },
  'app.hosts.pair': {
    en: 'Pair',
    ko: '페어링',
    ja: 'ペアリング',
  },
  'app.hosts.checking': {
    en: 'Checking…',
    ko: '확인 중…',
    ja: '確認中…',
  },
  'app.hosts.scan': {
    en: 'Scan QR instead',
    ko: '대신 QR 스캔',
    ja: '代わりにQRをスキャン',
  },
  'app.hosts.errEmpty': {
    en: 'Paste the offer line first.',
    ko: '먼저 오퍼 줄을 붙여넣으세요.',
    ja: '先にオファー行を貼り付けてください。',
  },
  'app.hosts.errVersion': {
    en: 'This offer is from a newer gadak. Update the app, then pair.',
    ko: '이 오퍼는 더 최신 gadak에서 만들어졌습니다. 앱을 업데이트한 뒤 페어링하세요.',
    ja: 'このオファーは新しいgadakからのものです。アプリを更新してからペアリングしてください。',
  },
  'app.hosts.errBad': {
    en: 'That does not look like a pairing offer. Copy the whole line from `gadak pairing mint`.',
    ko: '페어링 오퍼로 보이지 않습니다. `gadak pairing mint`가 출력한 줄 전체를 복사하세요.',
    ja: 'ペアリングオファーではないようです。`gadak pairing mint` の出力行全体をコピーしてください。',
  },
  'app.hosts.errClipboardEmpty': {
    en: 'Clipboard is empty. Copy the offer line first.',
    ko: '클립보드가 비어 있습니다. 먼저 오퍼 줄을 복사하세요.',
    ja: 'クリップボードが空です。先にオファー行をコピーしてください。',
  },
  'app.hosts.errClipboardFail': {
    en: 'Could not read the clipboard. Paste into the field instead.',
    ko: '클립보드를 읽을 수 없습니다. 입력란에 직접 붙여넣으세요.',
    ja: 'クリップボードを読み取れません。入力欄に貼り付けてください。',
  },
  'app.hosts.errCamera': {
    en: 'Could not open the camera. Paste the offer line instead.',
    ko: '카메라를 열 수 없습니다. 대신 오퍼 줄을 붙여넣으세요.',
    ja: 'カメラを開けません。代わりにオファー行を貼り付けてください。',
  },
} as const satisfies Record<string, Message>
