/*
 * Shared chrome, relative time, ADF fallbacks.
 * One key = {en, ko, ja}; omitting a locale is a type error.
 */
import type { Message } from '../types'

export const common = {
  /* ── Common ── */
  'common.closeEsc': {
    en: 'Close (Esc)',
    ko: '닫기 (Esc)',
    ja: '閉じる (Esc)',
  },
  'common.cancel': {
    en: 'Cancel',
    ko: '취소',
    ja: 'キャンセル',
  },
  'common.save': {
    en: 'Save',
    ko: '저장',
    ja: '保存',
  },
  'common.saving': {
    en: 'Saving…',
    ko: '저장 중…',
    ja: '保存中…',
  },
  'common.delete': {
    en: 'Delete',
    ko: '삭제',
    ja: '削除',
  },
  'common.change': {
    en: 'Change',
    ko: '변경',
    ja: '変更',
  },
  'common.apply': {
    en: 'Apply',
    ko: '적용',
    ja: '適用',
  },
  'common.retry': {
    en: 'Retry',
    ko: '다시 시도',
    ja: '再試行',
  },
  'common.loading': {
    en: 'Loading…',
    ko: '불러오는 중…',
    ja: '読み込み中…',
  },
  'common.searching': {
    en: 'Searching…',
    ko: '검색 중…',
    ja: '検索中…',
  },
  'common.none': {
    en: 'None',
    ko: '없음',
    ja: 'なし',
  },
  'common.defaultParen': {
    en: '(default)',
    ko: '(기본)',
    ja: '(既定)',
  },
  'common.unknown': {
    en: 'Unknown',
    ko: '알 수 없음',
    ja: '不明',
  },
  // GDK-590: the server's one-word judgement (account_type agent/app) shown
  // wherever a person's name is shown — comments, history, dev links.
  'common.bot': {
    en: 'Bot',
    ko: '봇',
    ja: 'ボット',
  },
  'common.me': {
    en: 'Me',
    ko: '나',
    ja: '自分',
  },
  'common.all': {
    en: 'All',
    ko: '전체',
    ja: 'すべて',
  },
  'common.setCredentials': {
    en: 'Set credentials',
    ko: '자격증명 설정',
    ja: '資格情報を設定',
  },
  'common.deselect': {
    en: 'Deselect',
    ko: '선택 해제',
    ja: '選択を解除',
  },
  'common.create': {
    en: 'Create',
    ko: '생성',
    ja: '作成',
  },
  'common.creating': {
    en: 'Creating…',
    ko: '생성 중…',
    ja: '作成中…',
  },
  'common.processing': {
    en: 'Working…',
    ko: '처리 중…',
    ja: '処理中…',
  },
  'common.verifying': {
    en: 'Verifying…',
    ko: '검증 중…',
    ja: '検証中…',
  },
  'common.noResults': {
    en: 'No results',
    ko: '결과 없음',
    ja: '結果なし',
  },
  'common.noValues': {
    en: 'No values',
    ko: '값 없음',
    ja: '値なし',
  },
  'common.unassigned': {
    en: 'Unassigned',
    ko: '미할당',
    ja: '未割り当て',
  },
  'common.unspecified': {
    en: 'Unspecified',
    ko: '미지정',
    ja: '未指定',
  },
  'common.unclassified': {
    en: 'Unclassified',
    ko: '미분류',
    ja: '未分類',
  },
  'common.version': {
    en: 'Version',
    ko: '버전',
    ja: 'バージョン',
  },
  'common.labels': {
    en: 'Labels',
    ko: '라벨',
    ja: 'ラベル',
  },
  'common.attachment': {
    en: 'Attachment',
    ko: '첨부',
    ja: '添付',
  },
  'common.attachmentFile': {
    en: 'Attachment',
    ko: '첨부 파일',
    ja: '添付ファイル',
  },
  'common.status': {
    en: 'Status',
    ko: '상태',
    ja: 'ステータス',
  },
  'common.assignee': {
    en: 'Assignee',
    ko: '담당자',
    ja: '担当者',
  },
  'common.reporter': {
    en: 'Reporter',
    ko: '보고자',
    ja: '報告者',
  },
  'common.priority': {
    en: 'Priority',
    ko: '우선순위',
    ja: '優先度',
  },
  'common.severity': {
    en: 'Severity',
    ko: '심각도',
    ja: '重大度',
  },
  // GDK-831: one label for the issue-type field. JA keeps 課題タイプ (the
  // catalog's 課題 canon — write.issueTypeRequired already uses it); this key
  // labels the same field in the create dialog and the type column.
  'common.type': {
    en: 'Type',
    ko: '유형',
    ja: '課題タイプ',
  },
  'common.epic': {
    en: 'Epic',
    ko: '에픽',
    ja: 'エピック',
  },
  'common.project': {
    en: 'Project',
    ko: '프로젝트',
    ja: 'プロジェクト',
  },
  'common.title': {
    en: 'Title',
    ko: '제목',
    ja: 'タイトル',
  },
  'common.description': {
    en: 'Description',
    ko: '설명',
    ja: '説明',
  },
  'common.due': {
    en: 'Due date',
    ko: '기한',
    ja: '期限',
  },
  'common.group': {
    en: 'Group',
    ko: '그룹',
    ja: 'グループ',
  },
  'common.detail': {
    en: 'Details',
    ko: '상세',
    ja: '詳細',
  },
  'common.feed': {
    en: 'Feed',
    ko: '피드',
    ja: 'フィード',
  },
  'common.watch': {
    en: 'Watch',
    ko: '워치',
    ja: 'ウォッチ',
  },
  'common.watching': {
    en: 'Watching',
    ko: '워치 중',
    ja: 'ウォッチ中',
  },
  'common.favorite': {
    en: 'Favorite',
    ko: '즐겨찾기',
    ja: 'お気に入り',
  },
  'common.unfavorite': {
    en: 'Remove favorite',
    ko: '즐겨찾기 해제',
    ja: 'お気に入りを解除',
  },
  'common.reply': {
    en: 'Reply',
    ko: '답글',
    ja: '返信',
  },
  'common.openInNewTab': {
    en: 'Open in new tab',
    ko: '새 탭에서 열기',
    ja: '新しいタブで開く',
  },
  /* ── Relative time ── */
  'time.justNow': {
    en: 'just now',
    ko: '방금',
    ja: 'たった今',
  },
  'time.yesterday': {
    en: 'yesterday',
    ko: '어제',
    ja: '昨日',
  },
  'time.minute': {
    en: '{n}m',
    ko: '{n}분',
    ja: '{n}分',
  },
  'time.hour': {
    en: '{n}h',
    ko: '{n}시간',
    ja: '{n}時間',
  },
  'time.day': {
    en: '{n}d',
    ko: '{n}일',
    ja: '{n}日',
  },
  'time.week': {
    en: '{n}w',
    ko: '{n}주',
    ja: '{n}週',
  },
  'time.month': {
    en: '{n}mo',
    ko: '{n}개월',
    ja: '{n}か月',
  },
  'time.year': {
    en: '{n}y',
    ko: '{n}년',
    ja: '{n}年',
  },
  'time.minuteAgo': {
    en: '{n}m ago',
    ko: '{n}분 전',
    ja: '{n}分前',
  },
  'time.hourAgo': {
    en: '{n}h ago',
    ko: '{n}시간 전',
    ja: '{n}時間前',
  },
  'time.dayAgo': {
    en: '{n}d ago',
    ko: '{n}일 전',
    ja: '{n}日前',
  },
  'time.weekAgo': {
    en: '{n}w ago',
    ko: '{n}주 전',
    ja: '{n}週前',
  },
  'time.monthAgo': {
    en: '{n}mo ago',
    ko: '{n}개월 전',
    ja: '{n}か月前',
  },
  'time.yearAgo': {
    en: '{n}y ago',
    ko: '{n}년 전',
    ja: '{n}年前',
  },
  'time.seenJustNow': {
    en: 'Seen just now',
    ko: '방금 봄',
    ja: 'たった今閲覧',
  },
  'time.seenAgo': {
    en: 'Seen {relative} ago',
    ko: '{relative} 전 봄',
    ja: '{relative}前に閲覧',
  },

  /* ── ADF fallbacks ── */
  'adf.unknownMention': {
    en: 'unknown',
    ko: '알 수 없음',
    ja: '不明',
  },
} as const satisfies Record<string, Message>
