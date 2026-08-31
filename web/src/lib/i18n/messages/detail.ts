/*
 * Issue detail, documents, history, person, QA.
 * One key = {en, ko, ja}; omitting a locale is a type error.
 */
import type { Message } from '../types'

export const detail = {
  /* ── Detail ── */
  'detail.details': {
    en: 'Details',
    ko: '상세',
    ja: '詳細',
  },
  'detail.description': {
    en: 'Description',
    ko: '설명',
    ja: '説明',
  },
  'detail.noDescription': {
    en: 'No description',
    ko: '설명 없음',
    ja: '説明なし',
  },
  'detail.noContent': {
    en: 'No content',
    ko: '내용 없음',
    ja: '内容なし',
  },
  'detail.attachments': {
    en: 'Attachments',
    ko: '첨부',
    ja: '添付',
  },
  /* ── Commands in the body, and the shell they go to (GDK-1162/GDK-1164) ──
     "Place", never "run". The button puts a line someone else wrote at a
     prompt; the Enter that runs it is a keystroke a person makes, and the
     serve enforces that (internal/server/terminal.go refuses \n and \r). Copy
     that said "run" would be describing a feature this product declined to
     build. */
  'detail.runInShell': {
    en: "Place at this issue's shell prompt — does not run it",
    ko: '이 이슈의 셸 프롬프트에 놓기 — 실행하지는 않습니다',
    ja: 'この課題のシェルのプロンプトに置く — 実行はしません',
  },
  'detail.placeFailed': {
    en: 'Could not place the command in the shell.',
    ko: '명령을 셸에 놓지 못했습니다.',
    ja: 'コマンドをシェルに置けませんでした。',
  },
  'detail.noShellForIssue': {
    en: 'No shell here is attached to {key}. Run `gadak claim {key}` in the terminal pane to bind one.',
    ko: '여기에는 {key}에 붙어 있는 셸이 없습니다. 터미널 패널에서 `gadak claim {key}`를 실행하면 붙습니다.',
    ja: 'ここには {key} に紐づいたシェルがありません。ターミナルパネルで `gadak claim {key}` を実行すると紐づきます。',
  },
  /* The mark's wording is the feature. A session binding is runtime state
     that dies with the serve, so what is actually known is "no shell *here*
     is on it" — never "this work is dead". Someone who reads the mark as the
     second thing has been lied to. */
  'detail.unattended': {
    en: 'No shell here',
    ko: '붙은 셸 없음',
    ja: 'シェルなし',
  },
  'detail.unattendedHint': {
    en: 'In progress, but no shell this serve knows about is attached to it. A shell on another machine — or one from before this serve restarted — is not visible here.',
    ko: '진행 중이지만 이 serve가 아는 셸 중 이 이슈에 붙은 것이 없습니다. 다른 기기에서 도는 셸이나 이 serve가 재시작되기 전의 셸은 여기서 보이지 않습니다.',
    ja: '進行中ですが、この serve が把握しているシェルの中にこの課題へ紐づいたものはありません。別のマシンで動いているシェルや、この serve の再起動前のシェルはここには見えません。',
  },
  'detail.qaImpact': {
    en: 'QA impact',
    ko: 'QA 영향',
    ja: 'QA 影響',
  },
  'detail.comments': {
    en: 'Comments',
    ko: '코멘트',
    ja: 'コメント',
  },
  'detail.history': {
    en: 'History',
    ko: '변경 이력',
    ja: '履歴',
  },
  'detail.links': {
    en: 'Linked issues',
    ko: '연결 이슈',
    ja: 'リンクされた課題',
  },
  'detail.docs': {
    en: 'Documents',
    ko: '문서',
    ja: 'ドキュメント',
  },
  'detail.docMentions': {
    en: 'mentions this issue',
    ko: '이 이슈를 언급',
    ja: 'この課題に言及',
  },
  'detail.docMentioned': {
    en: 'mentioned in this issue',
    ko: '이 이슈에서 언급됨',
    ja: 'この課題で言及',
  },
  'detail.breadcrumb': {
    en: 'Issue hierarchy',
    ko: '이슈 위계',
    ja: '課題の階層',
  },
  'detail.epicChildren': {
    en: 'In this epic',
    ko: '이 에픽의 이슈',
    ja: 'このエピック内',
  },
  'detail.childIssues': {
    en: 'Child issues',
    ko: '하위 이슈',
    ja: '子課題',
  },
  'detail.epicProgress': {
    en: '{done} of {total} done',
    ko: '{total}건 중 {done}건 완료',
    ja: '{total}件中 {done}件完了',
  },
  'detail.epicShowAll': {
    en: 'Show {n} more',
    ko: '{n}건 더 보기',
    ja: 'さらに {n}件表示',
  },
  'detail.epicShowLess': {
    en: 'Show fewer',
    ko: '접기',
    ja: '少なく表示',
  },
  'detail.deploy': {
    en: 'Deploy status',
    ko: '배포 상태',
    ja: 'デプロイ状況',
  },
  'detail.prs': {
    en: 'Linked PRs',
    ko: '연결 PR',
    ja: 'リンクされた PR',
  },
  'detail.noPrs': {
    en: 'No linked PRs',
    ko: '연결된 PR 없음',
    ja: 'リンクされた PR はありません',
  },
  // GDK-555: connected workspace with an empty list — PRs exist on Jira's
  // GitHub app and are mirrored only when config.json `devStatus` is on.
  // cmd/gadak/dev.go names both halves of this sentence.
  'detail.prsNotMirrored': {
    en: "PRs are mirrored via devStatus; writes go through Jira's GitHub app",
    ko: 'PR은 devStatus로 미러됩니다. 쓰기는 Jira의 GitHub 앱을 통합니다',
    ja: 'PR は devStatus でミラーされます。書き込みは Jira の GitHub アプリ経由です',
  },
  // Who attached the link (dev-panel actor) — distinct from the PR's author.
  'detail.prLinkedBy': {
    en: 'Linked by {name}',
    ko: '연결: {name}',
    ja: 'リンクした人: {name}',
  },
  'detail.notFound': {
    en: 'Could not find this issue. It may have been deleted.',
    ko: '이 이슈를 찾을 수 없습니다. 삭제되었을 수 있습니다.',
    ja: 'この課題は見つかりませんでした。削除された可能性があります。',
  },
  'detail.loadFailed': {
    en: 'Could not load details.',
    ko: '상세를 불러오지 못했습니다.',
    ja: '詳細を読み込めませんでした。',
  },
  'detail.openJira': {
    en: 'Open in Jira',
    ko: 'Jira 원본 열기',
    ja: 'Jira で開く',
  },
  'detail.copyLink': {
    en: 'Copy link',
    ko: '링크 복사',
    ja: 'リンクをコピー',
  },
  'detail.linkCopied': {
    en: 'Copied',
    ko: '복사됨',
    ja: 'コピーしました',
  },
  // Toast copy rule: a single-sentence toast carries no trailing period
  // (`filter.saveServerFailed` in list.ts is the standard); hints and
  // standalone sentences keep theirs. (GDK-1226)
  'clipboard.copyFailed': {
    en: 'Could not copy — the clipboard refused the write',
    ko: '복사하지 못했습니다 — 클립보드가 쓰기를 거부했습니다',
    ja: 'コピーできませんでした — クリップボードが書き込みを拒否しました',
  },
  'detail.reopened': {
    en: 'Reopened',
    ko: '재오픈됨',
    ja: '再オープン',
  },
  'detail.reopenTimes': {
    en: 'Reopened ×{n}',
    ko: '재오픈 ×{n}',
    ja: '再オープン ×{n}',
  },
  // GDK-590 durations chip — the same spans the CLI's durations line prints.
  // Rendered only when the changelog can answer; absent spans drop their part.
  'detail.waitSpan': {
    en: 'Waited {span}',
    ko: '대기 {span}',
    ja: '待機 {span}',
  },
  'detail.progressSpan': {
    en: 'In progress {span}',
    ko: '진행 {span}',
    ja: '進行中 {span}',
  },
  'detail.priorityShort': {
    en: 'Prio {p}',
    ko: '우선 {p}',
    ja: '優先度 {p}',
  },
  'detail.severityShort': {
    en: 'Sev {s}',
    ko: '심각도 {s}',
    ja: '重大度 {s}',
  },
  'detail.linked': {
    en: 'Linked',
    ko: '연결됨',
    ja: 'リンク済み',
  },
  'detail.inLocalPool': {
    en: 'In local pool',
    ko: '로컬 풀에 있음',
    ja: 'ローカルプール内',
  },
  'detail.linkAdd': {
    en: 'Add link',
    ko: '링크 추가',
    ja: 'リンクを追加',
  },
  'detail.linkType': {
    en: 'Link type',
    ko: '링크 유형',
    ja: 'リンクタイプ',
  },
  'detail.linkKey': {
    en: 'Issue key',
    ko: '이슈 키',
    ja: '課題キー',
  },
  'detail.linkAddFailed': {
    en: 'Could not add this link.',
    ko: '링크를 추가하지 못했습니다.',
    ja: 'このリンクを追加できませんでした。',
  },
  'detail.linkSelf': {
    en: 'An issue cannot be linked to itself.',
    ko: '이슈를 자기 자신과 연결할 수 없습니다.',
    ja: '課題を自分自身にリンクできません。',
  },
  'detail.emptyComment': {
    en: '(empty comment)',
    ko: '(빈 코멘트)',
    ja: '(空のコメント)',
  },
  'detail.replyToComment': {
    en: 'Reply to this comment',
    ko: '이 코멘트에 답글',
    ja: 'このコメントに返信',
  },
  'detail.enlarge': {
    en: 'Enlarge {name}',
    ko: '{name} 크게 보기',
    ja: '{name} を拡大',
  },
  'detail.play': {
    en: 'Play {name}',
    ko: '{name} 재생',
    ja: '{name} を再生',
  },
  'detail.attachmentLabel': {
    en: 'Attachment: {name}',
    ko: '첨부: {name}',
    ja: '添付: {name}',
  },
  'detail.unknownAuthor': {
    en: 'Unknown',
    ko: '알 수 없음',
    ja: '不明',
  },
  /* ── Document panel (mirrored wiki pages) ── */
  'doc.badge': {
    en: 'Doc',
    ko: '문서',
    ja: 'ドキュメント',
  },
  'doc.content': {
    en: 'Content',
    ko: '본문',
    ja: '本文',
  },
  'doc.comments': {
    en: 'Comments',
    ko: '코멘트',
    ja: 'コメント',
  },
  'doc.noContent': {
    en: 'This document is empty.',
    ko: '내용이 비어 있습니다.',
    ja: 'このドキュメントは空です。',
  },
  'doc.commentPlaceholder': {
    en: 'Comment on this document…',
    ko: '이 문서에 코멘트 남기기…',
    ja: 'このドキュメントにコメント…',
  },
  'doc.commentNeedCredentials': {
    en: 'Set credentials to leave a comment',
    ko: '코멘트를 남기려면 자격증명을 설정하세요',
    ja: 'コメントするには資格情報を設定してください',
  },
  'doc.version': {
    en: 'v{n}',
    ko: 'v{n}',
    ja: 'v{n}',
  },
  'doc.breadcrumb': {
    en: 'Document path',
    ko: '문서 경로',
    ja: 'ドキュメントのパス',
  },
  'doc.openSource': {
    en: 'Open the original document',
    ko: '원본 문서 열기',
    ja: '元のドキュメントを開く',
  },
  'doc.notFound': {
    en: 'Could not find this document. It may have been deleted.',
    ko: '문서를 찾을 수 없습니다. 삭제되었을 수 있습니다.',
    ja: 'このドキュメントは見つかりませんでした。削除された可能性があります。',
  },
  'doc.loadFailed': {
    en: 'Could not load this document. Try again.',
    ko: '문서를 불러오지 못했습니다. 다시 시도하세요.',
    ja: 'このドキュメントを読み込めませんでした。再試行してください。',
  },
  'doc.issues': {
    en: 'Issues',
    ko: '이슈',
    ja: '課題',
  },
  'doc.issueMentions': {
    en: 'mentions this document',
    ko: '이 문서를 언급',
    ja: 'このドキュメントに言及',
  },
  'doc.issueMentioned': {
    en: 'mentioned on this document',
    ko: '이 문서에서 언급됨',
    ja: 'このドキュメントで言及',
  },
  /* ── Documents (main column) ── */
  'docs.title': {
    en: 'Documents',
    ko: '문서',
    ja: 'ドキュメント',
  },
  'docs.tabViewed': {
    en: 'Viewed',
    ko: '내가 본',
    ja: '閲覧',
  },
  'docs.tabUpdated': {
    en: 'Updated',
    ko: '최근 갱신',
    ja: '更新',
  },
  'docs.tabAuthor': {
    en: 'By author',
    ko: '작성자별',
    ja: '作成者別',
  },
  'docs.viewList': {
    en: 'List',
    ko: '목록',
    ja: '一覧',
  },
  'docs.viewTree': {
    en: 'Tree',
    ko: '트리',
    ja: 'ツリー',
  },
  /* Row meta reads as one sentence: "Alex Kim · 3h · in Engineering". */
  'docs.metaIn': {
    en: 'in {space}',
    ko: '{space}',
    ja: '{space}',
  },
  'docs.unread': {
    en: 'Edited since you last opened it',
    ko: '마지막으로 연 뒤에 수정됨',
    ja: '前回開いてから編集されています',
  },
  'docs.authorUnknown': {
    en: 'Unknown author',
    ko: '작성자 미상',
    ja: '作成者不明',
  },
  'docs.viewedEmpty': {
    en: 'Documents you open will appear here',
    ko: '연 문서가 여기에 쌓입니다',
    ja: '開いたドキュメントがここに表示されます',
  },
  'docs.viewedEmptyHint': {
    en: 'Until then, Updated shows what changed across every space.',
    ko: '그전까지는 최근 갱신 탭이 모든 스페이스의 변경을 보여줍니다.',
    ja: 'それまでは「更新」がすべてのスペースの変化を表示します。',
  },
  'docs.recentEmpty': {
    en: 'No mirrored documents yet',
    ko: '미러링된 문서가 아직 없습니다',
    ja: 'ミラーされたドキュメントはまだありません',
  },
  /* GDK-1054: the index request failed — the list-screen counterpart of
     doc.loadFailed ("this document"), which is about one open page. */
  'docs.loadFailed': {
    en: 'Could not load documents.',
    ko: '문서 목록을 불러오지 못했습니다.',
    ja: 'ドキュメントの一覧を読み込めませんでした。',
  },
  /* The field narrows what is on screen; Enter leaves for the whole mirror. The
     placeholder carries both, because Enter changing screens has to be asked
     for, not discovered. */
  'docs.filterPlaceholder': {
    en: 'Filter — Enter searches everything',
    ko: '필터 — Enter로 전체 검색',
    ja: '絞り込み — Enter で全体を検索',
  },
  'docs.filterLabel': {
    en: 'Filter documents',
    ko: '문서 필터',
    ja: 'ドキュメントを絞る',
  },
  'docs.filterEmpty': {
    en: 'No documents match',
    ko: '일치하는 문서가 없습니다',
    ja: '一致するドキュメントはありません',
  },
  'docs.filterEmptyHint': {
    en: 'Press Enter to search every issue and document instead.',
    ko: 'Enter를 누르면 이슈와 문서 전체에서 검색합니다.',
    ja: 'Enter を押すと、すべての課題とドキュメントを検索します。',
  },
  /* Labels: a chip on a row is a way to keep looking; the chip in the header is
     the narrowing that is currently on, and the way out of it. */
  'docs.labelFilterTo': {
    en: 'Show only documents labelled {label}',
    ko: '{label} 라벨이 붙은 문서만 보기',
    ja: '{label} ラベルのドキュメントだけを表示',
  },
  /* Nothing typed, so Enter has nothing to search with — the way out is the
     label, not the whole mirror. */
  'docs.filterEmptyLabelHint': {
    en: 'Nothing here carries the {label} label. Clear it to see the rest.',
    ko: '여기에는 {label} 라벨이 붙은 문서가 없습니다. 라벨을 해제하면 전체가 보입니다.',
    ja: 'ここには {label} ラベルのドキュメントはありません。解除すると残りが見えます。',
  },
  'docs.labelClear': {
    en: 'Clear the {label} label',
    ko: '{label} 라벨 해제',
    ja: '{label} ラベルを解除',
  },
  /* How many documents sit under a collapsed branch of the tree. */
  'docs.treeChildCount': {
    en: '{n} documents under this one',
    ko: '이 아래 문서 {n}개',
    ja: '配下 {n}件のドキュメント',
  },
  /* ── History (visits + searches from local.db) ── */
  'history.title': {
    en: 'History',
    ko: '히스토리',
    ja: '履歴',
  },
  'history.tabAll': {
    en: 'All',
    ko: '전체',
    ja: 'すべて',
  },
  'history.tabIssues': {
    en: 'Issues',
    ko: '이슈',
    ja: '課題',
  },
  'history.tabDocs': {
    en: 'Documents',
    ko: '문서',
    ja: 'ドキュメント',
  },
  'history.tabSearches': {
    en: 'Searches',
    ko: '검색',
    ja: '検索',
  },
  'history.groupToday': {
    en: 'Today',
    ko: '오늘',
    ja: '今日',
  },
  'history.groupYesterday': {
    en: 'Yesterday',
    ko: '어제',
    ja: '昨日',
  },
  'history.groupThisWeek': {
    en: 'This week',
    ko: '이번 주',
    ja: '今週',
  },
  'history.groupOlder': {
    en: 'Older',
    ko: '그 이전',
    ja: 'それ以前',
  },
  'history.visitCount': {
    en: '{n} times',
    ko: '{n}회',
    ja: '{n}回',
  },
  'history.empty': {
    en: 'Nothing viewed or searched yet',
    ko: '열람하거나 검색한 항목이 없습니다',
    ja: 'まだ閲覧も検索もしていません',
  },
  /* GDK-1054: the history request failed — not "nothing viewed yet". */
  'history.loadFailed': {
    en: 'Could not load history.',
    ko: '히스토리를 불러오지 못했습니다.',
    ja: '履歴を読み込めませんでした。',
  },
  'history.emptyHint': {
    en: 'Issues and documents you open, and searches you run, appear here.',
    ko: '연 이슈·문서와 실행한 검색이 여기에 모입니다.',
    ja: '開いた課題・ドキュメントと実行した検索がここに現れます。',
  },
  'history.filterEmpty': {
    en: 'No matches',
    ko: '일치하는 항목 없음',
    ja: '一致なし',
  },
  /* Same shape as docs.filterPlaceholder: the field narrows this screen;
     Enter leaves for the whole mirror, and the placeholder has to say so. */
  'history.filterPlaceholder': {
    en: 'Filter — Enter searches everything',
    ko: '필터 — Enter로 전체 검색',
    ja: '絞り込み — Enter で全体を検索',
  },
  'history.filterLabel': {
    en: 'Filter history',
    ko: '히스토리 필터',
    ja: '履歴を絞る',
  },
  'history.openAsList': {
    en: 'Show issues in list',
    ko: '이슈 목록으로 보기',
    ja: '課題を一覧で表示',
  },
  'history.loadMore': {
    en: 'Load more',
    ko: '더 보기',
    ja: 'さらに読み込む',
  },
  'history.searchResults': {
    en: '{n} results',
    ko: '결과 {n}개',
    ja: '{n}件の結果',
  },
  'history.searchOpened': {
    en: 'Opened {key}',
    ko: '{key} 열람',
    ja: '{key} を開きました',
  },
  /* ── Person panel (people axis) ── */
  'person.comments': {
    en: 'Comments',
    ko: '코멘트',
    ja: 'コメント',
  },
  'person.noComments': {
    en: 'No comments from this person in the mirror.',
    ko: '미러에 이 사람의 코멘트가 없습니다.',
    ja: 'ミラーにこの人のコメントはありません。',
  },
  'person.commentsFailed': {
    en: "Could not load this person's comments.",
    ko: '이 사람의 코멘트를 불러오지 못했습니다.',
    ja: 'この人のコメントを読み込めませんでした。',
  },
  'person.unlinked': {
    en: 'The mirror has no account id for this person yet, so their comments cannot be listed.',
    ko: '미러에 이 사람의 계정 id가 아직 없어 코멘트를 나열할 수 없습니다.',
    ja: 'ミラーにこの人のアカウント id がまだないため、コメントを一覧できません。',
  },
  'person.showingOf': {
    en: 'Showing the {n} most recent of {total}.',
    ko: '{total}건 중 최근 {n}건.',
    ja: '{total}件中、最近の {n}件を表示しています。',
  },
  'person.assigned': {
    en: 'Assigned',
    ko: '담당',
    ja: '担当',
  },
  'person.assignedTo': {
    en: 'Issues assigned to {name}',
    ko: '{name} 담당 이슈',
    ja: '{name} が担当する課題',
  },
  'person.reported': {
    en: 'Reported',
    ko: '보고',
    ja: '報告',
  },
  'person.reportedBy': {
    en: 'Issues reported by {name}',
    ko: '{name} 보고 이슈',
    ja: '{name} が報告した課題',
  },
  'person.docs': {
    en: 'Documents',
    ko: '문서',
    ja: 'ドキュメント',
  },
  'person.docsBy': {
    en: 'Documents written by {name}',
    ko: '{name} 작성 문서',
    ja: '{name} が書いたドキュメント',
  },
  /* ── QA impact ── */
  'qa.pass': {
    en: 'Pass',
    ko: '합격',
    ja: '合格',
  },
  'qa.fail': {
    en: 'Fail',
    ko: '실패',
    ja: '失敗',
  },
  'qa.block': {
    en: 'Block',
    ko: '블록',
    ja: 'ブロック',
  },
  'qa.retest': {
    en: 'Retest',
    ko: '재검증',
    ja: '再テスト',
  },
  'qa.inProgress': {
    en: 'In progress',
    ko: '진행',
    ja: '進行',
  },
  'qa.untested': {
    en: 'Untested',
    ko: '미검증',
    ja: '未テスト',
  },
  'qa.skip': {
    en: 'Skip',
    ko: '스킵',
    ja: 'スキップ',
  },
  'qa.openQase': {
    en: 'Open in Qase',
    ko: 'Qase에서 열기',
    ja: 'Qase で開く',
  },
  'qa.openSuite': {
    en: 'Open {path} in QA dashboard',
    ko: '{path} 영역을 QA 대시보드에서 열기',
    ja: '{path} を QA ダッシュボードで開く',
  },
  'qa.linkedTc': {
    en: '{n} linked TCs',
    ko: '연결 TC {n}개',
    ja: 'リンクされた TC {n}件',
  },
  /* ── QA field editor ── */
  'qaEditor.none': {
    en: 'None',
    ko: '없음',
    ja: 'なし',
  },
  'qaEditor.noVersions': {
    en: 'No versions available',
    ko: '선택 가능한 버전이 없습니다',
    ja: '利用できるバージョンはありません',
  },
  'qaEditor.searchVersion': {
    en: 'Search versions',
    ko: '버전 검색',
    ja: 'バージョンを検索',
  },
  'qaEditor.clearAssignee': {
    en: 'Clear assignee',
    ko: '담당자 해제',
    ja: '担当者をクリア',
  },
  'qaEditor.reporter': {
    en: 'Reporter',
    ko: '보고자',
    ja: '報告者',
  },
  /* ── General field editor (non-version option lists) ── */
  'fieldEditor.noOptions': {
    en: 'No options available',
    ko: '선택 가능한 값이 없습니다',
    ja: '利用できる選択肢はありません',
  },
  'fieldEditor.searchOptions': {
    en: 'Search',
    ko: '검색',
    ja: '検索',
  },
  'fieldEditor.parentSelf': {
    en: 'An issue cannot be its own parent.',
    ko: '이슈는 자기 자신을 상위 항목으로 둘 수 없습니다.',
    ja: '課題を自分自身の親にはできません。',
  },
} as const satisfies Record<string, Message>
