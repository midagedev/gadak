/*
 * Issue list, filters, sort, bulk, triage.
 * One key = {en, ko, ja}; omitting a locale is a type error.
 */
import type { Message } from '../types'

export const list = {
  /* ── Filter bar ── */
  'filter.add': {
    en: '+ Filter',
    ko: '+ 필터',
    ja: '+ フィルター',
  },
  'filter.properties': {
    en: 'Properties',
    ko: '속성',
    ja: 'プロパティ',
  },
  'filter.quick': {
    en: 'Quick filters',
    ko: '빠른 필터',
    ja: 'クイックフィルター',
  },
  'filter.saveAsView': {
    en: 'Save as view',
    ko: '뷰로 저장',
    ja: 'ビューとして保存',
  },
  'filter.saveServerFailed': {
    en: 'Server save failed — saved in this browser instead',
    ko: '서버 저장에 실패해 이 브라우저에 저장했습니다',
    ja: 'サーバー保存に失敗したため、このブラウザに保存しました',
  },
  'filter.saveDemoLocal': {
    en: 'The demo keeps views in this browser',
    ko: '데모에서는 뷰가 이 브라우저에만 저장됩니다',
    ja: 'デモではビューはこのブラウザに保存されます',
  },
  'filter.clear': {
    en: 'Clear filters',
    ko: '필터 초기화',
    ja: 'フィルターをクリア',
  },
  'list.changedSinceSeen': {
    en: 'Changed since you opened it',
    ko: '연 뒤에 바뀜',
    ja: '開いた後に変更あり',
  },
  'filter.jqlNotAvailable': {
    en: 'JQL needs the app or gadak serve',
    ko: 'JQL은 앱 또는 gadak serve가 필요합니다',
    ja: 'JQL にはアプリまたは gadak serve が必要です',
  },
  'filter.jqlFailed': {
    en: 'Could not reach JQL. Check the connection and try again.',
    ko: 'JQL에 연결하지 못했습니다. 연결을 확인한 뒤 다시 시도하세요.',
    ja: 'JQL に到達できませんでした。接続を確認して再試行してください。',
  },
  'view.copyLink': {
    en: 'Copy link to this view',
    ko: '이 뷰 링크 복사',
    ja: 'このビューのリンクをコピー',
  },
  'filter.jqlCopiedPartial': {
    en: 'Copied JQL (not in Jira: {omitted})',
    ko: 'JQL 복사 (Jira에 없는 항목: {omitted})',
    ja: 'JQL をコピーしました（Jira にない項目: {omitted}）',
  },
  'filter.jqlApplied': {
    en: 'JQL filter applied',
    ko: 'JQL 필터를 적용했습니다',
    ja: 'JQL フィルターを適用しました',
  },
  'filter.jqlPartial': {
    en: 'Applied, except: {clauses}',
    ko: '적용했습니다. 빠진 절: {clauses}',
    ja: '適用しました。除いた節: {clauses}',
  },
  'filter.jqlParseFailed': {
    en: 'Could not parse that JQL',
    ko: 'JQL을 해석하지 못했습니다',
    ja: 'その JQL を解析できませんでした',
  },
  // Catch of emitJql/parseJql: json() throws ApiError on 4xx/5xx, fetch throws
  // on a dead connection. Patterned on write.jiraUnavailable.
  // internal/jql/compile.go ErrNotJQL: URL has no jql= parameter.
  'filter.notJql': {
    en: 'URL has no jql= parameter',
    ko: 'URL에 jql= 파라미터가 없습니다',
    ja: 'URL に jql= パラメータがありません',
  },
  'filter.remove': {
    en: 'Remove filter',
    ko: '필터 제거',
    ja: 'フィルターを削除',
  },
  'filter.viewName': {
    en: 'View name',
    ko: '뷰 이름',
    ja: 'ビュー名',
  },
  'filter.searchField': {
    en: 'Search {field}',
    ko: '{field} 검색',
    ja: '{field} を検索',
  },
  'filter.chipFieldValue': {
    en: '{field}: {value}',
    ko: '{field}: {value}',
    ja: '{field}: {value}',
  },
  // GDK-438 negation chip. {neg} is chipNegWord, split out so the chip can
  // render just that word heavier (vision verdict: polarity must survive a scan).
  'filter.chipFieldValueNot': {
    en: '{field}: {neg} {value}',
    ko: '{field}: {value} {neg}',
    ja: '{field}: {neg} {value}',
  },
  'filter.chipNegWord': {
    en: 'not',
    ko: '제외',
    ja: '除外',
  },
  'filter.chipKeys': {
    en: '{n} keys',
    ko: '키 {n}개',
    ja: '{n}件のキー',
  },
  // CLI KeyLimitMessage (internal/jql/keys.go:21) + shown count (GDK-35).
  'filter.keysCapped': {
    en: 'key list has {given} values; the limit is {limit}. Showing {shown} keys.',
    ko: '키 목록에 값이 {given}개입니다. 한도는 {limit}개입니다. {shown}개 키를 표시합니다.',
    ja: 'キー一覧は {given} 件です。上限は {limit} 件です。{shown} 件のキーを表示しています。',
  },
  // GDK-771: every visible axis excludes via the per-value ⊘ (tri-state
  // rows replaced the GDK-438 modal toggle and the GDK-474 "No exclude"
  // caption — that label was noise once negation stopped being project-only).
  'filter.excludeValue': {
    en: 'Exclude {value} (Alt-click)',
    ko: '{value} 제외 (Alt+클릭)',
    ja: '{value} を除外 (Alt+クリック)',
  },
  'filter.chipCreatedRange': {
    en: 'Created {from}–{to}',
    ko: '생성 {from}~{to}',
    ja: '作成 {from}–{to}',
  },
  'filter.chipUpdatedRange': {
    en: 'Updated {from}–{to}',
    ko: '갱신 {from}~{to}',
    ja: '更新 {from}–{to}',
  },
  'filter.chipDueRange': {
    en: 'Due {from}–{to}',
    ko: '기한 {from}~{to}',
    ja: '期限 {from}–{to}',
  },
  'filter.chipResolvedRange': {
    en: 'Resolved {from}–{to}',
    ko: '해결 {from}~{to}',
    ja: '解決 {from}–{to}',
  },
  'filter.dateFrom': {
    en: 'From',
    ko: '시작',
    ja: '開始',
  },
  'filter.dateTo': {
    en: 'To',
    ko: '끝',
    ja: '終了',
  },
  'filter.flagReopened': {
    en: 'Reopened',
    ko: '재오픈',
    ja: '再オープン',
  },
  'filter.flagUnassigned': {
    en: 'Unassigned',
    ko: '미할당',
    ja: '未割り当て',
  },
  'filter.flagStale': {
    en: 'Stale',
    ko: '정체',
    ja: '滞留',
  },
  'filter.qaBlocking': {
    en: 'Blocking current run',
    ko: '현재 차수 차단',
    ja: '今ランをブロック',
  },
  'filter.qaRetest': {
    en: 'Awaiting retest',
    ko: '재검증 대기',
    ja: '再テスト待ち',
  },
  'filter.qaVerified': {
    en: 'Verified',
    ko: '검증 완료',
    ja: '検証済み',
  },
  'filter.qaLinked': {
    en: 'Linked to current run',
    ko: '현재 차수 연결',
    ja: '今ランにリンク',
  },
  /* ── View settings menu (GDK-1391): layout / sort / columns / save view ── */
  'view.settings': {
    en: 'View settings',
    ko: '보기 설정',
    ja: '表示設定',
  },
  'sort.label': {
    en: 'Sort',
    ko: '정렬',
    ja: '並べ替え',
  },
  'sort.direction': {
    en: 'Sort direction',
    ko: '정렬 방향',
    ja: '並べ替え方向',
  },
  'sort.updated': {
    en: 'Recently updated',
    ko: '갱신순',
    ja: '最近の更新',
  },
  'sort.created': {
    en: 'Recently created',
    ko: '생성순',
    ja: '最近の作成',
  },
  'sort.due': {
    en: 'Due date',
    ko: '기한순',
    ja: '期限',
  },
  'sort.priority': {
    en: 'Priority',
    ko: '우선순위',
    ja: '優先度',
  },
  'sort.reopenCount': {
    en: 'Reopen count',
    ko: '재오픈수',
    ja: '再オープン回数',
  },
  'sort.relevance': {
    en: 'Relevance',
    ko: '관련도',
    ja: '関連度',
  },
  'sort.keys': {
    en: 'Given order',
    ko: '지정 순서',
    ja: '指定順',
  },
  'sort.desc': {
    en: '↓ Desc',
    ko: '↓ 내림',
    ja: '↓ 降順',
  },
  'sort.asc': {
    en: '↑ Asc',
    ko: '↑ 오름',
    ja: '↑ 昇順',
  },
  'columns.exposed': {
    en: 'Visible columns',
    ko: '노출 컬럼',
    ja: '表示する列',
  },
  'columns.defaults': {
    en: 'Defaults',
    ko: '기본값',
    ja: '既定',
  },
  'columns.reset': {
    en: 'Reset to default columns',
    ko: '기본 컬럼으로 되돌리기',
    ja: '既定の列に戻す',
  },
  /* ── List ── */
  'list.countIssues': {
    en: '{n} issues',
    ko: '{n}건',
    ja: '{n}件',
  },
  'list.bodyMatchCount': {
    en: '{n} body matches · "{q}"',
    ko: '본문 매치 {n}건 · "{q}"',
    ja: '本文一致 {n}件 · "{q}"',
  },
  'list.emptyTitle': {
    en: 'No issues',
    ko: '이슈가 없습니다',
    ja: '課題はありません',
  },
  'list.emptyHint': {
    en: 'They will appear here when sync finishes.',
    ko: '동기화가 완료되면 여기 표시됩니다.',
    ja: '同期が終わるとここに表示されます。',
  },
  'list.emptyLocalTitle': {
    en: 'No issues yet',
    ko: '아직 이슈가 없습니다',
    ja: '課題はまだありません',
  },
  'list.emptyLocalHint': {
    en: 'The first one is yours to write — c, or New issue in the sidebar.',
    ko: '첫 이슈를 만들어 보세요 — c 키, 또는 사이드바의 새 이슈.',
    ja: '最初の課題を作成しましょう — c キー、またはサイドバーの新規課題。',
  },
  'list.emptyRunSync': {
    en: 'Sync now',
    ko: '지금 동기화',
    ja: '今すぐ同期',
  },
  'list.emptySyncHint': {
    en: 'Or keep the server running with gadak serve for automatic updates.',
    ko: '자동 갱신이 필요하면 gadak serve로 서버를 실행하세요.',
    ja: '自動更新には gadak serve でサーバーを起動してください。',
  },
  // GDK-835: list-body render crash. Names the next action the way the
  // other list dead ends do — not "Something went wrong".
  'list.renderFailedTitle': {
    en: 'Could not show this list. Retry to load it again.',
    ko: '이 목록을 표시하지 못했습니다. 다시 불러오려면 재시도하세요.',
    ja: 'この一覧を表示できませんでした。再試行して読み込み直してください。',
  },
  'list.renderFailedRetry': {
    en: 'Retry',
    ko: '다시 시도',
    ja: '再試行',
  },
  'list.bodyOnlyTitle': {
    en: 'No issues match — found in body',
    ko: '조건에 맞는 이슈는 없고, 본문에서 찾았습니다',
    ja: '一致する課題はありません — 本文で見つかりました',
  },
  'list.bodyOnlyHint': {
    en: "See the 'Body matches' section above.",
    ko: "위 '본문 매치' 섹션을 확인하세요.",
    ja: '上の「本文一致」セクションを見てください。',
  },
  'list.noMatchTitle': {
    en: 'No issues match',
    ko: '조건에 맞는 이슈가 없습니다',
    ja: '一致する課題はありません',
  },
  'list.noMatchHint': {
    en: 'Relax filters or change the search query.',
    ko: '필터를 완화하거나 검색어를 바꿔보세요.',
    ja: 'フィルターを緩めるか、検索語を変えてください。',
  },
  'list.noMatchQueryHint': {
    en: 'No issues match this search.',
    ko: '이 검색어에 맞는 이슈가 없습니다.',
    ja: 'この検索に一致する課題はありません。',
  },
  // GDK-478: Enter ran body search and the extra/docs groups are also empty.
  'list.noMatchBodyHint': {
    en: 'No matches in body or comments either.',
    ko: '본문·코멘트에서도 0건입니다.',
    ja: '本文・コメントにも一致はありません。',
  },
  'list.clearSearch': {
    en: 'Clear search',
    ko: '검색 지우기',
    ja: '検索をクリア',
  },
  'list.select': {
    en: 'Select',
    ko: '선택',
    ja: '選択',
  },
  'list.selectedCount': {
    en: '{n} selected',
    ko: '{n}건 선택',
    ja: '{n}件選択',
  },
  'list.moreCount': {
    en: '+{n} more',
    ko: '외 {n}개',
    ja: 'ほか {n}件',
  },
  'list.commentCount': {
    en: '{n} comments',
    ko: '코멘트 {n}개',
    ja: 'コメント {n}件',
  },
  'list.reopenCount': {
    en: 'Reopened {n}×',
    ko: '재오픈 {n}회',
    ja: '再オープン {n}回',
  },
  'list.reopenCountReason': {
    en: 'Reopened {n}× · {reason}',
    ko: '재오픈 {n}회 · {reason}',
    ja: '再オープン {n}回 · {reason}',
  },
  'list.staleDays': {
    en: '{n} days in this status',
    ko: '이 상태로 {n}일째',
    ja: 'このステータスで {n}日',
  },
  // Session strip ([list]) — one quiet line above the list saying what
  // changed since the previous session (spec r2-session; THEORY.md "Session
  // start"). G1: the subject is the issues, never the reader — no you/your,
  // no 당신/님 (C3). Singular is its own key (no plural infrastructure).
  'list.sessionSince': {
    en: 'Since last session {ago}',
    ko: '지난 세션 이후 {ago}',
    ja: '前回のセッション以降 {ago}',
  },
  'list.sessionChanged': {
    en: '{n} issues changed',
    ko: '이슈 {n}건 변경',
    ja: '{n}件の課題が変更',
  },
  'list.sessionChangedOne': {
    en: '1 issue changed',
    ko: '이슈 1건 변경',
    ja: '1件の課題が変更',
  },
  'list.sessionMine': {
    en: '{k} of them assigned here',
    ko: '그중 이 계정 배정 {k}건',
    ja: 'うちこのアカウント担当 {k}件',
  },
  // Hover title when the threshold was learned, not set — the row names its
  // rule (G7): what the 85% line is and where it came from.
  // The sample count rides along with the number (G7: the basis is one
  // line, and how many issues a percentile stands on is part of the basis —
  // second literature round, 2026-09-06).
  'list.staleDaysLearned': {
    en: '{n} days in this status — longer than 85% of the {s} issues finished in the last 90 days ({p} days)',
    ko: '이 상태로 {n}일째 — 최근 90일간 완료된 {s}건의 85%보다 깁니다 ({p}일)',
    ja: 'このステータスで {n}日 — 直近90日に完了した {s}件の85%より長くかかっています ({p}日)',
  },
  // The started-clock twins (2026-09-07): when the mirror knows when work
  // started (started_at), the age is work item age, and the title says so.
  'list.staleDaysStarted': {
    en: '{n} days since work started',
    ko: '착수 후 {n}일째',
    ja: '着手から {n}日',
  },
  'list.staleDaysStartedLearned': {
    en: '{n} days since work started — longer than 85% of the {s} issues finished in the last 90 days ({p} days)',
    ko: '착수 후 {n}일째 — 최근 90일간 완료된 {s}건의 85%보다 깁니다 ({p}일)',
    ja: '着手から {n}日 — 直近90日に完了した {s}件の85%より長くかかっています ({p}日)',
  },
  'list.staleDaysShort': {
    en: '{n}d',
    ko: '{n}일',
    ja: '{n}日',
  },
  'list.createdAt': {
    en: 'Created {time}',
    ko: '생성 {time}',
    ja: '作成 {time}',
  },
  'list.dueAt': {
    en: 'Due {date}',
    ko: '기한 {date}',
    ja: '期限 {date}',
  },
  'list.categoryTitle': {
    en: 'Category: {label} ({status})',
    ko: '분류: {label} ({status})',
    ja: 'カテゴリ: {label} ({status})',
  },
  'list.categoryFilter': {
    en: 'Filter by category {label}',
    ko: '분류 {label} 필터',
    ja: 'カテゴリ {label} で絞る',
  },
  'list.fieldValue': {
    en: '{field}: {value}',
    ko: '{field}: {value}',
    ja: '{field}: {value}',
  },
  'list.priorityLabel': {
    en: 'Priority {label}',
    ko: '우선순위 {label}',
    ja: '優先度 {label}',
  },
  'list.priorityNone': {
    en: 'No priority',
    ko: '우선순위 없음',
    ja: '優先度なし',
  },
  'list.qaBlock': {
    en: 'QA blocked',
    ko: 'QA 차단',
    ja: 'QA ブロック',
  },
  'list.qaRetest': {
    en: 'Retest',
    ko: '재검증',
    ja: '再テスト',
  },
  'list.qaDone': {
    en: 'QA done',
    ko: 'QA 완료',
    ja: 'QA 完了',
  },
  'list.qaRun': {
    en: 'QA run',
    ko: 'QA 차수',
    ja: 'QA ラン',
  },
  'list.qaPending': {
    en: 'QA pending',
    ko: 'QA 대기',
    ja: 'QA 待ち',
  },
  'list.searchPlaceholder': {
    en: 'Search this list — key, title, @assignee…',
    ko: '이 목록에서 검색 — 키·제목·@담당자…',
    ja: 'この一覧を検索 — キー・タイトル・@担当者…',
  },
  'list.searchPlaceholderShort': {
    en: 'Search this list…',
    ko: '이 목록에서 검색…',
    ja: 'この一覧を検索…',
  },
  'list.searchHelp': {
    en: 'Searches this list (key, title, assignee, labels). Example: @dana or is:unassigned. Paste JQL to apply it. Enter searches body and comments here.',
    ko: '이 목록을 검색합니다 (키·제목·담당자·라벨). 예: @dana 또는 is:unassigned. JQL을 붙여넣으면 적용됩니다. Enter로 본문·코멘트를 검색합니다.',
    ja: 'この一覧を検索します（キー、タイトル、担当者、ラベル）。例: @dana または is:unassigned。JQL を貼ると適用されます。Enter で本文とコメントを検索します。',
  },
  'list.searchHelpShortcuts': {
    en: 'Full syntax is in the keyboard cheat sheet (?)',
    ko: '전체 문법은 키보드 치트시트(?)에 있습니다',
    ja: '構文の全体はキーボードチートシート (?) にあります',
  },
  'list.searchClear': {
    en: 'Clear (Esc)',
    ko: '지우기 (Esc)',
    ja: 'クリア (Esc)',
  },
  'list.searchOpen': {
    en: 'Open with Enter',
    ko: 'Enter로 열기',
    ja: 'Enter で開く',
  },
  'omnibox.issueMissing': {
    en: '{key} is not in the mirror',
    ko: '미러에 {key}가 없습니다',
    ja: '{key} はミラーにありません',
  },
  'list.searchFailed': {
    en: 'Could not search body text. Check the connection and try again.',
    ko: '본문 검색에 실패했습니다. 연결을 확인한 뒤 다시 시도하세요.',
    ja: '本文検索に失敗しました。接続を確認して再試行してください。',
  },
  // A deployment with no server FTS (static snapshot). Not a failure: the
  // network is fine, and title/key search still ran.
  'list.searchBodyUnavailable': {
    en: 'This snapshot searches titles and keys only.',
    ko: '이 스냅샷은 제목과 키만 검색합니다.',
    ja: 'このスナップショットはタイトルとキーだけを検索します。',
  },
  'list.searchRetry': {
    en: 'Retry body search',
    ko: '본문 검색 다시 시도',
    ja: '本文検索を再試行',
  },
  'list.matchInComment': {
    en: 'in a comment',
    ko: '코멘트에서',
    ja: 'コメント内',
  },
  'list.docMatchCount': {
    en: '{n} documents · "{q}"',
    ko: '문서 {n}건 · "{q}"',
    ja: 'ドキュメント {n}件 · "{q}"',
  },
  'list.docOnlyTitle': {
    en: 'No issues match — found in documents',
    ko: '조건에 맞는 이슈는 없고, 문서에서 찾았습니다',
    ja: '一致する課題はありません — ドキュメントで見つかりました',
  },
  'list.docOnlyHint': {
    en: "See the 'Documents' section above.",
    ko: "위 '문서' 섹션을 확인하세요.",
    ja: '上の「ドキュメント」セクションを見てください。',
  },
  /* ── Bulk bar ── */
  'bulk.changeStatus': {
    en: 'Change status',
    ko: '상태 변경',
    ja: 'ステータスを変更',
  },
  'bulk.changePriority': {
    en: 'Change priority',
    ko: '우선순위 변경',
    ja: '優先度を変更',
  },
  'bulk.changeAssignee': {
    en: 'Change assignee',
    ko: '담당자 변경',
    ja: '担当者を変更',
  },
  'bulk.changeLabels': {
    en: 'Change labels',
    ko: '라벨 변경',
    ja: 'ラベルを変更',
  },
  'bulk.noCommonTransitions': {
    en: 'No shared transitions.',
    ko: '공통 전환이 없습니다.',
    ja: '共通のトランジションがありません。',
  },
  'bulk.resultOk': {
    en: '{n} succeeded',
    ko: '성공 {n}',
    ja: '{n}件成功',
  },
  'bulk.resultFail': {
    en: '{n} failed',
    ko: '실패 {n}',
    ja: '{n}件失敗',
  },
  'bulk.resultSkip': {
    en: '{n} skipped',
    ko: '건너뜀 {n}',
    ja: '{n}件スキップ',
  },
  'bulk.pickLabel': {
    en: 'Choose a label',
    ko: '라벨 선택',
    ja: 'ラベルを選ぶ',
  },
  'bulk.searchLabel': {
    en: 'Type a label',
    ko: '라벨 입력',
    ja: 'ラベルを入力',
  },
  'bulk.onSelection': {
    en: 'On selection',
    ko: '선택한 이슈에',
    ja: '選択に対して',
  },
  'bulk.typeALabel': {
    en: 'Type a label to add',
    ko: '추가할 라벨을 입력하세요',
    ja: '追加するラベルを入力',
  },
  /* ── Keyboard triage ── */
  'triage.commentOn': {
    en: 'Comment on {key}',
    ko: '{key}에 코멘트',
    ja: '{key} にコメント',
  },
  /* ── Board layout (GDK-1175) ── */
  'board.label': {
    en: 'Board',
    ko: '보드',
    ja: 'ボード',
  },
  'board.layout': {
    en: 'Layout',
    ko: '레이아웃',
    ja: 'レイアウト',
  },
  'board.asList': {
    en: 'List',
    ko: '목록',
    ja: '一覧',
  },
  'board.asBoard': {
    en: 'Board',
    ko: '보드',
    ja: 'ボード',
  },
  'board.columnEmpty': {
    en: 'Nothing here',
    ko: '비어 있음',
    ja: '何もありません',
  },
  // The card's way into the shell already on this issue (GDK-1197).
  'board.openShell': {
    en: 'Open this shell',
    ko: '이 셸 열기',
    ja: 'このシェルを開く',
  },
  'board.shellNeeds': {
    en: 'This shell is waiting for you',
    ko: '이 셸이 기다리고 있습니다',
    ja: 'このシェルが待っています',
  },
  'board.shellRunning': {
    en: 'A shell is printing on this issue',
    ko: '이 이슈의 셸이 출력 중입니다',
    ja: 'この課題のシェルが出力中です',
  },
  'board.shellQuiet': {
    en: 'A shell is attached to this issue',
    ko: '이 이슈에 셸이 붙어 있습니다',
    ja: 'この課題にシェルが接続中です',
  },
  // An ambiguous drop: 2+ transitions reach the column (GDK-1176).
  'board.dropChoose': {
    en: 'Choose a transition',
    ko: '전환 선택',
    ja: 'トランジションを選択',
  },
  // The transitions GET never answered, so no transition id exists to attempt
  // and nothing dimmed anything (GDK-1221). Multi-sentence failure copy, so it
  // keeps its periods — the no-period rule is for single-sentence toasts.
  'board.dropTransitionsFailed': {
    en: 'Could not load transitions, so the card cannot move. Check the connection and try again.',
    ko: '전환 목록을 가져오지 못해 카드를 옮길 수 없습니다. 연결을 확인한 뒤 다시 시도하세요.',
    ja: 'トランジションを読み込めなかったため、カードを移動できません。接続を確認して再試行してください。',
  },
} as const satisfies Record<string, Message>
