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
  // The header's shell verb (GDK-1388): enter the shell bound to this issue,
  // or open a new one bound to it from its first prompt.
  'detail.openShell': {
    en: 'Open a shell for {key}',
    ko: '{key}의 셸 열기',
    ja: '{key} のシェルを開く',
  },
  'detail.enterShell': {
    en: 'Show the shell on {key}',
    ko: '{key}에 붙은 셸 보기',
    ja: '{key} のシェルを表示',
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
  // GDK-1704: the phone's empty-thread line (mobile Detail.svelte).
  'detail.noComments': {
    en: 'No comments yet — yours starts the thread.',
    ko: '아직 코멘트가 없습니다. 첫 코멘트가 스레드를 엽니다.',
    ja: 'まだコメントはありません。最初のコメントがスレッドを始めます。',
  },
  'detail.history': {
    en: 'History',
    ko: '변경 이력',
    ja: '履歴',
  },
  /* ── Resume card (spec w1-resume): what changed since this issue was last
   * opened. G1/G5 — the subject stays the issue, the form stays dim meta. */
  'detail.resume.sinceOpened': {
    en: 'Since last opened {ago}',
    ko: '마지막 열람 {ago}',
    ja: '最終閲覧 {ago}',
  },
  'detail.resume.statusChanges': {
    en: '{n} status changes',
    ko: '상태 변경 {n}건',
    ja: 'ステータス変更 {n}件',
  },
  'detail.resume.statusChangeOne': {
    en: '{n} status change',
    ko: '상태 변경 {n}건',
    ja: 'ステータス変更 {n}件',
  },
  'detail.resume.newComments': {
    en: '{n} new comments',
    ko: '새 코멘트 {n}건',
    ja: '新着コメント {n}件',
  },
  'detail.resume.newCommentOne': {
    en: '{n} new comment',
    ko: '새 코멘트 {n}건',
    ja: '新着コメント {n}件',
  },
  'detail.resume.assigneeChanged': {
    en: 'assignee changed',
    ko: '담당자 변경',
    ja: '担当者変更',
  },
  'detail.resume.otherChanges': {
    en: '{n} other changes',
    ko: '기타 변경 {n}건',
    ja: 'その他の変更 {n}件',
  },
  'detail.resume.otherChangeOne': {
    en: '{n} other change',
    ko: '기타 변경 {n}건',
    ja: 'その他の変更 {n}件',
  },
  /* The phone's card carries an explicit × (GDK-1495 A4 vision FIX): the
   * desk's card is dismissed by leaving the issue, the phone's sits in the
   * scroll and needs a way out that is not the sentence itself. Label only —
   * the desk has no such control and asks for no such key. */
  'detail.resume.dismiss': {
    en: 'Dismiss',
    ko: '닫기',
    ja: '閉じる',
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
  // {tracker} is the origin's brand name (Jira/Linear, `originTrackerName`)
  // and passes through untranslated (GDK-1149). The key keeps its name: it
  // is referenced by the command registry and tests as a wire id.
  'detail.wideOn': {
    en: 'Wide reading',
    ko: '넓게 읽기',
    ja: '広く読む',
  },
  'detail.wideOff': {
    en: 'Back to the list width',
    ko: '리스트 옆으로',
    ja: 'リストの横に戻す',
  },
  'detail.openJira': {
    en: 'Open in {tracker}',
    ko: '{tracker} 원본 열기',
    ja: '{tracker} で開く',
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
  // The paste's first line is the origin's own page for the key, so the
  // toast names the tracker that page belongs to. {tracker} is a brand name
  // (Jira/Linear) and passes through untranslated. The built-in tracker has
  // no origin page and keeps detail.linkCopied above.
  'detail.originLinkCopied': {
    en: '{tracker} link copied',
    ko: '{tracker} 링크를 복사했습니다',
    ja: '{tracker} のリンクをコピーしました',
  },
  // Toast copy rule (GDK-1588, which reversed GDK-1226's no-period rule): a
  // toast ends in a sentence terminator, an ellipsis, or an interpolation /
  // closing paren when the value is the tail — one ending per surface, all
  // locales. The catalog test derives the key set from every call spelling
  // (write.toast / this.toast / say), so this comment is a pointer, not the
  // contract.
  'clipboard.copyFailed': {
    en: 'Could not copy — the clipboard refused the write.',
    ko: '복사하지 못했습니다 — 클립보드가 쓰기를 거부했습니다.',
    ja: 'コピーできませんでした — クリップボードが書き込みを拒否しました。',
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
  // Coaching, M1 (THEORY.md "Opening an issue", G7): the durations chip's
  // hover-only baseline — the learned team p85 the stale mark already uses.
  // Visible text of the chip is unchanged; this is the "say why on hover" half.
  'detail.teamP85': {
    en: 'team p85 {d}d (issues finished in the last 90 days)',
    ko: '팀 p85 {d}일 (최근 90일간 완료된 이슈)',
    ja: 'チーム p85 {d}日（直近90日に完了した課題）',
  },
  'detail.priorityShort': {
    en: 'Prio {p}',
    ko: '우선순위 {p}',
    ja: '優先度 {p}',
  },
  'detail.severityShort': {
    en: 'Sev {s}',
    ko: '심각도 {s}',
    ja: '重大度 {s}',
  },
  'detail.refs': {
    en: 'References',
    ko: '참조',
    ja: '参照',
  },
  'detail.refRelates': {
    en: 'relates to',
    ko: '관련',
    ja: '関連',
  },
  'detail.refNotMirrored': {
    en: 'workspace {workspace} is not mirrored on this machine',
    ko: '이 컴퓨터에 {workspace} 워크스페이스 미러가 없습니다',
    ja: 'このマシンに {workspace} ワークスペースのミラーがありません',
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
    ja: '（空のコメント）',
  },
  'detail.replyToComment': {
    en: 'Reply to this comment',
    ko: '이 코멘트에 답글',
    ja: 'このコメントに返信',
  },
  // Coaching, M2 (THEORY.md "Writing a done-word comment", G2+G7): the verb
  // lives on the button; the hover states the fact that earned it. {status}
  // is the issue's current status name.
  'detail.moveToDone': {
    en: 'Move to done',
    ko: '완료로 이동',
    ja: '完了に移動',
  },
  'detail.moveToDoneWhy': {
    en: 'The latest comment says done; the status is still {status}',
    ko: '마지막 코멘트는 완료를 말하는데 상태는 아직 {status}',
    ja: '最新のコメントは完了を示しているが、ステータスはまだ {status}',
  },
  // Coaching, M3 (THEORY.md "Just before moving to in-progress", G6): the
  // count is the whole message — no warning icon, no confirm. {n} is the
  // in-progress issue count for this account.
  'detail.wipCount': {
    en: 'in progress: {n}',
    ko: '진행 중 {n}건',
    ja: '進行中 {n}件',
  },
  'detail.wipCountWhy': {
    en: 'Open in-progress issues assigned to this account',
    ko: '이 계정에 배정된 진행 중 이슈',
    ja: 'このアカウントに割り当てられた進行中の課題',
  },
  // Coaching, M4 (THEORY.md "Choosing a priority", G4): the distribution's
  // hover basis. No judgement text — the reader decides.
  'detail.priorityShare': {
    en: '{n} of the {total} open issues in this view ({pct}%)',
    ko: '이 뷰의 미해결 이슈 {total}건 중 {n}건 ({pct}%)',
    ja: 'このビューの未解決課題 {total}件のうち {n}件（{pct}%）',
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
  /*
   * Phone attachment affordances (GDK-1503). The packaged app has no opener
   * plugin and no outbound host but the paired endpoint, so a video's bytes
   * arrive on tap through the same road images use and a file chip can only
   * hand the reader a link. These say which of those is happening.
   */
  'detail.attachmentTapToPlay': {
    en: 'Tap to play',
    ko: '눌러서 재생',
    ja: 'タップで再生',
  },
  'detail.attachmentLoading': {
    en: 'Loading…',
    ko: '불러오는 중…',
    ja: '読み込み中…',
  },
  'detail.attachmentLoadFailed': {
    en: 'Could not load — tap to retry',
    ko: '불러오지 못했습니다 — 다시 누르면 재시도합니다',
    ja: '読み込めませんでした — タップで再試行します',
  },
  // The chip's title, and the only promise the phone can keep for a file it
  // cannot open or save: the link goes to the clipboard.
  'detail.attachmentCopiesLink': {
    en: 'Copies the link',
    ko: '링크를 복사합니다',
    ja: 'リンクをコピーします',
  },
  // The full-screen image viewer's own control. `common` has no Close and
  // this round does not own that file.
  'detail.viewerClose': {
    en: 'Close',
    ko: '닫기',
    ja: '閉じる',
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
  // Weekly retro view (GDK-1660): the CLI table's rows, as labels.
  'retro.title': { en: 'Weekly retro', ko: '주간 회고', ja: '週次ふりかえり' },
  'retro.thisWeek': { en: 'this week', ko: '이번 주', ja: '今週' },
  // The partial column's badge under sprint columns — "this week" on a
  // column headed "Sprint 42" names the wrong unit (GDK-1693).
  'retro.thisSprint': { en: 'running', ko: '진행 중', ja: '進行中' },
  'retro.range4w': { en: '4 weeks', ko: '4주', ja: '4 週' },
  'retro.range8w': { en: '8 weeks', ko: '8주', ja: '8 週' },
  'retro.range12w': { en: '12 weeks', ko: '12주', ja: '12 週' },
  'retro.sessions': { en: 'Sessions', ko: '세션', ja: 'セッション' },
  'retro.resume': { en: 'Resume time (median)', ko: '재개 시간(중앙값)', ja: '再開までの時間 (中央値)' },
  'retro.closed': { en: 'Closed', ko: '완료', ja: '完了' },
  'retro.cycleP50': { en: 'Cycle time p50', ko: '사이클 타임 p50', ja: 'サイクルタイム p50' },
  'retro.cycleP85': { en: 'Cycle time p85', ko: '사이클 타임 p85', ja: 'サイクルタイム p85' },
  'retro.inProgress': { en: 'In progress', ko: '진행 중', ja: '進行中' },
  'retro.wipAge': { en: 'Oldest in progress', ko: '가장 오래된 진행 중', ja: '最も古い進行中' },
  'retro.mismatch': { en: 'Mismatch', ko: '불일치', ja: '不一致' },
  // The definitions under each row label (GDK-1692). These used to be read
  // straight out of `doc.definitions`, which is `internal/retro`'s footer —
  // English, because the CLI is English — so a Korean frame stood on eight
  // rows of English prose. The server keeps that object (it is the CLI
  // footer and the --json contract); the view reads these instead. The
  // English here is the footer's own sentence, so the two surfaces still
  // say the same thing. `{gap}` comes from the report (`session_gap`) and
  // is never written into a translation: --session-gap moves it.
  // What one column is, substituted into every definition below as {bucket}
  // (GDK-1693): the same sentences describe a week or a sprint, and a footer
  // that says "week" beside columns headed "Sprint 42" is describing a table
  // that is not on the screen.
  'retro.bucket.week': { en: 'week', ko: '주', ja: '週' },
  'retro.bucket.sprint': { en: 'sprint', ko: '스프린트', ja: 'スプリント' },
  'retro.bySprint': { en: 'By sprint', ko: '스프린트별', ja: 'スプリント別' },
  'retro.def.sessions': {
    en: 'person reads — visits with source ui or unknown — split where the gap to the previous read exceeds {gap}; a session counts in the {bucket} it started',
    ko: '사람이 읽은 기록 — source 가 ui 이거나 미상인 방문 — 을 직전 읽기와의 간격이 {gap} 을 넘는 곳에서 끊은 것. 세션은 시작한 {bucket}에 센다',
    ja: '人が読んだ記録 — source が ui または不明の閲覧 — を、直前の閲覧との間隔が {gap} を超えた地点で区切ったもの。セッションは開始した{bucket}に数える',
  },
  'retro.def.resume': {
    en: 'time from a session start to its first write — a changelog entry or comment by the configured account — counted only before the next session starts; sessions without a write are excluded (the cell shows k of n)',
    ko: '세션이 시작된 뒤 첫 쓰기 — 설정된 계정의 변경 이력 항목이나 코멘트 — 까지 걸린 시간. 다음 세션이 시작되기 전까지만 세고, 쓰기가 없던 세션은 뺀다(칸에 n 중 k 로 표시)',
    ja: 'セッション開始から最初の書き込み — 設定したアカウントによる変更履歴かコメント — までの時間。次のセッションが始まるまでのみ数え、書き込みのないセッションは除く (セルは n 件中 k 件を示す)',
  },
  'retro.def.closed': {
    en: 'issues that entered a done status during the {bucket} (status ids resolved through status_catalog)',
    ko: '그 {bucket}에 완료 상태로 들어간 이슈(상태 id 는 status_catalog 로 판정)',
    ja: 'その{bucket}に完了ステータスへ入った課題 (ステータス id は status_catalog で解決)',
  },
  'retro.def.cycleP50': {
    en: 'median of cycle_hours — first entry into progress to the latest done entry — in days, over issues resolved during the {bucket} that are done now and were never reopened (reopen_count = 0)',
    ko: 'cycle_hours — 진행 중에 처음 들어간 때부터 마지막 완료까지 — 의 중앙값, 일 단위. 그 {bucket}에 해결됐고 지금도 완료이며 리오픈된 적 없는(reopen_count = 0) 이슈만',
    ja: 'cycle_hours — 最初に進行中へ入った時点から最後の完了まで — の中央値 (日)。その{bucket}に解決され、現在も完了で、再オープンされたことがない (reopen_count = 0) 課題のみ',
  },
  'retro.def.cycleP85': {
    en: 'nearest-rank 85th percentile of cycle_hours — first entry into progress to the latest done entry — in days, over issues resolved during the {bucket} that are done now and were never reopened (reopen_count = 0)',
    ko: 'cycle_hours — 진행 중에 처음 들어간 때부터 마지막 완료까지 — 의 85 백분위(최근접 순위), 일 단위. 그 {bucket}에 해결됐고 지금도 완료이며 리오픈된 적 없는(reopen_count = 0) 이슈만',
    ja: 'cycle_hours — 最初に進行中へ入った時点から最後の完了まで — の 85 パーセンタイル (最近接順位、日)。その{bucket}に解決され、現在も完了で、再オープンされたことがない (reopen_count = 0) 課題のみ',
  },
  'retro.def.inProgress': {
    en: 'issues in progress at {bucket} end',
    ko: '그 {bucket}이 끝나는 시점에 진행 중이던 이슈',
    ja: '{bucket}終了時点で進行中だった課題',
  },
  'retro.def.wipAge': {
    en: 'the oldest in-progress issue at {bucket} end, in days',
    ko: '그 {bucket}이 끝나는 시점에 가장 오래 진행 중이던 이슈의 경과일',
    ja: '{bucket}終了時点で最も長く進行中だった課題の経過日数',
  },
  'retro.def.mismatch': {
    en: "comments claiming the work is finished on issues not done now (heuristic: a done-word standing on its own, negations and quoted text excluded; only comments newer than the issue's last status change count)",
    ko: '지금 완료가 아닌 이슈에 달린, 일이 끝났다고 말하는 코멘트(휴리스틱: 완료를 뜻하는 낱말이 홀로 선 경우만, 부정문과 인용문은 제외. 이슈의 마지막 상태 변경보다 나중에 달린 코멘트만 센다)',
    ja: '現在完了していない課題に付いた、作業が終わったと述べるコメント (ヒューリスティック: 完了を表す語が単独で立つ場合のみ、否定文と引用文は除く。課題の最後のステータス変更より後のコメントだけを数える)',
  },
  'retro.empty': { en: 'No sessions in this range', ko: '이 기간에는 세션이 없습니다', ja: 'この期間にセッションはありません' },
  'retro.loadFailed': { en: 'Could not load the retro.', ko: '회고를 불러오지 못했습니다.', ja: 'ふりかえりを読み込めませんでした。' },
  'retro.openIssues': { en: 'Open these issues', ko: '이 이슈들 열기', ja: 'これらの課題を開く' },
  'retro.truncated': { en: '(first 500)', ko: '(앞 500개)', ja: '(先頭 500 件)' },
  // GDK-1712: the summary strip, the per-cell step, and the toggle that
  // unfolds the definitions the table used to print under every label.
  'retro.definitions': { en: 'Definitions', ko: '정의', ja: '定義' },
  'retro.vsPrevious': { en: 'change from the previous bucket', ko: '직전 구간 대비 변화', ja: '直前の区間からの変化' },
  // GDK-1713: the sprint cut needs a board when several carry sprints. The
  // sentence under the heading is the server's own — it names the boards —
  // so these two are the heading and the picker's label, nothing more.
  'retro.board': { en: 'Board', ko: '보드', ja: 'ボード' },
  'retro.boardPick': { en: 'Pick a board', ko: '보드 선택', ja: 'ボードを選択' },
  'retro.pickBoardHint': {
    en: 'Sprint windows from two boards overlap, so one report cannot be both. Choose a board above.',
    ko: '보드가 다르면 스프린트 기간이 겹쳐서 한 보고서로 묶을 수 없습니다. 위에서 보드를 고르세요.',
    ja: 'ボードが違えばスプリントの期間が重なるため、ひとつの報告にまとめられません。上でボードを選んでください。',
  },
  'retro.noSprintsHint': {
    en: 'Sprint columns need a board with sprints — a Jira Software board, a Linear cycle, or a sprint in the built-in tracker.',
    ko: '스프린트 열을 그리려면 스프린트가 있는 보드가 필요합니다. Jira Software 보드, Linear 사이클, 또는 내장 트래커의 스프린트.',
    ja: 'スプリントの列には、スプリントを持つボードが要ります。Jira Software のボード、Linear のサイクル、または内蔵トラッカーのスプリントです。',
  },
  'retro.pickBoard': {
    en: 'Several boards have sprints — pick one',
    ko: '스프린트가 있는 보드가 여럿입니다. 하나를 고르세요',
    ja: 'スプリントを持つボードが複数あります。ひとつ選んでください',
  },
  'retro.noSprints': {
    en: 'This workspace has no sprints',
    ko: '이 워크스페이스에는 스프린트가 없습니다',
    ja: 'このワークスペースにスプリントはありません',
  },
  /*
   * GDK-1721..1726: the retro as materials rather than a table.
   *
   * The sentence, the sections that draw what happened, and the paragraph
   * that says how to read each one. The three-line explanations are written
   * per locale rather than translated clause for clause — an explanation
   * that reads like a translation is one nobody finishes.
   */
  'retro.sentence': {
    en: 'Closed {closed} ({unplanned} unplanned), reopened {reopened}, oldest in progress {age}.',
    ko: '닫힌 것 {closed}(계획 외 {unplanned}), 되돌아온 것 {reopened}, 가장 오래된 진행 중 {age}.',
    ja: '完了 {closed}（計画外 {unplanned}）、戻ってきたもの {reopened}、最も古い進行中 {age}。',
  },
  'retro.sentenceSprint': {
    en: 'Closed {closed} ({unplanned} unplanned), reopened {reopened}, oldest in progress {age}, added after sprint start {added}.',
    ko: '닫힌 것 {closed}(계획 외 {unplanned}), 되돌아온 것 {reopened}, 가장 오래된 진행 중 {age}, 스프린트 시작 후 들어온 것 {added}.',
    ja: '完了 {closed}（計画外 {unplanned}）、戻ってきたもの {reopened}、最も古い進行中 {age}、スプリント開始後に入ったもの {added}。',
  },
  /*
   * The same sentence without the reopen clause (GDK-1690). An origin that
   * supplies no changelog cannot count reopens — the number is 0 forever —
   * and "reopened 0" reads as "this team has no regressions" when nobody can
   * tell. Written out per locale rather than assembled from fragments,
   * because a clause removed from the middle of a sentence has to leave
   * grammar behind in each language, not a comma.
   */
  'retro.sentenceNoReopen': {
    en: 'Closed {closed} ({unplanned} unplanned), oldest in progress {age}.',
    ko: '닫힌 것 {closed}(계획 외 {unplanned}), 가장 오래된 진행 중 {age}.',
    ja: '完了 {closed}（計画外 {unplanned}）、最も古い進行中 {age}。',
  },
  'retro.sentenceSprintNoReopen': {
    en: 'Closed {closed} ({unplanned} unplanned), oldest in progress {age}, added after sprint start {added}.',
    ko: '닫힌 것 {closed}(계획 외 {unplanned}), 가장 오래된 진행 중 {age}, 스프린트 시작 후 들어온 것 {added}.',
    ja: '完了 {closed}（計画外 {unplanned}）、最も古い進行中 {age}、スプリント開始後に入ったもの {added}。',
  },
  'retro.actions.title': { en: 'Decided last time', ko: '지난번에 정한 것', ja: '前回決めたこと' },
  'retro.actions.hint': {
    en: 'Issues labelled retro-action stand here, with the number they named then and now.',
    ko: 'retro-action 라벨을 단 이슈가 여기 섭니다. 그때 가리킨 수와 지금 수가 함께 보입니다.',
    ja: 'retro-action ラベルを付けた課題がここに並びます。そのとき指した数と今の数が並びます。',
  },
  'retro.actions.thenNow': { en: 'then {then} → now {now}', ko: '그때 {then} → 지금 {now}', ja: 'そのとき {then} → 今 {now}' },
  'retro.aging.title': { en: 'Aging work in progress', ko: '진행 중인 일의 나이', ja: '進行中の仕事の古さ' },
  'retro.aging.more': { en: '{n} more', ko: '{n}개 더', ja: 'ほか {n} 件' },
  'retro.aging.empty': { en: 'Nothing in progress.', ko: '진행 중인 것이 없습니다.', ja: '進行中のものはありません。' },
  'retro.events.title': { en: 'What happened', ko: '이 구간에 있었던 일', ja: 'この区間に起きたこと' },
  'retro.events.day': { en: '{date} · {n}', ko: '{date} · {n}건', ja: '{date} · {n} 件' },
  'retro.events.none': { en: 'Nothing recorded in this window.', ko: '이 구간에는 기록이 없습니다.', ja: 'この区間には記録がありません。' },
  'retro.surprises.title': { en: 'Surprises', ko: '놀라운 것', ja: '意外だったこと' },
  'retro.surprise.reopened': { en: 'came back', ko: '되돌아왔다', ja: '戻ってきた' },
  'retro.surprise.reversal': { en: 'went back and forth', ko: '상태를 오갔다', ja: 'ステータスを行き来した' },
  'retro.surprise.added_after_start': { en: 'joined after the start', ko: '시작한 뒤 들어왔다', ja: '開始後に入ってきた' },
  'retro.surprise.carried': { en: 'carried over', ko: '넘어왔다', ja: '持ち越された' },
  'retro.closed.title': { en: 'What closed', ko: '닫힌 것', ja: '完了したもの' },
  'retro.closed.byType': { en: 'By type', ko: '유형별', ja: '種類別' },
  'retro.closed.byEpic': { en: 'By epic', ko: '에픽별', ja: 'エピック別' },
  'retro.closed.noEpic': { en: '(no epic)', ko: '(에픽 없음)', ja: '(エピックなし)' },
  'retro.closed.unplanned': { en: 'Unplanned {n}', ko: '계획 외 {n}', ja: '計画外 {n}' },
  'retro.closed.cycle': { en: 'Cycle time of each', ko: '하나하나의 사이클 타임', ja: '一件ごとのサイクルタイム' },
  'retro.closed.clipped': { en: '{n} above the top', ko: '위쪽 밖 {n}개', ja: '上端の外に {n} 件' },
  'retro.closed.none': { en: 'Nothing closed in this window.', ko: '이 구간에 닫힌 것이 없습니다.', ja: 'この区間に完了したものはありません。' },
  'retro.seen.title': { en: 'Seen and moved', ko: '본 것과 움직인 것', ja: '見たものと動いたもの' },
  'retro.seen.notMoved': { en: 'Opened, never moved {n}', ko: '열었지만 안 움직인 것 {n}', ja: '開いたが動かなかったもの {n}' },
  'retro.moved.notSeen': { en: 'Moved, never opened {n}', ko: '움직였지만 안 본 것 {n}', ja: '動いたが見ていないもの {n}' },
  'retro.table.title': { en: 'Every number', ko: '숫자 표 전체', ja: '数値表のすべて' },
  'retro.table.show': { en: 'Show', ko: '펼치기', ja: '開く' },
  'retro.table.hide': { en: 'Hide', ko: '접기', ja: '畳む' },
  // The three lines each section unfolds when the definitions are on: what
  // it is, why a retro looks at it, how to read it.
  'retro.explain.aging.what': {
    en: 'How many days each in-progress issue has stood where it is. One bar is one issue.',
    ko: '진행 중인 이슈가 지금 상태에 머문 날수입니다. 막대 하나가 이슈 하나입니다.',
    ja: '進行中の課題が今の状態にとどまっている日数です。棒ひとつが課題ひとつです。',
  },
  'retro.explain.aging.why': {
    en: 'Cycle time is what finished work already cost. Age is the one figure you can still change.',
    ko: '끝난 일의 사이클 타임은 뒤늦은 지표이고, 나이는 아직 손댈 수 있는 유일한 앞선 지표입니다.',
    ja: '終わった仕事のサイクルタイムは後追いの指標です。古さは今からまだ動かせる唯一の指標です。',
  },
  'retro.explain.aging.how': {
    en: 'The bars past the p85 line are next week\u2019s first look. If the line itself drifts right, flow has slowed.',
    ko: 'p85 선을 넘은 막대가 다음 주에 먼저 볼 것입니다. 선 자체가 오른쪽으로 밀리면 흐름이 느려진 것입니다.',
    ja: 'p85 の線を越えた棒が来週まず見るものです。線そのものが右へ動いたなら、流れが遅くなっています。',
  },
  'retro.explain.events.what': {
    en: 'One column per day, as tall as the day was busy. Starts and finishes take the status colours; everything else is grey.',
    ko: '하루가 한 칸이고, 칸 높이가 그날의 사건 수입니다. 시작과 완료는 상태 색을 쓰고 나머지는 회색입니다.',
    ja: '1 日が 1 本で、高さがその日の件数です。開始と完了はステータスの色、ほかは灰色です。',
  },
  'retro.explain.events.why': {
    en: 'Totals hide rhythm. A week where everything landed on the last day counts the same as a steady one.',
    ko: '합계는 리듬을 지웁니다. 마지막 날에 다 몰린 주와 고르게 흘러간 주는 숫자가 같습니다.',
    ja: '合計はリズムを消します。最終日にすべてが集まった週も、平らに流れた週も、数字は同じです。',
  },
  'retro.explain.events.how': {
    en: 'Look for the gaps and the spike. The surprises below are the same window, named.',
    ko: '빈 자리와 솟은 자리를 봅니다. 아래 목록은 같은 구간을 이름으로 적은 것입니다.',
    ja: '空いた場所と跳ねた場所を見ます。下の一覧は同じ区間を名前で書いたものです。',
  },
  'retro.explain.closed.what': {
    en: 'The bucket\u2019s closures cut by type and by epic, and the cycle time of each one as a dot.',
    ko: '이 구간에 닫힌 것을 유형별·에픽별로 나눈 것, 그리고 하나하나의 사이클 타임을 점으로 찍은 것입니다.',
    ja: 'この区間に完了したものを種類別・エピック別に分けたもの、そして一件ごとのサイクルタイムを点で置いたものです。',
  },
  'retro.explain.closed.why': {
    en: 'p50 and p85 are two numbers standing for a shape. The shape is what tells you whether the tail is one issue or a habit.',
    ko: 'p50 과 p85 는 분포를 대신하는 두 숫자입니다. 꼬리가 이슈 하나인지 습관인지는 분포를 봐야 압니다.',
    ja: 'p50 と p85 は分布を代表する 2 つの数字です。尾がひとつの課題なのか癖なのかは、分布を見ないと分かりません。',
  },
  'retro.explain.closed.how': {
    en: 'Dots bunched under p50 with a few far above is a healthy week with an outlier. A flat spread is not.',
    ko: '점이 p50 아래 모이고 몇 개만 위로 튀면 이상치가 있는 건강한 구간입니다. 고르게 퍼져 있으면 아닙니다.',
    ja: '点が p50 の下に集まり、いくつかだけ上へ跳ねているなら、外れ値のある健全な区間です。平らに散っていればそうではありません。',
  },
  'retro.explain.seen.what': {
    en: 'Two counts from your own reading history against the bucket\u2019s changelog.',
    ko: '내가 읽은 기록과 이 구간의 변경 이력을 맞대어 센 두 숫자입니다.',
    ja: '自分が読んだ記録とこの区間の変更履歴を突き合わせて数えた 2 つの数字です。',
  },
  'retro.explain.seen.why': {
    en: 'Attention and movement come apart quietly. Both directions are worth a sentence in a retro.',
    ko: '주의와 움직임은 조용히 어긋납니다. 어느 쪽이든 회고에서 한 문장 값은 합니다.',
    ja: '注意と動きは静かにずれます。どちらの向きも、ふりかえりで一言の価値があります。',
  },
  'retro.explain.seen.how': {
    en: 'Opened but never moved is where the week went. Moved but never opened is what moved without you.',
    ko: '열었지만 안 움직인 것에 그 주가 갔습니다. 움직였지만 안 본 것은 나 없이 움직인 것입니다.',
    ja: '開いたのに動かなかったものに、その週が消えています。動いたのに見ていないものは、自分抜きで動いたものです。',
  },
  'retro.explain.actions.what': {
    en: 'Issues labelled retro-action, with the metric each one named, then and now.',
    ko: 'retro-action 라벨이 붙은 이슈와, 그 이슈가 가리킨 지표의 그때 값과 지금 값입니다.',
    ja: 'retro-action ラベルの付いた課題と、その課題が指した指標の当時の値と今の値です。',
  },
  'retro.explain.actions.why': {
    en: 'A retro that never reads its last one is a meeting, not a loop.',
    ko: '지난 회고를 다시 읽지 않는 회고는 고리가 아니라 회의일 뿐입니다.',
    ja: '前回を読み返さないふりかえりは、ループではなく会議です。',
  },
  'retro.explain.actions.how': {
    en: 'A decision whose number has not moved is either the wrong decision or one nobody did.',
    ko: '수가 그대로인 결정은 잘못 고른 결정이거나 아무도 하지 않은 결정입니다.',
    ja: '数字が動いていない決定は、選び方を誤ったか、誰もやらなかったかのどちらかです。',
  },
  'retro.explain.table.what': {
    en: 'Every metric against every bucket — the report the CLI prints.',
    ko: '모든 지표를 모든 구간에 대해 적은 표입니다. CLI 가 찍는 그 보고서입니다.',
    ja: 'すべての指標をすべての区間について並べた表です。CLI が出力する報告そのものです。',
  },
  'retro.explain.table.why': {
    en: 'The sections above are readings of these numbers. This is where you check one.',
    ko: '위의 섹션들은 이 숫자를 읽은 것입니다. 하나를 확인하고 싶을 때 여기를 엽니다.',
    ja: '上の各節はこの数字を読んだものです。ひとつを確かめたいときにここを開きます。',
  },
  'retro.explain.table.how': {
    en: 'Folded by default, because eight rows against twelve columns is not a first read.',
    ko: '기본은 접힘입니다. 여덟 행에 열두 열은 처음 읽을 것이 아니기 때문입니다.',
    ja: '既定では畳んであります。8 行 × 12 列は、最初に読むものではないからです。',
  },
  // GDK-1150: the phone Detail header's byline fragments. Lowercase en on
  // purpose — they read as run-ins inside the meta line, not sentences.
  // ko/ja are en placeholders until the lead writes them (see the
  // ALLOWED_BYTE_EQUAL entry in catalog.test.ts).
  'detail.updatedWhen': {
    en: 'updated {when}',
    ko: '{when} 갱신',
    ja: '{when} 更新',
  },
  'detail.byline': {
    en: 'by {name}',
    ko: '{name}',
    ja: '{name}',
  },
} as const satisfies Record<string, Message>
